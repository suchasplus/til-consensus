package consensus

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type DefaultWaitCoordinator struct {
	delegate TaskDelegate
	now      func() time.Time
}

func NewDefaultWaitCoordinator(delegate TaskDelegate) *DefaultWaitCoordinator {
	return &DefaultWaitCoordinator{
		delegate: delegate,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (c *DefaultWaitCoordinator) WaitRound(ctx context.Context, req WaitRoundRequest) (WaitRoundResult, error) {
	type settled struct {
		taskID        string
		participantID string
		status        string
		output        *ParticipantRoundOutput
		err           string
		at            string
	}
	states := map[string]settled{}
	var mu sync.Mutex
	ch := make(chan settled, len(req.Tasks))
	for _, task := range req.Tasks {
		go func(task WaitTask) {
			awaited, err := c.delegate.Await(ctx, task.TaskID, req.Policy.PerTaskTimeout)
			item := settled{
				taskID:        task.TaskID,
				participantID: task.ParticipantID,
				at:            c.now().Format(time.RFC3339Nano),
			}
			if err != nil {
				item.status = "failed"
				item.err = err.Error()
				ch <- item
				return
			}
			if !awaited.OK {
				if awaited.Error == "__timeout__" {
					item.status = "timeout"
					item.err = awaited.Error
				} else {
					item.status = "failed"
					item.err = awaited.Error
				}
				ch <- item
				return
			}
			result, ok := awaited.Output.(RoundTaskResult)
			if !ok {
				item.status = "failed"
				item.err = "invalid_round_result"
				ch <- item
				return
			}
			output := result.Output
			output.RespondedAt = item.at
			item.status = "completed"
			item.output = &output
			ch <- item
		}(task)
	}

	roundTimer := time.NewTimer(req.Policy.PerRoundTimeout)
	defer roundTimer.Stop()

	completedCount := 0
	for completedCount < len(req.Tasks) {
		select {
		case <-ctx.Done():
			return WaitRoundResult{}, ctx.Err()
		case item := <-ch:
			mu.Lock()
			if _, ok := states[item.taskID]; ok {
				mu.Unlock()
				continue
			}
			states[item.taskID] = item
			mu.Unlock()
			completedCount++
			if req.OnTaskSettled != nil {
				if err := req.OnTaskSettled(ctx, TaskSettled{
					TaskID:        item.taskID,
					ParticipantID: item.participantID,
					Status:        item.status,
					At:            item.at,
					Output:        item.output,
					Error:         item.err,
				}); err != nil {
					return WaitRoundResult{}, err
				}
			}
		case <-roundTimer.C:
			for _, task := range req.Tasks {
				mu.Lock()
				_, ok := states[task.TaskID]
				mu.Unlock()
				if ok {
					continue
				}
				if err := c.delegate.Cancel(ctx, task.TaskID); err != nil {
					return WaitRoundResult{}, fmt.Errorf("cancel task %s: %w", task.TaskID, err)
				}
				item := settled{
					taskID:        task.TaskID,
					participantID: task.ParticipantID,
					status:        "timeout",
					err:           "__round_timeout__",
					at:            c.now().Format(time.RFC3339Nano),
				}
				mu.Lock()
				states[task.TaskID] = item
				mu.Unlock()
				completedCount++
				if req.OnTaskSettled != nil {
					if err := req.OnTaskSettled(ctx, TaskSettled{
						TaskID:        item.taskID,
						ParticipantID: item.participantID,
						Status:        item.status,
						At:            item.at,
						Error:         item.err,
					}); err != nil {
						return WaitRoundResult{}, err
					}
				}
			}
		}
	}

	result := WaitRoundResult{}
	for _, task := range req.Tasks {
		state := states[task.TaskID]
		switch state.status {
		case "completed":
			if state.output != nil {
				result.Completed = append(result.Completed, *state.output)
			}
		case "timeout":
			result.TimedOut = append(result.TimedOut, task.TaskID)
		default:
			result.Failed = append(result.Failed, task.TaskID)
		}
	}
	return result, nil
}
