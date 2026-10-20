package codexserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// ModelInfo 只保留 model/list 的公开目录与能力，目录不代表调用权限已验证。
type ModelInfo struct {
	ID               string
	Label            string
	ReasoningValues  []string
	DefaultReasoning string
}

// ListModels 不创建线程或发起 turn，不写全局配置或登录状态。
func ListModels(ctx context.Context, options Options) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	command := options.Command
	if command == "" {
		command = "codex"
	}
	cmd := exec.CommandContext(ctx, command, "app-server", "--listen", "stdio://")
	cmd.Dir, cmd.Env = options.Cwd, options.Environment
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	var diagnostics lockedBuffer
	cmd.Stderr = &diagnostics
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("启动 Codex 模型目录：%w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(750 * time.Millisecond):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}()
	send := func(message any) error { return json.NewEncoder(stdin).Encode(message) }
	if err = send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]any{"name": "tcview", "title": "TCView", "version": "0.5.0"}}}); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	initialized := false
	id := 10
	models := []ModelInfo{}
	seenModels, seenCursors := map[string]bool{}, map[string]bool{}
	list := func(cursor string) error {
		params := map[string]any{"limit": 100, "includeHidden": false}
		if cursor != "" {
			params["cursor"] = cursor
		}
		return send(map[string]any{"id": id, "method": "model/list", "params": params})
	}
	for scanner.Scan() {
		var frame packet
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			return nil, fmt.Errorf("Codex 模型目录协议 JSON：%w", err)
		}
		if len(frame.Error) > 0 && string(frame.Error) != "null" {
			return nil, fmt.Errorf("Codex 模型目录请求失败：%s", frame.Error)
		}
		if string(frame.ID) == "1" && !initialized {
			initialized = true
			if err = send(map[string]any{"method": "initialized"}); err != nil {
				return nil, err
			}
			if err = list(""); err != nil {
				return nil, err
			}
			continue
		}
		if string(frame.ID) != fmt.Sprint(id) {
			continue
		}
		var response struct {
			Data []struct {
				ID               string `json:"id"`
				Model            string `json:"model"`
				DisplayName      string `json:"displayName"`
				Hidden           bool   `json:"hidden"`
				DefaultReasoning string `json:"defaultReasoningEffort"`
				Supported        []struct {
					Value string `json:"reasoningEffort"`
				} `json:"supportedReasoningEfforts"`
			} `json:"data"`
			NextCursor string `json:"nextCursor"`
		}
		if err = json.Unmarshal(frame.Result, &response); err != nil {
			return nil, fmt.Errorf("Codex 模型目录响应：%w", err)
		}
		for _, entry := range response.Data {
			selector := entry.Model
			if selector == "" {
				selector = entry.ID
			}
			if entry.Hidden || strings.TrimSpace(selector) == "" || seenModels[selector] {
				continue
			}
			seenModels[selector] = true
			model := ModelInfo{ID: selector, Label: entry.DisplayName, DefaultReasoning: entry.DefaultReasoning, ReasoningValues: []string{}}
			seenEfforts := map[string]bool{}
			for _, effort := range entry.Supported {
				if effort.Value != "" && !seenEfforts[effort.Value] {
					model.ReasoningValues = append(model.ReasoningValues, effort.Value)
					seenEfforts[effort.Value] = true
				}
			}
			models = append(models, model)
		}
		if len(models) > 4096 {
			return nil, fmt.Errorf("Codex 模型目录超过大小限制")
		}
		if response.NextCursor == "" {
			if len(models) == 0 {
				return nil, fmt.Errorf("Codex 模型目录为空，可手工填写后检查")
			}
			return models, nil
		}
		if seenCursors[response.NextCursor] || len(seenCursors) >= 32 {
			return nil, fmt.Errorf("Codex 模型目录分页重复或过多")
		}
		seenCursors[response.NextCursor] = true
		id++
		if err = list(response.NextCursor); err != nil {
			return nil, err
		}
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("Codex 模型目录查询取消或超时：%w", ctx.Err())
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("Codex app-server 在返回模型目录前退出")
}
