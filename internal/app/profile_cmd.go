package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/artifact"
	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	"github.com/suchasplus/til-consensus/internal/preflight"
	"github.com/suchasplus/til-consensus/internal/telemetry"
	"github.com/suchasplus/til-consensus/internal/viewer"
	"github.com/urfave/cli/v3"
)

func newProfileCommand() *cli.Command {
	return &cli.Command{
		Name:  "profile",
		Usage: "检查 provider / agent profile 可用性",
		Commands: []*cli.Command{
			newProfilePreflightCommand(),
		},
	}
}

func newProfilePreflightCommand() *cli.Command {
	return &cli.Command{
		Name:  "preflight",
		Usage: "真实调用 provider / agent，检查最小 JSON 非交互输出",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.BoolFlag{Name: "all", Usage: "检查所有 provider；不传 --provider/--agent 时默认等价于 --all"},
			&cli.StringSliceFlag{Name: "provider", Usage: "要检查的 provider id，可重复或逗号分隔"},
			&cli.StringSliceFlag{Name: "agent", Usage: "要检查的 agent id，可重复或逗号分隔"},
			&cli.DurationFlag{Name: "timeout", Usage: "单个 provider 探测超时", Value: 90 * time.Second},
			&cli.BoolFlag{Name: "verbose", Usage: "输出 command、stdout/stderr preview 和 artifact 路径"},
			&cli.BoolFlag{Name: "web", Usage: "完成 preflight 后启动本地只读 Web viewer"},
			&cli.StringFlag{Name: "host", Usage: "Web viewer 监听地址", Value: "127.0.0.1"},
			&cli.IntFlag{Name: "port", Usage: "Web viewer 监听端口；0 表示自动分配", Value: 0},
			&cli.BoolFlag{Name: "open", Usage: "显式打开默认浏览器"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runProfilePreflightCommand(ctx, cmd)
		},
	}
}

func runProfilePreflightCommand(ctx context.Context, cmd *cli.Command) error {
	configPath, err := config.ResolveConfigPath(cmd.String("config"))
	if err != nil {
		return err
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		return err
	}
	requestID := artifact.NewRequestID(time.Now().UTC())
	paths := config.ResolveRunArtifacts(loaded, requestID)
	if err := os.MkdirAll(paths.ArtifactsDir, 0o755); err != nil {
		return fmt.Errorf("create preflight artifacts dir: %w", err)
	}
	if err := os.MkdirAll(paths.RunDir, 0o755); err != nil {
		return fmt.Errorf("create preflight run dir: %w", err)
	}

	startedAt := time.Now()
	sink := preflightArtifactSink{dir: paths.ArtifactsDir}
	entries, err := preflight.Run(ctx, loaded.Config, preflight.Options{
		ProviderIDs: cmd.StringSlice("provider"),
		AgentIDs:    cmd.StringSlice("agent"),
		All:         cmd.Bool("all"),
		Timeout:     cmd.Duration("timeout"),
	}, sink)
	if err != nil {
		return err
	}
	if err := telemetry.WriteProviderReadinessFile(filepath.Join(paths.ArtifactsDir, "provider-readiness.json"), entries, time.Now().UTC()); err != nil {
		return fmt.Errorf("write provider readiness: %w", err)
	}
	result := buildPreflightRunResult(requestID, entries, time.Since(startedAt))
	if err := writeJSONFile(paths.ResultPath, result); err != nil {
		return fmt.Errorf("write preflight result: %w", err)
	}
	if err := writeTextFile(paths.LedgerPath, ""); err != nil {
		return fmt.Errorf("write preflight ledger: %w", err)
	}
	if err := writeTextFile(paths.EventsPath, ""); err != nil {
		return fmt.Errorf("write preflight events: %w", err)
	}
	if err := writeTextFile(paths.SummaryPath, buildPreflightSummary(result, entries)); err != nil {
		return fmt.Errorf("write preflight summary: %w", err)
	}

	printPreflightSummary(cmd.Writer, entries, paths, cmd.Bool("verbose"))
	if cmd.Bool("web") {
		bundle, err := viewer.LoadBundle(viewer.InferRunFiles(paths.ResultPath))
		if err != nil {
			return err
		}
		server, err := viewer.NewWebServer(bundle, viewer.WebOptions{
			Host: cmd.String("host"),
			Port: cmd.Int("port"),
			RenderOptions: viewer.RenderOptions{
				Format:  viewer.FormatText,
				Verbose: cmd.Bool("verbose"),
			},
		})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.Writer, "web viewer started: %s\n", server.URL())
		_, _ = fmt.Fprintf(cmd.Writer, "requestId: %s | mode: %s\n", result.RequestID, result.Mode)
		_, _ = fmt.Fprintln(cmd.Writer, "按 Ctrl+C 退出")
		if cmd.Bool("open") {
			if err := viewer.OpenBrowser(server.URL()); err != nil {
				_ = server.Close()
				return err
			}
		}
		return server.Serve(ctx)
	}
	return nil
}

type preflightArtifactSink struct {
	dir string
}

func (s preflightArtifactSink) WriteInput(candidateID string, payload any) string {
	path := filepath.Join(s.dir, "preflight-input-"+sanitizeFilename(candidateID)+".json")
	_ = writeJSONFile(path, payload)
	return path
}

func (s preflightArtifactSink) WriteRaw(candidateID string, raw string) string {
	path := filepath.Join(s.dir, "preflight-raw-"+sanitizeFilename(candidateID)+".txt")
	_ = writeTextFile(path, raw)
	return path
}

func (s preflightArtifactSink) WriteError(candidateID string, message string) string {
	path := filepath.Join(s.dir, "preflight-error-"+sanitizeFilename(candidateID)+".json")
	_ = writeJSONFile(path, map[string]string{"error": message})
	return path
}

func buildPreflightRunResult(requestID string, entries []telemetry.ProviderReadinessEntry, elapsed time.Duration) consensus.RunResult {
	ready := 0
	for _, entry := range entries {
		if entry.Ready {
			ready++
		}
	}
	terminal := consensus.TerminalStateCompleted
	if ready != len(entries) {
		terminal = consensus.TerminalStateRequiresHumanReview
	}
	summary := fmt.Sprintf("Provider preflight completed: ready=%d/%d", ready, len(entries))
	highlights := make([]string, 0, len(entries))
	nextActions := []string{}
	for _, entry := range entries {
		label := entry.Provider
		if entry.Model != "" {
			label += "/" + entry.Model
		}
		status := "ready"
		if !entry.Ready {
			status = "not ready"
			nextActions = append(nextActions, fmt.Sprintf("修复 %s: %s", label, firstNonEmptyProfile(entry.Error, "unknown readiness failure")))
		}
		highlights = append(highlights, fmt.Sprintf("%s: %s strict=%t recoverable=%t duration=%dms", label, status, entry.StrictJSON, entry.RecoverableJSON, entry.DurationMs))
	}
	return consensus.RunResult{
		SchemaVersion: consensus.SchemaVersion,
		Mode:          consensus.WorkflowModeAdjudication,
		RequestID:     requestID,
		SessionID:     "preflight_" + requestID,
		TaskSpec: consensus.TaskSpec{
			Goal:     "provider readiness preflight",
			TaskType: consensus.CaseTaskTypeOperational,
		},
		TerminalState: terminal,
		Report: consensus.AdjudicationReport{
			Summary:     summary,
			Highlights:  highlights,
			NextActions: nextActions,
		},
		Metrics: consensus.Metrics{
			ElapsedMs:       elapsed.Milliseconds(),
			TasksDispatched: len(entries),
		},
	}
}

func buildPreflightSummary(result consensus.RunResult, entries []telemetry.ProviderReadinessEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Provider Preflight\n\n")
	fmt.Fprintf(&b, "- requestId: `%s`\n", result.RequestID)
	fmt.Fprintf(&b, "- terminalState: `%s`\n", result.TerminalState)
	fmt.Fprintf(&b, "- summary: %s\n\n", result.Report.Summary)
	b.WriteString("## Providers\n\n")
	for _, entry := range entries {
		label := entry.Provider
		if entry.Model != "" {
			label += "/" + entry.Model
		}
		fmt.Fprintf(&b, "- `%s`: ready=%t strict=%t recoverable=%t duration=%dms\n", label, entry.Ready, entry.StrictJSON, entry.RecoverableJSON, entry.DurationMs)
		if entry.Error != "" {
			fmt.Fprintf(&b, "  error: `%s`\n", entry.Error)
		}
	}
	return b.String()
}

func printPreflightSummary(writer interface{ Write([]byte) (int, error) }, entries []telemetry.ProviderReadinessEntry, paths config.RunArtifactPaths, verbose bool) {
	ready := 0
	for _, entry := range entries {
		if entry.Ready {
			ready++
		}
	}
	_, _ = fmt.Fprintf(writer, "[til-consensus] profile preflight completed ready=%d/%d\n", ready, len(entries))
	for _, entry := range entries {
		label := entry.Provider
		if entry.Model != "" {
			label += "/" + entry.Model
		}
		if entry.Agent != "" {
			label += " agent=" + entry.Agent
		}
		_, _ = fmt.Fprintf(writer, "  - %s ready=%t strict=%t recoverable=%t duration=%dms\n", label, entry.Ready, entry.StrictJSON, entry.RecoverableJSON, entry.DurationMs)
		if entry.Error != "" {
			_, _ = fmt.Fprintf(writer, "    error: %s\n", entry.Error)
		}
		if verbose {
			if entry.ProviderType != "" || entry.Protocol != "" {
				_, _ = fmt.Fprintf(writer, "    provider: type=%s protocol=%s\n", firstNonEmptyProfile(entry.ProviderType, "-"), firstNonEmptyProfile(entry.Protocol, "-"))
			}
			if entry.BaseURL != "" {
				_, _ = fmt.Fprintf(writer, "    base_url: %s\n", entry.BaseURL)
			}
			if entry.APIKeyEnv != "" {
				_, _ = fmt.Fprintf(writer, "    api_key_env: %s\n", entry.APIKeyEnv)
			}
			if len(entry.Command) > 0 {
				_, _ = fmt.Fprintf(writer, "    command: %s\n", strings.Join(entry.Command, " "))
			}
			if entry.StdoutPreview != "" {
				_, _ = fmt.Fprintf(writer, "    stdout: %s\n", entry.StdoutPreview)
			}
			if entry.StderrPreview != "" {
				_, _ = fmt.Fprintf(writer, "    stderr: %s\n", entry.StderrPreview)
			}
		}
	}
	_, _ = fmt.Fprintf(writer, "  result: %s\n", paths.ResultPath)
	_, _ = fmt.Fprintf(writer, "  readiness: %s\n", filepath.Join(paths.ArtifactsDir, "provider-readiness.json"))
	_, _ = fmt.Fprintf(writer, "  summary: %s\n", paths.SummaryPath)
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func writeTextFile(path string, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value), 0o644)
}

func firstNonEmptyProfile(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
