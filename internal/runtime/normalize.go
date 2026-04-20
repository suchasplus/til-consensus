package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TaskOutputJSONSchema(task consensus.Task) map[string]any {
	switch task.(type) {
	case consensus.ProposalTask:
		return objectSchema(
			[]string{"summary", "claims"},
			map[string]any{
				"summary": schemaString(),
				"claims":  arraySchema(proposalClaimDraftSchema()),
			},
		)
	case consensus.InitialProposalTask:
		return objectSchema(
			[]string{"summary", "claims"},
			map[string]any{
				"summary": schemaString(),
				"claims":  arraySchema(proposalClaimDraftSchema()),
			},
		)
	case consensus.ChallengeTask:
		return objectSchema(
			[]string{"summary", "tickets"},
			map[string]any{
				"summary": schemaString(),
				"tickets": arraySchema(challengeDraftSchema()),
			},
		)
	case consensus.ReviseTask:
		return objectSchema(
			[]string{"summary", "revisions"},
			map[string]any{
				"summary":             schemaString(),
				"revisions":           arraySchema(revisionDraftSchema()),
				"unresolvedQuestions": stringArraySchema(),
			},
		)
	case consensus.DebateRoundTask:
		return objectSchema(
			[]string{"summary", "judgements"},
			map[string]any{
				"summary":    schemaString(),
				"newClaims":  arraySchema(proposalClaimDraftSchema()),
				"judgements": arraySchema(debateJudgementSchema()),
			},
		)
	case consensus.FinalVoteTask:
		return objectSchema(
			[]string{"summary", "votes"},
			map[string]any{
				"summary": schemaString(),
				"votes":   arraySchema(debateVoteSchema()),
			},
		)
	case consensus.DelphiQuestionnaireTask, consensus.DelphiRevisionTask:
		return objectSchema(
			[]string{"summary", "responses"},
			map[string]any{
				"summary":   schemaString(),
				"responses": arraySchema(delphiResponseSchema()),
			},
		)
	case consensus.DelphiFacilitatorSummaryTask:
		return objectSchema(
			[]string{"summary"},
			map[string]any{
				"summary":        schemaString(),
				"recommendation": schemaString(),
				"dissentSummary": stringArraySchema(),
				"statements":     arraySchema(delphiStatementSchema()),
			},
		)
	case consensus.SemanticVerificationTask:
		return objectSchema(
			[]string{"summary", "results"},
			map[string]any{
				"summary": schemaString(),
				"results": arraySchema(semanticFindingSchema()),
			},
		)
	case consensus.ArbiterTask:
		return objectSchema(
			[]string{"summary", "taskVerdict", "decisions"},
			map[string]any{
				"summary":     schemaString(),
				"taskVerdict": enumStringSchema([]string{string(consensus.TaskVerdictSupported), string(consensus.TaskVerdictPartiallySupported), string(consensus.TaskVerdictUndetermined), string(consensus.TaskVerdictFailed)}),
				"decisions":   arraySchema(arbiterDecisionSchema()),
			},
		)
	case consensus.ReportTask:
		return objectSchema(
			[]string{"summary"},
			map[string]any{
				"summary":             schemaString(),
				"highlights":          stringArraySchema(),
				"retainedClaims":      stringArraySchema(),
				"downgradedClaims":    stringArraySchema(),
				"unresolvedQuestions": stringArraySchema(),
				"nextActions":         stringArraySchema(),
			},
		)
	default:
		return objectSchema(
			[]string{"fullResponse", "summary"},
			map[string]any{
				"fullResponse": schemaString(),
				"summary":      schemaString(),
			},
		)
	}
}

func TaskOutputJSONSchemaForAgent(task consensus.Task, agent ResolvedAgentRuntime) map[string]any {
	schema := cloneSchemaMap(TaskOutputJSONSchema(task))
	if usesCodexStructuredSchema(agent) {
		enforceAllObjectPropertiesRequired(schema)
	}
	return schema
}

func usesCodexStructuredSchema(agent ResolvedAgentRuntime) bool {
	return agent.Provider.Type == config.ProviderTypeCLI && agent.Provider.CLIType == "codex"
}

func objectSchema(required []string, properties map[string]any) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func arraySchema(items map[string]any) map[string]any {
	return map[string]any{
		"type":  "array",
		"items": items,
	}
}

func schemaString() map[string]any {
	return map[string]any{"type": "string"}
}

func schemaNumber() map[string]any {
	return map[string]any{"type": "number"}
}

func schemaInteger() map[string]any {
	return map[string]any{"type": "integer"}
}

func schemaBoolean() map[string]any {
	return map[string]any{"type": "boolean"}
}

func enumStringSchema(values []string) map[string]any {
	return map[string]any{
		"type": "string",
		"enum": values,
	}
}

func stringArraySchema() map[string]any {
	return arraySchema(schemaString())
}

func cloneSchemaMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	clone := make(map[string]any, len(src))
	for key, value := range src {
		clone[key] = cloneSchemaValue(value)
	}
	return clone
}

func cloneSchemaSlice(src []any) []any {
	if src == nil {
		return nil
	}
	clone := make([]any, len(src))
	for idx, value := range src {
		clone[idx] = cloneSchemaValue(value)
	}
	return clone
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSchemaMap(typed)
	case []any:
		return cloneSchemaSlice(typed)
	case []string:
		clone := make([]string, len(typed))
		copy(clone, typed)
		return clone
	default:
		return typed
	}
}

func enforceAllObjectPropertiesRequired(schema map[string]any) {
	if schema == nil {
		return
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		required := make([]string, 0, len(properties))
		for key, value := range properties {
			required = append(required, key)
			if nested, ok := value.(map[string]any); ok {
				enforceAllObjectPropertiesRequired(nested)
			}
		}
		slices.Sort(required)
		schema["required"] = required
	}
	if items, ok := schema["items"].(map[string]any); ok {
		enforceAllObjectPropertiesRequired(items)
	}
	if oneOf, ok := schema["oneOf"].([]any); ok {
		for _, option := range oneOf {
			if nested, ok := option.(map[string]any); ok {
				enforceAllObjectPropertiesRequired(nested)
			}
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

func StrictDecodeTaskOutput(task consensus.Task, raw any) (consensus.TaskResult, error) {
	switch value := raw.(type) {
	case string:
		payload, err := StrictJSONObjectBytes(value)
		if err != nil {
			return nil, err
		}
		return decodeTaskOutputFromJSON(task, payload)
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal raw task output: %w", err)
		}
		return decodeTaskOutputFromJSON(task, payload)
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
	normalizedPayload, err := normalizeTaskOutputPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("normalize task output payload: %w", err)
	}
	return decodeTaskOutputFromJSON(task, normalizedPayload)
}

func decodeTaskOutputFromJSON(task consensus.Task, payload []byte) (consensus.TaskResult, error) {
	switch typed := task.(type) {
	case consensus.ProposalTask:
		var output consensus.ProposalOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode proposal output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("proposal output missing summary")
		}
		if err := validateClaimDrafts(output.Claims, false); err != nil {
			return nil, fmt.Errorf("validate proposal output: %w", err)
		}
		return consensus.ProposalTaskResult{Output: output}, nil
	case consensus.InitialProposalTask:
		var output consensus.InitialProposalOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode initial proposal output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("initial proposal output missing summary")
		}
		if err := validateClaimDrafts(output.Claims, false); err != nil {
			return nil, fmt.Errorf("validate initial proposal output: %w", err)
		}
		return consensus.InitialProposalTaskResult{Output: output}, nil
	case consensus.ChallengeTask:
		var output consensus.ChallengeOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode challenge output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("challenge output missing summary")
		}
		return consensus.ChallengeTaskResult{Output: output}, nil
	case consensus.ReviseTask:
		var output consensus.ReviseOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode revise output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("revise output missing summary")
		}
		if err := validateRevisionDrafts(typed.Claims, output.Revisions); err != nil {
			return nil, fmt.Errorf("validate revise output: %w", err)
		}
		return consensus.ReviseTaskResult{Output: output}, nil
	case consensus.DebateRoundTask:
		var output consensus.DebateRoundOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode debate round output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("debate round output missing summary")
		}
		if err := validateClaimDrafts(output.NewClaims, false); err != nil {
			return nil, fmt.Errorf("validate debate round newClaims: %w", err)
		}
		if err := validateDebateJudgements(output.Judgements); err != nil {
			return nil, fmt.Errorf("validate debate round output: %w", err)
		}
		return consensus.DebateRoundTaskResult{Output: output}, nil
	case consensus.FinalVoteTask:
		var output consensus.FinalVoteOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode final vote output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("final vote output missing summary")
		}
		if err := validateDebateVotes(output.Votes); err != nil {
			return nil, fmt.Errorf("validate final vote output: %w", err)
		}
		return consensus.FinalVoteTaskResult{Output: output}, nil
	case consensus.SemanticVerificationTask:
		semanticTask := task.(consensus.SemanticVerificationTask)
		var output consensus.SemanticVerificationOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode semantic verification output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("semantic verification output missing summary")
		}
		if err := validateSemanticFindings(semanticTask.Claim.ClaimID, output.Results); err != nil {
			return nil, fmt.Errorf("validate semantic verification output: %w", err)
		}
		return consensus.SemanticVerificationTaskResult{Output: output}, nil
	case consensus.DelphiQuestionnaireTask:
		var output consensus.DelphiQuestionnaireOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode delphi questionnaire output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("delphi questionnaire output missing summary")
		}
		if err := validateDelphiResponses(output.Responses); err != nil {
			return nil, fmt.Errorf("validate delphi questionnaire output: %w", err)
		}
		return consensus.DelphiQuestionnaireTaskResult{Output: output}, nil
	case consensus.DelphiRevisionTask:
		var output consensus.DelphiRevisionOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode delphi revision output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("delphi revision output missing summary")
		}
		if err := validateDelphiResponses(output.Responses); err != nil {
			return nil, fmt.Errorf("validate delphi revision output: %w", err)
		}
		return consensus.DelphiRevisionTaskResult{Output: output}, nil
	case consensus.DelphiFacilitatorSummaryTask:
		var output consensus.DelphiFacilitatorSummaryOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode delphi facilitator summary output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("delphi facilitator summary output missing summary")
		}
		return consensus.DelphiFacilitatorSummaryTaskResult{Output: output}, nil
	case consensus.ArbiterTask:
		var output consensus.ArbiterTaskOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode arbiter output: %w", err)
		}
		if output.TaskVerdict == "" {
			return nil, fmt.Errorf("arbiter output missing taskVerdict")
		}
		if err := validateArbiterOutput(output); err != nil {
			return nil, fmt.Errorf("validate arbiter output: %w", err)
		}
		return consensus.ArbiterTaskResult{Output: output}, nil
	case consensus.ReportTask:
		var output consensus.AdjudicationReport
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode report output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("report output missing summary")
		}
		return consensus.ReportTaskResult{Output: output}, nil
	case consensus.ActionTask:
		var output consensus.ActionExecution
		if err := json.Unmarshal(payload, &output); err == nil && strings.TrimSpace(output.FullResponse) != "" {
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

func proposalClaimDraftSchema() map[string]any {
	return objectSchema(
		[]string{"statement"},
		map[string]any{
			"title":              schemaString(),
			"statement":          schemaString(),
			"claimType":          enumStringSchema([]string{string(consensus.ClaimTypeFact), string(consensus.ClaimTypeInference), string(consensus.ClaimTypeRecommendation), string(consensus.ClaimTypeAssumption)}),
			"applicability":      schemaString(),
			"boundaryConditions": stringArraySchema(),
			"confidence":         schemaNumber(),
		},
	)
}

func challengeDraftSchema() map[string]any {
	return objectSchema(
		[]string{"statement", "kind"},
		map[string]any{
			"claimId":                      schemaString(),
			"statement":                    schemaString(),
			"kind":                         schemaString(),
			"attackType":                   schemaString(),
			"severity":                     enumStringSchema([]string{string(consensus.AttackSeverityLow), string(consensus.AttackSeverityMedium), string(consensus.AttackSeverityHigh)}),
			"requestedChecks":              stringArraySchema(),
			"suggestedFalsificationMethod": schemaString(),
		},
	)
}

func revisionDraftSchema() map[string]any {
	return objectSchema(
		[]string{"targetClaimId", "action"},
		map[string]any{
			"targetClaimId":      schemaString(),
			"action":             enumStringSchema([]string{string(consensus.RevisionActionRevise), string(consensus.RevisionActionDowngrade), string(consensus.RevisionActionWithdraw), string(consensus.RevisionActionUnresolved), string(consensus.RevisionActionUnchanged)}),
			"revisedText":        schemaString(),
			"confidenceDelta":    schemaNumber(),
			"caveats":            stringArraySchema(),
			"boundaryConditions": stringArraySchema(),
			"reason":             schemaString(),
			"unresolved":         schemaBoolean(),
		},
	)
}

func debateJudgementSchema() map[string]any {
	return objectSchema(
		[]string{"claimId", "judgement"},
		map[string]any{
			"claimId":          schemaString(),
			"judgement":        enumStringSchema([]string{string(consensus.DebateJudgementAgree), string(consensus.DebateJudgementDisagree), string(consensus.DebateJudgementRevise), string(consensus.DebateJudgementNoChange)}),
			"rationale":        schemaString(),
			"revisedStatement": schemaString(),
			"mergeWithClaims":  stringArraySchema(),
		},
	)
}

func debateVoteSchema() map[string]any {
	return objectSchema(
		[]string{"claimId", "vote"},
		map[string]any{
			"claimId":   schemaString(),
			"vote":      enumStringSchema([]string{string(consensus.DebateVoteAccept), string(consensus.DebateVoteReject), string(consensus.DebateVoteAbstain)}),
			"rationale": schemaString(),
		},
	)
}

func delphiResponseSchema() map[string]any {
	return objectSchema(
		nil,
		map[string]any{
			"statementId": schemaString(),
			"statement":   schemaString(),
			"rating":      schemaNumber(),
			"rationale":   schemaString(),
		},
	)
}

func delphiStatementSchema() map[string]any {
	return objectSchema(
		[]string{"statementId", "statement", "meanRating", "consensusLevel", "responseCount", "lastRound"},
		map[string]any{
			"statementId":           schemaString(),
			"statement":             schemaString(),
			"meanRating":            schemaNumber(),
			"consensusLevel":        schemaNumber(),
			"responseCount":         schemaInteger(),
			"lastRound":             schemaInteger(),
			"representativeReasons": stringArraySchema(),
		},
	)
}

func semanticFindingSchema() map[string]any {
	return objectSchema(
		[]string{"claimId", "verdict", "rationale"},
		map[string]any{
			"claimId":    schemaString(),
			"targetType": schemaString(),
			"verdict":    enumStringSchema([]string{string(consensus.ClaimVerdictSupported), string(consensus.ClaimVerdictRefuted), string(consensus.ClaimVerdictInsufficientEvidence), string(consensus.ClaimVerdictUndetermined)}),
			"confidence": schemaNumber(),
			"rationale":  schemaString(),
		},
	)
}

func arbiterDecisionSchema() map[string]any {
	return objectSchema(
		[]string{"claimId", "verdict"},
		map[string]any{
			"claimId":      schemaString(),
			"verdict":      enumStringSchema([]string{string(consensus.ClaimVerdictSupported), string(consensus.ClaimVerdictRefuted), string(consensus.ClaimVerdictInsufficientEvidence), string(consensus.ClaimVerdictUndetermined)}),
			"confidence":   schemaNumber(),
			"rationale":    schemaString(),
			"evidenceRefs": stringArraySchema(),
		},
	)
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
)

func normalizeTaskOutputPayload(payload []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != nil && err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing data: %w", err)
	}

	coerceNumericStrings(value)

	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func coerceNumericStrings(value any) {
	switch typed := value.(type) {
	case map[string]any:
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

func parseFlexibleFloat(raw string) (float64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
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

func validateClaimDrafts(claims []consensus.ClaimDraft, allowRelationships bool) error {
	for idx, claim := range claims {
		if strings.TrimSpace(claim.Statement) == "" {
			return fmt.Errorf("claims[%d].statement is required", idx)
		}
		if claim.ClaimType != "" && !isAllowedClaimType(claim.ClaimType) {
			return fmt.Errorf("claims[%d].claimType must be one of fact|inference|recommendation|assumption", idx)
		}
		if !allowRelationships {
			if len(claim.Dependencies) > 0 {
				return fmt.Errorf("claims[%d].dependencies is not allowed in proposal output; express prerequisites in applicability or boundaryConditions instead", idx)
			}
			if len(claim.ParentClaimIDs) > 0 {
				return fmt.Errorf("claims[%d].parentClaimIds is not allowed in proposal output", idx)
			}
		}
	}
	return nil
}

func validateSemanticFindings(expectedClaimID string, results []consensus.SemanticVerificationFinding) error {
	if len(results) != 1 {
		return fmt.Errorf("results must contain exactly one claim-level entry, got %d", len(results))
	}
	for idx, finding := range results {
		if strings.TrimSpace(finding.ClaimID) == "" {
			return fmt.Errorf("results[%d].claimId is required", idx)
		}
		if expected := strings.TrimSpace(expectedClaimID); expected != "" && strings.TrimSpace(finding.ClaimID) != expected {
			return fmt.Errorf("results[%d].claimId must equal the current claim %q", idx, expected)
		}
		if finding.TargetType != "" && finding.TargetType != "claim" {
			return fmt.Errorf("results[%d].targetType must be claim when provided", idx)
		}
		if !isAllowedClaimVerdict(finding.Verdict) {
			return fmt.Errorf("results[%d].verdict must be one of supported|refuted|insufficient_evidence|undetermined", idx)
		}
		if err := validateSemanticConfidence(idx, finding.Verdict, finding.Confidence); err != nil {
			return err
		}
		if strings.TrimSpace(finding.Rationale) == "" {
			return fmt.Errorf("results[%d].rationale is required", idx)
		}
	}
	return nil
}

func validateSemanticConfidence(idx int, verdict consensus.ClaimVerdict, confidence float64) error {
	if confidence <= 0 || confidence > 1 {
		return fmt.Errorf("results[%d].confidence must be within (0, 1]", idx)
	}
	switch verdict {
	case consensus.ClaimVerdictSupported, consensus.ClaimVerdictRefuted:
		if confidence < 0.60 {
			return fmt.Errorf("results[%d].confidence must be at least 0.60 when verdict=%s", idx, verdict)
		}
	case consensus.ClaimVerdictInsufficientEvidence:
		if confidence > 0.60 {
			return fmt.Errorf("results[%d].confidence must be at most 0.60 when verdict=insufficient_evidence", idx)
		}
	case consensus.ClaimVerdictUndetermined:
		if confidence < 0.35 || confidence > 0.65 {
			return fmt.Errorf("results[%d].confidence must be between 0.35 and 0.65 when verdict=undetermined", idx)
		}
	}
	return nil
}

func validateRevisionDrafts(claims []consensus.ClaimNode, revisions []consensus.ClaimRevisionDraft) error {
	validClaims := make(map[string]consensus.ClaimNode, len(claims))
	for _, claim := range claims {
		if claimID := strings.TrimSpace(claim.ClaimID); claimID != "" {
			validClaims[claimID] = claim
		}
	}
	seen := make(map[string]struct{}, len(revisions))
	for idx, revision := range revisions {
		targetClaimID := strings.TrimSpace(revision.TargetClaimID)
		if targetClaimID == "" {
			return fmt.Errorf("revisions[%d].targetClaimId is required", idx)
		}
		if _, exists := seen[targetClaimID]; exists {
			return fmt.Errorf("revisions[%d].targetClaimId duplicates earlier entry for %q", idx, targetClaimID)
		}
		seen[targetClaimID] = struct{}{}
		if !isAllowedRevisionAction(revision.Action) {
			return fmt.Errorf("revisions[%d].action must be one of revise|downgrade_confidence|withdraw|mark_unresolved|unchanged", idx)
		}
		claim, ok := validClaims[targetClaimID]
		if len(validClaims) > 0 && !ok {
			return fmt.Errorf("revisions[%d].targetClaimId must reference an existing claim in this task", idx)
		}
		revisedText := strings.TrimSpace(revision.RevisedText)
		if revision.Action == consensus.RevisionActionRevise || revision.Action == consensus.RevisionActionUnresolved {
			if revisedText == "" {
				return fmt.Errorf("revisions[%d].revisedText is required when action=%s", idx, revision.Action)
			}
			if ok && revisedText == strings.TrimSpace(claim.Statement) {
				return fmt.Errorf("revisions[%d].revisedText must materially narrow or clarify the current claim when action=%s", idx, revision.Action)
			}
		}
		if revision.Action == consensus.RevisionActionUnresolved && !revision.Unresolved {
			return fmt.Errorf("revisions[%d].unresolved must be true when action=mark_unresolved", idx)
		}
		if revision.Action == consensus.RevisionActionWithdraw && revisedText != "" {
			return fmt.Errorf("revisions[%d].revisedText is not allowed when action=withdraw", idx)
		}
		if revision.Action == consensus.RevisionActionUnchanged && (revisedText != "" || revision.Unresolved || revision.ConfidenceDelta != 0) {
			return fmt.Errorf("revisions[%d].unchanged must not carry revisedText, unresolved, or confidenceDelta", idx)
		}
		if revision.Action != consensus.RevisionActionUnchanged && strings.TrimSpace(revision.Reason) == "" {
			return fmt.Errorf("revisions[%d].reason is required when action=%s", idx, revision.Action)
		}
	}
	return nil
}

func validateDebateJudgements(judgements []consensus.DebateJudgementDraft) error {
	seen := make(map[string]struct{}, len(judgements))
	for idx, judgement := range judgements {
		claimID := strings.TrimSpace(judgement.ClaimID)
		if claimID == "" {
			return fmt.Errorf("judgements[%d].claimId is required", idx)
		}
		if _, exists := seen[claimID]; exists {
			return fmt.Errorf("judgements[%d].claimId duplicates earlier entry for %q", idx, claimID)
		}
		seen[claimID] = struct{}{}
		if !isAllowedDebateJudgement(judgement.Judgement) {
			return fmt.Errorf("judgements[%d].judgement must be one of agree|disagree|revise|no_change", idx)
		}
		if judgement.Judgement == consensus.DebateJudgementRevise && strings.TrimSpace(judgement.RevisedStatement) == "" {
			return fmt.Errorf("judgements[%d].revisedStatement is required when judgement=revise", idx)
		}
		if judgement.Judgement != consensus.DebateJudgementRevise && strings.TrimSpace(judgement.RevisedStatement) != "" {
			return fmt.Errorf("judgements[%d].revisedStatement is only allowed when judgement=revise", idx)
		}
	}
	return nil
}

func validateDebateVotes(votes []consensus.DebateVoteDraft) error {
	for idx, vote := range votes {
		if strings.TrimSpace(vote.ClaimID) == "" {
			return fmt.Errorf("votes[%d].claimId is required", idx)
		}
		if !isAllowedDebateVote(vote.Vote) {
			return fmt.Errorf("votes[%d].vote must be one of accept|reject|abstain", idx)
		}
	}
	return nil
}

func validateDelphiResponses(responses []consensus.DelphiResponseDraft) error {
	for idx, response := range responses {
		if strings.TrimSpace(response.StatementID) == "" && strings.TrimSpace(response.Statement) == "" {
			return fmt.Errorf("responses[%d] requires statementId or statement", idx)
		}
	}
	return nil
}

func validateArbiterOutput(output consensus.ArbiterTaskOutput) error {
	if !isAllowedTaskVerdict(output.TaskVerdict) {
		return fmt.Errorf("taskVerdict must be one of supported|partially_supported|undetermined|failed")
	}
	for idx, decision := range output.Decisions {
		if strings.TrimSpace(decision.ClaimID) == "" {
			return fmt.Errorf("decisions[%d].claimId is required", idx)
		}
		if !isAllowedClaimVerdict(decision.Verdict) {
			return fmt.Errorf("decisions[%d].verdict must be one of supported|refuted|insufficient_evidence|undetermined", idx)
		}
	}
	return nil
}

func isAllowedClaimType(value consensus.ClaimType) bool {
	switch value {
	case consensus.ClaimTypeFact, consensus.ClaimTypeInference, consensus.ClaimTypeRecommendation, consensus.ClaimTypeAssumption:
		return true
	default:
		return false
	}
}

func isAllowedClaimVerdict(value consensus.ClaimVerdict) bool {
	switch value {
	case consensus.ClaimVerdictSupported, consensus.ClaimVerdictRefuted, consensus.ClaimVerdictInsufficientEvidence, consensus.ClaimVerdictUndetermined:
		return true
	default:
		return false
	}
}

func isAllowedTaskVerdict(value consensus.TaskVerdict) bool {
	switch value {
	case consensus.TaskVerdictSupported, consensus.TaskVerdictPartiallySupported, consensus.TaskVerdictUndetermined, consensus.TaskVerdictFailed:
		return true
	default:
		return false
	}
}

func isAllowedRevisionAction(value consensus.RevisionAction) bool {
	switch value {
	case consensus.RevisionActionRevise, consensus.RevisionActionDowngrade, consensus.RevisionActionWithdraw, consensus.RevisionActionUnresolved, consensus.RevisionActionUnchanged:
		return true
	default:
		return false
	}
}

func isAllowedDebateJudgement(value consensus.DebateJudgement) bool {
	switch value {
	case consensus.DebateJudgementAgree, consensus.DebateJudgementDisagree, consensus.DebateJudgementRevise, consensus.DebateJudgementNoChange:
		return true
	default:
		return false
	}
}

func isAllowedDebateVote(value consensus.DebateVoteChoice) bool {
	switch value {
	case consensus.DebateVoteAccept, consensus.DebateVoteReject, consensus.DebateVoteAbstain:
		return true
	default:
		return false
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
