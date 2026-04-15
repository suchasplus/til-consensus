package consensus

import "fmt"

type SessionPhase string

const (
	SessionPhaseCreated    SessionPhase = "created"
	SessionPhaseIngest     SessionPhase = "ingest"
	SessionPhasePropose    SessionPhase = "propose"
	SessionPhaseChallenge  SessionPhase = "challenge"
	SessionPhaseVerify     SessionPhase = "verify"
	SessionPhaseAdjudicate SessionPhase = "adjudicate"
	SessionPhaseReport     SessionPhase = "report"
	SessionPhaseAction     SessionPhase = "action"
	SessionPhaseFinished   SessionPhase = "finished"
	SessionPhaseFailed     SessionPhase = "failed"
)

type StateMachine struct {
	state SessionPhase
}

func NewStateMachine() *StateMachine {
	return &StateMachine{state: SessionPhaseCreated}
}

func (m *StateMachine) Current() SessionPhase {
	return m.state
}

func (m *StateMachine) Transition(next SessionPhase) error {
	if !allowedTransition(m.state, next) {
		return fmt.Errorf("invalid session phase transition: %s -> %s", m.state, next)
	}
	m.state = next
	return nil
}

func allowedTransition(current, next SessionPhase) bool {
	switch current {
	case SessionPhaseCreated:
		return next == SessionPhaseIngest || next == SessionPhaseFailed
	case SessionPhaseIngest:
		return next == SessionPhasePropose || next == SessionPhaseFailed
	case SessionPhasePropose:
		return next == SessionPhaseChallenge || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseChallenge:
		return next == SessionPhaseVerify || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseVerify:
		return next == SessionPhaseAdjudicate || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseAdjudicate:
		return next == SessionPhaseReport || next == SessionPhaseAction || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseReport:
		return next == SessionPhaseAction || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseAction:
		return next == SessionPhaseFinished || next == SessionPhaseFailed
	default:
		return false
	}
}
