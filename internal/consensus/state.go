package consensus

import "fmt"

type SessionPhase string

const (
	SessionPhaseCreated             SessionPhase = "created"
	SessionPhaseFrame               SessionPhase = "frame"
	SessionPhaseIngest              SessionPhase = "ingest"
	SessionPhasePropose             SessionPhase = "propose"
	SessionPhaseChallenge           SessionPhase = "challenge"
	SessionPhaseVerify              SessionPhase = "verify"
	SessionPhaseRevise              SessionPhase = "revise"
	SessionPhaseAdjudicate          SessionPhase = "adjudicate"
	SessionPhaseInitial             SessionPhase = "initial"
	SessionPhaseDebate              SessionPhase = "debate"
	SessionPhaseFinalVote           SessionPhase = "final_vote"
	SessionPhaseDelphiQuestionnaire SessionPhase = "delphi_questionnaire"
	SessionPhaseDelphiSummary       SessionPhase = "delphi_summary"
	SessionPhaseDelphiRevision      SessionPhase = "delphi_revision"
	SessionPhaseReport              SessionPhase = "report"
	SessionPhaseAction              SessionPhase = "action"
	SessionPhaseObserve             SessionPhase = "observe"
	SessionPhaseFinished            SessionPhase = "finished"
	SessionPhaseFailed              SessionPhase = "failed"
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
		return next == SessionPhaseFrame || next == SessionPhaseFailed
	case SessionPhaseFrame:
		return next == SessionPhaseIngest || next == SessionPhaseFailed
	case SessionPhaseIngest:
		return next == SessionPhasePropose || next == SessionPhaseChallenge || next == SessionPhaseAdjudicate || next == SessionPhaseInitial || next == SessionPhaseDelphiQuestionnaire || next == SessionPhaseFailed
	case SessionPhasePropose:
		return next == SessionPhaseChallenge || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseChallenge:
		return next == SessionPhaseVerify || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseVerify:
		return next == SessionPhaseRevise || next == SessionPhaseAdjudicate || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseRevise:
		return next == SessionPhaseChallenge || next == SessionPhaseVerify || next == SessionPhaseAdjudicate || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseAdjudicate:
		return next == SessionPhaseIngest || next == SessionPhaseRevise || next == SessionPhaseReport || next == SessionPhaseAction || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseInitial:
		return next == SessionPhaseDebate || next == SessionPhaseFinalVote || next == SessionPhaseReport || next == SessionPhaseAction || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseDebate:
		return next == SessionPhaseFinalVote || next == SessionPhaseReport || next == SessionPhaseAction || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseFinalVote:
		return next == SessionPhaseReport || next == SessionPhaseAction || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseDelphiQuestionnaire:
		return next == SessionPhaseDelphiSummary || next == SessionPhaseReport || next == SessionPhaseAction || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseDelphiSummary:
		return next == SessionPhaseDelphiRevision || next == SessionPhaseReport || next == SessionPhaseAction || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseDelphiRevision:
		return next == SessionPhaseDelphiSummary || next == SessionPhaseReport || next == SessionPhaseAction || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseReport:
		return next == SessionPhaseAction || next == SessionPhaseObserve || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseAction:
		return next == SessionPhaseObserve || next == SessionPhaseFinished || next == SessionPhaseFailed
	case SessionPhaseObserve:
		return next == SessionPhaseFinished || next == SessionPhaseFailed
	default:
		return false
	}
}
