package consensus

import "fmt"

type SessionState string

const (
	SessionStateCreated    SessionState = "created"
	SessionStateRunning    SessionState = "running"
	SessionStateFinalizing SessionState = "finalizing"
	SessionStateFinished   SessionState = "finished"
	SessionStateFailed     SessionState = "failed"
)

type StateMachine struct {
	state SessionState
}

func NewStateMachine() *StateMachine {
	return &StateMachine{state: SessionStateCreated}
}

func (m *StateMachine) Current() SessionState {
	return m.state
}

func (m *StateMachine) Transition(next SessionState) error {
	if !allowedTransition(m.state, next) {
		return fmt.Errorf("invalid session state transition: %s -> %s", m.state, next)
	}
	m.state = next
	return nil
}

func allowedTransition(current, next SessionState) bool {
	switch current {
	case SessionStateCreated:
		return next == SessionStateRunning || next == SessionStateFailed
	case SessionStateRunning:
		return next == SessionStateFinalizing || next == SessionStateFailed
	case SessionStateFinalizing:
		return next == SessionStateFinished || next == SessionStateFailed
	default:
		return false
	}
}
