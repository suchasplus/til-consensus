package runmanager

import (
	"context"
	"fmt"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func deletedRunError() error {
	return fmt.Errorf("讨论已移至最近删除，请先恢复；新建讨论请使用新的请求标识")
}

// ListDeleted is a view over the same durable records as List. The storage
// layer deliberately retains every record for lineage and chain accounting.
func (m *Manager) ListDeleted() ([]model.RunSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runs, err := m.store.All(context.Background())
	if err != nil {
		return nil, err
	}
	out := []model.RunSummary{}
	for _, r := range runs {
		if r.Snapshot.DeletedAt != "" {
			out = append(out, r.Snapshot.RunSummary)
		}
	}
	return out, nil
}

// DeleteRun hides an idle discussion without removing evidence, checkpoints,
// request identities, or its contribution to the shared CLI-call budget.
func (m *Manager) DeleteRun(id string) (model.RunSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return model.RunSnapshot{}, fmt.Errorf("后台正在关闭")
	}
	r, err := m.store.Load(context.Background(), id)
	if err != nil {
		return model.RunSnapshot{}, err
	}
	// The active lease remains present until execution and subprocess cleanup
	// finish, even if a failure has already written a terminal-looking status.
	if _, active := m.active[id]; active || r.Snapshot.Status == "running" || r.Snapshot.Status == "queued" {
		return model.RunSnapshot{}, fmt.Errorf("请先停止讨论，等待 CLI 完全退出后再删除")
	}
	if err := m.refreshChainSnapshotLocked(r); err != nil {
		return model.RunSnapshot{}, err
	}
	if r.Snapshot.DeletedAt != "" {
		return r.Snapshot, nil
	}
	r.Snapshot.DeletedAt = now()
	if err := m.saveLocked(r, "run.deleted", map[string]any{"deletedAt": r.Snapshot.DeletedAt}, "", "", ""); err != nil {
		return model.RunSnapshot{}, err
	}
	return r.Snapshot, nil
}

// RestoreRun changes visibility only. It never starts a model request or
// replaces frozen settings with current application settings.
func (m *Manager) RestoreRun(id string) (model.RunSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return model.RunSnapshot{}, fmt.Errorf("后台正在关闭")
	}
	r, err := m.store.Load(context.Background(), id)
	if err != nil {
		return model.RunSnapshot{}, err
	}
	if err := m.refreshChainSnapshotLocked(r); err != nil {
		return model.RunSnapshot{}, err
	}
	if r.Snapshot.DeletedAt == "" {
		return r.Snapshot, nil
	}
	previous := r.Snapshot.DeletedAt
	r.Snapshot.DeletedAt = ""
	if err := m.saveLocked(r, "run.restored", map[string]any{"previousDeletedAt": previous}, "", "", ""); err != nil {
		return model.RunSnapshot{}, err
	}
	return r.Snapshot, nil
}
