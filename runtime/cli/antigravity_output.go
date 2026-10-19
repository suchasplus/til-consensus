package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const maxAntigravityJSONLLineBytes = 10 * 1024 * 1024

// AntigravityResultError 保留 CLI 协议终态。进程退出码不能代替此终态。
type AntigravityResultError struct {
	Status  string
	Message string
}

func (e *AntigravityResultError) Error() string {
	if e == nil {
		return "antigravity result failed"
	}
	message := "antigravity result failed: " + e.Status
	if e.Message != "" {
		message += ": " + e.Message
	}
	return message
}

func (e *AntigravityResultError) Unwrap() error {
	if e == nil {
		return nil
	}
	status := strings.ToUpper(e.Status)
	message := strings.ToLower(e.Message)
	if status == "CANCELED" || status == "CANCELLED" || status == "INTERRUPTED" ||
		strings.Contains(message, "interrupted") || strings.Contains(message, "canceled") || strings.Contains(message, "cancelled") {
		return context.Canceled
	}
	return nil
}

type antigravityCLIResult struct {
	Status           string          `json:"status"`
	Response         string          `json:"response"`
	Error            json.RawMessage `json:"error"`
	NumTurns         *int            `json:"num_turns"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	JSONSchema       json.RawMessage `json:"json_schema"`
}

type antigravityCLIEvent struct {
	Event  string                `json:"event"`
	Result *antigravityCLIResult `json:"result"`
}

// normalizeAntigravityCLIOutput 只处理独立任务的原生 JSON 或 JSONL 结果。
// 调用方须使用全新进程，禁止 --conversation/--continue/-c，发送单个 user 后关闭 stdin。
// 尤其原生 JSON 只有最后终态，无法单凭 envelope 判断是否复用了已有会话。
// 1.4.2 实测多轮 structured_output 可能仍是上一轮值，不能选最后结果蒙混过关。
// num_turns 和 user_input step 还包括内部 schema 修复，不能当成外部用户输入数。
func normalizeAntigravityCLIOutput(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("antigravity protocol error: no result envelope")
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &object); err == nil {
		if _, hasEvent := object["event"]; hasEvent {
			var event antigravityCLIEvent
			if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
				return "", fmt.Errorf("antigravity protocol error: %w", err)
			}
			if event.Event != "result" || event.Result == nil {
				return "", fmt.Errorf("antigravity protocol error: no terminal result envelope")
			}
			return antigravityResultOutput(*event.Result)
		}
		var result antigravityCLIResult
		if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
			return "", fmt.Errorf("antigravity protocol error: %w", err)
		}
		return antigravityResultOutput(result)
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), maxAntigravityJSONLLineBytes)
	var result *antigravityCLIResult
	externalUserEvents := 0
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event antigravityCLIEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return "", fmt.Errorf("antigravity jsonl protocol error at line %d: %w", lineNumber, err)
		}
		if event.Event == "" {
			return "", fmt.Errorf("antigravity jsonl protocol error at line %d: missing event type", lineNumber)
		}
		if result != nil {
			return "", fmt.Errorf("antigravity protocol error: events after terminal result; multiple turns are unsupported because structured_output may be stale")
		}
		// 若未来协议回显外部 user 消息，只允许一个。当前 step_update 中的
		// user_input 可能由 CLI 内部 schema 修复生成，不能据此拒绝独立任务。
		if event.Event == "user" {
			externalUserEvents++
		}
		if event.Event != "result" {
			continue
		}
		if event.Result == nil {
			return "", fmt.Errorf("antigravity protocol error: result event is missing result payload")
		}
		// 先处理真实失败，避免后续输出覆盖取消或其他协议错误。
		if _, err := antigravityResultOutput(*event.Result); err != nil {
			return "", err
		}
		result = event.Result
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("antigravity jsonl protocol error: %w", err)
	}
	if result == nil {
		return "", fmt.Errorf("antigravity protocol error: no terminal result envelope; partial response is incomplete")
	}
	if externalUserEvents > 1 {
		return "", fmt.Errorf("antigravity protocol error: multiple external user events; cannot safely associate structured_output with a single task")
	}
	return antigravityResultOutput(*result)
}

func antigravityResultOutput(result antigravityCLIResult) (string, error) {
	message := antigravityErrorMessage(result.Error)
	if result.Status == "" {
		return "", fmt.Errorf("antigravity protocol error: result is missing status")
	}
	if result.Status != "SUCCESS" || message != "" {
		return "", &AntigravityResultError{Status: result.Status, Message: message}
	}
	// 内部 schema 修复也会增加 num_turns；只验证有效的正计数。
	// 独立任务靠单输入、全新进程和唯一 result 保证，不靠此计数推断。
	if result.NumTurns == nil || *result.NumTurns <= 0 {
		return "", fmt.Errorf("antigravity protocol error: successful result is missing a positive num_turns count")
	}
	structured := bytes.TrimSpace(result.StructuredOutput)
	if len(structured) != 0 && !bytes.Equal(structured, []byte("null")) {
		var compact bytes.Buffer
		if err := json.Compact(&compact, structured); err != nil {
			return "", fmt.Errorf("antigravity protocol error: invalid structured_output: %w", err)
		}
		return compact.String(), nil
	}
	schema := bytes.TrimSpace(result.JSONSchema)
	if len(schema) != 0 && !bytes.Equal(schema, []byte("null")) {
		return "", fmt.Errorf("antigravity protocol error: schema result is missing structured_output")
	}
	if strings.TrimSpace(result.Response) != "" {
		return result.Response, nil
	}
	return "", fmt.Errorf("antigravity protocol error: successful result has no structured_output or response")
}

func antigravityErrorMessage(raw json.RawMessage) string {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return ""
	}
	var message string
	if err := json.Unmarshal(value, &message); err == nil {
		return strings.TrimSpace(message)
	}
	return string(value)
}
