package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

type setupFile struct {
	Path string
	Body []byte
}

type setupRootConfig struct {
	SchemaVersion int                 `yaml:"schema_version"`
	Include       []string            `yaml:"include"`
	Profile       string              `yaml:"profile"`
	Output        config.OutputConfig `yaml:"output"`
}

type setupProvidersConfig struct {
	SchemaVersion int                              `yaml:"schema_version"`
	Providers     map[string]config.ProviderConfig `yaml:"providers"`
	Agents        []config.AgentConfig             `yaml:"agents"`
}

type setupProfilesConfig struct {
	SchemaVersion int                             `yaml:"schema_version"`
	Profiles      map[string]config.ProfileConfig `yaml:"profiles"`
}

func newSetupCommand() *cli.Command {
	return &cli.Command{
		Name:  "setup",
		Usage: "生成 split config 起步骨架",
		Flags: setupFlags(),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runSetupCommand(cmd)
		},
	}
}

func newConfigWizardCommand() *cli.Command {
	return &cli.Command{
		Name:  "wizard",
		Usage: "生成 split config 起步骨架；等价于顶层 setup",
		Flags: setupFlags(),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runSetupCommand(cmd)
		},
	}
}

func setupFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "dir", Usage: "目标目录", Value: "."},
		&cli.StringFlag{Name: "config", Usage: "主配置文件名", Value: "til-consensus.yaml"},
		&cli.StringFlag{Name: "profile", Usage: "默认 profile 名称", Value: "default"},
		&cli.StringFlag{Name: "preset", Usage: "模板别名(quickstart|openai|coding|debate|delphi|generic|codex|claude|gemini)"},
		&cli.StringFlag{Name: "mode", Usage: "workflow 模式(adjudication|free-debate|delphi)"},
		&cli.StringFlag{Name: "provider-profile", Usage: "provider profile(mock|openai|generic|codex|claude|gemini)"},
		&cli.StringFlag{Name: "task-profile", Usage: "task profile(general|coding)", Value: config.TemplateTaskProfileGeneral},
		&cli.StringFlag{Name: "output", Usage: "默认输出目录模板", Value: "./out/{requestId}"},
		&cli.BoolFlag{Name: "stdout", Usage: "只打印将写入的文件，不落盘"},
		&cli.BoolFlag{Name: "force", Usage: "允许覆盖已存在文件"},
	}
}

func runSetupCommand(cmd *cli.Command) error {
	files, selection, err := buildSetupFiles(
		cmd.String("dir"),
		cmd.String("config"),
		cmd.String("profile"),
		cmd.String("preset"),
		cmd.String("mode"),
		cmd.String("provider-profile"),
		cmd.String("task-profile"),
		cmd.String("output"),
	)
	if err != nil {
		return err
	}
	if cmd.Bool("stdout") {
		for _, file := range files {
			_, _ = fmt.Fprintf(cmd.Writer, "# file: %s\n%s\n", file.Path, string(file.Body))
		}
		return nil
	}
	for _, file := range files {
		if !cmd.Bool("force") {
			if _, statErr := os.Stat(file.Path); statErr == nil {
				return appError(ExitUsageError, "config file already exists: "+file.Path, "传入 --force 覆盖，或选择新的 --dir/--config", nil)
			}
		}
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return fmt.Errorf("create setup dir: %w", err)
		}
		if err := os.WriteFile(file.Path, file.Body, 0o644); err != nil {
			return fmt.Errorf("write setup file %s: %w", file.Path, err)
		}
	}
	_, _ = fmt.Fprintf(cmd.Writer, "config skeleton written: %s (mode=%s provider_profile=%s task_profile=%s profile=%s)\n", files[0].Path, selection.Mode, selection.ProviderProfile, selection.TaskProfile, cmd.String("profile"))
	_, _ = fmt.Fprintln(cmd.Writer, "next:")
	_, _ = fmt.Fprintf(cmd.Writer, "  til-consensus doctor --config %s\n", files[0].Path)
	_, _ = fmt.Fprintf(cmd.Writer, "  til-consensus ask \"你的问题\" --config %s\n", files[0].Path)
	return nil
}

func buildSetupFiles(dir string, configName string, profile string, preset string, mode string, providerProfile string, taskProfile string, output string) ([]setupFile, config.TemplateSelection, error) {
	selection, err := config.ResolveTemplateSelection(preset, mode, providerProfile, taskProfile)
	if err != nil {
		return nil, config.TemplateSelection{}, err
	}
	cfg, err := config.BuildTemplateConfig(selection)
	if err != nil {
		return nil, config.TemplateSelection{}, err
	}
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = "default"
	}
	rootDir, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return nil, config.TemplateSelection{}, fmt.Errorf("resolve setup dir: %w", err)
	}
	if strings.TrimSpace(configName) == "" {
		configName = "til-consensus.yaml"
	}
	if strings.TrimSpace(output) != "" {
		cfg.Output.Directory = output
	}
	rootPath := filepath.Join(rootDir, configName)
	providersPath := filepath.Join(rootDir, "conf", "providers.yaml")
	profilesPath := filepath.Join(rootDir, "conf", "profiles.yaml")
	root := setupRootConfig{
		SchemaVersion: 1,
		Include: []string{
			filepath.ToSlash(filepath.Join("conf", "providers.yaml")),
			filepath.ToSlash(filepath.Join("conf", "profiles.yaml")),
		},
		Profile: profile,
		Output:  cfg.Output,
	}
	providers := setupProvidersConfig{
		SchemaVersion: 1,
		Providers:     cfg.Providers,
		Agents:        cfg.Agents,
	}
	profiles := setupProfilesConfig{
		SchemaVersion: 1,
		Profiles: map[string]config.ProfileConfig{
			profile: {
				Defaults: cfg.Defaults,
				Roles:    cfg.Roles,
			},
		},
	}
	files := []setupFile{
		{Path: rootPath},
		{Path: providersPath},
		{Path: profilesPath},
	}
	payloads := []any{root, providers, profiles}
	for idx := range files {
		body, err := yaml.Marshal(payloads[idx])
		if err != nil {
			return nil, config.TemplateSelection{}, fmt.Errorf("marshal setup file: %w", err)
		}
		files[idx].Body = body
	}
	return files, selection, nil
}
