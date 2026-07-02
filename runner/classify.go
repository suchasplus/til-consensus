package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/suchasplus/til-consensus/config"
	runtimelib "github.com/suchasplus/til-consensus/runtime"
	apirunner "github.com/suchasplus/til-consensus/runtime/api"
)

const (
	DefaultClassifyProvider  = "gemini-api"
	DefaultClassifyModel     = "default"
	DefaultClassifyMaxTokens = 2048
)

type ClassifyInput struct {
	Task       string
	ProviderID string
	ModelID    string
}

type ClassifyResult struct {
	Recommendation                  string   `json:"recommendation"`
	Confidence                      float64  `json:"confidence"`
	Summary                         string   `json:"summary"`
	Why                             []string `json:"why"`
	MissingInformation              []string `json:"missingInformation"`
	EstimatedModeAfterClarification string   `json:"estimatedModeAfterClarification"`
	EstimatedModeReason             string   `json:"estimatedModeReason"`
	SuggestedTask                   string   `json:"suggestedTask"`
	Raw                             string   `json:"-"`
}

type ClassifyProviderSelection struct {
	ProviderID    string
	ModelID       string
	Provider      config.ProviderConfig
	Model         config.ProviderModelConfig
	ProviderModel string
}

type ClassifyPreparedRequest struct {
	Task         string
	SystemPrompt string
	Prompt       string
	Schema       map[string]any
	Selection    ClassifyProviderSelection
}

func (e *Executor) PrepareClassify(input ClassifyInput) (ClassifyPreparedRequest, error) {
	task := strings.TrimSpace(input.Task)
	if task == "" {
		return ClassifyPreparedRequest{}, fmt.Errorf("classify task is required")
	}
	selection, err := ResolveClassifyProvider(e.Loaded.Config, input.ProviderID, input.ModelID)
	if err != nil {
		return ClassifyPreparedRequest{}, err
	}
	return ClassifyPreparedRequest{
		Task:         task,
		SystemPrompt: "Output only a JSON object. No markdown.",
		Prompt:       BuildClassifyPrompt(task),
		Schema:       ClassifyJSONSchema(),
		Selection:    selection,
	}, nil
}

func (e *Executor) Classify(ctx context.Context, input ClassifyInput) (ClassifyResult, error) {
	prepared, err := e.PrepareClassify(input)
	if err != nil {
		return ClassifyResult{}, err
	}
	return e.ClassifyPrepared(ctx, prepared)
}

func (e *Executor) ClassifyPrepared(ctx context.Context, prepared ClassifyPreparedRequest) (ClassifyResult, error) {
	raw, err := apirunner.NewRunner(prepared.Selection.Provider).RunTask(
		ctx,
		prepared.Prompt,
		prepared.SystemPrompt,
		prepared.Selection.ProviderModel,
		prepared.Selection.Model.Temperature,
		prepared.Selection.Model.Reasoning,
		prepared.MaxOutputTokens(),
		prepared.Schema,
	)
	if err != nil {
		return ClassifyResult{}, err
	}
	result, err := DecodeClassifyOutput(raw)
	if err != nil {
		return ClassifyResult{}, err
	}
	result.Raw = raw
	result.SuggestedTask = firstNonEmptyString(result.SuggestedTask, prepared.Task)
	return result, nil
}

func (p ClassifyPreparedRequest) MaxOutputTokens() int {
	return classifyMaxOutputTokens(p.Selection.Model)
}

func (p ClassifyPreparedRequest) PreviewRequestContext() (map[string]any, error) {
	return apirunner.PreviewRequestContext(
		p.Selection.Provider,
		p.Prompt,
		p.SystemPrompt,
		p.Selection.ProviderModel,
		p.Selection.Model.Temperature,
		p.Selection.Model.Reasoning,
		p.MaxOutputTokens(),
		p.Schema,
	)
}

func ResolveClassifyProvider(cfg config.Config, providerID string, modelID string) (ClassifyProviderSelection, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		providerID = DefaultClassifyProvider
	}
	provider, ok := cfg.Providers[providerID]
	if !ok {
		return ClassifyProviderSelection{}, fmt.Errorf("classify provider not found: %s", providerID)
	}
	if !config.IsProviderEnabled(provider) {
		return ClassifyProviderSelection{}, fmt.Errorf("classify provider is disabled: %s", providerID)
	}
	if provider.Type != config.ProviderTypeAPI {
		return ClassifyProviderSelection{}, fmt.Errorf("classify provider %s must be type=api, got %s", providerID, provider.Type)
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		var err error
		modelID, err = defaultClassifyModelID(provider)
		if err != nil {
			return ClassifyProviderSelection{}, fmt.Errorf("classify provider %s: %w", providerID, err)
		}
	}
	model, ok := provider.Models[modelID]
	if !ok {
		return ClassifyProviderSelection{}, fmt.Errorf("classify model not found: provider=%s model=%s", providerID, modelID)
	}
	if !config.IsProviderModelEnabled(model) {
		return ClassifyProviderSelection{}, fmt.Errorf("classify model is disabled: provider=%s model=%s", providerID, modelID)
	}
	if provider.APIKeyEnv != "" && strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) == "" {
		return ClassifyProviderSelection{}, fmt.Errorf("classify provider %s requires env %s", providerID, provider.APIKeyEnv)
	}
	providerModel := strings.TrimSpace(model.ProviderModel)
	if providerModel == "" {
		providerModel = modelID
	}
	return ClassifyProviderSelection{
		ProviderID:    providerID,
		ModelID:       modelID,
		Provider:      provider,
		Model:         model,
		ProviderModel: providerModel,
	}, nil
}

func BuildClassifyPrompt(task string) string {
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

func ClassifyJSONSchema() map[string]any {
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

func DecodeClassifyOutput(raw string) (ClassifyResult, error) {
	var out ClassifyResult
	if err := decodeStrictClassifyOutput([]byte(strings.TrimSpace(raw)), &out); err != nil {
		value, parseErr := runtimelib.ParseJSONObject(raw)
		if parseErr != nil {
			return ClassifyResult{}, fmt.Errorf("decode classify output: %w", err)
		}
		body, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return ClassifyResult{}, fmt.Errorf("decode classify output: %w", marshalErr)
		}
		if strictErr := decodeStrictClassifyOutput(body, &out); strictErr != nil {
			return ClassifyResult{}, fmt.Errorf("decode classify output: %w", strictErr)
		}
	}
	if err := validateClassifyOutput(out); err != nil {
		return ClassifyResult{}, err
	}
	return out, nil
}

func defaultClassifyModelID(provider config.ProviderConfig) (string, error) {
	if model, ok := provider.Models[DefaultClassifyModel]; ok && config.IsProviderModelEnabled(model) {
		return DefaultClassifyModel, nil
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

func decodeStrictClassifyOutput(body []byte, out *ClassifyResult) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func validateClassifyOutput(out ClassifyResult) error {
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
	if model.MaxOutputTokens > 0 && model.MaxOutputTokens < DefaultClassifyMaxTokens {
		return model.MaxOutputTokens
	}
	return DefaultClassifyMaxTokens
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
