package consensus

import (
	"context"
	"sync"
	"testing"
	"time"
)

type waitTestDelegate struct {
	mu       sync.Mutex
	results  map[string]AwaitedTask
	delays   map[string]time.Duration
	canceled []string
}

func (d *waitTestDelegate) Dispatch(context.Context, Task) (DispatchReceipt, error) {
	return DispatchReceipt{}, nil
}

func (d *waitTestDelegate) Await(ctx context.Context, taskID string, _ time.Duration) (AwaitedTask, error) {
	delay := d.delays[taskID]
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return AwaitedTask{OK: false, Error: "__timeout__"}, nil
		case <-timer.C:
		}
	}
	return d.results[taskID], nil
}

func (d *waitTestDelegate) Cancel(_ context.Context, taskID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.canceled = append(d.canceled, taskID)
	return nil
}

func TestDefaultWaitCoordinatorTimeoutAndCancel(t *testing.T) {
	delegate := &waitTestDelegate{
		results: map[string]AwaitedTask{
			"done": {OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
				ParticipantID: "a",
				Phase:         PhaseDebate,
				Round:         1,
				FullResponse:  "ok",
				Summary:       "ok",
				Judgements:    []ClaimJudgement{{ClaimID: "c1", Stance: ClaimStanceAgree}},
			}}},
		},
		delays: map[string]time.Duration{
			"slow": 200 * time.Millisecond,
		},
	}
	coordinator := NewDefaultWaitCoordinator(delegate)
	result, err := coordinator.WaitRound(context.Background(), WaitRoundRequest{
		Round: 1,
		Tasks: []WaitTask{
			{TaskID: "done", ParticipantID: "a"},
			{TaskID: "slow", ParticipantID: "b"},
		},
		Policy: WaitingPolicy{
			PerTaskTimeout:  time.Second,
			PerRoundTimeout: 50 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Completed) != 1 {
		t.Fatalf("expected 1 completed result, got %#v", result)
	}
	if len(result.TimedOut) != 1 || result.TimedOut[0] != "slow" {
		t.Fatalf("expected slow to time out, got %#v", result.TimedOut)
	}
	if len(delegate.canceled) != 1 || delegate.canceled[0] != "slow" {
		t.Fatalf("expected slow to be canceled, got %#v", delegate.canceled)
	}
}
