package consensus

import (
	"context"
	"fmt"
	"strings"
)

func ComposeReport(ctx context.Context, delegate TaskDelegate, request StartRequest, sessionID string, arbiter ArbiterReport, claims []ClaimNode, tickets []ChallengeTicket, timeout WaitingPolicy) (AdjudicationReport, *ArtifactRef, error) {
	if request.Roles.Reporter == "" || delegate == nil {
		return BuildBuiltinReport(request, arbiter, claims, tickets), nil, nil
	}
	task := ReportTask{
		TaskMeta: TaskMeta{
			SessionID: sessionID,
			RequestID: request.RequestID,
			AgentID:   request.Roles.Reporter,
			Role:      "reporter",
		},
		TaskSpec:    request.TaskSpec,
		TaskVerdict: arbiter.TaskVerdict,
		Claims:      claims,
		Challenges:  tickets,
		Arbiter:     arbiter,
	}
	_, awaited, _, err := ExecuteTaskWithRetry(ctx, delegate, task, timeout.PerTaskTimeout, timeout.RetryAttempts, TaskRetryHooks{})
	if err != nil || !awaited.OK {
		return BuildBuiltinReport(request, arbiter, claims, tickets), awaited.Artifact, nil
	}
	typed, ok := awaited.Output.(ReportTaskResult)
	if !ok {
		return BuildBuiltinReport(request, arbiter, claims, tickets), awaited.Artifact, nil
	}
	return typed.Output, awaited.Artifact, nil
}

func BuildBuiltinReport(request StartRequest, arbiter ArbiterReport, claims []ClaimNode, tickets []ChallengeTicket) AdjudicationReport {
	highlights := make([]string, 0, len(claims))
	retained := make([]string, 0)
	downgraded := make([]string, 0)
	unresolved := make([]string, 0)
	nextActions := make([]string, 0)
	for _, claim := range claims {
		label := firstNonEmpty(claim.Title, claim.ClaimText, claim.Statement)
		summary := fmt.Sprintf("%s/%s: %s", firstNonEmpty(string(claim.Disposition), string(claim.Verdict)), claim.ClaimType, label)
		if len(claim.Caveats) > 0 {
			summary += "；注意：" + strings.Join(claim.Caveats, "；")
		}
		highlights = append(highlights, summary)
		switch claim.Disposition {
		case ClaimDispositionKeep:
			retained = append(retained, label)
		case ClaimDispositionKeepWithCaveat:
			downgraded = append(downgraded, label)
			nextActions = append(nextActions, "补充边界与 caveat: "+label)
		case ClaimDispositionUnresolved:
			unresolved = append(unresolved, label)
			nextActions = append(nextActions, "补充证据并重新验证: "+label)
		case ClaimDispositionReject:
			nextActions = append(nextActions, "放弃或重写 claim: "+label)
		default:
			if claim.Verdict == ClaimVerdictSupported {
				retained = append(retained, label)
			} else {
				unresolved = append(unresolved, label)
			}
		}
	}
	openChallenges := 0
	for _, ticket := range tickets {
		if ticket.Status == ChallengeStatusOpen {
			openChallenges++
			unresolved = append(unresolved, firstNonEmpty(ticket.AttackText, ticket.Statement))
		}
	}
	if openChallenges > 0 {
		nextActions = append(nextActions, fmt.Sprintf("关闭剩余 %d 个 challenge", openChallenges))
	}
	summary := strings.TrimSpace(arbiter.Summary)
	if summary == "" {
		summary = fmt.Sprintf("任务 %q 的裁决结果为 %s", request.TaskSpec.Goal, arbiter.TaskVerdict)
	}
	return AdjudicationReport{
		Summary:             summary,
		Highlights:          highlights,
		RetainedClaims:      dedupeStrings(retained),
		DowngradedClaims:    dedupeStrings(downgraded),
		UnresolvedQuestions: dedupeStrings(unresolved),
		NextActions:         dedupeStrings(nextActions),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
