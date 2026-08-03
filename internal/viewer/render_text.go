package viewer

import (
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
)

func renderText(doc Document, verbose bool) string {
	var b strings.Builder
	requested := sectionSet(doc.RequestedSections)
	if shouldRenderSection(doc, requested, SectionOverview) {
		writeTextHeading(&b, "运行头部")
		fmt.Fprintf(&b, "requestId: %s\n", doc.Overview.RequestID)
		fmt.Fprintf(&b, "mode: %s\n", doc.Overview.Mode)
		if doc.Overview.ParentRequestID != "" || doc.Overview.ParentSessionID != "" {
			fmt.Fprintf(&b, "parent: request=%s session=%s case=%s trigger=%s\n", firstNonEmpty(doc.Overview.ParentRequestID, "-"), firstNonEmpty(doc.Overview.ParentSessionID, "-"), firstNonEmpty(doc.Overview.ParentCaseID, "-"), firstNonEmpty(doc.Overview.Trigger, "-"))
		}
		fmt.Fprintf(&b, "result: %s\n", doc.Overview.PrimaryResult)
		if doc.Overview.TerminalState != "" {
			fmt.Fprintf(&b, "terminal state: %s\n", doc.Overview.TerminalState)
		}
		fmt.Fprintf(&b, "elapsed: %s\n", doc.Overview.Elapsed)
		fmt.Fprintf(&b, "result dir: %s\n", doc.Files.RunDir)

		writeTextHeading(&b, "任务摘要")
		fmt.Fprintf(&b, "goal: %s\n", doc.Overview.Goal)
		if doc.Overview.TaskType != "" {
			fmt.Fprintf(&b, "task type: %s\n", doc.Overview.TaskType)
		}
		if doc.Overview.RiskLevel != "" || doc.Overview.RequiredEvidence != "" {
			fmt.Fprintf(&b, "risk/evidence: %s / %s\n", firstNonEmpty(doc.Overview.RiskLevel, "-"), firstNonEmpty(doc.Overview.RequiredEvidence, "-"))
		}
		if len(doc.Overview.SuccessCriteria) > 0 {
			b.WriteString("success criteria:\n")
			for _, item := range doc.Overview.SuccessCriteria {
				fmt.Fprintf(&b, "  - %s\n", item)
			}
		}
		if doc.Overview.WorkspaceRoot != "" {
			b.WriteString("workspace:\n")
			fmt.Fprintf(&b, "  root: %s\n", doc.Overview.WorkspaceRoot)
			if doc.Overview.WorkspaceRev != "" {
				fmt.Fprintf(&b, "  revision: %s\n", doc.Overview.WorkspaceRev)
			}
		}

		writeTextHeading(&b, "统计")
		if len(doc.Overview.ClaimCounts) > 0 {
			fmt.Fprintf(&b, "claims: %d\n", sumCounts(doc.Overview.ClaimCounts))
			fmt.Fprintf(&b, "supported: %d\n", doc.Overview.ClaimCounts[string(consensus.ClaimVerdictSupported)])
			fmt.Fprintf(&b, "refuted: %d\n", doc.Overview.ClaimCounts[string(consensus.ClaimVerdictRefuted)])
			fmt.Fprintf(&b, "insufficient evidence: %d\n", doc.Overview.ClaimCounts[string(consensus.ClaimVerdictInsufficientEvidence)])
			fmt.Fprintf(&b, "undetermined: %d\n", doc.Overview.ClaimCounts[string(consensus.ClaimVerdictUndetermined)])
			fmt.Fprintf(&b, "challenges: %d\n", doc.Overview.ChallengeCount)
			fmt.Fprintf(&b, "verifications: %d\n", doc.Overview.VerificationCount)
		}
		if doc.Overview.RoundCount > 0 {
			fmt.Fprintf(&b, "rounds: %d\n", doc.Overview.RoundCount)
		}
		fmt.Fprintf(&b, "artifacts: %d\n", doc.Overview.ArtifactCount)
	}

	if shouldRenderSection(doc, requested, SectionClaims) {
		writeTextHeading(&b, "关键 Claims")
		if len(doc.Claims) == 0 {
			b.WriteString("(无 claims)\n")
		}
		for _, claim := range doc.Claims {
			fmt.Fprintf(&b, "[%s/%s] %s (%.2f)\n", claim.Verdict, firstNonEmpty(claim.Disposition, "-"), firstNonEmpty(claim.Title, claim.ClaimID), claim.Confidence)
			fmt.Fprintf(&b, "  statement: %s\n", claim.Statement)
			if verbose && claim.ClaimType != "" {
				fmt.Fprintf(&b, "  claim type: %s\n", claim.ClaimType)
			}
			if verbose && claim.Rationale != "" {
				fmt.Fprintf(&b, "  rationale: %s\n", claim.Rationale)
			}
			if verbose && len(claim.Caveats) > 0 {
				fmt.Fprintf(&b, "  caveats: %s\n", strings.Join(claim.Caveats, "; "))
			}
		}
	}

	if shouldRenderSection(doc, requested, SectionChallenges) {
		writeTextHeading(&b, "挑战明细")
		if len(doc.Challenges) == 0 {
			b.WriteString("(无 challenges)\n")
		}
		for _, item := range doc.Challenges {
			fmt.Fprintf(&b, "- %s | %s | %s | %s\n", item.TicketID, item.ClaimID, firstNonEmpty(item.AttackType, item.Kind), item.Status)
			fmt.Fprintf(&b, "  %s\n", item.Statement)
			if verbose && item.Severity != "" {
				fmt.Fprintf(&b, "  severity: %s\n", item.Severity)
			}
		}
	}

	if shouldRenderSection(doc, requested, SectionVerifications) {
		writeTextHeading(&b, "验证明细")
		if len(doc.Verifications) == 0 {
			b.WriteString("(无 verifications)\n")
		}
		for _, item := range doc.Verifications {
			fmt.Fprintf(&b, "- %s | %s | %s\n", item.ClaimID, item.Kind, item.Status)
			fmt.Fprintf(&b, "  %s\n", item.Summary)
			if verbose && item.ArtifactPath != "" {
				fmt.Fprintf(&b, "  artifact: %s\n", item.ArtifactPath)
			}
		}
	}

	if shouldRenderSection(doc, requested, SectionObservations) {
		writeTextHeading(&b, "Observations")
		if len(doc.Observations) == 0 {
			b.WriteString("(无 observation 数据)\n")
		}
		for _, item := range doc.Observations {
			fmt.Fprintf(&b, "- %s | %s\n", item.ObservationID, item.Outcome)
			fmt.Fprintf(&b, "  %s\n", item.Summary)
			if item.Reopen {
				fmt.Fprintf(&b, "  reopen: true\n")
			}
			if item.FollowUpCaseID != "" || item.FollowUpRequestID != "" {
				fmt.Fprintf(&b, "  follow-up: case=%s request=%s\n", firstNonEmpty(item.FollowUpCaseID, "-"), firstNonEmpty(item.FollowUpRequestID, "-"))
			}
			if verbose && item.FollowUpArtifact != "" {
				fmt.Fprintf(&b, "  artifact: %s\n", item.FollowUpArtifact)
			}
		}
	}

	if shouldRenderSection(doc, requested, SectionFollowups) {
		writeTextHeading(&b, "Follow-ups")
		if len(doc.FollowUps) == 0 {
			b.WriteString("(无 follow-up 数据)\n")
		}
		for _, item := range doc.FollowUps {
			fmt.Fprintf(&b, "- child request=%s | case=%s\n", firstNonEmpty(item.FollowUpRequestID, "-"), firstNonEmpty(item.FollowUpCaseID, "-"))
			fmt.Fprintf(&b, "  parent request=%s session=%s case=%s\n", firstNonEmpty(item.ParentRequestID, "-"), firstNonEmpty(item.ParentSessionID, "-"), firstNonEmpty(item.ParentCaseID, "-"))
			if item.ObservationID != "" {
				fmt.Fprintf(&b, "  triggered by observation=%s\n", item.ObservationID)
			}
			if item.Trigger != "" {
				fmt.Fprintf(&b, "  trigger=%s\n", item.Trigger)
			}
			if verbose && item.ArtifactPath != "" {
				fmt.Fprintf(&b, "  artifact: %s\n", item.ArtifactPath)
			}
		}
	}

	if shouldRenderSection(doc, requested, SectionDebug) {
		writeTextHeading(&b, "Debug Events")
		if len(doc.DebugEvents) == 0 {
			b.WriteString("(无 debug 事件)\n")
		}
		for _, item := range doc.DebugEvents {
			fmt.Fprintf(&b, "- #%d | %s | %s", item.Seq, firstNonEmpty(item.LoggedAt, item.EventAt), item.Type)
			if item.Phase != "" {
				fmt.Fprintf(&b, " | %s", item.Phase)
			}
			b.WriteString("\n")
			if item.Summary != "" {
				fmt.Fprintf(&b, "  %s\n", item.Summary)
			}
			if item.RawVerdict != "" {
				fmt.Fprintf(&b, "  rawVerdict: %s\n", item.RawVerdict)
			}
			if item.RawTaskVerdict != "" {
				fmt.Fprintf(&b, "  rawTaskVerdict: %s\n", item.RawTaskVerdict)
			}
			if len(item.ArtifactHints) > 0 {
				fmt.Fprintf(&b, "  artifacts: %s\n", strings.Join(item.ArtifactHints, ", "))
			}
			if verbose && item.PayloadPretty != "" {
				for _, line := range strings.Split(strings.TrimSpace(item.PayloadPretty), "\n") {
					fmt.Fprintf(&b, "  %s\n", line)
				}
			}
		}
		writeTextHeading(&b, "Telemetry")
		if doc.Telemetry == nil || (len(doc.Telemetry.Summary) == 0 && len(doc.Telemetry.Reports) == 0 && doc.Telemetry.Readiness == nil && doc.Telemetry.Run == nil) {
			b.WriteString("(无 telemetry 数据)\n")
		} else {
			if doc.Telemetry.GeneratedAt != "" {
				fmt.Fprintf(&b, "generated at: %s\n", doc.Telemetry.GeneratedAt)
			}
			if doc.Telemetry.Readiness != nil {
				if doc.Telemetry.Readiness.GeneratedAt != "" {
					fmt.Fprintf(&b, "provider readiness generated at: %s\n", doc.Telemetry.Readiness.GeneratedAt)
				}
				if len(doc.Telemetry.Readiness.Providers) > 0 {
					b.WriteString("provider readiness:\n")
					for _, item := range doc.Telemetry.Readiness.Providers {
						label := item.Provider
						if item.Model != "" {
							label += "/" + item.Model
						}
						if item.Agent != "" {
							label += " agent=" + item.Agent
						}
						fmt.Fprintf(&b, "- %s | ready=%t strict=%t recoverable=%t duration=%dms\n", label, item.Ready, item.StrictJSON, item.RecoverableJSON, item.DurationMs)
						if item.Error != "" {
							fmt.Fprintf(&b, "  error: %s\n", item.Error)
						}
						if verbose {
							if item.ProviderType != "" || item.Protocol != "" {
								fmt.Fprintf(&b, "  provider: type=%s protocol=%s\n", firstNonEmpty(item.ProviderType, "-"), firstNonEmpty(item.Protocol, "-"))
							}
							if item.BaseURL != "" {
								fmt.Fprintf(&b, "  base_url: %s\n", item.BaseURL)
							}
							if item.APIKeyEnv != "" {
								fmt.Fprintf(&b, "  api_key_env: %s\n", item.APIKeyEnv)
							}
							if len(item.Command) > 0 {
								fmt.Fprintf(&b, "  command: %s\n", strings.Join(item.Command, " "))
							}
							if len(item.RequestContext) > 0 {
								if body, err := prettyJSON(item.RequestContext, "  ", "  "); err == nil {
									fmt.Fprintf(&b, "  request_context:\n%s\n", body)
								}
							}
							if item.StdoutPreview != "" {
								fmt.Fprintf(&b, "  stdout: %s\n", item.StdoutPreview)
							}
							if item.StderrPreview != "" {
								fmt.Fprintf(&b, "  stderr: %s\n", item.StderrPreview)
							}
						}
					}
				}
			}
			if doc.Telemetry.Run != nil {
				b.WriteString("run summary:\n")
				fmt.Fprintf(&b, "- request=%s session=%s mode=%s\n", firstNonEmpty(doc.Telemetry.Run.RequestID, "-"), firstNonEmpty(doc.Telemetry.Run.SessionID, "-"), firstNonEmpty(doc.Telemetry.Run.Mode, "-"))
				fmt.Fprintf(&b, "- primary=%s taskVerdict=%s terminal=%s elapsed=%dms\n", firstNonEmpty(doc.Telemetry.Run.Primary, "-"), firstNonEmpty(doc.Telemetry.Run.TaskVerdict, "-"), firstNonEmpty(doc.Telemetry.Run.Terminal, "-"), doc.Telemetry.Run.ElapsedMs)
				if len(doc.Telemetry.Run.Providers) > 0 {
					fmt.Fprintf(&b, "- providers: %s\n", strings.Join(doc.Telemetry.Run.Providers, ", "))
				}
				fmt.Fprintf(&b, "- workflow: claims=%d keep=%d keep_with_caveat=%d unresolved=%d reject=%d observations=%d\n",
					doc.Telemetry.Run.Workflow.Claims,
					doc.Telemetry.Run.Workflow.KeepClaims,
					doc.Telemetry.Run.Workflow.KeepWithCaveatClaims,
					doc.Telemetry.Run.Workflow.UnresolvedClaims,
					doc.Telemetry.Run.Workflow.RejectClaims,
					doc.Telemetry.Run.Workflow.ObservationCount,
				)
				if doc.Telemetry.Run.Verify != nil {
					fmt.Fprintf(&b, "- verification: pass=%d fail=%d inconclusive=%d\n", doc.Telemetry.Run.Verify.Passed, doc.Telemetry.Run.Verify.Failed, doc.Telemetry.Run.Verify.Inconclusive)
				}
				if len(doc.Telemetry.Run.TaskSummary) > 0 {
					b.WriteString("task summary:\n")
					for _, item := range doc.Telemetry.Run.TaskSummary {
						fmt.Fprintf(&b, "- %s | total=%d strict=%d normalized=%d repaired=%d failed=%d\n", item.TaskKind, item.Total, item.Strict, item.Normalized, item.Repaired, item.Failed)
					}
				}
			}
			if len(doc.Telemetry.Summary) > 0 {
				b.WriteString("summary:\n")
				for _, item := range doc.Telemetry.Summary {
					fmt.Fprintf(&b, "- %s/%s | %s | total=%d strict=%d normalized=%d repaired=%d failed=%d\n", item.Provider, item.ProviderModel, item.TaskKind, item.Total, item.Strict, item.Normalized, item.Repaired, item.Failed)
				}
			}
			if len(doc.Telemetry.Reports) > 0 {
				b.WriteString("reports:\n")
				for _, item := range doc.Telemetry.Reports {
					fmt.Fprintf(&b, "- %s | %s/%s | %s\n", firstNonEmpty(item.TaskID, "-"), item.Provider, item.ProviderModel, item.FinalStatus)
					fmt.Fprintf(&b, "  task=%s agent=%s strict=%t normalized=%t repair=%t/%t\n", firstNonEmpty(item.TaskKind, "-"), firstNonEmpty(item.AgentID, "-"), item.StrictCompliant, item.NormalizedWithoutFix, item.RepairAttempted, item.RepairSucceeded)
					if item.StrictError != "" {
						fmt.Fprintf(&b, "  strictError: %s\n", item.StrictError)
					}
					if item.FinalError != "" {
						fmt.Fprintf(&b, "  finalError: %s\n", item.FinalError)
					}
					if verbose {
						if item.RawArtifact != "" {
							fmt.Fprintf(&b, "  rawArtifact: %s\n", item.RawArtifact)
						}
						if item.InitialErrorArtifact != "" {
							fmt.Fprintf(&b, "  initialErrorArtifact: %s\n", item.InitialErrorArtifact)
						}
						if item.FinalArtifact != "" {
							fmt.Fprintf(&b, "  finalArtifact: %s\n", item.FinalArtifact)
						}
					}
				}
			}
		}
	}

	if shouldRenderSection(doc, requested, SectionRounds) {
		writeTextHeading(&b, "Rounds")
		if len(doc.Rounds) == 0 {
			b.WriteString("(无 round 数据)\n")
		}
		for _, item := range doc.Rounds {
			fmt.Fprintf(&b, "- round %d | %s\n", item.Round, item.Phase)
			if item.Summary != "" {
				fmt.Fprintf(&b, "  %s\n", item.Summary)
			}
			if verbose && len(item.Participants) > 0 {
				fmt.Fprintf(&b, "  participants: %s\n", strings.Join(item.Participants, ", "))
			}
		}
	}

	if shouldRenderSection(doc, requested, SectionVotes) {
		writeTextHeading(&b, "Votes")
		if len(doc.Votes) == 0 {
			b.WriteString("(无 vote 数据)\n")
		}
		for _, item := range doc.Votes {
			fmt.Fprintf(&b, "- %s | %s | %s | confidence=%.2f\n", item.ClaimID, item.AgentID, item.Vote, item.Confidence)
			if verbose && item.Rationale != "" {
				fmt.Fprintf(&b, "  %s\n", item.Rationale)
			}
		}
	}

	if shouldRenderSection(doc, requested, SectionStatements) {
		writeTextHeading(&b, "Statements")
		if len(doc.Statements) == 0 {
			b.WriteString("(无 statement 数据)\n")
		}
		for _, item := range doc.Statements {
			fmt.Fprintf(&b, "- %s | mean=%.2f | consensus=%.2f\n", item.Statement, item.MeanRating, item.ConsensusLevel)
			if verbose && len(item.Reasons) > 0 {
				fmt.Fprintf(&b, "  reasons: %s\n", strings.Join(item.Reasons, "; "))
			}
		}
	}

	if shouldRenderSection(doc, requested, SectionConvergence) {
		writeTextHeading(&b, "Convergence")
		if doc.Convergence == nil {
			b.WriteString("(无 convergence 数据)\n")
		} else {
			fmt.Fprintf(&b, "consensus level: %.2f\n", doc.Convergence.ConsensusLevel)
			fmt.Fprintf(&b, "recommendation: %s\n", firstNonEmpty(doc.Convergence.Recommendation, "未形成明确推荐"))
			for _, item := range doc.Convergence.DissentSummary {
				fmt.Fprintf(&b, "- %s\n", item)
			}
		}
	}

	writeTextHeading(&b, "风险与未决项")
	if len(doc.Risks) == 0 {
		b.WriteString("(无明显风险)\n")
	}
	for _, risk := range doc.Risks {
		fmt.Fprintf(&b, "- %s | %s | %s\n", risk.Category, risk.TargetID, risk.Summary)
	}

	if shouldRenderSection(doc, requested, SectionArtifacts) {
		writeTextHeading(&b, "相关文件")
		fmt.Fprintf(&b, "- result.json: %s\n", doc.Files.Result)
		fmt.Fprintf(&b, "- ledger.jsonl: %s\n", doc.Files.Ledger)
		fmt.Fprintf(&b, "- summary.md: %s\n", doc.Files.Summary)
		if len(doc.Files.Missing) > 0 {
			fmt.Fprintf(&b, "- artifacts/manifest.jsonl: 缺失 (%s)\n", strings.Join(doc.Files.Missing, ", "))
		} else {
			fmt.Fprintf(&b, "- artifacts/manifest.jsonl: %s\n", doc.Files.Manifest)
		}
		for _, item := range doc.Artifacts {
			fmt.Fprintf(&b, "- %s | %s\n", item.Path, item.Kind)
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}
