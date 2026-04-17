package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

type Output struct {
	stdout       io.Writer
	stderr       io.Writer
	verbose      bool
	debug        bool
	color        bool
	artifactsDir string

	mu             sync.Mutex
	currentPhase   consensus.SessionPhase
	phaseStartedAt time.Time
	phaseStats     phaseStats
	taskStarts     map[string]time.Time
}

type phaseStats struct {
	tasksDispatched int
	tasksCompleted  int
	tasksFailed     int
	tasksRetried    int

	verificationsPassed       int
	verificationsFailed       int
	verificationsInconclusive int

	claimsRevised     int
	claimsAdjudicated int
	observationsAdded int

	revisionActions         map[string]int
	adjudicationDisposition map[string]int
}

func NewOutput(stdout, stderr io.Writer, verbose bool, debug bool, artifactsDir string) *Output {
	colorEnabled := shouldEnableColor(stdout)
	return &Output{
		stdout:       stdout,
		stderr:       stderr,
		verbose:      verbose,
		debug:        debug,
		color:        colorEnabled,
		artifactsDir: artifactsDir,
		taskStarts:   make(map[string]time.Time),
	}
}

func (o *Output) Printf(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	if o.color {
		text = colorizeRunOutput(text)
	}
	_, _ = io.WriteString(o.stdout, text)
}

func (o *Output) Errorf(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	if o.color {
		text = colorizeRunOutput(text)
	}
	_, _ = io.WriteString(o.stderr, text)
}

func (o *Output) EventObserver() consensus.Observer {
	return observerFunc(func(_ context.Context, event consensus.RunEvent) error {
		eventAt := parseEventTime(event.At)

		o.mu.Lock()
		defer o.mu.Unlock()

		switch event.Type {
		case consensus.RunEventPhaseChanged:
			if o.verbose {
				o.flushPhaseSummaryLocked(eventAt)
			}
			o.currentPhase = event.Phase
			o.phaseStartedAt = eventAt
			o.phaseStats = newPhaseStats()
			o.Printf("[til-consensus] %s\n", formatPhaseChanged(event.Phase))
			o.printDebugLocked(event)
			return nil
		case consensus.RunEventTaskDispatched:
			o.recordTaskDispatchedLocked(event.Payload, eventAt)
			o.Printf("[til-consensus] %s\n", formatTaskDispatched(event.Payload))
			o.printDebugLocked(event)
			return nil
		case consensus.RunEventTaskRetrying:
			o.recordTaskRetryingLocked()
			o.Printf("[til-consensus] %s\n", formatTaskRetrying(event.Payload))
			o.printDebugLocked(event)
			return nil
		case consensus.RunEventTaskFailed:
			payload := o.withTaskDurationLocked(event.Payload, eventAt, true)
			o.recordTaskFailedLocked()
			o.Printf("[til-consensus] %s\n", formatTaskFailed(payload))
			event.Payload = payload
			o.printDebugLocked(event)
			return nil
		case consensus.RunEventTaskCompleted:
			payload := o.withTaskDurationLocked(event.Payload, eventAt, true)
			o.recordTaskCompletedLocked()
			if o.verbose {
				o.Printf("[til-consensus] %s\n", formatTaskCompleted(payload))
			}
			event.Payload = payload
			o.printDebugLocked(event)
			return nil
		case consensus.RunEventLedgerAppended:
			o.recordLedgerAppendedLocked(event.Payload)
			if !o.verbose {
				o.printDebugLocked(event)
				return nil
			}
			if text := compactPayload(event.Payload); text != "" {
				o.Printf("[til-consensus] %s %s\n", event.Type, text)
			} else {
				o.Printf("[til-consensus] %s\n", event.Type)
			}
			o.printDebugLocked(event)
			return nil
		case consensus.RunEventClaimRevised:
			o.recordClaimRevisedLocked(event.Payload)
			if o.verbose {
				o.Printf("[til-consensus] %s\n", formatClaimRevised(event.Payload))
			}
			o.printDebugLocked(event)
			return nil
		case consensus.RunEventClaimAdjudicated:
			o.recordClaimAdjudicatedLocked(event.Payload)
			if o.verbose {
				o.Printf("[til-consensus] %s\n", formatClaimAdjudicated(event.Payload))
			}
			o.printDebugLocked(event)
			return nil
		case consensus.RunEventObservationAdded:
			o.recordObservationAddedLocked()
			if o.verbose {
				o.Printf("[til-consensus] %s\n", formatObservationAdded(event.Payload))
			}
			o.printDebugLocked(event)
			return nil
		case consensus.RunEventSessionFinalized, consensus.RunEventSessionFailed:
			if o.verbose {
				o.flushPhaseSummaryLocked(eventAt)
			}
			if !o.verbose {
				o.printDebugLocked(event)
				return nil
			}
			if text := compactPayload(event.Payload); text != "" {
				o.Printf("[til-consensus] %s %s\n", event.Type, text)
			} else {
				o.Printf("[til-consensus] %s\n", event.Type)
			}
			o.printDebugLocked(event)
			return nil
		}

		if !o.verbose {
			o.printDebugLocked(event)
			return nil
		}
		if text := compactPayload(event.Payload); text != "" {
			o.Printf("[til-consensus] %s %s\n", event.Type, text)
		} else {
			o.Printf("[til-consensus] %s\n", event.Type)
		}
		o.printDebugLocked(event)
		return nil
	})
}

func (o *Output) recordTaskDispatchedLocked(payload map[string]any, at time.Time) {
	o.phaseStats.tasksDispatched++
	if !at.IsZero() {
		o.taskStarts[taskAttemptKey(payload)] = at
	}
}

func (o *Output) recordTaskRetryingLocked() {
	o.phaseStats.tasksRetried++
}

func (o *Output) recordTaskFailedLocked() {
	o.phaseStats.tasksFailed++
}

func (o *Output) recordTaskCompletedLocked() {
	o.phaseStats.tasksCompleted++
}

func (o *Output) recordLedgerAppendedLocked(payload map[string]any) {
	kind := stringValue(payload, "kind")
	status := stringValue(payload, "status")
	switch kind {
	case string(consensus.EvidenceKindDeterministicCheck), string(consensus.EvidenceKindSemanticVerification):
		switch status {
		case string(consensus.VerificationStatusPassed):
			o.phaseStats.verificationsPassed++
		case string(consensus.VerificationStatusFailed):
			o.phaseStats.verificationsFailed++
		case string(consensus.VerificationStatusInconclusive):
			o.phaseStats.verificationsInconclusive++
		}
	}
}

func (o *Output) recordClaimRevisedLocked(payload map[string]any) {
	o.phaseStats.claimsRevised++
	action := stringValue(payload, "action")
	if action == "" {
		return
	}
	if o.phaseStats.revisionActions == nil {
		o.phaseStats.revisionActions = make(map[string]int)
	}
	o.phaseStats.revisionActions[action]++
}

func (o *Output) recordClaimAdjudicatedLocked(payload map[string]any) {
	o.phaseStats.claimsAdjudicated++
	disposition := stringValue(payload, "disposition")
	if disposition == "" {
		return
	}
	if o.phaseStats.adjudicationDisposition == nil {
		o.phaseStats.adjudicationDisposition = make(map[string]int)
	}
	o.phaseStats.adjudicationDisposition[disposition]++
}

func (o *Output) recordObservationAddedLocked() {
	o.phaseStats.observationsAdded++
}

func (o *Output) withTaskDurationLocked(payload map[string]any, at time.Time, deleteStart bool) map[string]any {
	if len(payload) == 0 {
		return payload
	}
	startedAt, ok := o.taskStarts[taskAttemptKey(payload)]
	if !ok || startedAt.IsZero() || at.IsZero() {
		return payload
	}
	if deleteStart {
		delete(o.taskStarts, taskAttemptKey(payload))
	}
	cloned := clonePayload(payload)
	cloned["duration"] = roundDuration(at.Sub(startedAt))
	return cloned
}

func (o *Output) flushPhaseSummaryLocked(now time.Time) {
	if o.currentPhase == "" || o.phaseStartedAt.IsZero() {
		return
	}
	duration := ""
	if !now.IsZero() {
		duration = roundDuration(now.Sub(o.phaseStartedAt))
	}
	summary := formatPhaseSummary(o.currentPhase, duration, o.phaseStats)
	if summary != "" {
		o.Printf("[til-consensus] %s\n", summary)
	}
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
		return withDurationAndAttemptSuffix(payload, "task completed")
	}
	if agentID == "" {
		return withDurationAndAttemptSuffix(payload, fmt.Sprintf("task completed: %s", taskKind))
	}
	if taskKind == "" {
		return withDurationAndAttemptSuffix(payload, fmt.Sprintf("task completed: %s", agentID))
	}
	return withDurationAndAttemptSuffix(payload, fmt.Sprintf("task completed: %s -> %s", agentID, taskKind))
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
		return withDurationAndAttemptSuffix(payload, prefix)
	}
	return withDurationAndAttemptSuffix(payload, prefix) + " error=" + errText
}

func formatClaimRevised(payload map[string]any) string {
	claimID := stringValue(payload, "claimId")
	action := stringValue(payload, "action")
	reason := stringValue(payload, "reason")
	delta := stringValue(payload, "confidenceDelta")
	text := fmt.Sprintf("claim revised: %s action=%s", claimID, action)
	if delta != "" && delta != "0" && delta != "0.0" {
		text += " confidenceDelta=" + delta
	}
	if reason != "" {
		text += " reason=" + reason
	}
	return text
}

func formatClaimAdjudicated(payload map[string]any) string {
	claimID := stringValue(payload, "claimId")
	disposition := stringValue(payload, "disposition")
	verdict := stringValue(payload, "verdict")
	confidence := stringValue(payload, "finalConfidence")
	if confidence == "" {
		confidence = stringValue(payload, "confidence")
	}
	text := fmt.Sprintf("claim adjudicated: %s disposition=%s", claimID, disposition)
	if verdict != "" {
		text += " verdict=" + verdict
	}
	if confidence != "" {
		text += " confidence=" + confidence
	}
	reason := stringValue(payload, "reason")
	if reason != "" {
		text += " reason=" + reason
	}
	return text
}

func formatObservationAdded(payload map[string]any) string {
	outcome := stringValue(payload, "outcome")
	summary := stringValue(payload, "summary")
	text := fmt.Sprintf("observation recorded: %s", outcome)
	if boolValue(payload, "reopen") {
		text += " reopen=true"
	}
	if followUpCaseID := stringValue(payload, "followUpCaseId"); followUpCaseID != "" {
		text += " followUpCaseId=" + followUpCaseID
	}
	if summary != "" {
		text += " summary=" + summary
	}
	return text
}

func formatPhaseSummary(phase consensus.SessionPhase, duration string, stats phaseStats) string {
	parts := make([]string, 0, 8)
	if duration != "" {
		parts = append(parts, "duration="+duration)
	}
	if stats.tasksDispatched > 0 || stats.tasksCompleted > 0 || stats.tasksFailed > 0 || stats.tasksRetried > 0 {
		parts = append(parts, fmt.Sprintf("tasks(d=%d c=%d f=%d r=%d)", stats.tasksDispatched, stats.tasksCompleted, stats.tasksFailed, stats.tasksRetried))
	}
	if stats.verificationsPassed > 0 || stats.verificationsFailed > 0 || stats.verificationsInconclusive > 0 {
		parts = append(parts, fmt.Sprintf("verifications(pass=%d fail=%d inconclusive=%d)", stats.verificationsPassed, stats.verificationsFailed, stats.verificationsInconclusive))
	}
	if stats.claimsRevised > 0 {
		parts = append(parts, fmt.Sprintf("revisions=%d", stats.claimsRevised))
		if text := joinCounterMap(stats.revisionActions); text != "" {
			parts = append(parts, "actions="+text)
		}
	}
	if stats.claimsAdjudicated > 0 {
		parts = append(parts, fmt.Sprintf("adjudications=%d", stats.claimsAdjudicated))
		if text := joinCounterMap(stats.adjudicationDisposition); text != "" {
			parts = append(parts, "dispositions="+text)
		}
	}
	if stats.observationsAdded > 0 {
		parts = append(parts, fmt.Sprintf("observations=%d", stats.observationsAdded))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("phase completed: %s %s", phase, strings.Join(parts, " "))
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

func boolValue(payload map[string]any, key string) bool {
	if len(payload) == 0 {
		return false
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return false
	}
	boolean, ok := value.(bool)
	return ok && boolean
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
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, payload[key]))
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

func withDurationAndAttemptSuffix(payload map[string]any, text string) string {
	text = withAttemptSuffix(payload, text)
	duration := stringValue(payload, "duration")
	if duration == "" {
		return text
	}
	return text + " duration=" + duration
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

func parseEventTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func taskAttemptKey(payload map[string]any) string {
	return strings.Join([]string{
		stringValue(payload, "agentId"),
		stringValue(payload, "taskKind"),
		stringValue(payload, "attempt"),
	}, "|")
}

func clonePayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return payload
	}
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = value
	}
	return cloned
}

func roundDuration(duration time.Duration) string {
	if duration <= 0 {
		return "0s"
	}
	if duration < time.Millisecond {
		return duration.String()
	}
	return duration.Round(time.Millisecond).String()
}

func joinCounterMap(values map[string]int) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, values[key]))
	}
	return strings.Join(parts, ",")
}

func newPhaseStats() phaseStats {
	return phaseStats{
		revisionActions:         make(map[string]int),
		adjudicationDisposition: make(map[string]int),
	}
}

func (o *Output) printDebugLocked(event consensus.RunEvent) {
	if !o.debug {
		return
	}
	if payload := prettyPayload(event.Payload); payload != "" {
		prefix := fmt.Sprintf("[til-consensus][debug] %s payload=", event.Type)
		if o.color {
			prefix = colorizeRunOutput(prefix)
			payload = colorizeDebugJSON(payload)
		}
		_, _ = io.WriteString(o.stdout, prefix+payload+"\n")
	}
	if text := debugArtifactText(o.artifactsDir, event.Type, event.Payload); text != "" {
		o.Printf("[til-consensus][debug] %s\n", text)
	}
}

func prettyPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return compactPayload(payload)
	}
	return string(body)
}

func debugArtifactText(artifactsDir string, eventType consensus.RunEventType, payload map[string]any) string {
	if strings.TrimSpace(artifactsDir) == "" {
		return ""
	}
	agentID := stringValue(payload, "agentId")
	taskKind := stringValue(payload, "taskKind")
	if agentID == "" || taskKind == "" {
		return ""
	}
	safeAgent := sanitizeFilename(agentID)
	inputPath := filepath.Join(artifactsDir, fmt.Sprintf("input-%s-%s-<taskID>.json", safeAgent, taskKind))
	rawPath := filepath.Join(artifactsDir, fmt.Sprintf("raw-%s-%s-<taskID>.json", safeAgent, taskKind))
	failurePath := filepath.Join(artifactsDir, fmt.Sprintf("failure-%s-%s-<taskID>.json", safeAgent, taskKind))
	parseErrPath := filepath.Join(artifactsDir, fmt.Sprintf("raw-error-%s-%s-<taskID>.txt", safeAgent, taskKind))
	switch eventType {
	case consensus.RunEventTaskDispatched:
		return "provider artifacts input=" + inputPath
	case consensus.RunEventTaskCompleted:
		return "provider artifacts input=" + inputPath + " raw=" + rawPath
	case consensus.RunEventTaskFailed:
		return "provider artifacts input=" + inputPath + " failure=" + failurePath + " parseError=" + parseErrPath
	default:
		return ""
	}
}

func sanitizeFilename(value string) string {
	replacer := strings.NewReplacer("/", "_", " ", "_", ":", "_")
	return replacer.Replace(value)
}
