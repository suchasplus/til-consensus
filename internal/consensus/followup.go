package consensus

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FollowUpCaseArtifact struct {
	SchemaVersion   int          `json:"schemaVersion"`
	CaseID          string       `json:"caseId"`
	RequestID       string       `json:"requestId"`
	ParentRequestID string       `json:"parentRequestId"`
	ParentSessionID string       `json:"parentSessionId"`
	ParentCaseID    string       `json:"parentCaseId"`
	Trigger         string       `json:"trigger"`
	CreatedAt       string       `json:"createdAt"`
	Request         StartRequest `json:"request"`
}

func LoadFollowUpCaseArtifact(path string) (FollowUpCaseArtifact, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return FollowUpCaseArtifact{}, fmt.Errorf("read follow-up case artifact: %w", err)
	}
	var artifact FollowUpCaseArtifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		return FollowUpCaseArtifact{}, fmt.Errorf("decode follow-up case artifact: %w", err)
	}
	if artifact.SchemaVersion != SchemaVersion {
		return FollowUpCaseArtifact{}, fmt.Errorf("unsupported follow-up case schema version: %d", artifact.SchemaVersion)
	}
	return artifact, nil
}

func (e *Engine) createFollowUpCaseArtifact(request StartRequest, run *workflowRun, source ExternalCommandSource, sourceResult externalSourceResult, reason string) (string, string, *ArtifactRef, error) {
	dir := strings.TrimSpace(e.deps.ArtifactDir)
	if dir == "" {
		return "", "", nil, nil
	}
	followUpCaseID := fmt.Sprintf("%s_observe_followup_%s", run.manifest.CaseID, sanitizeFilename(source.Name))
	followUpRequestID := e.ids.NewEntityID("followup")
	followUpRequest := request
	followUpRequest.RequestID = followUpRequestID
	followUpRequest.Lineage = &RunLineage{
		ParentRequestID: request.RequestID,
		ParentSessionID: run.sessionID,
		ParentCaseID:    run.manifest.CaseID,
		Trigger:         reason,
	}
	followUpRequest.ActionPolicy = nil
	followUpRequest.TaskSpec.Goal = fmt.Sprintf("复核上一轮裁决是否被新的观测证据推翻：%s", request.TaskSpec.Goal)
	followUpRequest.TaskSpec.Materials = append([]MaterialRef(nil), request.TaskSpec.Materials...)
	followUpRequest.TaskSpec.Materials = append(followUpRequest.TaskSpec.Materials, MaterialRef{
		ID:      "observe-" + sanitizeFilename(source.Name),
		Title:   "observation contradiction from " + source.Name,
		Kind:    "observation",
		Path:    artifactPath(sourceResult.Artifact),
		Content: sourceResult.Excerpt,
		Metadata: map[string]any{
			"parentRequestId": request.RequestID,
			"parentSessionId": run.sessionID,
			"caseId":          run.manifest.CaseID,
			"reason":          reason,
			"sourceName":      source.Name,
			"reference":       firstNonEmpty(source.Reference, source.Command),
			"summary":         sourceResult.Summary,
			"matchedOK":       sourceResult.MatchedOK,
			"contradicted":    sourceResult.Contradicted,
			"execFailed":      sourceResult.ExecFailed,
		},
	})
	followUpRequest.TaskSpec.Context = cloneAnyMap(request.TaskSpec.Context)
	if followUpRequest.TaskSpec.Context == nil {
		followUpRequest.TaskSpec.Context = map[string]any{}
	}
	followUpRequest.TaskSpec.Context["parentRequestId"] = request.RequestID
	followUpRequest.TaskSpec.Context["parentSessionId"] = run.sessionID
	followUpRequest.TaskSpec.Context["parentCaseId"] = run.manifest.CaseID
	followUpRequest.TaskSpec.Context["followUpReason"] = reason
	followUpRequest.TaskSpec.Context["observationSource"] = source.Name
	followUpRequest.TaskSpec.Context["observationSummary"] = sourceResult.Summary
	followUpRequest.TaskSpec.SuccessCriteria = appendUnique(append([]string(nil), request.TaskSpec.SuccessCriteria...), "必须判断新增观测是否推翻原有 retained claims")
	followUpRequest.TaskSpec.OutOfScope = appendUnique(append([]string(nil), request.TaskSpec.OutOfScope...), "不要直接复用上一轮结论而不重新检查新增观测")

	payload := FollowUpCaseArtifact{
		SchemaVersion:   SchemaVersion,
		CaseID:          followUpCaseID,
		RequestID:       followUpRequestID,
		ParentRequestID: request.RequestID,
		ParentSessionID: run.sessionID,
		ParentCaseID:    run.manifest.CaseID,
		Trigger:         "observe_contradiction",
		CreatedAt:       e.clock.Now().Format(time.RFC3339Nano),
		Request:         followUpRequest,
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", "", nil, fmt.Errorf("marshal follow-up case artifact: %w", err)
	}
	followUpDir := filepath.Join(dir, "followups")
	if err := os.MkdirAll(followUpDir, 0o755); err != nil {
		return "", "", nil, fmt.Errorf("create follow-up dir: %w", err)
	}
	path := filepath.Join(followUpDir, sanitizeFilename(followUpCaseID)+".json")
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", "", nil, fmt.Errorf("write follow-up case artifact: %w", err)
	}
	hash := sha256.Sum256(body)
	return followUpCaseID, followUpRequestID, &ArtifactRef{
		Path:      path,
		Hash:      hex.EncodeToString(hash[:]),
		MediaType: "application/json",
	}, nil
}

func artifactPath(ref *ArtifactRef) string {
	if ref == nil {
		return ""
	}
	return ref.Path
}
