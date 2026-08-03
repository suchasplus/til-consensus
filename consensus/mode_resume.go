package consensus

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

func (e *Engine) resumeFreeDebate(ctx context.Context, request StartRequest, snapshot SessionSnapshot) (_ *RunResult, err error) {
	run, err := e.restoreModeRun(ctx, snapshot, request)
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
	return e.continueFreeDebate(ctx, request, run, snapshot)
}

func (e *Engine) continueFreeDebate(ctx context.Context, request StartRequest, run *workflowRun, snapshot SessionSnapshot) (*RunResult, error) {
	claims := append([]DebateClaim(nil), snapshot.DebateClaims...)
	rounds := append([]DebateRoundRecord(nil), snapshot.DebateRounds...)
	votes := append([]DebateVoteRecord(nil), snapshot.DebateVotes...)
	checkpoint := snapshot.Checkpoint
	step := inferredModeCheckpointStep(snapshot)
	rank := freeDebateCheckpointRank(step)
	lastRound := checkpointDebateRound(checkpoint, rounds)
	if rank == 0 {
		if err := e.resumeModePrelude(ctx, request, run); err != nil {
			return nil, err
		}
	}

	if rank < freeDebateCheckpointRank(checkpointStepDebateInitialComplete) {
		if run.state.Current() != SessionPhaseInitial {
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseInitial, nil, nil, run.ledgerCursor); err != nil {
				return nil, err
			}
		}
		var err error
		claims, rounds, err = e.executeResumedDebateInitial(ctx, request, run, claims, rounds)
		if err != nil {
			return nil, err
		}
		if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseInitial, checkpointStepDebateInitialComplete, 0, claims, rounds, votes, nil); err != nil {
			return nil, err
		}
		step = checkpointStepDebateInitialComplete
		rank = freeDebateCheckpointRank(step)
	}

	if len(claims) == 0 {
		section := &FreeDebateResultSection{Outcome: FreeDebateOutcomeNoConsensus, Rounds: rounds, Claims: claims, Votes: votes}
		return e.finishResumedFreeDebate(ctx, request, run, snapshot, section, step)
	}

	if rank < freeDebateCheckpointRank(checkpointStepDebateRoundsComplete) {
		if run.state.Current() != SessionPhaseDebate {
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseDebate, nil, nil, run.ledgerCursor); err != nil {
				return nil, err
			}
		}
		nextRound := 1
		if step == checkpointStepDebateRoundComplete {
			nextRound = lastRound + 1
		}
		for round := nextRound; round <= request.DebatePolicy.MaxRounds; round++ {
			if e.isGlobalDeadlineHit(request, run.startedAt) {
				run.metrics.GlobalDeadlineHit = true
				break
			}
			var roundNewClaims int
			var onlyAgree bool
			var err error
			claims, rounds, roundNewClaims, onlyAgree, err = e.executeResumedDebateRound(ctx, request, run, claims, rounds, round)
			if err != nil {
				return nil, err
			}
			lastRound = round
			earlyStop := boolOrDefault(request.DebatePolicy.EnableEarlyStop, true) && round >= request.DebatePolicy.MinRounds && roundNewClaims == 0 && onlyAgree
			roundStep := checkpointStepDebateRoundComplete
			if earlyStop || round == request.DebatePolicy.MaxRounds {
				roundStep = checkpointStepDebateRoundsComplete
			}
			if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseDebate, roundStep, round, claims, rounds, votes, nil); err != nil {
				return nil, err
			}
			step = roundStep
			if earlyStop {
				break
			}
		}
		if step != checkpointStepDebateRoundsComplete {
			if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseDebate, checkpointStepDebateRoundsComplete, lastRound, claims, rounds, votes, nil); err != nil {
				return nil, err
			}
			step = checkpointStepDebateRoundsComplete
		}
		rank = freeDebateCheckpointRank(step)
	}

	if rank < freeDebateCheckpointRank(checkpointStepDebateSynthesisComplete) {
		if request.DebatePolicy.SemanticDedup.Cadence == DebateSemanticDedupCadenceFinal || len(rounds) <= 1 {
			claims = e.runDebateSemanticDedup(ctx, request, run, claims, len(rounds))
		}
		claims = e.runDebateSynthesis(ctx, request, run, claims, &rounds)
		if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseDebate, checkpointStepDebateSynthesisComplete, lastRound, claims, rounds, votes, nil); err != nil {
			return nil, err
		}
		step = checkpointStepDebateSynthesisComplete
		rank = freeDebateCheckpointRank(step)
	}

	var section *FreeDebateResultSection
	if rank < freeDebateCheckpointRank(checkpointStepDebateVoteComplete) {
		if run.state.Current() != SessionPhaseFinalVote {
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseFinalVote, nil, nil, run.ledgerCursor); err != nil {
				return nil, err
			}
		}
		var err error
		votes, section, err = e.executeResumedDebateVote(ctx, request, run, claims, rounds, votes)
		if err != nil {
			return nil, err
		}
		if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseFinalVote, checkpointStepDebateVoteComplete, lastRound, claims, rounds, votes, nil); err != nil {
			return nil, err
		}
		step = checkpointStepDebateVoteComplete
	} else {
		section = restoredFreeDebateSection(request, claims, rounds, votes)
	}
	return e.finishResumedFreeDebate(ctx, request, run, snapshot, section, step)
}

func (e *Engine) executeResumedDebateInitial(ctx context.Context, request StartRequest, run *workflowRun, claims []DebateClaim, rounds []DebateRoundRecord) ([]DebateClaim, []DebateRoundRecord, error) {
	if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind: EvidenceKindDebateRoundOpened, Source: EvidenceSourceCoordinator, ProducerRole: "coordinator", Summary: "initial round opened",
		Metadata: map[string]any{"round": 0, "phase": "initial"},
	}); err != nil {
		return nil, nil, err
	}
	roundRecord := DebateRoundRecord{Round: 0, Phase: "initial", ParticipantOutputs: make([]DebateParticipantOutput, 0, len(request.Roles.Participants))}
	tasks := make([]Task, 0, len(request.Roles.Participants))
	for _, participantID := range request.Roles.Participants {
		tasks = append(tasks, InitialProposalTask{TaskMeta: TaskMeta{SessionID: run.sessionID, RequestID: request.RequestID, AgentID: participantID, Role: "participant"}, TaskSpec: request.TaskSpec, Round: 0, MaxClaims: request.ProposalPolicy.MaxClaimsPerWorker})
	}
	for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "initial", 0, "该参与者未提交初始提案，辩论基于其余参与者的 claim"))
			continue
		}
		participantID := result.Task.Meta().AgentID
		output, ok := result.Awaited.Output.(InitialProposalTaskResult)
		if !ok {
			return nil, nil, fmt.Errorf("initial proposal task returned unexpected result type")
		}
		entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind: EvidenceKindDebateRoundOutput, Source: EvidenceSourceWorker, ProducerID: participantID, ProducerRole: "participant", Summary: output.Output.Summary,
			Artifact: result.Awaited.Artifact, Metadata: map[string]any{"round": 0, "taskId": result.Receipt.TaskID},
		})
		if err != nil {
			return nil, nil, err
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
			if claimID != "" {
				participant.NewClaimIDs = append(participant.NewClaimIDs, claimID)
				run.metrics.ClaimsProposed++
			}
		}
		roundRecord.ParticipantOutputs = append(roundRecord.ParticipantOutputs, participant)
	}
	return claims, append(rounds, roundRecord), nil
}

func (e *Engine) executeResumedDebateRound(ctx context.Context, request StartRequest, run *workflowRun, claims []DebateClaim, rounds []DebateRoundRecord, round int) ([]DebateClaim, []DebateRoundRecord, int, bool, error) {
	if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind: EvidenceKindDebateRoundOpened, Source: EvidenceSourceCoordinator, ProducerRole: "coordinator", Summary: "debate round opened",
		Metadata: map[string]any{"round": round, "phase": "debate"},
	}); err != nil {
		return nil, nil, 0, false, err
	}
	roundRecord := DebateRoundRecord{Round: round, Phase: "debate", ParticipantOutputs: make([]DebateParticipantOutput, 0, len(request.Roles.Participants))}
	roundNewClaims := 0
	onlyAgree := true
	maxNewClaims := request.DebatePolicy.MaxNewClaimsPerRound
	if ceiling := request.DebatePolicy.MaxActiveClaims; ceiling > 0 && len(activeDebateClaims(claims)) >= ceiling {
		maxNewClaims = -1
	}
	tasks := make([]Task, 0, len(request.Roles.Participants))
	for _, participantID := range request.Roles.Participants {
		selfClaims, peerClaims := splitDebateClaims(claims, participantID)
		tasks = append(tasks, DebateRoundTask{
			TaskMeta: TaskMeta{SessionID: run.sessionID, RequestID: request.RequestID, AgentID: participantID, Role: "participant"}, TaskSpec: request.TaskSpec,
			Round: round, SelfClaims: selfClaims, PeerClaims: peerClaims, RoundSummary: summarizeDebateClaims(peerClaims), MaxNewClaims: maxNewClaims, PeerContextMode: request.DebatePolicy.PeerContextMode,
		})
	}
	for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		if result.Err != nil {
			onlyAgree = false
			run.addDegradation(participantAbsentDegradation(result, "debate", round, "该参与者缺席本轮辩论，未对 peer claims 给出评判"))
			continue
		}
		participantID := result.Task.Meta().AgentID
		output, ok := result.Awaited.Output.(DebateRoundTaskResult)
		if !ok {
			return nil, nil, 0, false, fmt.Errorf("debate round task returned unexpected result type")
		}
		newClaims, truncated := truncateDebateNewClaims(output.Output.NewClaims, maxNewClaims)
		metadata := map[string]any{"round": round, "taskId": result.Receipt.TaskID}
		if truncated > 0 {
			metadata["truncatedNewClaims"] = truncated
			metadata["maxNewClaims"] = maxNewClaims
		}
		entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind: EvidenceKindDebateRoundOutput, Source: EvidenceSourceWorker, ProducerID: participantID, ProducerRole: "participant", Summary: output.Output.Summary, Artifact: result.Awaited.Artifact, Metadata: metadata,
		})
		if err != nil {
			return nil, nil, 0, false, err
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
			if claimID != "" {
				participant.NewClaimIDs = append(participant.NewClaimIDs, claimID)
				roundNewClaims++
				run.metrics.ClaimsProposed++
			}
		}
		for _, judgement := range output.Output.Judgements {
			record := DebateJudgementRecord{ClaimID: strings.TrimSpace(judgement.ClaimID), Judgement: judgement.Judgement, Rationale: judgement.Rationale, MergeWithClaims: dedupeStrings(judgement.MergeWithClaims)}
			if judgement.Judgement != DebateJudgementAgree && judgement.Judgement != DebateJudgementNoChange {
				onlyAgree = false
			}
			if judgement.Judgement == DebateJudgementRevise && strings.TrimSpace(judgement.RevisedStatement) != "" {
				claims, record.RevisedClaimID = upsertDebateClaim(claims, ClaimDraft{Title: "Revision by " + participantID, Statement: judgement.RevisedStatement}, participantID, round, entry.EntryID, e.ids)
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
		claims = e.runDebateSemanticDedup(ctx, request, run, claims, round)
	}
	return claims, rounds, roundNewClaims, onlyAgree, nil
}

func (e *Engine) executeResumedDebateVote(ctx context.Context, request StartRequest, run *workflowRun, claims []DebateClaim, rounds []DebateRoundRecord, votes []DebateVoteRecord) ([]DebateVoteRecord, *FreeDebateResultSection, error) {
	activeClaims := activeDebateClaims(claims)
	tasks := make([]Task, 0, len(request.Roles.Participants))
	for _, participantID := range request.Roles.Participants {
		tasks = append(tasks, FinalVoteTask{TaskMeta: TaskMeta{SessionID: run.sessionID, RequestID: request.RequestID, AgentID: participantID, Role: "participant"}, TaskSpec: request.TaskSpec, Round: len(rounds), Claims: activeClaims})
	}
	voters := make([]string, 0, len(request.Roles.Participants))
	for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "final_vote", len(rounds), "该参与者未投票，claim 接受判定仅基于成功投票的参与者"))
			continue
		}
		participantID := result.Task.Meta().AgentID
		output, ok := result.Awaited.Output.(FinalVoteTaskResult)
		if !ok {
			return nil, nil, fmt.Errorf("final vote task returned unexpected result type")
		}
		if err := ValidateDebateVotes(activeClaims, output.Output.Votes); err != nil {
			run.addDegradation(Degradation{Kind: DegradationParticipantAbsent, Phase: "final_vote", Round: len(rounds), AgentID: participantID, Reason: err.Error(), Impact: "该参与者提交的 ballot 不完整或无效，整张 ballot 已拒绝且不计入 voters"})
			continue
		}
		voters = appendUnique(voters, participantID)
		for _, draft := range output.Output.Votes {
			entry, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				ClaimID: draft.ClaimID, Kind: EvidenceKindDebateVoteCast, Source: EvidenceSourceWorker, ProducerID: participantID, ProducerRole: "participant", Summary: output.Output.Summary, Artifact: result.Awaited.Artifact,
				Metadata: map[string]any{"taskId": result.Receipt.TaskID, "vote": draft.Vote, "confidence": *draft.Confidence, "rationale": draft.Rationale},
			})
			if err != nil {
				return nil, nil, err
			}
			votes = append(votes, DebateVoteRecord{ClaimID: draft.ClaimID, AgentID: participantID, Vote: draft.Vote, Confidence: *draft.Confidence, Rationale: draft.Rationale, EvidenceRef: entry.EntryID})
		}
	}
	section, failures := restoredFreeDebateSectionWithFailures(request, claims, rounds, votes)
	if len(failures) > 0 {
		details := make([]string, 0, len(failures))
		metadata := make([]map[string]any, 0, len(failures))
		for _, failure := range failures {
			details = append(details, fmt.Sprintf("%s=%d/%d", failure.ClaimID, failure.VoteCount, failure.RequiredVotes))
			metadata = append(metadata, map[string]any{"claimId": failure.ClaimID, "voteCount": failure.VoteCount, "requiredVotes": failure.RequiredVotes})
		}
		impact := fmt.Sprintf("active claims 未分别满足 quorum %.2f（%s），outcome 降级为 %s", request.DebatePolicy.VoteQuorum, strings.Join(details, ", "), FreeDebateOutcomeQuorumNotMet)
		run.addDegradation(Degradation{Kind: DegradationQuorumNotMet, Phase: "final_vote", Round: len(rounds), Impact: impact})
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind: EvidenceKindDebateVoteQuorum, Source: EvidenceSourceCoordinator, ProducerRole: "coordinator", Summary: impact,
			Metadata: map[string]any{"voteQuorum": request.DebatePolicy.VoteQuorum, "voters": section.Voters, "absentVoters": section.AbsentVoters, "participants": len(request.Roles.Participants), "claimQuorum": metadata},
		}); err != nil {
			return nil, nil, err
		}
	}
	return votes, section, nil
}

func restoredFreeDebateSection(request StartRequest, claims []DebateClaim, rounds []DebateRoundRecord, votes []DebateVoteRecord) *FreeDebateResultSection {
	section, _ := restoredFreeDebateSectionWithFailures(request, claims, rounds, votes)
	return section
}

func restoredFreeDebateSectionWithFailures(request StartRequest, claims []DebateClaim, rounds []DebateRoundRecord, votes []DebateVoteRecord) (*FreeDebateResultSection, []debateClaimQuorumFailure) {
	activeClaims := activeDebateClaims(claims)
	resolutions, outcome := resolveDebateClaims(activeClaims, claims, votes, request.DebatePolicy)
	voters := make([]string, 0, len(request.Roles.Participants))
	for _, vote := range votes {
		voters = appendUnique(voters, vote.AgentID)
	}
	absentVoters := make([]string, 0)
	for _, participantID := range request.Roles.Participants {
		if !slices.Contains(voters, participantID) {
			absentVoters = append(absentVoters, participantID)
		}
	}
	var failures []debateClaimQuorumFailure
	if quorum := request.DebatePolicy.VoteQuorum; quorum > 0 && len(request.Roles.Participants) > 0 {
		resolutions, failures = enforceDebateClaimQuorum(resolutions, activeClaims, votes, request.Roles.Participants, quorum)
		if len(failures) > 0 {
			outcome = FreeDebateOutcomeQuorumNotMet
		}
	}
	return &FreeDebateResultSection{Outcome: outcome, Rounds: rounds, Claims: claims, ClaimResolutions: resolutions, Votes: votes, Voters: voters, AbsentVoters: absentVoters, BallotSize: len(activeClaims)}, failures
}

func (e *Engine) finishResumedFreeDebate(ctx context.Context, request StartRequest, run *workflowRun, snapshot SessionSnapshot, section *FreeDebateResultSection, step string) (*RunResult, error) {
	report := derefReport(snapshot.Report)
	if freeDebateCheckpointRank(step) < freeDebateCheckpointRank(checkpointStepReportComplete) {
		if run.state.Current() != SessionPhaseReport {
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseReport, nil, nil, run.ledgerCursor); err != nil {
				return nil, err
			}
		}
		var artifact *ArtifactRef
		var err error
		report = buildFreeDebateReport(request, section)
		report, artifact, err = e.composeModeReport(ctx, request, run, WorkflowModeFreeDebate, report, TaskVerdictFromDebateOutcome(section.Outcome), map[string]any{"freeDebate": section})
		if err != nil {
			return nil, err
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{Kind: EvidenceKindReportGenerated, Source: EvidenceSourceReporter, ProducerID: request.Roles.Reporter, ProducerRole: "reporter", Summary: report.Summary, Artifact: artifact}); err != nil {
			return nil, err
		}
		if err := e.saveFreeDebateCheckpoint(ctx, run, SessionPhaseReport, checkpointStepReportComplete, checkpointDebateRound(snapshot.Checkpoint, section.Rounds), section.Claims, section.Rounds, section.Votes, &report); err != nil {
			return nil, err
		}
	}
	result := &RunResult{SchemaVersion: SchemaVersion, Mode: WorkflowModeFreeDebate, RequestID: request.RequestID, SessionID: run.sessionID, Lineage: request.Lineage, TaskSpec: request.TaskSpec, Report: report, Action: cloneAction(snapshot.Action), Metrics: finalizeMetrics(run.metrics, run.startedAt, e.clock), Degradations: run.degradations, FreeDebate: section}
	return e.finalizeWorkflow(ctx, request, run, result)
}

func inferredModeCheckpointStep(snapshot SessionSnapshot) string {
	if snapshot.Checkpoint == nil {
		return ""
	}
	if snapshot.Checkpoint.Step != "" {
		return snapshot.Checkpoint.Step
	}
	switch snapshot.Checkpoint.LastCompletedPhase {
	case SessionPhaseInitial:
		return checkpointStepDebateInitialComplete
	case SessionPhaseDebate:
		return checkpointStepDebateRoundComplete
	case SessionPhaseFinalVote:
		return checkpointStepDebateVoteComplete
	case SessionPhaseDelphiQuestionnaire, SessionPhaseDelphiRevision:
		return checkpointStepDelphiRoundComplete
	case SessionPhaseReport:
		return checkpointStepReportComplete
	case SessionPhaseAction:
		return checkpointStepActionComplete
	case SessionPhaseObserve:
		return checkpointStepObserveComplete
	default:
		return ""
	}
}

func freeDebateCheckpointRank(step string) int {
	switch step {
	case checkpointStepDebateInitialComplete:
		return 1
	case checkpointStepDebateRoundComplete:
		return 2
	case checkpointStepDebateRoundsComplete:
		return 3
	case checkpointStepDebateSynthesisComplete:
		return 4
	case checkpointStepDebateVoteComplete:
		return 5
	case checkpointStepReportComplete:
		return 6
	case checkpointStepActionComplete:
		return 7
	case checkpointStepObserveComplete:
		return 8
	default:
		return 0
	}
}

func checkpointDebateRound(checkpoint *SessionCheckpoint, rounds []DebateRoundRecord) int {
	if checkpoint != nil && checkpoint.DebateRound > 0 {
		return checkpoint.DebateRound
	}
	last := 0
	for _, round := range rounds {
		if round.Phase == "debate" && round.Round > last {
			last = round.Round
		}
	}
	return last
}

func lastDelphiRound(rounds []DelphiRoundRecord) int {
	if len(rounds) == 0 {
		return 0
	}
	return rounds[len(rounds)-1].Round
}

type delphiResumeProgress struct {
	statements            []DelphiStatement
	distributions         map[string][]float64
	rounds                []DelphiRoundRecord
	lastSummary           string
	stableRounds          int
	lastStableStatementID string
	lastResponseQuorumMet bool
	lastMinResponsesMet   bool
	recommendation        string
	dissent               []string
	converged             bool
}

func (e *Engine) resumeDelphi(ctx context.Context, request StartRequest, snapshot SessionSnapshot) (_ *RunResult, err error) {
	run, err := e.restoreModeRun(ctx, snapshot, request)
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
	return e.continueDelphi(ctx, request, run, snapshot)
}

func (e *Engine) continueDelphi(ctx context.Context, request StartRequest, run *workflowRun, snapshot SessionSnapshot) (*RunResult, error) {
	progress := delphiResumeProgress{
		statements:            cloneDelphiStatements(snapshot.DelphiStatements),
		distributions:         cloneRatingDistributions(snapshot.DelphiRatingDistributions),
		rounds:                append([]DelphiRoundRecord(nil), snapshot.DelphiRounds...),
		stableRounds:          snapshot.DelphiStableRounds,
		lastStableStatementID: snapshot.DelphiLastStableStatementID,
		lastResponseQuorumMet: snapshot.DelphiResponseQuorumMet,
		lastMinResponsesMet:   snapshot.DelphiMinResponsesMet,
		recommendation:        snapshot.DelphiRecommendation,
		dissent:               append([]string(nil), snapshot.DelphiDissentSummary...),
		converged:             snapshot.DelphiConverged,
	}
	if len(progress.rounds) > 0 {
		progress.lastSummary = progress.rounds[len(progress.rounds)-1].Summary
	}
	if progress.lastStableStatementID == "" && snapshot.Checkpoint != nil {
		progress.lastStableStatementID = snapshot.Checkpoint.DelphiStableStatementID
	}
	step := inferredModeCheckpointStep(snapshot)
	rank := delphiCheckpointRank(step)
	lastRound := lastDelphiRound(progress.rounds)
	if rank == 0 {
		if err := e.resumeModePrelude(ctx, request, run); err != nil {
			return nil, err
		}
	}

	if rank < delphiCheckpointRank(checkpointStepDelphiRoundsComplete) {
		nextRound := 1
		if step == checkpointStepDelphiRoundComplete && snapshot.Checkpoint != nil {
			nextRound = snapshot.Checkpoint.DelphiRound + 1
		}
		for round := nextRound; round <= request.DelphiPolicy.MaxRounds; round++ {
			phase := SessionPhaseDelphiQuestionnaire
			if round > 1 {
				if run.state.Current() != SessionPhaseDelphiSummary {
					if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseDelphiSummary, nil, nil, run.ledgerCursor); err != nil {
						return nil, err
					}
				}
				phase = SessionPhaseDelphiRevision
			}
			if run.state.Current() != phase {
				if err := e.advancePhase(ctx, request, run.sessionID, run.state, phase, nil, nil, run.ledgerCursor); err != nil {
					return nil, err
				}
			}
			var err error
			progress, err = e.executeResumedDelphiRound(ctx, request, run, progress, round, phase)
			if err != nil {
				return nil, err
			}
			lastRound = round
			roundStep := checkpointStepDelphiRoundComplete
			if progress.converged || round == request.DelphiPolicy.MaxRounds {
				roundStep = checkpointStepDelphiRoundsComplete
			}
			if !progress.converged && round == request.DelphiPolicy.MaxRounds {
				progress = finalizeNonConvergedDelphiProgress(progress)
			}
			section := delphiSectionFromProgress(progress)
			if err := e.saveDelphiCheckpoint(ctx, run, phase, roundStep, round, progress.lastStableStatementID, section, nil); err != nil {
				return nil, err
			}
			step = roundStep
			if progress.converged {
				break
			}
		}
		if step != checkpointStepDelphiRoundsComplete {
			progress = finalizeNonConvergedDelphiProgress(progress)
			section := delphiSectionFromProgress(progress)
			if err := e.saveDelphiCheckpoint(ctx, run, run.state.Current(), checkpointStepDelphiRoundsComplete, lastRound, progress.lastStableStatementID, section, nil); err != nil {
				return nil, err
			}
			step = checkpointStepDelphiRoundsComplete
		}
	}

	section := delphiSectionFromProgress(progress)
	if delphiCheckpointRank(step) >= delphiCheckpointRank(checkpointStepReportComplete) {
		result := &RunResult{
			SchemaVersion: SchemaVersion, Mode: WorkflowModeDelphi, RequestID: request.RequestID, SessionID: run.sessionID, Lineage: request.Lineage, TaskSpec: request.TaskSpec,
			Report: derefReport(snapshot.Report), Action: cloneAction(snapshot.Action), Metrics: finalizeMetrics(run.metrics, run.startedAt, e.clock), Degradations: run.degradations, Delphi: section,
		}
		return e.finalizeWorkflow(ctx, request, run, result)
	}
	return e.finishDelphi(ctx, request, run, section, progress.lastStableStatementID)
}

func (e *Engine) executeResumedDelphiRound(ctx context.Context, request StartRequest, run *workflowRun, progress delphiResumeProgress, round int, phase SessionPhase) (delphiResumeProgress, error) {
	if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind: EvidenceKindDelphiRoundOpened, Source: EvidenceSourceCoordinator, ProducerRole: "coordinator", Summary: "delphi round opened", Metadata: map[string]any{"round": round, "phase": phase},
	}); err != nil {
		return progress, err
	}
	ratingQuestion := delphiRatingQuestion(request)
	responses := make([]DelphiResponseRecord, 0, len(request.Roles.Participants))
	responders := make([]string, 0, len(request.Roles.Participants))
	tasks := make([]Task, 0, len(request.Roles.Participants))
	for _, participantID := range request.Roles.Participants {
		meta := TaskMeta{SessionID: run.sessionID, RequestID: request.RequestID, AgentID: participantID, Role: "participant"}
		if round == 1 {
			tasks = append(tasks, DelphiQuestionnaireTask{
				TaskMeta: meta, TaskSpec: request.TaskSpec, Round: round, Questionnaire: request.TaskSpec.Goal, RatingQuestion: ratingQuestion,
				RatingScaleMin: request.DelphiPolicy.RatingScaleMin, RatingScaleMax: request.DelphiPolicy.RatingScaleMax,
				PreviousStatements: progress.statements, PreviousSummary: progress.lastSummary,
			})
		} else {
			tasks = append(tasks, DelphiRevisionTask{
				TaskMeta: meta, TaskSpec: request.TaskSpec, Round: round, RatingQuestion: ratingQuestion,
				RatingScaleMin: request.DelphiPolicy.RatingScaleMin, RatingScaleMax: request.DelphiPolicy.RatingScaleMax,
				StatementSummaries: progress.statements, PreviousSummary: progress.lastSummary,
			})
		}
	}
	for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
		recordTaskBatchResultMetrics(&run.metrics, result)
		if result.Err != nil {
			run.addDegradation(participantAbsentDegradation(result, "delphi_round", round, "该参与者本轮未提交评分/修订，共识统计缺少其意见"))
			continue
		}
		var drafts []DelphiResponseDraft
		var summary string
		switch typed := result.Awaited.Output.(type) {
		case DelphiQuestionnaireTaskResult:
			drafts, summary = typed.Output.Responses, typed.Output.Summary
		case DelphiRevisionTaskResult:
			drafts, summary = typed.Output.Responses, typed.Output.Summary
		default:
			return progress, fmt.Errorf("delphi task returned unexpected result type")
		}
		if validationErr := ValidateDelphiResponses(drafts, request.DelphiPolicy.RatingScaleMin, request.DelphiPolicy.RatingScaleMax); validationErr != nil {
			invalid := result
			invalid.Err = fmt.Errorf("validate delphi responses: %w", validationErr)
			run.addDegradation(participantAbsentDegradation(invalid, "delphi_round", round, "该参与者本轮响应违反评分契约，整份响应未计入 quorum 或聚合"))
			continue
		}
		agentID := result.Task.Meta().AgentID
		responders = append(responders, agentID)
		for _, draft := range drafts {
			record := DelphiResponseRecord{agentID: agentID, StatementID: strings.TrimSpace(draft.StatementID), Statement: strings.TrimSpace(draft.Statement), Rating: *draft.Rating, Rationale: draft.Rationale}
			responses = append(responses, record)
			if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind: EvidenceKindDelphiResponse, Source: EvidenceSourceWorker, ProducerID: agentID, ProducerRole: "participant", Summary: summary, Artifact: result.Awaited.Artifact,
				Metadata: map[string]any{"round": round, "taskId": result.Receipt.TaskID, "statement": record.Statement, "rating": record.Rating},
			}); err != nil {
				return progress, err
			}
		}
	}
	responders = dedupeStrings(responders)
	absentResponders := make([]string, 0, len(request.Roles.Participants)-len(responders))
	for _, participantID := range request.Roles.Participants {
		if !slices.Contains(responders, participantID) {
			absentResponders = append(absentResponders, participantID)
		}
	}
	progress.statements, progress.distributions = aggregateDelphiResponses(progress.statements, responses, round, request.DelphiPolicy)
	requiredResponders := delphiRequiredResponders(len(request.Roles.Participants), request.DelphiPolicy.ResponseQuorum)
	responseQuorumMet := len(responders) >= requiredResponders
	minResponsesMet := delphiMinResponsesMet(progress.statements, request.DelphiPolicy.MinResponsesPerStatement)
	bestConsensus, _ := bestDelphiStatement(progress.statements)
	bestStatementID := ""
	if len(progress.statements) > 0 {
		bestStatementID = progress.statements[0].StatementID
	}
	stableThisRound := responseQuorumMet && minResponsesMet && bestConsensus >= request.DelphiPolicy.ConvergenceThreshold
	if !stableThisRound {
		progress.stableRounds = 0
		progress.lastStableStatementID = ""
	} else if bestStatementID == progress.lastStableStatementID {
		progress.stableRounds++
	} else {
		progress.stableRounds = 1
		progress.lastStableStatementID = bestStatementID
	}
	progress.lastResponseQuorumMet = responseQuorumMet
	progress.lastMinResponsesMet = minResponsesMet
	if !responseQuorumMet {
		impact := fmt.Sprintf("Delphi 第 %d 轮有效响应者 %d/%d，低于 response_quorum %.2f 要求的 %d，不能计为稳定收敛轮", round, len(responders), len(request.Roles.Participants), request.DelphiPolicy.ResponseQuorum, requiredResponders)
		run.addDegradation(Degradation{Kind: DegradationQuorumNotMet, Phase: "delphi_round", Round: round, Impact: impact})
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind: EvidenceKindDelphiResponseQuorum, Source: EvidenceSourceCoordinator, ProducerRole: "coordinator", Summary: impact,
			Metadata: map[string]any{"round": round, "responseQuorum": request.DelphiPolicy.ResponseQuorum, "responderCount": len(responders), "absentResponderCount": len(absentResponders), "requiredResponders": requiredResponders},
		}); err != nil {
			return progress, err
		}
	}
	if !minResponsesMet {
		counts := make([]string, 0, len(progress.statements))
		for _, statement := range progress.statements {
			if statement.ResponseCount < request.DelphiPolicy.MinResponsesPerStatement {
				counts = append(counts, fmt.Sprintf("%s=%d", statement.StatementID, statement.ResponseCount))
			}
		}
		impact := fmt.Sprintf("Delphi 第 %d 轮至少一个 active statement 未达到最小独立响应数 %d（%s），不能计为稳定收敛轮", round, request.DelphiPolicy.MinResponsesPerStatement, strings.Join(counts, ", "))
		run.addDegradation(Degradation{Kind: DegradationDelphiSampleNotMet, Phase: "delphi_round", Round: round, Impact: impact})
	}
	var artifact *ArtifactRef
	var err error
	progress.lastSummary, progress.recommendation, progress.dissent, artifact, err = e.buildDelphiSummary(ctx, request, run, round, progress.statements)
	if err != nil {
		return progress, err
	}
	if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
		Kind: EvidenceKindDelphiRoundSummary, Source: EvidenceSourceCoordinator, ProducerID: request.Roles.Facilitator, ProducerRole: "facilitator", Summary: progress.lastSummary, Artifact: artifact,
		Metadata: map[string]any{"round": round, "recommendation": progress.recommendation, "responseQuorumMet": responseQuorumMet, "minResponsesMet": minResponsesMet, "stableRounds": progress.stableRounds},
	}); err != nil {
		return progress, err
	}
	progress.rounds = append(progress.rounds, DelphiRoundRecord{
		Round: round, Phase: string(phase), Summary: progress.lastSummary, Responses: responses, Statements: cloneDelphiStatements(progress.statements),
		ResponderCount: len(responders), AbsentResponderCount: len(absentResponders), ResponseQuorumMet: responseQuorumMet, MinResponsesMet: minResponsesMet, StableRounds: progress.stableRounds,
	})
	progress.converged = round >= request.DelphiPolicy.MinRounds && progress.stableRounds >= request.DelphiPolicy.StabilityRounds
	if progress.converged {
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind: EvidenceKindDelphiConvergence, Source: EvidenceSourceCoordinator, ProducerRole: "coordinator", Summary: "delphi sample, dispersion, and stability requirements reached",
			Metadata: map[string]any{"round": round, "consensusLevel": bestConsensus, "responseQuorumMet": responseQuorumMet, "minResponsesMet": minResponsesMet, "stableRounds": progress.stableRounds},
		}); err != nil {
			return progress, err
		}
	}
	return progress, nil
}

func delphiSectionFromProgress(progress delphiResumeProgress) *DelphiResultSection {
	bestConsensus, recommendation := bestDelphiStatement(progress.statements)
	if strings.TrimSpace(progress.recommendation) != "" {
		recommendation = progress.recommendation
	}
	dissent := progress.dissent
	if dissent == nil {
		dissent = buildDelphiDissentSummary(progress.statements)
	}
	return &DelphiResultSection{
		Rounds: progress.rounds, Statements: progress.statements, RatingDistributions: progress.distributions, ConsensusLevel: bestConsensus,
		Converged: progress.converged, ResponseQuorumMet: progress.lastResponseQuorumMet, MinResponsesMet: progress.lastMinResponsesMet,
		StableRounds: progress.stableRounds, Recommendation: recommendation, DissentSummary: dissent,
	}
}

func finalizeNonConvergedDelphiProgress(progress delphiResumeProgress) delphiResumeProgress {
	progress.converged = false
	_, progress.recommendation = bestDelphiStatement(progress.statements)
	progress.dissent = buildDelphiDissentSummary(progress.statements)
	return progress
}

func delphiCheckpointRank(step string) int {
	switch step {
	case checkpointStepDelphiRoundComplete:
		return 1
	case checkpointStepDelphiRoundsComplete:
		return 2
	case checkpointStepReportComplete:
		return 3
	case checkpointStepActionComplete:
		return 4
	case checkpointStepObserveComplete:
		return 5
	default:
		return 0
	}
}

func (e *Engine) resumeModePrelude(ctx context.Context, request StartRequest, run *workflowRun) error {
	if run.state.Current() == SessionPhaseCreated {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseFrame, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return err
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind: EvidenceKindCaseFramed, Source: EvidenceSourceCoordinator, ProducerRole: "coordinator", Summary: run.manifest.CanonicalProblemStatement,
			SourceType: "case_manifest", ProvenanceQuality: ProvenanceQualityHigh, FirstHandVsSecondHand: EvidencePerspectiveFirstHand,
			Metadata: map[string]any{
				"caseId": run.manifest.CaseID, "taskType": run.manifest.TaskType, "riskLevel": run.manifest.RiskLevel,
				"requiredEvidenceLevel": run.manifest.RequiredEvidenceLevel, "outOfScope": run.manifest.OutOfScope, "unresolvedQuestions": run.manifest.UnresolvedQuestions,
			},
		}); err != nil {
			return err
		}
	}
	if run.state.Current() == SessionPhaseFrame {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseIngest, run.claimGraph, run.challengeTickets, run.ledgerCursor); err != nil {
			return err
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind: EvidenceKindTaskIngested, Source: EvidenceSourceCoordinator, ProducerRole: "coordinator", Summary: "task ingested",
			Metadata: map[string]any{"goal": request.TaskSpec.Goal, "mode": request.Mode, "successCriteria": request.TaskSpec.SuccessCriteria, "allowedTools": request.TaskSpec.AllowedTools, "workspaceSnapshot": request.TaskSpec.WorkspaceSnapshot},
		}); err != nil {
			return err
		}
		for _, material := range request.TaskSpec.Materials {
			summary := firstNonEmpty(material.Title, material.ID, material.Path, "source material")
			excerpt := strings.TrimSpace(material.Content)
			if len(excerpt) > 240 {
				excerpt = excerpt[:240] + "..."
			}
			if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind: EvidenceKindSourceMaterial, Source: EvidenceSourceCoordinator, ProducerRole: "coordinator", SourceType: firstNonEmpty(material.Kind, "material"),
				SourceLocator: firstNonEmpty(material.Path, material.ID), Summary: summary, ContentExcerpt: excerpt,
				ProvenanceQuality: ProvenanceQualityMedium, FirstHandVsSecondHand: EvidencePerspectiveSecondHand,
				Metadata: map[string]any{"id": material.ID, "title": material.Title, "hash": material.Hash},
			}); err != nil {
				return err
			}
		}
		if _, err := e.runIngestSources(ctx, request, run, "resume-initial"); err != nil {
			return err
		}
	}
	if run.state.Current() != SessionPhaseIngest {
		return fmt.Errorf("session %s cannot resume workflow prelude from phase %s", run.sessionID, run.state.Current())
	}
	return e.patchRunState(ctx, run, SessionPatch{Phase: ptr(SessionPhaseIngest), Checkpoint: &SessionCheckpoint{Mode: request.Mode, LastCompletedPhase: SessionPhaseIngest}})
}
