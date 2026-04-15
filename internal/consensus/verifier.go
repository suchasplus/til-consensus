package consensus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type CompositeVerifierDeps struct {
	TaskDelegate   TaskDelegate
	Clock          Clock
	IDFactory      IDFactory
	ArtifactDir    string
	PerTaskTimeout time.Duration
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
	case "command":
		return v.runCommandCheck(ctx, req, check)
	default:
		return VerificationResult{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
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
		Kind:           check.Kind,
	}
	snapshot := req.Request.TaskSpec.WorkspaceSnapshot
	if snapshot == nil {
		result.Status = VerificationStatusInconclusive
		result.Summary = "workspace snapshot 未提供"
		return result
	}
	if snapshot.Root == "" {
		result.Status = VerificationStatusInconclusive
		result.Summary = "workspace snapshot 缺少 root"
		return result
	}
	if snapshot.Hash != "" && len(snapshot.Paths) > 0 {
		hash, err := computePathsHash(snapshot.Root, snapshot.Paths)
		if err != nil {
			result.Status = VerificationStatusInconclusive
			result.Summary = "计算 workspace hash 失败: " + err.Error()
			return result
		}
		if hash != snapshot.Hash {
			result.Status = VerificationStatusFailed
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
			result.Summary = "读取 git revision 失败: " + err.Error()
			return result
		}
		current := strings.TrimSpace(string(body))
		if current != snapshot.Revision {
			result.Status = VerificationStatusFailed
			result.Summary = fmt.Sprintf("workspace revision 不匹配: got=%s want=%s", current, snapshot.Revision)
			return result
		}
		result.Status = VerificationStatusPassed
		result.Summary = "workspace revision 校验通过"
		return result
	}
	result.Status = VerificationStatusInconclusive
	result.Summary = "workspace snapshot 缺少可执行校验字段"
	return result
}

func (v *CompositeVerifier) runAllowedPathsCheck(req VerificationRequest, check VerificationCheck) VerificationResult {
	result := VerificationResult{
		VerificationID: v.deps.IDFactory.NewEntityID("verify"),
		ClaimID:        req.Claim.ClaimID,
		Kind:           check.Kind,
	}
	allowed := req.Request.TaskSpec.Constraints.AllowedPaths
	if len(allowed) == 0 {
		result.Status = VerificationStatusInconclusive
		result.Summary = "未配置 allowed_paths"
		return result
	}
	touched := extractTouchedPaths(req.Claim.Metadata)
	if len(touched) == 0 {
		result.Status = VerificationStatusInconclusive
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
		result.Summary = "发现越界路径: " + strings.Join(violations, ", ")
		return result
	}
	result.Status = VerificationStatusPassed
	result.Summary = "touchedPaths 均在允许范围内"
	return result
}

func (v *CompositeVerifier) runCommandCheck(ctx context.Context, req VerificationRequest, check VerificationCheck) VerificationResult {
	result := VerificationResult{
		VerificationID: v.deps.IDFactory.NewEntityID("verify"),
		ClaimID:        req.Claim.ClaimID,
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
		result.Summary = fmt.Sprintf("命令检查失败 %s: %v", check.Name, err)
		return result
	}
	result.Status = VerificationStatusPassed
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
	dispatched, err := v.deps.TaskDelegate.Dispatch(ctx, task)
	if err != nil {
		return []VerificationResult{{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
			Kind:           "semantic",
			Status:         VerificationStatusInconclusive,
			Summary:        "semantic verifier dispatch 失败: " + err.Error(),
		}}
	}
	awaited, err := v.deps.TaskDelegate.Await(ctx, dispatched.TaskID, timeout)
	if err != nil || !awaited.OK {
		message := "semantic verifier await 失败"
		if err != nil {
			message += ": " + err.Error()
		} else if awaited.Error != "" {
			message += ": " + awaited.Error
		}
		return []VerificationResult{{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
			Kind:           "semantic",
			Status:         VerificationStatusInconclusive,
			Summary:        message,
			Artifact:       awaited.Artifact,
		}}
	}
	typed, ok := awaited.Output.(SemanticVerificationTaskResult)
	if !ok {
		return []VerificationResult{{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
			Kind:           "semantic",
			Status:         VerificationStatusInconclusive,
			Summary:        "semantic verifier 返回类型不正确",
			Artifact:       awaited.Artifact,
		}}
	}
	results := make([]VerificationResult, 0, len(typed.Output.Results))
	for _, item := range typed.Output.Results {
		status := VerificationStatusInconclusive
		switch item.Verdict {
		case ClaimVerdictSupported:
			status = VerificationStatusPassed
		case ClaimVerdictRefuted:
			status = VerificationStatusFailed
		}
		results = append(results, VerificationResult{
			VerificationID:    v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:           item.ClaimID,
			Kind:              "semantic",
			Status:            status,
			Summary:           item.Rationale,
			Artifact:          awaited.Artifact,
			VerdictSuggestion: item.Verdict,
			Confidence:        item.Confidence,
		})
	}
	if len(results) == 0 {
		results = append(results, VerificationResult{
			VerificationID: v.deps.IDFactory.NewEntityID("verify"),
			ClaimID:        req.Claim.ClaimID,
			Kind:           "semantic",
			Status:         VerificationStatusInconclusive,
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
