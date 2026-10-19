package consensus

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

var errAdjudicationCheckpointCrash = errors.New("injected interruption after durable adjudication checkpoint")

// captured 是持久化完成、尚未执行下一阶段时的独立 JSON 快照，避免把
// 错误处理期间写入的 failed 状态或旧 Engine 的内存对象当成崩溃恢复输入。
type adjudicationCrashStore struct {
	stubStore
	target   SessionPhase
	armed    bool
	captured []byte
}

func (s *adjudicationCrashStore) Save(ctx context.Context, snapshot SessionSnapshot) error {
	if err := s.stubStore.Save(ctx, snapshot); err != nil {
		return err
	}
	return s.interrupt()
}

func (s *adjudicationCrashStore) Patch(ctx context.Context, id string, patch SessionPatch) error {
	if err := s.stubStore.Patch(ctx, id, patch); err != nil {
		return err
	}
	if patch.Checkpoint == nil {
		return nil
	}
	return s.interrupt()
}

func (s *adjudicationCrashStore) interrupt() error {
	if !s.armed || s.snapshot.Checkpoint == nil || s.snapshot.Checkpoint.LastCompletedPhase != s.target {
		return nil
	}
	body, err := json.Marshal(s.snapshot)
	if err != nil {
		return err
	}
	s.captured = body
	s.armed = false
	return errAdjudicationCheckpointCrash
}

type adjudicationCountingVerifier struct {
	mu    sync.Mutex
	calls int
}

func (v *adjudicationCountingVerifier) Run(ctx context.Context, req VerificationRequest) ([]VerificationResult, error) {
	v.mu.Lock()
	v.calls++
	v.mu.Unlock()
	return (stubVerifier{status: VerificationStatusPassed}).Run(ctx, req)
}

func TestEngineResumesAdjudicationAfterDurablePhaseCheckpoint(t *testing.T) {
	for _, phase := range []SessionPhase{SessionPhaseCreated, SessionPhaseIngest, SessionPhasePropose, SessionPhaseChallenge, SessionPhaseVerify, SessionPhaseAdjudicate, SessionPhaseReport} {
		t.Run(string(phase), func(t *testing.T) {
			delegate := &stubDelegate{}
			verifier := &adjudicationCountingVerifier{}
			store := &adjudicationCrashStore{target: phase, armed: true}
			request := baseRequest()
			request.Roles.Arbiter = "arbiter-1"
			engine := NewEngine(EngineDeps{TaskDelegate: delegate, Verifier: verifier, SessionStore: store, Clock: fixedClock{now: time.Unix(1, 0).UTC()}, IDFactory: &deterministicIDs{}})
			if _, err := engine.Start(context.Background(), request); !errors.Is(err, errAdjudicationCheckpointCrash) {
				t.Fatalf("did not interrupt at durable %s checkpoint: %v", phase, err)
			}
			var saved SessionSnapshot
			if err := json.Unmarshal(store.captured, &saved); err != nil {
				t.Fatalf("missing durable JSON checkpoint: %v", err)
			}
			if saved.Checkpoint == nil || saved.Checkpoint.LastCompletedPhase != phase || saved.Phase != phase {
				t.Fatalf("interruption did not capture completed %s: %#v", phase, saved.Checkpoint)
			}
			if phase != SessionPhaseCreated && phase != SessionPhaseIngest && len(saved.ClaimGraph) == 0 {
				t.Fatal("phase recovery test did not preserve completed proposal data")
			}
			// 新 Engine 仅收到反序列化后的快照，不能复用旧状态机或已有 run。
			restoredStore := &stubStore{snapshot: saved}
			restarted := NewEngine(EngineDeps{TaskDelegate: delegate, Verifier: verifier, SessionStore: restoredStore, Clock: fixedClock{now: time.Unix(1, 0).UTC()}, IDFactory: &deterministicIDs{n: 1000}})
			result, err := restarted.Resume(context.Background(), saved)
			if err != nil {
				t.Fatalf("Resume from durable %s failed: %v", phase, err)
			}
			if result.SessionID != saved.SessionID || restoredStore.snapshot.Phase != SessionPhaseFinished || restoredStore.snapshot.ResumeCount != 1 {
				t.Fatalf("did not finish the original session: %#v", restoredStore.snapshot)
			}
			section := requireAdjudicationSection(t, result)
			if len(section.ClaimGraph) != 1 || len(section.ChallengeTickets) != 1 || len(section.VerificationResults) != 1 || len(section.AdjudicationRecords) != 1 {
				t.Fatalf("checkpoint data lost or replayed: %#v", section)
			}
			for _, kind := range []TaskKind{TaskKindPropose, TaskKindChallenge, TaskKindArbitrate} {
				if count := countDelegateTasks(delegate, kind); count != 1 {
					t.Errorf("completed %s was replayed or skipped: calls=%d", kind, count)
				}
			}
			verifier.mu.Lock()
			calls := verifier.calls
			verifier.mu.Unlock()
			if calls != 1 {
				t.Errorf("completed verification was replayed or skipped: calls=%d", calls)
			}
		})
	}
}
