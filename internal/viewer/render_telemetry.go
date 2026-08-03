package viewer

func buildTelemetry(bundle Bundle, limit int) *TelemetryView {
	if len(bundle.ComplianceSummary.Entries) == 0 && len(bundle.ComplianceReports) == 0 && len(bundle.ProviderReadiness.Providers) == 0 && bundle.RunTelemetry.RequestID == "" {
		return nil
	}
	view := &TelemetryView{
		GeneratedAt: bundle.ComplianceSummary.GeneratedAt,
	}
	if len(bundle.ProviderReadiness.Providers) > 0 {
		view.Readiness = &ProviderReadinessView{
			GeneratedAt: bundle.ProviderReadiness.GeneratedAt,
		}
		for _, item := range bundle.ProviderReadiness.Providers[:min(limit, len(bundle.ProviderReadiness.Providers))] {
			view.Readiness.Providers = append(view.Readiness.Providers, ProviderReadinessEntryView{
				Provider:        item.Provider,
				ProviderType:    item.ProviderType,
				Protocol:        item.Protocol,
				Model:           item.Model,
				BaseURL:         item.BaseURL,
				APIKeyEnv:       item.APIKeyEnv,
				Agent:           item.Agent,
				Command:         append([]string(nil), item.Command...),
				RequestContext:  cloneMap(item.RequestContext),
				Ready:           item.Ready,
				StrictJSON:      item.StrictJSON,
				RecoverableJSON: item.RecoverableJSON,
				DurationMs:      item.DurationMs,
				StdoutPreview:   item.StdoutPreview,
				StderrPreview:   item.StderrPreview,
				Error:           item.Error,
			})
		}
	}
	if bundle.RunTelemetry.RequestID != "" {
		view.Run = &RunTelemetryView{
			GeneratedAt: bundle.RunTelemetry.GeneratedAt,
			RequestID:   bundle.RunTelemetry.RequestID,
			SessionID:   bundle.RunTelemetry.SessionID,
			Mode:        string(bundle.RunTelemetry.Mode),
			Providers:   append([]string(nil), bundle.RunTelemetry.Providers...),
			Primary:     bundle.RunTelemetry.Result.PrimaryResult,
			TaskVerdict: bundle.RunTelemetry.Result.TaskVerdict,
			Terminal:    string(bundle.RunTelemetry.Result.TerminalState),
			ElapsedMs:   bundle.RunTelemetry.Timing.ElapsedMs,
			Workflow: RunWorkflowSummaryView{
				Claims:               bundle.RunTelemetry.WorkflowSummary.Claims,
				SupportedClaims:      bundle.RunTelemetry.WorkflowSummary.SupportedClaims,
				RefutedClaims:        bundle.RunTelemetry.WorkflowSummary.RefutedClaims,
				InsufficientClaims:   bundle.RunTelemetry.WorkflowSummary.InsufficientClaims,
				UndeterminedClaims:   bundle.RunTelemetry.WorkflowSummary.UndeterminedClaims,
				KeepClaims:           bundle.RunTelemetry.WorkflowSummary.KeepClaims,
				KeepWithCaveatClaims: bundle.RunTelemetry.WorkflowSummary.KeepWithCaveatClaims,
				UnresolvedClaims:     bundle.RunTelemetry.WorkflowSummary.UnresolvedClaims,
				RejectClaims:         bundle.RunTelemetry.WorkflowSummary.RejectClaims,
				ChallengeCount:       bundle.RunTelemetry.WorkflowSummary.ChallengeCount,
				ObservationCount:     bundle.RunTelemetry.WorkflowSummary.ObservationCount,
				RoundCount:           max(bundle.RunTelemetry.WorkflowSummary.FreeDebateRoundCount, bundle.RunTelemetry.WorkflowSummary.DelphiRoundCount),
				ClaimCount:           bundle.RunTelemetry.WorkflowSummary.FreeDebateClaimCount,
				VoteCount:            bundle.RunTelemetry.WorkflowSummary.FreeDebateVoteCount,
				StatementCount:       bundle.RunTelemetry.WorkflowSummary.DelphiStatementCount,
			},
		}
		if bundle.RunTelemetry.VerificationSummary.Passed > 0 || bundle.RunTelemetry.VerificationSummary.Failed > 0 || bundle.RunTelemetry.VerificationSummary.Inconclusive > 0 {
			view.Run.Verify = &RunVerificationSummaryView{
				Passed:       bundle.RunTelemetry.VerificationSummary.Passed,
				Failed:       bundle.RunTelemetry.VerificationSummary.Failed,
				Inconclusive: bundle.RunTelemetry.VerificationSummary.Inconclusive,
			}
		}
		for _, item := range bundle.RunTelemetry.TaskSummary {
			view.Run.TaskSummary = append(view.Run.TaskSummary, RunTaskSummaryView{
				TaskKind:   string(item.TaskKind),
				Total:      item.Total,
				Strict:     item.Strict,
				Normalized: item.Normalized,
				Repaired:   item.Repaired,
				Failed:     item.Failed,
			})
		}
	}
	for _, item := range bundle.ComplianceSummary.Entries {
		view.Summary = append(view.Summary, ComplianceSummaryEntryView{
			Provider:      item.Provider,
			ProviderType:  item.ProviderType,
			ProviderModel: item.ProviderModel,
			TaskKind:      string(item.TaskKind),
			Total:         item.Total,
			Strict:        item.Strict,
			Normalized:    item.Normalized,
			Repaired:      item.Repaired,
			Failed:        item.Failed,
		})
	}
	for _, item := range bundle.ComplianceReports[:min(limit, len(bundle.ComplianceReports))] {
		view.Reports = append(view.Reports, ComplianceReportView{
			TaskID:               item.Task.TaskID,
			TaskKind:             item.Task.Kind,
			AgentID:              firstNonEmpty(item.Task.AgentID, item.Agent.ID),
			Provider:             item.Agent.Provider,
			ProviderType:         item.Agent.ProviderType,
			ProviderModel:        item.Agent.ProviderModel,
			StrictCompliant:      item.Compliance.StrictCompliant,
			NormalizedWithoutFix: item.Compliance.NormalizedWithoutFix,
			RepairAttempted:      item.Compliance.RepairAttempted,
			RepairSucceeded:      item.Compliance.RepairSucceeded,
			FinalStatus:          item.Compliance.FinalStatus,
			StrictError:          item.Compliance.StrictError,
			FinalError:           item.Compliance.FinalError,
			RawArtifact:          displayCompanionPath(bundle.Files.RunDir, artifactRefPath(item.Compliance.RawArtifact)),
			InitialErrorArtifact: displayCompanionPath(bundle.Files.RunDir, artifactRefPath(item.Compliance.InitialErrorArtifact)),
			FinalArtifact:        displayCompanionPath(bundle.Files.RunDir, artifactRefPath(item.Compliance.FinalArtifact)),
		})
	}
	if len(view.Summary) == 0 && len(view.Reports) == 0 {
		if view.Readiness == nil && view.Run == nil {
			return nil
		}
	}
	if len(view.Summary) == 0 && len(view.Reports) == 0 && view.Readiness == nil && view.Run == nil {
		return nil
	}
	return view
}
