package runtime

import (
	"context"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
)

type ResolvedAgentRuntime struct {
	config.AgentConfig
	ProviderName  string
	Provider      config.ProviderConfig
	ModelConfig   config.ProviderModelConfig
	ProviderModel string
}

func (r ResolvedAgentRuntime) EffectiveTemperature() *float64 {
	if r.Temperature != nil {
		return r.Temperature
	}
	return r.ModelConfig.Temperature
}

func (r ResolvedAgentRuntime) EffectiveReasoning() string {
	if r.Reasoning != "" {
		return r.Reasoning
	}
	return r.ModelConfig.Reasoning
}

type ProviderTaskRequest struct {
	Task           consensus.Task
	Agent          ResolvedAgentRuntime
	PromptOverride string
	OutputSchema   map[string]any
}

type ProviderRunner interface {
	RunTask(ctx context.Context, req ProviderTaskRequest) (any, error)
}

type providerRunnerFunc func(context.Context, ProviderTaskRequest) (any, error)

func (f providerRunnerFunc) RunTask(ctx context.Context, req ProviderTaskRequest) (any, error) {
	return f(ctx, req)
}
