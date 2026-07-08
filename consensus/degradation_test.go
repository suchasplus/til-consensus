package consensus

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func newDebateDegradationRequest(participants ...string) StartRequest {
	request := baseRequest()
	request.Mode = WorkflowModeFreeDebate
	request.Roles = RoleAssignments{Participants: participants}
	request.DebatePolicy = DebatePolicy{
		MinRounds:       1,
		MaxRounds:       1,
		VoteThreshold:   1.0,
		EnableEarlyStop: true,
		PeerContextMode: "summary+active_claims",
	}
	return request
}

func newDegradationEngine(delegate TaskDelegate) *Engine {
	return NewEngine(EngineDeps{
		TaskDelegate: delegate,
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
}

func degradationsByKind(result *RunResult, kind DegradationKind) []Degradation {
	matched := make([]Degradation, 0)
	for _, item := range result.Degradations {
		if item.Kind == kind {
			matched = append(matched, item)
		}
	}
	return matched
}

func TestFreeDebateRecordsAbsentVoterDegradation(t *testing.T) {
	delegate := &stubDelegate{failAgentKinds: map[string]int{"debater-3/" + string(TaskKindFinalVote): 2}}
	engine := newDegradationEngine(delegate)
	result, err := engine.Start(context.Background(), newDebateDegradationRequest("debater-1", "debater-2", "debater-3"))
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := result.FreeDebate
	if section == nil {
		t.Fatal("expected free debate section")
	}
	if !slices.Equal(section.Voters, []string{"debater-1", "debater-2"}) {
		t.Fatalf("expected voters debater-1/debater-2, got %#v", section.Voters)
	}
	if !slices.Equal(section.AbsentVoters, []string{"debater-3"}) {
		t.Fatalf("expected absent voter debater-3, got %#v", section.AbsentVoters)
	}
	if section.Outcome != FreeDebateOutcomeConsensus {
		t.Fatalf("expected consensus outcome without quorum policy, got %s", section.Outcome)
	}
	absences := degradationsByKind(result, DegradationParticipantAbsent)
	if len(absences) != 1 {
		t.Fatalf("expected exactly one participant_absent degradation, got %#v", result.Degradations)
	}
	if absences[0].Phase != "final_vote" || absences[0].AgentID != "debater-3" {
		t.Fatalf("unexpected degradation record: %#v", absences[0])
	}
	if !strings.Contains(absences[0].Reason, "forced agent failure") {
		t.Fatalf("expected reason to carry task error, got %#v", absences[0])
	}
}

func TestFreeDebateQuorumNotMetOverridesOutcome(t *testing.T) {
	delegate := &stubDelegate{failAgentKinds: map[string]int{"debater-3/" + string(TaskKindFinalVote): 2}}
	engine := newDegradationEngine(delegate)
	request := newDebateDegradationRequest("debater-1", "debater-2", "debater-3")
	request.DebatePolicy.VoteQuorum = 0.75
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := result.FreeDebate
	if section == nil {
		t.Fatal("expected free debate section")
	}
	if section.Outcome != FreeDebateOutcomeQuorumNotMet {
		t.Fatalf("expected quorum_not_met outcome, got %s", section.Outcome)
	}
	if verdict := TaskVerdictFromDebateOutcome(section.Outcome); verdict != TaskVerdictUndetermined {
		t.Fatalf("expected undetermined verdict for quorum_not_met, got %s", verdict)
	}
	if quorumRecords := degradationsByKind(result, DegradationQuorumNotMet); len(quorumRecords) != 1 {
		t.Fatalf("expected one quorum_not_met degradation, got %#v", result.Degradations)
	}
	if absences := degradationsByKind(result, DegradationParticipantAbsent); len(absences) != 1 {
		t.Fatalf("expected one participant_absent degradation, got %#v", result.Degradations)
	}
	// Per-claim resolutions still reflect the votes that were cast.
	accepted := 0
	for _, resolution := range section.ClaimResolutions {
		if resolution.Accepted {
			accepted++
		}
	}
	if accepted == 0 {
		t.Fatal("expected cast votes to still resolve claims under quorum failure")
	}
}

func TestFreeDebateQuorumSatisfiedKeepsOutcome(t *testing.T) {
	delegate := &stubDelegate{failAgentKinds: map[string]int{"debater-3/" + string(TaskKindFinalVote): 2}}
	engine := newDegradationEngine(delegate)
	request := newDebateDegradationRequest("debater-1", "debater-2", "debater-3")
	request.DebatePolicy.VoteQuorum = 0.6
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.FreeDebate.Outcome != FreeDebateOutcomeConsensus {
		t.Fatalf("expected consensus with 2/3 voters over quorum 0.6, got %s", result.FreeDebate.Outcome)
	}
	if quorumRecords := degradationsByKind(result, DegradationQuorumNotMet); len(quorumRecords) != 0 {
		t.Fatalf("expected no quorum degradation, got %#v", quorumRecords)
	}
}

func TestFreeDebateSemanticDedupFailureRecordsStepSkipped(t *testing.T) {
	delegate := &stubDelegate{failKinds: map[TaskKind]int{TaskKindSemanticDedup: 2}}
	engine := newDegradationEngine(delegate)
	request := newDebateDegradationRequest("debater-1", "debater-2")
	request.Roles.SemanticDeduper = "deduper-1"
	request.DebatePolicy.SemanticDedup = DebateSemanticDedupPolicy{Enabled: true, SimilarityThreshold: 0.85}
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	skipped := degradationsByKind(result, DegradationStepSkipped)
	if len(skipped) != 1 {
		t.Fatalf("expected one step_skipped degradation, got %#v", result.Degradations)
	}
	if skipped[0].Phase != "semantic_dedup" || skipped[0].AgentID != "deduper-1" {
		t.Fatalf("unexpected degradation record: %#v", skipped[0])
	}
	for _, claim := range result.FreeDebate.Claims {
		if claim.MergedInto != "" {
			t.Fatalf("expected no merges after dedup failure, got %#v", claim)
		}
	}
}

func TestFreeDebateReporterFailureFallsBackToBuiltinReport(t *testing.T) {
	delegate := &stubDelegate{failKinds: map[TaskKind]int{TaskKindReport: 2}}
	engine := newDegradationEngine(delegate)
	request := newDebateDegradationRequest("debater-1", "debater-2")
	request.Roles.Reporter = "reporter-1"
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.Report.Summary == "report" {
		t.Fatal("expected builtin report fallback, got reporter output")
	}
	skipped := degradationsByKind(result, DegradationStepSkipped)
	if len(skipped) != 1 {
		t.Fatalf("expected one step_skipped degradation, got %#v", result.Degradations)
	}
	if skipped[0].Phase != "report" || skipped[0].AgentID != "reporter-1" {
		t.Fatalf("unexpected degradation record: %#v", skipped[0])
	}
}

func TestBuildRunSummaryRendersDegradationsAndVoters(t *testing.T) {
	result := &RunResult{
		RequestID: "tc_test",
		Mode:      WorkflowModeFreeDebate,
		TaskSpec:  TaskSpec{Goal: "goal"},
		Report:    AdjudicationReport{Summary: "conclusion"},
		Degradations: []Degradation{
			{Kind: DegradationStepSkipped, Phase: "semantic_dedup", Round: 4, AgentID: "deduper-gemini-api", Reason: "EOF", Impact: "语义去重未执行"},
			{Kind: DegradationParticipantAbsent, Phase: "final_vote", AgentID: "participant-gemini-api", Impact: "该参与者未投票"},
		},
		FreeDebate: &FreeDebateResultSection{
			Outcome:      FreeDebateOutcomeConsensus,
			Voters:       []string{"a", "b"},
			AbsentVoters: []string{"participant-gemini-api"},
		},
	}
	summary := BuildRunSummary(result)
	for _, fragment := range []string{
		"## Degradations",
		"- ⚠ step_skipped | semantic_dedup (round 4) | deduper-gemini-api | EOF",
		"  impact: 语义去重未执行",
		"- ⚠ participant_absent | final_vote | participant-gemini-api",
		"- voters: 2/3 (absent: participant-gemini-api)",
	} {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("summary missing fragment %q:\n%s", fragment, summary)
		}
	}
}
