package consensus

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func countDelegateTasks(delegate *stubDelegate, kind TaskKind) int {
	delegate.mu.Lock()
	defer delegate.mu.Unlock()
	total := 0
	for _, task := range delegate.tasks {
		if task.Kind() == kind {
			total++
		}
	}
	return total
}

func debateRoundTasks(delegate *stubDelegate) []DebateRoundTask {
	delegate.mu.Lock()
	defer delegate.mu.Unlock()
	tasks := make([]DebateRoundTask, 0)
	for _, task := range delegate.tasks {
		if typed, ok := task.(DebateRoundTask); ok {
			tasks = append(tasks, typed)
		}
	}
	return tasks
}

func TestFreeDebatePerRoundDedupRunsEachRound(t *testing.T) {
	for _, tc := range []struct {
		cadence   DebateSemanticDedupCadence
		wantCalls int
	}{
		{cadence: DebateSemanticDedupCadencePerRound, wantCalls: 2},
		{cadence: DebateSemanticDedupCadenceFinal, wantCalls: 1},
	} {
		delegate := &stubDelegate{}
		engine := newDegradationEngine(delegate)
		request := newDebateDegradationRequest("debater-1", "debater-2")
		request.DebatePolicy.MinRounds = 2
		request.DebatePolicy.MaxRounds = 2
		request.Roles.SemanticDeduper = "deduper-1"
		request.DebatePolicy.SemanticDedup = DebateSemanticDedupPolicy{
			Enabled:             true,
			SimilarityThreshold: 0.85,
			Cadence:             tc.cadence,
		}
		if _, err := engine.Start(context.Background(), request); err != nil {
			t.Fatalf("Start failed for cadence %s: %v", tc.cadence, err)
		}
		if calls := countDelegateTasks(delegate, TaskKindSemanticDedup); calls != tc.wantCalls {
			t.Fatalf("cadence %s: expected %d dedup calls, got %d", tc.cadence, tc.wantCalls, calls)
		}
	}
}

func TestFreeDebateTruncatesNewClaimsBeyondBudget(t *testing.T) {
	drafts := make([]ClaimDraft, 0, 4)
	for idx := 0; idx < 4; idx++ {
		drafts = append(drafts, ClaimDraft{
			Title:     fmt.Sprintf("extra claim %d", idx),
			Statement: fmt.Sprintf("distinct new position number %d", idx),
		})
	}
	delegate := &stubDelegate{debateDrafts: drafts}
	engine := newDegradationEngine(delegate)
	request := newDebateDegradationRequest("debater-1", "debater-2")
	request.DebatePolicy.MaxNewClaimsPerRound = 2
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	roundOneClaims := 0
	for _, claim := range result.FreeDebate.Claims {
		if claim.Round == 1 {
			roundOneClaims++
		}
	}
	if roundOneClaims != 2 {
		t.Fatalf("expected budget to keep 2 of 4 drafted claims, got %d", roundOneClaims)
	}
	for _, task := range debateRoundTasks(delegate) {
		if task.MaxNewClaims != 2 {
			t.Fatalf("expected round task budget 2, got %#v", task.MaxNewClaims)
		}
	}
}

func TestFreeDebateActiveClaimCeilingBlocksNewClaims(t *testing.T) {
	delegate := &stubDelegate{debateDrafts: []ClaimDraft{
		{Title: "late claim", Statement: "a brand new late position"},
	}}
	engine := newDegradationEngine(delegate)
	request := newDebateDegradationRequest("debater-1", "debater-2", "debater-3")
	request.DebatePolicy.MaxActiveClaims = 3
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	for _, claim := range result.FreeDebate.Claims {
		if claim.Round >= 1 {
			t.Fatalf("expected ceiling to block all round claims, got %#v", claim)
		}
	}
	tasks := debateRoundTasks(delegate)
	if len(tasks) == 0 {
		t.Fatal("expected debate round tasks to be dispatched")
	}
	for _, task := range tasks {
		if task.MaxNewClaims != -1 {
			t.Fatalf("expected ceiling to set MaxNewClaims=-1, got %d", task.MaxNewClaims)
		}
	}
}

func TestBuildRunSummaryShowsBallotAcceptanceRate(t *testing.T) {
	result := &RunResult{
		RequestID: "tc_rate",
		Mode:      WorkflowModeFreeDebate,
		TaskSpec:  TaskSpec{Goal: "goal"},
		Report:    AdjudicationReport{Summary: "conclusion"},
		FreeDebate: &FreeDebateResultSection{
			Outcome:    FreeDebateOutcomePartialConsensus,
			BallotSize: 4,
			ClaimResolutions: []DebateClaimResolution{
				{ClaimID: "claim-1", Accepted: true},
				{ClaimID: "claim-2", Accepted: true},
				{ClaimID: "claim-3", Accepted: false},
				{ClaimID: "claim-4", Accepted: false},
			},
		},
	}
	summary := BuildRunSummary(result)
	if !strings.Contains(summary, "- accepted claims: 2/4 ballot (50%)") {
		t.Fatalf("expected ballot acceptance rate line, got:\n%s", summary)
	}
}
