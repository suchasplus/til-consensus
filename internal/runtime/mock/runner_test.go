package mock

import (
	"context"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestRunTaskBehaviors(t *testing.T) {
	task := consensus.RoundTask{
		TaskMeta: consensus.TaskMeta{ParticipantID: "a"},
		Phase:    consensus.PhaseInitial,
		Round:    0,
	}
	agent := config.AgentConfig{ID: "a"}

	t.Run("deterministic", func(t *testing.T) {
		got, err := RunTask(context.Background(), task, agent, config.ProviderConfig{Type: "mock", Behavior: "deterministic"})
		if err != nil {
			t.Fatal(err)
		}
		if got == nil {
			t.Fatal("expected deterministic output")
		}
	})

	t.Run("error", func(t *testing.T) {
		_, err := RunTask(context.Background(), task, agent, config.ProviderConfig{Type: "mock", Behavior: "error", Error: "boom"})
		if err == nil || err.Error() != "boom" {
			t.Fatalf("expected boom error, got %v", err)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		got, err := RunTask(context.Background(), task, agent, config.ProviderConfig{Type: "mock", Behavior: "malformed"})
		if err != nil {
			t.Fatal(err)
		}
		if got.(string) != "not json" {
			t.Fatalf("unexpected malformed output: %#v", got)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if _, err := RunTask(ctx, task, agent, config.ProviderConfig{Type: "mock", Behavior: "timeout"}); err == nil {
			t.Fatal("expected timeout error")
		}
	})
}
