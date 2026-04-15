package app

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

type Output struct {
	stdout  io.Writer
	stderr  io.Writer
	verbose bool
}

func NewOutput(stdout, stderr io.Writer, verbose bool) *Output {
	return &Output{stdout: stdout, stderr: stderr, verbose: verbose}
}

func (o *Output) Printf(format string, args ...any) {
	_, _ = fmt.Fprintf(o.stdout, format, args...)
}

func (o *Output) Errorf(format string, args ...any) {
	_, _ = fmt.Fprintf(o.stderr, format, args...)
}

func (o *Output) EventObserver() consensus.Observer {
	return observerFunc(func(_ context.Context, event consensus.ConsensusEvent) error {
		if !o.verbose {
			switch event.Type {
			case consensus.EventRoundDispatched, consensus.EventParticipantEliminated, consensus.EventActionFailed:
				o.Printf("[til-consensus] %s %s\n", event.Type, compactPayload(event.Payload))
			}
			return nil
		}
		o.Printf("[til-consensus] %s %s\n", event.Type, compactPayload(event.Payload))
		return nil
	})
}

func (o *Output) RunStarted(requestID, task string, agents []string) {
	o.Printf("[til-consensus] run started\n")
	o.Printf("  requestId: %s\n", requestID)
	o.Printf("  task: %s\n", task)
	o.Printf("  agents: %s\n", strings.Join(agents, ", "))
}

func (o *Output) RunCompleted(resultPath, summaryPath string) {
	o.Printf("[til-consensus] run completed\n")
	o.Printf("  result: %s\n", resultPath)
	o.Printf("  summary: %s\n", summaryPath)
}

func compactPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	parts := make([]string, 0, len(payload))
	for key, value := range payload {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	return strings.Join(parts, " ")
}

type observerFunc func(context.Context, consensus.ConsensusEvent) error

func (f observerFunc) OnEvent(ctx context.Context, event consensus.ConsensusEvent) error {
	return f(ctx, event)
}
