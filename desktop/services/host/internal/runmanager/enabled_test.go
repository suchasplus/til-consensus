package runmanager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

func setEnabled(t *testing.T, m *Manager, disabled ...string) []providers.Config {
	t.Helper()
	configs := providers.ApplicationDefaults()
	for i := range configs {
		for _, id := range disabled {
			if configs[i].ID == id {
				configs[i].Enabled = providers.Bool(false)
			}
		}
	}
	if _, err := m.SaveProviderSettings(configs); err != nil {
		t.Fatal(err)
	}
	return configs
}

// All model calls here use the explicitly selected Mock delegate. The real til-consensus engine still schedules all three workflows.
func TestDisabledCLIUsesFrozenDynamicRolesInThreeModes(t *testing.T) {
	cleanProviderEnv(t)
	for _, disabled := range []string{"codex", "claude", "grok", "agy"} {
		for _, mode := range []string{"free_debate", "adjudication", "delphi"} {
			t.Run(disabled+"/"+mode, func(t *testing.T) {
				m := manager(t, t.TempDir(), time.Millisecond)
				setEnabled(t, m, disabled)
				run, err := m.Start(model.StartRunInput{Goal: "显式 Mock：验证可选择参与者的独立讨论", Mode: mode, IdempotencyKey: "enabled-" + disabled + mode})
				if err != nil {
					t.Fatal(err)
				}
				finished := await(t, m, run.ID, terminal)
				if finished.Status != "completed" {
					t.Fatalf("%s: %s", finished.Status, finished.Error)
				}
				r, err := m.stored(run.ID)
				if err != nil {
					t.Fatal(err)
				}
				plan := workflow.RolePlanFor(r.Frozen)
				if len(r.Frozen.Providers) != 3 || len(plan.Reviewers) != 2 || plan.Proposer == disabled || r.Request == nil || r.Request.Roles.Proposers[0] != plan.Proposer || r.Request.DelphiPolicy.MinResponsesPerStatement != 2 {
					t.Fatalf("wrong role freeze: %#v", r.Frozen)
				}
				for _, p := range r.Frozen.Providers {
					if p.ID == disabled {
						t.Fatal("disabled CLI entered freeze")
					}
				}
				for _, meta := range r.ActualMetadata {
					if meta["agentId"] == disabled {
						t.Fatal("disabled CLI was called")
					}
				}
				for _, reservation := range r.CallReservations {
					if reservation.AgentID == disabled {
						t.Fatal("disabled CLI consumed budget")
					}
				}
				p0 := false
				for _, msg := range finished.Messages {
					if msg.Kind == "proposal" && msg.Phase == "preparing" {
						if msg.AgentID != plan.Proposer {
							t.Fatal("P0 used old proposer")
						}
						p0 = true
					}
				}
				if !p0 {
					t.Fatal("missing dynamic P0 message")
				}
				if err := validateFrozen(r); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestEnabledSettingsPersistMinimumGateAndResumeFrozen(t *testing.T) {
	cleanProviderEnv(t)
	dir := t.TempDir()
	m, err := New(Options{DataDir: dir, ProviderMode: "mock", Delay: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	setEnabled(t, m, "codex")
	run, err := m.Start(model.StartRunInput{Goal: "显式 Mock：冻结参与者", Mode: "free_debate", IdempotencyKey: "old-frozen-participants"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	before, err := m.stored(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	setEnabled(t, m, "codex", "claude", "grok", "agy")
	for _, mode := range []string{"free_debate", "adjudication", "delphi"} {
		if _, err = m.Start(model.StartRunInput{Goal: "少于三家应在调度前拒绝", Mode: mode, IdempotencyKey: "too-few-" + mode}); err == nil || !strings.Contains(err.Error(), "至少") {
			t.Fatalf("minimum gate: %v", err)
		}
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = New(Options{DataDir: dir, ProviderMode: "mock", Delay: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	views, err := m.ProviderSettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.Enabled || providers.IsEnabled(v.Effective) || v.Saved == nil || v.Sources["enabled"].Kind != "saved" {
			t.Fatalf("lost disabled persistence: %#v", v)
		}
	}
	resumed, err := m.Resume(model.ResumeRunInput{RunID: run.ID, IdempotencyKey: "old-resume-after-disable"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Stop(resumed.ID); err != nil {
		t.Fatal(err)
	}
	after, err := m.stored(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Frozen, after.Frozen) || before.ConfigHash != after.ConfigHash {
		t.Fatal("saved enable flags rewrote frozen configuration")
	}
	t.Setenv("TCVIEW_GROK_ENABLED", "true")
	views, err = m.ProviderSettings()
	if err != nil {
		t.Fatal(err)
	}
	grok := settingByID(t, views, "grok")
	if !grok.Enabled || providers.IsEnabled(*grok.Saved) || grok.Sources["enabled"].Env != "TCVIEW_GROK_ENABLED" {
		t.Fatalf("enable priority: %#v", grok)
	}
	if _, err = m.ResetProviderSettings(); err != nil {
		t.Fatal(err)
	}
	views, err = m.ProviderSettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if !v.Enabled || v.Saved != nil {
			t.Fatal("reset did not restore default participation")
		}
	}
}

// The shell scripts below are explicit version-probe fixtures; no CLI model is called.
func TestDisabledUnavailableCLIIsNeverProbedAtFreeze(t *testing.T) {
	cleanProviderEnv(t)
	binaries := t.TempDir()
	marker := filepath.Join(binaries, "disabled-probed")
	for _, id := range []string{"codex", "claude", "grok", "agy"} {
		script := "#!/bin/sh\nprintf '%s\\n' 'fixture-version-1'\n"
		if id == "codex" {
			script = "#!/bin/sh\nprintf touched > '" + marker + "'\nexit 42\n"
		}
		if err := os.WriteFile(filepath.Join(binaries, id), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binaries+":/usr/bin:/bin")
	m, err := New(Options{DataDir: t.TempDir(), ProviderMode: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	// Save directly avoids the deliberate settings-page inventory probe; freeze is the behavior under test.
	settings := model.CLISettings{Version: 1}
	for _, c := range providers.ApplicationDefaults() {
		settings.Providers = append(settings.Providers, model.CLIProviderSetting{ID: c.ID, Enabled: providers.Bool(c.ID != "codex"), Model: c.Model, Reasoning: c.Reasoning})
	}
	if err = m.store.SaveProviderSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	frozen, err := m.freezeProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(frozen) != 3 {
		t.Fatal("wrong enabled count")
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("disabled CLI was probed")
	}
	if err = verifyProviderVersions(model.FrozenConfig{ProviderMode: "cli", Providers: frozen}); err != nil {
		t.Fatal(err)
	}
	settings.Providers[0].Enabled = providers.Bool(true)
	if err = m.store.SaveProviderSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	if _, err = m.freezeProviders(); err == nil || !strings.Contains(err.Error(), "codex") {
		t.Fatal("enabled failed CLI silently removed")
	}
}

func TestDisabledCapabilityDoesNotPreventSavingAndPreparationSelection(t *testing.T) {
	cleanProviderEnv(t)
	var calls atomic.Int32
	m := preparationManager(t, t.TempDir(), func(ctx context.Context, r model.PreparationRecord, c providers.Config, mode string, delay time.Duration) (model.PreparationRecord, error) {
		calls.Add(1)
		return preparedFixture(ctx, r, c, mode, delay)
	})
	configs := setEnabled(t, m, "agy")
	for i := range configs {
		if configs[i].ID == "agy" {
			configs[i].Reasoning = "unconfirmed-but-safe"
		}
	}
	if _, err := m.SaveProviderSettings(configs); err != nil {
		t.Fatal("disabled capability blocked saving:", err)
	}
	in := prepInput("default-enabled-analysis")
	r, err := m.AnalyzeTopic(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if r.Source.ProviderID != "codex" || calls.Load() != 1 {
		t.Fatalf("wrong preparation default: %#v", r.Source)
	}
	disabled := prepInput("explicit-disabled-analysis")
	disabled.ProviderID = "agy"
	if _, err = m.AnalyzeTopic(context.Background(), disabled); err == nil || calls.Load() != 1 {
		t.Fatal("explicit disabled analysis was dispatched")
	}
	setEnabled(t, m, "codex")
	if _, err = m.Start(model.StartRunInput{Goal: in.Goal, Materials: in.Materials, Mode: "auto", PreparationID: r.ID, IdempotencyKey: "disabled-source"}); err == nil || !strings.Contains(err.Error(), "停用") {
		t.Fatalf("disabled preparation source adopted: %v", err)
	}
	stored, err := m.store.GetPreparation(context.Background(), r.ID)
	if err != nil || stored.UsedByRunID != "" {
		t.Fatal("rejected preparation was consumed")
	}
	for i := range configs {
		if configs[i].ID == "agy" {
			configs[i].Model = "$(invalid)"
		}
	}
	if _, err = m.SaveProviderSettings(configs); err == nil {
		t.Fatal("disabled syntax bypass")
	}
}

func TestRoleFreezeAndSnapshotTamperIsRejected(t *testing.T) {
	cleanProviderEnv(t)
	m := manager(t, t.TempDir(), time.Hour)
	setEnabled(t, m, "codex")
	run, err := m.Start(model.StartRunInput{Goal: "角色冻结回归", Mode: "free_debate", IdempotencyKey: "role-tamper"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	r, err := m.stored(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.Snapshot.RolePlan.Proposer = "grok"
	if err = validateFrozen(r); err == nil {
		t.Fatal("snapshot role rewrite accepted")
	}
	// A consistent public snapshot cannot replace the frozen plan either, even if an attacker recomputes the outer hash.
	r.Frozen.RolePlan = &model.RolePlan{Proposer: "grok", Reviewers: []string{"claude", "agy"}}
	r.Snapshot.RolePlan = r.Frozen.RolePlan
	body, _ := json.Marshal(r.Frozen)
	r.ConfigHash = workflow.Hash(string(body))
	if err = validateFrozen(r); err == nil {
		t.Fatal("role plan inconsistent with frozen provider order accepted")
	}
}

func TestLegacyM4FrozenRolesSurviveAllCurrentProvidersDisabled(t *testing.T) {
	cleanProviderEnv(t)
	dir := t.TempDir()
	m, err := New(Options{DataDir: dir, ProviderMode: "mock", Delay: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	run, err := m.Start(model.StartRunInput{Goal: "旧 m4 记录兼容（显式 Mock）", Mode: "free_debate", IdempotencyKey: "legacy-m4"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	var legacyHash string
	if err = m.update(run.ID, "fixture.legacy-m4", func(r *model.StoredRun) error {
		r.Frozen.PolicyVersion = "m4-topic-preparation-v1"
		r.Frozen.RolePlan = nil
		r.Snapshot.RolePlan = nil
		for i := range r.Frozen.Providers {
			r.Frozen.Providers[i].Enabled = nil
			delete(r.Frozen.Providers[i].Sources, "enabled")
		}
		r.Snapshot.Providers = r.Frozen.Providers
		body, _ := json.Marshal(r.Frozen)
		r.ConfigHash = workflow.Hash(string(body))
		legacyHash = r.ConfigHash
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	setEnabled(t, m, "codex", "claude", "grok", "agy")
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = New(Options{DataDir: dir, ProviderMode: "mock", Delay: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r, err := m.stored(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateFrozen(r); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Resume(model.ResumeRunInput{RunID: run.ID, IdempotencyKey: "legacy-m4-resume"}); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Stop(run.ID); err != nil {
		t.Fatal(err)
	}
	r, err = m.stored(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	plan := workflow.RolePlanFor(r.Frozen)
	if r.ConfigHash != legacyHash || r.Frozen.RolePlan != nil || plan.Proposer != "codex" || len(plan.Reviewers) != 3 || len(r.Frozen.Providers) != 4 {
		t.Fatal("current disabled settings rewrote legacy participants")
	}
}
