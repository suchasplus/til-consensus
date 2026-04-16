package consensus

import (
	"context"
	"errors"
	"testing"
	"time"
)

type flakyTaskDelegate struct {
	dispatches int
	awaits     int
	failFirst  string
}

func (d *flakyTaskDelegate) Dispatch(_ context.Context, task Task) (DispatchReceipt, error) {
	d.dispatches++
	return DispatchReceipt{
		TaskID:  "task",
		AgentID: task.Meta().AgentID,
		Kind:    task.Kind(),
	}, nil
}

func (d *flakyTaskDelegate) Await(_ context.Context, _ string, _ time.Duration) (AwaitedTask, error) {
	d.awaits++
	if d.awaits == 1 {
		switch d.failFirst {
		case "timeout":
			return AwaitedTask{OK: false, Error: "__timeout__"}, nil
		case "await":
			return AwaitedTask{}, errors.New("await failed")
		}
	}
	return AwaitedTask{
		OK: true,
		Output: ProposalTaskResult{
			Output: ProposalOutput{Summary: "ok"},
		},
	}, nil
}

func (d *flakyTaskDelegate) Cancel(_ context.Context, _ string) error { return nil }

func TestExecuteTaskWithRetryRetriesOnceOnTimeout(t *testing.T) {
	delegate := &flakyTaskDelegate{failFirst: "timeout"}
	task := ProposalTask{TaskMeta: TaskMeta{AgentID: "proposer-a"}}
	receipt, awaited, attempts, err := ExecuteTaskWithRetry(context.Background(), delegate, task, time.Second, 1, TaskRetryHooks{})
	if err != nil {
		t.Fatalf("ExecuteTaskWithRetry failed: %v", err)
	}
	if attempts != 2 || delegate.dispatches != 2 || delegate.awaits != 2 {
		t.Fatalf("expected one retry, attempts=%d dispatches=%d awaits=%d", attempts, delegate.dispatches, delegate.awaits)
	}
	if !awaited.OK || receipt.AgentID != "proposer-a" {
		t.Fatalf("unexpected execution result: receipt=%#v awaited=%#v", receipt, awaited)
	}
}

func TestExecuteTaskWithRetryStopsWithoutRetryWhenDisabled(t *testing.T) {
	delegate := &flakyTaskDelegate{failFirst: "await"}
	task := ProposalTask{TaskMeta: TaskMeta{AgentID: "proposer-a"}}
	_, _, attempts, err := ExecuteTaskWithRetry(context.Background(), delegate, task, time.Second, 0, TaskRetryHooks{})
	if err == nil {
		t.Fatal("expected execution to fail")
	}
	if attempts != 1 || delegate.dispatches != 1 || delegate.awaits != 1 {
		t.Fatalf("expected no retry, attempts=%d dispatches=%d awaits=%d", attempts, delegate.dispatches, delegate.awaits)
	}
}
