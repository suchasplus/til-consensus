package app

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
)

type dryRunPlan struct {
	Source      string                    `json:"source"`
	ConfigPath  string                    `json:"configPath"`
	RequestID   string                    `json:"requestId"`
	Mode        consensus.WorkflowMode    `json:"mode"`
	TaskPreview string                    `json:"taskPreview"`
	Roles       consensus.RoleAssignments `json:"roles"`
	Agents      []dryRunAgent             `json:"agents"`
	Phases      []string                  `json:"phases"`
	Notices     []string                  `json:"notices,omitempty"`
	Output      dryRunOutput              `json:"output"`
	Policies    dryRunPolicies            `json:"policies"`
}

type dryRunAgent struct {
	ID            string   `json:"id"`
	Role          string   `json:"role,omitempty"`
	AssignedRoles []string `json:"assignedRoles,omitempty"`
	Provider      string   `json:"provider"`
	ProviderType  string   `json:"providerType,omitempty"`
	Protocol      string   `json:"protocol,omitempty"`
	CLIType       string   `json:"cliType,omitempty"`
	ModelID       string   `json:"modelId,omitempty"`
	ProviderModel string   `json:"providerModel,omitempty"`
	BaseURL       string   `json:"baseUrl,omitempty"`
	APIKeyEnv     string   `json:"apiKeyEnv,omitempty"`
	Command       []string `json:"command,omitempty"`
}

type dryRunOutput struct {
	RunDir          string `json:"runDir"`
	ResultPath      string `json:"resultPath"`
	SummaryPath     string `json:"summaryPath"`
	LedgerPath      string `json:"ledgerPath"`
	EventsPath      string `json:"eventsPath"`
	ManifestPath    string `json:"manifestPath"`
	ArtifactsDir    string `json:"artifactsDir"`
	SessionStoreDir string `json:"sessionStoreDir"`
}

type dryRunPolicies struct {
	PerTaskTimeout         string   `json:"perTaskTimeout,omitempty"`
	GlobalDeadline         string   `json:"globalDeadline,omitempty"`
	RetryAttempts          int      `json:"retryAttempts"`
	VerificationChecks     []string `json:"verificationChecks,omitempty"`
	AllowSemanticVerifier  bool     `json:"allowSemanticVerifier,omitempty"`
	DebateMinRounds        int      `json:"debateMinRounds,omitempty"`
	DebateMaxRounds        int      `json:"debateMaxRounds,omitempty"`
	SupportThreshold       float64  `json:"supportThreshold,omitempty"`
	VoteAggregation        string   `json:"voteAggregation,omitempty"`
	VoteQuorum             float64  `json:"voteQuorum,omitempty"`
	MaxNewClaimsPerRound   int      `json:"maxNewClaimsPerRound,omitempty"`
	MaxActiveClaims        int      `json:"maxActiveClaims,omitempty"`
	SemanticDedup          bool     `json:"semanticDedup,omitempty"`
	SemanticDedupThreshold float64  `json:"semanticDedupThreshold,omitempty"`
	SemanticDedupCadence   string   `json:"semanticDedupCadence,omitempty"`
	DelphiMinRounds        int      `json:"delphiMinRounds,omitempty"`
	DelphiMaxRounds        int      `json:"delphiMaxRounds,omitempty"`
	ConvergenceThreshold   float64  `json:"convergenceThreshold,omitempty"`
}

func writeDryRunPlan(writer io.Writer, loaded config.LoadedConfig, plan config.ResolvedRunPlan, source string, format string) error {
	payload := buildDryRunPlan(loaded, plan, source)
	switch strings.TrimSpace(format) {
	case "", "text":
		_, _ = fmt.Fprint(writer, renderDryRunText(payload))
	case "json":
		body, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal dry-run plan: %w", err)
		}
		_, _ = fmt.Fprintln(writer, string(body))
	default:
		return appError(ExitUsageError, "unsupported dry-run format: "+format, "使用 --format text 或 --format json", nil)
	}
	return nil
}

func buildDryRunPlan(loaded config.LoadedConfig, plan config.ResolvedRunPlan, source string) dryRunPlan {
	assigned := assignedRolesByConsensusAgent(plan.Roles)
	agentIDs := orderedPlanAgentIDs(plan.Roles)
	agentsByID := make(map[string]config.AgentConfig, len(loaded.Config.Agents))
	for _, agent := range loaded.Config.Agents {
		agentsByID[agent.ID] = agent
	}
	agents := make([]dryRunAgent, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		agent, ok := agentsByID[agentID]
		if !ok {
			continue
		}
		provider := loaded.Config.Providers[agent.Provider]
		model := provider.Models[agent.Model]
		item := dryRunAgent{
			ID:            agent.ID,
			Role:          agent.Role,
			AssignedRoles: assigned[agent.ID],
			Provider:      agent.Provider,
			ProviderType:  provider.Type,
			Protocol:      provider.Protocol,
			CLIType:       provider.CLIType,
			ModelID:       agent.Model,
			ProviderModel: firstNonEmptyApp(model.ProviderModel, agent.Model, provider.Model),
			BaseURL:       provider.BaseURL,
			APIKeyEnv:     provider.APIKeyEnv,
		}
		if provider.Command != "" || len(provider.Args) > 0 {
			item.Command = append([]string{provider.Command}, provider.Args...)
		}
		agents = append(agents, item)
	}
	checks := make([]string, 0, len(plan.StartRequest.VerificationPolicy.RequiredChecks))
	for _, check := range plan.StartRequest.VerificationPolicy.RequiredChecks {
		checks = append(checks, firstNonEmptyApp(check.Name, check.Kind))
	}
	return dryRunPlan{
		Source:      source,
		ConfigPath:  loaded.Path,
		RequestID:   plan.RequestID,
		Mode:        plan.Mode,
		TaskPreview: previewRunTask(plan.Task),
		Roles:       plan.Roles,
		Agents:      agents,
		Phases:      phasesForMode(plan.Mode, plan.StartRequest.ActionPolicy != nil),
		Notices:     plan.Notices,
		Output: dryRunOutput{
			RunDir:          filepath.Dir(plan.ResultPath),
			ResultPath:      plan.ResultPath,
			SummaryPath:     plan.SummaryPath,
			LedgerPath:      plan.LedgerPath,
			EventsPath:      plan.EventsPath,
			ManifestPath:    plan.ManifestPath,
			ArtifactsDir:    plan.ArtifactsDir,
			SessionStoreDir: plan.SessionStoreDir,
		},
		Policies: dryRunPolicies{
			PerTaskTimeout:         plan.StartRequest.WaitingPolicy.PerTaskTimeout.String(),
			GlobalDeadline:         plan.StartRequest.WaitingPolicy.GlobalDeadline.String(),
			RetryAttempts:          plan.StartRequest.WaitingPolicy.RetryAttempts,
			VerificationChecks:     checks,
			AllowSemanticVerifier:  plan.StartRequest.VerificationPolicy.AllowSemanticVerifier,
			DebateMinRounds:        plan.StartRequest.DebatePolicy.MinRounds,
			DebateMaxRounds:        plan.StartRequest.DebatePolicy.MaxRounds,
			SupportThreshold:       plan.StartRequest.DebatePolicy.VoteThreshold,
			VoteAggregation:        string(plan.StartRequest.DebatePolicy.VoteAggregation),
			VoteQuorum:             plan.StartRequest.DebatePolicy.VoteQuorum,
			MaxNewClaimsPerRound:   plan.StartRequest.DebatePolicy.MaxNewClaimsPerRound,
			MaxActiveClaims:        plan.StartRequest.DebatePolicy.MaxActiveClaims,
			SemanticDedup:          plan.StartRequest.DebatePolicy.SemanticDedup.Enabled,
			SemanticDedupThreshold: plan.StartRequest.DebatePolicy.SemanticDedup.SimilarityThreshold,
			SemanticDedupCadence:   string(plan.StartRequest.DebatePolicy.SemanticDedup.Cadence),
			DelphiMinRounds:        plan.StartRequest.DelphiPolicy.MinRounds,
			DelphiMaxRounds:        plan.StartRequest.DelphiPolicy.MaxRounds,
			ConvergenceThreshold:   plan.StartRequest.DelphiPolicy.ConvergenceThreshold,
		},
	}
}

func renderDryRunText(plan dryRunPlan) string {
	var b strings.Builder
	b.WriteString("[til-consensus] dry run\n")
	b.WriteString("  source: " + plan.Source + "\n")
	b.WriteString("  config: " + plan.ConfigPath + "\n")
	b.WriteString("  requestId: " + plan.RequestID + "\n")
	b.WriteString("  mode: " + string(plan.Mode) + "\n")
	b.WriteString("  task: " + plan.TaskPreview + "\n")
	b.WriteString("  phases: " + strings.Join(plan.Phases, " -> ") + "\n")
	for _, notice := range plan.Notices {
		b.WriteString("  notice: " + notice + "\n")
	}
	b.WriteString("\nroles\n")
	b.WriteString("  proposers: " + strings.Join(plan.Roles.Proposers, ",") + "\n")
	b.WriteString("  challengers: " + strings.Join(plan.Roles.Challengers, ",") + "\n")
	b.WriteString("  participants: " + strings.Join(plan.Roles.Participants, ",") + "\n")
	b.WriteString("  semantic_verifier: " + plan.Roles.SemanticVerifier + "\n")
	b.WriteString("  semantic_deduper: " + plan.Roles.SemanticDeduper + "\n")
	b.WriteString("  arbiter: " + plan.Roles.Arbiter + "\n")
	b.WriteString("  facilitator: " + plan.Roles.Facilitator + "\n")
	b.WriteString("  reporter: " + plan.Roles.Reporter + "\n")
	b.WriteString("  actor: " + plan.Roles.Actor + "\n")
	b.WriteString("\nagents\n")
	for _, agent := range plan.Agents {
		b.WriteString("  - " + agent.ID + " provider=" + agent.Provider + " model=" + agent.ModelID)
		if agent.ProviderModel != "" {
			b.WriteString(" providerModel=" + agent.ProviderModel)
		}
		if len(agent.AssignedRoles) > 0 {
			b.WriteString(" assigned=" + strings.Join(agent.AssignedRoles, ","))
		}
		b.WriteString("\n")
	}
	b.WriteString("\noutput\n")
	b.WriteString("  result: " + plan.Output.ResultPath + "\n")
	b.WriteString("  summary: " + plan.Output.SummaryPath + "\n")
	b.WriteString("  artifacts: " + plan.Output.ArtifactsDir + "\n")
	b.WriteString("  sessions: " + plan.Output.SessionStoreDir + "\n")
	b.WriteString("\npolicies\n")
	_, _ = fmt.Fprintf(&b, "  timeout: perTask=%s global=%s retries=%d\n", plan.Policies.PerTaskTimeout, plan.Policies.GlobalDeadline, plan.Policies.RetryAttempts)
	if len(plan.Policies.VerificationChecks) > 0 {
		b.WriteString("  verification: " + strings.Join(plan.Policies.VerificationChecks, ",") + "\n")
	}
	return b.String()
}

func orderedPlanAgentIDs(roles consensus.RoleAssignments) []string {
	out := []string{}
	seen := map[string]struct{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range roles.Proposers {
		add(id)
	}
	for _, id := range roles.Challengers {
		add(id)
	}
	for _, id := range roles.Participants {
		add(id)
	}
	add(roles.SemanticVerifier)
	add(roles.SemanticDeduper)
	add(roles.Arbiter)
	add(roles.Facilitator)
	add(roles.Reporter)
	add(roles.Actor)
	return out
}

func assignedRolesByConsensusAgent(roles consensus.RoleAssignments) map[string][]string {
	out := map[string][]string{}
	add := func(agentID string, role string) {
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			return
		}
		out[agentID] = append(out[agentID], role)
	}
	for _, id := range roles.Proposers {
		add(id, "proposer")
	}
	for _, id := range roles.Challengers {
		add(id, "challenger")
	}
	for _, id := range roles.Participants {
		add(id, "participant")
	}
	add(roles.SemanticVerifier, "semantic_verifier")
	add(roles.SemanticDeduper, "semantic_deduper")
	add(roles.Arbiter, "arbiter")
	add(roles.Facilitator, "facilitator")
	add(roles.Reporter, "reporter")
	add(roles.Actor, "actor")
	return out
}

func phasesForMode(mode consensus.WorkflowMode, hasAction bool) []string {
	var phases []string
	switch mode {
	case consensus.WorkflowModeFreeDebate:
		phases = []string{"frame", "ingest", "initial", "debate", "final_vote", "report"}
	case consensus.WorkflowModeDelphi:
		phases = []string{"frame", "ingest", "questionnaire", "revise", "facilitate", "report"}
	default:
		phases = []string{"frame", "ingest", "propose", "challenge", "verify", "revise", "adjudicate", "report"}
	}
	if hasAction {
		phases = append(phases, "action")
	}
	return append(phases, "observe")
}

func previewRunTask(task string) string {
	task = strings.Join(strings.Fields(task), " ")
	if len(task) <= 180 {
		return task
	}
	return task[:177] + "..."
}

func firstNonEmptyApp(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
