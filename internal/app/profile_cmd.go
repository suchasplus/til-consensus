package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/internal/artifact"
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
			&cli.StringFlag{Name: "config-profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "output", Usage: "覆盖本次 preflight 输出目录模板，例如 ./out/{requestId}"},
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
	loaded, err := config.LoadProfilesWithProfile(configPath, cmd.String("config-profile"))
	if err != nil {
		return err
	}
	if output := strings.TrimSpace(cmd.String("output")); output != "" {
		loaded.Config.Output.Directory = output
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
	printer := newPreflightPrinter(cmd.Writer, cmd.Bool("verbose"))
	entries, err := preflight.Run(ctx, loaded.Config, preflight.Options{
		ProviderIDs: cmd.StringSlice("provider"),
		AgentIDs:    cmd.StringSlice("agent"),
		All:         cmd.Bool("all"),
		Timeout:     cmd.Duration("timeout"),
		OnEntry:     printer.PrintEntry,
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

	printer.PrintFinal(entries, paths)
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

type preflightPrinter struct {
	writer  io.Writer
	verbose bool
	color   bool
}

func newPreflightPrinter(writer io.Writer, verbose bool) *preflightPrinter {
	return &preflightPrinter{
		writer:  writer,
		verbose: verbose,
		color:   shouldEnableColor(writer),
	}
}

func (p *preflightPrinter) PrintEntry(entry telemetry.ProviderReadinessEntry) {
	label := entry.Provider
	if entry.Model != "" {
		label += "/" + entry.Model
	}
	if entry.Agent != "" {
		label += " agent=" + entry.Agent
	}
	if p.color {
		label = ansi(34, label)
	}
	readyText := fmt.Sprintf("ready=%t", entry.Ready)
	if p.color {
		if entry.Ready {
			readyText = ansi(32, readyText)
		} else {
			readyText = ansi(31, readyText)
		}
	}
	_, _ = fmt.Fprintf(p.writer, "  - %s %s strict=%t recoverable=%t duration=%dms\n", label, readyText, entry.StrictJSON, entry.RecoverableJSON, entry.DurationMs)
	if entry.Error != "" {
		errorText := "error: " + entry.Error
		if p.color {
			errorText = ansi(31, errorText)
		}
		_, _ = fmt.Fprintf(p.writer, "    %s\n", errorText)
	}
	if !p.verbose {
		return
	}
	if entry.ProviderType != "" || entry.Protocol != "" {
		_, _ = fmt.Fprintf(p.writer, "    provider: type=%s protocol=%s\n", firstNonEmptyProfile(entry.ProviderType, "-"), firstNonEmptyProfile(entry.Protocol, "-"))
	}
	if entry.BaseURL != "" {
		_, _ = fmt.Fprintf(p.writer, "    base_url: %s\n", entry.BaseURL)
	}
	if entry.APIKeyEnv != "" {
		_, _ = fmt.Fprintf(p.writer, "    api_key_env: %s\n", entry.APIKeyEnv)
	}
	if len(entry.Command) > 0 {
		_, _ = fmt.Fprintf(p.writer, "    command: %s\n", formatCommandLine(entry.Command))
	}
	p.printRequestContext(entry.RequestContext)
	p.printResponseContext(entry.ResponseContext)
	if entry.InputArtifact != "" {
		_, _ = fmt.Fprintf(p.writer, "    input_artifact: %s\n", entry.InputArtifact)
	}
	if entry.RawArtifact != "" {
		_, _ = fmt.Fprintf(p.writer, "    raw_artifact: %s\n", entry.RawArtifact)
	}
	if entry.ErrorArtifact != "" {
		_, _ = fmt.Fprintf(p.writer, "    error_artifact: %s\n", entry.ErrorArtifact)
	}
	if entry.StdoutFull != "" {
		_, _ = fmt.Fprintf(p.writer, "    stdout: %s\n", strings.TrimRight(entry.StdoutFull, "\n"))
	} else if entry.StdoutPreview != "" {
		_, _ = fmt.Fprintf(p.writer, "    stdout: %s\n", entry.StdoutPreview)
	}
	if entry.StderrPreview != "" {
		_, _ = fmt.Fprintf(p.writer, "    stderr: %s\n", entry.StderrPreview)
	}
}

func (p *preflightPrinter) printRequestContext(ctx map[string]any) {
	if len(ctx) == 0 {
		return
	}
	if errText := stringFromAny(ctx["previewError"]); errText != "" {
		_, _ = fmt.Fprintf(p.writer, "    request: preview_error=%s\n", errText)
		return
	}
	endpoint := stringFromAny(ctx["endpoint"])
	method := firstNonEmptyProfile(stringFromAny(ctx["method"]), "POST")
	if endpoint != "" {
		_, _ = fmt.Fprintf(p.writer, "    request: %s %s\n", method, endpoint)
	} else {
		_, _ = fmt.Fprintln(p.writer, "    request:")
	}
	if transport := stringFromAny(ctx["transport"]); transport != "" {
		_, _ = fmt.Fprintf(p.writer, "      transport: %s\n", transport)
	}
	if auth := stringFromAny(ctx["auth"]); auth != "" {
		_, _ = fmt.Fprintf(p.writer, "      auth: %s\n", auth)
	}
	timeout := stringFromAny(ctx["timeout"])
	if timeout == "" {
		timeout = stringFromAny(ctx["timeoutMs"]) + "ms"
	}
	if timeout != "" && timeout != "ms" {
		_, _ = fmt.Fprintf(p.writer, "      timeout: %s\n", timeout)
	}
	if generation, ok := ctx["generation"].(map[string]any); ok && len(generation) > 0 {
		_, _ = fmt.Fprintf(p.writer, "      generation: %s\n", formatContextMap(generation, []string{
			"maxOutputTokens",
			"configuredMaxOutputTokens",
			"budgetPolicy",
			"temperature",
			"responseMimeType",
			"responseJsonSchema",
			"responseFormat",
			"responseFormatName",
			"thinkingLevel",
			"reasoning",
			"reasoningField",
			"maxOutputTokensField",
			"maxTokensField",
		}))
	}
	if schema, ok := ctx["schema"].(map[string]any); ok && len(schema) > 0 {
		_, _ = fmt.Fprintf(p.writer, "      schema: %s\n", formatContextMap(schema, []string{"type", "required", "additionalProperties", "enabled"}))
	}
	if extra := stringSliceFromAny(ctx["extraBody"]); len(extra) > 0 {
		_, _ = fmt.Fprintf(p.writer, "      extra_body: %s\n", strings.Join(extra, ", "))
	}
	if system := stringFromAny(ctx["system"]); system != "" {
		_, _ = fmt.Fprintf(p.writer, "      system: %q\n", system)
	}
	if prompt := stringFromAny(ctx["prompt"]); prompt != "" {
		_, _ = fmt.Fprintf(p.writer, "      prompt: %q\n", prompt)
	}
}

func (p *preflightPrinter) printResponseContext(ctx map[string]any) {
	if len(ctx) == 0 {
		return
	}
	format := firstNonEmptyProfile(stringFromAny(ctx["format"]), "response")
	parts := []string{format}
	for _, key := range []string{"provider", "model", "id", "choiceCount"} {
		if value := stringFromAny(ctx[key]); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	_, _ = fmt.Fprintf(p.writer, "    response: %s\n", strings.Join(parts, " "))
	choice, ok := ctx["choice"].(map[string]any)
	if !ok || len(choice) == 0 {
		return
	}
	choiceParts := []string{}
	for _, key := range []string{"index", "finishReason", "nativeFinishReason"} {
		if value := stringFromAny(choice[key]); value != "" {
			choiceParts = append(choiceParts, key+"="+value)
		}
	}
	if len(choiceParts) > 0 {
		_, _ = fmt.Fprintf(p.writer, "      choice: %s\n", strings.Join(choiceParts, " "))
	}
	message, ok := choice["message"].(map[string]any)
	if !ok || len(message) == 0 {
		return
	}
	messageParts := []string{}
	for _, key := range []string{"role", "contentState", "contentChars", "refusalState", "reasoningState", "reasoningContent"} {
		if value := stringFromAny(message[key]); value != "" {
			messageParts = append(messageParts, key+"="+value)
		}
	}
	if len(messageParts) > 0 {
		_, _ = fmt.Fprintf(p.writer, "      message: %s\n", strings.Join(messageParts, " "))
	}
	if preview := stringFromAny(message["contentPreview"]); preview != "" {
		_, _ = fmt.Fprintf(p.writer, "      content_preview: %q\n", preview)
	}
}

func (p *preflightPrinter) PrintFinal(entries []telemetry.ProviderReadinessEntry, paths config.RunArtifactPaths) {
	ready := 0
	for _, entry := range entries {
		if entry.Ready {
			ready++
		}
	}
	line := fmt.Sprintf("[til-consensus] profile preflight completed ready=%d/%d", ready, len(entries))
	if p.color {
		if ready == len(entries) {
			line = ansi(32, line)
		} else {
			line = ansi(31, line)
		}
	}
	_, _ = fmt.Fprintf(p.writer, "%s\n", line)
	_, _ = fmt.Fprintf(p.writer, "  result: %s\n", paths.ResultPath)
	_, _ = fmt.Fprintf(p.writer, "  readiness: %s\n", filepath.Join(paths.ArtifactsDir, "provider-readiness.json"))
	_, _ = fmt.Fprintf(p.writer, "  summary: %s\n", paths.SummaryPath)
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

func formatContextMap(values map[string]any, order []string) string {
	if len(values) == 0 {
		return "-"
	}
	seen := map[string]struct{}{}
	parts := []string{}
	appendKey := func(key string) {
		value, ok := values[key]
		if !ok {
			return
		}
		text := stringFromAny(value)
		if text == "" {
			return
		}
		parts = append(parts, key+"="+text)
		seen[key] = struct{}{}
	}
	for _, key := range order {
		appendKey(key)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		appendKey(key)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case []string:
		return strings.Join(typed, ",")
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text := stringFromAny(item)
			if text != "" {
				values = append(values, text)
			}
		}
		return strings.Join(values, ",")
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringFromAny(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmptyProfile(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func formatCommandLine(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, arg := range argv {
		parts = append(parts, quoteCommandArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteCommandArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if strings.ContainsAny(arg, " \t\r\n\"'{}[]():;|&<>$`\\") {
		return strconv.Quote(arg)
	}
	return arg
}
