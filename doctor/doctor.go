package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/preflight"
	"github.com/suchasplus/til-consensus/telemetry"
)

type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Options struct {
	ConfigPath string
	Profile    string
	Providers  bool
	All        bool
	Timeout    time.Duration
}

type Report struct {
	GeneratedAt string  `json:"generatedAt"`
	ConfigPath  string  `json:"configPath,omitempty"`
	Checks      []Check `json:"checks"`
	Summary     Summary `json:"summary"`
}

type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type Summary struct {
	OK    int `json:"ok"`
	Warn  int `json:"warn"`
	Fail  int `json:"fail"`
	Total int `json:"total"`
}

func Run(ctx context.Context, opts Options) Report {
	report := Report{GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	add := func(status Status, name string, message string, hint string) {
		report.Checks = append(report.Checks, Check{Name: name, Status: status, Message: message, Hint: hint})
	}
	configPath, err := config.ResolveConfigPath(opts.ConfigPath)
	if err != nil {
		add(StatusFail, "config.resolve", err.Error(), "传入 --config 或在当前目录放置 til-consensus.yaml")
		finalizeReport(&report)
		return report
	}
	report.ConfigPath = configPath
	add(StatusOK, "config.resolve", configPath, "")

	profileLoaded, profileErr := config.LoadProfilesWithProfile(configPath, opts.Profile)
	if profileErr != nil {
		add(StatusFail, "config.profiles", profileErr.Error(), "先修复 providers / agents 基础配置")
		finalizeReport(&report)
		return report
	}
	add(StatusOK, "config.profiles", "provider / agent profiles are valid", "")

	loaded, fullErr := config.LoadWithProfile(configPath, opts.Profile)
	if fullErr != nil {
		add(StatusFail, "config.workflow", fullErr.Error(), "如果只想检查 provider，使用 profile preflight；完整 run 仍需 roles 合法")
	} else {
		add(StatusOK, "config.workflow", "workflow config is valid", "")
	}
	if fullErr != nil {
		loaded = profileLoaded
	}
	checkOutputWritable(loaded, add)
	checkProviderLocalReadiness(loaded.Config, add)

	if opts.Providers || opts.All {
		entries, err := preflight.Run(ctx, profileLoaded.Config, preflight.Options{
			All:     true,
			Timeout: opts.Timeout,
		}, nil)
		if err != nil {
			add(StatusFail, "provider.preflight", err.Error(), "检查 provider id / agent id / model 配置")
		} else {
			addProviderPreflightChecks(entries, add)
		}
	}

	finalizeReport(&report)
	return report
}

func checkOutputWritable(loaded config.LoadedConfig, add func(Status, string, string, string)) {
	paths := config.ResolveRunArtifacts(loaded, "doctor")
	parent := filepath.Dir(paths.RunDir)
	probeDir, parentExists, err := nearestExistingDirectory(parent)
	if err != nil {
		add(StatusFail, "output.directory", err.Error(), "检查 output.directory 的父目录权限")
		return
	}
	tmp, err := os.CreateTemp(probeDir, ".til-consensus-doctor-*")
	if err != nil {
		add(StatusFail, "output.directory", err.Error(), "检查 output.directory 是否可写")
		return
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	if !parentExists {
		add(StatusWarn, "output.directory", "parent will be created by run; existing ancestor is writable: "+probeDir, "如需提前消除 warning，手动创建 "+parent)
		return
	}
	add(StatusOK, "output.directory", "writable: "+parent, "")
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

func checkProviderLocalReadiness(cfg config.Config, add func(Status, string, string, string)) {
	for id, provider := range cfg.Providers {
		switch provider.Type {
		case config.ProviderTypeAPI:
			if strings.TrimSpace(provider.APIKeyEnv) == "" {
				add(StatusWarn, "provider."+id+".api_key_env", "api_key_env is empty", "如果 provider 需要鉴权，请配置 api_key_env")
				continue
			}
			if strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) == "" {
				add(StatusWarn, "provider."+id+".api_key_env", "env "+provider.APIKeyEnv+" is not set", "export "+provider.APIKeyEnv+"=...")
				continue
			}
			add(StatusOK, "provider."+id+".api_key_env", "env "+provider.APIKeyEnv+" is set", "")
		case config.ProviderTypeCLI:
			command := provider.Command
			if command == "" {
				command = provider.CLIType
			}
			if strings.TrimSpace(command) == "" {
				add(StatusFail, "provider."+id+".command", "CLI command is empty", "配置 command 或 cli_type")
				continue
			}
			if _, err := exec.LookPath(command); err != nil {
				add(StatusWarn, "provider."+id+".command", "binary not found: "+command, "安装 CLI 或修正 PATH")
				continue
			}
			add(StatusOK, "provider."+id+".command", "binary found: "+command, "")
		case config.ProviderTypeMock:
			add(StatusOK, "provider."+id, "mock provider", "")
		default:
			add(StatusWarn, "provider."+id, "provider type "+provider.Type+" has no doctor probe", "")
		}
	}
}

func addProviderPreflightChecks(entries []telemetry.ProviderReadinessEntry, add func(Status, string, string, string)) {
	if len(entries) == 0 {
		add(StatusWarn, "provider.preflight", "no provider candidates", "检查 providers 配置")
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
			add(StatusOK, name, fmt.Sprintf("%s ready strict=%v recoverable=%v duration=%dms", entry.Model, entry.StrictJSON, entry.RecoverableJSON, entry.DurationMs), "")
			continue
		}
		add(StatusFail, name, entry.Error, "运行 profile preflight --verbose 查看 stdout/stderr preview")
	}
	if ready == 0 {
		add(StatusFail, "provider.preflight.summary", "no providers are ready", "先修复至少一个 provider")
	}
}

func finalizeReport(report *Report) {
	for _, check := range report.Checks {
		switch check.Status {
		case StatusOK:
			report.Summary.OK++
		case StatusWarn:
			report.Summary.Warn++
		case StatusFail:
			report.Summary.Fail++
		}
	}
	report.Summary.Total = len(report.Checks)
}
