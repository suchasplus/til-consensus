package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPathPrecedence(t *testing.T) {
	tmp := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(original)
	})

	projectPath := filepath.Join(tmp, "til-consensus.yaml")
	globalHome := filepath.Join(tmp, "xdg")
	defaultGlobalPath := filepath.Join(globalHome, "til-consensus", "default.yaml")
	legacyGlobalPath := filepath.Join(globalHome, "til-consensus", "config.yaml")
	t.Setenv("XDG_CONFIG_HOME", globalHome)

	if err := os.MkdirAll(filepath.Dir(defaultGlobalPath), 0o755); err != nil {
		t.Fatalf("mkdir global dir: %v", err)
	}
	if err := os.WriteFile(defaultGlobalPath, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	resolved, err := ResolveConfigPath("")
	if err != nil {
		t.Fatalf("ResolveConfigPath global failed: %v", err)
	}
	if samePath(t, resolved, defaultGlobalPath) == false {
		t.Fatalf("expected global path, got %s", resolved)
	}

	if err := os.Remove(defaultGlobalPath); err != nil {
		t.Fatalf("remove default global config: %v", err)
	}
	if err := os.WriteFile(legacyGlobalPath, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write legacy global config: %v", err)
	}
	resolved, err = ResolveConfigPath("")
	if err != nil {
		t.Fatalf("ResolveConfigPath legacy global failed: %v", err)
	}
	if samePath(t, resolved, legacyGlobalPath) == false {
		t.Fatalf("expected legacy global path, got %s", resolved)
	}

	if err := os.WriteFile(projectPath, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	resolved, err = ResolveConfigPath("")
	if err != nil {
		t.Fatalf("ResolveConfigPath project failed: %v", err)
	}
	if samePath(t, resolved, projectPath) == false {
		t.Fatalf("expected project path, got %s", resolved)
	}

	explicit := filepath.Join(tmp, "custom.yaml")
	if err := os.WriteFile(explicit, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write explicit config: %v", err)
	}
	resolved, err = ResolveConfigPath(explicit)
	if err != nil {
		t.Fatalf("ResolveConfigPath explicit failed: %v", err)
	}
	if samePath(t, resolved, explicit) == false {
		t.Fatalf("expected explicit path, got %s", resolved)
	}
}

func TestLoadRunInputJSONAndHelpers(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "run.json")
	if err := os.WriteFile(inputPath, []byte(`{
  "request_id": "req-1",
  "task_spec": {
    "goal": "verify patch",
    "success_criteria": ["a", "b"]
  }
}`), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	input, err := LoadRunInput(inputPath)
	if err != nil {
		t.Fatalf("LoadRunInput failed: %v", err)
	}
	if input.RequestID != "req-1" || input.TaskSpec.Goal != "verify patch" {
		t.Fatalf("unexpected input: %#v", input)
	}

	defaultPath, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath failed: %v", err)
	}
	if filepath.Base(defaultPath) != "default.yaml" {
		t.Fatalf("unexpected default path: %s", defaultPath)
	}
	if got := toAbs("til-consensus.yaml", "/tmp/base"); got != filepath.Join("/tmp/base", "til-consensus.yaml") {
		t.Fatalf("unexpected toAbs result: %s", got)
	}
}

func TestModelIDsAndSingleModelID(t *testing.T) {
	provider := ProviderConfig{
		Models: map[string]ProviderModelConfig{
			"b": {ProviderModel: "b"},
			"a": {ProviderModel: "a"},
		},
	}
	ids := ModelIDs(provider)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("unexpected model ids: %#v", ids)
	}
	if _, ok := singleModelID(provider); ok {
		t.Fatalf("expected multiple models to fail inference")
	}
	if got, ok := singleModelID(ProviderConfig{
		Models: map[string]ProviderModelConfig{"default": {ProviderModel: "model"}},
	}); !ok || got != "default" {
		t.Fatalf("unexpected single model inference: %s %t", got, ok)
	}
}

func samePath(t *testing.T, left string, right string) bool {
	t.Helper()
	leftEval, err := filepath.EvalSymlinks(left)
	if err != nil {
		leftEval = left
	}
	rightEval, err := filepath.EvalSymlinks(right)
	if err != nil {
		rightEval = right
	}
	return leftEval == rightEval
}
