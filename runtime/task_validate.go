package runtime

import (
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
)

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
		if err := validateSemanticRationale(idx, finding.Verdict, finding.Rationale); err != nil {
			return err
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

func validateSemanticRationale(idx int, verdict consensus.ClaimVerdict, rationale string) error {
	sections, err := parseSemanticRationaleSections(rationale)
	if err != nil {
		return fmt.Errorf("results[%d].rationale %w", idx, err)
	}
	supportedCore := strings.TrimSpace(sections["supported_core"])
	missingOrConflict := strings.TrimSpace(sections["missing_or_conflict"])
	verdictReason := strings.TrimSpace(sections["verdict_reason"])
	if supportedCore == "" {
		return fmt.Errorf("results[%d].rationale supported_core must be non-empty", idx)
	}
	if verdictReason == "" {
		return fmt.Errorf("results[%d].rationale verdict_reason must be non-empty", idx)
	}
	switch verdict {
	case consensus.ClaimVerdictInsufficientEvidence, consensus.ClaimVerdictUndetermined:
		if semanticRationaleSectionMeansNone(missingOrConflict) {
			return fmt.Errorf("results[%d].rationale missing_or_conflict must name a concrete gap or conflict when verdict=%s", idx, verdict)
		}
	}
	return nil
}

func parseSemanticRationaleSections(rationale string) (map[string]string, error) {
	labels := []string{"supported_core:", "missing_or_conflict:", "verdict_reason:"}
	lower := strings.ToLower(rationale)
	indices := make([]int, 0, len(labels))
	for _, label := range labels {
		idx := strings.Index(lower, label)
		if idx < 0 {
			return nil, fmt.Errorf("must contain %q", strings.TrimSuffix(label, ":"))
		}
		indices = append(indices, idx)
	}
	if indices[0] >= indices[1] || indices[1] >= indices[2] {
		return nil, fmt.Errorf("must contain supported_core, missing_or_conflict, and verdict_reason in that order")
	}
	sections := make(map[string]string, len(labels))
	for i, label := range labels {
		start := indices[i] + len(label)
		end := len(rationale)
		if i+1 < len(indices) {
			end = indices[i+1]
		}
		key := strings.TrimSuffix(label, ":")
		sections[key] = strings.TrimSpace(strings.Trim(rationale[start:end], " |"))
	}
	return sections, nil
}

func semanticRationaleSectionMeansNone(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.Map(func(r rune) rune {
		switch r {
		case '.', ',', ';', ':', '|', '!', '?', '，', '。', '；', '：', '！', '？':
			return -1
		default:
			return r
		}
	}, normalized)
	normalized = strings.Join(strings.Fields(normalized), " ")
	switch normalized {
	case "", "none", "n/a", "na", "no material gaps", "no material conflict", "no material conflicts", "already stated caveats only":
		return true
	default:
		return false
	}
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

// validateDebateVoteCalibration rejects only the laziest ballots: five or
// more support-bearing votes that all carry the exact same confidence carry
// no comparative signal. Abstentions are diagnostic and their confidence is
// intentionally ignored here and by support-score aggregation.
func validateDebateVoteCalibration(votes []consensus.DebateVoteDraft) error {
	const minBallotForCalibration = 5
	supportVotes := make([]consensus.DebateVoteDraft, 0, len(votes))
	for _, vote := range votes {
		if vote.Vote != consensus.DebateVoteAbstain {
			supportVotes = append(supportVotes, vote)
		}
	}
	if len(supportVotes) < minBallotForCalibration {
		return nil
	}
	first := supportVotes[0].Confidence
	if first == nil {
		return nil
	}
	for _, vote := range supportVotes[1:] {
		if vote.Confidence == nil || *vote.Confidence != *first {
			return nil
		}
	}
	return fmt.Errorf("all %d support-bearing votes share confidence %.2f; calibrate scores comparatively so stronger claims score higher than weaker ones", len(supportVotes), *first)
}

// validateDebateNewClaims only checks the category label. Process/meta
// classification itself is the engine's job: it reroutes process claims to
// participant process notes, so one meta observation must not invalidate an
// otherwise good (and expensive) model response with a whole-task retry.
func validateDebateNewClaims(claims []consensus.ClaimDraft) error {
	for idx, claim := range claims {
		switch claim.Category {
		case "", consensus.DebateClaimCategoryDomain, consensus.DebateClaimCategoryProcess, consensus.DebateClaimCategorySynthesis:
		default:
			return fmt.Errorf("claims[%d].category must be domain, process, or synthesis, got %q", idx, claim.Category)
		}
	}
	return nil
}

func validateDebateClaimMerges(claims []consensus.DebateClaim, merges []consensus.DebateClaimMergeDraft, threshold float64) error {
	active := make(map[string]struct{}, len(claims))
	for _, claim := range claims {
		if claim.Active {
			active[claim.ClaimID] = struct{}{}
		}
	}
	seenSources := make(map[string]struct{}, len(merges))
	seenTargets := make(map[string]struct{}, len(merges))
	for idx, merge := range merges {
		sourceID := strings.TrimSpace(merge.SourceClaimID)
		targetID := strings.TrimSpace(merge.TargetClaimID)
		if sourceID == "" {
			return fmt.Errorf("merges[%d].sourceClaimId is required", idx)
		}
		if targetID == "" {
			return fmt.Errorf("merges[%d].targetClaimId is required", idx)
		}
		if sourceID == targetID {
			return fmt.Errorf("merges[%d] cannot merge a claim into itself", idx)
		}
		if _, ok := active[sourceID]; !ok {
			return fmt.Errorf("merges[%d].sourceClaimId %q is not an active input claim", idx, sourceID)
		}
		if _, ok := active[targetID]; !ok {
			return fmt.Errorf("merges[%d].targetClaimId %q is not an active input claim", idx, targetID)
		}
		if _, ok := seenSources[sourceID]; ok {
			return fmt.Errorf("merges[%d].sourceClaimId %q is duplicated", idx, sourceID)
		}
		seenSources[sourceID] = struct{}{}
		seenTargets[targetID] = struct{}{}
		if merge.Similarity < threshold || merge.Similarity > 1 {
			return fmt.Errorf("merges[%d].similarity must be in [%.2f,1]", idx, threshold)
		}
		if strings.TrimSpace(merge.Rationale) == "" {
			return fmt.Errorf("merges[%d].rationale is required", idx)
		}
	}
	for targetID := range seenTargets {
		if _, ok := seenSources[targetID]; ok {
			return fmt.Errorf("semantic dedup merges cannot be chained or cyclic: %q is both sourceClaimId and targetClaimId", targetID)
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
