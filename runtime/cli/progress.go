package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// RunnerOptions are local execution hooks, not model parameters. OnEvent is
// synchronous for backpressure. Its error cancels the child process.
type RunnerOptions struct{ OnEvent func(CLIEvent) error }

// CLIEvent contains only public answer/progress fields. Thinking and reasoning
// events are never forwarded. Usage is a provider snapshot, not a token delta.
type CLIEvent struct {
	CLIType   string         `json:"cliType"`
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Status    string         `json:"status,omitempty"`
	Model     string         `json:"model,omitempty"`
	SessionID string         `json:"sessionId,omitempty"`
	Usage     map[string]any `json:"usage,omitempty"`
}

type progressWriter struct {
	cliType  string
	callback func(CLIEvent) error
	cancel   context.CancelFunc
	pending  []byte
	err      error
	sawDelta bool
	document bool
}

func (w *progressWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.pending = append(w.pending, p...)
	if len(w.pending) > 10*1024*1024 {
		w.fail(fmt.Errorf("CLI progress line exceeds 10 MiB"))
		return 0, w.err
	}
	if w.document {
		return len(p), nil
	}
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		line := append([]byte(nil), w.pending[:index]...)
		w.pending = w.pending[index+1:]
		if err := w.consume(line); err != nil {
			w.fail(err)
			return len(p), err
		}
	}
	return len(p), nil
}
func (w *progressWriter) fail(err error) { w.err = err; w.cancel() }
func (w *progressWriter) flush() error {
	if w.err != nil {
		return w.err
	}
	if len(bytes.TrimSpace(w.pending)) > 0 {
		if err := w.consume(w.pending); err != nil {
			return err
		}
	}
	w.pending = nil
	return nil
}
func (w *progressWriter) consume(line []byte) error {
	if len(bytes.TrimSpace(line)) == 0 {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return fmt.Errorf("%s progress JSONL: %w", w.cliType, err)
	}
	str := func(value any) string { result, _ := value.(string); return result }
	object := func(value any) map[string]any { result, _ := value.(map[string]any); return result }
	e := CLIEvent{CLIType: w.cliType}
	switch w.cliType {
	case "codex":
		switch str(raw["type"]) {
		case "item.completed":
			item := object(raw["item"])
			if str(item["type"]) == "agent_message" {
				e.Type = "message"
				e.Text = str(item["text"])
			}
		case "turn.completed":
			e.Type = "terminal"
			e.Status = "completed"
			e.Usage = object(raw["usage"])
		case "turn.failed", "error":
			e.Type = "terminal"
			e.Status = "failed"
		}
	case "claude":
		switch str(raw["type"]) {
		case "system":
			if str(raw["subtype"]) == "init" {
				e.Type = "started"
				e.Model = str(raw["model"])
				e.SessionID = str(raw["session_id"])
			}
		case "stream_event":
			event := object(raw["event"])
			delta := object(event["delta"])
			if str(delta["type"]) == "text_delta" {
				e.Type = "text_delta"
				e.Text = str(delta["text"])
				w.sawDelta = true
			}
		case "result":
			e.Type = "terminal"
			e.Status = str(raw["subtype"])
			e.Usage = object(raw["usage"])
			e.SessionID = str(raw["session_id"])
			if !w.sawDelta {
				e.Text = str(raw["result"])
			}
		}
	case "grok":
		switch str(raw["type"]) {
		case "text":
			e.Type = "text_delta"
			e.Text = str(raw["data"])
		case "usage":
			e.Type = "usage"
			e.Usage = object(raw["usage"])
			if e.Usage == nil {
				e.Usage = object(raw["data"])
			}
		case "end":
			e.Type = "terminal"
			e.Status = str(raw["stopReason"])
		case "error":
			e.Type = "terminal"
			e.Status = "failed"
		default:
			if raw["stopReason"] != nil {
				e.Type = "terminal"
				e.Status = str(raw["stopReason"])
				e.Text = str(raw["text"])
				e.Usage = object(raw["usage"])
			}
		}
	case "antigravity":
		switch str(raw["event"]) {
		case "init":
			init := object(raw["init"])
			e.Type = "started"
			e.Model = str(init["model"])
		case "step_update":
			step := object(raw["step_update"])
			if str(step["step_type"]) == "agent_response" && str(step["text_delta"]) != "" {
				e.Type = "text_delta"
				e.Text = str(step["text_delta"])
			}
		case "result":
			result := object(raw["result"])
			e.Type = "terminal"
			e.Status = str(result["status"])
			e.Usage = object(result["usage"])
			e.SessionID = str(result["conversation_id"])
		}
	}
	if e.Type != "" {
		return w.callback(e)
	}
	return nil
}
