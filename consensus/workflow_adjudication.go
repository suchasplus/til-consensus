package consensus

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

func (e *Engine) startAdjudication(ctx context.Context, request StartRequest) (_ *RunResult, err error) {
	run, err := e.beginWorkflow(ctx, request)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err == nil {
			return
		}
		_ = e.failSession(ctx, request, run.sessionID, run.state, run.claimGraph, run.challengeTickets, &run.ledgerCursor, run.startedAt, err)
	}()

	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhasePropose, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	if err := e.runProposalPhase(ctx, request, run); err != nil {
		return nil, err
	}
	SortClaims(run.claimGraph)
	if err := ValidateClaimDependencies(run.claimGraph); err != nil {
		return nil, err
	}
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhasePropose, claimIDs(run.claimGraph), 0, 0, 0, true, nil, nil, nil); err != nil {
		return nil, err
	}
	if len(run.claimGraph) == 0 {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseReport, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
		result := e.buildAdjudicationResult(request, run.sessionID, run.manifest, run.claimGraph, run.challengeTickets, run.verificationResults, run.revisionRecords, nil, ArbiterReport{
			TaskVerdict: TaskVerdictFailed,
			Summary:     "未产生任何可裁决 claim",
		}, AdjudicationReport{
			Summary:             "未产生任何可裁决 claim",
			UnresolvedQuestions: append([]string(nil), run.manifest.UnresolvedQuestions...),
		}, nil, TerminalStateInsufficientEvidence, run.observations, run.metrics, run.startedAt, nil)
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindReportGenerated,
			Source:       EvidenceSourceReporter,
			ProducerID:   request.Roles.Reporter,
			ProducerRole: "reporter",
			Summary:      result.Report.Summary,
		}); err != nil {
			return nil, err
		}
		return e.finalizeWorkflow(ctx, request, run, result)
	}
	activeClaimIDs := claimIDs(run.claimGraph)
	var (
		arbiterReport     ArbiterReport
		terminalState     WorkflowTerminalState
		fallbacksUsed     int
		lastRevisionRound int
		lastVerifyRounds  int
	)
adjudicationCycle:
	for {
		verifyRounds := 0
		for revisionRound := 0; ; revisionRound++ {
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseChallenge, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
			if err := e.runChallengePhase(ctx, request, run, activeClaimIDs, revisionRound); err != nil {
				return nil, err
			}
			SortChallenges(run.challengeTickets)
			if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseChallenge, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, true, nil, nil, nil); err != nil {
				return nil, err
			}

			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseVerify, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
			if err := e.runVerifyPhase(ctx, request, run, activeClaimIDs); err != nil {
				return nil, err
			}
			verifyRounds++
			if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseVerify, activeClaimIDs, revisionRound, verifyRounds, fallbacksUsed, true, nil, nil, nil); err != nil {
				return nil, err
			}
			lastRevisionRound = revisionRound
			lastVerifyRounds = verifyRounds
			if verifyRounds >= request.LoopPolicy.MaxVerificationRounds {
				break
			}

			if revisionRound >= request.LoopPolicy.MaxRevisionRounds {
				break
			}
			revisionCandidates := selectClaimsForRevision(run.claimGraph, run.challengeTickets, run.verificationResults, activeClaimIDs)
			if len(revisionCandidates) == 0 {
				break
			}
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseRevise, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
			revisedClaimIDs, materialChange, err := e.runRevisionPhase(ctx, request, run, revisionRound+1, revisionCandidates)
			if err != nil {
				return nil, err
			}
			if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseRevise, revisedClaimIDs, revisionRound+1, verifyRounds, fallbacksUsed, materialChange, nil, nil, nil); err != nil {
				return nil, err
			}
			lastRevisionRound = revisionRound + 1
			lastVerifyRounds = verifyRounds
			if len(revisedClaimIDs) == 0 || !materialChange {
				break
			}
			activeClaimIDs = revisedClaimIDs
		}

		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseAdjudicate, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return nil, err
		}
		arbiterInput := ArbiterInput{
			Request:    request,
			SessionID:  run.sessionID,
			Claims:     activeAdjudicationClaims(run.claimGraph),
			Challenges: run.challengeTickets,
			Ledger:     run.ledgerEntries,
			Findings:   run.verificationResults,
		}
		arbiterReport, err = e.decideArbiter(ctx, request, run.startedAt, arbiterInput, &run.metrics)
		if err != nil {
			return nil, err
		}
		run.adjudicationRecords = append([]AdjudicationRecord(nil), arbiterReport.Records...)
		for idx, decision := range arbiterReport.Decisions {
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				ClaimID:      decision.ClaimID,
				Kind:         EvidenceKindArbiterDecision,
				Source:       EvidenceSourceArbiter,
				ProducerID:   request.Roles.Arbiter,
				ProducerRole: "arbiter",
				Summary:      decision.Rationale,
				Metadata: mergeAnyMaps(maps.Clone(decision.Metadata), map[string]any{
					"verdict":     decision.Verdict,
					"confidence":  decision.Confidence,
					"disposition": firstRecordDisposition(run.adjudicationRecords, decision.ClaimID),
				}),
			})
			if err != nil {
				return nil, err
			}
			arbiterReport.Decisions[idx].EvidenceRefs = appendUnique(decision.EvidenceRefs, entry.EntryID)
		}
		if len(arbiterReport.Records) > 0 {
			for idx := range arbiterReport.Records {
				arbiterReport.Records[idx].EvidenceRefs = appendUnique(arbiterReport.Records[idx].EvidenceRefs, matchingDecisionEvidence(arbiterReport.Decisions, arbiterReport.Records[idx].TargetClaimID)...)
			}
		}
		for _, record := range arbiterReport.Records {
			decision := findArbiterDecision(arbiterReport.Decisions, record.TargetClaimID)
			if err := e.emit(ctx, request, run.sessionID, RunEventClaimAdjudicated, SessionPhaseAdjudicate, map[string]any{
				"claimId":           record.TargetClaimID,
				"disposition":       record.Disposition,
				"finalConfidence":   record.FinalConfidence,
				"actionability":     record.Actionability,
				"blockingRiskCount": len(record.BlockingRisks),
				"verdict":           decision.Verdict,
				"confidence":        decision.Confidence,
				"reason":            firstNonEmpty(record.Rationale, decision.Rationale),
				"metadata":          mergeAnyMaps(maps.Clone(record.Metadata), maps.Clone(decision.Metadata)),
			}); err != nil {
				return nil, err
			}
		}
		run.claimGraph = ApplyDecisions(run.claimGraph, arbiterReport.Decisions)
		run.claimGraph = ApplyAdjudicationRecords(run.claimGraph, arbiterReport.Records)
		if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseAdjudicate, activeClaimIDs, lastRevisionRound, lastVerifyRounds, fallbacksUsed, true, &arbiterReport, nil, nil); err != nil {
			return nil, err
		}
		terminalState = DetermineTerminalState(run.claimGraph, run.challengeTickets, run.manifest, nil)

		target, fallbackClaimIDs, fallbackReason := decideAdjudicationFallback(request, run.claimGraph, run.challengeTickets, arbiterReport, terminalState, fallbacksUsed)
		if target == FallbackTargetStop {
			break adjudicationCycle
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindAdjudicationFallback,
			Source:       EvidenceSourceCoordinator,
			ProducerRole: "coordinator",
			Summary:      fallbackReason,
			Metadata: map[string]any{
				"target":        target,
				"terminalState": terminalState,
				"claims":        fallbackClaimIDs,
				"attempt":       fallbacksUsed + 1,
			},
		}); err != nil {
			return nil, err
		}
		fallbacksUsed++
		switch target {
		case FallbackTargetIngest:
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseIngest, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
			newEvidence, err := e.runIngestSources(ctx, request, run, fmt.Sprintf("fallback-%d", fallbacksUsed))
			if err != nil {
				return nil, err
			}
			nextClaims := fallbackClaimIDs
			if len(nextClaims) == 0 {
				nextClaims = claimIDs(run.claimGraph)
			}
			if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseIngest, nextClaims, lastRevisionRound, 0, fallbacksUsed, newEvidence, &arbiterReport, nil, nil); err != nil {
				return nil, err
			}
			if !newEvidence {
				break adjudicationCycle
			}
			if len(fallbackClaimIDs) > 0 {
				activeClaimIDs = fallbackClaimIDs
			} else {
				activeClaimIDs = claimIDs(run.claimGraph)
			}
		case FallbackTargetRevise:
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseRevise, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
			revisedClaimIDs, materialChange, err := e.runRevisionPhase(ctx, request, run, request.LoopPolicy.MaxRevisionRounds+fallbacksUsed, fallbackClaimIDs)
			if err != nil {
				return nil, err
			}
			if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseRevise, revisedClaimIDs, request.LoopPolicy.MaxRevisionRounds+fallbacksUsed, 0, fallbacksUsed, materialChange, &arbiterReport, nil, nil); err != nil {
				return nil, err
			}
			if !materialChange || len(revisedClaimIDs) == 0 {
				break adjudicationCycle
			}
			activeClaimIDs = revisedClaimIDs
		default:
			break adjudicationCycle
		}
	}

	if current := run.state.Current(); current != SessionPhaseAdjudicate {
		switch current {
		case SessionPhaseIngest, SessionPhaseRevise:
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseAdjudicate, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
				return nil, err
			}
		}
	}
	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseReport, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
		return nil, err
	}
	report, reportArtifact, err := e.composeReport(ctx, request, run.sessionID, run.startedAt, arbiterReport, run.claimGraph, run.challengeTickets, &run.metrics)
	if err != nil {
		return nil, err
	}
	if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindReportGenerated,
		Source:       EvidenceSourceReporter,
		ProducerID:   request.Roles.Reporter,
		ProducerRole: "reporter",
		Summary:      report.Summary,
		Artifact:     reportArtifact,
	}); err != nil {
		return nil, err
	}
	if err := e.saveAdjudicationCheckpoint(ctx, run, SessionPhaseReport, claimIDs(run.claimGraph), 0, 0, fallbacksUsed, true, &arbiterReport, &report, nil); err != nil {
		return nil, err
	}

	result := e.buildAdjudicationResult(request, run.sessionID, run.manifest, run.claimGraph, run.challengeTickets, run.verificationResults, run.revisionRecords, run.adjudicationRecords, arbiterReport, report, nil, terminalState, nil, run.metrics, run.startedAt, nil)
	return e.finalizeWorkflow(ctx, request, run, result)
}

func (e *Engine) runProposalPhase(ctx context.Context, request StartRequest, run *workflowRun) error {
proposalPhase:
	for pass := 0; pass < request.ProposalPolicy.MaxPasses; pass++ {
		if e.isGlobalDeadlineHit(request, run.startedAt) {
			run.metrics.GlobalDeadlineHit = true
			break proposalPhase
		}
		tasks := make([]Task, 0, len(request.Roles.Proposers))
		for _, proposerID := range request.Roles.Proposers {
			tasks = append(tasks, ProposalTask{
				TaskMeta: TaskMeta{
					SessionID: run.sessionID,
					RequestID: request.RequestID,
					AgentID:   proposerID,
					Role:      "proposer",
					Metadata:  map[string]any{"pass": pass, "blind": true},
				},
				TaskSpec:       request.TaskSpec,
				Scope:          fmt.Sprintf("proposal pass %d", pass+1),
				MaxClaims:      request.ProposalPolicy.MaxClaimsPerWorker,
				DedupeStrategy: request.ProposalPolicy.DedupeStrategy,
			})
		}
		for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
			recordTaskBatchResultMetrics(&run.metrics, result)
			if result.Err != nil {
				run.addDegradation(participantAbsentDegradation(result, "proposal", pass+1, "该 proposer 的提案缺席，claim 图基于其余 proposer 的输出"))
				continue
			}
			proposerID := result.Task.Meta().AgentID
			output, ok := result.Awaited.Output.(ProposalTaskResult)
			if !ok {
				return fmt.Errorf("proposal task returned unexpected result type")
			}
			workerEntry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:                  EvidenceKindWorkerOutput,
				Source:                EvidenceSourceWorker,
				ProducerID:            proposerID,
				ProducerRole:          "proposer",
				SourceType:            "proposal",
				Summary:               output.Output.Summary,
				Artifact:              result.Awaited.Artifact,
				ProvenanceQuality:     ProvenanceQualityMedium,
				FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
				Metadata: map[string]any{
					"taskId":   result.Receipt.TaskID,
					"taskKind": TaskKindPropose,
					"pass":     pass,
					"blind":    true,
				},
			})
			if err != nil {
				return err
			}
			for _, draft := range output.Output.Claims {
				if strings.TrimSpace(draft.Statement) == "" {
					continue
				}
				if draft.ClaimType == "" {
					draft.ClaimType = inferClaimType(draft.Statement)
				}
				var created bool
				run.claimGraph, _, created = UpsertClaim(run.claimGraph, draft, proposerID, workerEntry.EntryID, e.ids)
				if !created {
					continue
				}
				run.metrics.ClaimsProposed++
				claim := run.claimGraph[len(run.claimGraph)-1]
				entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
					ClaimID:               claim.ClaimID,
					Kind:                  EvidenceKindClaimProposed,
					Source:                EvidenceSourceCoordinator,
					ProducerID:            proposerID,
					ProducerRole:          "proposer",
					SourceType:            "claim",
					Summary:               claim.Statement,
					ProvenanceQuality:     ProvenanceQualityMedium,
					FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
					Metadata: map[string]any{
						"title":                 claim.Title,
						"claimType":             claim.ClaimType,
						"scope":                 claim.Scope,
						"dependencies":          claim.Dependencies,
						"parentClaimIds":        claim.ParentClaimIDs,
						"boundaryConditions":    claim.BoundaryConditions,
						"applicability":         claim.Applicability,
						"sourceProposalAgentId": proposerID,
					},
				})
				if err != nil {
					return err
				}
				run.claimGraph = AttachEvidenceToClaim(run.claimGraph, claim.ClaimID, entry.EntryID)
			}
		}
		if run.metrics.GlobalDeadlineHit {
			break proposalPhase
		}
	}
	return nil
}

func (e *Engine) runChallengePhase(ctx context.Context, request StartRequest, run *workflowRun, claimIDs []string, round int) error {
	claims := filterClaimsByID(run.claimGraph, claimIDs)
	tasks := make([]Task, 0, len(request.Roles.Challengers))
	for _, challengerID := range request.Roles.Challengers {
		if e.isGlobalDeadlineHit(request, run.startedAt) {
			run.metrics.GlobalDeadlineHit = true
			return nil
		}
		tasks = append(tasks, ChallengeTask{
			TaskMeta: TaskMeta{
				SessionID: run.sessionID,
				RequestID: request.RequestID,
				AgentID:   challengerID,
				Role:      "challenger",
				Metadata:  map[string]any{"round": round},
			},
			TaskSpec: request.TaskSpec,
			Claims:   claims,
		})
	}
	for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "challenge", round, "该 challenger 的质询缺席，claim 未经过其攻击检验"))
			continue
		}
		challengerID := result.Task.Meta().AgentID
		output, ok := result.Awaited.Output.(ChallengeTaskResult)
		if !ok {
			return fmt.Errorf("challenge task returned unexpected result type")
		}
		workerEntry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:                  EvidenceKindWorkerOutput,
			Source:                EvidenceSourceWorker,
			ProducerID:            challengerID,
			ProducerRole:          "challenger",
			SourceType:            "attack",
			Summary:               output.Output.Summary,
			Artifact:              result.Awaited.Artifact,
			ProvenanceQuality:     ProvenanceQualityMedium,
			FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
			Metadata: map[string]any{
				"taskId":   result.Receipt.TaskID,
				"taskKind": TaskKindChallenge,
				"round":    round,
			},
		})
		if err != nil {
			return err
		}
		for _, draft := range output.Output.Tickets {
			claim, ok := ResolveClaimRef(run.claimGraph, draft.ClaimID, draft.Statement)
			if !ok {
				continue
			}
			var created bool
			run.challengeTickets, _, created = UpsertChallenge(run.challengeTickets, draft, claim.ClaimID, challengerID, workerEntry.EntryID, e.ids)
			if !created {
				continue
			}
			run.metrics.ChallengesOpened++
			ticket := run.challengeTickets[len(run.challengeTickets)-1]
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				ClaimID:               claim.ClaimID,
				ChallengeID:           ticket.TicketID,
				Kind:                  EvidenceKindChallengeOpened,
				Source:                EvidenceSourceCoordinator,
				ProducerID:            challengerID,
				ProducerRole:          "challenger",
				SourceType:            "attack_record",
				Summary:               ticket.Statement,
				ProvenanceQuality:     ProvenanceQualityMedium,
				FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
				Metadata: map[string]any{
					"kind":                         ticket.Kind,
					"attackType":                   firstNonEmpty(ticket.AttackType, ticket.Kind),
					"severity":                     ticket.Severity,
					"requestedChecks":              ticket.RequestedChecks,
					"suggestedFalsificationMethod": ticket.SuggestedFalsificationMethod,
					"round":                        round,
				},
			})
			if err != nil {
				return err
			}
			run.challengeTickets[len(run.challengeTickets)-1].EvidenceRefs = appendUnique(run.challengeTickets[len(run.challengeTickets)-1].EvidenceRefs, entry.EntryID)
			run.claimGraph = AttachChallengeToClaim(run.claimGraph, claim.ClaimID, ticket.TicketID)
		}
	}
	return nil
}

func (e *Engine) runVerifyPhase(ctx context.Context, request StartRequest, run *workflowRun, claimIDs []string) error {
	verifyCtx, cancel, deadlineErr := e.withGlobalDeadline(ctx, request, run.startedAt)
	if deadlineErr != nil {
		run.metrics.GlobalDeadlineHit = true
		return nil
	}
	defer cancel()
	verifier := e.deps.Verifier
	if verifier == nil {
		verifier = NewCompositeVerifier(CompositeVerifierDeps{
			TaskDelegate:   e.deps.TaskDelegate,
			Clock:          e.clock,
			IDFactory:      newLockedIDFactory(e.ids),
			ArtifactDir:    e.deps.ArtifactDir,
			PerTaskTimeout: e.effectivePerTaskTimeout(request, run.startedAt, request.WaitingPolicy.PerTaskTimeout),
			RetryAttempts:  request.WaitingPolicy.RetryAttempts,
		})
	}
	claims := make([]ClaimNode, 0)
	for _, claim := range filterClaimsByID(run.claimGraph, claimIDs) {
		if claim.Status == ClaimStatusWithdrawn {
			continue
		}
		claims = append(claims, claim)
	}
	for _, verifyResult := range runVerificationBatch(verifyCtx, verifier, request, run.sessionID, claims, run.challengeTickets, request.VerificationPolicy.MaxParallelChecks) {
		claim := verifyResult.Claim
		findings := verifyResult.Findings
		verifyErr := verifyResult.Err
		if verifyErr != nil {
			if errors.Is(verifyErr, context.DeadlineExceeded) || errors.Is(verifyErr, context.Canceled) {
				run.metrics.GlobalDeadlineHit = true
				return nil
			}
			findings = []VerificationResult{{
				VerificationID: e.ids.NewEntityID("verify"),
				ClaimID:        claim.ClaimID,
				CheckName:      "verifier",
				Kind:           "verifier_error",
				Method:         "verifier",
				Status:         VerificationStatusInconclusive,
				Result:         VerificationStatusInconclusive,
				Summary:        verifyErr.Error(),
			}}
		}
		claimFindings := make([]VerificationResult, 0, len(findings))
		for _, finding := range findings {
			kind := EvidenceKindDeterministicCheck
			if finding.Kind == "semantic" {
				kind = EvidenceKindSemanticVerification
			}
			finding.Method = firstNonEmpty(finding.Method, finding.CheckName, finding.Kind)
			finding.Result = finding.Status
			if finding.Artifact != nil {
				finding.RawOutputReference = finding.Artifact.Path
			}
			metadata := map[string]any{
				"checkName":         finding.CheckName,
				"kind":              finding.Kind,
				"method":            finding.Method,
				"status":            finding.Status,
				"failureCode":       finding.FailureCode,
				"verdictSuggestion": finding.VerdictSuggestion,
				"confidence":        finding.Confidence,
				"confidenceDelta":   finding.ConfidenceDelta,
			}
			for key, value := range finding.Metadata {
				metadata[key] = value
			}
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				ClaimID:               claim.ClaimID,
				ChallengeID:           finding.ChallengeID,
				VerificationID:        finding.VerificationID,
				Kind:                  kind,
				Source:                EvidenceSourceVerifier,
				ProducerRole:          "verifier",
				SourceType:            finding.Method,
				Summary:               finding.Summary,
				Artifact:              finding.Artifact,
				SourceLocator:         finding.RawOutputReference,
				ProvenanceQuality:     evidenceQualityFromFinding(finding),
				FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
				Metadata:              metadata,
			})
			if err != nil {
				return err
			}
			finding.EvidenceRef = entry.EntryID
			run.verificationResults = append(run.verificationResults, finding)
			claimFindings = append(claimFindings, finding)
			run.metrics.VerificationsRun++
		}
		for _, finding := range claimFindings {
			run.claimGraph = AttachVerificationOutcome(run.claimGraph, claim.ClaimID, finding)
		}
		run.challengeTickets = CloseChallenges(run.challengeTickets, claim.ClaimID, claimFindings, "verification completed")
	}
	return nil
}

func selectClaimsForRevision(claims []ClaimNode, tickets []ChallengeTicket, findings []VerificationResult, activeClaimIDs []string) []string {
	active := make(map[string]struct{}, len(activeClaimIDs))
	for _, id := range activeClaimIDs {
		active[id] = struct{}{}
	}
	out := make([]string, 0)
	for _, claim := range claims {
		if len(active) > 0 {
			if _, ok := active[claim.ClaimID]; !ok {
				continue
			}
		}
		if claim.Status == ClaimStatusWithdrawn {
			continue
		}
		for _, ticket := range tickets {
			if ticket.ClaimID == claim.ClaimID && ticket.Status == ChallengeStatusOpen {
				out = appendUnique(out, claim.ClaimID)
				break
			}
		}
		for _, finding := range findings {
			if finding.ClaimID != claim.ClaimID {
				continue
			}
			if finding.Status != VerificationStatusPassed {
				out = appendUnique(out, claim.ClaimID)
			}
		}
	}
	return out
}

func (e *Engine) runRevisionPhase(ctx context.Context, request StartRequest, run *workflowRun, round int, targetClaimIDs []string) ([]string, bool, error) {
	revisionsByProposer := map[string][]ClaimNode{}
	for _, claim := range filterClaimsByID(run.claimGraph, targetClaimIDs) {
		proposerID := firstNonEmpty(claim.SourceProposalAgentID)
		if proposerID == "" && len(claim.ProposedBy) > 0 {
			proposerID = claim.ProposedBy[0]
		}
		if proposerID == "" && len(request.Roles.Proposers) > 0 {
			proposerID = request.Roles.Proposers[0]
		}
		revisionsByProposer[proposerID] = append(revisionsByProposer[proposerID], claim)
	}
	applied := make([]ClaimRevisionRecord, 0)
	proposerIDs := make([]string, 0, len(revisionsByProposer))
	for proposerID := range revisionsByProposer {
		proposerIDs = append(proposerIDs, proposerID)
	}
	slices.Sort(proposerIDs)
	type revisionTaskItem struct {
		proposerID string
		builtin    []ClaimRevisionRecord
	}
	taskItems := make([]revisionTaskItem, 0, len(proposerIDs))
	tasks := make([]Task, 0, len(proposerIDs))
	for _, proposerID := range proposerIDs {
		claims := revisionsByProposer[proposerID]
		groupClaimIDs := claimIDs(claims)
		findings := selectFindingsForClaims(run.verificationResults, groupClaimIDs)
		challenges := selectChallengesForClaims(run.challengeTickets, groupClaimIDs)
		builtin := builtinRevisionRecords(claims, challenges, findings, round, proposerID, e.ids)
		if proposerID == "" {
			applied = append(applied, builtin...)
			continue
		}
		taskItems = append(taskItems, revisionTaskItem{proposerID: proposerID, builtin: builtin})
		tasks = append(tasks, ReviseTask{
			TaskMeta: TaskMeta{
				SessionID: run.sessionID,
				RequestID: request.RequestID,
				AgentID:   proposerID,
				Role:      "proposer",
				Metadata:  map[string]any{"round": round},
			},
			TaskSpec:   request.TaskSpec,
			Manifest:   run.manifest,
			Round:      round,
			Claims:     claims,
			Challenges: challenges,
			Findings:   findings,
		})
	}
	for idx, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		item := taskItems[idx]
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "revision", round, "该 proposer 的修订缺席，改用内置降级修订"))
			applied = append(applied, item.builtin...)
			continue
		}
		output, ok := result.Awaited.Output.(ReviseTaskResult)
		if !ok {
			applied = append(applied, item.builtin...)
			continue
		}
		entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:                  EvidenceKindWorkerOutput,
			Source:                EvidenceSourceWorker,
			ProducerID:            item.proposerID,
			ProducerRole:          "proposer",
			SourceType:            "revision",
			Summary:               output.Output.Summary,
			Artifact:              result.Awaited.Artifact,
			ProvenanceQuality:     ProvenanceQualityMedium,
			FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
			Metadata:              map[string]any{"taskId": result.Receipt.TaskID, "taskKind": TaskKindRevise, "round": round},
		})
		if err != nil {
			return nil, false, err
		}
		if len(output.Output.Revisions) == 0 {
			applied = append(applied, item.builtin...)
			continue
		}
		for _, draft := range output.Output.Revisions {
			applied = append(applied, ClaimRevisionRecord{
				RevisionID:         e.ids.NewEntityID("revision"),
				TargetClaimID:      draft.TargetClaimID,
				ProposerID:         item.proposerID,
				Action:             draft.Action,
				RevisedText:        draft.RevisedText,
				ConfidenceDelta:    draft.ConfidenceDelta,
				Caveats:            dedupeStrings(draft.Caveats),
				BoundaryConditions: dedupeStrings(draft.BoundaryConditions),
				Unresolved:         draft.Unresolved,
				Reason:             draft.Reason,
				EvidenceRefs:       filterEmpty([]string{entry.EntryID}),
				Round:              round,
				Metadata:           maps.Clone(draft.Metadata),
			})
		}
	}
	run.revisionRecords = append(run.revisionRecords, applied...)
	for _, record := range applied {
		entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			ClaimID:               record.TargetClaimID,
			Kind:                  EvidenceKindClaimRevised,
			Source:                EvidenceSourceCoordinator,
			ProducerID:            record.ProposerID,
			ProducerRole:          "proposer",
			SourceType:            "revision_record",
			Summary:               firstNonEmpty(record.Reason, string(record.Action)),
			ProvenanceQuality:     ProvenanceQualityMedium,
			FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
			Metadata: map[string]any{
				"action":             record.Action,
				"confidenceDelta":    record.ConfidenceDelta,
				"caveats":            record.Caveats,
				"boundaryConditions": record.BoundaryConditions,
				"round":              record.Round,
				"metadata":           maps.Clone(record.Metadata),
			},
		})
		if err != nil {
			return nil, false, err
		}
		record.EvidenceRefs = appendUnique(record.EvidenceRefs, entry.EntryID)
		if err := e.emit(ctx, request, run.sessionID, RunEventClaimRevised, SessionPhaseRevise, map[string]any{
			"claimId":         record.TargetClaimID,
			"proposerId":      record.ProposerID,
			"action":          record.Action,
			"confidenceDelta": record.ConfidenceDelta,
			"round":           record.Round,
			"unresolved":      record.Unresolved,
			"reason":          record.Reason,
			"caveatCount":     len(record.Caveats),
			"boundaryCount":   len(record.BoundaryConditions),
			"revisedText":     record.RevisedText,
			"metadata":        maps.Clone(record.Metadata),
		}); err != nil {
			return nil, false, err
		}
	}
	var materialChange bool
	var revisedClaimIDs []string
	run.claimGraph, revisedClaimIDs, materialChange = ApplyRevisionRecords(run.claimGraph, applied, request.LoopPolicy.MaterialConfidenceDelta)
	return revisedClaimIDs, materialChange, nil
}

func builtinRevisionRecords(claims []ClaimNode, challenges []ChallengeTicket, findings []VerificationResult, round int, proposerID string, ids IDFactory) []ClaimRevisionRecord {
	out := make([]ClaimRevisionRecord, 0, len(claims))
	for _, claim := range claims {
		action := RevisionActionUnchanged
		reason := "未检测到材料性问题"
		confidenceDelta := 0.0
		caveats := make([]string, 0)
		for _, finding := range findings {
			if finding.ClaimID != claim.ClaimID {
				continue
			}
			switch finding.Status {
			case VerificationStatusFailed:
				action = RevisionActionWithdraw
				reason = firstNonEmpty(finding.Summary, "验证失败，暂时撤回 claim")
				confidenceDelta = -0.5
			case VerificationStatusInconclusive:
				if action != RevisionActionWithdraw {
					action = RevisionActionUnresolved
					reason = firstNonEmpty(finding.Summary, "验证结论不足，标记 unresolved")
					confidenceDelta = -0.2
				}
			}
		}
		for _, challenge := range challenges {
			if challenge.ClaimID != claim.ClaimID || challenge.Status != ChallengeStatusOpen {
				continue
			}
			if action == RevisionActionUnchanged {
				action = RevisionActionDowngrade
				reason = firstNonEmpty(challenge.AttackText, challenge.Statement, "存在未解决 challenge，降低信心")
				confidenceDelta = -0.15
			}
			caveats = append(caveats, firstNonEmpty(challenge.AttackText, challenge.Statement))
		}
		out = append(out, ClaimRevisionRecord{
			RevisionID:      ids.NewEntityID("revision"),
			TargetClaimID:   claim.ClaimID,
			ProposerID:      proposerID,
			Action:          action,
			ConfidenceDelta: confidenceDelta,
			Caveats:         dedupeStrings(caveats),
			Unresolved:      action == RevisionActionUnresolved,
			Reason:          reason,
			Round:           round,
		})
	}
	return out
}

func activeAdjudicationClaims(claims []ClaimNode) []ClaimNode {
	out := make([]ClaimNode, 0, len(claims))
	for _, claim := range claims {
		if claim.Status == ClaimStatusWithdrawn {
			continue
		}
		out = append(out, claim)
	}
	return out
}

func claimIDs(claims []ClaimNode) []string {
	out := make([]string, 0, len(claims))
	for _, claim := range claims {
		out = append(out, claim.ClaimID)
	}
	return out
}

func filterClaimsByID(claims []ClaimNode, ids []string) []ClaimNode {
	if len(ids) == 0 {
		return append([]ClaimNode(nil), claims...)
	}
	index := map[string]struct{}{}
	for _, id := range ids {
		index[id] = struct{}{}
	}
	out := make([]ClaimNode, 0, len(claims))
	for _, claim := range claims {
		if _, ok := index[claim.ClaimID]; ok {
			out = append(out, claim)
		}
	}
	return out
}

func selectFindingsForClaims(findings []VerificationResult, claimIDs []string) []VerificationResult {
	if len(claimIDs) == 0 {
		return append([]VerificationResult(nil), findings...)
	}
	index := map[string]struct{}{}
	for _, id := range claimIDs {
		index[id] = struct{}{}
	}
	out := make([]VerificationResult, 0)
	for _, finding := range findings {
		if _, ok := index[finding.ClaimID]; ok {
			out = append(out, finding)
		}
	}
	return out
}

func selectChallengesForClaims(tickets []ChallengeTicket, claimIDs []string) []ChallengeTicket {
	if len(claimIDs) == 0 {
		return append([]ChallengeTicket(nil), tickets...)
	}
	index := map[string]struct{}{}
	for _, id := range claimIDs {
		index[id] = struct{}{}
	}
	out := make([]ChallengeTicket, 0)
	for _, ticket := range tickets {
		if _, ok := index[ticket.ClaimID]; ok {
			out = append(out, ticket)
		}
	}
	return out
}

func inferClaimType(statement string) ClaimType {
	text := strings.ToLower(strings.TrimSpace(statement))
	switch {
	case strings.Contains(text, "should") || strings.Contains(text, "建议") || strings.Contains(text, "推荐"):
		return ClaimTypeRecommendation
	case strings.Contains(text, "because") || strings.Contains(text, "因此") || strings.Contains(text, "所以"):
		return ClaimTypeInference
	case strings.Contains(text, "assume") || strings.Contains(text, "假设"):
		return ClaimTypeAssumption
	default:
		return ClaimTypeFact
	}
}

func evidenceQualityFromFinding(finding VerificationResult) ProvenanceQuality {
	switch finding.Status {
	case VerificationStatusPassed:
		return ProvenanceQualityHigh
	case VerificationStatusFailed:
		return ProvenanceQualityHigh
	default:
		return ProvenanceQualityMedium
	}
}

func firstRecordDisposition(records []AdjudicationRecord, claimID string) ClaimDisposition {
	for _, record := range records {
		if record.TargetClaimID == claimID {
			return record.Disposition
		}
	}
	return ""
}

func matchingDecisionEvidence(decisions []ArbiterDecision, claimID string) []string {
	for _, decision := range decisions {
		if decision.ClaimID == claimID {
			return decision.EvidenceRefs
		}
	}
	return nil
}

func findArbiterDecision(decisions []ArbiterDecision, claimID string) ArbiterDecision {
	for _, decision := range decisions {
		if decision.ClaimID == claimID {
			return decision
		}
	}
	return ArbiterDecision{}
}

func (e *Engine) runIngestSources(ctx context.Context, request StartRequest, run *workflowRun, reason string) (bool, error) {
	if len(request.IngestPolicy.Sources) == 0 {
		return false, nil
	}
	newEvidence := false
	for _, source := range request.IngestPolicy.Sources {
		sourceResult, err := runExternalCommandSource(ctx, e.deps, e.clock, e.ids, request, run.sessionID, source, "ingest", map[string]string{
			"TIL_CONSENSUS_CASE_ID": run.manifest.CaseID,
			"TIL_CONSENSUS_REASON":  reason,
		})
		if err != nil {
			return newEvidence, err
		}
		if sourceResult.Artifact != nil || strings.TrimSpace(sourceResult.Excerpt) != "" || sourceResult.MatchedOK || sourceResult.Contradicted || sourceResult.ExecFailed {
			newEvidence = true
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:                  EvidenceKindSourceMaterial,
			Source:                EvidenceSourceCoordinator,
			ProducerRole:          "coordinator",
			SourceType:            firstNonEmpty(source.SourceType, "external_ingest"),
			SourceLocator:         firstNonEmpty(source.Reference, source.Command),
			Summary:               sourceResult.Summary,
			ContentExcerpt:        sourceResult.Excerpt,
			Artifact:              sourceResult.Artifact,
			ProvenanceQuality:     ProvenanceQualityHigh,
			FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
			ObservedAt:            e.clock.Now().Format(time.RFC3339Nano),
			Notes:                 append([]string(nil), sourceResult.Notes...),
			Metadata: mergeAnyMaps(sourceResult.Metadata, map[string]any{
				"reason":       reason,
				"collector":    "external_command",
				"sourceName":   source.Name,
				"matchedOK":    sourceResult.MatchedOK,
				"contradicted": sourceResult.Contradicted,
				"execFailed":   sourceResult.ExecFailed,
				"failureClass": sourceResult.FailureClass,
				"command":      source.Command,
				"args":         append([]string(nil), source.Args...),
			}),
		}); err != nil {
			return newEvidence, err
		}
	}
	return newEvidence, nil
}

func decideAdjudicationFallback(request StartRequest, claims []ClaimNode, tickets []ChallengeTicket, arbiter ArbiterReport, terminalState WorkflowTerminalState, fallbacksUsed int) (FallbackTarget, []string, string) {
	if fallbacksUsed >= request.FallbackPolicy.MaxFallbackRounds {
		return FallbackTargetStop, nil, ""
	}
	records := arbiter.Records
	if len(records) == 0 {
		records = deriveRecordsFromDecisions(request, activeAdjudicationClaims(claims), arbiter.Decisions)
	}
	unresolvedClaimIDs := claimIDsByDisposition(records, ClaimDispositionUnresolved)
	caveatedClaimIDs := claimIDsByDisposition(records, ClaimDispositionKeepWithCaveat)
	openChallengeClaimIDs := claimIDsForOpenChallenges(tickets)
	switch terminalState {
	case TerminalStateInsufficientEvidence:
		return request.FallbackPolicy.OnInsufficientEvidence, fallbackClaimIDs(unresolvedClaimIDs, openChallengeClaimIDs, claimIDs(claims)), "裁决结果显示证据不足，自动回退到补充证据/修订流程"
	case TerminalStateUnresolvedConflict:
		return request.FallbackPolicy.OnUnresolvedConflict, fallbackClaimIDs(openChallengeClaimIDs, unresolvedClaimIDs, claimIDs(claims)), "裁决结果存在未解决冲突，自动回退到补充证据/修订流程"
	}
	if len(unresolvedClaimIDs) > 0 {
		return request.FallbackPolicy.OnUnresolvedClaims, fallbackClaimIDs(unresolvedClaimIDs, openChallengeClaimIDs, claimIDs(claims)), "存在 unresolved claim，自动回退继续修订或补充证据"
	}
	if len(caveatedClaimIDs) > 0 {
		return request.FallbackPolicy.OnKeepWithCaveat, fallbackClaimIDs(caveatedClaimIDs, openChallengeClaimIDs, claimIDs(claims)), "存在 keep_with_caveat claim，自动回退继续修订或补充证据"
	}
	return FallbackTargetStop, nil, ""
}

func claimIDsByDisposition(records []AdjudicationRecord, dispositions ...ClaimDisposition) []string {
	allowed := make(map[ClaimDisposition]struct{}, len(dispositions))
	for _, disposition := range dispositions {
		allowed[disposition] = struct{}{}
	}
	out := make([]string, 0)
	for _, record := range records {
		if _, ok := allowed[record.Disposition]; ok {
			out = appendUnique(out, record.TargetClaimID)
		}
	}
	return out
}

func claimIDsForOpenChallenges(tickets []ChallengeTicket) []string {
	out := make([]string, 0)
	for _, ticket := range tickets {
		if ticket.Status == ChallengeStatusOpen {
			out = appendUnique(out, ticket.ClaimID)
		}
	}
	return out
}

func fallbackClaimIDs(groups ...[]string) []string {
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		return dedupeStrings(group)
	}
	return nil
}

func mergeAnyMaps(base map[string]any, overlay map[string]any) map[string]any {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := cloneAnyMap(base)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range overlay {
		out[key] = value
	}
	return out
}
