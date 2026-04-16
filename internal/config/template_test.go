package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderTemplatePresetsAreValidYAML(t *testing.T) {
	tests := []struct {
		name    string
		preset  string
		needles []string
	}{
		{
			name:   "quickstart",
			preset: TemplatePresetQuickstart,
			needles: []string{
				"推荐修改顺序：先改 provider/agent，再改 taskSpec，再改 verificationPolicy",
				"result.json、ledger.jsonl、summary.md 和 artifacts/",
			},
		},
		{
			name:   "openai",
			preset: TemplatePresetOpenAI,
			needles: []string{
				"OPENAI_API_KEY",
				"your-openai-model",
			},
		},
		{
			name:   "coding",
			preset: TemplatePresetCoding,
			needles: []string{
				"workspace_snapshot",
				"benchmark_threshold",
			},
		},
		{
			name:   "debate",
			preset: TemplatePresetDebate,
			needles: []string{
				"mode: free_debate",
				"participants:",
			},
		},
		{
			name:   "delphi",
			preset: TemplatePresetDelphi,
			needles: []string{
				"mode: delphi",
				"convergence_threshold",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, err := RenderTemplate(tc.preset)
			if err != nil {
				t.Fatalf("RenderTemplate failed: %v", err)
			}
			for _, needle := range tc.needles {
				if !strings.Contains(body, needle) {
					t.Fatalf("template missing %q\n%s", needle, body)
				}
			}
			path := filepath.Join(t.TempDir(), "til-consensus.yaml")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatalf("write template: %v", err)
			}
			loaded, err := Load(path)
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}
			if loaded.Config.SchemaVersion != 1 {
				t.Fatalf("unexpected schema version: %d", loaded.Config.SchemaVersion)
			}
			if tc.preset == TemplatePresetDebate || tc.preset == TemplatePresetDelphi {
				if len(loaded.Config.Roles.Participants) == 0 {
					t.Fatal("expected participants role")
				}
				return
			}
			if loaded.Config.Roles.Arbiter == "" {
				t.Fatal("expected arbiter role")
			}
		})
	}
}

func TestWritePresetTemplateForce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "til-consensus.yaml")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := WritePresetTemplate(path, TemplatePresetQuickstart, false); err == nil {
		t.Fatal("expected existing file to fail without force")
	}
	if err := WritePresetTemplate(path, TemplatePresetQuickstart, true); err != nil {
		t.Fatalf("WritePresetTemplate failed: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(body), "til-consensus quickstart 配置") {
		t.Fatalf("unexpected file content:\n%s", string(body))
	}
}

func TestRenderTemplateRejectsUnknownPreset(t *testing.T) {
	if _, err := RenderTemplate("unknown"); err == nil {
		t.Fatal("expected unknown preset to fail")
	}
}
