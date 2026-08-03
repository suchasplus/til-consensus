package viewer

import (
	"fmt"
	"strings"
)

func renderMarkdown(doc Document, verbose bool) string {
	var b strings.Builder
	b.WriteString("# til-consensus 结果浏览\n\n")
	fmt.Fprintf(&b, "- requestId: `%s`\n", doc.Overview.RequestID)
	fmt.Fprintf(&b, "- mode: `%s`\n", doc.Overview.Mode)
	if doc.Overview.ParentRequestID != "" || doc.Overview.ParentSessionID != "" {
		fmt.Fprintf(&b, "- parent: request=`%s` session=`%s` case=`%s` trigger=`%s`\n", firstNonEmpty(doc.Overview.ParentRequestID, "-"), firstNonEmpty(doc.Overview.ParentSessionID, "-"), firstNonEmpty(doc.Overview.ParentCaseID, "-"), firstNonEmpty(doc.Overview.Trigger, "-"))
	}
	fmt.Fprintf(&b, "- result: `%s`\n", doc.Overview.PrimaryResult)
	if doc.Overview.TerminalState != "" {
		fmt.Fprintf(&b, "- terminal state: `%s`\n", doc.Overview.TerminalState)
	}
	fmt.Fprintf(&b, "- elapsed: `%s`\n", doc.Overview.Elapsed)
	fmt.Fprintf(&b, "- result dir: `%s`\n\n", doc.Files.RunDir)
	b.WriteString("## Task\n\n")
	fmt.Fprintf(&b, "%s\n\n", doc.Overview.Goal)
	if len(doc.Claims) > 0 {
		b.WriteString("## Claims\n\n")
		for _, claim := range doc.Claims {
			fmt.Fprintf(&b, "- `%s` | `%s` | %.2f | %s\n", claim.ClaimID, claim.Verdict, claim.Confidence, claim.Statement)
			if verbose && claim.Rationale != "" {
				fmt.Fprintf(&b, "  rationale: %s\n", claim.Rationale)
			}
		}
		b.WriteString("\n")
	}
	if len(doc.Rounds) > 0 {
		b.WriteString("## Rounds\n\n")
		for _, round := range doc.Rounds {
			fmt.Fprintf(&b, "- round %d | `%s` | %s\n", round.Round, round.Phase, round.Summary)
		}
		b.WriteString("\n")
	}
	if len(doc.Statements) > 0 {
		b.WriteString("## Statements\n\n")
		for _, item := range doc.Statements {
			fmt.Fprintf(&b, "- `%s` | mean=%.2f | consensus=%.2f\n", item.Statement, item.MeanRating, item.ConsensusLevel)
		}
		b.WriteString("\n")
	}
	if len(doc.Observations) > 0 {
		b.WriteString("## Observations\n\n")
		for _, item := range doc.Observations {
			fmt.Fprintf(&b, "- `%s` | `%s` | %s\n", item.ObservationID, item.Outcome, item.Summary)
			if item.FollowUpRequestID != "" || item.FollowUpCaseID != "" {
				fmt.Fprintf(&b, "  follow-up: request=`%s` case=`%s`\n", firstNonEmpty(item.FollowUpRequestID, "-"), firstNonEmpty(item.FollowUpCaseID, "-"))
			}
			if verbose && item.FollowUpArtifact != "" {
				fmt.Fprintf(&b, "  artifact: `%s`\n", item.FollowUpArtifact)
			}
		}
		b.WriteString("\n")
	}
	if len(doc.FollowUps) > 0 {
		b.WriteString("## Follow-ups\n\n")
		for _, item := range doc.FollowUps {
			fmt.Fprintf(&b, "- child request=`%s` case=`%s` <- parent request=`%s`\n", firstNonEmpty(item.FollowUpRequestID, "-"), firstNonEmpty(item.FollowUpCaseID, "-"), firstNonEmpty(item.ParentRequestID, "-"))
			if item.ObservationID != "" {
				fmt.Fprintf(&b, "  observation: `%s`\n", item.ObservationID)
			}
		}
		b.WriteString("\n")
	}
	if len(doc.DebugEvents) > 0 {
		b.WriteString("## Debug Events\n\n")
		for _, item := range doc.DebugEvents {
			fmt.Fprintf(&b, "- `#%d` | `%s` | `%s`", item.Seq, firstNonEmpty(item.LoggedAt, item.EventAt), item.Type)
			if item.Phase != "" {
				fmt.Fprintf(&b, " | `%s`", item.Phase)
			}
			b.WriteString("\n")
			if item.Summary != "" {
				fmt.Fprintf(&b, "  %s\n", item.Summary)
			}
			if item.RawVerdict != "" {
				fmt.Fprintf(&b, "  rawVerdict: `%s`\n", item.RawVerdict)
			}
			if item.RawTaskVerdict != "" {
				fmt.Fprintf(&b, "  rawTaskVerdict: `%s`\n", item.RawTaskVerdict)
			}
			if len(item.ArtifactHints) > 0 {
				fmt.Fprintf(&b, "  artifacts: `%s`\n", strings.Join(item.ArtifactHints, "`, `"))
			}
		}
		b.WriteString("\n")
	}
	if doc.Telemetry != nil && (len(doc.Telemetry.Summary) > 0 || len(doc.Telemetry.Reports) > 0 || doc.Telemetry.Readiness != nil || doc.Telemetry.Run != nil) {
		b.WriteString("## Telemetry\n\n")
		if doc.Telemetry.GeneratedAt != "" {
			fmt.Fprintf(&b, "- generatedAt: `%s`\n", doc.Telemetry.GeneratedAt)
		}
		if doc.Telemetry.Readiness != nil {
			if doc.Telemetry.Readiness.GeneratedAt != "" {
				fmt.Fprintf(&b, "- readinessGeneratedAt: `%s`\n", doc.Telemetry.Readiness.GeneratedAt)
			}
			for _, item := range doc.Telemetry.Readiness.Providers {
				label := item.Provider
				if item.Model != "" {
					label += "/" + item.Model
				}
				if item.Agent != "" {
					label += " agent=" + item.Agent
				}
				fmt.Fprintf(&b, "- readiness | `%s` | ready=%t strict=%t recoverable=%t duration=%dms\n", label, item.Ready, item.StrictJSON, item.RecoverableJSON, item.DurationMs)
				if item.Error != "" {
					fmt.Fprintf(&b, "  error: `%s`\n", item.Error)
				}
			}
		}
		if doc.Telemetry.Run != nil {
			fmt.Fprintf(&b, "- run | request=`%s` session=`%s` mode=`%s` primary=`%s` taskVerdict=`%s` terminal=`%s` elapsed=%dms\n",
				firstNonEmpty(doc.Telemetry.Run.RequestID, "-"),
				firstNonEmpty(doc.Telemetry.Run.SessionID, "-"),
				firstNonEmpty(doc.Telemetry.Run.Mode, "-"),
				firstNonEmpty(doc.Telemetry.Run.Primary, "-"),
				firstNonEmpty(doc.Telemetry.Run.TaskVerdict, "-"),
				firstNonEmpty(doc.Telemetry.Run.Terminal, "-"),
				doc.Telemetry.Run.ElapsedMs,
			)
			for _, item := range doc.Telemetry.Run.TaskSummary {
				fmt.Fprintf(&b, "  - task | `%s` | total=%d strict=%d normalized=%d repaired=%d failed=%d\n", item.TaskKind, item.Total, item.Strict, item.Normalized, item.Repaired, item.Failed)
			}
		}
		for _, item := range doc.Telemetry.Summary {
			fmt.Fprintf(&b, "- summary | `%s/%s` | `%s` | total=%d strict=%d normalized=%d repaired=%d failed=%d\n", item.Provider, item.ProviderModel, item.TaskKind, item.Total, item.Strict, item.Normalized, item.Repaired, item.Failed)
		}
		for _, item := range doc.Telemetry.Reports {
			fmt.Fprintf(&b, "- report | `%s` | `%s/%s` | `%s`\n", firstNonEmpty(item.TaskID, "-"), item.Provider, item.ProviderModel, item.FinalStatus)
			if item.StrictError != "" {
				fmt.Fprintf(&b, "  strictError: `%s`\n", item.StrictError)
			}
			if item.FinalError != "" {
				fmt.Fprintf(&b, "  finalError: `%s`\n", item.FinalError)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("## 风险与未决项\n\n")
	if len(doc.Risks) == 0 {
		b.WriteString("_无明显风险_\n\n")
	} else {
		for _, risk := range doc.Risks {
			fmt.Fprintf(&b, "- %s | `%s` | %s\n", risk.Category, risk.TargetID, risk.Summary)
		}
		b.WriteString("\n")
	}
	b.WriteString("## 相关文件\n\n")
	fmt.Fprintf(&b, "- result.json: `%s`\n", doc.Files.Result)
	fmt.Fprintf(&b, "- ledger.jsonl: `%s`\n", doc.Files.Ledger)
	fmt.Fprintf(&b, "- summary.md: `%s`\n", doc.Files.Summary)
	fmt.Fprintf(&b, "- artifacts/manifest.jsonl: `%s`\n", doc.Files.Manifest)
	return b.String()
}
