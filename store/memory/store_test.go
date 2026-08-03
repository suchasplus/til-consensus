package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/suchasplus/til-consensus/consensus"
)

func TestStoreSaveLoadAndPatch(t *testing.T) {
	store := New()
	if err := store.Save(context.Background(), consensus.SessionSnapshot{
		SessionID:    "session-1",
		RequestID:    "req-1",
		Phase:        consensus.SessionPhaseIngest,
		LedgerCursor: 1,
	}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	snapshot, err := store.Load(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if snapshot == nil || snapshot.RequestID != "req-1" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if snapshot.Version != 1 {
		t.Fatalf("expected initial version 1, got %d", snapshot.Version)
	}

	phase := consensus.SessionPhaseFinished
	finishedAt := "2026-04-15T00:00:00Z"
	cursor := 7
	claims := []consensus.ClaimNode{{ClaimID: "claim-1", Statement: "statement"}}
	challenges := []consensus.ChallengeTicket{{TicketID: "ticket-1", ClaimID: "claim-1"}}
	result := &consensus.RunResult{
		SchemaVersion: consensus.SchemaVersion,
		Mode:          consensus.WorkflowModeAdjudication,
		RequestID:     "req-1",
		Adjudication: &consensus.AdjudicationResultSection{
			TaskVerdict: consensus.TaskVerdictSupported,
		},
	}
	failure := &consensus.FailureInfo{Code: "code", Message: "message"}
	stableStatementID := "statement-1"
	degradations := []consensus.Degradation{{Kind: consensus.DegradationParticipantAbsent, Phase: "delphi_round", Impact: "partial panel"}}
	if err := store.Patch(context.Background(), "session-1", consensus.SessionPatch{
		Phase:                       &phase,
		FinishedAt:                  &finishedAt,
		ClaimGraph:                  claims,
		ChallengeTickets:            challenges,
		LedgerCursor:                &cursor,
		Result:                      result,
		Error:                       failure,
		Degradations:                degradations,
		DelphiLastStableStatementID: &stableStatementID,
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	claims[0].Statement = "mutated"
	challenges[0].ClaimID = "mutated"

	patched, err := store.Load(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Load after patch failed: %v", err)
	}
	if patched.Phase != consensus.SessionPhaseFinished || patched.FinishedAt != finishedAt || patched.LedgerCursor != 7 {
		t.Fatalf("unexpected patched session: %#v", patched)
	}
	if patched.ClaimGraph[0].Statement != "statement" {
		t.Fatalf("expected claim graph to be cloned, got %#v", patched.ClaimGraph)
	}
	if patched.ChallengeTickets[0].ClaimID != "claim-1" {
		t.Fatalf("expected challenge tickets to be cloned, got %#v", patched.ChallengeTickets)
	}
	if patched.Result == nil || patched.Result.Adjudication == nil || patched.Result.Adjudication.TaskVerdict != consensus.TaskVerdictSupported {
		t.Fatalf("expected result to be stored, got %#v", patched.Result)
	}
	if patched.Error == nil || patched.Error.Code != "code" {
		t.Fatalf("expected error info to be stored, got %#v", patched.Error)
	}
	if len(patched.Degradations) != 1 || patched.Degradations[0].Impact != "partial panel" || patched.DelphiLastStableStatementID != stableStatementID {
		t.Fatalf("expected resumable workflow state to be stored, got %#v", patched)
	}
	if patched.Version != 2 {
		t.Fatalf("expected patched version 2, got %d", patched.Version)
	}
}

func TestStoreSaveAndPatchErrors(t *testing.T) {
	store := New()
	if err := store.Save(context.Background(), consensus.SessionSnapshot{}); err == nil {
		t.Fatal("expected empty session id to fail")
	}
	snapshot, err := store.Load(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Load missing failed: %v", err)
	}
	if snapshot != nil {
		t.Fatalf("expected nil snapshot, got %#v", snapshot)
	}
	if err := store.Patch(context.Background(), "missing", consensus.SessionPatch{}); err == nil {
		t.Fatal("expected patching unknown session to fail")
	}
}

func TestStorePatchCAS(t *testing.T) {
	ctx := context.Background()
	store := New()
	if err := store.Save(ctx, consensus.SessionSnapshot{SessionID: "session-cas"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	expected := uint64(1)
	phase := consensus.SessionPhaseIngest
	if err := store.Patch(ctx, "session-cas", consensus.SessionPatch{ExpectedVersion: &expected, Phase: &phase}); err != nil {
		t.Fatalf("first CAS patch failed: %v", err)
	}
	if err := store.Patch(ctx, "session-cas", consensus.SessionPatch{ExpectedVersion: &expected, Phase: &phase}); !errors.Is(err, consensus.ErrSessionVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}
