package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
)

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
		if err := validateDebateNewClaims(output.Claims); err != nil {
			return nil, fmt.Errorf("validate initial proposal claims: %w", err)
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
		if err := validateDebateNewClaims(output.NewClaims); err != nil {
			return nil, fmt.Errorf("validate debate round newClaims: %w", err)
		}
		if err := validateDebateJudgements(output.Judgements); err != nil {
			return nil, fmt.Errorf("validate debate round output: %w", err)
		}
		return consensus.DebateRoundTaskResult{Output: output}, nil
	case consensus.SemanticDedupTask:
		var output consensus.SemanticDedupOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode semantic dedup output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("semantic dedup output missing summary")
		}
		if err := validateDebateClaimMerges(typed.Claims, output.Merges, typed.SimilarityThreshold); err != nil {
			return nil, fmt.Errorf("validate semantic dedup output: %w", err)
		}
		return consensus.SemanticDedupTaskResult{Output: output}, nil
	case consensus.SynthesisTask:
		var output consensus.SynthesisOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode synthesis output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("synthesis output missing summary")
		}
		if strings.TrimSpace(output.Claim.Statement) == "" {
			return nil, fmt.Errorf("synthesis output missing claim.statement")
		}
		return consensus.SynthesisTaskResult{Output: output}, nil
	case consensus.FinalVoteTask:
		voteTask := task.(consensus.FinalVoteTask)
		var output consensus.FinalVoteOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode final vote output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("final vote output missing summary")
		}
		if err := consensus.ValidateDebateVotes(voteTask.Claims, output.Votes); err != nil {
			return nil, fmt.Errorf("validate final vote output: %w", err)
		}
		if err := validateDebateVoteCalibration(output.Votes); err != nil {
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
		delphiTask := task.(consensus.DelphiQuestionnaireTask)
		var output consensus.DelphiQuestionnaireOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode delphi questionnaire output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("delphi questionnaire output missing summary")
		}
		if err := consensus.ValidateDelphiResponses(output.Responses, delphiTask.RatingScaleMin, delphiTask.RatingScaleMax); err != nil {
			return nil, fmt.Errorf("validate delphi questionnaire output: %w", err)
		}
		return consensus.DelphiQuestionnaireTaskResult{Output: output}, nil
	case consensus.DelphiRevisionTask:
		delphiTask := task.(consensus.DelphiRevisionTask)
		var output consensus.DelphiRevisionOutput
		if err := json.Unmarshal(payload, &output); err != nil {
			return nil, fmt.Errorf("decode delphi revision output: %w", err)
		}
		if strings.TrimSpace(output.Summary) == "" {
			return nil, fmt.Errorf("delphi revision output missing summary")
		}
		if err := consensus.ValidateDelphiResponses(output.Responses, delphiTask.RatingScaleMin, delphiTask.RatingScaleMax); err != nil {
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
