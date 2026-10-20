package providers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func unset(t *testing.T, key string) { t.Helper(); t.Setenv(key, ""); _ = os.Unsetenv(key) }
func TestEffectivePriorityAndExplicitCLIDefault(t *testing.T) {
	unset(t, "TCVIEW_GROK_MODEL")
	unset(t, "TCVIEW_GROK_REASONING")
	c, s, err := Effective("grok", nil)
	if err != nil || c.Model != "grok-4.7" || c.Reasoning != "low" || s["model"].Kind != "default" {
		t.Fatalf("defaults: %#v %#v %v", c, s, err)
	}
	saved := []Config{{ID: "grok", Model: "grok-4.6", Reasoning: "xhigh"}}
	c, s, _ = Effective("grok", saved)
	if c.Model != "grok-4.6" || c.Reasoning != "xhigh" || s["reasoning"].Kind != "saved" {
		t.Fatalf("saved: %#v %#v", c, s)
	}
	t.Setenv("TCVIEW_GROK_MODEL", "grok-4.5")
	t.Setenv("TCVIEW_GROK_REASONING", "")
	c, s, _ = Effective("grok", saved)
	if c.Model != "grok-4.5" || c.Reasoning != "" || s["reasoning"].Kind != "environment" || s["reasoning"].Env != "TCVIEW_GROK_REASONING" {
		t.Fatalf("environment incl explicit empty: %#v %#v", c, s)
	}
	unset(t, "TCVIEW_GROK_MODEL")
	unset(t, "TCVIEW_GROK_REASONING")
	c, s, _ = Effective("grok", []Config{{ID: "grok", Model: "", Reasoning: ""}})
	if c.Model != "" || c.Reasoning != "" || s["model"].Kind != "saved" {
		t.Fatal("saved CLI default must omit overrides")
	}
}
func TestModelEffortsRejectUnsupportedAndAGYConflict(t *testing.T) {
	for _, c := range []Config{{ID: "grok", Model: "grok-4.5", Reasoning: "xhigh"}, {ID: "grok", Model: "grok-4.7-build-fast", Reasoning: "low"}, {ID: "agy", Model: "gemini-3.8-flash-medium", Reasoning: "low"}, {ID: "claude", Model: "claude-sonnet-4-6", Reasoning: "xhigh"}, {ID: "codex", Model: "unlisted", Reasoning: "ultra"}, {ID: "claude", Model: "", Reasoning: "high"}, {ID: "grok", Model: "$(bad)", Reasoning: ""}} {
		if err := Validate(c, nil); err == nil {
			t.Errorf("unexpectedly accepted %#v", c)
		}
	}
	for _, c := range []Config{{ID: "grok", Model: "grok-4.7", Reasoning: "low"}, {ID: "grok", Model: "grok-4.6", Reasoning: "xhigh"}, {ID: "agy", Model: "gemini-3.8-flash-medium"}, {ID: "claude", Model: "claude-opus-5-5", Reasoning: "max"}, {ID: "claude", Model: "claude-fable-5-1[1m]"}, {ID: "codex"}} {
		if err := Validate(c, nil); err != nil {
			t.Errorf("rejected %#v: %v", c, err)
		}
	}
	cat := &Catalog{Models: []ModelOption{{ID: "test-alias", ResolvedModel: "test-canonical", ReasoningMode: "argument", ReasoningValues: []string{"low", "ultra"}}}}
	if err := Validate(Config{ID: "codex", Model: "test-canonical", Reasoning: "ultra"}, cat); err != nil {
		t.Fatal(err)
	}
	if err := Validate(Config{ID: "codex", Model: "test-alias", Reasoning: "medium"}, cat); err == nil {
		t.Fatal("directory capability was ignored")
	}
}
func TestRedactSensitiveErrors(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "secret-token-sentinel-12345")
	got := RedactSensitive("server denied secret-token-sentinel-12345")
	if strings.Contains(got, "secret-token") || !strings.Contains(got, "已隐藏") {
		t.Fatal("credential leaked")
	}
}
func TestActualReasoningNeverComesFromRequestOrModelSuffix(t *testing.T) {
	for _, raw := range []string{`{"method":"thread/start","params":{"config":{"model_reasoning_effort":"high"}}}`, `{"type":"result","model":"gemini-3.8-flash-high","effort":"high"}`, `{"type":"assistant","message":{"content":"{\"reasoningEffort\":\"high\"}"}}`} {
		if v := ReportedReasoning(raw); v != "" {
			t.Fatalf("invented report: %s", v)
		}
	}
	if got := ReportedReasoning(`{"id":2,"result":{"thread":{"id":"t"},"reasoningEffort":"low"}}`); got != "low" {
		t.Fatal(got)
	}
}
func TestCatalogParsersOnlyExposeModelWhitelist(t *testing.T) {
	models := ParseModels("agy", "Fetching available models...\ngemini-3.8-flash-low\tGemini (Low)\ngemini-3.8-flash-medium\tGemini (Medium)\n")
	if len(models) != 2 || models[0].Family != "gemini-3.8-flash" || models[0].Effort != "low" || models[0].ReasoningMode != "encoded" {
		t.Fatalf("%#v", models)
	}
	grok := ParseModels("grok", "Default model: grok-4.7\n  * grok-4.7 (default)\n - grok-4.7-build-fast\n")
	if len(grok) != 2 || len(grok[0].ReasoningValues) != 4 || len(grok[1].ReasoningValues) != 0 {
		t.Fatalf("%#v", grok)
	}
	raw := `{"type":"control_response","response":{"response":{"models":[{"value":"opus","resolvedModel":"claude-opus-5-5","displayName":"Opus","supportsEffort":true,"supportedEffortLevels":["low","xhigh","max"]}],"account":{"secret":"DO-NOT-EXPOSE"}}}}`
	claude := ParseClaudeModels(raw)
	if len(claude) != 1 || claude[0].ID != "opus" || claude[0].ResolvedModel != "claude-opus-5-5" || len(claude[0].ReasoningValues) != 3 {
		t.Fatalf("%#v", claude)
	}
	b, _ := json.Marshal(claude)
	if strings.Contains(string(b), "DO-NOT-EXPOSE") {
		t.Fatal("account metadata leaked")
	}
}

func TestRedactionDoesNotBreakJSONOrLeakURLCredentials(t *testing.T) {
	t.Setenv("XAI_API_KEY", "secret-with-quote-\"-sentinel")
	encoded, _ := json.Marshal(map[string]any{"error": "secret-with-quote-\"-sentinel", "url": "https://username:pass-sentinel@example.invalid/path?token=query-sentinel&other=visible"})
	clean := RedactJSON(encoded)
	if !json.Valid(clean) || strings.Contains(string(clean), "sentinel") || strings.Contains(string(clean), "username:") {
		t.Fatalf("unsafe JSON redaction: %s", clean)
	}
	if !strings.Contains(string(clean), "other=visible") {
		t.Fatal("redaction dropped useful diagnostics")
	}
}

func TestDirectoryFailureShowsBoundedRedactedStderrAndNeverAccountStdout(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "private-key-directory-sentinel")
	command := filepath.Join(t.TempDir(), "agy-fixture")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then printf '%s\n' 'AGY fixture 1.4.2'; exit 0; fi
printf '%s\n' '{"account":{"email":"private-account-sentinel"}}'
printf '模型目录权限不足 credential=%s: ' "$GEMINI_API_KEY" >&2
x=0
while [ "$x" -lt 5000 ]; do printf x >&2; x=$((x + 1)); done
exit 1
`
	if err := os.WriteFile(command, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	catalog, err := DiscoverModels(context.Background(), Config{ID: "agy", Command: command})
	if err == nil || !strings.Contains(catalog.Error, "模型目录权限不足") || !strings.Contains(catalog.Error, "已隐藏") || catalog.CLIVersion != "AGY fixture 1.4.2" {
		t.Fatalf("missing actionable failure: %#v %v", catalog, err)
	}
	public, _ := json.Marshal(catalog)
	if strings.Contains(string(public), "private-key-directory-sentinel") || strings.Contains(string(public), "private-account-sentinel") {
		t.Fatal("directory error exposed secret or account stdout")
	}
	if len([]rune(catalog.Error)) > 2200 || len(catalog.Models) != 0 {
		t.Fatal("directory error exceeded public bound or accepted a failed catalogue")
	}
}
func TestStaleDirectoryNeverConfirmsDynamicEffort(t *testing.T) {
	original := &Catalog{ID: "codex", CLIVersion: "CLI old", LoadedAt: "2026-01-01T00:00:00Z", Source: "cli-directory", Models: []ModelOption{{ID: "dynamic", ReasoningMode: "argument", ReasoningValues: []string{"ultra"}}}}
	request := Config{ID: "codex", Model: "dynamic", Reasoning: "ultra"}
	current := CatalogForVersion(original, "CLI old")
	if err := Validate(request, current); err != nil {
		t.Fatal(err)
	}
	stale := CatalogForVersion(original, "CLI new")
	if stale.Source != "cli-directory-stale" || stale.Error == "" || stale.LoadedAt != original.LoadedAt {
		t.Fatalf("%#v", stale)
	}
	if err := Validate(request, stale); err == nil || !strings.Contains(err.Error(), "重新加载") {
		t.Fatal("obsolete capability accepted")
	}
	if original.Source != "cli-directory" || original.Error != "" {
		t.Fatal("version projection rewrote historical directory")
	}
	legacy := *original
	legacy.CLIVersion = ""
	if err := Validate(request, CatalogForVersion(&legacy, "CLI new")); err == nil {
		t.Fatal("unversioned directory confirmed a dynamic capability")
	}
}

func TestEnabledPriorityAndLegacyDefaults(t *testing.T) {
	unset(t, "TCVIEW_GROK_ENABLED")
	for _, saved := range [][]Config{nil, {{ID: "grok", Model: "old-model"}}} {
		c, sources, err := Effective("grok", saved)
		if err != nil || !IsEnabled(c) || sources["enabled"].Kind != "default" {
			t.Fatalf("legacy/default: %#v %#v %v", c, sources, err)
		}
	}
	saved := []Config{{ID: "grok", Enabled: Bool(false)}}
	c, sources, err := Effective("grok", saved)
	if err != nil || IsEnabled(c) || sources["enabled"].Kind != "saved" {
		t.Fatalf("saved: %#v %#v %v", c, sources, err)
	}
	t.Setenv("TCVIEW_GROK_ENABLED", "true")
	c, sources, err = Effective("grok", saved)
	if err != nil || !IsEnabled(c) || sources["enabled"].Env != "TCVIEW_GROK_ENABLED" {
		t.Fatalf("environment: %#v %#v %v", c, sources, err)
	}
	t.Setenv("TCVIEW_GROK_ENABLED", "0")
	c, _, err = Effective("grok", nil)
	if err != nil || IsEnabled(c) {
		t.Fatal("explicit environment off was ignored")
	}
	t.Setenv("TCVIEW_GROK_ENABLED", "private-invalid-sentinel")
	if _, _, err = Effective("grok", nil); err == nil || strings.Contains(err.Error(), "sentinel") {
		t.Fatal("invalid enable flag accepted or leaked value")
	}
}
