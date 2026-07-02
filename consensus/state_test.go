package consensus

import "testing"

func TestStateMachineRejectsInvalidTransition(t *testing.T) {
	state := NewStateMachine()
	if err := state.Transition(SessionPhaseChallenge); err == nil {
		t.Fatal("expected invalid transition to fail")
	}
}

func TestStateMachineAcceptsNominalFlow(t *testing.T) {
	state := NewStateMachine()
	phases := []SessionPhase{
		SessionPhaseFrame,
		SessionPhaseIngest,
		SessionPhasePropose,
		SessionPhaseChallenge,
		SessionPhaseVerify,
		SessionPhaseAdjudicate,
		SessionPhaseReport,
		SessionPhaseObserve,
		SessionPhaseFinished,
	}
	for _, phase := range phases {
		if err := state.Transition(phase); err != nil {
			t.Fatalf("transition to %s failed: %v", phase, err)
		}
	}
}

func TestStateMachineAcceptsRevisionLoop(t *testing.T) {
	state := NewStateMachine()
	phases := []SessionPhase{
		SessionPhaseFrame,
		SessionPhaseIngest,
		SessionPhasePropose,
		SessionPhaseChallenge,
		SessionPhaseVerify,
		SessionPhaseRevise,
		SessionPhaseChallenge,
		SessionPhaseVerify,
		SessionPhaseAdjudicate,
		SessionPhaseReport,
		SessionPhaseAction,
		SessionPhaseObserve,
		SessionPhaseFinished,
	}
	for _, phase := range phases {
		if err := state.Transition(phase); err != nil {
			t.Fatalf("transition to %s failed: %v", phase, err)
		}
	}
}
