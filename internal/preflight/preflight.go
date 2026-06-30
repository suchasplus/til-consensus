package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	tilruntime "github.com/suchasplus/til-consensus/internal/runtime"
	apirunner "github.com/suchasplus/til-consensus/internal/runtime/api"
	clirunner "github.com/suchasplus/til-consensus/internal/runtime/cli"
	"github.com/suchasplus/til-consensus/internal/telemetry"
)

const (
	defaultPrompt       = `只返回一个 JSON 对象：{"ok":true}`
	defaultSystemPrompt = "你必须只输出一个 JSON 对象，不要输出 markdown。"
)

type Options struct {
	ProviderIDs []string
	AgentIDs    []string
	All         bool
	Timeout     time.Duration
	Prompt      string
	OnEntry     func(telemetry.ProviderReadinessEntry)
}

type ArtifactSink interface {
	WriteInput(candidateID string, payload any) string
	WriteRaw(candidateID string, raw string) string
	WriteError(candidateID string, message string) string
}

type candidate struct {
	ID            string
	ProviderID    string
	Provider      config.ProviderConfig
	ProviderType  string
	Protocol      string
	CLIType       string
	ModelID       string
	ModelConfig   config.ProviderModelConfig
	ProviderModel string
	AgentID       string
	Agent         *config.AgentConfig
}

type preflightTask struct {
	consensus.TaskMeta
}

func (preflightTask) Kind() consensus.TaskKind { return consensus.TaskKind("preflight") }
func (t preflightTask) Meta() consensus.TaskMeta {
	return t.TaskMeta
}

func Run(ctx context.Context, cfg config.Config, opts Options, sink ArtifactSink) ([]telemetry.ProviderReadinessEntry, error) {
	cfg = config.Normalize(cfg)
	candidates, err := buildCandidates(cfg, opts)
	if err != nil {
		return nil, err
	}
	entries := make([]telemetry.ProviderReadinessEntry, 0, len(candidates))
	for _, item := range candidates {
		entry := probeCandidate(ctx, item, opts, sink)
		entries = append(entries, entry)
		if opts.OnEntry != nil {
			opts.OnEntry(entry)
		}
	}
	return entries, nil
}

func buildCandidates(cfg config.Config, opts Options) ([]candidate, error) {
	agentsByID := make(map[string]config.AgentConfig, len(cfg.Agents))
	for _, agent := range cfg.Agents {
		agentsByID[agent.ID] = agent
	}
	providerFilter := stringSet(opts.ProviderIDs)
	agentFilter := stringSet(opts.AgentIDs)

	if len(agentFilter) > 0 {
		out := make([]candidate, 0, len(agentFilter))
		for _, agentID := range sortedSetKeys(agentFilter) {
			agent, ok := agentsByID[agentID]
			if !ok {
				return nil, fmt.Errorf("unknown agent: %s", agentID)
			}
			item, err := candidateForAgent(cfg, agent)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, nil
	}

	out := []candidate{}
	providerIDs := sortedProviderIDs(cfg.Providers)
	for _, providerID := range providerIDs {
		explicitlySelected := false
		if len(providerFilter) > 0 {
			if _, ok := providerFilter[providerID]; !ok {
				continue
			}
			explicitlySelected = true
		}
		provider := cfg.Providers[providerID]
		if !config.IsProviderEnabled(provider) {
			if explicitlySelected {
				return nil, fmt.Errorf("provider %s is disabled", providerID)
			}
			continue
		}
		items := candidatesForProvider(providerID, provider)
		out = append(out, items...)
	}
	if len(providerFilter) > 0 {
		known := stringSet(providerIDs)
		for providerID := range providerFilter {
			if _, ok := known[providerID]; !ok {
				return nil, fmt.Errorf("unknown provider: %s", providerID)
			}
		}
	}
	if !opts.All && len(providerFilter) == 0 && len(agentFilter) == 0 {
		return out, nil
	}
	return out, nil
}

func candidateForAgent(cfg config.Config, agent config.AgentConfig) (candidate, error) {
	provider, ok := cfg.Providers[agent.Provider]
	if !ok {
		return candidate{}, fmt.Errorf("agent %s references unknown provider %s", agent.ID, agent.Provider)
	}
	if !config.IsProviderEnabled(provider) {
		return candidate{}, fmt.Errorf("agent %s references disabled provider %s", agent.ID, agent.Provider)
	}
	modelID := strings.TrimSpace(agent.Model)
	modelConfig := config.ProviderModelConfig{}
	if len(provider.Models) > 0 {
		if modelID == "" {
			if inferred, ok := singleModelID(provider); ok {
				modelID = inferred
			}
		}
		if modelID == "" {
			return candidate{}, fmt.Errorf("agent %s: model is required", agent.ID)
		}
		resolved, ok := provider.Models[modelID]
		if !ok {
			return candidate{}, fmt.Errorf("agent %s: unknown model %s for provider %s", agent.ID, modelID, agent.Provider)
		}
		if !config.IsProviderModelEnabled(resolved) {
			return candidate{}, fmt.Errorf("agent %s references disabled model %s for provider %s", agent.ID, modelID, agent.Provider)
		}
		modelConfig = resolved
	}
	providerModel := providerModelFor(provider, modelID, modelConfig)
	return candidate{
		ID:            agent.ID,
		ProviderID:    agent.Provider,
		Provider:      provider,
		ProviderType:  provider.Type,
		Protocol:      provider.Protocol,
		CLIType:       provider.CLIType,
		ModelID:       modelID,
		ModelConfig:   modelConfig,
		ProviderModel: providerModel,
		AgentID:       agent.ID,
		Agent:         &agent,
	}, nil
}

func candidatesForProvider(providerID string, provider config.ProviderConfig) []candidate {
	modelIDs := sortedModelIDs(provider)
	out := make([]candidate, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		modelConfig := provider.Models[modelID]
		if len(provider.Models) == 0 {
			modelConfig = config.ProviderModelConfig{}
		}
		if !config.IsProviderModelEnabled(modelConfig) {
			continue
		}
		providerModel := providerModelFor(provider, modelID, modelConfig)
		id := providerID
		if modelID != "" {
			id = providerID + "/" + modelID
		}
		out = append(out, candidate{
			ID:            id,
			ProviderID:    providerID,
			Provider:      provider,
			ProviderType:  provider.Type,
			Protocol:      provider.Protocol,
			CLIType:       provider.CLIType,
			ModelID:       modelID,
			ModelConfig:   modelConfig,
			ProviderModel: providerModel,
		})
	}
	return out
}

func probeCandidate(ctx context.Context, item candidate, opts Options, sink ArtifactSink) telemetry.ProviderReadinessEntry {
	startedAt := time.Now()
	entry := telemetry.ProviderReadinessEntry{
		Provider:     item.ProviderID,
		ProviderType: item.ProviderType,
		Protocol:     firstNonEmpty(item.Protocol, item.CLIType),
		Model:        item.ProviderModel,
		BaseURL:      item.Provider.BaseURL,
		APIKeyEnv:    item.Provider.APIKeyEnv,
		Agent:        item.AgentID,
	}
	if item.ModelID != "" && item.ModelID != "default" {
		entry.Model = item.ProviderModel + " (" + item.ModelID + ")"
	}
	switch item.ProviderType {
	case config.ProviderTypeCLI:
		command := item.Provider.Command
		if command == "" {
			command = firstNonEmpty(item.Provider.CLIType, config.CLITypeGeneric)
		}
		entry.Command = append([]string{command}, item.Provider.Args...)
	case config.ProviderTypeAPI:
		entry.Command = []string{item.Protocol, item.ProviderModel, item.Provider.BaseURL}
	default:
		entry.Command = []string{item.ProviderType}
	}

	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		prompt = defaultPrompt
	}
	schema := readinessSchema()
	maxOutputTokens := effectiveMaxOutputTokens(item)
	if item.ProviderType == config.ProviderTypeAPI {
		requestContext, err := apirunner.PreviewRequestContext(
			item.Provider,
			prompt,
			defaultSystemPrompt,
			item.ProviderModel,
			effectiveTemperature(item),
			effectiveReasoning(item),
			maxOutputTokens,
			schema,
		)
		if err != nil {
			requestContext = map[string]any{"previewError": err.Error()}
		}
		annotatePreflightBudget(requestContext, item.ModelConfig.MaxOutputTokens, maxOutputTokens)
		entry.RequestContext = requestContext
	}
	if sink != nil {
		sink.WriteInput(item.ID, map[string]any{
			"provider":       item.ProviderID,
			"providerType":   item.ProviderType,
			"protocol":       item.Protocol,
			"cliType":        item.CLIType,
			"baseUrl":        item.Provider.BaseURL,
			"apiKeyEnv":      item.Provider.APIKeyEnv,
			"agent":          item.AgentID,
			"modelId":        item.ModelID,
			"providerModel":  item.ProviderModel,
			"prompt":         prompt,
			"schema":         schema,
			"requestContext": entry.RequestContext,
		})
	}
	if item.ProviderType == config.ProviderTypeCLI {
		preview, err := previewCLICommand(item, prompt, schema)
		if err != nil {
			entry.Error = err.Error()
			if sink != nil {
				sink.WriteError(item.ID, entry.Error)
			}
			return entry
		}
		entry.Command = preview
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := runCandidate(probeCtx, item, prompt, schema)
	entry.DurationMs = time.Since(startedAt).Milliseconds()
	if len(result.command) > 0 {
		entry.Command = result.command
	}
	if err != nil {
		if probeCtx.Err() == context.DeadlineExceeded {
			entry.Error = fmt.Sprintf("timed out after %s", timeout)
		} else {
			entry.Error = err.Error()
		}
		if sink != nil {
			sink.WriteError(item.ID, entry.Error)
		}
		return entry
	}
	raw := result.raw
	entry.StdoutPreview = previewText(raw, 220)
	if sink != nil {
		sink.WriteRaw(item.ID, raw)
	}
	if strict, strictErr := tilruntime.StrictJSONObjectBytes(raw); strictErr == nil && len(strict) > 0 {
		entry.StrictJSON = true
		entry.RecoverableJSON = true
	} else if _, parseErr := tilruntime.ParseJSONObject(raw); parseErr == nil {
		entry.RecoverableJSON = true
	}
	if !entry.RecoverableJSON {
		entry.Error = "probe succeeded but did not return a recoverable JSON object"
		if sink != nil {
			sink.WriteError(item.ID, entry.Error)
		}
		return entry
	}
	entry.Ready = true
	return entry
}

type candidateRunResult struct {
	raw     string
	command []string
}

func previewCLICommand(item candidate, prompt string, schema map[string]any) ([]string, error) {
	agentID := firstNonEmpty(item.AgentID, "preflight-"+sanitizeID(item.ProviderID))
	role := "preflight"
	if item.Agent != nil && strings.TrimSpace(item.Agent.Role) != "" {
		role = item.Agent.Role
	}
	task := preflightTask{TaskMeta: consensus.TaskMeta{
		RequestID: "preflight",
		SessionID: "preflight",
		AgentID:   agentID,
		Role:      role,
	}}
	return clirunner.PreviewCommand(item.Provider, task, prompt, agentID, role, item.ProviderModel, effectiveReasoning(item), effectiveTemperature(item), schema)
}

func runCandidate(ctx context.Context, item candidate, prompt string, schema map[string]any) (candidateRunResult, error) {
	switch item.ProviderType {
	case config.ProviderTypeAPI:
		if strings.TrimSpace(item.Provider.APIKeyEnv) != "" && strings.TrimSpace(os.Getenv(item.Provider.APIKeyEnv)) == "" {
			return candidateRunResult{}, fmt.Errorf("env %s is not set", item.Provider.APIKeyEnv)
		}
		runner := apirunner.NewRunner(item.Provider)
		raw, err := runner.RunTask(ctx, prompt, defaultSystemPrompt, item.ProviderModel, effectiveTemperature(item), effectiveReasoning(item), effectiveMaxOutputTokens(item), schema)
		return candidateRunResult{raw: raw}, err
	case config.ProviderTypeCLI:
		runner := clirunner.NewRunner(item.Provider)
		agentID := firstNonEmpty(item.AgentID, "preflight-"+sanitizeID(item.ProviderID))
		role := "preflight"
		if item.Agent != nil && strings.TrimSpace(item.Agent.Role) != "" {
			role = item.Agent.Role
		}
		task := preflightTask{TaskMeta: consensus.TaskMeta{
			RequestID: "preflight",
			SessionID: "preflight",
			AgentID:   agentID,
			Role:      role,
		}}
		result, err := runner.RunTaskDetailed(ctx, task, prompt, agentID, role, item.ProviderModel, effectiveReasoning(item), effectiveTemperature(item), schema)
		return candidateRunResult{raw: result.Output, command: result.Command}, err
	case config.ProviderTypeMock:
		body, _ := json.Marshal(map[string]bool{"ok": true})
		return candidateRunResult{raw: string(body)}, nil
	default:
		return candidateRunResult{}, fmt.Errorf("provider type %s is not supported by preflight", item.ProviderType)
	}
}

func annotatePreflightBudget(ctx map[string]any, configured int, effective int) {
	if len(ctx) == 0 || configured <= 0 || configured == effective {
		return
	}
	generation, ok := ctx["generation"].(map[string]any)
	if !ok {
		return
	}
	generation["configuredMaxOutputTokens"] = configured
	generation["budgetPolicy"] = "preflight_cap"
}

func readinessSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"ok"},
		"properties": map[string]any{
			"ok": map[string]any{
				"type": "boolean",
			},
		},
	}
}

func effectiveTemperature(item candidate) *float64 {
	if item.Agent != nil && item.Agent.Temperature != nil {
		return item.Agent.Temperature
	}
	return item.ModelConfig.Temperature
}

func effectiveReasoning(item candidate) string {
	if item.Agent != nil && strings.TrimSpace(item.Agent.Reasoning) != "" {
		return item.Agent.Reasoning
	}
	return item.ModelConfig.Reasoning
}

func effectiveMaxOutputTokens(item candidate) int {
	defaultTokens := 2048
	if item.ModelConfig.MaxOutputTokens > 0 && item.ModelConfig.MaxOutputTokens < defaultTokens {
		return item.ModelConfig.MaxOutputTokens
	}
	return defaultTokens
}

func providerModelFor(provider config.ProviderConfig, modelID string, model config.ProviderModelConfig) string {
	if strings.TrimSpace(model.ProviderModel) != "" {
		return strings.TrimSpace(model.ProviderModel)
	}
	if strings.TrimSpace(modelID) != "" {
		return strings.TrimSpace(modelID)
	}
	return strings.TrimSpace(provider.Model)
}

func sortedProviderIDs(providers map[string]config.ProviderConfig) []string {
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedModelIDs(provider config.ProviderConfig) []string {
	if len(provider.Models) == 0 {
		if strings.TrimSpace(provider.Model) != "" {
			return []string{""}
		}
		return []string{""}
	}
	ids := make([]string, 0, len(provider.Models))
	for id := range provider.Models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func singleModelID(provider config.ProviderConfig) (string, bool) {
	if len(provider.Models) != 1 {
		return "", false
	}
	for id := range provider.Models {
		return id, true
	}
	return "", false
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				out[trimmed] = struct{}{}
			}
		}
	}
	return out
}

func sortedSetKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func previewText(text string, max int) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max] + "..."
}

func sanitizeID(value string) string {
	replacer := strings.NewReplacer("/", "-", " ", "-", ":", "-", "\\", "-", "\t", "-")
	return strings.Trim(replacer.Replace(strings.TrimSpace(value)), "-")
}
