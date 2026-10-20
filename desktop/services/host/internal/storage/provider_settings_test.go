package storage

import (
	"context"
	"encoding/json"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsPersistResetMigrateAndPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := s.ProviderSettings(context.Background())
	if err != nil || initial.Version != 1 || len(initial.Providers) != 0 {
		t.Fatalf("%#v %v", initial, err)
	}
	input := model.CLISettings{Providers: []model.CLIProviderSetting{{ID: "grok", Model: "grok-4.6", Reasoning: "xhigh"}, {ID: "codex", Model: "", Reasoning: ""}}}
	if err = s.SaveProviderSettings(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.ProviderSettings(context.Background())
	if err != nil || len(got.Providers) != 2 || got.Providers[0].Reasoning != "xhigh" || got.Providers[1].Model != "" {
		t.Fatalf("%#v %v", got, err)
	}
	stat, err := os.Stat(filepath.Join(dir, "desktop.sqlite"))
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatalf("settings DB permissions: %v %v", stat, err)
	}
	legacy, _ := json.Marshal(model.CLISettings{Providers: input.Providers})
	if _, err = s.db.Exec("UPDATE provider_settings SET body=? WHERE id='cli'", legacy); err != nil {
		t.Fatal(err)
	}
	got, err = s.ProviderSettings(context.Background())
	if err != nil || got.Version != 1 || len(got.Providers) != 2 {
		t.Fatal("legacy settings migration lost choices")
	}
	if err = s.ResetProviderSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = s.ProviderSettings(context.Background())
	if err != nil || len(got.Providers) != 0 {
		t.Fatal("reset did not clear settings")
	}
}
