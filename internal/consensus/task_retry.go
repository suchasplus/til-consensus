package consensus

import (
	"context"
	"errors"
	"strings"
	"time"
)

type TaskExecutionStage string

const (
	TaskExecutionStageDispatch TaskExecutionStage = "dispatch"
	TaskExecutionStageAwait    TaskExecutionStage = "await"
	TaskExecutionStageResult   TaskExecutionStage = "result"
)

type TaskExecutionError struct {
	Stage TaskExecutionStage
	Cause error
}

func (e *TaskExecutionError) Error() string {
	if e == nil || e.Cause == nil {
		return "task execution failed"
	}
	return e.Cause.Error()
}

func (e *TaskExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type TaskRetryHooks struct {
	BeforeDispatch func(attempt int, maxAttempts int) error
	OnFailure      func(attempt int, maxAttempts int, reason string) error
	OnRetry        func(nextAttempt int, maxAttempts int, reason string) error
	OnSuccess      func(attempt int, maxAttempts int) error
}

func ExecuteTaskWithRetry(ctx context.Context, delegate TaskDelegate, task Task, timeout time.Duration, retryAttempts int, hooks TaskRetryHooks) (DispatchReceipt, AwaitedTask, int, error) {
	maxAttempts := retryAttempts + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var (
		lastReceipt DispatchReceipt
		lastAwaited AwaitedTask
		lastErr     error
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if hooks.BeforeDispatch != nil {
			if err := hooks.BeforeDispatch(attempt, maxAttempts); err != nil {
				return lastReceipt, lastAwaited, attempt, err
			}
		}
		receipt, err := delegate.Dispatch(ctx, task)
		lastReceipt = receipt
		if err != nil {
			lastErr = &TaskExecutionError{Stage: TaskExecutionStageDispatch, Cause: err}
			if hooks.OnFailure != nil {
				if hookErr := hooks.OnFailure(attempt, maxAttempts, lastErr.Error()); hookErr != nil {
					return lastReceipt, lastAwaited, attempt, hookErr
				}
			}
			if attempt < maxAttempts && isRetriableTaskError(lastErr) {
				if hooks.OnRetry != nil {
					if hookErr := hooks.OnRetry(attempt+1, maxAttempts, lastErr.Error()); hookErr != nil {
						return lastReceipt, lastAwaited, attempt, hookErr
					}
				}
				continue
			}
			return lastReceipt, lastAwaited, attempt, lastErr
		}
		awaited, err := delegate.Await(ctx, receipt.TaskID, timeout)
		lastAwaited = awaited
		if err != nil {
			lastErr = &TaskExecutionError{Stage: TaskExecutionStageAwait, Cause: err}
			if hooks.OnFailure != nil {
				if hookErr := hooks.OnFailure(attempt, maxAttempts, lastErr.Error()); hookErr != nil {
					return lastReceipt, lastAwaited, attempt, hookErr
				}
			}
			if attempt < maxAttempts && isRetriableTaskError(lastErr) {
				if hooks.OnRetry != nil {
					if hookErr := hooks.OnRetry(attempt+1, maxAttempts, lastErr.Error()); hookErr != nil {
						return lastReceipt, lastAwaited, attempt, hookErr
					}
				}
				continue
			}
			return lastReceipt, AwaitedTask{}, attempt, lastErr
		}
		if !awaited.OK {
			lastErr = &TaskExecutionError{Stage: TaskExecutionStageResult, Cause: errors.New(strings.TrimSpace(awaited.Error))}
			if hooks.OnFailure != nil {
				if hookErr := hooks.OnFailure(attempt, maxAttempts, retryReason(lastErr, awaited)); hookErr != nil {
					return lastReceipt, lastAwaited, attempt, hookErr
				}
			}
			if attempt < maxAttempts && isRetriableTaskError(lastErr) {
				if hooks.OnRetry != nil {
					if hookErr := hooks.OnRetry(attempt+1, maxAttempts, retryReason(lastErr, awaited)); hookErr != nil {
						return lastReceipt, lastAwaited, attempt, hookErr
					}
				}
				continue
			}
			return lastReceipt, awaited, attempt, lastErr
		}
		if hooks.OnSuccess != nil {
			if err := hooks.OnSuccess(attempt, maxAttempts); err != nil {
				return lastReceipt, lastAwaited, attempt, err
			}
		}
		return receipt, awaited, attempt, nil
	}
	if lastErr == nil {
		lastErr = errors.New("task execution failed")
	}
	return lastReceipt, lastAwaited, maxAttempts, lastErr
}

func isRetriableTaskError(err error) bool {
	if err == nil {
		return false
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrGlobalDeadlineExceeded)
}

func retryReason(err error, awaited AwaitedTask) string {
	if text := strings.TrimSpace(awaited.Error); text != "" {
		return text
	}
	if err != nil {
		return err.Error()
	}
	return "task failed"
}
