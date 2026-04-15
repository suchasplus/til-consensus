package runtime

import (
	"context"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

type ResolvedAgentRuntime struct {
	config.AgentConfig
	ProviderName  string
	Provider      config.ProviderConfig
	ProviderModel string
}

type ProviderTaskRequest struct {
	Task  consensus.Task
	Agent ResolvedAgentRuntime
}

type ProviderRunner interface {
	RunTask(ctx context.Context, req ProviderTaskRequest) (any, error)
}

type providerRunnerFunc func(context.Context, ProviderTaskRequest) (any, error)

func (f providerRunnerFunc) RunTask(ctx context.Context, req ProviderTaskRequest) (any, error) {
	return f(ctx, req)
}
