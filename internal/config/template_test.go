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
				"mode=adjudication provider_profile=mock task_profile=general",
				"# 兼容别名: quickstart",
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
				"participant-c",
			},
		},
		{
			name:   "delphi",
			preset: TemplatePresetDelphi,
			needles: []string{
				"mode: delphi",
				"convergence_threshold",
				"participant-c",
			},
		},
		{
			name:   "generic",
			preset: TemplatePresetGeneric,
			needles: []string{
				"cli_type: generic",
				"./scripts/generic_adapter.py",
			},
		},
		{
			name:   "codex",
			preset: TemplatePresetCodex,
			needles: []string{
				"cli_type: codex",
				"provider_model: gpt-5.4",
			},
		},
		{
			name:   "claude",
			preset: TemplatePresetClaude,
			needles: []string{
				"cli_type: claude",
				"provider_model: claude-opus-4-6",
			},
		},
		{
			name:   "gemini",
			preset: TemplatePresetGemini,
			needles: []string{
				"cli_type: gemini",
				"provider_model: gemini-3.1-pro-preview",
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
	if !strings.Contains(string(body), "mode=adjudication provider_profile=mock task_profile=general") {
		t.Fatalf("unexpected file content:\n%s", string(body))
	}
}

func TestRenderTemplateRejectsUnknownPreset(t *testing.T) {
	if _, err := RenderTemplate("unknown"); err == nil {
		t.Fatal("expected unknown preset to fail")
	}
}

func TestResolveTemplateSelectionAxes(t *testing.T) {
	selection, err := ResolveTemplateSelection("", "delphi", "codex", "general")
	if err != nil {
		t.Fatalf("ResolveTemplateSelection failed: %v", err)
	}
	if selection.Mode != "delphi" || selection.ProviderProfile != TemplateProviderProfileCodex || selection.TaskProfile != TemplateTaskProfileGeneral {
		t.Fatalf("unexpected selection: %#v", selection)
	}

	selection, err = ResolveTemplateSelection("coding", "", "", "")
	if err != nil {
		t.Fatalf("ResolveTemplateSelection failed: %v", err)
	}
	if selection.Mode != "adjudication" || selection.ProviderProfile != TemplateProviderProfileMock || selection.TaskProfile != TemplateTaskProfileCoding {
		t.Fatalf("unexpected coding alias selection: %#v", selection)
	}

	if _, err := ResolveTemplateSelection("", "free-debate", "", "coding"); err == nil {
		t.Fatal("expected coding task profile with free-debate to fail")
	}
}

func TestRenderTemplateRequestBuildsComposedTemplate(t *testing.T) {
	body, selection, err := RenderTemplateRequest("", "delphi", "gemini", "general")
	if err != nil {
		t.Fatalf("RenderTemplateRequest failed: %v", err)
	}
	if selection.Mode != "delphi" || selection.ProviderProfile != TemplateProviderProfileGemini {
		t.Fatalf("unexpected selection: %#v", selection)
	}
	for _, needle := range []string{
		"mode=delphi provider_profile=gemini task_profile=general",
		"cli_type: gemini",
		"provider_model: gemini-3.1-pro-preview",
		"participant-c",
		"facilitator-a",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("rendered template missing %q\n%s", needle, body)
		}
	}
}
