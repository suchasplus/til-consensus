package consensus

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
)

func (e *Engine) startDelphi(ctx context.Context, request StartRequest) (_ *RunResult, err error) {
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

	statements := make([]DelphiStatement, 0)
	distributions := map[string][]float64{}
	rounds := make([]DelphiRoundRecord, 0, request.DelphiPolicy.MaxRounds*2)
	lastSummary := ""
	stableRounds := 0
	lastStableStatementID := ""
	lastResponseQuorumMet := false
	lastMinResponsesMet := false
	ratingQuestion := delphiRatingQuestion(request)
	for round := 1; round <= request.DelphiPolicy.MaxRounds; round++ {
		var phase SessionPhase
		if round == 1 {
			phase = SessionPhaseDelphiQuestionnaire
		} else {
			phase = SessionPhaseDelphiRevision
		}
		if run.state.Current() != phase {
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, phase, nil, nil, run.ledgerCursor); err != nil {
				return nil, err
			}
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDelphiRoundOpened,
			Source:       EvidenceSourceCoordinator,
			ProducerRole: "coordinator",
			Summary:      "delphi round opened",
			Metadata: map[string]any{
				"round": round,
				"phase": phase,
			},
		}); err != nil {
			return nil, err
		}
		responses := make([]DelphiResponseRecord, 0, len(request.Roles.Participants))
		responders := make([]string, 0, len(request.Roles.Participants))
		tasks := make([]Task, 0, len(request.Roles.Participants))
		for _, participantID := range request.Roles.Participants {
			if round == 1 {
				tasks = append(tasks, DelphiQuestionnaireTask{
					TaskMeta: TaskMeta{
						SessionID: run.sessionID,
						RequestID: request.RequestID,
						AgentID:   participantID,
						Role:      "participant",
					},
					TaskSpec:           request.TaskSpec,
					Round:              round,
					Questionnaire:      request.TaskSpec.Goal,
					RatingQuestion:     ratingQuestion,
					RatingScaleMin:     request.DelphiPolicy.RatingScaleMin,
					RatingScaleMax:     request.DelphiPolicy.RatingScaleMax,
					PreviousStatements: statements,
					PreviousSummary:    lastSummary,
				})
			} else {
				tasks = append(tasks, DelphiRevisionTask{
					TaskMeta: TaskMeta{
						SessionID: run.sessionID,
						RequestID: request.RequestID,
						AgentID:   participantID,
						Role:      "participant",
					},
					TaskSpec:           request.TaskSpec,
					Round:              round,
					RatingQuestion:     ratingQuestion,
					RatingScaleMin:     request.DelphiPolicy.RatingScaleMin,
					RatingScaleMax:     request.DelphiPolicy.RatingScaleMax,
					StatementSummaries: statements,
					PreviousSummary:    lastSummary,
				})
			}
		}
		for _, result := range e.executeTaskBatch(ctx, request, run.sessionID, tasks, run.startedAt, request.WaitingPolicy.PerTaskTimeout, len(tasks)) {
			recordTaskBatchResultMetrics(&run.metrics, result)
			if result.Err != nil {
				run.addDegradation(participantAbsentDegradation(result, "delphi_round", round, "该参与者本轮未提交评分/修订，共识统计缺少其意见"))
				continue
			}
			var outputResponses []DelphiResponseDraft
			var summary string
			switch typed := result.Awaited.Output.(type) {
			case DelphiQuestionnaireTaskResult:
				outputResponses = typed.Output.Responses
				summary = typed.Output.Summary
			case DelphiRevisionTaskResult:
				outputResponses = typed.Output.Responses
				summary = typed.Output.Summary
			default:
				return nil, fmt.Errorf("delphi task returned unexpected result type")
			}
			if validationErr := ValidateDelphiResponses(outputResponses, request.DelphiPolicy.RatingScaleMin, request.DelphiPolicy.RatingScaleMax); validationErr != nil {
				invalidResult := result
				invalidResult.Err = fmt.Errorf("validate delphi responses: %w", validationErr)
				run.addDegradation(participantAbsentDegradation(invalidResult, "delphi_round", round, "该参与者本轮响应违反评分契约，整份响应未计入 quorum 或聚合"))
				continue
			}
			agentID := result.Task.Meta().AgentID
			responders = append(responders, agentID)
			for _, draft := range outputResponses {
				record := DelphiResponseRecord{
					agentID:     agentID,
					StatementID: strings.TrimSpace(draft.StatementID),
					Statement:   strings.TrimSpace(draft.Statement),
					Rating:      *draft.Rating,
					Rationale:   draft.Rationale,
				}
				responses = append(responses, record)
				if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
					Kind:         EvidenceKindDelphiResponse,
					Source:       EvidenceSourceWorker,
					ProducerRole: "participant",
					Summary:      summary,
					Artifact:     result.Awaited.Artifact,
					Metadata: map[string]any{
						"round":     round,
						"taskId":    result.Receipt.TaskID,
						"statement": record.Statement,
						"rating":    record.Rating,
					},
				}); err != nil {
					return nil, err
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
		statements, distributions = aggregateDelphiResponses(statements, responses, round, request.DelphiPolicy)
		requiredResponders := delphiRequiredResponders(len(request.Roles.Participants), request.DelphiPolicy.ResponseQuorum)
		responseQuorumMet := len(responders) >= requiredResponders
		minResponsesMet := delphiMinResponsesMet(statements, request.DelphiPolicy.MinResponsesPerStatement)
		bestConsensus, _ := bestDelphiStatement(statements)
		bestStatementID := ""
		if len(statements) > 0 {
			bestStatementID = statements[0].StatementID
		}
		stableThisRound := responseQuorumMet && minResponsesMet && bestConsensus >= request.DelphiPolicy.ConvergenceThreshold
		if !stableThisRound {
			stableRounds = 0
			lastStableStatementID = ""
		} else if bestStatementID == lastStableStatementID {
			stableRounds++
		} else {
			stableRounds = 1
			lastStableStatementID = bestStatementID
		}
		lastResponseQuorumMet = responseQuorumMet
		lastMinResponsesMet = minResponsesMet
		if !responseQuorumMet {
			impact := fmt.Sprintf("Delphi 第 %d 轮有效响应者 %d/%d，低于 response_quorum %.2f 要求的 %d，不能计为稳定收敛轮", round, len(responders), len(request.Roles.Participants), request.DelphiPolicy.ResponseQuorum, requiredResponders)
			run.addDegradation(Degradation{Kind: DegradationQuorumNotMet, Phase: "delphi_round", Round: round, Impact: impact})
			if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindDelphiResponseQuorum,
				Source:       EvidenceSourceCoordinator,
				ProducerRole: "coordinator",
				Summary:      impact,
				Metadata: map[string]any{
					"round": round, "responseQuorum": request.DelphiPolicy.ResponseQuorum,
					"responderCount": len(responders), "absentResponderCount": len(absentResponders),
					"requiredResponders": requiredResponders,
				},
			}); err != nil {
				return nil, err
			}
		}
		if !minResponsesMet {
			counts := make([]string, 0, len(statements))
			for _, statement := range statements {
				if statement.ResponseCount < request.DelphiPolicy.MinResponsesPerStatement {
					counts = append(counts, fmt.Sprintf("%s=%d", statement.StatementID, statement.ResponseCount))
				}
			}
			impact := fmt.Sprintf("Delphi 第 %d 轮至少一个 active statement 未达到最小独立响应数 %d（%s），不能计为稳定收敛轮", round, request.DelphiPolicy.MinResponsesPerStatement, strings.Join(counts, ", "))
			run.addDegradation(Degradation{Kind: DegradationDelphiSampleNotMet, Phase: "delphi_round", Round: round, Impact: impact})
		}
		lastSummary, recommendation, dissent, facilitatorArtifact, err := e.buildDelphiSummary(ctx, request, run, round, statements)
		if err != nil {
			return nil, err
		}
		if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
			Kind:         EvidenceKindDelphiRoundSummary,
			Source:       EvidenceSourceCoordinator,
			ProducerID:   request.Roles.Facilitator,
			ProducerRole: "facilitator",
			Summary:      lastSummary,
			Artifact:     facilitatorArtifact,
			Metadata: map[string]any{
				"round":             round,
				"recommendation":    recommendation,
				"responseQuorumMet": responseQuorumMet,
				"minResponsesMet":   minResponsesMet,
				"stableRounds":      stableRounds,
			},
		}); err != nil {
			return nil, err
		}
		rounds = append(rounds, DelphiRoundRecord{
			Round:                round,
			Phase:                string(phase),
			Summary:              lastSummary,
			Responses:            responses,
			Statements:           cloneDelphiStatements(statements),
			ResponderCount:       len(responders),
			AbsentResponderCount: len(absentResponders),
			ResponseQuorumMet:    responseQuorumMet,
			MinResponsesMet:      minResponsesMet,
			StableRounds:         stableRounds,
		})
		converged := round >= request.DelphiPolicy.MinRounds && stableRounds >= request.DelphiPolicy.StabilityRounds
		checkpointSection := &DelphiResultSection{
			Rounds:              rounds,
			Statements:          statements,
			RatingDistributions: distributions,
			ConsensusLevel:      bestConsensus,
			Converged:           converged,
			ResponseQuorumMet:   responseQuorumMet,
			MinResponsesMet:     minResponsesMet,
			StableRounds:        stableRounds,
			Recommendation:      recommendation,
			DissentSummary:      dissent,
		}
		checkpointStep := checkpointStepDelphiRoundComplete
		if converged || round == request.DelphiPolicy.MaxRounds {
			checkpointStep = checkpointStepDelphiRoundsComplete
		}
		if !converged && round == request.DelphiPolicy.MaxRounds {
			checkpointSection.ConsensusLevel, checkpointSection.Recommendation = bestDelphiStatement(statements)
			checkpointSection.DissentSummary = buildDelphiDissentSummary(statements)
		}
		if converged {
			if _, err := e.appendEvidence(ctx, request, run.sessionID, &run.ledgerEntries, &run.ledgerCursor, EvidenceRecord{
				Kind:         EvidenceKindDelphiConvergence,
				Source:       EvidenceSourceCoordinator,
				ProducerRole: "coordinator",
				Summary:      "delphi sample, dispersion, and stability requirements reached",
				Metadata: map[string]any{
					"round": round, "consensusLevel": bestConsensus,
					"responseQuorumMet": responseQuorumMet, "minResponsesMet": minResponsesMet,
					"stableRounds": stableRounds,
				},
			}); err != nil {
				return nil, err
			}
		}
		if err := e.saveDelphiCheckpoint(ctx, run, phase, checkpointStep, round, lastStableStatementID, checkpointSection, nil); err != nil {
			return nil, err
		}
		if converged {
			return e.finishDelphi(ctx, request, run, checkpointSection, lastStableStatementID)
		}
		if round < request.DelphiPolicy.MaxRounds {
			if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseDelphiSummary, nil, nil, run.ledgerCursor); err != nil {
				return nil, err
			}
		}
	}
	bestConsensus, recommendation := bestDelphiStatement(statements)
	section := &DelphiResultSection{
		Rounds:              rounds,
		Statements:          statements,
		RatingDistributions: distributions,
		ConsensusLevel:      bestConsensus,
		Converged:           false,
		ResponseQuorumMet:   lastResponseQuorumMet,
		MinResponsesMet:     lastMinResponsesMet,
		StableRounds:        stableRounds,
		Recommendation:      recommendation,
		DissentSummary:      buildDelphiDissentSummary(statements),
	}
	return e.finishDelphi(ctx, request, run, section, lastStableStatementID)
}

func (e *Engine) finishDelphi(ctx context.Context, request StartRequest, run *workflowRun, section *DelphiResultSection, stableStatementID string) (*RunResult, error) {
	if run.state.Current() != SessionPhaseReport {
		if err := e.advancePhase(ctx, request, run.sessionID, run.state, SessionPhaseReport, nil, nil, run.ledgerCursor); err != nil {
			return nil, err
		}
	}
	report := buildDelphiReport(request, section)
	report, artifact, err := e.composeModeReport(ctx, request, run, WorkflowModeDelphi, report, TaskVerdictFromDelphi(section), map[string]any{
		"delphi": section,
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
		Artifact:     artifact,
	}); err != nil {
		return nil, err
	}
	result := &RunResult{
		SchemaVersion: SchemaVersion,
		Mode:          WorkflowModeDelphi,
		RequestID:     request.RequestID,
		SessionID:     run.sessionID,
		Lineage:       request.Lineage,
		TaskSpec:      request.TaskSpec,
		Report:        report,
		Metrics:       finalizeMetrics(run.metrics, run.startedAt, e.clock),
		Degradations:  run.degradations,
		Delphi:        section,
	}
	if err := e.saveDelphiCheckpoint(ctx, run, SessionPhaseReport, checkpointStepReportComplete, lastDelphiRound(section.Rounds), stableStatementID, section, &report); err != nil {
		return nil, err
	}
	return e.finalizeWorkflow(ctx, request, run, result)
}

func aggregateDelphiResponses(existing []DelphiStatement, responses []DelphiResponseRecord, round int, policy DelphiPolicy) ([]DelphiStatement, map[string][]float64) {
	type aggregate struct {
		statement  string
		ratings    []float64
		reasons    []string
		responders map[string]struct{}
	}
	index := map[string]int{}
	for idx := range existing {
		index[existing[idx].StatementID] = idx
	}
	aggregates := map[string]*aggregate{}
	for responseIndex, response := range responses {
		statementID := strings.TrimSpace(response.StatementID)
		statementText := strings.TrimSpace(response.Statement)
		if statementID == "" {
			statementID = findDelphiStatementID(existing, statementText)
		}
		if statementID == "" {
			statementID = "statement_" + normalizedClaimKey(statementText)
		}
		if statementText == "" {
			if idx, ok := index[statementID]; ok {
				statementText = existing[idx].Statement
			}
		}
		agg, ok := aggregates[statementID]
		if !ok {
			agg = &aggregate{statement: statementText, responders: map[string]struct{}{}}
			aggregates[statementID] = agg
		}
		responderKey := strings.TrimSpace(response.agentID)
		if responderKey == "" {
			responderKey = fmt.Sprintf("anonymous-response-%d", responseIndex)
		}
		if _, duplicate := agg.responders[responderKey]; duplicate {
			continue
		}
		agg.responders[responderKey] = struct{}{}
		agg.ratings = append(agg.ratings, response.Rating)
		if reason := strings.TrimSpace(response.Rationale); reason != "" {
			agg.reasons = append(agg.reasons, reason)
		}
	}
	next := make([]DelphiStatement, 0, len(aggregates))
	distributions := map[string][]float64{}
	for statementID, agg := range aggregates {
		mean := average(agg.ratings)
		consensus := consensusFromRatings(agg.ratings, policy.RatingScaleMin, policy.RatingScaleMax)
		distributions[statementID] = append([]float64(nil), agg.ratings...)
		next = append(next, DelphiStatement{
			StatementID:           statementID,
			Statement:             agg.statement,
			MeanRating:            mean,
			ConsensusLevel:        consensus,
			ResponseCount:         len(agg.ratings),
			LastRound:             round,
			RepresentativeReasons: dedupeStrings(agg.reasons)[:min(3, len(dedupeStrings(agg.reasons)))],
		})
	}
	slices.SortFunc(next, func(left, right DelphiStatement) int {
		if left.MeanRating == right.MeanRating {
			if left.ConsensusLevel == right.ConsensusLevel {
				return strings.Compare(left.StatementID, right.StatementID)
			}
			if left.ConsensusLevel > right.ConsensusLevel {
				return -1
			}
			return 1
		}
		if left.MeanRating > right.MeanRating {
			return -1
		}
		return 1
	})
	return next, distributions
}

func findDelphiStatementID(existing []DelphiStatement, statement string) string {
	key := normalizedClaimKey(statement)
	for _, item := range existing {
		if normalizedClaimKey(item.Statement) == key {
			return item.StatementID
		}
	}
	return ""
}

func consensusFromRatings(ratings []float64, scaleMin int, scaleMax int) float64 {
	// Dispersion cannot be estimated from a singleton sample. Returning zero
	// prevents a lone response from looking like perfect consensus even before
	// the explicit quorum and per-statement sample gates are applied.
	if len(ratings) < 2 {
		return 0
	}
	minRating := ratings[0]
	maxRating := ratings[0]
	for _, rating := range ratings[1:] {
		if rating < minRating {
			minRating = rating
		}
		if rating > maxRating {
			maxRating = rating
		}
	}
	span := float64(scaleMax - scaleMin)
	if span <= 0 {
		return 0
	}
	level := 1 - ((maxRating - minRating) / span)
	if level < 0 {
		return 0
	}
	if level > 1 {
		return 1
	}
	return math.Round(level*100) / 100
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return math.Round((total/float64(len(values)))*100) / 100
}

func (e *Engine) buildDelphiSummary(ctx context.Context, request StartRequest, run *workflowRun, round int, statements []DelphiStatement) (string, string, []string, *ArtifactRef, error) {
	builtinSummary := buildDelphiRoundSummary(statements)
	recommendation := ""
	if len(statements) > 0 {
		recommendation = statements[0].Statement
	}
	dissent := buildDelphiDissentSummary(statements)
	if request.Roles.Facilitator == "" || e.deps.TaskDelegate == nil {
		return builtinSummary, recommendation, dissent, nil, nil
	}
	_, awaited, err := e.executeTask(ctx, request, run.sessionID, DelphiFacilitatorSummaryTask{
		TaskMeta: TaskMeta{
			SessionID: run.sessionID,
			RequestID: request.RequestID,
			AgentID:   request.Roles.Facilitator,
			Role:      "facilitator",
		},
		TaskSpec:           request.TaskSpec,
		Round:              round,
		StatementSummaries: statements,
	}, run.startedAt, request.WaitingPolicy.PerTaskTimeout)
	run.metrics.TasksDispatched++
	if err != nil {
		if strings.Contains(err.Error(), "__timeout__") {
			run.metrics.WaitTimeouts++
		}
		return builtinSummary, recommendation, dissent, nil, nil
	}
	typed, ok := awaited.Output.(DelphiFacilitatorSummaryTaskResult)
	if !ok {
		return builtinSummary, recommendation, dissent, awaited.Artifact, nil
	}
	if len(typed.Output.Statements) > 0 {
		statements = typed.Output.Statements
		builtinSummary = buildDelphiRoundSummary(statements)
		if len(statements) > 0 {
			recommendation = statements[0].Statement
		}
		dissent = buildDelphiDissentSummary(statements)
	}
	if typed.Output.Recommendation != "" {
		recommendation = typed.Output.Recommendation
	}
	if len(typed.Output.DissentSummary) > 0 {
		dissent = typed.Output.DissentSummary
	}
	return firstNonEmpty(typed.Output.Summary, builtinSummary), recommendation, dissent, awaited.Artifact, nil
}

func buildDelphiRoundSummary(statements []DelphiStatement) string {
	if len(statements) == 0 {
		return "本轮未形成有效候选结论"
	}
	parts := make([]string, 0, len(statements))
	for _, item := range statements {
		parts = append(parts, fmt.Sprintf("%s(%.2f/%.2f)", item.Statement, item.MeanRating, item.ConsensusLevel))
	}
	return strings.Join(parts, "; ")
}

func bestDelphiStatement(statements []DelphiStatement) (float64, string) {
	if len(statements) == 0 {
		return 0, ""
	}
	return statements[0].ConsensusLevel, statements[0].Statement
}

func delphiRequiredResponders(participantCount int, quorum float64) int {
	if participantCount <= 0 || quorum <= 0 {
		return 0
	}
	return int(math.Ceil(float64(participantCount) * quorum))
}

func delphiMinResponsesMet(statements []DelphiStatement, minimum int) bool {
	if len(statements) == 0 || minimum < 1 {
		return false
	}
	for _, statement := range statements {
		if statement.ResponseCount < minimum {
			return false
		}
	}
	return true
}

func delphiRatingQuestion(request StartRequest) string {
	return fmt.Sprintf(
		"请按 %d–%d 分评价每个候选陈述对任务目标“%s”的适切性、可行性与证据充分度；%d 表示最低，%d 表示最高。",
		request.DelphiPolicy.RatingScaleMin,
		request.DelphiPolicy.RatingScaleMax,
		request.TaskSpec.Goal,
		request.DelphiPolicy.RatingScaleMin,
		request.DelphiPolicy.RatingScaleMax,
	)
}

func buildDelphiDissentSummary(statements []DelphiStatement) []string {
	out := make([]string, 0)
	for _, statement := range statements {
		if statement.ConsensusLevel >= 0.7 {
			continue
		}
		out = append(out, fmt.Sprintf("%s 仍存在明显分歧", statement.Statement))
	}
	return dedupeStrings(out)
}

func buildDelphiReport(request StartRequest, section *DelphiResultSection) AdjudicationReport {
	highlights := make([]string, 0, min(3, len(section.Statements)))
	for _, item := range section.Statements[:min(3, len(section.Statements))] {
		highlights = append(highlights, fmt.Sprintf("%s: %.2f / %.2f", item.Statement, item.MeanRating, item.ConsensusLevel))
	}
	nextActions := make([]string, 0)
	if !section.Converged {
		nextActions = append(nextActions, "继续补充材料后再发起下一轮 Delphi")
	}
	return AdjudicationReport{
		Summary:     fmt.Sprintf("Delphi 推荐结论：%s", firstNonEmpty(section.Recommendation, "未形成明确推荐")),
		Highlights:  highlights,
		NextActions: nextActions,
	}
}

func TaskVerdictFromDelphi(section *DelphiResultSection) TaskVerdict {
	switch {
	case section == nil || len(section.Statements) == 0:
		return TaskVerdictFailed
	case !section.ResponseQuorumMet || !section.MinResponsesMet:
		return TaskVerdictUndetermined
	case section.Converged:
		return TaskVerdictSupported
	case section.ConsensusLevel >= 0.5:
		return TaskVerdictPartiallySupported
	default:
		return TaskVerdictUndetermined
	}
}

func cloneDelphiStatements(values []DelphiStatement) []DelphiStatement {
	out := make([]DelphiStatement, len(values))
	copy(out, values)
	return out
}
