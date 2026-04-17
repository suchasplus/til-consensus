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

func StrictDecodeTaskOutput(task consensus.Task, raw any) (consensus.TaskResult, error) {
	switch value := raw.(type) {
	case string:
		payload, err := strictJSONObjectBytes(value)
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
	switch task.(type) {
	case consensus.ProposalTask:
		var output consensus.ProposalOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode proposal output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("proposal output missing summary")
		}
		if err := validateClaimDrafts(output.Claims); err != nil {
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
		if err := validateClaimDrafts(output.Claims); err != nil {
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
		if err := validateRevisionDrafts(output.Revisions); err != nil {
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
		var output consensus.SemanticVerificationOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode semantic verification output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("semantic verification output missing summary")
		}
		if err := validateSemanticFindings(output.Results); err != nil {
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

func strictJSONObjectBytes(text string) ([]byte, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("strict JSON object required: output is empty")
	}
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return nil, fmt.Errorf("strict JSON object required: output must contain exactly one JSON object and no wrapper text")
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("strict JSON decode failed: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != nil && err != io.EOF {
		return nil, fmt.Errorf("strict JSON object required: unexpected trailing data: %w", err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal strict JSON object: %w", err)
	}
	return payload, nil
}

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

func validateClaimDrafts(claims []consensus.ClaimDraft) error {
	for idx, claim := range claims {
		if strings.TrimSpace(claim.Statement) == "" {
			return fmt.Errorf("claims[%d].statement is required", idx)
		}
		if claim.ClaimType != "" && !isAllowedClaimType(claim.ClaimType) {
			return fmt.Errorf("claims[%d].claimType must be one of fact|inference|recommendation|assumption", idx)
		}
	}
	return nil
}

func validateSemanticFindings(results []consensus.SemanticVerificationFinding) error {
	for idx, finding := range results {
		if strings.TrimSpace(finding.ClaimID) == "" {
			return fmt.Errorf("results[%d].claimId is required", idx)
		}
		if !isAllowedClaimVerdict(finding.Verdict) {
			return fmt.Errorf("results[%d].verdict must be one of supported|refuted|insufficient_evidence|undetermined", idx)
		}
		if strings.TrimSpace(finding.Rationale) == "" {
			return fmt.Errorf("results[%d].rationale is required", idx)
		}
	}
	return nil
}

func validateRevisionDrafts(revisions []consensus.ClaimRevisionDraft) error {
	for idx, revision := range revisions {
		if strings.TrimSpace(revision.TargetClaimID) == "" {
			return fmt.Errorf("revisions[%d].targetClaimId is required", idx)
		}
		if !isAllowedRevisionAction(revision.Action) {
			return fmt.Errorf("revisions[%d].action must be one of revise|downgrade_confidence|withdraw|mark_unresolved|unchanged", idx)
		}
	}
	return nil
}

func validateDebateJudgements(judgements []consensus.DebateJudgementDraft) error {
	for idx, judgement := range judgements {
		if strings.TrimSpace(judgement.ClaimID) == "" {
			return fmt.Errorf("judgements[%d].claimId is required", idx)
		}
		if !isAllowedDebateJudgement(judgement.Judgement) {
			return fmt.Errorf("judgements[%d].judgement must be one of agree|disagree|revise|no_change", idx)
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
