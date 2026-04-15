package consensus

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type deterministicIDs struct{ n int }

func (i *deterministicIDs) NewSessionID() string { return "session-1" }
func (i *deterministicIDs) NewEntityID(prefix string) string {
	i.n++
	return fmt.Sprintf("%s-%d", prefix, i.n)
}

type memoryLedger struct {
	entries []EvidenceRecord
}

func (l *memoryLedger) Append(_ context.Context, entry EvidenceRecord) (EvidenceRecord, error) {
	entry.Seq = len(l.entries)
	l.entries = append(l.entries, entry)
	return entry, nil
}

type captureObserver struct {
	events []RunEventType
}

func (o *captureObserver) OnEvent(_ context.Context, event RunEvent) error {
	o.events = append(o.events, event.Type)
	return nil
}

type stubStore struct {
	snapshot SessionSnapshot
}

func (s *stubStore) Save(_ context.Context, snapshot SessionSnapshot) error {
	s.snapshot = snapshot
	return nil
}

func (s *stubStore) Load(_ context.Context, _ string) (*SessionSnapshot, error) {
	cloned := s.snapshot
	return &cloned, nil
}

func (s *stubStore) Patch(_ context.Context, _ string, patch SessionPatch) error {
	if patch.Phase != nil {
		s.snapshot.Phase = *patch.Phase
	}
	if patch.ClaimGraph != nil {
		s.snapshot.ClaimGraph = append([]ClaimNode(nil), patch.ClaimGraph...)
	}
	if patch.ChallengeTickets != nil {
		s.snapshot.ChallengeTickets = append([]ChallengeTicket(nil), patch.ChallengeTickets...)
	}
	if patch.LedgerCursor != nil {
		s.snapshot.LedgerCursor = *patch.LedgerCursor
	}
	if patch.Result != nil {
		s.snapshot.Result = patch.Result
	}
	if patch.Error != nil {
		s.snapshot.Error = patch.Error
	}
	return nil
}

type stubDelegate struct {
	tasks map[string]Task
	next  int
}

func (d *stubDelegate) Dispatch(_ context.Context, task Task) (DispatchReceipt, error) {
	if d.tasks == nil {
		d.tasks = map[string]Task{}
	}
	taskID := fmt.Sprintf("task-%d", d.next)
	d.next++
	d.tasks[taskID] = task
	return DispatchReceipt{TaskID: taskID, AgentID: task.Meta().AgentID, Kind: task.Kind()}, nil
}

func (d *stubDelegate) Await(_ context.Context, taskID string, _ time.Duration) (AwaitedTask, error) {
	task := d.tasks[taskID]
	switch value := task.(type) {
	case ProposalTask:
		return AwaitedTask{OK: true, Output: ProposalTaskResult{Output: ProposalOutput{
			Summary: "proposal",
			Claims: []ClaimDraft{{
				Title:     "Race fix",
				Statement: "The patch fixes the race condition",
				Metadata:  map[string]any{"touchedPaths": []string{"internal/consensus/engine.go"}},
			}},
		}}}, nil
	case ChallengeTask:
		return AwaitedTask{OK: true, Output: ChallengeTaskResult{Output: ChallengeOutput{
			Summary: "challenge",
			Tickets: []ChallengeDraft{{
				ClaimID:   value.Claims[0].ClaimID,
				Statement: "Need more evidence",
				Kind:      "evidence-gap",
			}},
		}}}, nil
	case ReportTask:
		return AwaitedTask{OK: true, Output: ReportTaskResult{Output: AdjudicationReport{Summary: "report"}}}, nil
	case ActionTask:
		return AwaitedTask{OK: true, Output: ActionTaskResult{Output: ActionExecution{FullResponse: "done", Summary: "done"}}}, nil
	default:
		return AwaitedTask{OK: false, Error: "unexpected task"}, nil
	}
}

func (d *stubDelegate) Cancel(_ context.Context, _ string) error { return nil }

type stubVerifier struct {
	status VerificationStatus
}

func (v stubVerifier) Run(_ context.Context, req VerificationRequest) ([]VerificationResult, error) {
	return []VerificationResult{{
		VerificationID: "verify-1",
		ClaimID:        req.Claim.ClaimID,
		Kind:           "allowed_paths",
		Status:         v.status,
		Summary:        "verification result",
	}}, nil
}

func baseRequest() StartRequest {
	return StartRequest{
		RequestID: "req-1",
		TaskSpec: TaskSpec{
			Goal: "verify patch",
			Constraints: TaskConstraints{
				AllowedPaths: []string{"internal/consensus"},
			},
		},
		Roles: RoleAssignments{
			Proposers:   []string{"proposer-1"},
			Challengers: []string{"challenger-1"},
		},
		ProposalPolicy: ProposalPolicy{
			MaxPasses:          1,
			MaxClaimsPerWorker: 1,
		},
		VerificationPolicy: VerificationPolicy{
			RequiredChecks:    []VerificationCheck{{Name: "allowed", Kind: "allowed_paths"}},
			MaxParallelChecks: 1,
		},
		ArbiterPolicy: ArbiterPolicy{
			AllowUndetermined: true,
			BlindReview:       true,
		},
		ReportPolicy: ReportPolicy{Style: "builtin"},
		WaitingPolicy: WaitingPolicy{
			PerTaskTimeout: time.Second,
		},
	}
}

func TestEngineProducesSupportedVerdict(t *testing.T) {
	ids := &deterministicIDs{}
	observer := &captureObserver{}
	ledger := &memoryLedger{}
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusPassed},
		Observer:     observer,
		Ledger:       ledger,
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    ids,
	})
	result, err := engine.Start(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.TaskVerdict != TaskVerdictSupported {
		t.Fatalf("expected supported verdict, got %s", result.TaskVerdict)
	}
	if len(result.ClaimGraph) != 1 || result.ClaimGraph[0].Verdict != ClaimVerdictSupported {
		t.Fatalf("unexpected claim graph: %#v", result.ClaimGraph)
	}
	if len(result.ChallengeTickets) != 1 || result.ChallengeTickets[0].Status != ChallengeStatusClosed {
		t.Fatalf("expected closed challenge, got %#v", result.ChallengeTickets)
	}
	if len(ledger.entries) == 0 {
		t.Fatal("expected ledger entries to be written")
	}
	if len(observer.events) == 0 || observer.events[0] != RunEventSessionStarted {
		t.Fatalf("unexpected event stream: %#v", observer.events)
	}
}

func TestEngineMarksFailedVerificationAsRefuted(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusFailed},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	result, err := engine.Start(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.TaskVerdict != TaskVerdictFailed {
		t.Fatalf("expected failed verdict, got %s", result.TaskVerdict)
	}
	if result.ClaimGraph[0].Verdict != ClaimVerdictRefuted {
		t.Fatalf("expected refuted claim, got %#v", result.ClaimGraph[0])
	}
}
