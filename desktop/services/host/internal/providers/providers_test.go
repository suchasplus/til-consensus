package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/consensus"
	clirunner "github.com/suchasplus/til-consensus/runtime/cli"
)

func TestProviderEnvironmentNeverSharesOtherCredentials(t *testing.T) {
	keys := []string{"CODEX_HOME", "CODEX_API_KEY", "OPENAI_API_KEY", "OPENAI_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "XAI_API_KEY", "GEMINI_API_KEY", "UNRELATED_SECRET_TOKEN"}
	for _, key := range keys {
		t.Setenv(key, "test-value")
	}
	for _, id := range []string{"codex", "claude", "grok", "agy"} {
		c, info, err := Resolve(id, nil)
		if err != nil {
			t.Fatal(err)
		}
		task := consensus.ActionTask{TaskMeta: consensus.TaskMeta{AgentID: id}}
		environment := clirunner.BuildEnvironment(clirunner.SnapshotHostEnvironment(os.Environ()), Runtime(c, info, t.TempDir()), task, id, "", "")
		included := make(map[string]bool)
		for _, entry := range environment.Values {
			key, _, _ := strings.Cut(entry, "=")
			included[key] = true
		}
		allowed := make(map[string]bool)
		for _, key := range EnvironmentKeys(id) {
			allowed[key] = true
		}
		for _, key := range keys {
			if included[key] != allowed[key] {
				t.Fatalf("%s has an incorrect environment boundary for %s", id, key)
			}
		}
		if len(environment.MissingKnownKeys) != 0 {
			t.Fatalf("%s dropped its own required configuration names", id)
		}
	}
}

func TestFinderPathResolvesExecutableOutsideMinimalPATH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' 'version-probe-fixture'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	info := Probe(context.Background(), Config{ID: "codex", Command: "codex"})
	if !info.Available || info.Path != path || info.Version != "version-probe-fixture" {
		t.Fatalf("Finder-style executable discovery failed: %#v", info)
	}
}

func TestRequestedModelIsNotReportedCanonicalModel(t *testing.T) {
	// `grok models` on 1.0.62 advertises grok-4.7, while the real captured
	// response uses grok-4.7-build as its modelUsage key. They are different
	// namespaces and a reported canonical name must never rewrite CLI argv.
	c, info, err := Resolve("grok", []Config{{ID: "grok", Model: "grok-4.7"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Model != "grok-4.7" || info.RequestedModel != "grok-4.7" || info.ActualModel != "" {
		t.Fatalf("probe invented or rewrote model metadata: %#v %#v", c, info)
	}
	raw, err := os.ReadFile(filepath.Join("../../../../fixtures/cli/grok/stream-success.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	actual, _, _ := OutputMetadata(string(raw))
	if actual != "grok-4.7-build" || c.Model != "grok-4.7" {
		t.Fatal("reported canonical model was confused with CLI selection id")
	}
}
