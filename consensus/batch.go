package consensus

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

type taskBatchResult struct {
	Task    Task
	Receipt DispatchReceipt
	Awaited AwaitedTask
	Err     error
}

type verificationBatchResult struct {
	Claim    ClaimNode
	Findings []VerificationResult
	Err      error
}

func (e *Engine) executeTaskBatch(ctx context.Context, request StartRequest, sessionID string, tasks []Task, startedAt time.Time, timeout time.Duration, maxParallel int) []taskBatchResult {
	if len(tasks) == 0 {
		return nil
	}
	maxParallel = normalizeParallelism(maxParallel, len(tasks))
	results := make([]taskBatchResult, len(tasks))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for idx, task := range tasks {
		results[idx].Task = task
		wg.Add(1)
		go func(idx int, task Task) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx].Err = ctx.Err()
				return
			}
			receipt, awaited, err := e.executeTask(ctx, request, sessionID, task, startedAt, timeout)
			results[idx].Receipt = receipt
			results[idx].Awaited = awaited
			results[idx].Err = err
		}(idx, task)
	}
	wg.Wait()
	return results
}

func runVerificationBatch(ctx context.Context, verifier Verifier, request StartRequest, sessionID string, claims []ClaimNode, tickets []ChallengeTicket, maxParallel int) []verificationBatchResult {
	if len(claims) == 0 {
		return nil
	}
	maxParallel = normalizeParallelism(maxParallel, len(claims))
	results := make([]verificationBatchResult, len(claims))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for idx, claim := range claims {
		results[idx].Claim = claim
		wg.Add(1)
		go func(idx int, claim ClaimNode) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx].Err = ctx.Err()
				return
			}
			findings, err := verifier.Run(ctx, VerificationRequest{
				Request:    request,
				SessionID:  sessionID,
				Claim:      claim,
				Challenges: selectChallenges(tickets, claim.ClaimID),
			})
			results[idx].Findings = findings
			results[idx].Err = err
		}(idx, claim)
	}
	wg.Wait()
	return results
}

func normalizeParallelism(maxParallel int, itemCount int) int {
	if itemCount <= 0 {
		return 1
	}
	if maxParallel <= 0 || maxParallel > itemCount {
		return itemCount
	}
	return maxParallel
}

func recordTaskBatchResultMetrics(metrics *Metrics, result taskBatchResult) {
	metrics.TasksDispatched++
	if result.Err == nil {
		return
	}
	if errors.Is(result.Err, ErrGlobalDeadlineExceeded) {
		metrics.GlobalDeadlineHit = true
	}
	if strings.Contains(result.Err.Error(), "__timeout__") {
		metrics.WaitTimeouts++
	}
}

type lockedIDFactory struct {
	mu    sync.Mutex
	inner IDFactory
}

func newLockedIDFactory(inner IDFactory) IDFactory {
	if inner == nil {
		return nil
	}
	return &lockedIDFactory{inner: inner}
}

func (f *lockedIDFactory) NewSessionID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inner.NewSessionID()
}

func (f *lockedIDFactory) NewEntityID(prefix string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inner.NewEntityID(prefix)
}
