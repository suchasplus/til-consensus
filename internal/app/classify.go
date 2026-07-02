package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/suchasplus/til-consensus/config"
	tilrunner "github.com/suchasplus/til-consensus/runner"
	"github.com/urfave/cli/v3"
)

func newClassifyCommand() *cli.Command {
	return &cli.Command{
		Name:      "classify",
		Usage:     "判断一段任务更适合 adjudication / free_debate / delphi，或指出需要先完善问题",
		ArgsUsage: "[task text]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "provider", Usage: "用于分类的 API provider", Value: tilrunner.DefaultClassifyProvider},
			&cli.StringFlag{Name: "model", Usage: "用于分类的 provider model id；默认使用 default，或唯一 enabled model"},
			&cli.StringFlag{Name: "file", Usage: "读取本地文件全文作为待分类任务"},
			&cli.BoolFlag{Name: "stdin", Usage: "从 stdin 读取待分类任务"},
			&cli.StringFlag{Name: "format", Usage: "输出格式(text|json)", Value: "text"},
			&cli.BoolFlag{Name: "verbose", Usage: "输出 provider/model 和请求摘要"},
			&cli.BoolFlag{Name: "debug", Usage: "输出原始 provider 响应"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runClassifyCommand(ctx, cmd)
		},
	}
}

func runClassifyCommand(ctx context.Context, cmd *cli.Command) error {
	task, source, err := resolveClassifyInput(cmd)
	if err != nil {
		return err
	}
	configPath, err := config.ResolveConfigPath(cmd.String("config"))
	if err != nil {
		return err
	}
	loaded, err := config.LoadProfilesWithProfile(configPath, cmd.String("profile"))
	if err != nil {
		return err
	}
	executor := tilrunner.NewExecutor(loaded)
	input := tilrunner.ClassifyInput{
		Task:       task,
		ProviderID: cmd.String("provider"),
		ModelID:    cmd.String("model"),
	}
	prepared, err := executor.PrepareClassify(input)
	if err != nil {
		return err
	}
	if cmd.Bool("verbose") {
		printClassifyVerbose(cmd.Writer, loaded, prepared, source)
	}
	result, err := executor.ClassifyPrepared(ctx, prepared)
	if err != nil {
		return err
	}
	if cmd.Bool("debug") {
		_, _ = fmt.Fprintf(cmd.Writer, "[til-consensus][debug] classify raw=%s\n", strings.TrimSpace(result.Raw))
	}
	return writeClassifyOutput(cmd.Writer, cmd.String("format"), result, configPath, source)
}

func resolveClassifyInput(cmd *cli.Command) (string, string, error) {
	filePath := strings.TrimSpace(cmd.String("file"))
	useStdin := cmd.Bool("stdin")
	args := cmd.Args().Slice()
	if filePath != "" && (useStdin || len(args) > 0) {
		return "", "", fmt.Errorf("--file 不能与 --stdin 或位置参数同时使用")
	}
	if useStdin && len(args) > 0 {
		return "", "", fmt.Errorf("--stdin 不能与位置参数同时使用")
	}
	switch {
	case filePath != "":
		body, err := os.ReadFile(filePath)
		if err != nil {
			return "", "", fmt.Errorf("read classify file: %w", err)
		}
		task := strings.TrimSpace(string(body))
		if task == "" {
			return "", "", fmt.Errorf("classify file is empty: %s", filePath)
		}
		return task, filePath, nil
	case useStdin:
		reader := cmd.Reader
		if reader == nil {
			reader = os.Stdin
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			return "", "", fmt.Errorf("read classify stdin: %w", err)
		}
		task := strings.TrimSpace(string(body))
		if task == "" {
			return "", "", fmt.Errorf("stdin is empty")
		}
		return task, "stdin", nil
	case len(args) > 0:
		task := strings.TrimSpace(strings.Join(args, " "))
		if task == "" {
			return "", "", fmt.Errorf("classify task is empty")
		}
		return task, "argument", nil
	default:
		return "", "", fmt.Errorf("missing classify input: pass text, --file, or --stdin")
	}
}

func printClassifyVerbose(
	writer io.Writer,
	loaded config.LoadedConfig,
	prepared tilrunner.ClassifyPreparedRequest,
	source string,
) {
	_, _ = fmt.Fprintf(writer, "[til-consensus] classify started\n")
	_, _ = fmt.Fprintf(writer, "  config: %s\n", loaded.Path)
	if loaded.Profile != "" {
		_, _ = fmt.Fprintf(writer, "  profile: %s\n", loaded.Profile)
	}
	_, _ = fmt.Fprintf(writer, "  source: %s\n", source)
	_, _ = fmt.Fprintf(writer, "  input_chars: %d\n", len([]rune(prepared.Task)))
	selection := prepared.Selection
	_, _ = fmt.Fprintf(writer, "  provider: %s/%s type=%s protocol=%s\n", selection.ProviderID, selection.ModelID, selection.Provider.Type, selection.Provider.Protocol)
	_, _ = fmt.Fprintf(writer, "  provider_model: %s\n", selection.ProviderModel)
	request, err := prepared.PreviewRequestContext()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "  request_preview_error: %v\n", err)
		return
	}
	if endpoint, ok := request["endpoint"].(string); ok && endpoint != "" {
		_, _ = fmt.Fprintf(writer, "  request: POST %s\n", endpoint)
	}
	if transport, ok := request["transport"].(string); ok && transport != "" {
		_, _ = fmt.Fprintf(writer, "    transport: %s\n", transport)
	}
	if generation, ok := request["generation"].(map[string]any); ok && len(generation) > 0 {
		encoded, _ := json.Marshal(generation)
		_, _ = fmt.Fprintf(writer, "    generation: %s\n", encoded)
	}
}

func writeClassifyOutput(writer io.Writer, format string, result tilrunner.ClassifyResult, configPath string, source string) error {
	switch strings.TrimSpace(format) {
	case "", "text":
		writeClassifyText(writer, result, configPath, source)
		return nil
	case "json":
		body, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		_, _ = writer.Write(append(body, '\n'))
		return nil
	default:
		return fmt.Errorf("unsupported classify format: %s", format)
	}
}

func writeClassifyText(writer io.Writer, result tilrunner.ClassifyResult, configPath string, source string) {
	_, _ = fmt.Fprintf(writer, "[til-consensus] classify completed recommendation=%s confidence=%.2f\n", result.Recommendation, result.Confidence)
	_, _ = fmt.Fprintf(writer, "  summary: %s\n", strings.TrimSpace(result.Summary))
	if len(result.Why) > 0 {
		_, _ = fmt.Fprintf(writer, "  why:\n")
		for _, item := range result.Why {
			if strings.TrimSpace(item) != "" {
				_, _ = fmt.Fprintf(writer, "    - %s\n", strings.TrimSpace(item))
			}
		}
	}
	if len(result.MissingInformation) > 0 {
		_, _ = fmt.Fprintf(writer, "  missing_information:\n")
		for _, item := range result.MissingInformation {
			if strings.TrimSpace(item) != "" {
				_, _ = fmt.Fprintf(writer, "    - %s\n", strings.TrimSpace(item))
			}
		}
	}
	if result.Recommendation == "needs_clarification" && strings.TrimSpace(result.EstimatedModeAfterClarification) != "" {
		_, _ = fmt.Fprintf(writer, "  estimated_mode_after_clarification: %s\n", strings.TrimSpace(result.EstimatedModeAfterClarification))
		if strings.TrimSpace(result.EstimatedModeReason) != "" {
			_, _ = fmt.Fprintf(writer, "  estimated_mode_reason: %s\n", strings.TrimSpace(result.EstimatedModeReason))
		}
	}
	if strings.TrimSpace(result.SuggestedTask) != "" {
		_, _ = fmt.Fprintf(writer, "  suggested_task: %s\n", strings.TrimSpace(result.SuggestedTask))
	}
	if command := classifyRecommendedCommand(result, configPath, source); command != "" {
		_, _ = fmt.Fprintf(writer, "  command: %s\n", command)
	}
}

func classifyRecommendedCommand(result tilrunner.ClassifyResult, configPath string, source string) string {
	var command string
	switch result.Recommendation {
	case "adjudication":
		command = "ask"
	case "free_debate":
		command = "debate"
	case "delphi":
		command = "delphi"
	default:
		return ""
	}
	task := strings.TrimSpace(result.SuggestedTask)
	if task == "" {
		return ""
	}
	if source != "" && source != "argument" && source != "stdin" {
		return fmt.Sprintf("til-consensus %s --config %s --task-file %s", command, shellQuote(configPath), shellQuote(source))
	}
	return fmt.Sprintf("til-consensus %s %s --config %s", command, shellQuote(task), shellQuote(configPath))
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return r == '\'' || r == '"' || r == '\\' || r == '$' || r == '`' || r == ' ' || r == '\t' || r == '\n'
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
