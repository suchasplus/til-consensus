package consensus

import (
	"context"
	"strings"
	"testing"
)

func newSynthesisRequest(amendmentRounds int, participants ...string) StartRequest {
	request := newDebateDegradationRequest(participants...)
	request.Roles.Synthesizer = "synthesizer-1"
	request.DebatePolicy.Synthesis = DebateSynthesisPolicy{Enabled: true, AmendmentRounds: amendmentRounds}
	return request
}

func findSynthesisClaims(result *RunResult) []DebateClaim {
	matched := make([]DebateClaim, 0)
	for _, claim := range result.FreeDebate.Claims {
		if claim.Category == DebateClaimCategorySynthesis && claim.Active {
			matched = append(matched, claim)
		}
	}
	return matched
}

func TestFreeDebateSynthesisCreatesSingleCanonicalClaim(t *testing.T) {
	delegate := &stubDelegate{debateDrafts: []ClaimDraft{{
		Title:     "my synthesis",
		Statement: "participant-authored integrated recommendation",
		Category:  DebateClaimCategorySynthesis,
	}}}
	engine := newDegradationEngine(delegate)
	result, err := engine.Start(context.Background(), newSynthesisRequest(0, "debater-1", "debater-2"))
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	canonical := findSynthesisClaims(result)
	if len(canonical) != 1 {
		t.Fatalf("expected exactly one active synthesis claim, got %#v", canonical)
	}
	if canonical[0].OwnerID != "synthesizer-1" {
		t.Fatalf("expected canonical owned by synthesizer, got %#v", canonical[0])
	}
	// The participant-authored synthesis draft is consumed with provenance.
	consumed := false
	for _, claim := range result.FreeDebate.Claims {
		if strings.Contains(claim.Statement, "participant-authored") && claim.MergedInto == canonical[0].ClaimID {
			consumed = true
		}
	}
	if !consumed {
		t.Fatalf("expected participant synthesis draft merged into canonical, got %#v", result.FreeDebate.Claims)
	}
	// The canonical claim is on the ballot and voted.
	voted := false
	for _, resolution := range result.FreeDebate.ClaimResolutions {
		if resolution.ClaimID == canonical[0].ClaimID && resolution.VoteCount > 0 {
			voted = true
		}
	}
	if !voted {
		t.Fatal("expected canonical synthesis claim to receive votes")
	}
	for _, round := range result.FreeDebate.Rounds {
		if round.Phase == "amend" {
			t.Fatalf("expected no amend rounds with amendment_rounds=0, got %#v", round)
		}
	}
}

func TestFreeDebateSynthesisAmendmentIntegratesRevisions(t *testing.T) {
	delegate := &stubDelegate{reviseSynthesis: "revised canonical statement with SRM gate"}
	engine := newDegradationEngine(delegate)
	result, err := engine.Start(context.Background(), newSynthesisRequest(1, "debater-1", "debater-2"))
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	canonical := findSynthesisClaims(result)
	if len(canonical) != 1 {
		t.Fatalf("expected one canonical synthesis claim, got %#v", canonical)
	}
	if !strings.Contains(canonical[0].Statement, "[amended]") {
		t.Fatalf("expected amendments integrated into canonical statement, got %q", canonical[0].Statement)
	}
	amendRounds := 0
	for _, round := range result.FreeDebate.Rounds {
		if round.Phase == "amend" {
			amendRounds++
			for _, participant := range round.ParticipantOutputs {
				for _, judgement := range participant.Judgements {
					if judgement.Judgement == DebateJudgementRevise && judgement.RevisedStatement == "" {
						t.Fatalf("expected revised statement recorded, got %#v", judgement)
					}
				}
			}
		}
	}
	if amendRounds != 1 {
		t.Fatalf("expected one amend round record, got %d", amendRounds)
	}
}

func TestFreeDebateSynthesisAllAgreeKeepsDraft(t *testing.T) {
	delegate := &stubDelegate{}
	engine := newDegradationEngine(delegate)
	result, err := engine.Start(context.Background(), newSynthesisRequest(1, "debater-1", "debater-2"))
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	canonical := findSynthesisClaims(result)
	if len(canonical) != 1 {
		t.Fatalf("expected one canonical synthesis claim, got %#v", canonical)
	}
	if strings.Contains(canonical[0].Statement, "[amended]") {
		t.Fatalf("expected no integration pass when everyone agrees, got %q", canonical[0].Statement)
	}
}

func TestFreeDebateSynthesisFailureFallsBackToDrafts(t *testing.T) {
	delegate := &stubDelegate{
		failKinds: map[TaskKind]int{TaskKindSynthesis: 2},
		debateDrafts: []ClaimDraft{{
			Title:     "my synthesis",
			Statement: "participant-authored integrated recommendation",
			Category:  DebateClaimCategorySynthesis,
		}},
	}
	engine := newDegradationEngine(delegate)
	result, err := engine.Start(context.Background(), newSynthesisRequest(1, "debater-1", "debater-2"))
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	skipped := degradationsByKind(result, DegradationStepSkipped)
	if len(skipped) != 1 || skipped[0].Phase != "synthesis" {
		t.Fatalf("expected synthesis step_skipped degradation, got %#v", result.Degradations)
	}
	// Fallback: the participant drafts stay active on the ballot.
	drafts := findSynthesisClaims(result)
	if len(drafts) == 0 {
		t.Fatalf("expected participant synthesis drafts to remain active, got %#v", result.FreeDebate.Claims)
	}
	for _, round := range result.FreeDebate.Rounds {
		if round.Phase == "amend" {
			t.Fatalf("expected no amend rounds after synthesis failure, got %#v", round)
		}
	}
}

func TestFreeDebateSynthesisSkippedWithoutRole(t *testing.T) {
	request := newSynthesisRequest(1, "debater-1", "debater-2")
	request.Roles.Synthesizer = ""
	engine := newDegradationEngine(&stubDelegate{})
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if claims := findSynthesisClaims(result); len(claims) != 0 {
		t.Fatalf("expected no synthesis claims without a synthesizer role, got %#v", claims)
	}
	if len(result.Degradations) != 0 {
		t.Fatalf("expected no degradations for an unconfigured optional phase, got %#v", result.Degradations)
	}
}

func TestBuildRunSummaryRendersSynthesisGroup(t *testing.T) {
	result := &RunResult{
		RequestID: "tc_syn",
		Mode:      WorkflowModeFreeDebate,
		TaskSpec:  TaskSpec{Goal: "goal"},
		Report:    AdjudicationReport{Summary: "conclusion"},
		FreeDebate: &FreeDebateResultSection{
			Outcome:    FreeDebateOutcomeConsensus,
			BallotSize: 2,
			Claims: []DebateClaim{
				{ClaimID: "claim-syn", Statement: "the ratified synthesis", Category: DebateClaimCategorySynthesis, Active: true},
				{ClaimID: "claim-atom", Statement: "an atom claim", Active: true},
			},
			ClaimResolutions: []DebateClaimResolution{
				{ClaimID: "claim-syn", Accepted: true, SupportScore: 0.9, VoteCount: 2, SupportingVoters: []string{"a", "b"}, FinalStatement: "the ratified synthesis"},
				{ClaimID: "claim-atom", Accepted: true, SupportScore: 0.8, VoteCount: 2, SupportingVoters: []string{"a", "b"}, FinalStatement: "an atom claim"},
			},
		},
	}
	summary := BuildRunSummary(result)
	for _, fragment := range []string{"### Synthesis", "→ ratified", "### Accepted (1)"} {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("summary missing %q:\n%s", fragment, summary)
		}
	}
	synthesisAt := strings.Index(summary, "### Synthesis")
	acceptedAt := strings.Index(summary, "### Accepted")
	if synthesisAt > acceptedAt {
		t.Fatalf("expected synthesis group before accepted group, got %d/%d", synthesisAt, acceptedAt)
	}
}
