package runmanager

import (
	"context"
	"fmt"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func runtimeConfigChanged(reason string) bool {
	return reason == "version_changed" || reason == "model_changed"
}

// CLI selectors and reported canonical models are distinct identifiers. Bind the
// first validated response's canonical identity without changing frozen config.
func (m *Manager) enforceReportedModel(id, attempt, agent, invocation, reported, source string, completed bool) error {
	if source != "cli-report" || reported == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.store.Load(context.Background(), id)
	if err != nil {
		return err
	}
	if r.Frozen.ProviderMode != "cli" {
		return nil
	}
	if runtimeConfigChanged(r.StopReason) {
		return fmt.Errorf("%s", r.Snapshot.Error)
	}
	if r.Snapshot.AttemptID != attempt {
		return fmt.Errorf("过期 attempt 不能绑定当前模型身份")
	}
	requested := ""
	for _, p := range r.Frozen.Providers {
		if p.ID == agent {
			requested = p.Model
			break
		}
	}
	if binding, ok := r.ModelBindings[agent]; ok {
		if normalizeModel(binding.Model) == normalizeModel(reported) {
			return nil
		}
		message := fmt.Sprintf("CLI %s 报告的模型由 %q 变为 %q（冻结选择ID：%q），已停止讨论；请新建关联讨论", agent, binding.Model, reported, requested)
		r.StopReason = "model_changed"
		r.Snapshot.Status = "failed"
		r.Snapshot.Phase = "failed"
		r.Snapshot.HostPhase = "failed"
		r.Snapshot.Error = message
		for i := range r.Snapshot.Messages {
			r.Snapshot.Messages[i].Streaming = false
		}
		if err := m.saveLocked(r, "run.model_changed", map[string]any{"agentId": agent, "requestedModel": requested, "expectedReportedModel": binding.Model, "actualReportedModel": reported, "invocationId": invocation}, "", "", ""); err != nil {
			return err
		}
		if cancel := m.active[id]; cancel != nil {
			cancel()
		}
		return fmt.Errorf("%s", message)
	}
	if !completed {
		return nil
	}
	if r.ModelBindings == nil {
		r.ModelBindings = map[string]model.ModelBinding{}
	}
	r.ModelBindings[agent] = model.ModelBinding{Model: reported, RequestedModel: requested, InvocationID: invocation, BoundAt: now()}
	return m.saveLocked(r, "model.bound", map[string]any{"agentId": agent, "requestedModel": requested, "reportedModel": reported, "invocationId": invocation}, "", "", "")
}
