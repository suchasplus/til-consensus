package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/viewer"
	"github.com/urfave/cli/v3"
)

type artifactListItem struct {
	Index        int       `json:"index"`
	Kind         string    `json:"kind"`
	EntryID      string    `json:"entryId,omitempty"`
	ClaimID      string    `json:"claimId,omitempty"`
	ChallengeID  string    `json:"challengeId,omitempty"`
	ProducerRole string    `json:"producerRole,omitempty"`
	Path         string    `json:"path"`
	AbsPath      string    `json:"absPath"`
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"modTime,omitempty"`
	MediaType    string    `json:"mediaType,omitempty"`
	Source       string    `json:"source"`
}

func newArtifactCommand() *cli.Command {
	return &cli.Command{
		Name:  "artifact",
		Usage: "列出或查看 run artifact",
		Commands: []*cli.Command{
			newArtifactListCommand(),
			newArtifactShowCommand(),
		},
	}
}

func newArtifactListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "列出某次 run 的 artifact",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "request-id", Usage: "指定 request id"},
			&cli.StringFlag{Name: "result", Usage: "直接指定 result.json 路径"},
			&cli.StringFlag{Name: "type", Usage: "按 kind/path 过滤，例如 raw、error、telemetry"},
			&cli.StringFlag{Name: "format", Usage: "输出格式(text|json)", Value: "text"},
			&cli.IntFlag{Name: "limit", Usage: "限制展示数量；0 表示不限制", Value: 100},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			files, err := resolveRunFilesForCommand(cmd.String("config"), cmd.String("profile"), cmd.String("request-id"), cmd.String("result"))
			if err != nil {
				return err
			}
			items, err := loadArtifactItems(files, cmd.String("type"))
			if err != nil {
				return err
			}
			if limit := cmd.Int("limit"); limit > 0 && len(items) > limit {
				items = items[:limit]
			}
			return writeArtifactList(cmd.Writer, items, cmd.String("format"))
		},
	}
}

func newArtifactShowCommand() *cli.Command {
	return &cli.Command{
		Name:  "show",
		Usage: "查看某个 artifact 内容",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Usage: "配置文件路径"},
			&cli.StringFlag{Name: "profile", Usage: "选择 config.profiles 中的配置 overlay"},
			&cli.StringFlag{Name: "request-id", Usage: "指定 request id"},
			&cli.StringFlag{Name: "result", Usage: "直接指定 result.json 路径"},
			&cli.IntFlag{Name: "id", Usage: "artifact list 输出的 index"},
			&cli.StringFlag{Name: "path", Usage: "artifact 路径，可为 artifacts/... 或文件名"},
			&cli.StringFlag{Name: "type", Usage: "配合 --latest，按 kind/path 过滤"},
			&cli.BoolFlag{Name: "latest", Usage: "展示过滤后的最新 artifact"},
			&cli.BoolFlag{Name: "raw", Usage: "不做 JSON pretty print"},
			&cli.IntFlag{Name: "limit", Usage: "最多读取字节数；0 表示不限制", Value: 20000},
			&cli.BoolFlag{Name: "allow-outside-run-dir", Usage: "允许读取 run 目录外的 artifact 路径"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			files, err := resolveRunFilesForCommand(cmd.String("config"), cmd.String("profile"), cmd.String("request-id"), cmd.String("result"))
			if err != nil {
				return err
			}
			path, err := resolveArtifactSelection(files, cmd)
			if err != nil {
				return err
			}
			if !cmd.Bool("allow-outside-run-dir") && !pathInside(files.RunDir, path) {
				return appError(ExitArtifactInvalid, "artifact path is outside run dir: "+path, "如果确认需要读取外部路径，显式传入 --allow-outside-run-dir", nil)
			}
			return showArtifact(cmd.Writer, path, cmd.Int("limit"), cmd.Bool("raw"))
		},
	}
}

func resolveRunFilesForCommand(configPath string, profile string, requestID string, resultPath string) (viewer.RunFiles, error) {
	if strings.TrimSpace(resultPath) != "" {
		return viewer.InferRunFiles(resultPath), nil
	}
	resolvedConfig, err := config.ResolveConfigPath(configPath)
	if err != nil {
		return viewer.RunFiles{}, err
	}
	loaded, err := config.LoadWithProfile(resolvedConfig, profile)
	if err != nil {
		return viewer.RunFiles{}, err
	}
	template := config.ResolveResultTemplate(loaded)
	if strings.TrimSpace(requestID) == "" {
		latest, err := viewer.ResolveLatestRun(template)
		if err != nil {
			return viewer.RunFiles{}, err
		}
		if latest == nil {
			return viewer.RunFiles{}, appError(ExitArtifactNotFound, "no completed runs found", "传入 --request-id 或 --result 指向具体运行", nil)
		}
		requestID = latest.RequestID
	}
	paths := config.ResolveRunArtifacts(loaded, requestID)
	return viewer.RunFiles{
		RunDir:                paths.RunDir,
		ArtifactsDir:          paths.ArtifactsDir,
		ResultPath:            paths.ResultPath,
		LedgerPath:            paths.LedgerPath,
		SummaryPath:           paths.SummaryPath,
		ManifestPath:          paths.ManifestPath,
		EventsPath:            paths.EventsPath,
		ComplianceSummaryPath: filepath.Join(paths.ArtifactsDir, "strict-compliance-summary.json"),
		ProviderReadinessPath: filepath.Join(paths.ArtifactsDir, "provider-readiness.json"),
		RunTelemetryPath:      filepath.Join(paths.ArtifactsDir, "run-telemetry.json"),
	}, nil
}

func loadArtifactItems(files viewer.RunFiles, filter string) ([]artifactListItem, error) {
	bundle, err := viewer.LoadBundle(files)
	if err != nil {
		return nil, err
	}
	items := make([]artifactListItem, 0, len(bundle.Manifest))
	seen := map[string]struct{}{}
	for _, entry := range bundle.Manifest {
		path := entry.Artifact.Path
		if strings.TrimSpace(path) == "" {
			continue
		}
		abs := absoluteArtifactPath(files.RunDir, path)
		item := artifactListItem{
			Kind:         string(entry.Kind),
			EntryID:      entry.EntryID,
			ClaimID:      entry.ClaimID,
			ChallengeID:  entry.ChallengeID,
			ProducerRole: entry.ProducerRole,
			Path:         displayArtifactPath(files.RunDir, abs),
			AbsPath:      abs,
			MediaType:    entry.Artifact.MediaType,
			Source:       "manifest",
		}
		addArtifactStat(&item)
		items = append(items, item)
		seen[abs] = struct{}{}
	}
	scanned, err := scanArtifactDir(files, seen)
	if err != nil {
		return nil, err
	}
	items = append(items, scanned...)
	items = filterArtifactItems(items, filter)
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].ModTime.Equal(items[j].ModTime) {
			return items[i].ModTime.Before(items[j].ModTime)
		}
		return items[i].Path < items[j].Path
	})
	for i := range items {
		items[i].Index = i + 1
	}
	return items, nil
}

func scanArtifactDir(files viewer.RunFiles, seen map[string]struct{}) ([]artifactListItem, error) {
	if strings.TrimSpace(files.ArtifactsDir) == "" {
		return nil, nil
	}
	if _, err := os.Stat(files.ArtifactsDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := []artifactListItem{}
	err := filepath.WalkDir(files.ArtifactsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if _, ok := seen[abs]; ok {
			return nil
		}
		item := artifactListItem{
			Kind:    inferArtifactKind(path),
			Path:    displayArtifactPath(files.RunDir, abs),
			AbsPath: abs,
			Source:  "scan",
		}
		addArtifactStat(&item)
		out = append(out, item)
		return nil
	})
	return out, err
}

func writeArtifactList(writer interface{ Write([]byte) (int, error) }, items []artifactListItem, format string) error {
	switch strings.TrimSpace(format) {
	case "", "text":
		if len(items) == 0 {
			_, _ = fmt.Fprintln(writer, "no artifacts found")
			return nil
		}
		for _, item := range items {
			_, _ = fmt.Fprintf(writer, "%3d  %-18s %8d  %s\n", item.Index, item.Kind, item.Size, item.Path)
		}
	case "json":
		body, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal artifacts: %w", err)
		}
		_, _ = fmt.Fprintln(writer, string(body))
	default:
		return appError(ExitUsageError, "unsupported artifact list format: "+format, "使用 --format text 或 --format json", nil)
	}
	return nil
}

func resolveArtifactSelection(files viewer.RunFiles, cmd *cli.Command) (string, error) {
	if cmd.Int("id") > 0 {
		items, err := loadArtifactItems(files, "")
		if err != nil {
			return "", err
		}
		id := cmd.Int("id")
		if id > len(items) {
			return "", appError(ExitArtifactNotFound, "artifact id not found: "+strconv.Itoa(id), "先运行 artifact list 查看可用 index", nil)
		}
		return items[id-1].AbsPath, nil
	}
	if cmd.Bool("latest") {
		items, err := loadArtifactItems(files, cmd.String("type"))
		if err != nil {
			return "", err
		}
		if len(items) == 0 {
			return "", appError(ExitArtifactNotFound, "no artifact matches latest selection", "调整 --type 或先运行 artifact list", nil)
		}
		return items[len(items)-1].AbsPath, nil
	}
	if rawPath := strings.TrimSpace(cmd.String("path")); rawPath != "" {
		return resolveArtifactPath(files, rawPath)
	}
	return "", appError(ExitUsageError, "artifact show requires --id, --path, or --latest", "例如 artifact show --result ./out/tc_x/result.json --id 1", nil)
}

func showArtifact(writer interface{ Write([]byte) (int, error) }, path string, limit int, raw bool) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return appError(ExitArtifactNotFound, "artifact not found: "+path, "", err)
		}
		return err
	}
	truncated := false
	if limit > 0 && len(body) > limit {
		body = body[:limit]
		truncated = true
	}
	if !raw && json.Valid(body) {
		var value any
		if err := json.Unmarshal(body, &value); err == nil {
			if pretty, err := json.MarshalIndent(value, "", "  "); err == nil {
				body = pretty
			}
		}
	}
	_, _ = writer.Write(body)
	if len(body) == 0 || body[len(body)-1] != '\n' {
		_, _ = fmt.Fprintln(writer)
	}
	if truncated {
		_, _ = fmt.Fprintf(writer, "\n[truncated at %d bytes]\n", limit)
	}
	return nil
}

func filterArtifactItems(items []artifactListItem, filter string) []artifactListItem {
	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter == "" {
		return items
	}
	out := make([]artifactListItem, 0, len(items))
	for _, item := range items {
		haystack := strings.ToLower(strings.Join([]string{item.Kind, item.Path, item.EntryID, item.ClaimID, item.ChallengeID}, " "))
		if strings.Contains(haystack, filter) {
			out = append(out, item)
		}
	}
	return out
}

func resolveArtifactPath(files viewer.RunFiles, rawPath string) (string, error) {
	if filepath.IsAbs(rawPath) {
		return filepath.Abs(rawPath)
	}
	clean := filepath.Clean(rawPath)
	if clean == "." || clean == "" {
		return "", appError(ExitArtifactInvalid, "empty artifact path", "", nil)
	}
	if strings.HasPrefix(clean, "artifacts"+string(filepath.Separator)) {
		return filepath.Abs(filepath.Join(files.RunDir, clean))
	}
	return filepath.Abs(filepath.Join(files.ArtifactsDir, clean))
}

func absoluteArtifactPath(runDir string, path string) string {
	if filepath.IsAbs(path) {
		abs, _ := filepath.Abs(path)
		return abs
	}
	abs, _ := filepath.Abs(filepath.Join(runDir, path))
	return abs
}

func displayArtifactPath(runDir string, abs string) string {
	if rel, err := filepath.Rel(runDir, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return abs
}

func pathInside(base string, target string) bool {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func addArtifactStat(item *artifactListItem) {
	info, err := os.Stat(item.AbsPath)
	if err != nil {
		return
	}
	item.Size = info.Size()
	item.ModTime = info.ModTime()
}

func inferArtifactKind(path string) string {
	name := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(name, "decode-error") || strings.Contains(name, "raw-error") || strings.Contains(name, "failure") || strings.Contains(name, "error"):
		return "error"
	case strings.Contains(name, "raw"):
		return "raw"
	case strings.Contains(name, "input"):
		return "input"
	case strings.Contains(name, "telemetry") || strings.Contains(name, "compliance") || strings.Contains(name, "readiness"):
		return "telemetry"
	case strings.Contains(name, "repair"):
		return "repair"
	default:
		return "artifact"
	}
}
