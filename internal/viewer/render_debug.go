package viewer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
)

func buildDebugEvents(records []consensus.RunEventRecord, runDir string) []DebugEventView {
	if len(records) == 0 {
		return nil
	}
	out := make([]DebugEventView, 0, len(records))
	for _, record := range records {
		payload := cloneMap(record.Event.Payload)
		rawVerdict, rawTaskVerdict := extractDebugRawVerdicts(payload)
		out = append(out, DebugEventView{
			Seq:            record.Seq,
			LoggedAt:       record.LoggedAt,
			EventAt:        record.Event.At,
			Type:           record.Event.Type,
			Phase:          string(record.Event.Phase),
			Summary:        summarizeDebugEvent(record.Event),
			RawVerdict:     rawVerdict,
			RawTaskVerdict: rawTaskVerdict,
			Payload:        payload,
			PayloadPretty:  prettyPayload(payload),
			ArtifactHints:  debugArtifactHints(record.Event, runDir),
		})
	}
	return out
}

func extractDebugRawVerdicts(payload map[string]any) (string, string) {
	if len(payload) == 0 {
		return "", ""
	}
	var metadata map[string]any
	if rawMeta, ok := payload["metadata"]; ok {
		metadata = cloneGenericMap(rawMeta)
	}
	rawVerdict := formatDebugValue(firstNonNil(
		mapValue(metadata, "rawVerdict"),
		payload["rawVerdict"],
	))
	rawTaskVerdict := formatDebugValue(firstNonNil(
		mapValue(metadata, "rawTaskVerdict"),
		payload["rawTaskVerdict"],
	))
	return rawVerdict, rawTaskVerdict
}

func summarizeDebugEvent(event consensus.RunEvent) string {
	payload := event.Payload
	switch event.Type {
	case consensus.RunEventPhaseChanged:
		return fmt.Sprintf("进入阶段 %s", event.Phase)
	case consensus.RunEventTaskDispatched:
		return fmt.Sprintf("%s -> %s 已派发", stringValue(payload, "agentId"), stringValue(payload, "taskKind"))
	case consensus.RunEventTaskRetrying:
		return fmt.Sprintf("%s -> %s 重试：%s", stringValue(payload, "agentId"), stringValue(payload, "taskKind"), firstNonEmpty(stringValue(payload, "error"), "未知原因"))
	case consensus.RunEventTaskCompleted:
		return fmt.Sprintf("%s -> %s 已完成", stringValue(payload, "agentId"), stringValue(payload, "taskKind"))
	case consensus.RunEventTaskFailed:
		return fmt.Sprintf("%s -> %s 失败：%s", stringValue(payload, "agentId"), stringValue(payload, "taskKind"), firstNonEmpty(stringValue(payload, "error"), "未知错误"))
	case consensus.RunEventLedgerAppended:
		return fmt.Sprintf("ledger 追加：%s -> %s", stringValue(payload, "kind"), firstNonEmpty(stringValue(payload, "claimId"), stringValue(payload, "entryId")))
	case consensus.RunEventClaimRevised:
		return fmt.Sprintf("claim %s revised：%s", stringValue(payload, "claimId"), stringValue(payload, "action"))
	case consensus.RunEventClaimAdjudicated:
		return fmt.Sprintf("claim %s adjudicated：%s", stringValue(payload, "claimId"), stringValue(payload, "disposition"))
	case consensus.RunEventObservationAdded:
		return fmt.Sprintf("observation %s：%s", stringValue(payload, "observationId"), stringValue(payload, "outcome"))
	case consensus.RunEventSessionFinalized:
		return fmt.Sprintf("session finalized：%s", firstNonEmpty(stringValue(payload, "terminalState"), stringValue(payload, "taskVerdict")))
	case consensus.RunEventSessionFailed:
		return fmt.Sprintf("session failed：%s", firstNonEmpty(stringValue(payload, "error"), "未知错误"))
	default:
		return compactPayload(payload)
	}
}

func debugArtifactHints(event consensus.RunEvent, runDir string) []string {
	if len(event.Payload) == 0 {
		return nil
	}
	hints := make([]string, 0, 4)
	if artifactPath := strings.TrimSpace(stringValue(event.Payload, "artifactPath")); artifactPath != "" {
		hints = append(hints, displayCompanionPath(runDir, artifactPath))
	}
	if followUp := strings.TrimSpace(stringValue(event.Payload, "followUpArtifact")); followUp != "" {
		hints = append(hints, displayCompanionPath(runDir, followUp))
	}
	taskKind := strings.TrimSpace(stringValue(event.Payload, "taskKind"))
	agentID := strings.TrimSpace(stringValue(event.Payload, "agentId"))
	if taskKind == "" || agentID == "" {
		return uniqueStrings(hints)
	}
	taskID := strings.TrimSpace(stringValue(event.Payload, "taskId"))
	if taskID == "" {
		taskID = "<taskID>"
	}
	safeAgent := sanitizeFilename(agentID)
	safeTask := sanitizeFilename(taskKind)
	safeTaskID := sanitizeFilename(taskID)
	baseDir := filepath.Join(runDir, "artifacts")
	hints = append(hints, displayCompanionPath(runDir, filepath.Join(baseDir, fmt.Sprintf("input-%s-%s-%s.json", safeAgent, safeTask, safeTaskID))))
	switch event.Type {
	case consensus.RunEventTaskCompleted:
		hints = append(hints, displayCompanionPath(runDir, filepath.Join(baseDir, fmt.Sprintf("raw-%s-%s-%s.json", safeAgent, safeTask, safeTaskID))))
	case consensus.RunEventTaskFailed:
		hints = append(hints,
			displayCompanionPath(runDir, filepath.Join(baseDir, fmt.Sprintf("failure-%s-%s-%s.json", safeAgent, safeTask, safeTaskID))),
			displayCompanionPath(runDir, filepath.Join(baseDir, fmt.Sprintf("raw-error-%s-%s-%s.txt", safeAgent, safeTask, safeTaskID))),
		)
	}
	return uniqueStrings(hints)
}

func prettyPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	body, err := prettyJSON(payload, "", "  ")
	if err != nil {
		return ""
	}
	return body
}

func prettyJSON(value any, prefix string, indent string) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent(prefix, indent)
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func formatDebugValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		body, err := json.Marshal(typed)
		if err == nil {
			return strings.TrimSpace(string(body))
		}
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}
