package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TaskOutputJSONSchema(task consensus.Task) map[string]any {
	switch task.(type) {
	case consensus.ProposalTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "claims"},
		}
	case consensus.InitialProposalTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "claims"},
		}
	case consensus.ChallengeTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "tickets"},
		}
	case consensus.ReviseTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "revisions"},
		}
	case consensus.DebateRoundTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "judgements"},
		}
	case consensus.FinalVoteTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "votes"},
		}
	case consensus.DelphiQuestionnaireTask, consensus.DelphiRevisionTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "responses"},
		}
	case consensus.DelphiFacilitatorSummaryTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary"},
		}
	case consensus.SemanticVerificationTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "results"},
		}
	case consensus.ArbiterTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary", "taskVerdict", "decisions"},
		}
	case consensus.ReportTask:
		return map[string]any{
			"type":     "object",
			"required": []string{"summary"},
		}
	default:
		return map[string]any{
			"type":     "object",
			"required": []string{"fullResponse", "summary"},
		}
	}
}

func NormalizeTaskOutput(task consensus.Task, raw any) (consensus.TaskResult, error) {
	switch value := raw.(type) {
	case string:
		return NormalizeTaskOutputFromText(task, value)
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal raw task output: %w", err)
		}
		return normalizeTaskOutputFromJSON(task, payload)
	}
}

func NormalizeTaskOutputFromText(task consensus.Task, text string) (consensus.TaskResult, error) {
	if _, ok := task.(consensus.ActionTask); ok {
		summary := strings.TrimSpace(text)
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
		return consensus.ActionTaskResult{
			Output: consensus.ActionExecution{
				FullResponse: text,
				Summary:      summary,
			},
		}, nil
	}
	value, err := ParseJSONObject(text)
	if err != nil {
		return nil, err
	}
	return NormalizeTaskOutput(task, value)
}

func normalizeTaskOutputFromJSON(task consensus.Task, payload []byte) (consensus.TaskResult, error) {
	normalizedPayload, err := normalizeTaskOutputPayload(task, payload)
	if err != nil {
		return nil, fmt.Errorf("normalize task output payload: %w", err)
	}
	switch task.(type) {
	case consensus.ProposalTask:
		var output consensus.ProposalOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode proposal output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("proposal output missing summary")
		}
		return consensus.ProposalTaskResult{Output: output}, nil
	case consensus.InitialProposalTask:
		var output consensus.InitialProposalOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode initial proposal output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("initial proposal output missing summary")
		}
		return consensus.InitialProposalTaskResult{Output: output}, nil
	case consensus.ChallengeTask:
		var output consensus.ChallengeOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode challenge output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("challenge output missing summary")
		}
		return consensus.ChallengeTaskResult{Output: output}, nil
	case consensus.ReviseTask:
		var output consensus.ReviseOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode revise output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("revise output missing summary")
		}
		return consensus.ReviseTaskResult{Output: output}, nil
	case consensus.DebateRoundTask:
		var output consensus.DebateRoundOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode debate round output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("debate round output missing summary")
		}
		return consensus.DebateRoundTaskResult{Output: output}, nil
	case consensus.FinalVoteTask:
		var output consensus.FinalVoteOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode final vote output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("final vote output missing summary")
		}
		return consensus.FinalVoteTaskResult{Output: output}, nil
	case consensus.SemanticVerificationTask:
		var output consensus.SemanticVerificationOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode semantic verification output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("semantic verification output missing summary")
		}
		return consensus.SemanticVerificationTaskResult{Output: output}, nil
	case consensus.DelphiQuestionnaireTask:
		var output consensus.DelphiQuestionnaireOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode delphi questionnaire output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("delphi questionnaire output missing summary")
		}
		return consensus.DelphiQuestionnaireTaskResult{Output: output}, nil
	case consensus.DelphiRevisionTask:
		var output consensus.DelphiRevisionOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode delphi revision output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("delphi revision output missing summary")
		}
		return consensus.DelphiRevisionTaskResult{Output: output}, nil
	case consensus.DelphiFacilitatorSummaryTask:
		var output consensus.DelphiFacilitatorSummaryOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode delphi facilitator summary output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("delphi facilitator summary output missing summary")
		}
		return consensus.DelphiFacilitatorSummaryTaskResult{Output: output}, nil
	case consensus.ArbiterTask:
		var output consensus.ArbiterTaskOutput
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode arbiter output: %w", err)
		}
		if output.TaskVerdict == "" {
			return nil, fmt.Errorf("arbiter output missing taskVerdict")
		}
		return consensus.ArbiterTaskResult{Output: output}, nil
	case consensus.ReportTask:
		var output consensus.AdjudicationReport
		if err := json.Unmarshal(normalizedPayload, &output); err != nil {
			return nil, fmt.Errorf("decode report output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("report output missing summary")
		}
		return consensus.ReportTaskResult{Output: output}, nil
	case consensus.ActionTask:
		var output consensus.ActionExecution
		if err := json.Unmarshal(normalizedPayload, &output); err == nil && strings.TrimSpace(output.FullResponse) != "" {
			if output.Summary == "" {
				output.Summary = output.FullResponse
			}
			return consensus.ActionTaskResult{Output: output}, nil
		}
		text := strings.TrimSpace(string(payload))
		if text == "" {
			text = "{}"
		}
		return consensus.ActionTaskResult{Output: consensus.ActionExecution{
			FullResponse: text,
			Summary:      truncateSummary(text),
		}}, nil
	default:
		return nil, fmt.Errorf("unsupported task type")
	}
}

func truncateSummary(text string) string {
	if len(text) <= 200 {
		return text
	}
	return text[:200] + "..."
}

var (
	floatOutputKeys = map[string]struct{}{
		"confidence":      {},
		"confidenceDelta": {},
		"finalConfidence": {},
		"rating":          {},
		"meanRating":      {},
		"consensusLevel":  {},
		"supportRatio":    {},
	}
	intOutputKeys = map[string]struct{}{
		"responseCount": {},
		"lastRound":     {},
		"round":         {},
	}
	claimStatementAliases = []string{"claim", "text", "statementText"}
)

func normalizeTaskOutputPayload(task consensus.Task, payload []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != nil && err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing data: %w", err)
	}

	normalizeTaskShape(task, value)
	coerceNumericStrings(value)

	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeTaskShape(task consensus.Task, value any) {
	root, ok := value.(map[string]any)
	if !ok {
		return
	}
	switch task.(type) {
	case consensus.ProposalTask, consensus.InitialProposalTask:
		normalizeClaimDraftList(root["claims"])
	case consensus.SemanticVerificationTask:
		normalizeSemanticResults(root["results"])
	case consensus.ReviseTask:
		normalizeRevisionDrafts(root["revisions"])
	case consensus.ArbiterTask:
		normalizeArbiterOutput(root)
	}
}

func normalizeClaimDraftList(value any) {
	items, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		normalizeOutputAliases(entry)
	}
}

func normalizeSemanticResults(value any) {
	items, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		targetType := strings.ToLower(strings.TrimSpace(stringMapValue(entry, "targetType")))
		rawVerdict := strings.TrimSpace(stringMapValue(entry, "verdict"))
		if _, ok := entry["claimId"]; !ok {
			if targetID := firstNonEmpty(stringMapValue(entry, "targetId"), stringMapValue(entry, "claim")); targetID != "" && (targetType == "" || targetType == "claim") {
				entry["claimId"] = targetID
			}
		}
		if _, ok := entry["rationale"]; !ok {
			if rationale := firstNonEmpty(stringMapValue(entry, "reasoning"), stringMapValue(entry, "reason")); rationale != "" {
				entry["rationale"] = rationale
			}
		}
		if verdict := normalizeClaimVerdictString(rawVerdict); verdict != "" {
			entry["verdict"] = verdict
		}
		attachMetadata(entry, map[string]any{
			"rawVerdict":  rawVerdict,
			"rawTargetId": stringMapValue(entry, "targetId"),
		})
	}
}

func normalizeRevisionDrafts(value any) {
	items, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rawVerdict := strings.TrimSpace(stringMapValue(entry, "verdict"))
		if _, ok := entry["targetClaimId"]; !ok {
			if claimID := firstNonEmpty(stringMapValue(entry, "claimId"), stringMapValue(entry, "targetId")); claimID != "" {
				entry["targetClaimId"] = claimID
			}
		}
		if _, ok := entry["reason"]; !ok {
			if reason := firstNonEmpty(stringMapValue(entry, "rationale"), stringMapValue(entry, "reasoning")); reason != "" {
				entry["reason"] = reason
			}
		}
		if verdict := normalizeClaimVerdictString(rawVerdict); verdict == string(consensus.ClaimVerdictUndetermined) {
			if _, ok := entry["unresolved"]; !ok {
				entry["unresolved"] = true
			}
		}
		attachMetadata(entry, map[string]any{
			"rawVerdict": rawVerdict,
			"rawClaimId": stringMapValue(entry, "claimId"),
		})
	}
}

func normalizeArbiterOutput(root map[string]any) {
	metadata := map[string]any{}
	if rawTaskVerdict, ok := root["taskVerdict"].(map[string]any); ok {
		metadata["rawTaskVerdict"] = cloneAnyMap(rawTaskVerdict)
		if verdict := normalizeTaskVerdictString(stringMapValue(rawTaskVerdict, "verdict")); verdict != "" {
			root["taskVerdict"] = verdict
		}
	}
	items, ok := root["decisions"].([]any)
	if !ok {
		return
	}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rawVerdict := strings.TrimSpace(stringMapValue(entry, "verdict"))
		if _, ok := entry["claimId"]; !ok {
			if claimID := firstNonEmpty(stringMapValue(entry, "targetClaimId"), stringMapValue(entry, "targetId")); claimID != "" {
				entry["claimId"] = claimID
			}
		}
		if _, ok := entry["rationale"]; !ok {
			if rationale := firstNonEmpty(stringMapValue(entry, "reasoning"), stringMapValue(entry, "reason")); rationale != "" {
				entry["rationale"] = rationale
			}
		}
		if _, ok := entry["evidenceRefs"]; !ok {
			if refs, ok := entry["keyEvidenceRefs"]; ok {
				entry["evidenceRefs"] = refs
			}
		}
		if verdict := normalizeClaimVerdictString(rawVerdict); verdict != "" {
			entry["verdict"] = verdict
		}
		attachMetadata(entry, map[string]any{
			"rawVerdict":       rawVerdict,
			"rawTargetClaimId": stringMapValue(entry, "targetClaimId"),
		})
	}
	if len(metadata) > 0 {
		attachMetadata(root, metadata)
	}
}

func coerceNumericStrings(value any) {
	switch typed := value.(type) {
	case map[string]any:
		normalizeOutputAliases(typed)
		for key, item := range typed {
			switch raw := item.(type) {
			case string:
				if _, ok := floatOutputKeys[key]; ok {
					if parsed, ok := parseFlexibleFloat(raw); ok {
						typed[key] = parsed
						continue
					}
				}
				if _, ok := intOutputKeys[key]; ok {
					if parsed, ok := parseFlexibleInt(raw); ok {
						typed[key] = parsed
						continue
					}
				}
			}
			coerceNumericStrings(item)
		}
	case []any:
		for _, item := range typed {
			coerceNumericStrings(item)
		}
	}
}

func normalizeOutputAliases(values map[string]any) {
	if values == nil {
		return
	}
	if _, ok := values["statement"]; !ok {
		for _, key := range claimStatementAliases {
			if raw, ok := values[key]; ok {
				if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
					values["statement"] = text
					break
				}
			}
		}
	}
}

func normalizeClaimVerdictString(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "unknown":
		return ""
	case "supported", "support", "upheld", "relevant", "partially_supported", "partially-supported":
		return string(consensus.ClaimVerdictSupported)
	case "refuted", "rejected", "reject", "not_supported", "not-supported", "overstated":
		return string(consensus.ClaimVerdictRefuted)
	case "insufficient_for_claim", "insufficient-for-claim", "insufficient_evidence", "insufficient-evidence":
		return string(consensus.ClaimVerdictInsufficientEvidence)
	case "undetermined", "uncertain", "inconclusive", "mixed":
		return string(consensus.ClaimVerdictUndetermined)
	default:
		return raw
	}
}

func normalizeTaskVerdictString(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "unknown":
		return ""
	case "supported", "support", "keep":
		return string(consensus.TaskVerdictSupported)
	case "partially_supported", "partially-supported", "keep_with_caveat", "keep-with-caveat", "mixed":
		return string(consensus.TaskVerdictPartiallySupported)
	case "undetermined", "unresolved", "insufficient_evidence", "insufficient-evidence":
		return string(consensus.TaskVerdictUndetermined)
	case "failed", "reject", "rejected":
		return string(consensus.TaskVerdictFailed)
	default:
		return raw
	}
}

func parseFlexibleFloat(raw string) (float64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	if value, ok := parseConfidenceLabel(trimmed); ok {
		return value, true
	}
	if strings.HasSuffix(trimmed, "%") {
		percent := strings.TrimSpace(strings.TrimSuffix(trimmed, "%"))
		value, err := strconv.ParseFloat(percent, 64)
		if err != nil {
			return 0, false
		}
		return value / 100, true
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func stringMapValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func attachMetadata(values map[string]any, additions map[string]any) {
	if values == nil || len(additions) == 0 {
		return
	}
	meta, _ := values["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		values["metadata"] = meta
	}
	for key, value := range additions {
		if value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				continue
			}
		}
		meta[key] = value
	}
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func parseConfidenceLabel(raw string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "very_low", "very-low", "very low":
		return 0.15, true
	case "low":
		return 0.35, true
	case "medium", "moderate":
		return 0.6, true
	case "medium_high", "medium-high", "medium high":
		return 0.7, true
	case "high":
		return 0.82, true
	case "very_high", "very-high", "very high":
		return 0.93, true
	case "uncertain", "unknown":
		return 0.5, true
	default:
		return 0, false
	}
}

func parseFlexibleInt(raw string) (int64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	if value, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return value, true
	}
	floatValue, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	intValue := int64(floatValue)
	if float64(intValue) != floatValue {
		return 0, false
	}
	return intValue, true
}
