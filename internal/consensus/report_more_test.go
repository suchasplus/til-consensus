package consensus

import (
	"context"
	"testing"
	"time"
)

type singleResultDelegate struct {
	output  TaskResult
	awaited bool
}

func (d *singleResultDelegate) Dispatch(_ context.Context, task Task) (DispatchReceipt, error) {
	return DispatchReceipt{TaskID: "task-1", AgentID: task.Meta().AgentID, Kind: task.Kind()}, nil
}

func (d *singleResultDelegate) Await(_ context.Context, _ string, _ time.Duration) (AwaitedTask, error) {
	d.awaited = true
	return AwaitedTask{OK: true, Output: d.output}, nil
}

func (d *singleResultDelegate) Cancel(_ context.Context, _ string) error { return nil }

type failingResultDelegate struct {
	awaitErr error
}

func (d *failingResultDelegate) Dispatch(_ context.Context, task Task) (DispatchReceipt, error) {
	return DispatchReceipt{TaskID: "task-1", AgentID: task.Meta().AgentID, Kind: task.Kind()}, nil
}

func (d *failingResultDelegate) Await(_ context.Context, _ string, _ time.Duration) (AwaitedTask, error) {
	return AwaitedTask{}, d.awaitErr
}

func (d *failingResultDelegate) Cancel(_ context.Context, _ string) error { return nil }

func TestComposeReportBuiltinAndDelegated(t *testing.T) {
	request := baseRequest()
	request.TaskSpec.Goal = "verify patch"
	request.Roles.Reporter = ""
	arbiter := ArbiterReport{
		TaskVerdict: TaskVerdictUndetermined,
		Summary:     "",
	}
	claims := []ClaimNode{
		{ClaimID: "claim-1", Title: "Need more evidence", Verdict: ClaimVerdictInsufficientEvidence},
	}
	tickets := []ChallengeTicket{
		{TicketID: "ticket-1", Status: ChallengeStatusOpen},
	}
	report, artifactRef, err := ComposeReport(context.Background(), nil, request, "session-1", arbiter, claims, tickets, WaitingPolicy{PerTaskTimeout: time.Second})
	if err != nil {
		t.Fatalf("ComposeReport builtin failed: %v", err)
	}
	if artifactRef != nil {
		t.Fatalf("expected builtin report to have no artifact, got %#v", artifactRef)
	}
	if report.Summary == "" || len(report.NextActions) == 0 {
		t.Fatalf("expected builtin report summary and next actions, got %#v", report)
	}

	request.Roles.Reporter = "reporter-1"
	delegate := &singleResultDelegate{
		output: ReportTaskResult{Output: AdjudicationReport{Summary: "delegated report"}},
	}
	report, artifactRef, err = ComposeReport(context.Background(), delegate, request, "session-1", arbiter, claims, tickets, WaitingPolicy{PerTaskTimeout: time.Second})
	if err != nil {
		t.Fatalf("ComposeReport delegated failed: %v", err)
	}
	if artifactRef != nil {
		t.Fatalf("expected no artifact from stub delegate, got %#v", artifactRef)
	}
	if report.Summary != "delegated report" || !delegate.awaited {
		t.Fatalf("unexpected delegated report: %#v", report)
	}
}

func TestDefaultArbiterDelegatesWhenConfigured(t *testing.T) {
	delegate := &singleResultDelegate{
		output: ArbiterTaskResult{Output: ArbiterTaskOutput{
			Summary:     "delegated arbiter",
			TaskVerdict: TaskVerdictSupported,
			Decisions: []ArbiterDecision{{
				ClaimID:    "claim-1",
				Verdict:    ClaimVerdictSupported,
				Confidence: 0.9,
			}},
		}},
	}
	arbiter := NewDefaultArbiter(DefaultArbiterDeps{
		TaskDelegate:   delegate,
		PerTaskTimeout: time.Second,
	})
	request := baseRequest()
	request.Roles.Arbiter = "arbiter-1"
	report, err := arbiter.Decide(context.Background(), ArbiterInput{
		Request:   request,
		SessionID: "session-1",
		Claims: []ClaimNode{{
			ClaimID:   "claim-1",
			Statement: "statement",
		}},
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if report.Summary != "delegated arbiter" || report.TaskVerdict != TaskVerdictSupported || !delegate.awaited {
		t.Fatalf("unexpected delegated arbiter report: %#v", report)
	}
}

func TestDefaultArbiterReturnsDelegationError(t *testing.T) {
	arbiter := NewDefaultArbiter(DefaultArbiterDeps{
		TaskDelegate:   &failingResultDelegate{awaitErr: context.DeadlineExceeded},
		PerTaskTimeout: time.Second,
	})
	request := baseRequest()
	request.Roles.Arbiter = "arbiter-1"
	_, err := arbiter.Decide(context.Background(), ArbiterInput{
		Request:   request,
		SessionID: "session-1",
		Claims: []ClaimNode{{
			ClaimID:   "claim-1",
			Statement: "statement",
		}},
	})
	if err == nil {
		t.Fatal("expected delegated arbiter failure to surface")
	}
}

func TestTaskKindsAndMeta(t *testing.T) {
	meta := TaskMeta{SessionID: "session-1", RequestID: "req-1", AgentID: "agent-1"}
	tasks := []Task{
		ProposalTask{TaskMeta: meta},
		ChallengeTask{TaskMeta: meta},
		SemanticVerificationTask{TaskMeta: meta},
		ReviseTask{TaskMeta: meta},
		ArbiterTask{TaskMeta: meta},
		ReportTask{TaskMeta: meta},
		ActionTask{TaskMeta: meta},
	}
	for _, task := range tasks {
		if task.Meta().AgentID != "agent-1" {
			t.Fatalf("unexpected task meta for %T: %#v", task, task.Meta())
		}
	}

	results := []TaskResult{
		ProposalTaskResult{},
		ChallengeTaskResult{},
		SemanticVerificationTaskResult{},
		ReviseTaskResult{},
		ArbiterTaskResult{},
		ReportTaskResult{},
		ActionTaskResult{},
	}
	wantKinds := []TaskKind{
		TaskKindPropose,
		TaskKindChallenge,
		TaskKindSemanticVerify,
		TaskKindRevise,
		TaskKindArbitrate,
		TaskKindReport,
		TaskKindAction,
	}
	for idx, result := range results {
		if result.Kind() != wantKinds[idx] {
			t.Fatalf("unexpected task kind at %d: got %s want %s", idx, result.Kind(), wantKinds[idx])
		}
	}
}
