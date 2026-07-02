package consensus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type CompositeVerifierDeps struct {
	TaskDelegate   TaskDelegate
	Clock          Clock
	IDFactory      IDFactory
	ArtifactDir    string
	PerTaskTimeout time.Duration
	RetryAttempts  int
}

type CompositeVerifier struct {
	deps CompositeVerifierDeps
}

func NewCompositeVerifier(deps CompositeVerifierDeps) *CompositeVerifier {
	if deps.Clock == nil {
		deps.Clock = SystemClock{}
	}
	return &CompositeVerifier{deps: deps}
}

func (v *CompositeVerifier) Run(ctx context.Context, req VerificationRequest) ([]VerificationResult, error) {
	results := make([]VerificationResult, 0)
	for _, check := range req.Request.VerificationPolicy.RequiredChecks {
		result := v.runCheck(ctx, req, check)
		results = append(results, result)
	}
	if req.Request.VerificationPolicy.AllowSemanticVerifier && req.Request.Roles.SemanticVerifier != "" && v.deps.TaskDelegate != nil {
		result := v.runSemanticVerification(ctx, req)
		results = append(results, result...)
	}
	return results, nil
}

func (v *CompositeVerifier) runCheck(ctx context.Context, req VerificationRequest, check VerificationCheck) VerificationResult {
	switch check.Kind {
	case "workspace_snapshot":
		return v.runWorkspaceSnapshotCheck(req, check)
	case "allowed_paths":
		return v.runAllowedPathsCheck(req, check)
	case "git_diff_paths":
		return v.runGitDiffPathsCheck(ctx, req, check)
	case "benchmark_threshold":
		return v.runBenchmarkThresholdCheck(ctx, req, check)
	case "command":
		return v.runCommandCheck(ctx, req, check)
	default:
		return VerificationResult{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
			CheckName:      check.Name,
			Kind:           check.Kind,
			Status:         VerificationStatusInconclusive,
			Summary:        "unsupported verification check kind: " + check.Kind,
		}
	}
}

func (v *CompositeVerifier) runWorkspaceSnapshotCheck(req VerificationRequest, check VerificationCheck) VerificationResult {
	result := VerificationResult{
		VerificationID: v.deps.IDFactory.NewEntityID("verify"),
		ClaimID:        req.Claim.ClaimID,
		CheckName:      check.Name,
		Kind:           check.Kind,
	}
	snapshot := req.Request.TaskSpec.WorkspaceSnapshot
	if snapshot == nil {
		result.Status = VerificationStatusInconclusive
		result.FailureCode = "workspace_snapshot_missing"
		result.Summary = "workspace snapshot 未提供"
		return result
	}
	if snapshot.Root == "" {
		result.Status = VerificationStatusInconclusive
		result.FailureCode = "workspace_root_missing"
		result.Summary = "workspace snapshot 缺少 root"
		return result
	}
	if snapshot.Hash != "" && len(snapshot.Paths) > 0 {
		hash, err := computePathsHash(snapshot.Root, snapshot.Paths)
		if err != nil {
			result.Status = VerificationStatusInconclusive
			result.FailureCode = "workspace_hash_compute_failed"
			result.Summary = "计算 workspace hash 失败: " + err.Error()
			return result
		}
		if hash != snapshot.Hash {
			result.Status = VerificationStatusFailed
			result.FailureCode = "workspace_hash_mismatch"
			result.Summary = fmt.Sprintf("workspace hash 不匹配: got=%s want=%s", hash, snapshot.Hash)
			return result
		}
		result.Status = VerificationStatusPassed
		result.Summary = "workspace hash 校验通过"
		return result
	}
	if snapshot.Revision != "" {
		cmd := exec.Command("git", "-C", snapshot.Root, "rev-parse", "HEAD")
		body, err := cmd.Output()
		if err != nil {
			result.Status = VerificationStatusInconclusive
			result.FailureCode = "workspace_revision_read_failed"
			result.Summary = "读取 git revision 失败: " + err.Error()
			return result
		}
		current := strings.TrimSpace(string(body))
		if current != snapshot.Revision {
			result.Status = VerificationStatusFailed
			result.FailureCode = "workspace_revision_mismatch"
			result.Summary = fmt.Sprintf("workspace revision 不匹配: got=%s want=%s", current, snapshot.Revision)
			return result
		}
		result.Status = VerificationStatusPassed
		result.Summary = "workspace revision 校验通过"
		return result
	}
	result.Status = VerificationStatusInconclusive
	result.FailureCode = "workspace_snapshot_not_actionable"
	result.Summary = "workspace snapshot 缺少可执行校验字段"
	return result
}

func (v *CompositeVerifier) runAllowedPathsCheck(req VerificationRequest, check VerificationCheck) VerificationResult {
	result := VerificationResult{
		VerificationID: v.deps.IDFactory.NewEntityID("verify"),
		ClaimID:        req.Claim.ClaimID,
		CheckName:      check.Name,
		Kind:           check.Kind,
	}
	allowed := req.Request.TaskSpec.Constraints.AllowedPaths
	if len(allowed) == 0 {
		result.Status = VerificationStatusInconclusive
		result.FailureCode = "allowed_paths_missing"
		result.Summary = "未配置 allowed_paths"
		return result
	}
	touched := extractTouchedPaths(req.Claim.Metadata)
	if len(touched) == 0 {
		result.Status = VerificationStatusInconclusive
		result.FailureCode = "touched_paths_missing"
		result.Summary = "claim 未声明 touchedPaths"
		return result
	}
	violations := make([]string, 0)
	for _, path := range touched {
		if !matchesAllowedPath(path, allowed) {
			violations = append(violations, path)
		}
	}
	if len(violations) > 0 {
		result.Status = VerificationStatusFailed
		result.FailureCode = "path_out_of_scope"
		result.Summary = "发现越界路径: " + strings.Join(violations, ", ")
		return result
	}
	result.Status = VerificationStatusPassed
	result.Summary = "touchedPaths 均在允许范围内"
	return result
}

func (v *CompositeVerifier) runGitDiffPathsCheck(ctx context.Context, req VerificationRequest, check VerificationCheck) VerificationResult {
	result := VerificationResult{
		VerificationID: v.deps.IDFactory.NewEntityID("verify"),
		ClaimID:        req.Claim.ClaimID,
		CheckName:      check.Name,
		Kind:           check.Kind,
	}
	snapshot := req.Request.TaskSpec.WorkspaceSnapshot
	if snapshot == nil || strings.TrimSpace(snapshot.Root) == "" {
		result.Status = VerificationStatusInconclusive
		result.FailureCode = "workspace_root_missing"
		result.Summary = "git diff 检查缺少 workspace root"
		return result
	}
	baseRevision := strings.TrimSpace(check.BaseRevision)
	if baseRevision == "" {
		baseRevision = strings.TrimSpace(snapshot.Revision)
	}
	if baseRevision == "" {
		result.Status = VerificationStatusInconclusive
		result.FailureCode = "base_revision_missing"
		result.Summary = "git diff 检查缺少 base revision"
		return result
	}
	cmd := exec.CommandContext(ctx, "git", "-C", snapshot.Root, "diff", "--name-only", baseRevision)
	body, err := cmd.Output()
	if err != nil {
		result.Status = VerificationStatusInconclusive
		result.FailureCode = "git_diff_failed"
		result.Summary = "git diff 执行失败: " + err.Error()
		return result
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	changed := make([]string, 0, len(lines))
	for _, line := range lines {
		if item := strings.TrimSpace(line); item != "" {
			changed = append(changed, item)
		}
	}
	if len(changed) == 0 {
		result.Status = VerificationStatusInconclusive
		result.FailureCode = "git_diff_empty"
		result.Summary = "git diff 没有检测到任何变更"
		return result
	}
	allowed := check.Paths
	if len(allowed) == 0 {
		allowed = req.Request.TaskSpec.Constraints.AllowedPaths
	}
	if len(allowed) == 0 {
		result.Status = VerificationStatusPassed
		result.Summary = fmt.Sprintf("git diff 检测到 %d 个变更文件", len(changed))
		result.Metadata = map[string]any{"changedPaths": changed}
		return result
	}
	violations := make([]string, 0)
	for _, path := range changed {
		if !matchesAllowedPath(path, allowed) {
			violations = append(violations, path)
		}
	}
	if len(violations) > 0 {
		result.Status = VerificationStatusFailed
		result.FailureCode = "git_diff_path_out_of_scope"
		result.Summary = "git diff 发现越界路径: " + strings.Join(violations, ", ")
		result.Metadata = map[string]any{"changedPaths": changed}
		return result
	}
	result.Status = VerificationStatusPassed
	result.Summary = fmt.Sprintf("git diff 路径检查通过，变更文件 %d 个", len(changed))
	result.Metadata = map[string]any{"changedPaths": changed}
	return result
}

func (v *CompositeVerifier) runBenchmarkThresholdCheck(ctx context.Context, req VerificationRequest, check VerificationCheck) VerificationResult {
	result := VerificationResult{
		VerificationID: v.deps.IDFactory.NewEntityID("verify"),
		ClaimID:        req.Claim.ClaimID,
		CheckName:      check.Name,
		Kind:           check.Kind,
	}
	commandResult := v.runCommandCheck(ctx, req, check)
	result.Artifact = commandResult.Artifact
	if commandResult.Status != VerificationStatusPassed {
		result.Status = commandResult.Status
		result.FailureCode = commandResult.FailureCode
		result.Summary = commandResult.Summary
		return result
	}
	output, readErr := readArtifactBody(commandResult.Artifact)
	if readErr != nil {
		result.Status = VerificationStatusInconclusive
		result.FailureCode = "benchmark_artifact_read_failed"
		result.Summary = "读取 benchmark artifact 失败: " + readErr.Error()
		return result
	}
	value, parseErr := extractBenchmarkValue(output, check.Pattern)
	if parseErr != nil {
		result.Status = VerificationStatusInconclusive
		result.FailureCode = "benchmark_parse_failed"
		result.Summary = "解析 benchmark 值失败: " + parseErr.Error()
		return result
	}
	mode := check.ThresholdMode
	if mode == "" {
		mode = "max"
	}
	result.Metadata = map[string]any{
		"value":     value,
		"threshold": check.Threshold,
		"mode":      mode,
	}
	switch mode {
	case "min":
		if value < check.Threshold {
			result.Status = VerificationStatusFailed
			result.FailureCode = "benchmark_threshold_not_met"
			result.Summary = fmt.Sprintf("benchmark 值 %.4f 低于阈值 %.4f", value, check.Threshold)
			return result
		}
	default:
		if value > check.Threshold {
			result.Status = VerificationStatusFailed
			result.FailureCode = "benchmark_threshold_exceeded"
			result.Summary = fmt.Sprintf("benchmark 值 %.4f 高于阈值 %.4f", value, check.Threshold)
			return result
		}
	}
	result.Status = VerificationStatusPassed
	result.Summary = fmt.Sprintf("benchmark 阈值检查通过: %.4f", value)
	return result
}

func (v *CompositeVerifier) runCommandCheck(ctx context.Context, req VerificationRequest, check VerificationCheck) VerificationResult {
	result := VerificationResult{
		VerificationID: v.deps.IDFactory.NewEntityID("verify"),
		ClaimID:        req.Claim.ClaimID,
		CheckName:      check.Name,
		Kind:           check.Kind,
	}
	workdir := check.Workdir
	if workdir == "" && req.Request.TaskSpec.WorkspaceSnapshot != nil {
		workdir = req.Request.TaskSpec.WorkspaceSnapshot.Root
	}
	if workdir == "" {
		workdir = "."
	}
	cmd := exec.CommandContext(ctx, check.Command, check.Args...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), renderVerificationEnv(req, check.Env)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	artifact, writeErr := v.writeCommandArtifact(req.Claim.ClaimID, check.Name, stdout.Bytes(), stderr.Bytes())
	if writeErr == nil {
		result.Artifact = artifact
	}
	if err != nil {
		result.Status = VerificationStatusFailed
		result.FailureCode = classifyCommandFailure(err)
		result.Summary = fmt.Sprintf("命令检查失败 %s: %v", check.Name, err)
		return result
	}
	result.Status = VerificationStatusPassed
	result.FailureCode = "command_passed"
	result.Summary = "命令检查通过: " + check.Name
	return result
}

func (v *CompositeVerifier) runSemanticVerification(ctx context.Context, req VerificationRequest) []VerificationResult {
	timeout := v.deps.PerTaskTimeout
	if timeout <= 0 {
		timeout = DefaultPerTaskTimeout
	}
	task := SemanticVerificationTask{
		TaskMeta: TaskMeta{
			SessionID: req.SessionID,
			RequestID: req.Request.RequestID,
			AgentID:   req.Request.Roles.SemanticVerifier,
			Role:      "semantic-verifier",
		},
		TaskSpec:   req.Request.TaskSpec,
		Claim:      req.Claim,
		Challenges: slices.Clone(req.Challenges),
	}
	_, awaited, _, err := ExecuteTaskWithRetry(ctx, v.deps.TaskDelegate, task, timeout, v.deps.RetryAttempts, TaskRetryHooks{})
	if err != nil {
		failureCode := "semantic_await_failed"
		summary := "semantic verifier 执行失败: " + err.Error()
		var execErr *TaskExecutionError
		if errors.As(err, &execErr) && execErr.Stage == TaskExecutionStageDispatch {
			failureCode = "semantic_dispatch_failed"
			summary = "semantic verifier dispatch 失败: " + err.Error()
		}
		return []VerificationResult{{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
			CheckName:      "semantic",
			Kind:           "semantic",
			Status:         VerificationStatusInconclusive,
			FailureCode:    failureCode,
			Summary:        summary,
		}}
	}
	if !awaited.OK {
		message := "semantic verifier await 失败"
		if awaited.Error != "" {
			message += ": " + awaited.Error
		}
		return []VerificationResult{{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
			CheckName:      "semantic",
			Kind:           "semantic",
			Status:         VerificationStatusInconclusive,
			FailureCode:    "semantic_await_failed",
			Summary:        message,
			Artifact:       awaited.Artifact,
		}}
	}
	typed, ok := awaited.Output.(SemanticVerificationTaskResult)
	if !ok {
		return []VerificationResult{{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
			CheckName:      "semantic",
			Kind:           "semantic",
			Status:         VerificationStatusInconclusive,
			FailureCode:    "semantic_type_mismatch",
			Summary:        "semantic verifier 返回类型不正确",
			Artifact:       awaited.Artifact,
		}}
	}
	results := make([]VerificationResult, 0, len(typed.Output.Results))
	mismatchedClaim := false
	for _, item := range typed.Output.Results {
		if item.TargetType != "" && item.TargetType != "claim" {
			continue
		}
		itemClaimID := strings.TrimSpace(item.ClaimID)
		if itemClaimID == "" {
			itemClaimID = req.Claim.ClaimID
		}
		if itemClaimID != req.Claim.ClaimID {
			mismatchedClaim = true
			continue
		}
		status := VerificationStatusInconclusive
		switch item.Verdict {
		case ClaimVerdictSupported:
			status = VerificationStatusPassed
		case ClaimVerdictRefuted:
			status = VerificationStatusFailed
		}
		results = append(results, VerificationResult{
			VerificationID:    v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:           itemClaimID,
			CheckName:         "semantic",
			Kind:              "semantic",
			Status:            status,
			FailureCode:       "semantic_" + string(item.Verdict),
			Summary:           item.Rationale,
			Artifact:          awaited.Artifact,
			VerdictSuggestion: item.Verdict,
			Confidence:        item.Confidence,
			Metadata:          maps.Clone(item.Metadata),
		})
	}
	if len(results) == 0 && mismatchedClaim {
		results = append(results, VerificationResult{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
			CheckName:      "semantic",
			Kind:           "semantic",
			Status:         VerificationStatusInconclusive,
			FailureCode:    "semantic_claim_mismatch",
			Summary:        "semantic verifier 返回了不属于当前 claim 的结果",
			Artifact:       awaited.Artifact,
		})
	}
	if len(results) == 0 {
		results = append(results, VerificationResult{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
			CheckName:      "semantic",
			Kind:           "semantic",
			Status:         VerificationStatusInconclusive,
			FailureCode:    "semantic_empty_result",
			Summary:        "semantic verifier 未返回结果",
			Artifact:       awaited.Artifact,
		})
	}
	return results
}

func (v *CompositeVerifier) writeCommandArtifact(claimID string, checkName string, stdout []byte, stderr []byte) (*ArtifactRef, error) {
	if strings.TrimSpace(v.deps.ArtifactDir) == "" {
		return nil, nil
	}
	if err := os.MkdirAll(v.deps.ArtifactDir, 0o755); err != nil {
		return nil, err
	}
	body := bytes.NewBuffer(nil)
	_, _ = body.WriteString("# stdout\n")
	_, _ = body.Write(stdout)
	_, _ = body.WriteString("\n# stderr\n")
	_, _ = body.Write(stderr)
	name := filepath.Join(v.deps.ArtifactDir, sanitizeFilename(claimID+"-"+checkName)+".log")
	if err := os.WriteFile(name, body.Bytes(), 0o644); err != nil {
		return nil, err
	}
	hash := sha256.Sum256(body.Bytes())
	return &ArtifactRef{
		Path:      name,
		Hash:      hex.EncodeToString(hash[:]),
		MediaType: "text/plain",
	}, nil
}

func computePathsHash(root string, paths []string) (string, error) {
	files := make([]string, 0)
	for _, item := range paths {
		full := filepath.Join(root, item)
		info, err := os.Stat(full)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			err = filepath.Walk(full, func(path string, info os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if info.IsDir() {
					return nil
				}
				files = append(files, path)
				return nil
			})
			if err != nil {
				return "", err
			}
			continue
		}
		files = append(files, full)
	}
	slices.Sort(files)
	hasher := sha256.New()
	for _, file := range files {
		if _, err := io.WriteString(hasher, file+"\n"); err != nil {
			return "", err
		}
		body, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		if _, err := hasher.Write(body); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func renderVerificationEnv(req VerificationRequest, values map[string]string) []string {
	out := []string{
		"TIL_CONSENSUS_REQUEST_ID=" + req.Request.RequestID,
		"TIL_CONSENSUS_SESSION_ID=" + req.SessionID,
		"TIL_CONSENSUS_CLAIM_ID=" + req.Claim.ClaimID,
	}
	if req.Request.TaskSpec.WorkspaceSnapshot != nil && strings.TrimSpace(req.Request.TaskSpec.WorkspaceSnapshot.Root) != "" {
		out = append(out, "TIL_CONSENSUS_WORKSPACE_ROOT="+req.Request.TaskSpec.WorkspaceSnapshot.Root)
	}
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func extractTouchedPaths(metadata map[string]any) []string {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["touchedPaths"]
	if !ok {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return dedupeStrings(value)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return dedupeStrings(out)
	case string:
		if value == "" {
			return nil
		}
		return dedupeStrings(strings.Split(value, ","))
	default:
		return nil
	}
}

func matchesAllowedPath(path string, allowed []string) bool {
	clean := filepath.Clean(path)
	for _, prefix := range allowed {
		normalized := filepath.Clean(prefix)
		if clean == normalized || strings.HasPrefix(clean, normalized+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func sanitizeFilename(value string) string {
	replacer := strings.NewReplacer("/", "_", " ", "_", ":", "_")
	return replacer.Replace(value)
}

func classifyCommandFailure(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "command_exit_nonzero"
	}
	return "command_exec_failed"
}

func readArtifactBody(artifact *ArtifactRef) (string, error) {
	if artifact == nil || strings.TrimSpace(artifact.Path) == "" {
		return "", fmt.Errorf("artifact path is empty")
	}
	body, err := os.ReadFile(artifact.Path)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func extractBenchmarkValue(body string, pattern string) (float64, error) {
	source := body
	if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return 0, err
		}
		match := re.FindStringSubmatch(body)
		if len(match) < 2 {
			return 0, fmt.Errorf("pattern %q did not match benchmark output", pattern)
		}
		source = match[1]
	}
	numberRe := regexp.MustCompile(`[-+]?\d*\.?\d+(?:[eE][-+]?\d+)?`)
	token := numberRe.FindString(source)
	if token == "" {
		return 0, fmt.Errorf("no numeric token found")
	}
	value, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid numeric value")
	}
	return value, nil
}

func MarshalVerificationMetadata(value any) map[string]any {
	body, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil
	}
	return out
}
