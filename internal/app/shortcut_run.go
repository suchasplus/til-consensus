package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	"github.com/urfave/cli/v3"
)

func newAskCommand() *cli.Command {
	return newShortcutRunCommand("ask", "快速运行 adjudication：til-consensus ask \"任务\" 或 til-consensus ask ./task.md", consensus.WorkflowModeAdjudication)
}

func newDebateCommand() *cli.Command {
	return newShortcutRunCommand("debate", "快速运行 free_debate：til-consensus debate \"议题\" 或 til-consensus debate ./case.md", consensus.WorkflowModeFreeDebate)
}

func newDelphiCommand() *cli.Command {
	return newShortcutRunCommand("delphi", "快速运行 delphi：til-consensus delphi \"议题\" 或 til-consensus delphi ./case.md", consensus.WorkflowModeDelphi)
}

func newShortcutRunCommand(name string, usage string, mode consensus.WorkflowMode) *cli.Command {
	return &cli.Command{
		Name:      name,
		Usage:     usage,
		ArgsUsage: "[task text | task-file]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "input", Usage: "输入文件路径；位置参数或 --task-file 可覆盖其中的 task"},
			&cli.StringFlag{Name: "task-file", Usage: "任务文本文件路径，读取整个文件内容作为 task"},
			&cli.StringFlag{Name: "proposers", Usage: "逗号分隔的 proposer agent 列表"},
			&cli.StringFlag{Name: "challengers", Usage: "逗号分隔的 challenger agent 列表"},
			&cli.StringFlag{Name: "participants", Usage: "逗号分隔的 participant agent 列表"},
			&cli.StringFlag{Name: "arbiter", Usage: "arbiter agent"},
			&cli.StringFlag{Name: "semantic-verifier", Usage: "semantic verifier agent"},
			&cli.StringFlag{Name: "semantic-deduper", Usage: "free_debate semantic dedup agent"},
			&cli.StringFlag{Name: "facilitator", Usage: "delphi facilitator agent"},
			&cli.StringFlag{Name: "reporter", Usage: "reporter agent"},
			&cli.StringSliceFlag{Name: "success-criteria", Usage: "重复传入成功标准"},
			&cli.IntFlag{Name: "min-rounds", Usage: "free_debate / delphi 的最小轮数"},
			&cli.IntFlag{Name: "max-rounds", Usage: "free_debate / delphi 的最大轮数"},
			&cli.Float64Flag{Name: "vote-threshold", Usage: "free_debate 的最终投票阈值"},
			&cli.Float64Flag{Name: "convergence-threshold", Usage: "delphi 的收敛阈值"},
			&cli.DurationFlag{Name: "timeout", Usage: "单任务超时"},
			&cli.DurationFlag{Name: "global-deadline", Usage: "全局截止时间"},
			&cli.BoolFlag{Name: "dry-run", Usage: "只解析并展示最终 run plan，不调用 provider，不写运行产物"},
			&cli.StringFlag{Name: "format", Usage: "dry-run 输出格式(text|json)", Value: "text"},
			&cli.BoolFlag{Name: "verbose", Usage: "输出详细事件"},
			&cli.BoolFlag{Name: "debug", Usage: "输出完整事件 payload 以及 provider 输入/输出 artifact 路径"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runShortcutCommand(ctx, cmd, mode)
		},
	}
}

func runShortcutCommand(ctx context.Context, cmd *cli.Command, mode consensus.WorkflowMode) error {
	configPath, err := config.ResolveConfigPath(cmd.String("config"))
	if err != nil {
		return err
	}
	loaded, err := config.LoadWithProfile(configPath, cmd.String("profile"))
	if err != nil {
		return err
	}
	input, err := config.LoadRunInput(cmd.String("input"))
	if err != nil {
		return err
	}
	task, err := resolveShortcutTask(cmd.Args().Slice(), cmd.String("task-file"))
	if err != nil {
		return err
	}
	participants := splitComma(cmd.String("participants"))
	if len(participants) == 0 && (mode == consensus.WorkflowModeFreeDebate || mode == consensus.WorkflowModeDelphi) {
		participants = inferShortcutParticipants(loaded.Config.Roles, mode)
	}
	overrides := config.RunOverrides{
		ConfigPath:           cmd.String("config"),
		InputPath:            cmd.String("input"),
		Mode:                 mode,
		Task:                 task,
		Proposers:            splitComma(cmd.String("proposers")),
		Challengers:          splitComma(cmd.String("challengers")),
		Participants:         participants,
		Arbiter:              cmd.String("arbiter"),
		SemanticVerifier:     cmd.String("semantic-verifier"),
		SemanticDeduper:      cmd.String("semantic-deduper"),
		Facilitator:          cmd.String("facilitator"),
		Reporter:             cmd.String("reporter"),
		SuccessCriteria:      cmd.StringSlice("success-criteria"),
		Timeout:              cmd.Duration("timeout"),
		GlobalDeadline:       cmd.Duration("global-deadline"),
		MinRounds:            cmd.Int("min-rounds"),
		MaxRounds:            cmd.Int("max-rounds"),
		VoteThreshold:        cmd.Float64("vote-threshold"),
		ConvergenceThreshold: cmd.Float64("convergence-threshold"),
		Verbose:              cmd.Bool("verbose"),
		Debug:                cmd.Bool("debug"),
	}
	plan, err := config.ResolveRunPlan(loaded, input, overrides, time.Now().UTC())
	if err != nil {
		return err
	}
	if cmd.Bool("dry-run") {
		return writeDryRunPlan(cmd.Writer, loaded, plan, string(mode), cmd.String("format"))
	}
	return executeResolvedPlan(ctx, loaded, plan, cmd.Writer)
}

func resolveShortcutTask(args []string, taskFile string) (string, error) {
	taskFile = strings.TrimSpace(taskFile)
	if taskFile != "" && len(args) > 0 {
		return "", fmt.Errorf("--task-file 不能与位置参数同时使用")
	}
	if taskFile != "" {
		return resolveTaskOverride("", taskFile)
	}
	if len(args) == 0 {
		return "", nil
	}
	if len(args) == 1 {
		if info, err := os.Stat(args[0]); err == nil && !info.IsDir() {
			return resolveTaskOverride("", args[0])
		}
	}
	return strings.Join(args, " "), nil
}

func inferShortcutParticipants(roles config.RolesConfig, mode consensus.WorkflowMode) []string {
	modeRoles := config.RoleAssignmentsForMode(roles, mode)
	if len(modeRoles.Participants) > 0 {
		return modeRoles.Participants
	}
	adjudicationRoles := config.RoleAssignmentsForMode(roles, consensus.WorkflowModeAdjudication)
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
	for _, id := range adjudicationRoles.Proposers {
		add(id)
	}
	for _, id := range adjudicationRoles.Challengers {
		add(id)
	}
	return out
}
