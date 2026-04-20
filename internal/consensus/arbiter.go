package consensus

import (
	"context"
	"fmt"
	"maps"
	"math"
	"strings"
	"time"
)

type DefaultArbiterDeps struct {
	TaskDelegate   TaskDelegate
	Clock          Clock
	IDFactory      IDFactory
	PerTaskTimeout time.Duration
	RetryAttempts  int
}

type DefaultArbiter struct {
	deps DefaultArbiterDeps
}

func NewDefaultArbiter(deps DefaultArbiterDeps) *DefaultArbiter {
	if deps.Clock == nil {
		deps.Clock = SystemClock{}
	}
	return &DefaultArbiter{deps: deps}
}

func (a *DefaultArbiter) Decide(ctx context.Context, input ArbiterInput) (ArbiterReport, error) {
	if input.Request.Roles.Arbiter != "" && a.deps.TaskDelegate != nil {
		report, ok, err := a.tryDelegateArbiter(ctx, input)
		if err != nil {
			return ArbiterReport{}, err
		}
		if ok {
			return report, nil
		}
	}
	return a.ruleBasedDecision(input), nil
}

func (a *DefaultArbiter) tryDelegateArbiter(ctx context.Context, input ArbiterInput) (ArbiterReport, bool, error) {
	timeout := a.deps.PerTaskTimeout
	if timeout <= 0 {
		timeout = DefaultPerTaskTimeout
	}
	task := ArbiterTask{
		TaskMeta: TaskMeta{
			SessionID: input.SessionID,
			RequestID: input.Request.RequestID,
			AgentID:   input.Request.Roles.Arbiter,
			Role:      "arbiter",
		},
		TaskSpec:   input.Request.TaskSpec,
		Claims:     input.Claims,
		Challenges: input.Challenges,
		Ledger:     input.Ledger,
		Findings:   input.Findings,
	}
	_, awaited, _, err := ExecuteTaskWithRetry(ctx, a.deps.TaskDelegate, task, timeout, a.deps.RetryAttempts, TaskRetryHooks{})
	if err != nil {
		return ArbiterReport{}, false, fmt.Errorf("delegated arbiter failed: %w", err)
	}
	if !awaited.OK {
		return ArbiterReport{}, false, fmt.Errorf("delegated arbiter failed: %s", strings.TrimSpace(awaited.Error))
	}
	typed, ok := awaited.Output.(ArbiterTaskResult)
	if !ok {
		return ArbiterReport{}, false, fmt.Errorf("delegated arbiter returned unexpected result type")
	}
	report := ArbiterReport{
		TaskVerdict: typed.Output.TaskVerdict,
		Summary:     strings.TrimSpace(typed.Output.Summary),
		Decisions:   typed.Output.Decisions,
		Records:     typed.Output.Records,
		Metadata:    maps.Clone(typed.Output.Metadata),
	}
	if len(report.Records) == 0 {
		report.Records = deriveRecordsFromDecisions(input.Request, input.Claims, report.Decisions)
	}
	finalClaims := ApplyDecisions(append([]ClaimNode(nil), input.Claims...), report.Decisions)
	finalClaims = ApplyAdjudicationRecords(finalClaims, report.Records)
	reconciledVerdict := DetermineTaskVerdict(finalClaims)
	if report.TaskVerdict != "" && report.TaskVerdict != reconciledVerdict {
		if report.Metadata == nil {
			report.Metadata = map[string]any{}
		}
		report.Metadata["modelTaskVerdict"] = report.TaskVerdict
		report.Metadata["taskVerdictReconciled"] = true
	}
	report.TaskVerdict = reconciledVerdict
	return report, true, nil
}

func (a *DefaultArbiter) ruleBasedDecision(input ArbiterInput) ArbiterReport {
	findingMap := map[string][]VerificationResult{}
	for _, finding := range input.Findings {
		findingMap[finding.ClaimID] = append(findingMap[finding.ClaimID], finding)
	}
	challengeMap := map[string][]ChallengeTicket{}
	for _, ticket := range input.Challenges {
		challengeMap[ticket.ClaimID] = append(challengeMap[ticket.ClaimID], ticket)
	}
	evidenceMap := map[string][]EvidenceRecord{}
	for _, record := range input.Ledger {
		if strings.TrimSpace(record.ClaimID) == "" {
			continue
		}
		evidenceMap[record.ClaimID] = append(evidenceMap[record.ClaimID], record)
	}
	manifest := BuildCaseManifest(input.Request)
	decisions := make([]ArbiterDecision, 0, len(input.Claims))
	records := make([]AdjudicationRecord, 0, len(input.Claims))
	for _, claim := range input.Claims {
		decision, record := adjudicateClaim(manifest, claim, challengeMap[claim.ClaimID], findingMap[claim.ClaimID], evidenceMap[claim.ClaimID], input.Request.ArbiterPolicy.AllowUndetermined)
		decisions = append(decisions, decision)
		records = append(records, record)
	}
	claims := ApplyDecisions(append([]ClaimNode(nil), input.Claims...), decisions)
	claims = ApplyAdjudicationRecords(claims, records)
	taskVerdict := DetermineTaskVerdict(claims)
	return ArbiterReport{
		TaskVerdict: taskVerdict,
		Summary:     buildArbiterSummary(taskVerdict, records),
		Decisions:   decisions,
		Records:     records,
	}
}

func adjudicateClaim(manifest CaseManifest, claim ClaimNode, tickets []ChallengeTicket, findings []VerificationResult, evidence []EvidenceRecord, allowUndetermined bool) (ArbiterDecision, AdjudicationRecord) {
	openChallenges := 0
	blockingRisks := make([]string, 0)
	maxSeverity := AttackSeverityLow
	evidenceRefs := append([]string(nil), claim.EvidenceRefs...)
	for _, ticket := range tickets {
		evidenceRefs = appendUnique(evidenceRefs, ticket.EvidenceRefs...)
		evidenceRefs = appendUnique(evidenceRefs, ticket.VerificationRefs...)
		if ticket.Status == ChallengeStatusOpen {
			openChallenges++
			blockingRisks = append(blockingRisks, firstNonEmpty(ticket.AttackText, ticket.Statement))
			if severityRank(ticket.Severity) > severityRank(maxSeverity) {
				maxSeverity = firstNonEmptySeverity(ticket.Severity)
			}
		}
	}
	passed := 0
	failed := 0
	inconclusive := 0
	highQualityEvidence := 0
	for _, item := range evidence {
		evidenceRefs = appendUnique(evidenceRefs, item.EntryID)
		switch item.ProvenanceQuality {
		case ProvenanceQualityHigh:
			highQualityEvidence++
		}
	}
	confidence := claim.Confidence
	if confidence <= 0 {
		confidence = 0.5
	}
	for _, finding := range findings {
		evidenceRefs = appendUnique(evidenceRefs, finding.EvidenceRef)
		switch finding.Status {
		case VerificationStatusPassed:
			passed++
		case VerificationStatusFailed:
			failed++
		default:
			inconclusive++
		}
		if finding.Confidence > 0 {
			confidence = math.Max(confidence, finding.Confidence)
		}
		confidence += finding.ConfidenceDelta
		if finding.Status == VerificationStatusFailed {
			blockingRisks = append(blockingRisks, firstNonEmpty(finding.Summary, finding.FailureCode))
		}
	}
	confidence = clamp01(confidence + 0.05*float64(highQualityEvidence) - 0.08*float64(openChallenges))
	boundaryClear := len(claim.BoundaryConditions) > 0 || claim.ClaimType == ClaimTypeFact
	var disposition ClaimDisposition
	var actionability Actionability
	var rationale string
	switch {
	case failed > 0:
		disposition = ClaimDispositionReject
		actionability = ActionabilityBlocked
		rationale = fmt.Sprintf("存在 %d 条失败验证，claim 未能通过证据检验", failed)
		confidence = clamp01(confidence - 0.25)
	case passed > 0 && openChallenges == 0 && boundaryClear:
		disposition = ClaimDispositionKeep
		actionability = ActionabilityReady
		rationale = fmt.Sprintf("存在 %d 条通过验证，且无未关闭攻击记录", passed)
	case passed > 0:
		disposition = ClaimDispositionKeepWithCaveat
		actionability = ActionabilityGated
		rationale = fmt.Sprintf("存在 %d 条通过验证，但仍有 %d 个未完全解除的风险或边界条件", passed, openChallenges)
	case openChallenges > 0 || inconclusive > 0:
		disposition = ClaimDispositionUnresolved
		actionability = ActionabilityBlocked
		if !allowUndetermined && inconclusive > 0 {
			rationale = fmt.Sprintf("共有 %d 条验证但均不具决定性，且 challenge 尚未消解", inconclusive)
		} else {
			rationale = fmt.Sprintf("challenge=%d，inconclusive=%d，保留 unresolved", openChallenges, inconclusive)
		}
	default:
		disposition = ClaimDispositionUnresolved
		actionability = ActionabilityBlocked
		rationale = "缺少足够证据，不能仅凭表述保留 claim"
	}
	if manifest.RiskLevel == RiskLevelHigh && disposition == ClaimDispositionKeep {
		actionability = ActionabilityGated
		rationale += "；由于 case 风险较高，执行仍需 gate"
	}
	if maxSeverity == AttackSeverityHigh && disposition == ClaimDispositionKeep {
		disposition = ClaimDispositionKeepWithCaveat
		actionability = ActionabilityGated
		rationale += "；存在高严重度攻击，保留 caveat"
	}
	decisionVerdict := ClaimVerdictUndetermined
	switch disposition {
	case ClaimDispositionKeep, ClaimDispositionKeepWithCaveat:
		decisionVerdict = ClaimVerdictSupported
	case ClaimDispositionReject:
		decisionVerdict = ClaimVerdictRefuted
	case ClaimDispositionUnresolved:
		if len(evidenceRefs) > 0 && !allowUndetermined {
			decisionVerdict = ClaimVerdictInsufficientEvidence
		}
	}
	return ArbiterDecision{
			ClaimID:      claim.ClaimID,
			Verdict:      decisionVerdict,
			Confidence:   confidence,
			Rationale:    rationale,
			EvidenceRefs: dedupeStrings(evidenceRefs),
		}, AdjudicationRecord{
			TargetClaimID:   claim.ClaimID,
			Disposition:     disposition,
			Rationale:       rationale,
			FinalConfidence: confidence,
			BlockingRisks:   dedupeStrings(blockingRisks),
			Actionability:   actionability,
			EvidenceRefs:    dedupeStrings(evidenceRefs),
		}
}

func deriveRecordsFromDecisions(request StartRequest, claims []ClaimNode, decisions []ArbiterDecision) []AdjudicationRecord {
	index := make(map[string]ClaimNode, len(claims))
	for _, claim := range claims {
		index[claim.ClaimID] = claim
	}
	out := make([]AdjudicationRecord, 0, len(decisions))
	for _, decision := range decisions {
		disposition := ClaimDispositionUnresolved
		actionability := ActionabilityBlocked
		switch decision.Verdict {
		case ClaimVerdictSupported:
			disposition = ClaimDispositionKeep
			actionability = ActionabilityReady
		case ClaimVerdictRefuted:
			disposition = ClaimDispositionReject
		case ClaimVerdictInsufficientEvidence, ClaimVerdictUndetermined:
			disposition = ClaimDispositionUnresolved
		}
		if claim, ok := index[decision.ClaimID]; ok && shouldPreferCaveatedSupportFromDecision(request, claim, decision) {
			disposition = ClaimDispositionKeepWithCaveat
			actionability = ActionabilityGated
		}
		if claim, ok := index[decision.ClaimID]; ok && len(claim.Caveats) > 0 && disposition == ClaimDispositionKeep {
			disposition = ClaimDispositionKeepWithCaveat
			actionability = ActionabilityGated
		}
		if claim, ok := index[decision.ClaimID]; ok && request.TaskSpec.TaskType == CaseTaskTypeStrategy && claim.ClaimType == ClaimTypeRecommendation && actionability == ActionabilityReady {
			actionability = ActionabilityGated
		}
		out = append(out, AdjudicationRecord{
			TargetClaimID:   decision.ClaimID,
			Disposition:     disposition,
			Rationale:       decision.Rationale,
			FinalConfidence: decision.Confidence,
			Actionability:   actionability,
			EvidenceRefs:    append([]string(nil), decision.EvidenceRefs...),
			Metadata:        maps.Clone(decision.Metadata),
		})
	}
	return out
}

func shouldPreferCaveatedSupportFromDecision(request StartRequest, claim ClaimNode, decision ArbiterDecision) bool {
	switch request.TaskSpec.TaskType {
	case CaseTaskTypeStrategy, CaseTaskTypeOperational:
	default:
		return false
	}
	if claim.Status != ClaimStatusRevised {
		return false
	}
	if claim.ClaimType == ClaimTypeFact {
		return false
	}
	if strings.TrimSpace(claim.Applicability) == "" && len(claim.Caveats) == 0 && len(claim.BoundaryConditions) == 0 {
		return false
	}
	if strings.TrimSpace(claim.Statement) == "" {
		return false
	}
	switch decision.Verdict {
	case ClaimVerdictInsufficientEvidence:
		if decision.Confidence < 0.40 {
			return false
		}
	case ClaimVerdictUndetermined:
		if claim.ClaimType != ClaimTypeRecommendation || decision.Confidence < 0.42 {
			return false
		}
	default:
		return false
	}
	return true
}

func buildArbiterSummary(verdict TaskVerdict, records []AdjudicationRecord) string {
	counts := map[ClaimDisposition]int{}
	for _, record := range records {
		counts[record.Disposition]++
	}
	return fmt.Sprintf(
		"taskVerdict=%s keep=%d keep_with_caveat=%d unresolved=%d reject=%d",
		verdict,
		counts[ClaimDispositionKeep],
		counts[ClaimDispositionKeepWithCaveat],
		counts[ClaimDispositionUnresolved],
		counts[ClaimDispositionReject],
	)
}

func severityRank(value AttackSeverity) int {
	switch value {
	case AttackSeverityHigh:
		return 3
	case AttackSeverityMedium:
		return 2
	default:
		return 1
	}
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
