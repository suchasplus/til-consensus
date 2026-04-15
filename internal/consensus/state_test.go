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
		SessionPhaseIngest,
		SessionPhasePropose,
		SessionPhaseChallenge,
		SessionPhaseVerify,
		SessionPhaseAdjudicate,
		SessionPhaseReport,
		SessionPhaseFinished,
	}
	for _, phase := range phases {
		if err := state.Transition(phase); err != nil {
			t.Fatalf("transition to %s failed: %v", phase, err)
		}
	}
}
