package runmanager

import (
	"context"
	"fmt"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func cleanProviderEnv(t *testing.T) {
	t.Helper()
	for _, id := range []string{"CODEX", "CLAUDE", "GROK", "AGY"} {
		for _, suffix := range []string{"MODEL", "REASONING", "ENABLED"} {
			key := "TCVIEW_" + id + "_" + suffix
			t.Setenv(key, "")
			_ = os.Unsetenv(key)
		}
	}
}
func settingByID(t *testing.T, all []ProviderInfo, id string) ProviderInfo {
	t.Helper()
	for _, v := range all {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("missing %s", id)
	return ProviderInfo{}
}
func TestSavedChoicesPriorityPersistenceAndFrozenRuns(t *testing.T) {
	cleanProviderEnv(t)
	dir := t.TempDir()
	m, err := New(Options{DataDir: dir, ProviderMode: "mock", Delay: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	configs := providers.ApplicationDefaults()
	for i := range configs {
		if configs[i].ID == "grok" {
			configs[i].Model = "grok-4.6"
			configs[i].Reasoning = "xhigh"
		}
	}
	views, err := m.SaveProviderSettings(configs)
	if err != nil {
		t.Fatal(err)
	}
	view := settingByID(t, views, "grok")
	if view.Saved == nil || view.Saved.Model != "grok-4.6" || view.Effective.Reasoning != "xhigh" || view.Sources["model"].Kind != "saved" {
		t.Fatalf("%#v", view)
	}
	run, err := m.Start(model.StartRunInput{Goal: "冻结配置回归", Mode: "free_debate", IdempotencyKey: "frozen-before-save"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	for i := range configs {
		if configs[i].ID == "grok" {
			configs[i].Model = "grok-4.5"
			configs[i].Reasoning = "medium"
		}
	}
	if _, err = m.SaveProviderSettings(configs); err != nil {
		t.Fatal(err)
	}
	r, err := m.stored(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	old := false
	for _, p := range r.Frozen.Providers {
		if p.ID == "grok" {
			old = p.Model == "grok-4.6" && p.Reasoning == "xhigh" && p.Sources["model"].Kind == "saved"
		}
	}
	if !old {
		t.Fatal("saving rewrote frozen discussion")
	}
	if err = validateFrozen(r); err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = New(Options{DataDir: dir, ProviderMode: "mock", Delay: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	views, err = m.ProviderSettings()
	if err != nil {
		t.Fatal(err)
	}
	view = settingByID(t, views, "grok")
	if view.Effective.Model != "grok-4.5" || view.Effective.Reasoning != "medium" {
		t.Fatalf("restart lost settings: %#v", view)
	}
	resumed, err := m.Resume(model.ResumeRunInput{RunID: run.ID, IdempotencyKey: "resume-frozen"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Stop(resumed.ID); err != nil {
		t.Fatal(err)
	}
	r, err = m.stored(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range r.Frozen.Providers {
		if p.ID == "grok" && (p.Model != "grok-4.6" || p.Reasoning != "xhigh") {
			t.Fatal("resume switched saved choices")
		}
	}
	t.Setenv("TCVIEW_GROK_MODEL", "grok-4.7")
	t.Setenv("TCVIEW_GROK_REASONING", "low")
	views, err = m.ProviderSettings()
	if err != nil {
		t.Fatal(err)
	}
	view = settingByID(t, views, "grok")
	if view.Saved.Model != "grok-4.5" || view.Effective.Model != "grok-4.7" || view.Sources["reasoning"].Env != "TCVIEW_GROK_REASONING" {
		t.Fatalf("%#v", view)
	}
	if _, err = m.ResetProviderSettings(); err != nil {
		t.Fatal(err)
	}
	views, _ = m.ProviderSettings()
	view = settingByID(t, views, "grok")
	if view.Saved != nil || view.Sources["model"].Kind != "environment" {
		t.Fatal("reset deleted environment override or retained saved")
	}
}
func TestPreviewDoesNotSaveOrMislabelCurrentEffectiveConfig(t *testing.T) {
	cleanProviderEnv(t)
	m, err := New(Options{DataDir: t.TempDir(), ProviderMode: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	t.Setenv("TCVIEW_GROK_REASONING", "low")
	view, err := m.CheckProviderPreviewContext(context.Background(), "grok", &providers.Config{ID: "grok", Model: "grok-4.6", Reasoning: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if !view.RequestOK || view.CheckedConfiguration != "editing" || view.RequestedModel != "grok-4.6" || view.RequestedReasoning != "low" || view.Effective.Model != "grok-4.7" || view.Saved != nil || view.ReportedReasoning != "" {
		t.Fatalf("%#v", view)
	}
	all, _ := m.ProviderSettings()
	if settingByID(t, all, "grok").Saved != nil {
		t.Fatal("checking saved edits")
	}
	failed, err := m.CheckProviderPreviewContext(context.Background(), "grok", &providers.Config{ID: "grok", Model: "grok-4.5", Reasoning: "xhigh"})
	if err != nil {
		t.Fatal(err)
	}
	// The environment makes low the real request, so this combination is valid.
	if !failed.RequestOK || failed.RequestedReasoning != "low" {
		t.Fatalf("%#v", failed)
	}
	_ = os.Unsetenv("TCVIEW_GROK_REASONING")
	failed, err = m.CheckProviderPreviewContext(context.Background(), "grok", &providers.Config{ID: "grok", Model: "grok-4.5", Reasoning: "xhigh"})
	if err != nil || failed.RequestOK || failed.Status != "error" || failed.Error == "" {
		t.Fatalf("invalid effort silently accepted: %#v %v", failed, err)
	}
}

// Explicit substitute verifies request de-duplication without consuming model quota.
func TestProviderChecksDeduplicateAndRejectConcurrentDifferentConfig(t *testing.T) {
	cleanProviderEnv(t)
	binaries := t.TempDir()
	if err := os.WriteFile(filepath.Join(binaries, "grok"), []byte("#!/bin/sh\nprintf '%s\\n' 'grok-test-version'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binaries+":/usr/bin:/bin")
	entered, release := make(chan struct{}), make(chan struct{})
	var count atomic.Int32
	m, err := New(Options{DataDir: t.TempDir(), ProviderMode: "cli", CheckCLI: func(ctx context.Context, c providers.Config) (providers.CheckResult, error) {
		if count.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
		case <-ctx.Done():
			return providers.CheckResult{}, ctx.Err()
		}
		return providers.CheckResult{RequestAttempted: true, Provider: providers.Info{ID: c.ID, CLIType: c.ID, Path: filepath.Join(binaries, "grok"), Version: "grok-test-version", Available: true}, OK: true, RequestedModel: c.Model, ActualModel: "grok-canonical-fixture", ModelSource: "cli-report"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	type response struct {
		view ProviderInfo
		err  error
	}
	// 进入替身前仍会执行允许 5 秒的版本探测；race/繁忙磁盘下不能只等 1 秒。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first, second := make(chan response, 1), make(chan response, 1)
	go func() { v, e := m.CheckProviderContext(ctx, "grok"); first <- response{v, e} }()
	select {
	case <-entered:
	case r := <-first:
		t.Fatalf("check returned before entering substitute: %#v %v", r.view, r.err)
	case <-ctx.Done():
		t.Fatalf("check did not start: %v", ctx.Err())
	}
	go func() { v, e := m.CheckProviderContext(ctx, "grok"); second <- response{v, e} }()
	_, err = m.CheckProviderPreviewContext(ctx, "grok", &providers.Config{ID: "grok", Model: "grok-4.6", Reasoning: "low"})
	if err == nil {
		t.Fatal("concurrent different configuration dispatched")
	}
	time.Sleep(15 * time.Millisecond)
	if count.Load() != 1 {
		t.Fatal("duplicate native request")
	}
	close(release)
	for _, ch := range []chan response{first, second} {
		var r response
		select {
		case r = <-ch:
		case <-ctx.Done():
			t.Fatalf("check did not finish: %v", ctx.Err())
		}
		if r.err != nil || !r.view.RequestOK || !r.view.RequestAttempted || r.view.ReportedModel != "grok-canonical-fixture" || r.view.Model != "grok-4.7" {
			t.Fatalf("%#v %v", r.view, r.err)
		}
	}
}

func TestFailedDirectoryReloadPreservesSuccessfulProvenance(t *testing.T) {
	var calls int
	m, err := New(Options{DataDir: t.TempDir(), ProviderMode: "cli", DiscoverModels: func(ctx context.Context, c providers.Config) (providers.Catalog, error) {
		calls++
		if calls == 1 {
			return providers.Catalog{ID: c.ID, Models: []providers.ModelOption{{ID: "gemini-3.8-flash-medium", Verified: true}}, Source: "cli-directory", CLIVersion: "AGY 1.4.2", LoadedAt: "2026-01-01T00:00:00Z", ManualAllowed: true}, nil
		}
		return providers.Catalog{ID: c.ID, LoadedAt: "2026-10-20T12:00:00Z", Error: "目录接口暂时不可用", CLIVersion: "AGY 1.4.2"}, fmt.Errorf("目录接口暂时不可用")
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	initial, err := m.ReloadProviderCatalog(context.Background(), "agy")
	if err != nil {
		t.Fatal(err)
	}
	cached, err := m.ReloadProviderCatalog(context.Background(), "agy")
	if err != nil {
		t.Fatal(err)
	}
	if cached.Source != "cli-directory-cached" || cached.LoadedAt != initial.LoadedAt || cached.CLIVersion != initial.CLIVersion || cached.Error != "目录接口暂时不可用" || len(cached.Models) != 1 {
		t.Fatalf("cache misrepresented failed refresh: %#v", cached)
	}
	stored := m.cachedCatalog("agy")
	if stored == nil || stored.LoadedAt != initial.LoadedAt {
		t.Fatal("failed attempt replaced successful cache timestamp")
	}
}
