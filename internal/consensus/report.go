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
	dispatched, err := delegate.Dispatch(ctx, task)
	if err != nil {
		return BuildBuiltinReport(request, arbiter, claims, tickets), nil, nil
	}
	awaited, err := delegate.Await(ctx, dispatched.TaskID, timeout.PerTaskTimeout)
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
	nextActions := make([]string, 0)
	for _, claim := range claims {
		highlights = append(highlights, fmt.Sprintf("%s: %s", claim.Verdict, firstNonEmpty(claim.Title, claim.Statement)))
		if claim.Verdict == ClaimVerdictUndetermined || claim.Verdict == ClaimVerdictInsufficientEvidence {
			nextActions = append(nextActions, "补充证据: "+firstNonEmpty(claim.Title, claim.Statement))
		}
	}
	openChallenges := 0
	for _, ticket := range tickets {
		if ticket.Status == ChallengeStatusOpen {
			openChallenges++
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
		Summary:     summary,
		Highlights:  highlights,
		NextActions: dedupeStrings(nextActions),
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
