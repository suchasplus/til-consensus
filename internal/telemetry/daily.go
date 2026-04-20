package telemetry

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func BuildDailyReport(root string, since time.Time, now time.Time) (DailyReport, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return DailyReport{}, fmt.Errorf("telemetry root is empty")
	}
	report := DailyReport{
		Version:     1,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Root:        root,
		Since:       since.UTC().Format(time.RFC3339),
	}

	readinessIndex := map[string]*DailyReadinessSummary{}
	taskIndex := map[string]*DailyTaskSummary{}
	workflowIndex := map[string]*DailyWorkflowSummary{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.ModTime().After(since) {
			return nil
		}
		switch name {
		case "provider-readiness.json":
			file, err := ReadProviderReadinessFile(path)
			if err != nil {
				return err
			}
			for _, item := range file.Providers {
				entry := readinessIndex[item.Provider]
				if entry == nil {
					entry = &DailyReadinessSummary{Provider: item.Provider}
					readinessIndex[item.Provider] = entry
				}
				entry.Samples++
				if item.Ready {
					entry.ReadyCount++
				}
				if item.StrictJSON {
					entry.StrictJSONCount++
				}
				if item.RecoverableJSON {
					entry.RecoverableJSONCount++
				}
				entry.MeanDurationMs += float64(item.DurationMs)
				if item.Error != "" {
					entry.LastError = item.Error
				}
			}
		case "run-telemetry.json":
			file, err := ReadRunTelemetryFile(path)
			if err != nil {
				return err
			}
			report.RecentRunReports = append(report.RecentRunReports, DailyRunReportSummary{
				RequestID:     file.RequestID,
				Mode:          string(file.Mode),
				PrimaryResult: file.Result.PrimaryResult,
				TaskVerdict:   file.Result.TaskVerdict,
				TerminalState: string(file.Result.TerminalState),
				RunDir:        filepath.Dir(filepath.Dir(path)),
			})
			workflow := workflowIndex[string(file.Mode)]
			if workflow == nil {
				workflow = &DailyWorkflowSummary{Mode: string(file.Mode)}
				workflowIndex[string(file.Mode)] = workflow
			}
			workflow.Runs++
			if file.Result.TerminalState == "" || file.Result.TerminalState == "completed" {
				workflow.Completed++
			}
			workflow.MeanKeepWithCaveatClaims += float64(file.WorkflowSummary.KeepWithCaveatClaims)
			workflow.MeanUnresolvedClaims += float64(file.WorkflowSummary.UnresolvedClaims)
			workflow.MeanElapsedMs += float64(file.Timing.ElapsedMs)

			summaryPath := filepath.Join(filepath.Dir(path), "strict-compliance-summary.json")
			if _, err := os.Stat(summaryPath); err == nil {
				summary, err := ReadComplianceSummaryFile(summaryPath)
				if err != nil {
					return err
				}
				for _, item := range summary.Entries {
					key := strings.Join([]string{item.Provider, item.ProviderModel, string(file.Mode), string(item.TaskKind)}, "|")
					entry := taskIndex[key]
					if entry == nil {
						entry = &DailyTaskSummary{
							Provider:      item.Provider,
							ProviderModel: item.ProviderModel,
							Mode:          string(file.Mode),
							TaskKind:      string(item.TaskKind),
						}
						taskIndex[key] = entry
					}
					entry.Runs++
					entry.Total += item.Total
					entry.Strict += item.Strict
					entry.Normalized += item.Normalized
					entry.Repaired += item.Repaired
					entry.Failed += item.Failed
				}
			}
		}
		return nil
	})
	if err != nil {
		return DailyReport{}, err
	}

	for _, item := range readinessIndex {
		if item.Samples > 0 {
			item.MeanDurationMs = item.MeanDurationMs / float64(item.Samples)
		}
		report.Readiness = append(report.Readiness, *item)
	}
	for _, item := range taskIndex {
		report.TaskCompliance = append(report.TaskCompliance, *item)
	}
	for _, item := range workflowIndex {
		if item.Runs > 0 {
			item.MeanKeepWithCaveatClaims /= float64(item.Runs)
			item.MeanUnresolvedClaims /= float64(item.Runs)
			item.MeanElapsedMs /= float64(item.Runs)
		}
		report.Workflow = append(report.Workflow, *item)
	}

	slices.SortFunc(report.Readiness, func(a, b DailyReadinessSummary) int {
		return strings.Compare(a.Provider, b.Provider)
	})
	slices.SortFunc(report.TaskCompliance, func(a, b DailyTaskSummary) int {
		if a.Provider != b.Provider {
			return strings.Compare(a.Provider, b.Provider)
		}
		if a.Mode != b.Mode {
			return strings.Compare(a.Mode, b.Mode)
		}
		return strings.Compare(a.TaskKind, b.TaskKind)
	})
	slices.SortFunc(report.Workflow, func(a, b DailyWorkflowSummary) int {
		return strings.Compare(a.Mode, b.Mode)
	})
	slices.SortFunc(report.RecentRunReports, func(a, b DailyRunReportSummary) int {
		return strings.Compare(a.RequestID, b.RequestID)
	})
	return report, nil
}

func RenderDailyMarkdown(report DailyReport) string {
	var b strings.Builder
	b.WriteString("# 每日 Telemetry 汇总\n\n")
	fmt.Fprintf(&b, "- generatedAt: `%s`\n", report.GeneratedAt)
	fmt.Fprintf(&b, "- root: `%s`\n", report.Root)
	fmt.Fprintf(&b, "- since: `%s`\n\n", report.Since)

	b.WriteString("## Provider Readiness\n\n")
	if len(report.Readiness) == 0 {
		b.WriteString("_没有 readiness 数据_\n\n")
	} else {
		b.WriteString("| provider | samples | ready | strict json | recoverable json | mean duration (ms) | last error |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---|\n")
		for _, item := range report.Readiness {
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %.0f | %s |\n",
				item.Provider,
				item.Samples,
				item.ReadyCount,
				item.StrictJSONCount,
				item.RecoverableJSONCount,
				item.MeanDurationMs,
				escapePipe(item.LastError),
			)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Task Compliance\n\n")
	if len(report.TaskCompliance) == 0 {
		b.WriteString("_没有 task compliance 数据_\n\n")
	} else {
		b.WriteString("| provider | model | mode | task kind | runs | total | strict | normalized | repaired | failed |\n")
		b.WriteString("|---|---|---|---|---:|---:|---:|---:|---:|---:|\n")
		for _, item := range report.TaskCompliance {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %d | %d | %d | %d | %d |\n",
				item.Provider,
				escapePipe(item.ProviderModel),
				item.Mode,
				item.TaskKind,
				item.Runs,
				item.Total,
				item.Strict,
				item.Normalized,
				item.Repaired,
				item.Failed,
			)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Workflow Quality\n\n")
	if len(report.Workflow) == 0 {
		b.WriteString("_没有 workflow 数据_\n\n")
	} else {
		b.WriteString("| mode | runs | completed | mean keep_with_caveat | mean unresolved | mean elapsed (ms) |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|\n")
		for _, item := range report.Workflow {
			fmt.Fprintf(&b, "| %s | %d | %d | %.2f | %.2f | %.0f |\n",
				item.Mode,
				item.Runs,
				item.Completed,
				item.MeanKeepWithCaveatClaims,
				item.MeanUnresolvedClaims,
				item.MeanElapsedMs,
			)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Recent Runs\n\n")
	if len(report.RecentRunReports) == 0 {
		b.WriteString("_没有 run telemetry 数据_\n")
	} else {
		b.WriteString("| requestId | mode | primary result | task verdict | terminal state | run dir |\n")
		b.WriteString("|---|---|---|---|---|---|\n")
		for _, item := range report.RecentRunReports {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
				item.RequestID,
				item.Mode,
				escapePipe(item.PrimaryResult),
				escapePipe(item.TaskVerdict),
				escapePipe(item.TerminalState),
				escapePipe(item.RunDir),
			)
		}
	}
	return b.String()
}

func escapePipe(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return strings.ReplaceAll(value, "|", "\\|")
}
