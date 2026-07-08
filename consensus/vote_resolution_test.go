package consensus

import (
	"strings"
	"testing"
)

func singleActiveClaim() []DebateClaim {
	return []DebateClaim{{
		ClaimID:   "claim-1",
		Statement: "Use a monorepo when cross-service changes dominate.",
		Active:    true,
	}}
}

// The pathological case from the run review: every voter votes reject but
// carries a high "confidence in my judgment" score. Those votes contradict
// the support-score contract and must not accept the claim.
func TestResolveDebateClaimsExcludesIncoherentRejectVotes(t *testing.T) {
	claims := singleActiveClaim()
	votes := []DebateVoteRecord{
		{ClaimID: "claim-1", AgentID: "voter-1", Vote: DebateVoteReject, Confidence: 0.90},
		{ClaimID: "claim-1", AgentID: "voter-2", Vote: DebateVoteReject, Confidence: 0.95},
		{ClaimID: "claim-1", AgentID: "voter-3", Vote: DebateVoteReject, Confidence: 0.88},
	}
	resolutions, outcome := resolveDebateClaims(claims, claims, votes, DebatePolicy{VoteThreshold: 0.67})
	if outcome != FreeDebateOutcomeNoConsensus {
		t.Fatalf("expected no consensus, got %s", outcome)
	}
	resolution := resolutions[0]
	if resolution.Accepted {
		t.Fatalf("expected incoherent reject votes to never accept a claim, got %#v", resolution)
	}
	if resolution.IncoherentVotes != 3 || resolution.VoteCount != 0 {
		t.Fatalf("expected 3 incoherent votes excluded from vote count, got %#v", resolution)
	}
	if resolution.SupportScore != 0 {
		t.Fatalf("expected zero support score without coherent votes, got %#v", resolution)
	}
}

func TestResolveDebateClaimsExcludesIncoherentAcceptVotes(t *testing.T) {
	claims := singleActiveClaim()
	votes := []DebateVoteRecord{
		{ClaimID: "claim-1", AgentID: "voter-1", Vote: DebateVoteAccept, Confidence: 0.10},
		{ClaimID: "claim-1", AgentID: "voter-2", Vote: DebateVoteAccept, Confidence: 0.90},
	}
	resolutions, _ := resolveDebateClaims(claims, claims, votes, DebatePolicy{VoteThreshold: 0.67})
	resolution := resolutions[0]
	if resolution.IncoherentVotes != 1 || resolution.VoteCount != 1 {
		t.Fatalf("expected one incoherent accept vote excluded, got %#v", resolution)
	}
	if !resolution.Accepted || resolution.SupportScore != 0.90 {
		t.Fatalf("expected remaining coherent vote to decide, got %#v", resolution)
	}
}

// Median (the default) resists a single low-balling voter where mean flips.
func TestResolveDebateClaimsMedianResistsSingleOutlier(t *testing.T) {
	claims := singleActiveClaim()
	votes := []DebateVoteRecord{
		{ClaimID: "claim-1", AgentID: "voter-1", Vote: DebateVoteAccept, Confidence: 0.80},
		{ClaimID: "claim-1", AgentID: "voter-2", Vote: DebateVoteAccept, Confidence: 0.75},
		{ClaimID: "claim-1", AgentID: "voter-3", Vote: DebateVoteReject, Confidence: 0.05},
	}
	medianResolutions, _ := resolveDebateClaims(claims, claims, votes, DebatePolicy{VoteThreshold: 0.67, VoteAggregation: DebateVoteAggregationMedian})
	if !medianResolutions[0].Accepted || medianResolutions[0].SupportScore != 0.75 {
		t.Fatalf("expected median 0.75 to accept, got %#v", medianResolutions[0])
	}
	meanResolutions, _ := resolveDebateClaims(claims, claims, votes, DebatePolicy{VoteThreshold: 0.67, VoteAggregation: DebateVoteAggregationMean})
	if meanResolutions[0].Accepted {
		t.Fatalf("expected mean %.4f to stay below threshold, got %#v", meanResolutions[0].SupportScore, meanResolutions[0])
	}
	// Empty aggregation falls back to median.
	defaultResolutions, _ := resolveDebateClaims(claims, claims, votes, DebatePolicy{VoteThreshold: 0.67})
	if defaultResolutions[0].SupportScore != medianResolutions[0].SupportScore {
		t.Fatalf("expected empty aggregation to behave as median, got %#v", defaultResolutions[0])
	}
}

func TestResolveDebateClaimsCountsAbstainIntoScoreNotRatio(t *testing.T) {
	claims := singleActiveClaim()
	votes := []DebateVoteRecord{
		{ClaimID: "claim-1", AgentID: "voter-1", Vote: DebateVoteAccept, Confidence: 0.90},
		{ClaimID: "claim-1", AgentID: "voter-2", Vote: DebateVoteAbstain, Confidence: 0.50},
	}
	resolutions, _ := resolveDebateClaims(claims, claims, votes, DebatePolicy{VoteThreshold: 0.67})
	resolution := resolutions[0]
	if resolution.SupportRatio != 1.0 {
		t.Fatalf("expected abstain excluded from label ratio, got %#v", resolution)
	}
	if resolution.SupportScore != 0.70 {
		t.Fatalf("expected abstain 0.5 to pull the median to 0.70, got %#v", resolution)
	}
	if len(resolution.AbstainingVoters) != 1 || resolution.AbstainingVoters[0] != "voter-2" {
		t.Fatalf("expected abstaining voter recorded, got %#v", resolution)
	}
	if resolution.VoteCount != 2 {
		t.Fatalf("expected both coherent votes counted, got %#v", resolution)
	}
}

func TestSupportScoreShownInSummaryMatchesDecision(t *testing.T) {
	claims := singleActiveClaim()
	votes := []DebateVoteRecord{
		{ClaimID: "claim-1", AgentID: "voter-1", Vote: DebateVoteAccept, Confidence: 0.80},
		{ClaimID: "claim-1", AgentID: "voter-2", Vote: DebateVoteAccept, Confidence: 0.75},
		{ClaimID: "claim-1", AgentID: "voter-3", Vote: DebateVoteReject, Confidence: 0.05},
	}
	resolutions, outcome := resolveDebateClaims(claims, claims, votes, DebatePolicy{VoteThreshold: 0.67})
	result := &RunResult{
		RequestID: "tc_test",
		Mode:      WorkflowModeFreeDebate,
		TaskSpec:  TaskSpec{Goal: "goal"},
		Report:    AdjudicationReport{Summary: "conclusion"},
		FreeDebate: &FreeDebateResultSection{
			Outcome:          outcome,
			Claims:           claims,
			ClaimResolutions: resolutions,
		},
	}
	summary := BuildRunSummary(result)
	if !strings.Contains(summary, "support=0.75") {
		t.Fatalf("expected summary to show the decision score 0.75, got:\n%s", summary)
	}
}
