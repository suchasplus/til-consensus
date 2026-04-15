package consensus

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type DefaultArbiterDeps struct {
	TaskDelegate   TaskDelegate
	Clock          Clock
	IDFactory      IDFactory
	PerTaskTimeout time.Duration
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
	dispatched, err := a.deps.TaskDelegate.Dispatch(ctx, task)
	if err != nil {
		return ArbiterReport{}, false, nil
	}
	awaited, err := a.deps.TaskDelegate.Await(ctx, dispatched.TaskID, timeout)
	if err != nil || !awaited.OK {
		return ArbiterReport{}, false, nil
	}
	typed, ok := awaited.Output.(ArbiterTaskResult)
	if !ok {
		return ArbiterReport{}, false, nil
	}
	report := ArbiterReport{
		TaskVerdict: typed.Output.TaskVerdict,
		Summary:     strings.TrimSpace(typed.Output.Summary),
		Decisions:   typed.Output.Decisions,
	}
	if report.TaskVerdict == "" {
		report.TaskVerdict = DetermineTaskVerdict(ApplyDecisions(append([]ClaimNode(nil), input.Claims...), report.Decisions))
	}
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
	decisions := make([]ArbiterDecision, 0, len(input.Claims))
	for _, claim := range input.Claims {
		decision := ArbiterDecision{
			ClaimID: claim.ClaimID,
		}
		openChallenges := 0
		for _, ticket := range challengeMap[claim.ClaimID] {
			if ticket.Status == ChallengeStatusOpen {
				openChallenges++
			}
			decision.EvidenceRefs = appendUnique(decision.EvidenceRefs, ticket.EvidenceRefs...)
			decision.EvidenceRefs = appendUnique(decision.EvidenceRefs, ticket.VerificationRefs...)
		}
		passed := 0
		failed := 0
		inconclusive := 0
		var confidence float64
		for _, finding := range findingMap[claim.ClaimID] {
			switch finding.Status {
			case VerificationStatusPassed:
				passed++
			case VerificationStatusFailed:
				failed++
			default:
				inconclusive++
			}
			if finding.Confidence > confidence {
				confidence = finding.Confidence
			}
			if finding.EvidenceRef != "" {
				decision.EvidenceRefs = appendUnique(decision.EvidenceRefs, finding.EvidenceRef)
			}
		}
		decision.EvidenceRefs = appendUnique(decision.EvidenceRefs, claim.EvidenceRefs...)
		switch {
		case failed > 0:
			decision.Verdict = ClaimVerdictRefuted
			decision.Rationale = fmt.Sprintf("存在 %d 条失败的验证记录", failed)
		case passed > 0 && openChallenges == 0:
			decision.Verdict = ClaimVerdictSupported
			decision.Rationale = fmt.Sprintf("存在 %d 条通过的验证记录且无未关闭 challenge", passed)
		case passed == 0 && openChallenges > 0 && input.Request.ArbiterPolicy.AllowUndetermined:
			decision.Verdict = ClaimVerdictUndetermined
			decision.Rationale = fmt.Sprintf("仍有 %d 个未关闭 challenge，证据不足以裁决", openChallenges)
		case passed == 0 && inconclusive > 0:
			decision.Verdict = ClaimVerdictInsufficientEvidence
			decision.Rationale = fmt.Sprintf("共有 %d 条验证记录，但都不足以形成支持", inconclusive)
		case len(decision.EvidenceRefs) > 0:
			decision.Verdict = ClaimVerdictInsufficientEvidence
			decision.Rationale = "已有证据记录，但仍不足以形成支持或反驳"
		default:
			decision.Verdict = ClaimVerdictUndetermined
			decision.Rationale = "缺少足够证据"
		}
		decision.Confidence = confidence
		decisions = append(decisions, decision)
	}
	claims := ApplyDecisions(append([]ClaimNode(nil), input.Claims...), decisions)
	taskVerdict := DetermineTaskVerdict(claims)
	return ArbiterReport{
		TaskVerdict: taskVerdict,
		Summary:     buildArbiterSummary(taskVerdict, decisions),
		Decisions:   decisions,
	}
}

func buildArbiterSummary(verdict TaskVerdict, decisions []ArbiterDecision) string {
	counts := map[ClaimVerdict]int{}
	for _, decision := range decisions {
		counts[decision.Verdict]++
	}
	return fmt.Sprintf(
		"taskVerdict=%s supported=%d refuted=%d insufficient=%d undetermined=%d",
		verdict,
		counts[ClaimVerdictSupported],
		counts[ClaimVerdictRefuted],
		counts[ClaimVerdictInsufficientEvidence],
		counts[ClaimVerdictUndetermined],
	)
}
