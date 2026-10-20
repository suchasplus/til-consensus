package runmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/delegate"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func upsertMessage(s *model.RunSnapshot, message model.ChatMessage) {
	for i, old := range s.Messages {
		if old.ID == message.ID {
			message.At = old.At
			s.Messages[i] = message
			return
		}
	}
	s.Messages = append(s.Messages, message)
}
func (m *Manager) progress(id, attempt string, e delegate.ProgressEvent) error {
	if err := m.enforceCLIVersion(id, attempt, e.AgentID, e.CLIVersion, e.Kind == "started"); err != nil {
		return err
	}
	if err := m.enforceReportedModel(id, attempt, e.AgentID, e.InvocationID, e.Model, e.ModelSource, e.Kind == "completed" && e.Complete); err != nil {
		return err
	}
	if e.Kind == "started" {
		if err := m.reserveCall(id, attempt, e.InvocationID, e.AgentID); err != nil {
			return err
		}
	}
	eventType := "provider.progress"
	if e.Delta != "" {
		eventType = "message.delta"
	}
	if e.Usage != nil {
		eventType = "usage.updated"
	}
	return m.update(id, eventType, func(r *model.StoredRun) error {
		if r.Snapshot.Proposal == nil && e.AgentID == workflow.RolePlanFor(r.Frozen).Proposer && e.TaskKind == "propose" && e.InvocationID != "" {
			r.P0InvocationID = e.InvocationID
			r.P0AttemptID = attempt
		}
		if e.Kind != "failed" || r.ActualMetadata[e.InvocationID] != nil {
			recordUsage(r, e.InvocationID, e.AgentID, e.Model, e.CLIVersion, e.Usage, nil, e.ModelSource)
			if e.ReportedReasoning != "" {
				r.ActualMetadata[e.InvocationID]["reportedReasoning"] = e.ReportedReasoning
			}
		}
		if e.Delta != "" || e.Text != "" {
			agent, label := e.AgentID, e.AgentID
			if r.Snapshot.Mode == "delphi" {
				agent, label = workflow.AnonymousIdentity(workflow.RolePlanFor(r.Frozen), agent)
			}
			message := model.ChatMessage{ID: "message-" + e.InvocationID, AgentID: agent, Role: "assistant", Label: label, At: now(), Phase: e.Phase, AttemptID: attempt, Kind: messageKind(e.TaskKind), InvocationID: e.InvocationID, Round: e.Round, Streaming: !e.Complete}
			for _, old := range r.Snapshot.Messages {
				if old.ID == message.ID {
					message.Content = old.Content
					break
				}
			}
			if e.Delta != "" {
				message.Content += e.Delta
			} else {
				message.Content = e.Text
			}
			upsertMessage(&r.Snapshot, message)
		}
		if e.Complete {
			for i := range r.Snapshot.Messages {
				if r.Snapshot.Messages[i].InvocationID == e.InvocationID {
					r.Snapshot.Messages[i].Streaming = false
				}
			}
		}
		return nil
	}, map[string]any{"invocationId": e.InvocationID, "agentId": e.AgentID, "phase": e.Phase, "kind": e.Kind, "delta": e.Delta, "status": e.Status, "model": e.Model, "round": e.Round})
}
func recordUsage(r *model.StoredRun, invocation, agent, actualModel, version string, values map[string]any, cost *float64, modelSource string) {
	if invocation == "" {
		return
	}
	if r.ActualMetadata == nil {
		r.ActualMetadata = map[string]map[string]any{}
	}
	meta := r.ActualMetadata[invocation]
	if meta == nil {
		meta = map[string]any{"agentId": agent, "invocationId": invocation}
	}
	if actualModel != "" {
		meta["model"] = actualModel
	}
	for _, provider := range r.Frozen.Providers {
		if provider.ID == agent {
			meta["requestedModel"] = provider.Model
			meta["requestedReasoning"] = provider.Reasoning
			break
		}
	}
	if actualModel != "" && modelSource == "cli-report" {
		meta["reportedModel"] = actualModel
		meta["actualModel"] = actualModel
	}
	if modelSource != "" {
		meta["modelSource"] = modelSource
	}
	if version != "" {
		meta["cliVersion"] = version
	}
	if values != nil {
		meta["usage"] = values
	}
	r.ActualMetadata[invocation] = meta
	if r.UsageByInvocation == nil {
		r.UsageByInvocation = map[string]model.Usage{}
	}
	current := r.UsageByInvocation[invocation]
	// Provider usage events are cumulative snapshots. Never sum the same call twice.
	if values != nil {
		flat := flattenUsage(values)
		if n := usageNumber(flat, "input_tokens", "inputtokens", "prompt_tokens", "prompttokencount"); n > 0 {
			current.InputTokens = n
		}
		if n := usageNumber(flat, "output_tokens", "outputtokens", "completion_tokens", "candidatestokencount"); n > 0 {
			current.OutputTokens = n
		}
		if n := usageNumber(flat, "cached_input_tokens", "cache_read_input_tokens", "cached_tokens", "cachedcontenttokencount"); n > 0 {
			current.CachedTokens = n
		}
		current.Source = "cli"
	}
	if cost != nil {
		current.CostUSD = *cost
	}
	r.UsageByInvocation[invocation] = current
	total := model.Usage{Calls: effectiveRunCalls(r)}
	for _, u := range r.UsageByInvocation {
		total.InputTokens += u.InputTokens
		total.OutputTokens += u.OutputTokens
		total.CachedTokens += u.CachedTokens
		total.CostUSD += u.CostUSD
		if u.Source != "" {
			total.Source = u.Source
		}
	}
	r.Snapshot.Usage = total
}
func flattenUsage(values map[string]any) map[string]any {
	out := map[string]any{}
	var visit func(map[string]any)
	visit = func(m map[string]any) {
		for k, v := range m {
			if child, ok := v.(map[string]any); ok {
				visit(child)
			} else {
				out[strings.ToLower(k)] = v
			}
		}
	}
	visit(values)
	return out
}
func usageNumber(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		switch value := m[k].(type) {
		case float64:
			return int64(value)
		case int:
			return int64(value)
		case int64:
			return value
		case json.Number:
			n, _ := value.Int64()
			return n
		}
	}
	return 0
}

// A running CLI installation may auto-update between task dispatches. Refuse
// to mix it into a frozen run, including terminal metadata arriving later.
func (m *Manager) enforceCLIVersion(id, attempt, agent, actual string, required bool) error {
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
		return fmt.Errorf("过期 attempt 的 CLI 事件不能修改当前讨论")
	}
	if actual == "" && !required {
		return nil
	}
	expected := ""
	for _, p := range r.Frozen.Providers {
		if p.ID == agent {
			expected = p.Version
			break
		}
	}
	if expected != "" && actual == expected {
		return nil
	}
	message := fmt.Sprintf("CLI %s 版本与冻结配置不一致（冻结：%q，实际：%q），已停止讨论；请新建关联讨论", agent, expected, actual)
	r.StopReason = "version_changed"
	r.Snapshot.Status = "failed"
	r.Snapshot.HostPhase = "failed"
	r.Snapshot.Phase = "failed"
	r.Snapshot.Error = message
	for i := range r.Snapshot.Messages {
		r.Snapshot.Messages[i].Streaming = false
	}
	if err := m.saveLocked(r, "run.version_changed", map[string]any{"agentId": agent, "expectedVersion": expected, "actualVersion": actual}, "", "", ""); err != nil {
		return err
	}
	if cancel := m.active[id]; cancel != nil {
		cancel()
	}
	return fmt.Errorf("%s", message)
}
