package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
	"github.com/suchasplus/til-consensus/internal/viewer"
	"github.com/urfave/cli/v3"
)

func newViewCommand() *cli.Command {
	return &cli.Command{
		Name:  "view",
		Usage: "以终端友好的方式浏览某次裁决结果",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "request-id", Usage: "指定 request id"},
			&cli.StringFlag{Name: "result", Usage: "直接指定 result.json 路径"},
			&cli.StringFlag{Name: "format", Usage: "输出格式(text|markdown|json)", Value: viewer.FormatText},
			&cli.StringSliceFlag{Name: "section", Usage: "输出分段(overview|claims|challenges|verifications|observations|followups|artifacts|all)"},
			&cli.StringFlag{Name: "claim-verdict", Usage: "只显示特定 verdict 的 claims"},
			&cli.IntFlag{Name: "limit", Usage: "限制 claims/verifications/artifacts 的展示数量", Value: 20},
			&cli.BoolFlag{Name: "verbose", Usage: "展开 rationale、evidence refs 和 artifact 路径"},
			&cli.BoolFlag{Name: "web", Usage: "预留：启动浏览器 viewer（当前未实现）", Hidden: true},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runViewCommand(ctx, cmd)
		},
	}
}

func runViewCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.Bool("web") {
		return fmt.Errorf("view --web 预留给二期浏览器 viewer，当前未实现，见 docs/viewer.md")
	}
	if !isSupportedViewFormat(cmd.String("format")) {
		return fmt.Errorf("unsupported view format: %s", cmd.String("format"))
	}
	for _, section := range cmd.StringSlice("section") {
		if !isSupportedViewSection(section) {
			return fmt.Errorf("unsupported view section: %s", section)
		}
	}

	resultPath := cmd.String("result")
	var related viewer.RunFiles
	if resultPath == "" {
		configPath, err := config.ResolveConfigPath(cmd.String("config"))
		if err != nil {
			return err
		}
		loaded, err := config.Load(configPath)
		if err != nil {
			return err
		}
		template := config.ResolveResultTemplate(loaded)
		if requestID := cmd.String("request-id"); requestID != "" {
			resultPath = strings.ReplaceAll(template, "{requestId}", requestID)
			artifactPaths := config.ResolveRunArtifacts(loaded, requestID)
			related = viewer.RunFiles{
				RunDir:       artifactPaths.RunDir,
				ResultPath:   artifactPaths.ResultPath,
				LedgerPath:   artifactPaths.LedgerPath,
				SummaryPath:  artifactPaths.SummaryPath,
				ManifestPath: artifactPaths.ManifestPath,
			}
		} else {
			latest, err := viewer.ResolveLatestRun(template)
			if err != nil {
				return err
			}
			if latest == nil {
				return fmt.Errorf("no completed runs found")
			}
			resultPath = latest.ResultPath
			artifactPaths := config.ResolveRunArtifacts(loaded, latest.RequestID)
			related = viewer.RunFiles{
				RunDir:       artifactPaths.RunDir,
				ResultPath:   artifactPaths.ResultPath,
				LedgerPath:   artifactPaths.LedgerPath,
				SummaryPath:  artifactPaths.SummaryPath,
				ManifestPath: artifactPaths.ManifestPath,
			}
		}
	}

	if related.ResultPath == "" {
		related = viewer.InferRunFiles(resultPath)
	}
	bundle, err := viewer.LoadBundle(related)
	if err != nil {
		return err
	}
	claimVerdict := strings.TrimSpace(cmd.String("claim-verdict"))
	if claimVerdict != "" && !isSupportedClaimVerdict(claimVerdict) {
		return fmt.Errorf("unsupported claim verdict filter: %s", claimVerdict)
	}
	doc := viewer.BuildDocument(bundle, viewer.RenderOptions{
		Format:       cmd.String("format"),
		Sections:     cmd.StringSlice("section"),
		ClaimVerdict: consensus.ClaimVerdict(claimVerdict),
		Limit:        cmd.Int("limit"),
		Verbose:      cmd.Bool("verbose"),
	})
	rendered, err := viewer.RenderDocument(doc, viewer.RenderOptions{
		Format:  cmd.String("format"),
		Verbose: cmd.Bool("verbose"),
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(cmd.Writer, rendered)
	return nil
}

func isSupportedClaimVerdict(value string) bool {
	switch value {
	case "",
		string(consensus.ClaimVerdictSupported),
		string(consensus.ClaimVerdictRefuted),
		string(consensus.ClaimVerdictInsufficientEvidence),
		string(consensus.ClaimVerdictUndetermined):
		return true
	default:
		return false
	}
}

func isSupportedViewFormat(value string) bool {
	switch strings.TrimSpace(value) {
	case "", viewer.FormatText, viewer.FormatMarkdown, viewer.FormatJSON:
		return true
	default:
		return false
	}
}

func isSupportedViewSection(value string) bool {
	switch strings.TrimSpace(value) {
	case "",
		viewer.SectionAll,
		viewer.SectionOverview,
		viewer.SectionClaims,
		viewer.SectionChallenges,
		viewer.SectionVerifications,
		viewer.SectionObservations,
		viewer.SectionFollowups,
		viewer.SectionArtifacts,
		viewer.SectionRounds,
		viewer.SectionVotes,
		viewer.SectionStatements,
		viewer.SectionConvergence:
		return true
	default:
		return false
	}
}
