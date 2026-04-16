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
	return observerFunc(func(_ context.Context, event consensus.RunEvent) error {
		switch event.Type {
		case consensus.RunEventPhaseChanged:
			o.Printf("[til-consensus] %s\n", formatPhaseChanged(event.Phase))
			return nil
		case consensus.RunEventTaskDispatched:
			o.Printf("[til-consensus] %s\n", formatTaskDispatched(event.Payload))
			return nil
		case consensus.RunEventTaskRetrying:
			o.Printf("[til-consensus] %s\n", formatTaskRetrying(event.Payload))
			return nil
		case consensus.RunEventTaskFailed:
			o.Printf("[til-consensus] %s\n", formatTaskFailed(event.Payload))
			return nil
		}
		if !o.verbose {
			return nil
		}
		if event.Type == consensus.RunEventTaskCompleted {
			o.Printf("[til-consensus] %s\n", formatTaskCompleted(event.Payload))
			return nil
		}
		if text := compactPayload(event.Payload); text != "" {
			o.Printf("[til-consensus] %s %s\n", event.Type, text)
		} else {
			o.Printf("[til-consensus] %s\n", event.Type)
		}
		return nil
	})
}

func formatTaskDispatched(payload map[string]any) string {
	agentID := stringValue(payload, "agentId")
	taskKind := stringValue(payload, "taskKind")
	if agentID == "" && taskKind == "" {
		return withAttemptSuffix(payload, "task dispatched")
	}
	if agentID == "" {
		return withAttemptSuffix(payload, fmt.Sprintf("task dispatched: %s (%s)", taskKind, taskKindLabel(taskKind)))
	}
	if taskKind == "" {
		return withAttemptSuffix(payload, fmt.Sprintf("task dispatched: %s", agentID))
	}
	return withAttemptSuffix(payload, fmt.Sprintf("task dispatched: %s -> %s (%s)", agentID, taskKind, taskKindLabel(taskKind)))
}

func formatTaskCompleted(payload map[string]any) string {
	agentID := stringValue(payload, "agentId")
	taskKind := stringValue(payload, "taskKind")
	if agentID == "" && taskKind == "" {
		return withAttemptSuffix(payload, "task completed")
	}
	if agentID == "" {
		return withAttemptSuffix(payload, fmt.Sprintf("task completed: %s", taskKind))
	}
	if taskKind == "" {
		return withAttemptSuffix(payload, fmt.Sprintf("task completed: %s", agentID))
	}
	return withAttemptSuffix(payload, fmt.Sprintf("task completed: %s -> %s", agentID, taskKind))
}

func formatTaskRetrying(payload map[string]any) string {
	agentID := stringValue(payload, "agentId")
	taskKind := stringValue(payload, "taskKind")
	errText := stringValue(payload, "error")
	prefix := "task retrying"
	if agentID != "" && taskKind != "" {
		prefix = fmt.Sprintf("task retrying: %s -> %s (%s)", agentID, taskKind, taskKindLabel(taskKind))
	} else if agentID != "" {
		prefix = fmt.Sprintf("task retrying: %s", agentID)
	} else if taskKind != "" {
		prefix = fmt.Sprintf("task retrying: %s (%s)", taskKind, taskKindLabel(taskKind))
	}
	prefix = withAttemptSuffix(payload, prefix)
	if errText == "" {
		return prefix
	}
	return prefix + " reason=" + errText
}

func formatTaskFailed(payload map[string]any) string {
	agentID := stringValue(payload, "agentId")
	taskKind := stringValue(payload, "taskKind")
	errText := stringValue(payload, "error")
	prefix := "task failed"
	if agentID != "" && taskKind != "" {
		prefix = fmt.Sprintf("task failed: %s -> %s (%s)", agentID, taskKind, taskKindLabel(taskKind))
	} else if agentID != "" {
		prefix = fmt.Sprintf("task failed: %s", agentID)
	} else if taskKind != "" {
		prefix = fmt.Sprintf("task failed: %s (%s)", taskKind, taskKindLabel(taskKind))
	}
	if errText == "" {
		return withAttemptSuffix(payload, prefix)
	}
	return withAttemptSuffix(payload, prefix) + " error=" + errText
}

func stringValue(payload map[string]any, key string) string {
	if len(payload) == 0 {
		return ""
	}
	value, ok := payload[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func taskKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case string(consensus.TaskKindPropose):
		return "collecting claims"
	case string(consensus.TaskKindInitialProposal):
		return "collecting initial positions"
	case string(consensus.TaskKindChallenge):
		return "collecting challenges"
	case string(consensus.TaskKindRevise):
		return "revising challenged claims"
	case string(consensus.TaskKindDebateRound):
		return "running debate round"
	case string(consensus.TaskKindFinalVote):
		return "collecting final votes"
	case string(consensus.TaskKindSemanticVerify):
		return "running semantic verification"
	case string(consensus.TaskKindDelphiQuestionnaire):
		return "collecting delphi questionnaire"
	case string(consensus.TaskKindDelphiRevision):
		return "collecting delphi revisions"
	case string(consensus.TaskKindDelphiFacilitatorSummary):
		return "building delphi facilitator summary"
	case string(consensus.TaskKindArbitrate):
		return "making adjudication decision"
	case string(consensus.TaskKindReport):
		return "building report"
	case string(consensus.TaskKindAction):
		return "executing action"
	default:
		return "unknown"
	}
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

func withAttemptSuffix(payload map[string]any, text string) string {
	attempt := stringValue(payload, "attempt")
	maxAttempts := stringValue(payload, "maxAttempts")
	if attempt == "" || maxAttempts == "" {
		return text
	}
	return fmt.Sprintf("%s attempt=%s/%s", text, attempt, maxAttempts)
}

func formatPhaseChanged(phase consensus.SessionPhase) string {
	label := phaseLabel(phase)
	if phase == "" {
		return "phase changed"
	}
	return fmt.Sprintf("phase: %s (%s)", phase, label)
}

func phaseLabel(phase consensus.SessionPhase) string {
	switch phase {
	case consensus.SessionPhaseCreated:
		return "session created"
	case consensus.SessionPhaseFrame:
		return "framing case manifest"
	case consensus.SessionPhaseIngest:
		return "ingesting task"
	case consensus.SessionPhasePropose:
		return "collecting claims from proposers"
	case consensus.SessionPhaseInitial:
		return "collecting initial positions"
	case consensus.SessionPhaseChallenge:
		return "collecting challenges"
	case consensus.SessionPhaseDebate:
		return "running debate rounds"
	case consensus.SessionPhaseFinalVote:
		return "collecting final votes"
	case consensus.SessionPhaseVerify:
		return "running verification"
	case consensus.SessionPhaseRevise:
		return "revising claims after attacks and verification"
	case consensus.SessionPhaseAdjudicate:
		return "making adjudication decision"
	case consensus.SessionPhaseDelphiQuestionnaire:
		return "collecting delphi questionnaire"
	case consensus.SessionPhaseDelphiSummary:
		return "building anonymous delphi summary"
	case consensus.SessionPhaseDelphiRevision:
		return "collecting delphi revisions"
	case consensus.SessionPhaseReport:
		return "building report"
	case consensus.SessionPhaseAction:
		return "executing action"
	case consensus.SessionPhaseObserve:
		return "recording post-action observation"
	case consensus.SessionPhaseFinished:
		return "run finished"
	case consensus.SessionPhaseFailed:
		return "run failed"
	default:
		return "unknown"
	}
}

func (o *Output) RunStarted(requestID string, mode consensus.WorkflowMode, task string, roles consensus.RoleAssignments) {
	o.Printf("[til-consensus] run started\n")
	o.Printf("  requestId: %s\n", requestID)
	o.Printf("  mode: %s\n", mode)
	o.Printf("  task: %s\n", task)
	if len(roles.Proposers) > 0 {
		o.Printf("  proposers: %s\n", strings.Join(roles.Proposers, ", "))
	}
	if len(roles.Challengers) > 0 {
		o.Printf("  challengers: %s\n", strings.Join(roles.Challengers, ", "))
	}
	if len(roles.Participants) > 0 {
		o.Printf("  participants: %s\n", strings.Join(roles.Participants, ", "))
	}
	if roles.Facilitator != "" {
		o.Printf("  facilitator: %s\n", roles.Facilitator)
	}
	if roles.Arbiter != "" {
		o.Printf("  arbiter: %s\n", roles.Arbiter)
	}
}

func (o *Output) RunCompleted(resultPath, summaryPath string) {
	o.Printf("[til-consensus] run completed\n")
	o.Printf("  result: %s\n", resultPath)
	o.Printf("  summary: %s\n", summaryPath)
}

type observerFunc func(context.Context, consensus.RunEvent) error

func (f observerFunc) OnEvent(ctx context.Context, event consensus.RunEvent) error {
	return f(ctx, event)
}
