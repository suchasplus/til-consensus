package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/internal/preflight"
	"github.com/suchasplus/til-consensus/internal/telemetry"
	"github.com/urfave/cli/v3"
)

type doctorStatus string

const (
	doctorOK   doctorStatus = "ok"
	doctorWarn doctorStatus = "warn"
	doctorFail doctorStatus = "fail"
)

type doctorReport struct {
	GeneratedAt string        `json:"generatedAt"`
	ConfigPath  string        `json:"configPath,omitempty"`
	Checks      []doctorCheck `json:"checks"`
	Summary     doctorSummary `json:"summary"`
}

type doctorCheck struct {
	Name    string       `json:"name"`
	Status  doctorStatus `json:"status"`
	Message string       `json:"message"`
	Hint    string       `json:"hint,omitempty"`
}

type doctorSummary struct {
	OK    int `json:"ok"`
	Warn  int `json:"warn"`
	Fail  int `json:"fail"`
	Total int `json:"total"`
}

func newDoctorCommand() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "检查配置、输出目录和 provider 可用性",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.BoolFlag{Name: "providers", Usage: "真实调用 provider 做 readiness preflight"},
			&cli.BoolFlag{Name: "all", Usage: "执行所有检查，包含 provider preflight"},
			&cli.BoolFlag{Name: "strict", Usage: "有 warning 时也返回非零 exit code"},
			&cli.StringFlag{Name: "format", Usage: "输出格式(text|json)", Value: "text"},
			&cli.DurationFlag{Name: "timeout", Usage: "provider preflight 单项超时", Value: 90 * time.Second},
			&cli.BoolFlag{Name: "verbose", Usage: "展示更完整的 provider readiness 信息"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			report := runDoctor(ctx, cmd)
			if err := writeDoctorReport(cmd.Writer, report, cmd.String("format"), cmd.Bool("verbose")); err != nil {
				return err
			}
			if report.Summary.Fail > 0 {
				return appError(doctorExitCode(report), fmt.Sprintf("doctor failed: %d check(s) failed", report.Summary.Fail), "查看上方 fail 项并修复后重试", nil)
			}
			if cmd.Bool("strict") && report.Summary.Warn > 0 {
				return appError(ExitProviderNotReady, fmt.Sprintf("doctor warnings: %d check(s) warned", report.Summary.Warn), "去掉 --strict 可允许 warning 返回 0", nil)
			}
			return nil
		},
	}
}

func doctorExitCode(report doctorReport) int {
	code := ExitInternalError
	for _, check := range report.Checks {
		if check.Status != doctorFail {
			continue
		}
		switch {
		case check.Name == "config.resolve":
			return ExitConfigNotFound
		case strings.HasPrefix(check.Name, "config."):
			code = ExitConfigInvalid
		case strings.HasPrefix(check.Name, "provider."):
			if code == ExitInternalError {
				code = ExitProviderNotReady
			}
		case strings.HasPrefix(check.Name, "output."):
			if code == ExitInternalError {
				code = ExitArtifactInvalid
			}
		}
	}
	return code
}

func runDoctor(ctx context.Context, cmd *cli.Command) doctorReport {
	report := doctorReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	add := func(status doctorStatus, name string, message string, hint string) {
		report.Checks = append(report.Checks, doctorCheck{Name: name, Status: status, Message: message, Hint: hint})
	}
	configPath, err := config.ResolveConfigPath(cmd.String("config"))
	if err != nil {
		add(doctorFail, "config.resolve", err.Error(), "传入 --config 或在当前目录放置 til-consensus.yaml")
		finalizeDoctorReport(&report)
		return report
	}
	report.ConfigPath = configPath
	add(doctorOK, "config.resolve", configPath, "")

	profileLoaded, profileErr := config.LoadProfilesWithProfile(configPath, cmd.String("profile"))
	if profileErr != nil {
		add(doctorFail, "config.profiles", profileErr.Error(), "先修复 providers / agents 基础配置")
		finalizeDoctorReport(&report)
		return report
	}
	add(doctorOK, "config.profiles", "provider / agent profiles are valid", "")

	loaded, fullErr := config.LoadWithProfile(configPath, cmd.String("profile"))
	if fullErr != nil {
		add(doctorFail, "config.workflow", fullErr.Error(), "如果只想检查 provider，使用 profile preflight；完整 run 仍需 roles 合法")
	} else {
		add(doctorOK, "config.workflow", "workflow config is valid", "")
	}
	if fullErr != nil {
		loaded = profileLoaded
	}
	checkOutputWritable(loaded, add)
	checkProviderLocalReadiness(loaded.Config, add)

	if cmd.Bool("providers") || cmd.Bool("all") {
		entries, err := preflight.Run(ctx, profileLoaded.Config, preflight.Options{
			All:     true,
			Timeout: cmd.Duration("timeout"),
		}, nil)
		if err != nil {
			add(doctorFail, "provider.preflight", err.Error(), "检查 provider id / agent id / model 配置")
		} else {
			addProviderPreflightChecks(entries, add)
		}
	}

	finalizeDoctorReport(&report)
	return report
}

func checkOutputWritable(loaded config.LoadedConfig, add func(doctorStatus, string, string, string)) {
	paths := config.ResolveRunArtifacts(loaded, "doctor")
	parent := filepath.Dir(paths.RunDir)
	probeDir, parentExists, err := nearestExistingDirectory(parent)
	if err != nil {
		add(doctorFail, "output.directory", err.Error(), "检查 output.directory 的父目录权限")
		return
	}
	tmp, err := os.CreateTemp(probeDir, ".til-consensus-doctor-*")
	if err != nil {
		add(doctorFail, "output.directory", err.Error(), "检查 output.directory 是否可写")
		return
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	if !parentExists {
		add(doctorWarn, "output.directory", "parent will be created by run; existing ancestor is writable: "+probeDir, "如需提前消除 warning，手动创建 "+parent)
		return
	}
	add(doctorOK, "output.directory", "writable: "+parent, "")
}

func nearestExistingDirectory(path string) (string, bool, error) {
	path = filepath.Clean(path)
	parentExists := true
	for {
		info, err := os.Stat(path)
		if err == nil {
			if !info.IsDir() {
				return "", false, fmt.Errorf("output parent is not a directory: %s", path)
			}
			return path, parentExists, nil
		}
		if !os.IsNotExist(err) {
			return "", false, err
		}
		parentExists = false
		next := filepath.Dir(path)
		if next == path {
			return "", false, fmt.Errorf("no existing output ancestor for %s", path)
		}
		path = next
	}
}

func checkProviderLocalReadiness(cfg config.Config, add func(doctorStatus, string, string, string)) {
	for id, provider := range cfg.Providers {
		switch provider.Type {
		case config.ProviderTypeAPI:
			if strings.TrimSpace(provider.APIKeyEnv) == "" {
				add(doctorWarn, "provider."+id+".api_key_env", "api_key_env is empty", "如果 provider 需要鉴权，请配置 api_key_env")
				continue
			}
			if strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) == "" {
				add(doctorWarn, "provider."+id+".api_key_env", "env "+provider.APIKeyEnv+" is not set", "export "+provider.APIKeyEnv+"=...")
				continue
			}
			add(doctorOK, "provider."+id+".api_key_env", "env "+provider.APIKeyEnv+" is set", "")
		case config.ProviderTypeCLI:
			command := provider.Command
			if command == "" {
				command = provider.CLIType
			}
			if strings.TrimSpace(command) == "" {
				add(doctorFail, "provider."+id+".command", "CLI command is empty", "配置 command 或 cli_type")
				continue
			}
			if _, err := exec.LookPath(command); err != nil {
				add(doctorWarn, "provider."+id+".command", "binary not found: "+command, "安装 CLI 或修正 PATH")
				continue
			}
			add(doctorOK, "provider."+id+".command", "binary found: "+command, "")
		case config.ProviderTypeMock:
			add(doctorOK, "provider."+id, "mock provider", "")
		default:
			add(doctorWarn, "provider."+id, "provider type "+provider.Type+" has no doctor probe", "")
		}
	}
}

func addProviderPreflightChecks(entries []telemetry.ProviderReadinessEntry, add func(doctorStatus, string, string, string)) {
	if len(entries) == 0 {
		add(doctorWarn, "provider.preflight", "no provider candidates", "检查 providers 配置")
		return
	}
	ready := 0
	for _, entry := range entries {
		name := "provider.preflight." + entry.Provider
		if entry.Agent != "" {
			name += "." + entry.Agent
		}
		if entry.Ready {
			ready++
			add(doctorOK, name, fmt.Sprintf("%s ready strict=%v recoverable=%v duration=%dms", entry.Model, entry.StrictJSON, entry.RecoverableJSON, entry.DurationMs), "")
			continue
		}
		add(doctorFail, name, entry.Error, "运行 profile preflight --verbose 查看 stdout/stderr preview")
	}
	if ready == 0 {
		add(doctorFail, "provider.preflight.summary", "no providers are ready", "先修复至少一个 provider")
	}
}

func finalizeDoctorReport(report *doctorReport) {
	for _, check := range report.Checks {
		switch check.Status {
		case doctorOK:
			report.Summary.OK++
		case doctorWarn:
			report.Summary.Warn++
		case doctorFail:
			report.Summary.Fail++
		}
	}
	report.Summary.Total = len(report.Checks)
}

func writeDoctorReport(writer interface{ Write([]byte) (int, error) }, report doctorReport, format string, verbose bool) error {
	switch strings.TrimSpace(format) {
	case "", "text":
		_, _ = fmt.Fprintf(writer, "[til-consensus] doctor ok=%d warn=%d fail=%d\n", report.Summary.OK, report.Summary.Warn, report.Summary.Fail)
		for _, check := range report.Checks {
			prefix := "[" + string(check.Status) + "]"
			_, _ = fmt.Fprintf(writer, "%-6s %s: %s\n", prefix, check.Name, check.Message)
			if verbose && check.Hint != "" {
				_, _ = fmt.Fprintf(writer, "       hint: %s\n", check.Hint)
			}
		}
	case "json":
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal doctor report: %w", err)
		}
		_, _ = fmt.Fprintln(writer, string(body))
	default:
		return appError(ExitUsageError, "unsupported doctor format: "+format, "使用 --format text 或 --format json", nil)
	}
	return nil
}
