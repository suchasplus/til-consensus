package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	tilruntime "github.com/suchasplus/til-consensus/internal/runtime"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

type e2eFixtureManifest struct {
	Name                  string                 `yaml:"name"`
	Mode                  consensus.WorkflowMode `yaml:"mode"`
	Description           string                 `yaml:"description"`
	ViewSections          []string               `yaml:"view_sections"`
	ExpectedViewFragments []string               `yaml:"expected_view_fragments"`
}

type e2eFixture struct {
	Name     string
	Root     string
	RunPath  string
	Manifest e2eFixtureManifest
}

type e2eCLIProviderLayout struct {
	Agents               []config.AgentConfig
	Roles                config.RolesConfig
	ExpectedProviders    []string
	AssignmentSummary    []string
	UnavailableProviders []string
}

type complianceSummaryDoc struct {
	Version     int                       `json:"version"`
	GeneratedAt string                    `json:"generatedAt"`
	Entries     []complianceSummaryRecord `json:"entries"`
}

type complianceSummaryRecord struct {
	Provider      string             `json:"provider"`
	ProviderType  string             `json:"providerType"`
	ProviderModel string             `json:"providerModel"`
	TaskKind      consensus.TaskKind `json:"taskKind"`
	Total         int                `json:"total"`
	Strict        int                `json:"strict"`
	Normalized    int                `json:"normalized"`
	Repaired      int                `json:"repaired"`
	Failed        int                `json:"failed"`
}

type cliProviderPreflightResult struct {
	Provider        string
	Command         []string
	Duration        time.Duration
	Ready           bool
	StrictJSON      bool
	RecoverableJSON bool
	StdoutPreview   string
	StderrPreview   string
	Error           string
}

var (
	realCLIPreflightOnce    sync.Once
	realCLIPreflightResults []cliProviderPreflightResult
)

func TestE2EFixtureCatalog(t *testing.T) {
	fixtures := loadE2EFixtures(t)
	if len(fixtures) != 3 {
		t.Fatalf("expected 3 e2e fixtures, got %d", len(fixtures))
	}
	expectedModes := []consensus.WorkflowMode{
		consensus.WorkflowModeAdjudication,
		consensus.WorkflowModeFreeDebate,
		consensus.WorkflowModeDelphi,
	}
	actualModes := make([]consensus.WorkflowMode, 0, len(fixtures))
	for _, fixture := range fixtures {
		actualModes = append(actualModes, fixture.Manifest.Mode)
		if len(fixture.Manifest.ViewSections) == 0 {
			t.Fatalf("fixture %s missing view sections", fixture.Name)
		}
		if len(fixture.Manifest.ExpectedViewFragments) == 0 {
			t.Fatalf("fixture %s missing expected view fragments", fixture.Name)
		}
	}
	slices.Sort(actualModes)
	slices.Sort(expectedModes)
	if !slices.Equal(actualModes, expectedModes) {
		t.Fatalf("unexpected fixture modes: got=%v want=%v", actualModes, expectedModes)
	}
}

func TestE2EAPIFixtureMatrix(t *testing.T) {
	fixtures := loadE2EFixtures(t)
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			openAIHits := 0
			anthropicHits := 0
			openAIServer, anthropicServer := startE2EAPIServers(t, fixture.Manifest.Mode, &openAIHits, &anthropicHits)
			defer openAIServer.Close()
			defer anthropicServer.Close()

			staged := stageE2EFixture(t, fixture)
			configPath := filepath.Join(staged, "til-consensus.yaml")
			writeE2EProviderConfig(t, e2eConfigParams{
				Path:             configPath,
				OutputDir:        filepath.Join(staged, "out", "{requestId}"),
				Mode:             fixture.Manifest.Mode,
				ProviderKind:     "api",
				OpenAIBaseURL:    openAIServer.URL,
				AnthropicBaseURL: anthropicServer.URL,
			})

			runCmd := newRunCommand()
			runStdout, runStderr, err := runCLICommand(context.Background(), runCmd, []string{"run", "--config", configPath, "--input", filepath.Join(staged, "run.yaml")})
			if err != nil {
				t.Fatalf("api e2e run failed: %v\nstderr=%s\nstdout=%s", err, runStderr, runStdout)
			}
			resultPath := extractResultPath(t, runStdout)
			result := loadRunResult(t, resultPath)
			assertE2EResultForFixture(t, fixture, result)

			viewArgs := append([]string{"view", "--result", resultPath, "--verbose"}, renderViewSections(fixture.Manifest.ViewSections)...)
			viewCmd := newViewCommand()
			viewStdout, viewStderr, err := runCLICommand(context.Background(), viewCmd, viewArgs)
			if err != nil {
				t.Fatalf("api e2e view failed: %v\nstderr=%s", err, viewStderr)
			}
			for _, fragment := range fixture.Manifest.ExpectedViewFragments {
				if !strings.Contains(viewStdout, fragment) {
					t.Fatalf("expected %q in view output\n%s", fragment, viewStdout)
				}
			}

			summary := loadComplianceSummary(t, resultPath)
			assertComplianceSummary(t, summary, "api", []string{"openai-test", "anthropic-test"})
			if openAIHits == 0 || anthropicHits == 0 {
				t.Fatalf("expected both api providers to be exercised, openai=%d anthropic=%d", openAIHits, anthropicHits)
			}
		})
	}
}

func TestE2ERealCLIFixtureMatrix(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TIL_CONSENSUS_E2E_REAL")) == "" {
		t.Skip("set TIL_CONSENSUS_E2E_REAL=1 to run real CLI e2e matrix")
	}
	readiness := loadRealCLIProviderReadiness(t)
	logRealCLIProviderReadiness(t, readiness)
	if countReadyCLIProviders(readiness) == 0 {
		t.Fatalf("no real CLI providers are ready in current test context")
	}

	fixtures := loadE2EFixtures(t)
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			staged := stageE2EFixture(t, fixture)
			latestResultPath := ""
			latestStdout := ""
			latestStderr := ""
			defer func() {
				_ = preserveE2EWorkspaceIfNeeded(t, fixture, staged, latestResultPath, latestStdout, latestStderr)
			}()
			configPath := filepath.Join(staged, "til-consensus.yaml")
			layout := buildCLIE2EProviderLayout(fixture.Manifest.Mode, readiness)
			if len(layout.ExpectedProviders) == 0 {
				t.Skipf("no ready providers available for mode %s", fixture.Manifest.Mode)
			}
			t.Logf("cli provider layout: %s", strings.Join(layout.AssignmentSummary, ", "))
			if len(layout.UnavailableProviders) > 0 {
				t.Logf("cli unavailable providers: %s", strings.Join(layout.UnavailableProviders, ", "))
			}
			writeE2EProviderConfig(t, e2eConfigParams{
				Path:         configPath,
				OutputDir:    filepath.Join(staged, "out", "{requestId}"),
				Mode:         fixture.Manifest.Mode,
				ProviderKind: "cli",
				CLILayout:    &layout,
			})
			timeout := realE2ETimeoutForMode(fixture.Manifest.Mode)
			t.Logf("real cli e2e timeout: mode=%s timeout=%s workspace=%s", fixture.Manifest.Mode, timeout, staged)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			runCmd := newRunCommand()
			runStdout, runStderr, err := runCLICommandWithLiveLogs(ctx, t, fmt.Sprintf("%s/run", fixture.Name), runCmd, []string{"run", "--config", configPath, "--input", filepath.Join(staged, "run.yaml"), "--verbose", "--debug"})
			resultPath := ""
			if err == nil {
				resultPath = tryExtractResultPath(runStdout)
			}
			latestResultPath = resultPath
			latestStdout = runStdout
			latestStderr = runStderr
			preservePath := preserveE2EWorkspaceIfNeeded(t, fixture, staged, resultPath, runStdout, runStderr)
			if err != nil {
				if preservePath != "" {
					t.Fatalf("real cli e2e run failed: %v\npreserved=%s\nstderr=%s\nstdout=%s", err, preservePath, runStderr, runStdout)
				}
				t.Fatalf("real cli e2e run failed: %v\nstderr=%s\nstdout=%s", err, runStderr, runStdout)
			}
			result := loadRunResult(t, resultPath)
			assertE2EResultForFixture(t, fixture, result)

			viewArgs := append([]string{"view", "--result", resultPath, "--verbose"}, renderViewSections(fixture.Manifest.ViewSections)...)
			viewCmd := newViewCommand()
			viewStdout, viewStderr, err := runCLICommandWithLiveLogs(context.Background(), t, fmt.Sprintf("%s/view", fixture.Name), viewCmd, viewArgs)
			latestStdout = viewStdout
			latestStderr = viewStderr
			if err != nil {
				preservePath = preserveE2EWorkspaceIfNeeded(t, fixture, staged, resultPath, viewStdout, viewStderr)
				if preservePath != "" {
					t.Fatalf("real cli e2e view failed: %v\npreserved=%s\nstderr=%s\nstdout=%s", err, preservePath, viewStderr, viewStdout)
				}
				t.Fatalf("real cli e2e view failed: %v\nstderr=%s\nstdout=%s", err, viewStderr, viewStdout)
			}
			for _, fragment := range fixture.Manifest.ExpectedViewFragments {
				if !strings.Contains(viewStdout, fragment) {
					preservePath = preserveE2EWorkspaceIfNeeded(t, fixture, staged, resultPath, viewStdout, viewStderr)
					if preservePath != "" {
						t.Fatalf("expected %q in view output\npreserved=%s\n%s", fragment, preservePath, viewStdout)
					}
					t.Fatalf("expected %q in view output\n%s", fragment, viewStdout)
				}
			}

			summary := loadComplianceSummary(t, resultPath)
			assertComplianceSummary(t, summary, "cli", layout.ExpectedProviders)
		})
	}
}

func TestE2ERealCLIProviderReadinessPreflight(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TIL_CONSENSUS_E2E_REAL")) == "" {
		t.Skip("set TIL_CONSENSUS_E2E_REAL=1 to run real CLI readiness preflight")
	}
	readiness := loadRealCLIProviderReadiness(t)
	logRealCLIProviderReadiness(t, readiness)
	readyCount := 0
	for _, result := range readiness {
		t.Run(result.Provider, func(t *testing.T) {
			if !result.Ready {
				t.Skipf("provider not ready: %s", formatPreflightFailureReason(result))
			}
			readyCount++
		})
	}
	if readyCount == 0 {
		t.Fatalf("no real CLI providers are ready in current test context")
	}
}

func TestBuildCLIE2EProviderLayoutDegradesUnavailableProviders(t *testing.T) {
	layout := buildCLIE2EProviderLayout(consensus.WorkflowModeAdjudication, []cliProviderPreflightResult{
		{Provider: "claude", Ready: true},
		{Provider: "gemini", Ready: false},
		{Provider: "codex", Ready: true},
	})
	if len(layout.Agents) == 0 {
		t.Fatalf("expected degraded layout agents, got %#v", layout)
	}
	if layout.Roles.SemanticVerifier == "" || layout.Roles.Arbiter == "" {
		t.Fatalf("expected adjudication roles to be assigned, got %#v", layout.Roles)
	}
	if !slices.Contains(layout.ExpectedProviders, "claude-cli") || !slices.Contains(layout.ExpectedProviders, "codex-cli") {
		t.Fatalf("expected ready providers in layout, got %#v", layout.ExpectedProviders)
	}
	if !slices.Contains(layout.UnavailableProviders, "gemini-cli") {
		t.Fatalf("expected unavailable gemini provider, got %#v", layout.UnavailableProviders)
	}
}

func TestNormalizePreflightOutputExtractsClaudeStructuredOutput(t *testing.T) {
	got := normalizePreflightOutput("claude", `{"type":"result","structured_output":{"ok":true}}`)
	if got != `{"ok":true}` {
		t.Fatalf("unexpected normalized preflight output: %s", got)
	}
}

func loadE2EFixtures(t *testing.T) []e2eFixture {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "e2e")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read e2e fixture dir failed: %v", err)
	}
	fixtures := make([]e2eFixture, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		fixtureRoot := filepath.Join(root, entry.Name())
		manifestPath := filepath.Join(fixtureRoot, "fixture.yaml")
		runPath := filepath.Join(fixtureRoot, "run.yaml")
		manifestBody, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("read fixture manifest %s failed: %v", manifestPath, err)
		}
		var manifest e2eFixtureManifest
		if err := yaml.Unmarshal(manifestBody, &manifest); err != nil {
			t.Fatalf("decode fixture manifest %s failed: %v", manifestPath, err)
		}
		if manifest.Name == "" {
			manifest.Name = entry.Name()
		}
		if manifest.Mode == "" {
			t.Fatalf("fixture %s missing mode", manifestPath)
		}
		if _, err := os.Stat(runPath); err != nil {
			t.Fatalf("fixture %s missing run.yaml: %v", fixtureRoot, err)
		}
		fixtures = append(fixtures, e2eFixture{
			Name:     manifest.Name,
			Root:     fixtureRoot,
			RunPath:  runPath,
			Manifest: manifest,
		})
	}
	slices.SortFunc(fixtures, func(a, b e2eFixture) int {
		return strings.Compare(a.Name, b.Name)
	})
	return fixtures
}

func stageE2EFixture(t *testing.T, fixture e2eFixture) string {
	t.Helper()
	dst := t.TempDir()
	if err := copyDir(fixture.Root, dst); err != nil {
		t.Fatalf("copy e2e fixture failed: %v", err)
	}
	return dst
}

type e2eConfigParams struct {
	Path             string
	OutputDir        string
	Mode             consensus.WorkflowMode
	ProviderKind     string
	OpenAIBaseURL    string
	AnthropicBaseURL string
	CLILayout        *e2eCLIProviderLayout
}

func writeE2EProviderConfig(t *testing.T, params e2eConfigParams) {
	t.Helper()
	cfg := config.InitTemplate()
	cfg.Defaults.Mode = params.Mode
	cfg.Output.Directory = params.OutputDir
	cfg.Defaults.PerTaskTimeout = config.Duration{}
	cfg.Defaults.TaskRetryAttempts = 1
	cfg.Defaults.VerificationPolicy.MaxParallelChecks = 1
	cfg.Defaults.ProposalPolicy.MaxPasses = 1
	cfg.Defaults.ProposalPolicy.MaxClaimsPerWorker = 2

	switch params.Mode {
	case consensus.WorkflowModeFreeDebate:
		cfg.Defaults.DebatePolicy = config.DebatePolicyConfig{
			MinRounds:       2,
			MaxRounds:       2,
			VoteThreshold:   0.67,
			EnableEarlyStop: true,
			PeerContextMode: "summary+active_claims",
		}
	case consensus.WorkflowModeDelphi:
		cfg.Defaults.DelphiPolicy = config.DelphiPolicyConfig{
			MinRounds:               2,
			MaxRounds:               2,
			ConvergenceThreshold:    0.75,
			RatingScaleMin:          1,
			RatingScaleMax:          5,
			Anonymous:               true,
			FacilitatorSummaryStyle: "anonymous-aggregate",
		}
	}

	switch params.ProviderKind {
	case "cli":
		cfg.Providers = map[string]config.ProviderConfig{
			"claude-cli": {
				Type:    config.ProviderTypeCLI,
				CLIType: "claude",
				Command: "claude",
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "claude-opus-4-6", Reasoning: "medium"},
				},
			},
			"gemini-cli": {
				Type:    config.ProviderTypeCLI,
				CLIType: "gemini",
				Command: "gemini",
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "gemini-3.1-pro-preview", Reasoning: "medium"},
				},
			},
			"codex-cli": {
				Type:    config.ProviderTypeCLI,
				CLIType: "codex",
				Command: "codex",
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "gpt-5.4", Reasoning: "medium"},
				},
			},
		}
		if params.CLILayout != nil {
			cfg.Agents = append([]config.AgentConfig(nil), params.CLILayout.Agents...)
			cfg.Roles = params.CLILayout.Roles
		} else {
			cfg.Agents, cfg.Roles = defaultCLIE2EAgents(params.Mode)
		}
	case "api":
		cfg.Providers = map[string]config.ProviderConfig{
			"openai-test": {
				Type:     config.ProviderTypeAPI,
				Protocol: config.APIProtocolOpenAICompatible,
				BaseURL:  params.OpenAIBaseURL,
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "gpt-test"},
				},
			},
			"anthropic-test": {
				Type:     config.ProviderTypeAPI,
				Protocol: config.APIProtocolAnthropicCompatible,
				BaseURL:  params.AnthropicBaseURL,
				Models: map[string]config.ProviderModelConfig{
					"default": {ProviderModel: "claude-test"},
				},
			},
		}
		cfg.Agents, cfg.Roles = buildAPIE2EAgents(params.Mode)
	default:
		t.Fatalf("unsupported provider kind: %s", params.ProviderKind)
	}

	cfg = config.Normalize(cfg)
	if err := config.Write(params.Path, cfg); err != nil {
		t.Fatalf("write e2e provider config failed: %v", err)
	}
}

func defaultCLIE2EAgents(mode consensus.WorkflowMode) ([]config.AgentConfig, config.RolesConfig) {
	switch mode {
	case consensus.WorkflowModeFreeDebate:
		return []config.AgentConfig{
				{ID: "participant-claude", Provider: "claude-cli", Model: "default", Role: "participant"},
				{ID: "participant-gemini", Provider: "gemini-cli", Model: "default", Role: "participant"},
				{ID: "participant-codex", Provider: "codex-cli", Model: "default", Role: "participant"},
				{ID: "reporter-claude", Provider: "claude-cli", Model: "default", Role: "reporter"},
			}, config.RolesConfig{
				Participants: []string{"participant-claude", "participant-gemini", "participant-codex"},
				Reporter:     "reporter-claude",
			}
	case consensus.WorkflowModeDelphi:
		return []config.AgentConfig{
				{ID: "participant-claude", Provider: "claude-cli", Model: "default", Role: "participant"},
				{ID: "participant-gemini", Provider: "gemini-cli", Model: "default", Role: "participant"},
				{ID: "participant-codex", Provider: "codex-cli", Model: "default", Role: "participant"},
				{ID: "facilitator-claude", Provider: "claude-cli", Model: "default", Role: "facilitator"},
				{ID: "reporter-codex", Provider: "codex-cli", Model: "default", Role: "reporter"},
			}, config.RolesConfig{
				Participants: []string{"participant-claude", "participant-gemini", "participant-codex"},
				Facilitator:  "facilitator-claude",
				Reporter:     "reporter-codex",
			}
	default:
		return []config.AgentConfig{
				{ID: "proposer-claude", Provider: "claude-cli", Model: "default", Role: "proposer"},
				{ID: "challenger-gemini", Provider: "gemini-cli", Model: "default", Role: "challenger"},
				{ID: "verifier-codex", Provider: "codex-cli", Model: "default", Role: "semantic-verifier"},
				{ID: "arbiter-claude", Provider: "claude-cli", Model: "default", Role: "arbiter"},
				{ID: "reporter-gemini", Provider: "gemini-cli", Model: "default", Role: "reporter"},
			}, config.RolesConfig{
				Proposers:        []string{"proposer-claude"},
				Challengers:      []string{"challenger-gemini"},
				SemanticVerifier: "verifier-codex",
				Arbiter:          "arbiter-claude",
				Reporter:         "reporter-gemini",
			}
	}
}

func buildCLIE2EProviderLayout(mode consensus.WorkflowMode, readiness []cliProviderPreflightResult) e2eCLIProviderLayout {
	ready := readyCLIProviderSet(readiness)
	ordered := orderedReadyCLIProviders(ready)
	layout := e2eCLIProviderLayout{
		UnavailableProviders: unavailableCLIProviders(readiness),
	}
	if len(ordered) == 0 {
		return layout
	}

	addAgent := func(id string, provider string, role string) string {
		layout.Agents = append(layout.Agents, config.AgentConfig{ID: id, Provider: provider, Model: "default", Role: role})
		layout.AssignmentSummary = append(layout.AssignmentSummary, fmt.Sprintf("%s=%s", id, provider))
		return id
	}
	usedProviders := map[string]struct{}{}
	markProvider := func(provider string) {
		if provider == "" {
			return
		}
		usedProviders[provider] = struct{}{}
	}
	selectProvider := func(preferences ...string) string {
		for _, candidate := range preferences {
			if ready[candidate] {
				return candidate
			}
		}
		return ordered[0]
	}

	switch mode {
	case consensus.WorkflowModeFreeDebate:
		participantProviders := []string{
			selectProvider("claude-cli", "gemini-cli", "codex-cli"),
			selectProvider("gemini-cli", "codex-cli", "claude-cli"),
			selectProvider("codex-cli", "claude-cli", "gemini-cli"),
		}
		participantIDs := make([]string, 0, len(participantProviders))
		for idx, provider := range participantProviders {
			participantIDs = append(participantIDs, addAgent(fmt.Sprintf("participant-%d-%s", idx+1, shortCLIProviderName(provider)), provider, "participant"))
			markProvider(provider)
		}
		reporterProvider := selectProvider("claude-cli", "codex-cli", "gemini-cli")
		reporterID := addAgent("reporter-"+shortCLIProviderName(reporterProvider), reporterProvider, "reporter")
		markProvider(reporterProvider)
		layout.Roles = config.RolesConfig{
			Participants: participantIDs,
			Reporter:     reporterID,
		}
	case consensus.WorkflowModeDelphi:
		participantProviders := []string{
			selectProvider("claude-cli", "gemini-cli", "codex-cli"),
			selectProvider("gemini-cli", "codex-cli", "claude-cli"),
			selectProvider("codex-cli", "claude-cli", "gemini-cli"),
		}
		participantIDs := make([]string, 0, len(participantProviders))
		for idx, provider := range participantProviders {
			participantIDs = append(participantIDs, addAgent(fmt.Sprintf("participant-%d-%s", idx+1, shortCLIProviderName(provider)), provider, "participant"))
			markProvider(provider)
		}
		facilitatorProvider := selectProvider("claude-cli", "codex-cli", "gemini-cli")
		reporterProvider := selectProvider("codex-cli", "claude-cli", "gemini-cli")
		facilitatorID := addAgent("facilitator-"+shortCLIProviderName(facilitatorProvider), facilitatorProvider, "facilitator")
		reporterID := addAgent("reporter-"+shortCLIProviderName(reporterProvider), reporterProvider, "reporter")
		markProvider(facilitatorProvider)
		markProvider(reporterProvider)
		layout.Roles = config.RolesConfig{
			Participants: participantIDs,
			Facilitator:  facilitatorID,
			Reporter:     reporterID,
		}
	default:
		proposerProvider := selectProvider("claude-cli", "codex-cli", "gemini-cli")
		challengerProvider := selectProvider("gemini-cli", "claude-cli", "codex-cli")
		verifierProvider := selectProvider("codex-cli", "claude-cli", "gemini-cli")
		arbiterProvider := selectProvider("claude-cli", "codex-cli", "gemini-cli")
		reporterProvider := selectProvider("gemini-cli", "claude-cli", "codex-cli")
		proposerID := addAgent("proposer-"+shortCLIProviderName(proposerProvider), proposerProvider, "proposer")
		challengerID := addAgent("challenger-"+shortCLIProviderName(challengerProvider), challengerProvider, "challenger")
		verifierID := addAgent("verifier-"+shortCLIProviderName(verifierProvider), verifierProvider, "semantic-verifier")
		arbiterID := addAgent("arbiter-"+shortCLIProviderName(arbiterProvider), arbiterProvider, "arbiter")
		reporterID := addAgent("reporter-"+shortCLIProviderName(reporterProvider), reporterProvider, "reporter")
		markProvider(proposerProvider)
		markProvider(challengerProvider)
		markProvider(verifierProvider)
		markProvider(arbiterProvider)
		markProvider(reporterProvider)
		layout.Roles = config.RolesConfig{
			Proposers:        []string{proposerID},
			Challengers:      []string{challengerID},
			SemanticVerifier: verifierID,
			Arbiter:          arbiterID,
			Reporter:         reporterID,
		}
	}

	for _, provider := range []string{"claude-cli", "gemini-cli", "codex-cli"} {
		if _, ok := usedProviders[provider]; ok {
			layout.ExpectedProviders = append(layout.ExpectedProviders, provider)
		}
	}
	return layout
}

func readyCLIProviderSet(results []cliProviderPreflightResult) map[string]bool {
	ready := make(map[string]bool, len(results))
	for _, result := range results {
		if !result.Ready {
			continue
		}
		if providerID, ok := cliReadinessProviderID(result.Provider); ok {
			ready[providerID] = true
		}
	}
	return ready
}

func orderedReadyCLIProviders(ready map[string]bool) []string {
	ordered := make([]string, 0, len(ready))
	for _, provider := range []string{"claude-cli", "gemini-cli", "codex-cli"} {
		if ready[provider] {
			ordered = append(ordered, provider)
		}
	}
	return ordered
}

func unavailableCLIProviders(results []cliProviderPreflightResult) []string {
	unavailable := make([]string, 0)
	for _, result := range results {
		if result.Ready {
			continue
		}
		if providerID, ok := cliReadinessProviderID(result.Provider); ok {
			unavailable = append(unavailable, providerID)
		}
	}
	return unavailable
}

func shortCLIProviderName(provider string) string {
	switch provider {
	case "claude-cli":
		return "claude"
	case "gemini-cli":
		return "gemini"
	case "codex-cli":
		return "codex"
	default:
		return strings.TrimSuffix(provider, "-cli")
	}
}

func cliReadinessProviderID(provider string) (string, bool) {
	switch strings.TrimSpace(provider) {
	case "claude":
		return "claude-cli", true
	case "gemini":
		return "gemini-cli", true
	case "codex":
		return "codex-cli", true
	default:
		return "", false
	}
}

func buildAPIE2EAgents(mode consensus.WorkflowMode) ([]config.AgentConfig, config.RolesConfig) {
	switch mode {
	case consensus.WorkflowModeFreeDebate:
		return []config.AgentConfig{
				{ID: "participant-openai-a", Provider: "openai-test", Model: "default", Role: "participant"},
				{ID: "participant-anthropic", Provider: "anthropic-test", Model: "default", Role: "participant"},
				{ID: "participant-openai-b", Provider: "openai-test", Model: "default", Role: "participant"},
				{ID: "reporter-openai", Provider: "openai-test", Model: "default", Role: "reporter"},
			}, config.RolesConfig{
				Participants: []string{"participant-openai-a", "participant-anthropic", "participant-openai-b"},
				Reporter:     "reporter-openai",
			}
	case consensus.WorkflowModeDelphi:
		return []config.AgentConfig{
				{ID: "participant-openai-a", Provider: "openai-test", Model: "default", Role: "participant"},
				{ID: "participant-anthropic", Provider: "anthropic-test", Model: "default", Role: "participant"},
				{ID: "participant-openai-b", Provider: "openai-test", Model: "default", Role: "participant"},
				{ID: "facilitator-anthropic", Provider: "anthropic-test", Model: "default", Role: "facilitator"},
				{ID: "reporter-openai", Provider: "openai-test", Model: "default", Role: "reporter"},
			}, config.RolesConfig{
				Participants: []string{"participant-openai-a", "participant-anthropic", "participant-openai-b"},
				Facilitator:  "facilitator-anthropic",
				Reporter:     "reporter-openai",
			}
	default:
		return []config.AgentConfig{
				{ID: "proposer-openai", Provider: "openai-test", Model: "default", Role: "proposer"},
				{ID: "challenger-anthropic", Provider: "anthropic-test", Model: "default", Role: "challenger"},
				{ID: "verifier-openai", Provider: "openai-test", Model: "default", Role: "semantic-verifier"},
				{ID: "arbiter-anthropic", Provider: "anthropic-test", Model: "default", Role: "arbiter"},
				{ID: "reporter-openai", Provider: "openai-test", Model: "default", Role: "reporter"},
			}, config.RolesConfig{
				Proposers:        []string{"proposer-openai"},
				Challengers:      []string{"challenger-anthropic"},
				SemanticVerifier: "verifier-openai",
				Arbiter:          "arbiter-anthropic",
				Reporter:         "reporter-openai",
			}
	}
}

func renderViewSections(sections []string) []string {
	args := make([]string, 0, len(sections)*2)
	for _, section := range sections {
		args = append(args, "--section", section)
	}
	return args
}

type liveTestBuffer struct {
	t       *testing.T
	label   string
	mu      sync.Mutex
	buf     strings.Builder
	pending string
}

func (b *liveTestBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := string(p)
	if _, err := b.buf.WriteString(text); err != nil {
		return 0, err
	}
	b.pending += text
	for {
		idx := strings.IndexByte(b.pending, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(b.pending[:idx], "\r")
		if strings.TrimSpace(line) != "" {
			b.t.Logf("[%s] %s", b.label, line)
		}
		b.pending = b.pending[idx+1:]
	}
	return len(p), nil
}

func (b *liveTestBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func runCLICommandWithLiveLogs(ctx context.Context, t *testing.T, label string, cmd any, args []string) (string, string, error) {
	stdout := &liveTestBuffer{t: t, label: label + "/stdout"}
	stderr := &liveTestBuffer{t: t, label: label + "/stderr"}
	switch typed := cmd.(type) {
	case *cli.Command:
		setCommandWriters(typed, stdout, stderr)
		err := typed.Run(ctx, args)
		flushLiveTestBuffer(t, label+"/stdout", stdout)
		flushLiveTestBuffer(t, label+"/stderr", stderr)
		return stdout.String(), stderr.String(), err
	default:
		return "", "", fmt.Errorf("unsupported command type %T", cmd)
	}
}

func realE2ETimeoutForMode(mode consensus.WorkflowMode) time.Duration {
	if override := strings.TrimSpace(os.Getenv("TIL_CONSENSUS_E2E_REAL_TIMEOUT")); override != "" {
		if parsed, err := time.ParseDuration(override); err == nil && parsed > 0 {
			return parsed
		}
	}
	var key string
	var fallback time.Duration
	switch mode {
	case consensus.WorkflowModeFreeDebate:
		key = "TIL_CONSENSUS_E2E_REAL_TIMEOUT_FREE_DEBATE"
		fallback = 25 * time.Minute
	case consensus.WorkflowModeDelphi:
		key = "TIL_CONSENSUS_E2E_REAL_TIMEOUT_DELPHI"
		fallback = 25 * time.Minute
	default:
		key = "TIL_CONSENSUS_E2E_REAL_TIMEOUT_ADJUDICATION"
		fallback = 15 * time.Minute
	}
	if override := strings.TrimSpace(os.Getenv(key)); override != "" {
		if parsed, err := time.ParseDuration(override); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func preserveE2EWorkspaceIfNeeded(t *testing.T, fixture e2eFixture, staged string, resultPath string, stdout string, stderr string) string {
	t.Helper()
	if !t.Failed() && strings.TrimSpace(os.Getenv("TIL_CONSENSUS_E2E_PRESERVE")) == "" {
		return ""
	}
	dest, err := os.MkdirTemp("", "til-consensus-e2e-"+sanitizeFixtureName(fixture.Name)+"-")
	if err != nil {
		t.Logf("preserve e2e workspace failed: %v", err)
		return ""
	}
	if err := copyDir(staged, dest); err != nil {
		t.Logf("copy preserved workspace failed: %v", err)
		return ""
	}
	if strings.TrimSpace(stdout) != "" {
		_ = os.WriteFile(filepath.Join(dest, "__test_stdout.log"), []byte(stdout), 0o644)
	}
	if strings.TrimSpace(stderr) != "" {
		_ = os.WriteFile(filepath.Join(dest, "__test_stderr.log"), []byte(stderr), 0o644)
	}
	if resultPath != "" {
		relResult, err := filepath.Rel(staged, resultPath)
		if err == nil {
			_ = os.WriteFile(filepath.Join(dest, "__result_path.txt"), []byte(filepath.Join(dest, relResult)+"\n"), 0o644)
		}
	}
	t.Logf("preserved e2e workspace: %s", dest)
	return dest
}

func sanitizeFixtureName(name string) string {
	replacer := strings.NewReplacer("/", "-", " ", "-", "_", "-")
	return replacer.Replace(strings.TrimSpace(name))
}

func tryExtractResultPath(stdout string) string {
	re := regexp.MustCompile(`(?m)^\s*result:\s+(.+?/result\.json)\s*$`)
	match := re.FindStringSubmatch(stdout)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func flushLiveTestBuffer(t *testing.T, label string, buf *liveTestBuffer) {
	t.Helper()
	buf.mu.Lock()
	defer buf.mu.Unlock()
	if strings.TrimSpace(buf.pending) == "" {
		return
	}
	t.Logf("[%s] %s", label, strings.TrimRight(buf.pending, "\r\n"))
	buf.pending = ""
}

func assertE2EResultForFixture(t *testing.T, fixture e2eFixture, result consensus.RunResult) {
	t.Helper()
	if result.Mode != fixture.Manifest.Mode {
		t.Fatalf("unexpected mode: got=%s want=%s", result.Mode, fixture.Manifest.Mode)
	}
	if strings.TrimSpace(result.Report.Summary) == "" {
		t.Fatalf("expected report summary, got %#v", result.Report)
	}
	switch fixture.Manifest.Mode {
	case consensus.WorkflowModeAdjudication:
		if result.Adjudication == nil {
			t.Fatalf("expected adjudication section, got %#v", result)
		}
		if len(result.Adjudication.ClaimGraph) == 0 {
			t.Fatalf("expected adjudication claims, got %#v", result.Adjudication)
		}
		if result.Adjudication.ArbiterReport.TaskVerdict == "" {
			t.Fatalf("expected arbiter verdict, got %#v", result.Adjudication.ArbiterReport)
		}
	case consensus.WorkflowModeFreeDebate:
		if result.FreeDebate == nil {
			t.Fatalf("expected free debate section, got %#v", result)
		}
		if len(result.FreeDebate.Rounds) == 0 || len(result.FreeDebate.Claims) == 0 || len(result.FreeDebate.Votes) == 0 {
			t.Fatalf("expected rounds/claims/votes, got %#v", result.FreeDebate)
		}
	case consensus.WorkflowModeDelphi:
		if result.Delphi == nil {
			t.Fatalf("expected delphi section, got %#v", result)
		}
		if len(result.Delphi.Rounds) == 0 || len(result.Delphi.Statements) == 0 || strings.TrimSpace(result.Delphi.Recommendation) == "" {
			t.Fatalf("expected rounds/statements/recommendation, got %#v", result.Delphi)
		}
	default:
		t.Fatalf("unsupported fixture mode: %s", fixture.Manifest.Mode)
	}
}

func loadRealCLIProviderReadiness(t *testing.T) []cliProviderPreflightResult {
	t.Helper()
	realCLIPreflightOnce.Do(func() {
		realCLIPreflightResults = runRealCLIProviderPreflight()
	})
	return realCLIPreflightResults
}

func logRealCLIProviderReadiness(t *testing.T, results []cliProviderPreflightResult) {
	t.Helper()
	for _, result := range results {
		t.Logf("provider readiness: provider=%s ready=%t duration=%s strict_json=%t recoverable_json=%t command=%s stdout=%q stderr=%q error=%q",
			result.Provider,
			result.Ready,
			result.Duration.Round(time.Millisecond),
			result.StrictJSON,
			result.RecoverableJSON,
			strings.Join(result.Command, " "),
			result.StdoutPreview,
			result.StderrPreview,
			result.Error,
		)
	}
}

func countReadyCLIProviders(results []cliProviderPreflightResult) int {
	count := 0
	for _, result := range results {
		if result.Ready {
			count++
		}
	}
	return count
}

func runRealCLIProviderPreflight() []cliProviderPreflightResult {
	const prompt = `只返回一个 JSON 对象：{"ok":true}`
	specs := []struct {
		provider string
		command  string
	}{
		{provider: "claude", command: "claude"},
		{provider: "gemini", command: "gemini"},
		{provider: "codex", command: "codex"},
	}
	results := make([]cliProviderPreflightResult, 0, len(specs))
	for _, spec := range specs {
		results = append(results, probeCLIProviderReadiness(spec.provider, spec.command, prompt))
	}
	return results
}

func probeCLIProviderReadiness(provider string, command string, prompt string) cliProviderPreflightResult {
	result := cliProviderPreflightResult{
		Provider: provider,
	}
	args, stdin, cleanup, err := buildCLIProviderPreflightArgs(provider, prompt)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer cleanup()
	result.Command = append([]string{command}, args...)
	ctx, cancel := context.WithTimeout(context.Background(), realCLIPreflightTimeout())
	defer cancel()

	cmd := exec.Command(command, args...)
	cmd.Env = os.Environ()
	if provider != "claude" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	startedAt := time.Now()
	runErr := runCommandWithProcessGroupTimeout(ctx, cmd)
	result.Duration = time.Since(startedAt)
	rawStdout := strings.TrimSpace(stdout.String())
	rawStderr := strings.TrimSpace(stderr.String())
	result.StdoutPreview = previewText(rawStdout, 220)
	result.StderrPreview = previewText(rawStderr, 220)
	normalizedStdout := normalizePreflightOutput(provider, rawStdout)

	if strict, strictErr := tilruntime.StrictJSONObjectBytes(normalizedStdout); strictErr == nil && len(strict) > 0 {
		result.StrictJSON = true
		result.RecoverableJSON = true
	} else if _, parseErr := tilruntime.ParseJSONObject(normalizedStdout); parseErr == nil {
		result.RecoverableJSON = true
	}

	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Sprintf("timed out after %s", realCLIPreflightTimeout())
		} else if errors.Is(runErr, exec.ErrNotFound) {
			result.Error = fmt.Sprintf("binary %s not found", command)
		} else {
			result.Error = runErr.Error()
		}
		return result
	}
	if !result.RecoverableJSON {
		result.Error = "command succeeded but did not return a recoverable JSON object"
		return result
	}
	result.Ready = true
	return result
}

func buildCLIProviderPreflightArgs(provider string, prompt string) ([]string, string, func(), error) {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"ok"},
		"properties": map[string]any{
			"ok": map[string]any{
				"type": "boolean",
			},
		},
	}
	switch provider {
	case "claude":
		body, err := json.Marshal(schema)
		if err != nil {
			return nil, "", nil, fmt.Errorf("marshal claude preflight schema: %w", err)
		}
		return []string{"--print", "--model", "claude-opus-4-6", "--json-schema", string(body), "--output-format", "json"}, prompt, func() {}, nil
	case "codex":
		body, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return nil, "", nil, fmt.Errorf("marshal codex preflight schema: %w", err)
		}
		file, err := os.CreateTemp("", "til-consensus-e2e-codex-schema-*.json")
		if err != nil {
			return nil, "", nil, fmt.Errorf("create codex preflight schema temp file: %w", err)
		}
		if _, err := file.Write(append(body, '\n')); err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
			return nil, "", nil, fmt.Errorf("write codex preflight schema temp file: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(file.Name())
			return nil, "", nil, fmt.Errorf("close codex preflight schema temp file: %w", err)
		}
		return []string{"exec", "-m", "gpt-5.4", "--full-auto", "--color", "never", "--skip-git-repo-check", "--output-schema", file.Name()}, prompt, func() { _ = os.Remove(file.Name()) }, nil
	case "gemini":
		return []string{"--approval-mode", "yolo", "-m", "gemini-3.1-pro-preview"}, prompt, func() {}, nil
	default:
		return nil, prompt, func() {}, nil
	}
}

func normalizePreflightOutput(provider string, raw string) string {
	if provider != "claude" {
		return raw
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return raw
	}
	if structured, ok := envelope["structured_output"]; ok && structured != nil {
		body, err := json.Marshal(structured)
		if err == nil {
			return string(body)
		}
	}
	if result, ok := envelope["result"].(string); ok && strings.TrimSpace(result) != "" {
		return result
	}
	return raw
}

func runCommandWithProcessGroupTimeout(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if cmd.Process != nil {
			if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			} else {
				_ = cmd.Process.Kill()
			}
		}
		<-done
		return ctx.Err()
	}
}

func realCLIPreflightTimeout() time.Duration {
	if override := strings.TrimSpace(os.Getenv("TIL_CONSENSUS_E2E_REAL_PREFLIGHT_TIMEOUT")); override != "" {
		if parsed, err := time.ParseDuration(override); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 90 * time.Second
}

func previewText(text string, max int) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max] + "..."
}

func formatPreflightFailureReason(result cliProviderPreflightResult) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(result.Error) != "" {
		parts = append(parts, strings.TrimSpace(result.Error))
	}
	if strings.TrimSpace(result.StderrPreview) != "" {
		parts = append(parts, "stderr="+strings.TrimSpace(result.StderrPreview))
	}
	if strings.TrimSpace(result.StdoutPreview) != "" {
		parts = append(parts, "stdout="+strings.TrimSpace(result.StdoutPreview))
	}
	if len(parts) == 0 {
		return "unknown readiness failure"
	}
	return strings.Join(parts, " | ")
}

func startE2EAPIServers(t *testing.T, mode consensus.WorkflowMode, openAIHits *int, anthropicHits *int) (*httptest.Server, *httptest.Server) {
	t.Helper()
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected openai-compatible path: %s", r.URL.Path)
		}
		(*openAIHits)++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode openai-compatible request: %v", err)
		}
		if _, ok := body["response_format"].(map[string]any); !ok {
			t.Fatalf("expected response_format json_schema in openai-compatible request, got %#v", body)
		}
		prompt := extractUserPrompt(t, body["messages"])
		response := e2eAPIResponse(mode, prompt)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": response},
			}},
		})
	}))

	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("unexpected anthropic-compatible path: %s", r.URL.Path)
		}
		(*anthropicHits)++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode anthropic-compatible request: %v", err)
		}
		prompt := extractAnthropicPrompt(t, body["messages"])
		response := e2eAPIResponse(mode, prompt)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": response},
			},
		})
	}))

	return openAIServer, anthropicServer
}

func e2eAPIResponse(mode consensus.WorkflowMode, prompt string) string {
	switch mode {
	case consensus.WorkflowModeFreeDebate:
		return freeDebateTestResponse(prompt)
	case consensus.WorkflowModeDelphi:
		return delphiFixtureResponse(prompt)
	default:
		return adjudicationFixtureResponse(prompt)
	}
}

func adjudicationFixtureResponse(prompt string) string {
	switch {
	case strings.Contains(prompt, `"ledger"`) && strings.Contains(prompt, `"decisions"`):
		return `{"summary":"综合 claims、challenge 和 verification 后，建议保留渐进迁移方向，但必须显式保留前提与边界。","taskVerdict":"partially_supported","decisions":[{"claimId":"` + firstExtractedID(extractClaimIDs(prompt), "claim-adj-1") + `","verdict":"supported","confidence":0.72,"rationale":"当前材料支持将渐进式 monorepo 作为候选方向，但前提是先补齐构建与权限边界。","evidenceRefs":["material:current-state","material:engineering-constraints"]}]}`
	case strings.Contains(prompt, `"arbiter"`) && strings.Contains(prompt, `"highlights"`):
		return `{"summary":"结论是：可以评估渐进式 monorepo，但不能忽略 CI、权限和团队边界前提。","highlights":["跨仓库重构成本已经偏高","平台人力有限，不能做一次性大迁移"],"unresolvedQuestions":["是否已有可承载 monorepo 的增量构建与缓存能力"],"nextActions":["先验证增量构建、权限隔离和迁移试点"]}`
	case strings.Contains(prompt, `"findings"`) && strings.Contains(prompt, `"revisions"`):
		claimIDs := extractClaimIDs(prompt)
		if len(claimIDs) == 0 {
			return `{"summary":"当前无需修订。","revisions":[]}`
		}
		return `{"summary":"保留主张，但要求在最终结论里显式写出迁移前提。","revisions":[{"targetClaimId":"` + claimIDs[0] + `","action":"unchanged","reason":"当前表述已经围绕渐进迁移与前提约束展开，无需改写。"}]}`
	case strings.Contains(prompt, `"results"`) && strings.Contains(prompt, `"claim"`):
		results := make([]string, 0)
		for _, claimID := range extractClaimIDs(prompt) {
			results = append(results, fmt.Sprintf(`{"claimId":"%s","verdict":"supported","confidence":0.68,"rationale":"该主张与材料中的仓库规模、迁移约束和平台人力情况一致。","metadata":{"rawVerdict":"supported"}}`, claimID))
		}
		if len(results) == 0 {
			results = append(results, `{"claimId":"claim-adj-1","verdict":"supported","confidence":0.68,"rationale":"该主张与材料中的仓库规模、迁移约束和平台人力情况一致。","metadata":{"rawVerdict":"supported"}}`)
		}
		return `{"summary":"给出 claim 级语义核验。","results":[` + strings.Join(results, ",") + `]}`
	case strings.Contains(prompt, `"tickets"`) && strings.Contains(prompt, `"claims"`):
		claimIDs := extractClaimIDs(prompt)
		if len(claimIDs) == 0 {
			return `{"summary":"当前没有新增 challenge。","tickets":[]}`
		}
		return `{"summary":"提出一条关于适用前提的 challenge。","tickets":[{"claimId":"` + claimIDs[0] + `","statement":"需要明确 monorepo 只适用于先补齐增量构建与权限边界的前提。","kind":"scope_gap","attackType":"scope","severity":"medium"}]}`
	default:
		return `{"summary":"提出一条面向裁决的主张。","claims":[{"statement":"如果跨服务改动频繁且依赖漂移明显，团队应优先评估渐进式 monorepo，但前提是先补齐增量构建、缓存和权限边界。","claimType":"recommendation","confidence":0.74},{"statement":"如果当前构建基础设施和所有权边界尚未准备好，继续维持 polyrepo 并先修复版本漂移更稳妥。","claimType":"recommendation","confidence":0.69}]}`
	}
}

func delphiFixtureResponse(prompt string) string {
	switch {
	case strings.Contains(prompt, `"recommendation"`) && strings.Contains(prompt, `"dissentSummary"`):
		return `{"summary":"当前匿名收敛结果倾向于部分推荐迁移。","recommendation":"部分推荐在未来两个季度内启动分阶段迁移：先迁低风险仓库，再根据审计与 runner 隔离验证结果扩大范围。","dissentSummary":["仍有参与者担心核心流水线切换窗口不足。"],"statements":[{"statementId":"stmt-delphi-1","statement":"先迁低风险仓库，再视审计和 runner 隔离验证结果扩大迁移范围。","meanRating":4.3,"consensusLevel":0.82,"responseCount":3,"lastRound":2,"representativeReasons":["渐进迁移风险更低。","能兼顾审计与凭据边界。"]}]}`
	case strings.Contains(prompt, `"statementSummaries"`) && strings.Contains(prompt, `"responses"`):
		statementIDs := extractStatementIDs(prompt)
		if len(statementIDs) == 0 {
			return `{"summary":"在上一轮基础上维持谨慎支持。","responses":[{"statement":"先迁低风险仓库，再视审计能力扩围。","rating":4,"rationale":"相比一次性迁移，双轨试点更稳。"}]}`
		}
		responses := make([]string, 0, len(statementIDs))
		for _, statementID := range statementIDs {
			responses = append(responses, fmt.Sprintf(`{"statementId":"%s","rating":4,"rationale":"继续维持对分阶段迁移的谨慎支持。","metadata":{"rawVerdict":"stable"}}`, statementID))
		}
		return `{"summary":"在匿名汇总基础上维持谨慎支持。","responses":[` + strings.Join(responses, ",") + `]}`
	case strings.Contains(prompt, `"questionnaire"`) && strings.Contains(prompt, `"responses"`):
		return `{"summary":"给出两条 Delphi 候选结论。","responses":[{"statement":"优先采用双轨迁移：低风险仓库先迁入 GitHub Actions，核心流水线继续留在 Jenkins 直到审计与 runner 隔离通过验证。","rating":4,"rationale":"这样可以降低一次性迁移风险。"},{"statement":"如果两个季度内无法补齐审计、runner 隔离和 secret 管理能力，就不应扩大迁移范围。","rating":5,"rationale":"合规和凭据边界是迁移前置条件。"}]}`
	default:
		return `{"summary":"最终建议是分阶段迁移，并把审计、runner 隔离和凭据边界作为扩大范围的前置门槛。","highlights":["平台团队人力有限，更适合渐进推进","需要优先验证审计与凭据边界"],"unresolvedQuestions":["核心流水线何时适合迁移"],"nextActions":["先选低风险仓库开展双轨试点"]}`
	}
}

func extractStatementIDs(prompt string) []string {
	re := regexp.MustCompile(`"statementId"\s*:\s*"([^"]+)"`)
	matches := re.FindAllStringSubmatch(prompt, -1)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		statementID := strings.TrimSpace(match[1])
		if statementID == "" {
			continue
		}
		if _, ok := seen[statementID]; ok {
			continue
		}
		seen[statementID] = struct{}{}
		out = append(out, statementID)
	}
	return out
}

func firstExtractedID(ids []string, fallback string) string {
	if len(ids) > 0 {
		return ids[0]
	}
	return fallback
}

func loadComplianceSummary(t *testing.T, resultPath string) complianceSummaryDoc {
	t.Helper()
	path := filepath.Join(filepath.Dir(resultPath), "artifacts", "strict-compliance-summary.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read compliance summary failed: %v", err)
	}
	var summary complianceSummaryDoc
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("decode compliance summary failed: %v\n%s", err, string(body))
	}
	if len(summary.Entries) == 0 {
		t.Fatalf("expected compliance summary entries, got %#v", summary)
	}
	return summary
}

func assertComplianceSummary(t *testing.T, summary complianceSummaryDoc, providerType string, providers []string) {
	t.Helper()
	seenProviders := map[string]struct{}{}
	for _, entry := range summary.Entries {
		if entry.ProviderType != providerType {
			t.Fatalf("unexpected provider type in compliance summary: %#v", entry)
		}
		if entry.Total <= 0 {
			t.Fatalf("expected positive compliance total, got %#v", entry)
		}
		if entry.Strict+entry.Normalized+entry.Repaired+entry.Failed <= 0 {
			t.Fatalf("expected compliance counts, got %#v", entry)
		}
		seenProviders[entry.Provider] = struct{}{}
	}
	for _, provider := range providers {
		if _, ok := seenProviders[provider]; !ok {
			t.Fatalf("expected provider %s in compliance summary, got %#v", provider, summary.Entries)
		}
	}
}
