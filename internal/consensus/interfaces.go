package consensus

import (
	"context"
	"time"
)

type DispatchReceipt struct {
	TaskID        string
	ParticipantID string
	Kind          TaskKind
}

type AwaitedTask struct {
	OK     bool
	Output TaskResult
	Error  string
}

type TaskDelegate interface {
	Dispatch(ctx context.Context, task Task) (DispatchReceipt, error)
	Await(ctx context.Context, taskID string, timeout time.Duration) (AwaitedTask, error)
	Cancel(ctx context.Context, taskID string) error
}

type TaskSettled struct {
	TaskID        string
	ParticipantID string
	Status        string
	At            string
	Output        *ParticipantRoundOutput
	Error         string
}

type WaitTask struct {
	TaskID        string
	ParticipantID string
}

type WaitRoundRequest struct {
	Round         int
	Tasks         []WaitTask
	Policy        WaitingPolicy
	OnTaskSettled func(context.Context, TaskSettled) error
}

type WaitRoundResult struct {
	Completed []ParticipantRoundOutput
	TimedOut  []string
	Failed    []string
}

type WaitCoordinator interface {
	WaitRound(ctx context.Context, req WaitRoundRequest) (WaitRoundResult, error)
}

type Observer interface {
	OnEvent(ctx context.Context, event ConsensusEvent) error
}

type SessionSnapshot struct {
	SessionID          string
	RequestID          string
	Participants       []string
	ActiveParticipants []string
	Eliminations       []EliminationRecord
	State              SessionState
	StartedAt          string
	RunningAt          string
	FinalizingAt       string
	FinishedAt         string
	FailedAt           string
	Result             *ConsensusResult
	Error              *FailureInfo
}

type SessionPatch struct {
	ActiveParticipants []string
	Eliminations       []EliminationRecord
	State              *SessionState
	RunningAt          *string
	FinalizingAt       *string
	FinishedAt         *string
	FailedAt           *string
	Result             *ConsensusResult
	Error              *FailureInfo
}

type SessionStore interface {
	Save(ctx context.Context, session SessionSnapshot) error
	Load(ctx context.Context, sessionID string) (*SessionSnapshot, error)
	Patch(ctx context.Context, sessionID string, patch SessionPatch) error
}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type IDFactory interface {
	NewSessionID() string
}
