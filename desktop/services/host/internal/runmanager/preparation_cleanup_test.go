package runmanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func adoptedPreparationForCleanup(t *testing.T) (*Manager, model.PreparationRecord, model.RunSnapshot) {
	t.Helper()
	m := preparationManager(t, t.TempDir(), nil)
	in := prepInput("cleanup-adopted")
	p, err := m.AnalyzeTopic(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	run, err := m.Start(model.StartRunInput{Goal: in.Goal, Mode: "auto", Materials: in.Materials, PreparationID: p.ID, IdempotencyKey: "cleanup-adopt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	await(t, m, run.ID, terminal)
	return m, p, run
}
func TestRunCleanupIncludesOnlyItsAdoptedPreparationDiagnostics(t *testing.T) {
	m, p, run := adoptedPreparationForCleanup(t)
	dir := filepath.Join(m.opt.DataDir, "preparations", p.ID)
	paths, _ := filepath.Glob(filepath.Join(dir, "request-*.json"))
	if len(paths) == 0 {
		t.Fatal("准备诊断未产生")
	}
	if err := os.WriteFile(filepath.Join(dir, "request-private.json"), []byte("PRIVATE-REASONING"), 0600); err != nil {
		t.Fatal(err)
	}
	independent := prepInput("cleanup-independent")
	if _, err := m.AnalyzeTopic(context.Background(), independent); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ledger.jsonl", "result.json", "decision.json"} {
		if err := os.WriteFile(filepath.Join(m.runDir(run.ID), name), []byte("retained"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := m.ClearDiagnostics(run.ID)
	if err != nil || result.RemovedFiles < 2 {
		t.Fatal("没有清理已采用准备的原始IO", result, err)
	}
	remaining, _ := filepath.Glob(filepath.Join(dir, "request-*.json"))
	if len(remaining) != 0 {
		t.Fatal("已采用准备仍留原始诊断")
	}
	independentPaths, _ := filepath.Glob(filepath.Join(m.opt.DataDir, "preparations", independent.ID, "request-*.json"))
	if len(independentPaths) == 0 {
		t.Fatal("误删独立未采用的准备诊断")
	}
	stored, err := m.GetPreparation(p.ID)
	if err != nil || len(stored.Materials) != 1 || stored.Classification == nil || stored.MaterialAnalysis == nil || stored.UsedByRunID != run.ID {
		t.Fatal("结构化准备数据被清理", err)
	}
	for _, name := range []string{"ledger.jsonl", "result.json", "decision.json"} {
		if _, err := os.Stat(filepath.Join(m.runDir(run.ID), name)); err != nil {
			t.Fatal("误删结果/账本", name, err)
		}
	}
	record, _ := m.stored(run.ID)
	if err := validateFrozen(record); err != nil {
		t.Fatal("清理改变冻结快照", err)
	}
}
func TestPreparationCleanupDoesNotFollowDirectorySymlink(t *testing.T) {
	m, p, run := adoptedPreparationForCleanup(t)
	dir := filepath.Join(m.opt.DataDir, "preparations", p.ID)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	secret := filepath.Join(outside, "request-private.json")
	if err := os.WriteFile(secret, []byte("must remain"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ClearDiagnostics(run.ID); err == nil {
		t.Fatal("准备诊断目录symlink未拒绝")
	}
	if _, err := os.Stat(secret); err != nil {
		t.Fatal("跟随链接删除了其他目录文件", err)
	}
}
