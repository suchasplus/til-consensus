package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/codexserver"
	clirunner "github.com/suchasplus/til-consensus/runtime/cli"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type ModelOption struct {
	ID              string   `json:"id"`
	ResolvedModel   string   `json:"resolvedModel,omitempty"`
	Label           string   `json:"label,omitempty"`
	Family          string   `json:"family,omitempty"`
	Effort          string   `json:"effort,omitempty"`
	ReasoningMode   string   `json:"reasoningMode"`
	ReasoningValues []string `json:"reasoningValues"`
	Verified        bool     `json:"verified"`
}
type Catalog struct {
	ID            string        `json:"id"`
	CLIVersion    string        `json:"cliVersion,omitempty"`
	Models        []ModelOption `json:"models"`
	Source        string        `json:"source"`
	LoadedAt      string        `json:"loadedAt"`
	Error         string        `json:"error,omitempty"`
	ManualAllowed bool          `json:"manualAllowed"`
}

// DiscoverModels 只查询公开目录，不派发推理。目录不证明当前账号调用权限。
func DiscoverModels(ctx context.Context, c Config) (Catalog, error) {
	result := Catalog{ID: c.ID, Models: []ModelOption{}, Source: "cli-directory", LoadedAt: time.Now().UTC().Format(time.RFC3339Nano), ManualAllowed: true}
	info := Probe(ctx, c)
	result.CLIVersion = info.Version
	fail := func(err error) (Catalog, error) {
		result.Error = RedactSensitive(err.Error())
		return result, fmt.Errorf("%s", result.Error)
	}
	if !info.Available {
		return fail(fmt.Errorf("%s", info.Error))
	}
	timeout := 45 * time.Second
	if c.ID == "agy" {
		timeout = 75 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	task := consensus.ActionTask{TaskMeta: consensus.TaskMeta{AgentID: c.ID, Role: "model-directory"}}
	environment := clirunner.BuildEnvironment(clirunner.SnapshotHostEnvironment(os.Environ()), Runtime(c, info, ""), task, c.ID, task.Role, c.Model).Values
	if c.ID == "codex" {
		models, err := codexserver.ListModels(ctx, codexserver.Options{Command: info.Path, Environment: environment})
		if err != nil {
			return fail(err)
		}
		for _, entry := range models {
			mode := "unsupported"
			if len(entry.ReasoningValues) > 0 {
				mode = "argument"
			}
			result.Models = append(result.Models, ModelOption{ID: entry.ID, Label: entry.Label, ReasoningMode: mode, ReasoningValues: entry.ReasoningValues, Verified: true})
		}
		return result, nil
	}

	args := []string{"models"}
	if c.ID == "claude" {
		args = []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--safe-mode"}
	}
	cmd := exec.CommandContext(ctx, info.Path, args...)
	cmd.Env = environment
	cmd.WaitDelay = time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	if c.ID == "claude" {
		cmd.Stdin = strings.NewReader(`{"type":"control_request","request_id":"tcview-model-directory","request":{"subtype":"initialize"}}` + "\n")
	}
	var stdout directoryBuffer
	var stderr directoryErrorBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := stdout.Bytes()
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if ctx.Err() != nil {
		return fail(fmt.Errorf("%s 模型目录查询超时或取消：%w", c.ID, ctx.Err()))
	}
	if err != nil {
		reason := strings.TrimSpace(RedactSensitive(stderr.String()))
		runes := []rune(reason)
		if len(runes) > 2000 {
			reason = string(runes[:2000]) + "…"
		}
		if reason != "" {
			return fail(fmt.Errorf("%s 模型目录查询失败：%w；%s", c.ID, err, reason))
		}
		return fail(fmt.Errorf("%s 模型目录查询失败：%w", c.ID, err))
	}
	if c.ID == "claude" {
		result.Models = ParseClaudeModels(string(output))
	} else {
		result.Models = ParseModels(c.ID, string(output))
	}
	if len(result.Models) == 0 {
		return fail(fmt.Errorf("%s 模型目录未返回可识别的模型 ID，可手工填写后检查", c.ID))
	}
	return result, nil
}
func ParseModels(id, output string) []ModelOption {
	out := []ModelOption{}
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		var selector, label string
		switch id {
		case "grok":
			if !strings.HasPrefix(line, "* ") && !strings.HasPrefix(line, "- ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			selector = fields[1]
			if !strings.HasPrefix(selector, "grok-") {
				continue
			}
		case "agy":
			selector, label, _ = strings.Cut(line, "\t")
			if !selectorPattern.MatchString(selector) || label == "" {
				continue
			}
		default:
			continue
		}
		if !selectorPattern.MatchString(selector) || seen[selector] {
			continue
		}
		seen[selector] = true
		entry := ModelOption{ID: selector, Label: strings.TrimSpace(label), ReasoningMode: "unsupported", ReasoningValues: []string{}, Verified: true}
		if id == "agy" {
			entry.ReasoningMode = "encoded"
			for _, effort := range []string{"low", "medium", "high", "xhigh", "max", "minimal"} {
				if strings.HasSuffix(selector, "-"+effort) {
					entry.Family = strings.TrimSuffix(selector, "-"+effort)
					entry.Effort = effort
					break
				}
			}
		} else if values := KnownEfforts(id, selector); len(values) > 0 {
			entry.ReasoningMode = "argument"
			entry.ReasoningValues = values
		}
		out = append(out, entry)
	}
	return out
}

// ParseClaudeModels ignores all initialization/account metadata except this whitelist.
func ParseClaudeModels(raw string) []ModelOption {
	out := []ModelOption{}
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		var root map[string]json.RawMessage
		if json.Unmarshal([]byte(line), &root) != nil {
			continue
		}
		var kind string
		_ = json.Unmarshal(root["type"], &kind)
		if kind != "control_response" {
			continue
		}
		var response struct {
			Response struct {
				Models []struct {
					Value                 string   `json:"value"`
					ResolvedModel         string   `json:"resolvedModel"`
					DisplayName           string   `json:"displayName"`
					SupportsEffort        bool     `json:"supportsEffort"`
					SupportedEffortLevels []string `json:"supportedEffortLevels"`
				} `json:"models"`
			} `json:"response"`
		}
		if json.Unmarshal(root["response"], &response) != nil {
			continue
		}
		for _, entry := range response.Response.Models {
			if !selectorPattern.MatchString(entry.Value) || seen[entry.Value] {
				continue
			}
			seen[entry.Value] = true
			levels := []string{}
			if entry.SupportsEffort {
				for _, level := range entry.SupportedEffortLevels {
					for _, known := range []string{"low", "medium", "high", "xhigh", "max"} {
						if known == level {
							levels = append(levels, level)
							break
						}
					}
				}
			}
			mode := "unsupported"
			if len(levels) > 0 {
				mode = "argument"
			}
			out = append(out, ModelOption{ID: entry.Value, ResolvedModel: entry.ResolvedModel, Label: entry.DisplayName, ReasoningMode: mode, ReasoningValues: levels, Verified: true})
		}
	}
	return out
}

type directoryBuffer struct{ bytes.Buffer }

func (b *directoryBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4*1024*1024 {
		return 0, fmt.Errorf("模型目录输出超过 4 MiB")
	}
	return b.Buffer.Write(p)
}

// Capture more than the public error limit so credential replacement happens
// before truncation; excess diagnostics are discarded without blocking the CLI.
type directoryErrorBuffer struct{ bytes.Buffer }

func (b *directoryErrorBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 16*1024 - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

// CatalogForVersion keeps historical choices visible while excluding obsolete
// dynamic capability data from validation. It never mutates the stored cache.
func CatalogForVersion(catalog *Catalog, current string) *Catalog {
	if catalog == nil || current == "" {
		return catalog
	}
	copy := *catalog
	if copy.CLIVersion == "" {
		copy.Source = "cli-directory-stale"
		copy.Error = "旧模型目录未记录 CLI 版本，请重新加载目录确认当前能力。"
	} else if copy.CLIVersion != current {
		copy.Source = "cli-directory-stale"
		copy.Error = fmt.Sprintf("模型目录来自 CLI %s，当前 CLI 为 %s；请重新加载目录。", copy.CLIVersion, current)
	}
	return &copy
}
