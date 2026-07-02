package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/suchasplus/til-consensus/config"
	runtimelib "github.com/suchasplus/til-consensus/runtime"
	apirunner "github.com/suchasplus/til-consensus/runtime/api"
	"github.com/urfave/cli/v3"
)

const (
	classifyDefaultProvider  = "gemini-api"
	classifyDefaultModel     = "default"
	classifyDefaultMaxTokens = 2048
)

type classifyOutput struct {
	Recommendation                  string   `json:"recommendation"`
	Confidence                      float64  `json:"confidence"`
	Summary                         string   `json:"summary"`
	Why                             []string `json:"why"`
	MissingInformation              []string `json:"missingInformation"`
	EstimatedModeAfterClarification string   `json:"estimatedModeAfterClarification"`
	EstimatedModeReason             string   `json:"estimatedModeReason"`
	SuggestedTask                   string   `json:"suggestedTask"`
}

type classifyProviderSelection struct {
	ProviderID    string
	ModelID       string
	Provider      config.ProviderConfig
	Model         config.ProviderModelConfig
	ProviderModel string
}

func newClassifyCommand() *cli.Command {
	return &cli.Command{
		Name:      "classify",
		Usage:     "判断一段任务更适合 adjudication / free_debate / delphi，或指出需要先完善问题",
		ArgsUsage: "[task text]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "provider", Usage: "用于分类的 API provider", Value: classifyDefaultProvider},
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
	selection, err := resolveClassifyProvider(loaded.Config, cmd.String("provider"), cmd.String("model"))
	if err != nil {
		return err
	}

	systemPrompt := "Output only a JSON object. No markdown."
	prompt := buildClassifyPrompt(task)
	schema := classifyJSONSchema()
	if cmd.Bool("verbose") {
		printClassifyVerbose(cmd.Writer, loaded, selection, task, source, prompt, systemPrompt, schema)
	}

	raw, err := apirunner.NewRunner(selection.Provider).RunTask(
		ctx,
		prompt,
		systemPrompt,
		selection.ProviderModel,
		selection.Model.Temperature,
		selection.Model.Reasoning,
		classifyMaxOutputTokens(selection.Model),
		schema,
	)
	if err != nil {
		return err
	}
	if cmd.Bool("debug") {
		_, _ = fmt.Fprintf(cmd.Writer, "[til-consensus][debug] classify raw=%s\n", strings.TrimSpace(raw))
	}
	result, err := decodeClassifyOutput(raw)
	if err != nil {
		return err
	}
	result.SuggestedTask = firstClassifyNonEmptyString(result.SuggestedTask, task)
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

func resolveClassifyProvider(cfg config.Config, providerID string, modelID string) (classifyProviderSelection, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		providerID = classifyDefaultProvider
	}
	provider, ok := cfg.Providers[providerID]
	if !ok {
		return classifyProviderSelection{}, fmt.Errorf("classify provider not found: %s", providerID)
	}
	if !config.IsProviderEnabled(provider) {
		return classifyProviderSelection{}, fmt.Errorf("classify provider is disabled: %s", providerID)
	}
	if provider.Type != config.ProviderTypeAPI {
		return classifyProviderSelection{}, fmt.Errorf("classify provider %s must be type=api, got %s", providerID, provider.Type)
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		var err error
		modelID, err = defaultClassifyModelID(provider)
		if err != nil {
			return classifyProviderSelection{}, fmt.Errorf("classify provider %s: %w", providerID, err)
		}
	}
	model, ok := provider.Models[modelID]
	if !ok {
		return classifyProviderSelection{}, fmt.Errorf("classify model not found: provider=%s model=%s", providerID, modelID)
	}
	if !config.IsProviderModelEnabled(model) {
		return classifyProviderSelection{}, fmt.Errorf("classify model is disabled: provider=%s model=%s", providerID, modelID)
	}
	if provider.APIKeyEnv != "" && strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) == "" {
		return classifyProviderSelection{}, fmt.Errorf("classify provider %s requires env %s", providerID, provider.APIKeyEnv)
	}
	providerModel := strings.TrimSpace(model.ProviderModel)
	if providerModel == "" {
		providerModel = modelID
	}
	return classifyProviderSelection{
		ProviderID:    providerID,
		ModelID:       modelID,
		Provider:      provider,
		Model:         model,
		ProviderModel: providerModel,
	}, nil
}

func defaultClassifyModelID(provider config.ProviderConfig) (string, error) {
	if model, ok := provider.Models[classifyDefaultModel]; ok && config.IsProviderModelEnabled(model) {
		return classifyDefaultModel, nil
	}
	ids := make([]string, 0, len(provider.Models))
	for id, model := range provider.Models {
		if config.IsProviderModelEnabled(model) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("no enabled models")
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("multiple enabled models (%s); pass --model", strings.Join(ids, ","))
	}
}

func buildClassifyPrompt(task string) string {
	return strings.TrimSpace(fmt.Sprintf(`
你是 til-consensus 的任务路由器。请判断下面这段任务最适合哪种处理方式。

可选 recommendation：
- adjudication：适合 claim 级裁决、事实核查、代码/架构结论是否成立、需要 proposer/challenger/arbiter 给出裁决。
- free_debate：适合开放性取舍、方案碰撞、需要多个参与者提出不同立场并投票形成多数共识。
- delphi：适合专家匿名多轮评分、优先级排序、风险评估、路线图收敛，目标是降低权威偏见和从众效应。
- needs_clarification：当前问题缺少关键上下文、约束、评价标准或目标，不适合直接运行。
- not_suitable：任务过于简单、纯查事实、纯执行命令、或不需要多 agent 讨论。

判断标准：
- 不要为了使用工具而强行推荐三种模式。
- 如果缺少目标、约束、评价标准、候选方案、上下文或成功标准，优先 recommendation=needs_clarification。
- 如果只是简单事实查询或单步命令，优先 recommendation=not_suitable。
- confidence 必须是 0 到 1 的数字。
- why 用 2 到 5 条短理由说明。
- missingInformation 只列真正需要用户补充的信息；没有则返回 []。
- 当 recommendation=needs_clarification 时，estimatedModeAfterClarification 必须填写 adjudication/free_debate/delphi 三者之一，表示用户补齐 missingInformation 后大概率适合的任务模式。
- 当 recommendation=needs_clarification 时，estimatedModeReason 必须说明为什么补齐后会倾向这个模式。
- 当 recommendation 不是 needs_clarification 时，estimatedModeAfterClarification 和 estimatedModeReason 返回空字符串。
- suggestedTask 给出一个更适合直接运行的任务表述；如果不需要改写，返回原任务的精炼版。

待分类任务：
%s
`, task))
}

func classifyJSONSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{
			"recommendation",
			"confidence",
			"summary",
			"why",
			"missingInformation",
			"estimatedModeAfterClarification",
			"estimatedModeReason",
			"suggestedTask",
		},
		"properties": map[string]any{
			"recommendation": map[string]any{
				"type": "string",
				"enum": []string{"adjudication", "free_debate", "delphi", "needs_clarification", "not_suitable"},
			},
			"confidence": map[string]any{
				"type":    "number",
				"minimum": 0,
				"maximum": 1,
			},
			"summary": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"why": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": 5,
				"items": map[string]any{
					"type": "string",
				},
			},
			"missingInformation": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
			"estimatedModeAfterClarification": map[string]any{
				"type": "string",
				"enum": []string{"", "adjudication", "free_debate", "delphi"},
			},
			"estimatedModeReason": map[string]any{
				"type": "string",
			},
			"suggestedTask": map[string]any{
				"type": "string",
			},
		},
	}
}

func decodeClassifyOutput(raw string) (classifyOutput, error) {
	var out classifyOutput
	if err := decodeStrictClassifyOutput([]byte(strings.TrimSpace(raw)), &out); err != nil {
		value, parseErr := runtimelib.ParseJSONObject(raw)
		if parseErr != nil {
			return classifyOutput{}, fmt.Errorf("decode classify output: %w", err)
		}
		body, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return classifyOutput{}, fmt.Errorf("decode classify output: %w", marshalErr)
		}
		if strictErr := decodeStrictClassifyOutput(body, &out); strictErr != nil {
			return classifyOutput{}, fmt.Errorf("decode classify output: %w", strictErr)
		}
	}
	if err := validateClassifyOutput(out); err != nil {
		return classifyOutput{}, err
	}
	return out, nil
}

func decodeStrictClassifyOutput(body []byte, out *classifyOutput) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func validateClassifyOutput(out classifyOutput) error {
	switch out.Recommendation {
	case "adjudication", "free_debate", "delphi", "needs_clarification", "not_suitable":
	default:
		return fmt.Errorf("decode classify output: unsupported recommendation %q", out.Recommendation)
	}
	if out.Confidence < 0 || out.Confidence > 1 {
		return fmt.Errorf("decode classify output: confidence must be in [0,1], got %.4f", out.Confidence)
	}
	if strings.TrimSpace(out.Summary) == "" {
		return fmt.Errorf("decode classify output: summary is required")
	}
	if len(out.Why) == 0 {
		return fmt.Errorf("decode classify output: why must not be empty")
	}
	if out.Recommendation == "needs_clarification" {
		if !isConcreteClassifyMode(out.EstimatedModeAfterClarification) {
			return fmt.Errorf("decode classify output: estimatedModeAfterClarification must be adjudication/free_debate/delphi when recommendation=needs_clarification")
		}
		if strings.TrimSpace(out.EstimatedModeReason) == "" {
			return fmt.Errorf("decode classify output: estimatedModeReason is required when recommendation=needs_clarification")
		}
	}
	if out.EstimatedModeAfterClarification != "" && !isConcreteClassifyMode(out.EstimatedModeAfterClarification) {
		return fmt.Errorf("decode classify output: unsupported estimatedModeAfterClarification %q", out.EstimatedModeAfterClarification)
	}
	return nil
}

func isConcreteClassifyMode(value string) bool {
	switch value {
	case "adjudication", "free_debate", "delphi":
		return true
	default:
		return false
	}
}

func classifyMaxOutputTokens(model config.ProviderModelConfig) int {
	if model.MaxOutputTokens > 0 && model.MaxOutputTokens < classifyDefaultMaxTokens {
		return model.MaxOutputTokens
	}
	return classifyDefaultMaxTokens
}

func printClassifyVerbose(
	writer io.Writer,
	loaded config.LoadedConfig,
	selection classifyProviderSelection,
	task string,
	source string,
	prompt string,
	systemPrompt string,
	schema map[string]any,
) {
	_, _ = fmt.Fprintf(writer, "[til-consensus] classify started\n")
	_, _ = fmt.Fprintf(writer, "  config: %s\n", loaded.Path)
	if loaded.Profile != "" {
		_, _ = fmt.Fprintf(writer, "  profile: %s\n", loaded.Profile)
	}
	_, _ = fmt.Fprintf(writer, "  source: %s\n", source)
	_, _ = fmt.Fprintf(writer, "  input_chars: %d\n", len([]rune(task)))
	_, _ = fmt.Fprintf(writer, "  provider: %s/%s type=%s protocol=%s\n", selection.ProviderID, selection.ModelID, selection.Provider.Type, selection.Provider.Protocol)
	_, _ = fmt.Fprintf(writer, "  provider_model: %s\n", selection.ProviderModel)
	request, err := apirunner.PreviewRequestContext(
		selection.Provider,
		prompt,
		systemPrompt,
		selection.ProviderModel,
		selection.Model.Temperature,
		selection.Model.Reasoning,
		classifyMaxOutputTokens(selection.Model),
		schema,
	)
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

func writeClassifyOutput(writer io.Writer, format string, result classifyOutput, configPath string, source string) error {
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

func writeClassifyText(writer io.Writer, result classifyOutput, configPath string, source string) {
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

func classifyRecommendedCommand(result classifyOutput, configPath string, source string) string {
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

func firstClassifyNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
