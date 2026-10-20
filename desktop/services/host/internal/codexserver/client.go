// Package codexserver implements the local app-server protocol for isolated
// Codex tasks. Only public answer and progress events leave this package.
package codexserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/suchasplus/til-consensus/runtime/cli"
)

type Options struct {
	Command     string
	Cwd         string
	Model       string
	Reasoning   string
	Environment []string
	OnEvent     func(cli.CLIEvent) error
}
type Result struct {
	Output    string
	RawOutput string
	Model     string
	Reasoning string
	ThreadID  string
	Usage     map[string]any
	Command   []string
}
type packet struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}
type lockedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Len()+len(p) > 16*1024*1024 {
		return 0, fmt.Errorf("Codex diagnostic output exceeds 16 MiB")
	}
	return b.Buffer.Write(p)
}
func (b *lockedBuffer) text() string { b.mu.Lock(); defer b.mu.Unlock(); return b.String() }

func Run(ctx context.Context, options Options, prompt string, schema map[string]any) (result Result, resultErr error) {
	command := options.Command
	if command == "" {
		command = "codex"
	}
	args := []string{"app-server", "--listen", "stdio://"}
	result.Command = append([]string{command}, args...)
	cmd := exec.Command(command, args...)
	cmd.Dir = options.Cwd
	cmd.Env = options.Environment
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return result, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, err
	}
	var diagnostics, raw lockedBuffer
	cmd.Stderr = &diagnostics
	if err = cmd.Start(); err != nil {
		return result, fmt.Errorf("启动 Codex app-server: %w", err)
	}
	frames := make(chan packet, 64)
	scanDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			if _, err := raw.Write(append(line, '\n')); err != nil {
				scanDone <- err
				close(frames)
				return
			}
			var frame packet
			if err := json.Unmarshal(line, &frame); err != nil {
				scanDone <- fmt.Errorf("Codex 协议 JSON: %w", err)
				close(frames)
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				scanDone <- ctx.Err()
				close(frames)
				return
			}
		}
		scanDone <- scanner.Err()
		close(frames)
	}()
	processDone := make(chan error, 1)
	go func() { processDone <- cmd.Wait() }()
	var sendMu sync.Mutex
	send := func(message any) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return json.NewEncoder(stdin).Encode(message)
	}
	threadID, turnID := "", ""
	defer func() {
		if ctx.Err() != nil || resultErr != nil {
			if threadID != "" && turnID != "" {
				_ = send(map[string]any{"id": 99, "method": "turn/interrupt", "params": map[string]any{"threadId": threadID, "turnId": turnID}})
			}
		}
		_ = stdin.Close()
		select {
		case <-processDone:
		case <-time.After(1500 * time.Millisecond):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
			select {
			case <-processDone:
			case <-time.After(750 * time.Millisecond):
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				<-processDone
			}
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		result.RawOutput = raw.text()
		if resultErr != nil && strings.TrimSpace(diagnostics.text()) != "" {
			resultErr = fmt.Errorf("%w；Codex 诊断：%s", resultErr, truncate(diagnostics.text(), 1500))
		}
	}()
	emit := func(event cli.CLIEvent) error {
		event.CLIType = "codex"
		if options.OnEvent != nil {
			return options.OnEvent(event)
		}
		return nil
	}
	if err = send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]any{"name": "tcview", "title": "TCView", "version": "0.2.0"}}}); err != nil {
		return result, err
	}
	initialized, threadStarted := false, false
	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case frame, ok := <-frames:
			if !ok {
				scanErr := <-scanDone
				if scanErr != nil {
					return result, scanErr
				}
				return result, fmt.Errorf("Codex app-server 在完成当前任务前退出")
			}
			if len(frame.Error) > 0 && string(frame.Error) != "null" {
				return result, fmt.Errorf("Codex 请求失败：%s", frame.Error)
			}
			id := string(frame.ID)
			if id == "1" && !initialized {
				initialized = true
				if err = send(map[string]any{"method": "initialized"}); err != nil {
					return result, err
				}
				params := map[string]any{"cwd": options.Cwd, "ephemeral": true, "approvalPolicy": "never", "sandbox": "read-only"}
				if options.Model != "" {
					params["model"] = options.Model
				}
				if options.Reasoning != "" {
					params["config"] = map[string]any{"model_reasoning_effort": options.Reasoning}
				}
				if err = send(map[string]any{"id": 2, "method": "thread/start", "params": params}); err != nil {
					return result, err
				}
				continue
			}
			if id == "2" && !threadStarted {
				threadStarted = true
				var response struct {
					Thread struct {
						ID string `json:"id"`
					} `json:"thread"`
					Model     string `json:"model"`
					Reasoning string `json:"reasoningEffort"`
				}
				if err = json.Unmarshal(frame.Result, &response); err != nil {
					return result, err
				}
				threadID = response.Thread.ID
				result.ThreadID = threadID
				result.Model = response.Model
				result.Reasoning = response.Reasoning
				if options.Reasoning != "" && result.Reasoning != "" && options.Reasoning != result.Reasoning {
					return result, fmt.Errorf("Codex 报告的推理配置 %s 与请求 %s 不一致，未开始推理", result.Reasoning, options.Reasoning)
				}
				if threadID == "" {
					return result, fmt.Errorf("Codex 没有返回 thread ID")
				}
				if err = emit(cli.CLIEvent{Type: "started", Model: result.Model, SessionID: threadID}); err != nil {
					return result, err
				}
				params := map[string]any{"threadId": threadID, "input": []any{map[string]any{"type": "text", "text": prompt}}}
				if options.Reasoning != "" {
					params["effort"] = options.Reasoning
				}
				if len(schema) > 0 {
					params["outputSchema"] = schema
				}
				if err = send(map[string]any{"id": 3, "method": "turn/start", "params": params}); err != nil {
					return result, err
				}
				continue
			}
			var params map[string]any
			if len(frame.Params) > 0 {
				if err = json.Unmarshal(frame.Params, &params); err != nil {
					return result, err
				}
			}
			if len(frame.ID) > 0 && frame.Method != "" {
				var answer any
				switch frame.Method {
				case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
					answer = map[string]any{"decision": "decline"}
				case "item/permissions/requestApproval":
					answer = map[string]any{"permissions": map[string]any{}}
				case "item/tool/requestUserInput":
					answer = map[string]any{"answers": map[string]any{}}
				case "mcpServer/elicitation/request":
					answer = map[string]any{"action": "decline", "content": nil}
				default:
					return result, fmt.Errorf("Codex 请求了不支持的交互：%s", frame.Method)
				}
				if err = send(map[string]any{"id": frame.ID, "result": answer}); err != nil {
					return result, err
				}
				if err = emit(cli.CLIEvent{Type: "permission_denied", Status: frame.Method}); err != nil {
					return result, err
				}
				continue
			}
			switch frame.Method {
			case "model/rerouted":
				return result, fmt.Errorf("Codex 报告模型重路由，未按请求模型完成：%s → %s", stringField(params, "fromModel"), stringField(params, "toModel"))
			case "turn/started":
				turnID = stringField(object(params["turn"]), "id")
			case "item/agentMessage/delta":
				if err = emit(cli.CLIEvent{Type: "text_delta", Text: stringField(params, "delta")}); err != nil {
					return result, err
				}
			case "item/started":
				item := object(params["item"])
				kind := stringField(item, "type")
				if kind == "commandExecution" || kind == "mcpToolCall" || kind == "webSearch" {
					if err = emit(cli.CLIEvent{Type: "tool_started", Status: kind}); err != nil {
						return result, err
					}
				}
			case "item/completed":
				item := object(params["item"])
				if stringField(item, "type") == "agentMessage" {
					result.Output = stringField(item, "text")
				}
			case "thread/tokenUsage/updated":
				usage := object(params["tokenUsage"])
				result.Usage = object(usage["total"])
				if result.Usage != nil {
					if err = emit(cli.CLIEvent{Type: "usage", Usage: result.Usage}); err != nil {
						return result, err
					}
				}
			case "turn/completed":
				turn := object(params["turn"])
				status := stringField(turn, "status")
				if status != "completed" {
					if status == "interrupted" {
						return result, context.Canceled
					}
					return result, fmt.Errorf("Codex turn %s: %v", status, turn["error"])
				}
				if strings.TrimSpace(result.Output) == "" {
					return result, fmt.Errorf("Codex 已完成但未返回最终回答")
				}
				if !json.Valid([]byte(result.Output)) && len(schema) > 0 {
					return result, fmt.Errorf("Codex 最终结构化回答不是有效 JSON")
				}
				if err = emit(cli.CLIEvent{Type: "terminal", Status: "completed", Usage: result.Usage}); err != nil {
					return result, err
				}
				return result, nil
			}
		}
	}
}

func object(value any) map[string]any { result, _ := value.(map[string]any); return result }
func stringField(value map[string]any, key string) string {
	result, _ := value[key].(string)
	return result
}
func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) > max {
		return string(runes[:max]) + "…"
	}
	return value
}

var _ io.Writer = (*lockedBuffer)(nil)
