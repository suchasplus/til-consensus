package consensus

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

func (e *Engine) startFreeDebate(ctx context.Context, request StartRequest) (_ *RunResult, err error) {
	run, err := e.beginWorkflow(ctx, request)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err == nil {
			return
		}
		_ = e.patchRunState(context.Background(), run, SessionPatch{})
		_ = e.failSession(ctx, request, run.sessionID, run.state, nil, nil, &run.ledgerCursor, run.startedAt, err)
	}()

	claims := make([]DebateClaim, 0)
	rounds := make([]DebateRoundRecord, 0, request.DebatePolicy.MaxRounds+2)
	votes := make([]DebateVoteRecord, 0)

	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseInitial, nil, nil, run.ledgerCursor); err != nil {
		return nil, err
	}
	if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindDebateRoundOpened,
		Source:       EvidenceSourceCoordinator,
		ProducerRole: "coordinator",
		Summary:      "initial round opened",
		Metadata: map[string]any{
			"round": 0,
			"phase": "initial",
		},
	}); err != nil {
		return nil, err
	}
	initialRound := DebateRoundRecord{Round: 0, Phase: "initial", ParticipantOutputs: make([]DebateParticipantOutput, 0, len(request.Roles.Participants))}
	initialTasks := make([]Task, 0, len(request.Roles.Participants))
	for _, participantID := range request.Roles.Participants {
		initialTasks = append(initialTasks, InitialProposalTask{
			TaskMeta: TaskMeta{
				SessionID: run.sessionID,
				RequestID: request.RequestID,
				AgentID:   participantID,
				Role:      "participant",
			},
			TaskSpec:  request.TaskSpec,
			Round:     0,
			MaxClaims: request.ProposalPolicy.MaxClaimsPerWorker,
		})
	}
	for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, initialTasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(initialTasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "initial", 0, "该参与者未提交初始提案，辩论基于其余参与者的 claim"))
			continue
		}
		participantID := result.Task.Meta().AgentID
		output, ok := result.Awaited.Output.(InitialProposalTaskResult)
		if !ok {
			return nil, fmt.Errorf("initial proposal task returned unexpected result type")
		}
		entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDebateRoundOutput,
			Source:       EvidenceSourceWorker,
			ProducerID:   participantID,
			ProducerRole: "participant",
			Summary:      output.Output.Summary,
			Artifact:     result.Awaited.Artifact,
			Metadata:     map[string]any{"round": 0, "taskId": result.Receipt.TaskID},
		})
		if err != nil {
			return nil, err
		}
		participant := DebateParticipantOutput{AgentID: participantID, Summary: output.Output.Summary}
		for _, draft := range output.Output.Claims {
			draft = canonicalizeDebateClaimDraft(draft)
			if strings.TrimSpace(draft.Statement) == "" {
				continue
			}
			if isDebateProcessClaim(draft) {
				participant.ProcessNotes = append(participant.ProcessNotes, debateProcessNote(draft))
				continue
			}
			var claimID string
			claims, claimID = upsertDebateClaim(claims, draft, participantID, 0, entry.EntryID, e.ids)
			if claimID == "" {
				continue
			}
			participant.NewClaimIDs = append(participant.NewClaimIDs, claimID)
			run.metrics.ClaimsProposed++
		}
		initialRound.ParticipantOutputs = append(initialRound.ParticipantOutputs, participant)
	}
	rounds = append(rounds, initialRound)
	if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseInitial, checkpointStepDebateInitialComplete, 0, claims, rounds, votes, nil); err != nil {
		return nil, err
	}
	if len(claims) == 0 {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseReport, nil, nil, run.ledgerCursor); err != nil {
			return nil, err
		}
		section := &FreeDebateResultSection{Outcome: FreeDebateOutcomeNoConsensus, Rounds: rounds}
		report := buildFreeDebateReport(request, section)
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindReportGenerated,
			Source:       EvidenceSourceReporter,
			ProducerID:   request.Roles.Reporter,
			ProducerRole: "reporter",
			Summary:      report.Summary,
		}); err != nil {
			return nil, err
		}
		result := &RunResult{
			SchemaVersion: SchemaVersion,
			Mode:          WorkflowModeFreeDebate,
			RequestID:     request.RequestID,
			SessionID:     run.sessionID,
			Lineage:       request.Lineage,
			TaskSpec:      request.TaskSpec,
			Report:        report,
			Metrics:       finalizeMetrics(run.metrics, run.startedAt, e.clock),
			Degradations:  run.degradations,
			FreeDebate:    section,
		}
		if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseReport, checkpointStepReportComplete, 0, claims, rounds, votes, &report); err != nil {
			return nil, err
		}
		return e.finalizeWorkflow(ctx, request, run, result)
	}

	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseDebate, nil, nil, run.ledgerCursor); err != nil {
		return nil, err
	}
	for round := 1; round <= request.DebatePolicy.MaxRounds; round++ {
		if e.isGlobalDeadlineHit(request, run.startedAt) {
			run.metrics.GlobalDeadlineHit = true
			break
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDebateRoundOpened,
			Source:       EvidenceSourceCoordinator,
			ProducerRole: "coordinator",
			Summary:      "debate round opened",
			Metadata:     map[string]any{"round": round, "phase": "debate"},
		}); err != nil {
			return nil, err
		}
		roundRecord := DebateRoundRecord{Round: round, Phase: "debate", ParticipantOutputs: make([]DebateParticipantOutput, 0, len(request.Roles.Participants))}
		roundNewClaims := 0
		onlyAgree := true
		maxNewClaims := request.DebatePolicy.MaxNewClaimsPerRound
		if ceiling := request.DebatePolicy.MaxActiveClaims; ceiling > 0 && len(activeDebateClaims(claims)) >= ceiling {
			// Active-claim ceiling reached: this round may only judge, revise,
			// and merge; MaxNewClaims < 0 marks "no new claims allowed".
			maxNewClaims = -1
		}
		roundTasks := make([]Task, 0, len(request.Roles.Participants))
		for _, participantID := range request.Roles.Participants {
			selfClaims, peerClaims := splitDebateClaims(claims, participantID)
			roundTasks = append(roundTasks, DebateRoundTask{
				TaskMeta: TaskMeta{
					SessionID: run.sessionID,
					RequestID: request.RequestID,
					AgentID:   participantID,
					Role:      "participant",
				},
				TaskSpec:        request.TaskSpec,
				Round:           round,
				SelfClaims:      selfClaims,
				PeerClaims:      peerClaims,
				RoundSummary:    summarizeDebateClaims(peerClaims),
				MaxNewClaims:    maxNewClaims,
				PeerContextMode: request.DebatePolicy.PeerContextMode,
			})
		}
		for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, roundTasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(roundTasks)) {
			recordTaskBatchResultMetrics(&run.metrics, result)
			if result.Err != nil {
				onlyAgree = false
				run.addDegradation(participantAbsentDegradation(result, "debate", round, "该参与者缺席本轮辩论，未对 peer claims 给出评判"))
				continue
			}
			participantID := result.Task.Meta().AgentID
			output, ok := result.Awaited.Output.(DebateRoundTaskResult)
			if !ok {
				return nil, fmt.Errorf("debate round task returned unexpected result type")
			}
			newClaims, truncatedNewClaims := truncateDebateNewClaims(output.Output.NewClaims, maxNewClaims)
			metadata := map[string]any{"round": round, "taskId": result.Receipt.TaskID}
			if truncatedNewClaims > 0 {
				metadata["truncatedNewClaims"] = truncatedNewClaims
				metadata["maxNewClaims"] = maxNewClaims
			}
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindDebateRoundOutput,
				Source:       EvidenceSourceWorker,
				ProducerID:   participantID,
				ProducerRole: "participant",
				Summary:      output.Output.Summary,
				Artifact:     result.Awaited.Artifact,
				Metadata:     metadata,
			})
			if err != nil {
				return nil, err
			}
			participant := DebateParticipantOutput{AgentID: participantID, Summary: output.Output.Summary}
			for _, draft := range newClaims {
				draft = canonicalizeDebateClaimDraft(draft)
				if strings.TrimSpace(draft.Statement) == "" {
					continue
				}
				if isDebateProcessClaim(draft) {
					participant.ProcessNotes = append(participant.ProcessNotes, debateProcessNote(draft))
					continue
				}
				var claimID string
				claims, claimID = upsertDebateClaim(claims, draft, participantID, round, entry.EntryID, e.ids)
				if claimID == "" {
					continue
				}
				participant.NewClaimIDs = append(participant.NewClaimIDs, claimID)
				roundNewClaims++
				run.metrics.ClaimsProposed++
			}
			for _, judgement := range output.Output.Judgements {
				record := DebateJudgementRecord{
					ClaimID:         strings.TrimSpace(judgement.ClaimID),
					Judgement:       judgement.Judgement,
					Rationale:       judgement.Rationale,
					MergeWithClaims: dedupeStrings(judgement.MergeWithClaims),
				}
				if judgement.Judgement != DebateJudgementAgree && judgement.Judgement != DebateJudgementNoChange {
					onlyAgree = false
				}
				if judgement.Judgement == DebateJudgementRevise && strings.TrimSpace(judgement.RevisedStatement) != "" {
					claims, record.RevisedClaimID = upsertDebateClaim(claims, ClaimDraft{
						Title:     "Revision by " + participantID,
						Statement: judgement.RevisedStatement,
					}, participantID, round, entry.EntryID, e.ids)
					if record.RevisedClaimID != "" {
						roundNewClaims++
						run.metrics.ClaimsProposed++
					}
				}
				if len(record.MergeWithClaims) > 0 {
					claims = markDebateClaimMerged(claims, record.ClaimID, record.MergeWithClaims[0])
				}
				participant.Judgements = append(participant.Judgements, record)
			}
			roundRecord.ParticipantOutputs = append(roundRecord.ParticipantOutputs, participant)
		}
		roundRecord.Summary = summarizeDebateRound(roundRecord)
		rounds = append(rounds, roundRecord)
		if request.DebatePolicy.SemanticDedup.Cadence != DebateSemanticDedupCadenceFinal {
			// per_round cadence: consolidate right away so the next round's
			// peer context (and eventually the ballot) stays canonical, and a
			// single failed dedup call only loses one round of consolidation.
			claims = e.runDebateSemanticDedup(ctx, request, run, claims, round)
		}
		earlyStop := boolOrDefault(request.DebatePolicy.EnableEarlyStop, true) && round >= request.DebatePolicy.MinRounds && roundNewClaims == 0 && onlyAgree
		checkpointStep := checkpointStepDebateRoundComplete
		if earlyStop || round == request.DebatePolicy.MaxRounds {
			checkpointStep = checkpointStepDebateRoundsComplete
		}
		if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseDebate, checkpointStep, round, claims, rounds, votes, nil); err != nil {
			return nil, err
		}
		if earlyStop {
			break
		}
	}
	lastDebateRound := 0
	if len(rounds) > 0 {
		lastDebateRound = rounds[len(rounds)-1].Round
	}
	if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseDebate, checkpointStepDebateRoundsComplete, lastDebateRound, claims, rounds, votes, nil); err != nil {
		return nil, err
	}
	if request.DebatePolicy.SemanticDedup.Cadence == DebateSemanticDedupCadenceFinal || len(rounds) <= 1 {
		// final cadence keeps the single pre-vote pass; per_round already
		// deduped after the last executed round unless no debate round ran.
		claims = e.runDebateSemanticDedup(ctx, request, run, claims, len(rounds))
	}
	claims = e.runDebateSynthesis(ctx, request, run, claims, &rounds)
	if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseDebate, checkpointStepDebateSynthesisComplete, lastDebateRound, claims, rounds, votes, nil); err != nil {
		return nil, err
	}

	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseFinalVote, nil, nil, run.ledgerCursor); err != nil {
		return nil, err
	}
	activeClaims := activeDebateClaims(claims)
	voteTasks := make([]Task, 0, len(request.Roles.Participants))
	for _, participantID := range request.Roles.Participants {
		voteTasks = append(voteTasks, FinalVoteTask{
			TaskMeta: TaskMeta{
				SessionID: run.sessionID,
				RequestID: request.RequestID,
				AgentID:   participantID,
				Role:      "participant",
			},
			TaskSpec: request.TaskSpec,
			Round:    len(rounds),
			Claims:   activeClaims,
		})
	}
	voters := make([]string, 0, len(request.Roles.Participants))
	for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, voteTasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(voteTasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "final_vote", len(rounds), "该参与者未投票，claim 接受判定仅基于成功投票的参与者"))
			continue
		}
		participantID := result.Task.Meta().AgentID
		output, ok := result.Awaited.Output.(FinalVoteTaskResult)
		if !ok {
			return nil, fmt.Errorf("final vote task returned unexpected result type")
		}
		if err := ValidateDebateVotes(activeClaims, output.Output.Votes); err != nil {
			run.addDegradation(Degradation{
				Kind:    DegradationParticipantAbsent,
				Phase:   "final_vote",
				Round:   len(rounds),
				AgentID: participantID,
				Reason:  err.Error(),
				Impact:  "该参与者提交的 ballot 不完整或无效，整张 ballot 已拒绝且不计入 voters",
			})
			continue
		}
		voters = appendUnique(voters, participantID)
		participantVotes := make([]DebateVoteRecord, 0, len(output.Output.Votes))
		for _, draft := range output.Output.Votes {
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				ClaimID:      draft.ClaimID,
				Kind:         EvidenceKindDebateVoteCast,
				Source:       EvidenceSourceWorker,
				ProducerID:   participantID,
				ProducerRole: "participant",
				Summary:      output.Output.Summary,
				Artifact:     result.Awaited.Artifact,
				Metadata: map[string]any{
					"taskId":     result.Receipt.TaskID,
					"vote":       draft.Vote,
					"confidence": *draft.Confidence,
					"rationale":  draft.Rationale,
				},
			})
			if err != nil {
				return nil, err
			}
			participantVotes = append(participantVotes, DebateVoteRecord{
				ClaimID:     draft.ClaimID,
				AgentID:     participantID,
				Vote:        draft.Vote,
				Confidence:  *draft.Confidence,
				Rationale:   draft.Rationale,
				EvidenceRef: entry.EntryID,
			})
		}
		votes = append(votes, participantVotes...)
	}

	resolutions, outcome := resolveDebateClaims(activeClaims, claims, votes, request.DebatePolicy)
	absentVoters := make([]string, 0)
	for _, participantID := range request.Roles.Participants {
		if !slices.Contains(voters, participantID) {
			absentVoters = append(absentVoters, participantID)
		}
	}
	if quorum := request.DebatePolicy.VoteQuorum; quorum > 0 && len(request.Roles.Participants) > 0 {
		var quorumFailures []debateClaimQuorumFailure
		resolutions, quorumFailures = enforceDebateClaimQuorum(resolutions, activeClaims, votes, request.Roles.Participants, quorum)
		if len(quorumFailures) > 0 {
			details := make([]string, 0, len(quorumFailures))
			metadata := make([]map[string]any, 0, len(quorumFailures))
			for _, failure := range quorumFailures {
				details = append(details, fmt.Sprintf("%s=%d/%d", failure.ClaimID, failure.VoteCount, failure.RequiredVotes))
				metadata = append(metadata, map[string]any{
					"claimId":       failure.ClaimID,
					"voteCount":     failure.VoteCount,
					"requiredVotes": failure.RequiredVotes,
				})
			}
			impact := fmt.Sprintf("active claims 未分别满足 quorum %.2f（%s），outcome 由 %s 降级为 %s", quorum, strings.Join(details, ", "), outcome, FreeDebateOutcomeQuorumNotMet)
			outcome = FreeDebateOutcomeQuorumNotMet
			run.addDegradation(Degradation{
				Kind:   DegradationQuorumNotMet,
				Phase:  "final_vote",
				Round:  len(rounds),
				Impact: impact,
			})
			if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindDebateVoteQuorum,
				Source:       EvidenceSourceCoordinator,
				ProducerRole: "coordinator",
				Summary:      impact,
				Metadata: map[string]any{
					"voteQuorum":   quorum,
					"voters":       voters,
					"absentVoters": absentVoters,
					"participants": len(request.Roles.Participants),
					"claimQuorum":  metadata,
				},
			}); err != nil {
				return nil, err
			}
		}
	}
	if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseFinalVote, checkpointStepDebateVoteComplete, lastDebateRound, claims, rounds, votes, nil); err != nil {
		return nil, err
	}
	section := &FreeDebateResultSection{
		Outcome:          outcome,
		Rounds:           rounds,
		Claims:           claims,
		ClaimResolutions: resolutions,
		Votes:            votes,
		Voters:           voters,
		AbsentVoters:     absentVoters,
		BallotSize:       len(activeClaims),
	}
	if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseReport, nil, nil, run.ledgerCursor); err != nil {
		return nil, err
	}
	report := buildFreeDebateReport(request, section)
	report, reportArtifact, err := e.composeModeReport(ctx, request, run, WorkflowModeFreeDebate, report, TaskVerdictFromDebateOutcome(outcome), map[string]any{
		"freeDebate": section,
	})
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

	result := &RunResult{
		SchemaVersion: SchemaVersion,
		Mode:          WorkflowModeFreeDebate,
		RequestID:     request.RequestID,
		SessionID:     run.sessionID,
		Lineage:       request.Lineage,
		TaskSpec:      request.TaskSpec,
		Report:        report,
		Metrics:       finalizeMetrics(run.metrics, run.startedAt, e.clock),
		Degradations:  run.degradations,
		FreeDebate:    section,
	}
	if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseReport, checkpointStepReportComplete, lastDebateRound, claims, rounds, votes, &report); err != nil {
		return nil, err
	}
	return e.finalizeWorkflow(ctx, request, run, result)
}

func (e *Engine) runDebateSemanticDedup(ctx context.Context, request StartRequest, run *workflowRun, claims []DebateClaim, round int) []DebateClaim {
	policy := request.DebatePolicy.SemanticDedup
	if !policy.Enabled || request.Roles.SemanticDeduper == "" {
		return claims
	}
	activeClaims := activeDebateClaims(claims)
	if len(activeClaims) < 2 {
		return claims
	}
	receipt, awaited, err := e.executeTask(ctx, request, run.sessionID, SemanticDedupTask{
		TaskMeta: TaskMeta{
			SessionID: run.sessionID,
			RequestID: request.RequestID,
			AgentID:   request.Roles.SemanticDeduper,
			Role:      "semantic-deduper",
		},
		TaskSpec:            request.TaskSpec,
		Round:               round,
		Claims:              activeClaims,
		SimilarityThreshold: policy.SimilarityThreshold,
	}, run.startedAt, request.WaitingPolicy.PerTaskTimeout)
	run.metrics.TasksDispatched++
	if err != nil {
		if errors.Is(err, ErrGlobalDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			run.metrics.GlobalDeadlineHit = true
		}
		if strings.Contains(err.Error(), "__timeout__") {
			run.metrics.WaitTimeouts++
		}
		_, _ = e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDebateSemanticDedup,
			Source:       EvidenceSourceWorker,
			ProducerID:   request.Roles.SemanticDeduper,
			ProducerRole: "semantic-deduper",
			Summary:      "semantic dedup failed: " + err.Error(),
			Artifact:     awaited.Artifact,
			Metadata: map[string]any{
				"taskId":              receipt.TaskID,
				"taskKind":            TaskKindSemanticDedup,
				"round":               round,
				"similarityThreshold": policy.SimilarityThreshold,
				"status":              "failed",
			},
		})
		run.addDegradation(Degradation{
			Kind:    DegradationStepSkipped,
			Phase:   "semantic_dedup",
			Round:   round,
			AgentID: request.Roles.SemanticDeduper,
			Reason:  err.Error(),
			Impact:  fmt.Sprintf("语义去重未执行，%d 条 active claim 未合并直接进入 final vote", len(activeClaims)),
		})
		return claims
	}
	output, ok := awaited.Output.(SemanticDedupTaskResult)
	if !ok {
		_, _ = e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDebateSemanticDedup,
			Source:       EvidenceSourceWorker,
			ProducerID:   request.Roles.SemanticDeduper,
			ProducerRole: "semantic-deduper",
			Summary:      "semantic dedup returned unexpected result type",
			Artifact:     awaited.Artifact,
			Metadata: map[string]any{
				"taskId":   receipt.TaskID,
				"taskKind": TaskKindSemanticDedup,
				"round":    round,
				"status":   "failed",
			},
		})
		run.addDegradation(Degradation{
			Kind:    DegradationStepSkipped,
			Phase:   "semantic_dedup",
			Round:   round,
			AgentID: request.Roles.SemanticDeduper,
			Reason:  "semantic dedup returned unexpected result type",
			Impact:  fmt.Sprintf("语义去重未执行，%d 条 active claim 未合并直接进入 final vote", len(activeClaims)),
		})
		return claims
	}
	entry, appendErr := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindDebateSemanticDedup,
		Source:       EvidenceSourceWorker,
		ProducerID:   request.Roles.SemanticDeduper,
		ProducerRole: "semantic-deduper",
		Summary:      output.Output.Summary,
		Artifact:     awaited.Artifact,
		Metadata: map[string]any{
			"taskId":              receipt.TaskID,
			"taskKind":            TaskKindSemanticDedup,
			"round":               round,
			"mergeCount":          len(output.Output.Merges),
			"similarityThreshold": policy.SimilarityThreshold,
			"status":              "completed",
		},
	})
	evidenceRef := ""
	if appendErr == nil {
		evidenceRef = entry.EntryID
	}
	return applyDebateSemanticDedup(claims, output.Output.Merges, evidenceRef)
}

// runDebateSynthesis runs the pre-vote synthesis phase: a designated
// synthesizer drafts the single canonical integrated-recommendation claim
// from the atom claims plus the participants' category=synthesis drafts
// (which are merged into the canonical for provenance), the panel amends the
// draft for AmendmentRounds passes, and the result joins the ballot. Every
// failure degrades gracefully back to the pre-synthesis state.
func (e *Engine) runDebateSynthesis(ctx context.Context, request StartRequest, run *workflowRun, claims []DebateClaim, rounds *[]DebateRoundRecord) []DebateClaim {
	policy := request.DebatePolicy.Synthesis
	if !policy.Enabled || request.Roles.Synthesizer == "" {
		return claims
	}
	active := activeDebateClaims(claims)
	if len(active) == 0 {
		return claims
	}
	atoms := make([]DebateClaim, 0, len(active))
	drafts := make([]DebateClaim, 0)
	for _, claim := range active {
		if claim.Category == DebateClaimCategorySynthesis {
			drafts = append(drafts, claim)
		} else {
			atoms = append(atoms, claim)
		}
	}
	round := len(*rounds)
	draft, entryID, ok := e.executeSynthesisTask(ctx, request, run, SynthesisTask{
		TaskMeta: TaskMeta{
			SessionID: run.sessionID,
			RequestID: request.RequestID,
			AgentID:   request.Roles.Synthesizer,
			Role:      "synthesizer",
		},
		TaskSpec: request.TaskSpec,
		Round:    round,
		Claims:   atoms,
		Drafts:   drafts,
	}, "合成阶段被跳过，参与者的综合稿保留原样进入投票")
	if !ok {
		return claims
	}
	canonical := DebateClaim{
		ClaimID:      e.ids.NewEntityID("debate_claim"),
		Title:        strings.TrimSpace(draft.Title),
		Statement:    strings.TrimSpace(draft.Statement),
		Category:     DebateClaimCategorySynthesis,
		OwnerID:      request.Roles.Synthesizer,
		ProposedBy:   filterEmpty([]string{request.Roles.Synthesizer}),
		Round:        round,
		Active:       true,
		EvidenceRefs: filterEmpty([]string{entryID}),
	}
	claims = append(claims, canonical)
	for _, consumed := range drafts {
		claims = mergeDebateClaim(claims, consumed.ClaimID, canonical.ClaimID, entryID)
	}

	for amend := 1; amend <= policy.AmendmentRounds; amend++ {
		canonicalIdx := debateClaimIndex(claims, canonical.ClaimID)
		if canonicalIdx < 0 {
			break
		}
		amendRound := round + amend
		roundRecord := DebateRoundRecord{Round: amendRound, Phase: "amend", ParticipantOutputs: make([]DebateParticipantOutput, 0, len(request.Roles.Participants))}
		amendTasks := make([]Task, 0, len(request.Roles.Participants))
		for _, participantID := range request.Roles.Participants {
			amendTasks = append(amendTasks, DebateRoundTask{
				TaskMeta: TaskMeta{
					SessionID: run.sessionID,
					RequestID: request.RequestID,
					AgentID:   participantID,
					Role:      "participant",
				},
				TaskSpec:        request.TaskSpec,
				Round:           amendRound,
				PeerClaims:      []DebateClaim{claims[canonicalIdx]},
				RoundSummary:    "synthesis draft review",
				MaxNewClaims:    -1,
				SynthesisReview: true,
				PeerContextMode: request.DebatePolicy.PeerContextMode,
			})
		}
		revisions := make([]DebateJudgementRecord, 0)
		for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, amendTasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(amendTasks)) {
			recordTaskBatchResultMetrics(&run.metrics, result)
			if result.Err != nil {
				run.addDegradation(participantAbsentDegradation(result, "amend", amendRound, "该参与者未评审合成稿，修正基于其余参与者的意见"))
				continue
			}
			participantID := result.Task.Meta().AgentID
			output, typedOK := result.Awaited.Output.(DebateRoundTaskResult)
			if !typedOK {
				run.addDegradation(participantAbsentDegradation(result, "amend", amendRound, "该参与者的评审输出类型异常，已忽略"))
				continue
			}
			if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindDebateRoundOutput,
				Source:       EvidenceSourceWorker,
				ProducerID:   participantID,
				ProducerRole: "participant",
				Summary:      output.Output.Summary,
				Artifact:     result.Awaited.Artifact,
				Metadata:     map[string]any{"round": amendRound, "phase": "amend", "taskId": result.Receipt.TaskID},
			}); err != nil {
				continue
			}
			participant := DebateParticipantOutput{AgentID: participantID, Summary: output.Output.Summary}
			for _, judgement := range output.Output.Judgements {
				if strings.TrimSpace(judgement.ClaimID) != canonical.ClaimID {
					continue
				}
				record := DebateJudgementRecord{
					ClaimID:          canonical.ClaimID,
					Judgement:        judgement.Judgement,
					Rationale:        judgement.Rationale,
					RevisedStatement: strings.TrimSpace(judgement.RevisedStatement),
				}
				participant.Judgements = append(participant.Judgements, record)
				switch {
				case judgement.Judgement == DebateJudgementRevise && record.RevisedStatement != "":
					amendment := record
					amendment.Rationale = participantID + " | " + judgement.Rationale
					revisions = append(revisions, amendment)
				case judgement.Judgement == DebateJudgementDisagree:
					amendment := record
					amendment.Rationale = participantID + " | " + judgement.Rationale
					revisions = append(revisions, amendment)
				}
			}
			roundRecord.ParticipantOutputs = append(roundRecord.ParticipantOutputs, participant)
		}
		roundRecord.Summary = summarizeDebateRound(roundRecord)
		*rounds = append(*rounds, roundRecord)
		if len(revisions) == 0 {
			break
		}
		current := claims[canonicalIdx]
		integrated, integratedEntryID, integratedOK := e.executeSynthesisTask(ctx, request, run, SynthesisTask{
			TaskMeta: TaskMeta{
				SessionID: run.sessionID,
				RequestID: request.RequestID,
				AgentID:   request.Roles.Synthesizer,
				Role:      "synthesizer",
			},
			TaskSpec:   request.TaskSpec,
			Round:      amendRound,
			Claims:     atoms,
			Draft:      &current,
			Amendments: revisions,
		}, "修正意见未整合，合成稿以修正前版本进入投票")
		if !integratedOK {
			break
		}
		statement := strings.TrimSpace(integrated.Statement)
		if statement == "" {
			break
		}
		claims[canonicalIdx].Statement = statement
		if title := strings.TrimSpace(integrated.Title); title != "" {
			claims[canonicalIdx].Title = title
		}
		claims[canonicalIdx].EvidenceRefs = appendUnique(claims[canonicalIdx].EvidenceRefs, integratedEntryID)
	}
	return claims
}

// executeSynthesisTask runs one synthesizer call, records ledger evidence,
// and converts failures into step_skipped degradations with the given impact.
func (e *Engine) executeSynthesisTask(ctx context.Context, request StartRequest, run *workflowRun, task SynthesisTask, failureImpact string) (ClaimDraft, string, bool) {
	receipt, awaited, err := e.executeTask(ctx, request, run.sessionID, task, run.startedAt, request.WaitingPolicy.PerTaskTimeout)
	run.metrics.TasksDispatched++
	if err != nil {
		if errors.Is(err, ErrGlobalDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			run.metrics.GlobalDeadlineHit = true
		}
		_, _ = e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDebateSynthesis,
			Source:       EvidenceSourceWorker,
			ProducerID:   request.Roles.Synthesizer,
			ProducerRole: "synthesizer",
			Summary:      "synthesis failed: " + err.Error(),
			Artifact:     awaited.Artifact,
			Metadata:     map[string]any{"taskId": receipt.TaskID, "taskKind": TaskKindSynthesis, "round": task.Round, "status": "failed"},
		})
		run.addDegradation(Degradation{
			Kind:    DegradationStepSkipped,
			Phase:   "synthesis",
			Round:   task.Round,
			AgentID: request.Roles.Synthesizer,
			Reason:  err.Error(),
			Impact:  failureImpact,
		})
		return ClaimDraft{}, "", false
	}
	output, ok := awaited.Output.(SynthesisTaskResult)
	if !ok || strings.TrimSpace(output.Output.Claim.Statement) == "" {
		reason := "synthesis returned unexpected result type"
		if ok {
			reason = "synthesis returned an empty claim statement"
		}
		_, _ = e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDebateSynthesis,
			Source:       EvidenceSourceWorker,
			ProducerID:   request.Roles.Synthesizer,
			ProducerRole: "synthesizer",
			Summary:      reason,
			Artifact:     awaited.Artifact,
			Metadata:     map[string]any{"taskId": receipt.TaskID, "taskKind": TaskKindSynthesis, "round": task.Round, "status": "failed"},
		})
		run.addDegradation(Degradation{
			Kind:    DegradationStepSkipped,
			Phase:   "synthesis",
			Round:   task.Round,
			AgentID: request.Roles.Synthesizer,
			Reason:  reason,
			Impact:  failureImpact,
		})
		return ClaimDraft{}, "", false
	}
	entry, appendErr := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind:         EvidenceKindDebateSynthesis,
		Source:       EvidenceSourceWorker,
		ProducerID:   request.Roles.Synthesizer,
		ProducerRole: "synthesizer",
		Summary:      output.Output.Summary,
		Artifact:     awaited.Artifact,
		Metadata:     map[string]any{"taskId": receipt.TaskID, "taskKind": TaskKindSynthesis, "round": task.Round, "status": "completed", "integration": task.Draft != nil},
	})
	entryID := ""
	if appendErr == nil {
		entryID = entry.EntryID
	}
	draft := canonicalizeDebateClaimDraft(output.Output.Claim)
	return draft, entryID, true
}

func debateClaimIndex(claims []DebateClaim, claimID string) int {
	for idx := range claims {
		if claims[idx].ClaimID == claimID {
			return idx
		}
	}
	return -1
}

// truncateDebateNewClaims enforces the per-round new-claims budget: limit > 0
// keeps the first limit drafts, limit < 0 drops all drafts (active-claim
// ceiling reached), limit == 0 means no budget was set.
func truncateDebateNewClaims(drafts []ClaimDraft, limit int) ([]ClaimDraft, int) {
	if limit == 0 || len(drafts) == 0 {
		return drafts, 0
	}
	allowed := limit
	if allowed < 0 {
		allowed = 0
	}
	if len(drafts) <= allowed {
		return drafts, 0
	}
	return drafts[:allowed], len(drafts) - allowed
}

func upsertDebateClaim(claims []DebateClaim, draft ClaimDraft, ownerID string, round int, evidenceRef string, ids IDFactory) ([]DebateClaim, string) {
	draft = canonicalizeDebateClaimDraft(draft)
	if strings.TrimSpace(draft.Statement) == "" {
		return claims, ""
	}
	key := normalizedClaimKey(draft.Statement)
	for idx := range claims {
		if normalizedClaimKey(claims[idx].Statement) != key {
			continue
		}
		claims[idx].EvidenceRefs = appendUnique(claims[idx].EvidenceRefs, evidenceRef)
		claims[idx].ProposedBy = appendUnique(claims[idx].ProposedBy, ownerID)
		claims[idx].Active = true
		return claims, claims[idx].ClaimID
	}
	claim := DebateClaim{
		ClaimID:      ids.NewEntityID("debate_claim"),
		Title:        strings.TrimSpace(draft.Title),
		Statement:    strings.TrimSpace(draft.Statement),
		Category:     draft.Category,
		OwnerID:      ownerID,
		ProposedBy:   filterEmpty([]string{ownerID}),
		Round:        round,
		Active:       true,
		EvidenceRefs: filterEmpty([]string{evidenceRef}),
	}
	return append(claims, claim), claim.ClaimID
}

func markDebateClaimMerged(claims []DebateClaim, claimID string, targetID string) []DebateClaim {
	return mergeDebateClaim(claims, claimID, targetID, "")
}

func applyDebateSemanticDedup(claims []DebateClaim, merges []DebateClaimMergeDraft, evidenceRef string) []DebateClaim {
	for _, merge := range merges {
		claims = mergeDebateClaim(claims, merge.SourceClaimID, merge.TargetClaimID, evidenceRef)
	}
	return claims
}

func mergeDebateClaim(claims []DebateClaim, sourceID string, targetID string, evidenceRef string) []DebateClaim {
	sourceID = strings.TrimSpace(sourceID)
	targetID = strings.TrimSpace(targetID)
	if sourceID == "" || targetID == "" || sourceID == targetID {
		return claims
	}
	sourceIdx := -1
	targetIdx := -1
	for idx := range claims {
		switch claims[idx].ClaimID {
		case sourceID:
			sourceIdx = idx
		case targetID:
			targetIdx = idx
		}
	}
	if sourceIdx < 0 || targetIdx < 0 || !claims[sourceIdx].Active || !claims[targetIdx].Active {
		return claims
	}
	source := claims[sourceIdx]
	claims[sourceIdx].Active = false
	claims[sourceIdx].MergedInto = targetID
	claims[sourceIdx].EvidenceRefs = appendUnique(claims[sourceIdx].EvidenceRefs, evidenceRef)
	sourceProposers := appendUniqueStrings(source.ProposedBy, source.OwnerID)
	sourceEvidenceRefs := appendUniqueStrings(source.EvidenceRefs, evidenceRef)
	claims[targetIdx].ProposedBy = appendUniqueStrings(claims[targetIdx].ProposedBy, sourceProposers...)
	claims[targetIdx].MergedClaimIDs = appendUnique(claims[targetIdx].MergedClaimIDs, sourceID)
	claims[targetIdx].EvidenceRefs = appendUniqueStrings(claims[targetIdx].EvidenceRefs, sourceEvidenceRefs...)
	return claims
}

func appendUniqueStrings(base []string, values ...string) []string {
	out := slices.Clone(base)
	for _, value := range values {
		out = appendUnique(out, value)
	}
	return out
}

func splitDebateClaims(claims []DebateClaim, ownerID string) ([]DebateClaim, []DebateClaim) {
	self := make([]DebateClaim, 0)
	peer := make([]DebateClaim, 0)
	for _, claim := range claims {
		if !claim.Active {
			continue
		}
		if claim.OwnerID == ownerID || slices.Contains(claim.ProposedBy, ownerID) {
			self = append(self, claim)
			continue
		}
		peer = append(peer, claim)
	}
	return self, peer
}

func summarizeDebateClaims(claims []DebateClaim) string {
	if len(claims) == 0 {
		return "no peer claims"
	}
	parts := make([]string, 0, len(claims))
	for _, claim := range claims {
		parts = append(parts, firstNonEmpty(claim.Title, claim.Statement))
	}
	return strings.Join(parts, "; ")
}

func summarizeDebateRound(round DebateRoundRecord) string {
	if len(round.ParticipantOutputs) == 0 {
		return "no participant output"
	}
	parts := make([]string, 0, len(round.ParticipantOutputs))
	for _, item := range round.ParticipantOutputs {
		parts = append(parts, firstNonEmpty(item.Summary, item.AgentID))
	}
	return strings.Join(parts, "; ")
}

func activeDebateClaims(claims []DebateClaim) []DebateClaim {
	out := make([]DebateClaim, 0)
	for _, claim := range claims {
		if claim.Active {
			out = append(out, claim)
		}
	}
	return out
}

// debateVoteCoherent reports whether the coarse vote label agrees with the
// continuous confidence support score defined by the vote prompt contract:
// accept requires confidence >= 0.5 and reject requires confidence <= 0.5.
// Abstentions are always coherent but do not contribute their confidence to
// resolution math. Votes that fail this check are excluded so they cannot
// flip an accept decision.
func debateVoteCoherent(vote DebateVoteRecord) bool {
	switch vote.Vote {
	case DebateVoteAccept:
		return vote.Confidence >= 0.5
	case DebateVoteReject:
		return vote.Confidence <= 0.5
	case DebateVoteAbstain:
		return true
	default:
		return false
	}
}

type debateClaimQuorumFailure struct {
	ClaimID       string
	VoteCount     int
	RequiredVotes int
}

func enforceDebateClaimQuorum(
	resolutions []DebateClaimResolution,
	activeClaims []DebateClaim,
	votes []DebateVoteRecord,
	participants []string,
	quorum float64,
) ([]DebateClaimResolution, []debateClaimQuorumFailure) {
	participantIDs := dedupeStrings(participants)
	if quorum <= 0 || len(participantIDs) == 0 {
		return resolutions, nil
	}
	requiredVotes := int(math.Ceil(quorum * float64(len(participantIDs))))
	allowedVoters := make(map[string]struct{}, len(participantIDs))
	for _, participantID := range participantIDs {
		allowedVoters[participantID] = struct{}{}
	}
	votersByClaim := make(map[string]map[string]struct{}, len(activeClaims))
	for _, claim := range activeClaims {
		votersByClaim[claim.ClaimID] = map[string]struct{}{}
	}
	for _, vote := range votes {
		claimVoters, knownClaim := votersByClaim[vote.ClaimID]
		_, knownVoter := allowedVoters[vote.AgentID]
		if !knownClaim || !knownVoter || !debateVoteCoherent(vote) {
			continue
		}
		claimVoters[vote.AgentID] = struct{}{}
	}

	failures := make([]debateClaimQuorumFailure, 0)
	for _, claim := range activeClaims {
		voteCount := len(votersByClaim[claim.ClaimID])
		if voteCount >= requiredVotes {
			continue
		}
		failures = append(failures, debateClaimQuorumFailure{
			ClaimID:       claim.ClaimID,
			VoteCount:     voteCount,
			RequiredVotes: requiredVotes,
		})
		for idx := range resolutions {
			if resolutions[idx].ClaimID == claim.ClaimID {
				resolutions[idx].Accepted = false
				break
			}
		}
	}
	return resolutions, failures
}

func aggregateSupportScore(values []float64, method DebateVoteAggregation) float64 {
	if len(values) == 0 {
		return 0
	}
	if method == DebateVoteAggregationMean {
		mean, _ := meanAndVariance(values)
		return mean
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func resolveDebateClaims(activeClaims []DebateClaim, allClaims []DebateClaim, votes []DebateVoteRecord, policy DebatePolicy) ([]DebateClaimResolution, FreeDebateOutcome) {
	resolutions := make([]DebateClaimResolution, 0, len(allClaims))
	accepted := 0
	for _, claim := range allClaims {
		if !claim.Active {
			resolutions = append(resolutions, DebateClaimResolution{
				ClaimID:        claim.ClaimID,
				Accepted:       false,
				FinalStatement: claim.Statement,
				MergedInto:     claim.MergedInto,
				ProposedBy:     slices.Clone(claim.ProposedBy),
			})
			continue
		}
		supporters := make([]string, 0)
		opposers := make([]string, 0)
		abstainers := make([]string, 0)
		confidences := make([]float64, 0)
		seenVoters := make(map[string]struct{})
		incoherent := 0
		validVotes := 0
		for _, vote := range votes {
			if vote.ClaimID != claim.ClaimID {
				continue
			}
			if _, duplicate := seenVoters[vote.AgentID]; duplicate {
				incoherent++
				continue
			}
			seenVoters[vote.AgentID] = struct{}{}
			if !debateVoteCoherent(vote) {
				incoherent++
				continue
			}
			switch vote.Vote {
			case DebateVoteAccept:
				confidences = append(confidences, vote.Confidence)
				supporters = append(supporters, vote.AgentID)
				validVotes++
			case DebateVoteReject:
				confidences = append(confidences, vote.Confidence)
				opposers = append(opposers, vote.AgentID)
				validVotes++
			case DebateVoteAbstain:
				abstainers = append(abstainers, vote.AgentID)
			}
		}
		ratio := 0.0
		if validVotes > 0 {
			ratio = float64(len(supporters)) / float64(validVotes)
		}
		supportScore := aggregateSupportScore(confidences, policy.VoteAggregation)
		confidenceMean, confidenceVariance := meanAndVariance(confidences)
		acceptedClaim := len(confidences) > 0 && supportScore >= policy.VoteThreshold
		if acceptedClaim {
			accepted++
		}
		resolutions = append(resolutions, DebateClaimResolution{
			ClaimID:            claim.ClaimID,
			Accepted:           acceptedClaim,
			SupportScore:       supportScore,
			SupportRatio:       ratio,
			ConfidenceMean:     confidenceMean,
			ConfidenceVariance: confidenceVariance,
			ConfidenceStdDev:   math.Sqrt(confidenceVariance),
			VoteCount:          len(supporters) + len(opposers) + len(abstainers),
			IncoherentVotes:    incoherent,
			SupportingVoters:   supporters,
			OpposingVoters:     opposers,
			AbstainingVoters:   abstainers,
			FinalStatement:     claim.Statement,
			ProposedBy:         slices.Clone(claim.ProposedBy),
		})
	}
	switch {
	case len(activeClaims) > 0 && accepted == len(activeClaims):
		return resolutions, FreeDebateOutcomeConsensus
	case accepted > 0:
		return resolutions, FreeDebateOutcomePartialConsensus
	default:
		return resolutions, FreeDebateOutcomeNoConsensus
	}
}

func meanAndVariance(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	mean := sum / float64(len(values))
	varianceSum := 0.0
	for _, value := range values {
		delta := value - mean
		varianceSum += delta * delta
	}
	return mean, varianceSum / float64(len(values))
}

func TaskVerdictFromDebateOutcome(outcome FreeDebateOutcome) TaskVerdict {
	switch outcome {
	case FreeDebateOutcomeConsensus:
		return TaskVerdictSupported
	case FreeDebateOutcomePartialConsensus:
		return TaskVerdictPartiallySupported
	case FreeDebateOutcomeQuorumNotMet:
		return TaskVerdictUndetermined
	default:
		return TaskVerdictUndetermined
	}
}

func buildFreeDebateReport(request StartRequest, section *FreeDebateResultSection) AdjudicationReport {
	highlights := make([]string, 0, len(section.ClaimResolutions))
	for _, resolution := range section.ClaimResolutions {
		if resolution.Accepted {
			highlights = append(highlights, fmt.Sprintf("accepted: %s", resolution.FinalStatement))
		}
	}
	nextActions := make([]string, 0)
	if section.Outcome != FreeDebateOutcomeConsensus {
		nextActions = append(nextActions, "补充更多独立论据，或缩小争议范围")
	}
	return AdjudicationReport{
		Summary:     fmt.Sprintf("自由辩论结果为 %s。任务：%s", section.Outcome, request.TaskSpec.Goal),
		Highlights:  highlights,
		NextActions: nextActions,
	}
}
