package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/artifact"
	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	"github.com/suchasplus/til-consensus/internal/observer"
	"github.com/suchasplus/til-consensus/internal/runtime"
	filestore "github.com/suchasplus/til-consensus/internal/store/file"
	"github.com/urfave/cli/v3"
)

func newRunCommand() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "运行一次 til-consensus 裁决流程",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "input", Usage: "输入文件路径"},
			&cli.StringFlag{Name: "task-file", Usage: "任务文本文件路径，读取整个文件内容作为 task"},
			&cli.StringFlag{Name: "followup", Usage: "直接执行 follow-up case artifact"},
			&cli.StringFlag{Name: "resume-session", Usage: "从持久化 session store 重新执行某个 session 的 request"},
			&cli.StringFlag{Name: "replay-session", Usage: "基于某个 session 的 request 生成新的 child run"},
			&cli.StringFlag{Name: "mode", Usage: "工作流模式(adjudication|free-debate|delphi)"},
			&cli.StringFlag{Name: "task", Usage: "任务目标"},
			&cli.StringFlag{Name: "proposers", Usage: "逗号分隔的 proposer agent 列表"},
			&cli.StringFlag{Name: "challengers", Usage: "逗号分隔的 challenger agent 列表"},
			&cli.StringFlag{Name: "participants", Usage: "逗号分隔的 participant agent 列表"},
			&cli.StringFlag{Name: "arbiter", Usage: "arbiter agent"},
			&cli.StringFlag{Name: "semantic-verifier", Usage: "semantic verifier agent"},
			&cli.StringFlag{Name: "facilitator", Usage: "delphi facilitator agent"},
			&cli.StringFlag{Name: "reporter", Usage: "reporter agent"},
			&cli.StringFlag{Name: "actor", Usage: "actor agent"},
			&cli.StringSliceFlag{Name: "success-criteria", Usage: "重复传入成功标准"},
			&cli.StringFlag{Name: "workspace-snapshot", Usage: "workspace 根目录或 snapshot 路径"},
			&cli.IntFlag{Name: "min-rounds", Usage: "free_debate / delphi 的最小轮数"},
			&cli.IntFlag{Name: "max-rounds", Usage: "free_debate / delphi 的最大轮数"},
			&cli.Float64Flag{Name: "vote-threshold", Usage: "free_debate 的最终投票阈值"},
			&cli.Float64Flag{Name: "convergence-threshold", Usage: "delphi 的收敛阈值"},
			&cli.DurationFlag{Name: "timeout", Usage: "单任务超时"},
			&cli.DurationFlag{Name: "global-deadline", Usage: "全局截止时间"},
			&cli.StringFlag{Name: "action", Usage: "裁决后执行的 action"},
			&cli.BoolFlag{Name: "dry-run", Usage: "只解析并展示最终 run plan，不调用 provider，不写运行产物"},
			&cli.StringFlag{Name: "format", Usage: "dry-run 输出格式(text|json)", Value: "text"},
			&cli.BoolFlag{Name: "verbose", Usage: "输出详细事件"},
			&cli.BoolFlag{Name: "debug", Usage: "输出完整事件 payload 以及 provider 输入/输出 artifact 路径"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runCommand(ctx, cmd)
		},
	}
}

func runCommand(ctx context.Context, cmd *cli.Command) error {
	configPath, err := config.ResolveConfigPath(cmd.String("config"))
	if err != nil {
		return err
	}
	loaded, err := config.LoadWithProfile(configPath, cmd.String("profile"))
	if err != nil {
		return err
	}
	sessionStore := filestore.New(config.ResolveSessionStoreDir(loaded))
	if followupPath := strings.TrimSpace(cmd.String("followup")); followupPath != "" {
		if hasConflictingRunSource(cmd, "followup") {
			return fmt.Errorf("--followup 不能与 --input/--task/--task-file/--resume-session/--replay-session 同时使用")
		}
		artifact, err := consensus.LoadFollowUpCaseArtifact(followupPath)
		if err != nil {
			return err
		}
		plan, err := config.ResolveRunPlanForRequest(loaded, artifact.Request, cmd.Bool("verbose"), cmd.Bool("debug"))
		if err != nil {
			return err
		}
		if cmd.Bool("dry-run") {
			return writeDryRunPlan(cmd.Writer, loaded, plan, "followup", cmd.String("format"))
		}
		return executeResolvedPlan(ctx, loaded, plan, cmd.Writer)
	}
	if sessionID := strings.TrimSpace(cmd.String("resume-session")); sessionID != "" {
		if hasConflictingRunSource(cmd, "resume-session") {
			return fmt.Errorf("--resume-session 不能与 --input/--task/--task-file/--followup/--replay-session 同时使用")
		}
		snapshot, err := sessionStore.Load(ctx, sessionID)
		if err != nil {
			return err
		}
		if snapshot == nil || snapshot.Request == nil {
			return fmt.Errorf("session %s 没有可恢复的 request", sessionID)
		}
		plan, err := config.ResolveRunPlanForRequest(loaded, *snapshot.Request, cmd.Bool("verbose"), cmd.Bool("debug"))
		if err != nil {
			return err
		}
		if cmd.Bool("dry-run") {
			return writeDryRunPlan(cmd.Writer, loaded, plan, "resume-session", cmd.String("format"))
		}
		return executeResumedSession(ctx, loaded, plan, snapshot, cmd.Writer)
	}
	if sessionID := strings.TrimSpace(cmd.String("replay-session")); sessionID != "" {
		if hasConflictingRunSource(cmd, "replay-session") {
			return fmt.Errorf("--replay-session 不能与 --input/--task/--task-file/--followup/--resume-session 同时使用")
		}
		snapshot, err := sessionStore.Load(ctx, sessionID)
		if err != nil {
			return err
		}
		if snapshot == nil || snapshot.Request == nil {
			return fmt.Errorf("session %s 没有可重放的 request", sessionID)
		}
		replayRequest := *snapshot.Request
		replayRequest.RequestID = artifact.NewRequestID(time.Now().UTC())
		replayRequest.Lineage = &consensus.RunLineage{
			ParentRequestID: snapshot.RequestID,
			ParentSessionID: snapshot.SessionID,
			Trigger:         "session_replay",
		}
		if snapshot.Result != nil && snapshot.Result.CaseManifest != nil {
			replayRequest.Lineage.ParentCaseID = snapshot.Result.CaseManifest.CaseID
		}
		plan, err := config.ResolveRunPlanForRequest(loaded, replayRequest, cmd.Bool("verbose"), cmd.Bool("debug"))
		if err != nil {
			return err
		}
		if cmd.Bool("dry-run") {
			return writeDryRunPlan(cmd.Writer, loaded, plan, "replay-session", cmd.String("format"))
		}
		return executeResolvedPlan(ctx, loaded, plan, cmd.Writer)
	}
	input, err := config.LoadRunInput(cmd.String("input"))
	if err != nil {
		return err
	}
	taskOverride, err := resolveTaskOverride(cmd.String("task"), cmd.String("task-file"))
	if err != nil {
		return err
	}
	overrides := config.RunOverrides{
		ConfigPath:           cmd.String("config"),
		InputPath:            cmd.String("input"),
		Mode:                 parseMode(cmd.String("mode")),
		Task:                 taskOverride,
		Proposers:            splitComma(cmd.String("proposers")),
		Challengers:          splitComma(cmd.String("challengers")),
		Participants:         splitComma(cmd.String("participants")),
		Arbiter:              cmd.String("arbiter"),
		SemanticVerifier:     cmd.String("semantic-verifier"),
		Facilitator:          cmd.String("facilitator"),
		Reporter:             cmd.String("reporter"),
		Actor:                cmd.String("actor"),
		SuccessCriteria:      cmd.StringSlice("success-criteria"),
		WorkspaceSnapshot:    cmd.String("workspace-snapshot"),
		MinRounds:            cmd.Int("min-rounds"),
		MaxRounds:            cmd.Int("max-rounds"),
		VoteThreshold:        cmd.Float64("vote-threshold"),
		ConvergenceThreshold: cmd.Float64("convergence-threshold"),
		Timeout:              cmd.Duration("timeout"),
		GlobalDeadline:       cmd.Duration("global-deadline"),
		Action:               cmd.String("action"),
		Verbose:              cmd.Bool("verbose"),
		Debug:                cmd.Bool("debug"),
	}
	plan, err := config.ResolveRunPlan(loaded, input, overrides, time.Now().UTC())
	if err != nil {
		return err
	}
	if cmd.Bool("dry-run") {
		return writeDryRunPlan(cmd.Writer, loaded, plan, "run", cmd.String("format"))
	}
	return executeResolvedPlan(ctx, loaded, plan, cmd.Writer)
}

func hasConflictingRunSource(cmd *cli.Command, active string) bool {
	sources := map[string]bool{
		"input":          strings.TrimSpace(cmd.String("input")) != "",
		"task":           strings.TrimSpace(cmd.String("task")) != "",
		"task-file":      strings.TrimSpace(cmd.String("task-file")) != "",
		"followup":       strings.TrimSpace(cmd.String("followup")) != "",
		"resume-session": strings.TrimSpace(cmd.String("resume-session")) != "",
		"replay-session": strings.TrimSpace(cmd.String("replay-session")) != "",
	}
	sources[active] = false
	for _, present := range sources {
		if present {
			return true
		}
	}
	return false
}

func resolveTaskOverride(task string, taskFile string) (string, error) {
	task = strings.TrimSpace(task)
	taskFile = strings.TrimSpace(taskFile)
	if task != "" && taskFile != "" {
		return "", fmt.Errorf("--task 不能与 --task-file 同时使用")
	}
	if taskFile == "" {
		return task, nil
	}
	body, err := os.ReadFile(taskFile)
	if err != nil {
		return "", fmt.Errorf("read task file: %w", err)
	}
	if strings.TrimSpace(string(body)) == "" {
		return "", fmt.Errorf("task file is empty: %s", taskFile)
	}
	return string(body), nil
}

func executeResolvedPlan(ctx context.Context, loaded config.LoadedConfig, plan config.ResolvedRunPlan, writer interface{ Write([]byte) (int, error) }) error {
	output := NewOutput(writer, os.Stderr, plan.Verbose, plan.Debug, plan.ArtifactsDir)
	output.RunStarted(plan.RequestID, plan.Mode, plan.Task, plan.Roles)

	delegate, err := runtime.NewDelegate(loaded.Config, plan.ArtifactsDir)
	if err != nil {
		return err
	}
	engine := consensus.NewEngine(consensus.EngineDeps{
		TaskDelegate: delegate,
		Observer: observer.NewMulti(
			observer.NewJSONL(plan.EventsPath),
			output.EventObserver(),
		),
		Ledger:       observer.NewLedger(plan.LedgerPath, plan.ManifestPath),
		SessionStore: filestore.New(plan.SessionStoreDir),
		ArtifactDir:  plan.ArtifactsDir,
	})
	result, runErr := engine.Start(ctx, plan.StartRequest)
	if runErr != nil {
		_ = artifact.WriteErrorArtifact(plan.RequestID, plan.ErrorPath, runErr)
		return runErr
	}
	if err := artifact.WriteRunArtifacts(result, plan.ResultPath, plan.SummaryPath); err != nil {
		return err
	}
	if err := writeRunTelemetryArtifact(result, plan.ArtifactsDir); err != nil {
		return err
	}
	output.RunCompleted(plan.ResultPath, plan.SummaryPath)
	return nil
}

func executeResumedSession(ctx context.Context, loaded config.LoadedConfig, plan config.ResolvedRunPlan, snapshot *consensus.SessionSnapshot, writer interface{ Write([]byte) (int, error) }) error {
	output := NewOutput(writer, os.Stderr, plan.Verbose, plan.Debug, plan.ArtifactsDir)
	output.RunStarted(plan.RequestID, plan.Mode, plan.Task, plan.Roles)

	delegate, err := runtime.NewDelegate(loaded.Config, plan.ArtifactsDir)
	if err != nil {
		return err
	}
	engine := consensus.NewEngine(consensus.EngineDeps{
		TaskDelegate: delegate,
		Observer: observer.NewMulti(
			observer.NewJSONL(plan.EventsPath),
			output.EventObserver(),
		),
		Ledger:       observer.NewLedger(plan.LedgerPath, plan.ManifestPath),
		SessionStore: filestore.New(plan.SessionStoreDir),
		ArtifactDir:  plan.ArtifactsDir,
	})
	result, runErr := engine.Resume(ctx, *snapshot)
	if runErr != nil {
		_ = artifact.WriteErrorArtifact(plan.RequestID, plan.ErrorPath, runErr)
		return runErr
	}
	if err := artifact.WriteRunArtifacts(result, plan.ResultPath, plan.SummaryPath); err != nil {
		return err
	}
	if err := writeRunTelemetryArtifact(result, plan.ArtifactsDir); err != nil {
		return err
	}
	output.RunCompleted(plan.ResultPath, plan.SummaryPath)
	return nil
}

func parseMode(value string) consensus.WorkflowMode {
	normalized := strings.TrimSpace(strings.ToLower(value))
	switch normalized {
	case "":
		return ""
	case string(consensus.WorkflowModeAdjudication):
		return consensus.WorkflowModeAdjudication
	case "free-debate", string(consensus.WorkflowModeFreeDebate):
		return consensus.WorkflowModeFreeDebate
	case string(consensus.WorkflowModeDelphi):
		return consensus.WorkflowModeDelphi
	default:
		return consensus.WorkflowMode(strings.TrimSpace(value))
	}
}

func splitComma(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func marshalPretty(value any) (string, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(body) + "\n", nil
}
