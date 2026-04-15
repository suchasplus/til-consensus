package consensus

import "testing"

func TestStateMachineRejectsInvalidTransition(t *testing.T) {
	m := NewStateMachine()
	if err := m.Transition(SessionStateFinished); err == nil {
		t.Fatal("expected invalid transition error")
	}
}
