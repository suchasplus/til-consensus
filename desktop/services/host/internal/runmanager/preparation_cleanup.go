package runmanager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func (m *Manager) clearAdoptedPreparationDiagnosticsLocked(run *model.StoredRun) (int, error) {
	if run.Snapshot.Preparation == nil {
		return 0, nil
	}
	id := run.Snapshot.Preparation.ID
	if !preparationIDPattern.MatchString(id) {
		return 0, fmt.Errorf("已采用准备ID无效，未删除其诊断")
	}
	preparation, err := m.store.GetPreparation(context.Background(), id)
	if err != nil {
		return 0, err
	}
	if preparation.UsedByRunID != run.Snapshot.ID {
		return 0, fmt.Errorf("准备分析不属于当前讨论，未删除其诊断")
	}
	parent := filepath.Join(m.opt.DataDir, "preparations")
	directory := filepath.Join(parent, id)
	for _, path := range []string{parent, directory} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return 0, fmt.Errorf("准备诊断路径不是安全本机目录，未跟随链接")
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasPrefix(entry.Name(), "request-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
