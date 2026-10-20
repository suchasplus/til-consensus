package runmanager

import (
	"context"
	"fmt"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

const DefaultChainLimit = 90
const RunCallLimit = 30

func compatiblePolicyVersion(version string) bool {
	return version == model.PolicyVersion || version == "m4-topic-preparation-v1" || version == "m3-cli-v1" || version == "m3-cli-v2-chain-budget"
}

// Include legacy usage keys and new call reservations exactly once. The base
// preserves historical totals when an older snapshot lacks invocation details.
func knownCallIDs(r *model.StoredRun) map[string]bool {
	ids := map[string]bool{}
	for id := range r.CallReservations {
		ids[id] = true
	}
	for id := range r.UsageByInvocation {
		ids[id] = true
	}
	return ids
}
func effectiveRunCalls(r *model.StoredRun) int {
	count := r.CallCountBase + len(knownCallIDs(r))
	if r.Snapshot.Usage.Calls > count {
		count = r.Snapshot.Usage.Calls
	}
	return count
}
func reconcileCallBase(r *model.StoredRun) {
	known := r.CallCountBase + len(knownCallIDs(r))
	if r.Snapshot.Usage.Calls > known {
		r.CallCountBase += r.Snapshot.Usage.Calls - known
	}
}
func rootForRun(r *model.StoredRun, all map[string]*model.StoredRun) string {
	if r.Frozen.ChainRootID != "" {
		return r.Frozen.ChainRootID
	}
	if r.Snapshot.LineageRootID != "" {
		return r.Snapshot.LineageRootID
	}
	current := r
	seen := map[string]bool{}
	for current.Snapshot.ParentRunID != "" && !seen[current.Snapshot.ID] {
		seen[current.Snapshot.ID] = true
		parent := all[current.Snapshot.ParentRunID]
		if parent == nil {
			return current.Snapshot.ParentRunID
		}
		if parent.Frozen.ChainRootID != "" {
			return parent.Frozen.ChainRootID
		}
		if parent.Snapshot.LineageRootID != "" {
			return parent.Snapshot.LineageRootID
		}
		current = parent
	}
	return current.Snapshot.ID
}
func (m *Manager) chainStatusLocked(current *model.StoredRun) (int, int, error) {
	records, err := m.store.All(context.Background())
	if err != nil {
		return 0, 0, err
	}
	all := map[string]*model.StoredRun{}
	for i := range records {
		all[records[i].Snapshot.ID] = &records[i]
	}
	all[current.Snapshot.ID] = current
	rootID := rootForRun(current, all)
	limit := DefaultChainLimit
	if root := all[rootID]; root != nil && root.Frozen.ChainLimit > 0 {
		limit = root.Frozen.ChainLimit
	} else if current.Frozen.ChainLimit > 0 {
		limit = current.Frozen.ChainLimit
	}
	calls := 0
	for _, r := range all {
		if rootForRun(r, all) == rootID {
			calls += effectiveRunCalls(r)
		}
	}
	return calls, limit, nil
}
func (m *Manager) refreshChainSnapshotLocked(r *model.StoredRun) error {
	calls, limit, err := m.chainStatusLocked(r)
	if err != nil {
		return err
	}
	r.Snapshot.ChainCalls = calls
	r.Snapshot.ChainLimit = limit
	r.Snapshot.Usage.Calls = effectiveRunCalls(r)
	return nil
}

// The manager owns the data-directory lease. Its lock serializes aggregation,
// preflight and the durable reservation across every sibling in a root chain.
func (m *Manager) reserveCall(id, attempt, invocation, agent string) error {
	if invocation == "" {
		return fmt.Errorf("CLI 请求缺少 invocationId，不能预留预算")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.store.Load(context.Background(), id)
	if err != nil {
		return err
	}
	if r.Snapshot.AttemptID != attempt {
		return fmt.Errorf("过期 attempt 不能预留新的 CLI 请求")
	}
	if r.StopReason != "" || r.Snapshot.Status == "completed" || r.Snapshot.Status == "failed" || r.Snapshot.Status == "stopped" || r.Snapshot.Status == "interrupted" {
		return fmt.Errorf("讨论已停止，不能发起新的 CLI 请求")
	}
	if _, exists := r.CallReservations[invocation]; exists {
		return nil
	}
	reconcileCallBase(r)
	known := knownCallIDs(r)
	increment := 1
	if known[invocation] {
		increment = 0
	}
	runCalls := effectiveRunCalls(r)
	chainCalls, limit, err := m.chainStatusLocked(r)
	if err != nil {
		return err
	}
	r.Snapshot.ChainCalls = chainCalls
	r.Snapshot.ChainLimit = limit
	if increment > 0 && (runCalls+increment > RunCallLimit || chainCalls+increment > limit) {
		message := fmt.Sprintf("已达到本讨论 %d 次或关联链 %d 次 CLI 请求上限（本讨论 %d，关联链 %d）；不能通过创建分支绕过链预算", RunCallLimit, limit, runCalls, chainCalls)
		if err := m.saveLocked(r, "call.denied", map[string]any{"error": message, "calls": runCalls, "chainCalls": chainCalls, "chainLimit": limit}, "", "", ""); err != nil {
			return err
		}
		return fmt.Errorf("%s", message)
	}
	if r.CallReservations == nil {
		r.CallReservations = map[string]model.CallReservation{}
	}
	r.CallReservations[invocation] = model.CallReservation{AgentID: agent, AttemptID: attempt, At: now()}
	r.Snapshot.Usage.Calls = runCalls + increment
	r.Snapshot.ChainCalls = chainCalls + increment
	return m.saveLocked(r, "call.reserved", map[string]any{"invocationId": invocation, "agentId": agent, "calls": r.Snapshot.Usage.Calls, "chainCalls": r.Snapshot.ChainCalls, "chainLimit": limit}, "", "", "")
}

func (m *Manager) chainRootIDLocked(current *model.StoredRun) (string, error) {
	records, err := m.store.All(context.Background())
	if err != nil {
		return "", err
	}
	all := map[string]*model.StoredRun{}
	for i := range records {
		all[records[i].Snapshot.ID] = &records[i]
	}
	all[current.Snapshot.ID] = current
	return rootForRun(current, all), nil
}
