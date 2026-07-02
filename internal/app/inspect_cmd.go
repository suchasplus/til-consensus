package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/internal/viewer"
	"github.com/urfave/cli/v3"
)

func newLastCommand() *cli.Command {
	return &cli.Command{
		Name:  "last",
		Usage: "查看最近一次 run 结果",
		Flags: viewShortcutFlags(false),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runViewShortcutCommand(ctx, cmd, nil, false, false)
		},
	}
}

func newInspectCommand() *cli.Command {
	return &cli.Command{
		Name:      "inspect",
		Usage:     "查看指定 run；不传目标时等价于 last",
		ArgsUsage: "[request-id | result.json]",
		Flags:     viewShortcutFlags(true),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runViewShortcutCommand(ctx, cmd, []string{viewer.SectionOverview, viewer.SectionClaims, viewer.SectionArtifacts, viewer.SectionDebug}, false, false)
		},
	}
}

func newOpenCommand() *cli.Command {
	return &cli.Command{
		Name:      "open",
		Usage:     "直接打开某次 run 的 Web viewer",
		ArgsUsage: "[request-id | result.json]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "request-id", Usage: "指定 request id"},
			&cli.StringFlag{Name: "result", Usage: "直接指定 result.json 路径"},
			&cli.StringFlag{Name: "host", Usage: "Web viewer 监听地址", Value: "127.0.0.1"},
			&cli.IntFlag{Name: "port", Usage: "Web viewer 监听端口；0 表示自动分配", Value: 0},
			&cli.BoolFlag{Name: "verbose", Usage: "展开 rationale、evidence refs 和 artifact 路径"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runViewShortcutCommand(ctx, cmd, nil, true, true)
		},
	}
}

func newLogsCommand() *cli.Command {
	return &cli.Command{
		Name:      "logs",
		Usage:     "列出或查看某次 run 的 raw/debug artifact",
		ArgsUsage: "[request-id | result.json]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "request-id", Usage: "指定 request id"},
			&cli.StringFlag{Name: "result", Usage: "直接指定 result.json 路径"},
			&cli.StringFlag{Name: "type", Usage: "按 kind/path 过滤", Value: "raw"},
			&cli.IntFlag{Name: "id", Usage: "artifact list 输出的 index"},
			&cli.StringFlag{Name: "path", Usage: "artifact 路径，可为 artifacts/... 或文件名"},
			&cli.BoolFlag{Name: "latest", Usage: "展示过滤后的最新 artifact"},
			&cli.BoolFlag{Name: "raw", Usage: "不做 JSON pretty print"},
			&cli.StringFlag{Name: "format", Usage: "列表输出格式(text|json)", Value: "text"},
			&cli.IntFlag{Name: "limit", Usage: "列表数量或最多读取字节数；0 表示不限制", Value: 100},
			&cli.BoolFlag{Name: "allow-outside-run-dir", Usage: "允许读取 run 目录外的 artifact 路径"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runLogsCommand(cmd)
		},
	}
}

func viewShortcutFlags(includeTarget bool) []cli.Flag {
	flags := []cli.Flag{
		&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
		&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
		&cli.StringFlag{Name: "format", Usage: "输出格式(text|markdown|json)", Value: viewer.FormatText},
		&cli.StringSliceFlag{Name: "section", Usage: "输出分段(overview|claims|challenges|verifications|observations|followups|debug|artifacts|all)"},
		&cli.StringFlag{Name: "claim-verdict", Usage: "只显示特定 verdict 的 claims"},
		&cli.IntFlag{Name: "limit", Usage: "限制 claims/verifications/artifacts 的展示数量", Value: 20},
		&cli.BoolFlag{Name: "verbose", Usage: "展开 rationale、evidence refs 和 artifact 路径"},
		&cli.BoolFlag{Name: "web", Usage: "启动本地只读 Web viewer"},
		&cli.StringFlag{Name: "host", Usage: "Web viewer 监听地址", Value: "127.0.0.1"},
		&cli.IntFlag{Name: "port", Usage: "Web viewer 监听端口；0 表示自动分配", Value: 0},
		&cli.BoolFlag{Name: "open", Usage: "显式打开默认浏览器"},
	}
	if includeTarget {
		flags = append([]cli.Flag{
			&cli.StringFlag{Name: "request-id", Usage: "指定 request id"},
			&cli.StringFlag{Name: "result", Usage: "直接指定 result.json 路径"},
		}, flags...)
	}
	return flags
}

func runViewShortcutCommand(ctx context.Context, cmd *cli.Command, defaultSections []string, forceWeb bool, forceOpen bool) error {
	format := cmd.String("format")
	if forceWeb {
		format = viewer.FormatText
	}
	if !isSupportedViewFormat(format) {
		return fmt.Errorf("unsupported view format: %s", format)
	}
	sections := cmd.StringSlice("section")
	if len(sections) == 0 && len(defaultSections) > 0 && !cmd.IsSet("section") {
		sections = defaultSections
	}
	for _, section := range sections {
		if !isSupportedViewSection(section) {
			return fmt.Errorf("unsupported view section: %s", section)
		}
	}
	requestID, resultPath := resolveInspectTarget(cmd.Args().Slice(), cmd.String("request-id"), cmd.String("result"))
	files, err := resolveViewRunFiles(cmd.String("config"), cmd.String("profile"), requestID, resultPath)
	if err != nil {
		return err
	}
	bundle, err := viewer.LoadBundle(files)
	if err != nil {
		return err
	}
	claimVerdict := strings.TrimSpace(cmd.String("claim-verdict"))
	if claimVerdict != "" && !isSupportedClaimVerdict(claimVerdict) {
		return fmt.Errorf("unsupported claim verdict filter: %s", claimVerdict)
	}
	renderOptions := viewer.RenderOptions{
		Format:       format,
		Sections:     sections,
		ClaimVerdict: consensus.ClaimVerdict(claimVerdict),
		Limit:        cmd.Int("limit"),
		Verbose:      cmd.Bool("verbose"),
	}
	if cmd.Bool("web") || forceWeb {
		server, err := viewer.NewWebServer(bundle, viewer.WebOptions{
			Host:          firstNonEmptyApp(cmd.String("host"), "127.0.0.1"),
			Port:          cmd.Int("port"),
			RenderOptions: renderOptions,
		})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.Writer, "web viewer started: %s\n", server.URL())
		_, _ = fmt.Fprintf(cmd.Writer, "requestId: %s | mode: %s\n", bundle.Result.RequestID, bundle.Result.Mode)
		_, _ = fmt.Fprintln(cmd.Writer, "按 Ctrl+C 退出")
		if cmd.Bool("open") || forceOpen {
			if err := viewer.OpenBrowser(server.URL()); err != nil {
				_ = server.Close()
				return err
			}
		}
		return server.Serve(ctx)
	}
	doc := viewer.BuildDocument(bundle, renderOptions)
	rendered, err := viewer.RenderDocument(doc, renderOptions)
	if err != nil {
		return err
	}
	if format == viewer.FormatText && shouldEnableColor(cmd.Writer) {
		rendered = colorizeViewText(rendered)
	}
	_, _ = fmt.Fprint(cmd.Writer, rendered)
	return nil
}

func runLogsCommand(cmd *cli.Command) error {
	requestID, resultPath := resolveInspectTarget(cmd.Args().Slice(), cmd.String("request-id"), cmd.String("result"))
	files, err := resolveRunFilesForCommand(cmd.String("config"), cmd.String("profile"), requestID, resultPath)
	if err != nil {
		return err
	}
	if cmd.Int("id") > 0 || cmd.Bool("latest") || strings.TrimSpace(cmd.String("path")) != "" {
		path, err := resolveArtifactSelection(files, cmd)
		if err != nil {
			return err
		}
		if !cmd.Bool("allow-outside-run-dir") && !pathInside(files.RunDir, path) {
			return appError(ExitArtifactInvalid, "artifact path is outside run dir: "+path, "如果确认需要读取外部路径，显式传入 --allow-outside-run-dir", nil)
		}
		return showArtifact(cmd.Writer, path, cmd.Int("limit"), cmd.Bool("raw"))
	}
	items, err := loadArtifactItems(files, cmd.String("type"))
	if err != nil {
		return err
	}
	if limit := cmd.Int("limit"); limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	if strings.TrimSpace(cmd.String("format")) == "json" {
		body, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.Writer, string(body))
		return nil
	}
	return writeArtifactList(cmd.Writer, items, cmd.String("format"))
}

func resolveInspectTarget(args []string, requestID string, resultPath string) (string, string) {
	if strings.TrimSpace(requestID) != "" || strings.TrimSpace(resultPath) != "" || len(args) == 0 {
		return requestID, resultPath
	}
	target := strings.TrimSpace(args[0])
	if strings.HasSuffix(target, ".json") || strings.Contains(target, "/") {
		return requestID, target
	}
	return target, resultPath
}
