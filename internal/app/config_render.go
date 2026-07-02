package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

type configExplainReport struct {
	ConfigPath   string                     `json:"configPath"`
	ConfigDir    string                     `json:"configDir"`
	Profile      string                     `json:"profile,omitempty"`
	IncludeTrace []config.IncludeTraceEntry `json:"includeTrace,omitempty"`
	Mode         consensus.WorkflowMode     `json:"mode"`
	Providers    []configExplainProvider    `json:"providers"`
	Agents       []configExplainAgent       `json:"agents"`
	Roles        config.RolesConfig         `json:"roles"`
	Output       configExplainOutput        `json:"output"`
	Warnings     []string                   `json:"warnings,omitempty"`
}

type configExplainProvider struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Protocol  string   `json:"protocol,omitempty"`
	CLIType   string   `json:"cliType,omitempty"`
	Command   []string `json:"command,omitempty"`
	BaseURL   string   `json:"baseUrl,omitempty"`
	APIKeyEnv string   `json:"apiKeyEnv,omitempty"`
	Models    []string `json:"models,omitempty"`
}

type configExplainAgent struct {
	ID            string   `json:"id"`
	Role          string   `json:"role,omitempty"`
	AssignedRoles []string `json:"assignedRoles,omitempty"`
	Provider      string   `json:"provider"`
	ProviderType  string   `json:"providerType,omitempty"`
	ModelID       string   `json:"modelId,omitempty"`
	ProviderModel string   `json:"providerModel,omitempty"`
	Timeout       string   `json:"timeout,omitempty"`
}

type configExplainOutput struct {
	RunDir          string `json:"runDir"`
	ResultTemplate  string `json:"resultTemplate"`
	SessionStoreDir string `json:"sessionStoreDir"`
}

func newConfigRenderCommand() *cli.Command {
	return &cli.Command{
		Name:  "render",
		Usage: "渲染 include/overlay 后的最终配置",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "format", Usage: "输出格式(yaml|json)", Value: "yaml"},
			&cli.BoolFlag{Name: "profiles-only", Usage: "只校验 provider / agent profile，不校验完整 workflow roles"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path, err := config.ResolveConfigPath(cmd.String("config"))
			if err != nil {
				return err
			}
			loaded, err := config.LoadWithTraceAndProfile(path, cmd.Bool("profiles-only"), cmd.String("profile"))
			if err != nil {
				return err
			}
			switch strings.TrimSpace(cmd.String("format")) {
			case "", "yaml":
				body, err := yaml.Marshal(loaded.Config)
				if err != nil {
					return fmt.Errorf("marshal rendered config: %w", err)
				}
				_, _ = cmd.Writer.Write(body)
			case "json":
				body, err := marshalYAMLTaggedJSON(loaded.Config)
				if err != nil {
					return fmt.Errorf("marshal rendered config: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.Writer, string(body))
			default:
				return appError(ExitUsageError, "unsupported config render format: "+cmd.String("format"), "使用 --format yaml 或 --format json", nil)
			}
			return nil
		},
	}
}

func marshalYAMLTaggedJSON(value any) ([]byte, error) {
	yamlBody, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := yaml.Unmarshal(yamlBody, &decoded); err != nil {
		return nil, err
	}
	return json.MarshalIndent(decoded, "", "  ")
}

func newConfigExplainCommand() *cli.Command {
	return &cli.Command{
		Name:  "explain",
		Usage: "解释最终生效的 provider、agent、roles 和输出路径",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "format", Usage: "输出格式(text|json)", Value: "text"},
			&cli.StringFlag{Name: "provider", Usage: "只展示指定 provider"},
			&cli.StringFlag{Name: "agent", Usage: "只展示指定 agent"},
			&cli.BoolFlag{Name: "profiles-only", Usage: "只校验 provider / agent profile，不校验完整 workflow roles"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path, err := config.ResolveConfigPath(cmd.String("config"))
			if err != nil {
				return err
			}
			loaded, err := config.LoadWithTraceAndProfile(path, cmd.Bool("profiles-only"), cmd.String("profile"))
			if err != nil {
				return err
			}
			report := buildConfigExplainReport(loaded, cmd.String("provider"), cmd.String("agent"))
			switch strings.TrimSpace(cmd.String("format")) {
			case "", "text":
				_, _ = fmt.Fprint(cmd.Writer, renderConfigExplainText(report))
			case "json":
				body, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal config explain report: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.Writer, string(body))
			default:
				return appError(ExitUsageError, "unsupported config explain format: "+cmd.String("format"), "使用 --format text 或 --format json", nil)
			}
			return nil
		},
	}
}

func buildConfigExplainReport(loaded config.LoadedConfig, providerFilter string, agentFilter string) configExplainReport {
	providerFilter = strings.TrimSpace(providerFilter)
	agentFilter = strings.TrimSpace(agentFilter)
	report := configExplainReport{
		ConfigPath:   loaded.Path,
		ConfigDir:    loaded.ConfigDir,
		Profile:      loaded.Profile,
		IncludeTrace: loaded.IncludeTrace,
		Mode:         loaded.Config.Defaults.Mode,
		Roles:        loaded.Config.Roles,
		Output: configExplainOutput{
			RunDir:          config.ResolveRunArtifacts(loaded, "{requestId}").RunDir,
			ResultTemplate:  config.ResolveResultTemplate(loaded),
			SessionStoreDir: config.ResolveSessionStoreDir(loaded),
		},
	}
	providerIDs := make([]string, 0, len(loaded.Config.Providers))
	for id := range loaded.Config.Providers {
		providerIDs = append(providerIDs, id)
	}
	sort.Strings(providerIDs)
	for _, id := range providerIDs {
		if providerFilter != "" && providerFilter != id {
			continue
		}
		provider := loaded.Config.Providers[id]
		item := configExplainProvider{
			ID:        id,
			Type:      provider.Type,
			Protocol:  provider.Protocol,
			CLIType:   provider.CLIType,
			BaseURL:   provider.BaseURL,
			APIKeyEnv: provider.APIKeyEnv,
			Models:    config.ModelIDs(provider),
		}
		if provider.Command != "" || len(provider.Args) > 0 {
			item.Command = append([]string{provider.Command}, provider.Args...)
		}
		report.Providers = append(report.Providers, item)
	}
	assigned := assignedRolesByAgent(loaded.Config.Roles)
	for _, agent := range loaded.Config.Agents {
		if agentFilter != "" && agentFilter != agent.ID {
			continue
		}
		provider := loaded.Config.Providers[agent.Provider]
		model := provider.Models[agent.Model]
		item := configExplainAgent{
			ID:            agent.ID,
			Role:          agent.Role,
			AssignedRoles: assigned[agent.ID],
			Provider:      agent.Provider,
			ProviderType:  provider.Type,
			ModelID:       agent.Model,
			ProviderModel: firstNonEmptyApp(model.ProviderModel, agent.Model, provider.Model),
		}
		if agent.Timeout.Duration > 0 {
			item.Timeout = agent.Timeout.String()
		}
		report.Agents = append(report.Agents, item)
	}
	if providerFilter != "" && len(report.Providers) == 0 {
		report.Warnings = append(report.Warnings, "provider not found: "+providerFilter)
	}
	if agentFilter != "" && len(report.Agents) == 0 {
		report.Warnings = append(report.Warnings, "agent not found: "+agentFilter)
	}
	return report
}

func assignedRolesByAgent(roles config.RolesConfig) map[string][]string {
	out := map[string][]string{}
	add := func(agentID string, role string) {
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			return
		}
		out[agentID] = append(out[agentID], role)
	}
	roles = config.Normalize(config.Config{Roles: roles}).Roles
	for _, id := range roles.Adjudication.Proposers {
		add(id, "adjudication.proposer")
	}
	for _, id := range roles.Adjudication.Challengers {
		add(id, "adjudication.challenger")
	}
	add(roles.Adjudication.Arbiter, "adjudication.arbiter")
	add(roles.Adjudication.SemanticVerifier, "adjudication.semantic_verifier")
	add(roles.Adjudication.Reporter, "adjudication.reporter")
	add(roles.Adjudication.Actor, "adjudication.actor")
	for _, id := range roles.FreeDebate.Participants {
		add(id, "free_debate.participant")
	}
	add(roles.FreeDebate.SemanticDeduper, "free_debate.semantic_deduper")
	add(roles.FreeDebate.Reporter, "free_debate.reporter")
	add(roles.FreeDebate.Actor, "free_debate.actor")
	for _, id := range roles.Delphi.Participants {
		add(id, "delphi.participant")
	}
	add(roles.Delphi.Facilitator, "delphi.facilitator")
	add(roles.Delphi.Reporter, "delphi.reporter")
	add(roles.Delphi.Actor, "delphi.actor")
	return out
}

func renderConfigExplainText(report configExplainReport) string {
	var b strings.Builder
	b.WriteString("Config\n")
	b.WriteString("  path: " + report.ConfigPath + "\n")
	if report.Profile != "" {
		b.WriteString("  profile: " + report.Profile + "\n")
	}
	b.WriteString("  mode: " + string(report.Mode) + "\n")
	if len(report.IncludeTrace) > 0 {
		b.WriteString("  include trace:\n")
		for _, item := range report.IncludeTrace {
			if item.IncludedBy == "" {
				b.WriteString("    - " + item.Path + "\n")
			} else {
				b.WriteString("    - " + item.Path + " (included by " + item.IncludedBy + ")\n")
			}
		}
	}
	b.WriteString("\nProviders\n")
	if len(report.Providers) == 0 {
		b.WriteString("  - none\n")
	}
	for _, provider := range report.Providers {
		parts := []string{"type=" + provider.Type}
		if provider.Protocol != "" {
			parts = append(parts, "protocol="+provider.Protocol)
		}
		if provider.CLIType != "" {
			parts = append(parts, "cliType="+provider.CLIType)
		}
		if provider.BaseURL != "" {
			parts = append(parts, "baseUrl="+provider.BaseURL)
		}
		if provider.APIKeyEnv != "" {
			parts = append(parts, "apiKeyEnv="+provider.APIKeyEnv)
		}
		if len(provider.Models) > 0 {
			parts = append(parts, "models="+strings.Join(provider.Models, ","))
		}
		b.WriteString("  - " + provider.ID + " " + strings.Join(parts, " ") + "\n")
	}
	b.WriteString("\nAgents\n")
	if len(report.Agents) == 0 {
		b.WriteString("  - none\n")
	}
	for _, agent := range report.Agents {
		parts := []string{"provider=" + agent.Provider}
		if agent.ModelID != "" {
			parts = append(parts, "model="+agent.ModelID)
		}
		if agent.ProviderModel != "" {
			parts = append(parts, "providerModel="+agent.ProviderModel)
		}
		if len(agent.AssignedRoles) > 0 {
			parts = append(parts, "assigned="+strings.Join(agent.AssignedRoles, ","))
		}
		b.WriteString("  - " + agent.ID + " " + strings.Join(parts, " ") + "\n")
	}
	b.WriteString("\nRoles\n")
	b.WriteString("  adjudication:\n")
	b.WriteString("    proposers: " + strings.Join(report.Roles.Adjudication.Proposers, ",") + "\n")
	b.WriteString("    challengers: " + strings.Join(report.Roles.Adjudication.Challengers, ",") + "\n")
	b.WriteString("    semantic_verifier: " + report.Roles.Adjudication.SemanticVerifier + "\n")
	b.WriteString("    arbiter: " + report.Roles.Adjudication.Arbiter + "\n")
	b.WriteString("    reporter: " + report.Roles.Adjudication.Reporter + "\n")
	b.WriteString("    actor: " + report.Roles.Adjudication.Actor + "\n")
	b.WriteString("  free_debate:\n")
	b.WriteString("    participants: " + strings.Join(report.Roles.FreeDebate.Participants, ",") + "\n")
	b.WriteString("    semantic_deduper: " + report.Roles.FreeDebate.SemanticDeduper + "\n")
	b.WriteString("    reporter: " + report.Roles.FreeDebate.Reporter + "\n")
	b.WriteString("    actor: " + report.Roles.FreeDebate.Actor + "\n")
	b.WriteString("  delphi:\n")
	b.WriteString("    participants: " + strings.Join(report.Roles.Delphi.Participants, ",") + "\n")
	b.WriteString("    facilitator: " + report.Roles.Delphi.Facilitator + "\n")
	b.WriteString("    reporter: " + report.Roles.Delphi.Reporter + "\n")
	b.WriteString("    actor: " + report.Roles.Delphi.Actor + "\n")
	b.WriteString("\nOutput\n")
	b.WriteString("  runDir: " + report.Output.RunDir + "\n")
	b.WriteString("  resultTemplate: " + report.Output.ResultTemplate + "\n")
	b.WriteString("  sessionStoreDir: " + report.Output.SessionStoreDir + "\n")
	if len(report.Warnings) > 0 {
		b.WriteString("\nWarnings\n")
		for _, warning := range report.Warnings {
			b.WriteString("  - " + warning + "\n")
		}
	}
	return b.String()
}
