package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildExplainReportIncludesProvidersAgentsAndOutput(t *testing.T) {
	tmp := t.TempDir()
	cfg := InitTemplate()
	cfg.Output.Directory = filepath.Join(tmp, "out", "{requestId}")
	loaded := LoadedConfig{
		Path:      filepath.Join(tmp, "til-consensus.yaml"),
		ConfigDir: tmp,
		Config:    Normalize(cfg),
	}
	report := BuildExplainReport(loaded, ExplainOptions{ProviderFilter: "mock", AgentFilter: "proposer-a"})
	if len(report.Providers) != 1 || report.Providers[0].ID != "mock" {
		t.Fatalf("unexpected providers: %#v", report.Providers)
	}
	if len(report.Agents) != 1 || report.Agents[0].ID != "proposer-a" {
		t.Fatalf("unexpected agents: %#v", report.Agents)
	}
	if report.Output.RunDir == "" || report.Output.SessionStoreDir == "" {
		t.Fatalf("expected output paths: %#v", report.Output)
	}
	text := RenderExplainText(report)
	if !strings.Contains(text, "Providers") || !strings.Contains(text, "proposer-a") {
		t.Fatalf("unexpected rendered text:\n%s", text)
	}
}
