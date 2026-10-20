package delegate

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
)

func localizeCore(task consensus.Task, raw any) (map[string]any, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var core map[string]any
	if err := json.Unmarshal(data, &core); err != nil {
		return nil, err
	}
	label := agentLabel(task.Meta().AgentID)
	goal := shortGoal(taskGoal(task))
	focus := agentFocus(task.Meta().AgentID)
	core["summary"] = fmt.Sprintf("【Mock】%s完成%s：围绕“%s”检查%s。", label, phaseLabel(task), goal, focus)
	for i, claim := range objects(core["claims"]) {
		// 公开 proposal schema 不包含 mock runner 的 scope 字段。
		if _, ok := task.(consensus.ProposalTask); ok {
			delete(claim, "scope")
		}
		claim["title"] = fmt.Sprintf("%s建议 %d", focus, i+1)
		claim["statement"] = fmt.Sprintf("针对“%s”，%s；通过可追溯记录和明确验收条件验证该建议（Mock）。", goal, proposalPoint(task.Meta().AgentID, i))
	}
	for _, ticket := range objects(core["tickets"]) {
		ticket["statement"] = "【Mock】请为该主张补充可复现的验收依据，并说明异常与边界条件。"
		ticket["suggestedFalsificationMethod"] = "【Mock】用一项正常场景和一项取消/失败场景检查当前结论。"
	}
	for _, revision := range objects(core["revisions"]) {
		revision["reason"] = "【Mock】按评审补充适用边界，保留尚未验证的假设。"
		revision["caveats"] = []string{"【Mock】仅验证演示流程，真实 CLI 和业务行为仍需后续验证"}
	}
	for _, judgement := range objects(core["judgements"]) {
		judgement["rationale"] = "【Mock】已检查本轮冻结上下文；同意该主张，并保留实际接入后验证的边界。"
	}
	for _, vote := range objects(core["votes"]) {
		vote["rationale"] = "【Mock】支持当前冻结版本；本票仅用于演示讨论收敛。"
	}
	for _, finding := range objects(core["results"]) {
		finding["rationale"] = "supported_core: 【Mock】主张与当前任务和演示材料一致。 | missing_or_conflict: 真实执行结果尚未接入，仍保留演示边界。 | verdict_reason: 【Mock】当前演示证据支持受限主张，此结果不代表真实业务已经验证。"
	}
	for _, decision := range objects(core["decisions"]) {
		decision["rationale"] = "【Mock】依据当前质疑状态和验证记录裁决；未解决质疑将保留为不确定。"
	}
	// 公开 arbiter schema 接收 decisions，不包含 mock runner 的额外 records 字段。
	// 解码前仅保留 schema 定义的核心字段。
	if _, ok := task.(consensus.ArbiterTask); ok {
		delete(core, "records")
	}
	switch value := task.(type) {
	case consensus.SynthesisTask:
		statement := fmt.Sprintf("【Mock】围绕“%s”，综合 %d 条主张：先实现可追溯的最小闭环，明确任务边界、异常处理和验收条件，再分步验证。", goal, len(value.Claims))
		if value.Draft != nil {
			statement = value.Draft.Statement + fmt.Sprintf("\n【Mock 修订】已逐条复核 %d 项修正意见，保留尚待真实验证的条件。", len(value.Amendments))
		}
		core["claim"] = map[string]any{"title": "统一方案 · Mock", "statement": statement}
	case consensus.DelphiQuestionnaireTask:
		responses := objects(core["responses"])
		for _, response := range responses {
			response["statement"] = fmt.Sprintf("针对“%s”，先做可追溯的最小闭环，再分步验证（Mock）。", goal)
			response["rating"] = float64(value.RatingScaleMin) + 0.8*float64(value.RatingScaleMax-value.RatingScaleMin)
			response["rationale"] = "【Mock】独立评分：范围清楚、实施成本可控，仍需验证真实接入。"
		}
	case consensus.DelphiRevisionTask:
		responses := make([]map[string]any, 0, len(value.StatementSummaries))
		for _, statement := range value.StatementSummaries {
			responses = append(responses, map[string]any{"statementId": statement.StatementID, "rating": float64(value.RatingScaleMin) + 0.8*float64(value.RatingScaleMax-value.RatingScaleMin), "rationale": "【Mock】依据匿名汇总独立复评，继续支持分步验证方案。"})
		}
		if len(responses) > 0 {
			core["responses"] = responses
		}
	case consensus.DelphiFacilitatorSummaryTask:
		core["summary"] = "【Mock】匿名评分分布已汇总；一致程度由内核计算，最低推荐分由桌面规则另行检查。"
		core["recommendation"] = fmt.Sprintf("针对“%s”，先做可追溯的最小闭环，再分步验证（Mock）。", goal)
		if len(value.StatementSummaries) > 0 {
			core["recommendation"] = value.StatementSummaries[0].Statement
		}
	case consensus.ReportTask:
		core["highlights"] = []string{"【Mock】三方意见已进入内核工作流并留下审计记录"}
		core["retainedClaims"] = []string{"【Mock】保留与任务目标一致且满足当前规则的主张"}
		core["downgradedClaims"] = []string{}
		core["unresolvedQuestions"] = []string{"【Mock】真实 CLI、凭据和业务执行尚未接入"}
		core["nextActions"] = []string{"完成 M0 验收后接入真实 Provider"}
	case consensus.ActionTask:
		core["fullResponse"] = "【Mock】已生成演示结果，未执行外部操作。"
	}
	return core, nil
}

func buildReviews(task consensus.Task, id string, core map[string]any) []Review {
	reviews := make([]Review, 0)
	for i, claim := range objects(core["claims"]) {
		index := i
		reviews = append(reviews, Review{ID: fmt.Sprintf("%s-review-%d", id, i), InvocationID: id, ClaimDraftIndex: &index, DraftStatement: fmt.Sprint(claim["statement"]), RaisedBy: task.Meta().AgentID, Severity: "suggestion", Status: "open", Statement: "【Mock】" + fmt.Sprint(claim["title"]) + "：在综合阶段明确验收边界。", ResolutionRefs: []string{}, ExclusiveWith: []string{}})
	}
	add := func(claimID, statement, status string) {
		refs := []string{}
		if status == "resolved" {
			refs = append(refs, id)
		}
		reviews = append(reviews, Review{ID: fmt.Sprintf("%s-review-%d", id, len(reviews)), ClaimID: claimID, RaisedBy: task.Meta().AgentID, Severity: "suggestion", Status: status, Statement: statement, ResolutionRefs: refs, ExclusiveWith: []string{}})
	}
	for _, ticket := range objects(core["tickets"]) {
		add(fmt.Sprint(ticket["claimId"]), "【Mock】补充当前主张的验证依据。", "open")
	}
	for _, judgement := range objects(core["judgements"]) {
		add(fmt.Sprint(judgement["claimId"]), "【Mock】已检查该主张，当前演示范围内无新增反对意见。", "resolved")
	}
	return reviews
}

func taskClaimIDs(task consensus.Task) map[string]bool {
	ids := make(map[string]bool)
	addClaims := func(claims []consensus.ClaimNode) {
		for _, claim := range claims {
			ids[claim.ClaimID] = true
		}
	}
	addDebate := func(claims []consensus.DebateClaim) {
		for _, claim := range claims {
			ids[claim.ClaimID] = true
		}
	}
	switch value := task.(type) {
	case consensus.ChallengeTask:
		addClaims(value.Claims)
	case consensus.ReviseTask:
		addClaims(value.Claims)
	case consensus.ArbiterTask:
		addClaims(value.Claims)
	case consensus.ReportTask:
		addClaims(value.Claims)
	case consensus.SemanticVerificationTask:
		ids[value.Claim.ClaimID] = true
	case consensus.DebateRoundTask:
		addDebate(value.SelfClaims)
		addDebate(value.PeerClaims)
	case consensus.SemanticDedupTask:
		addDebate(value.Claims)
	case consensus.SynthesisTask:
		addDebate(value.Claims)
		if value.Draft != nil {
			ids[value.Draft.ClaimID] = true
		}
	case consensus.FinalVoteTask:
		addDebate(value.Claims)
	}
	return ids
}

func messageContent(task consensus.Task, core map[string]any) string {
	// 展示标签取自冻结任务；完整实体标识和精确数值继续保存在原始产物中。
	labels := make(map[string]string)
	shortLabel := func(value string) string {
		value = strings.Join(strings.Fields(value), " ")
		runes := []rune(value)
		if len(runes) > 40 {
			return string(runes[:40]) + "…"
		}
		return value
	}
	remember := func(item map[string]any) {
		id := firstString(item, "claimId", "statementId")
		label := shortLabel(firstString(item, "title", "statement"))
		if id != "" && label != "" {
			labels[id] = label
		}
	}
	inputJSON, _ := json.Marshal(task)
	var input map[string]any
	if json.Unmarshal(inputJSON, &input) == nil {
		for _, key := range []string{"claims", "selfClaims", "peerClaims", "statementSummaries", "previousStatements"} {
			for _, item := range objects(input[key]) {
				remember(item)
			}
		}
		for _, key := range []string{"claim", "draft"} {
			if item, ok := input[key].(map[string]any); ok {
				remember(item)
			}
		}
	}
	var text strings.Builder
	text.WriteString(fmt.Sprint(core["summary"]))
	for _, key := range []string{"claims", "newClaims", "tickets", "revisions", "judgements", "votes", "responses", "decisions", "results", "merges"} {
		for index, item := range objects(core[key]) {
			statement := firstString(item, "statement", "revisedText", "rationale", "reason")
			ref := labels[firstString(item, "claimId", "targetClaimId", "statementId")]
			if ref == "" {
				ref = shortLabel(firstString(item, "title"))
			}
			if ref == "" {
				noun := "主张"
				if key == "responses" {
					noun = "候选"
				}
				ref = fmt.Sprintf("%s %d", noun, index+1)
			}
			fmt.Fprintf(&text, "\n\n- %s：%s", ref, statement)
			if rating, ok := item["rating"]; ok {
				if number, err := strconv.ParseFloat(fmt.Sprint(rating), 64); err == nil {
					fmt.Fprintf(&text, "（评分 %.1f）", number)
				}
			}
			if vote, ok := item["vote"]; ok {
				label := map[string]string{"accept": "支持", "reject": "反对", "abstain": "弃权"}[fmt.Sprint(vote)]
				if label == "" {
					label = "待确认"
				}
				if confidence, err := strconv.ParseFloat(fmt.Sprint(item["confidence"]), 64); err == nil && vote != "abstain" {
					fmt.Fprintf(&text, "（%s，支持分 %.0f%%）", label, confidence*100)
				} else {
					fmt.Fprintf(&text, "（%s）", label)
				}
			}
		}
	}
	if claim, ok := core["claim"].(map[string]any); ok {
		fmt.Fprintf(&text, "\n\n%s", claim["statement"])
	}
	if recommendation, ok := core["recommendation"]; ok {
		fmt.Fprintf(&text, "\n\n建议：%v", recommendation)
	}
	if verdict, ok := core["taskVerdict"]; ok {
		fmt.Fprintf(&text, "\n\n内核裁决：%v", verdict)
	}
	return text.String()
}

func objects(value any) []map[string]any {
	var result []map[string]any
	switch values := value.(type) {
	case []any:
		for _, value := range values {
			if object, ok := value.(map[string]any); ok {
				result = append(result, object)
			}
		}
	case []map[string]any:
		return values
	}
	return result
}

func firstString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := value[key].(string); ok && text != "" {
			return text
		}
	}
	return ""
}

func agentLabel(id string) string {
	switch {
	case strings.Contains(id, "codex"):
		return "Codex"
	case strings.Contains(id, "claude"):
		return "Claude Code"
	case strings.Contains(id, "grok"):
		return "Grok Build"
	case strings.Contains(id, "agy"):
		return "agy"
	default:
		return id
	}
}

func agentFocus(id string) string {
	switch {
	case strings.Contains(id, "claude"):
		return "正确性和可维护性"
	case strings.Contains(id, "grok"):
		return "替代路线与反例"
	case strings.Contains(id, "agy"):
		return "集成边界和实施成本"
	default:
		return "目标、范围和验收"
	}
}

func proposalPoint(id string, index int) string {
	points := []string{"先交付最小可运行闭环，阶段和结果分别记录", "为取消、失败和重试保留明确状态与审计产物"}
	switch {
	case strings.Contains(id, "claude"):
		points = []string{"将方案、争议和投票分开保存，并核对同一版本", "以关键路径和异常路径验证正确性，保留待验证条件"}
	case strings.Contains(id, "grok"):
		points = []string{"比较最小闭环与一次性全量实现，优先可验证的小步交付", "保留反例和替代方案，避免仅凭相同表述判断通过"}
	case strings.Contains(id, "agy"):
		points = []string{"用明确进程边界隔离界面、调度和外部工具", "先打通启动、停止和持久化，再逐个接入真实工具"}
	}
	return points[index%len(points)]
}

func shortGoal(goal string) string {
	runes := []rune(strings.TrimSpace(goal))
	if len(runes) > 100 {
		return string(runes[:100]) + "…"
	}
	if len(runes) == 0 {
		return "当前任务"
	}
	return string(runes)
}

func taskGoal(task consensus.Task) string {
	switch value := task.(type) {
	case consensus.ProposalTask:
		return value.TaskSpec.Goal
	case consensus.InitialProposalTask:
		return value.TaskSpec.Goal
	case consensus.ChallengeTask:
		return value.TaskSpec.Goal
	case consensus.ReviseTask:
		return value.TaskSpec.Goal
	case consensus.ArbiterTask:
		return value.TaskSpec.Goal
	case consensus.ReportTask:
		return value.TaskSpec.Goal
	case consensus.SemanticVerificationTask:
		return value.TaskSpec.Goal
	case consensus.DebateRoundTask:
		return value.TaskSpec.Goal
	case consensus.SemanticDedupTask:
		return value.TaskSpec.Goal
	case consensus.SynthesisTask:
		return value.TaskSpec.Goal
	case consensus.FinalVoteTask:
		return value.TaskSpec.Goal
	case consensus.DelphiQuestionnaireTask:
		return value.TaskSpec.Goal
	case consensus.DelphiRevisionTask:
		return value.TaskSpec.Goal
	case consensus.DelphiFacilitatorSummaryTask:
		return value.TaskSpec.Goal
	case consensus.ActionTask:
		return value.Prompt
	}
	return ""
}

func round(task consensus.Task) int {
	switch value := task.(type) {
	case consensus.InitialProposalTask:
		return value.Round
	case consensus.DebateRoundTask:
		return value.Round
	case consensus.SynthesisTask:
		return value.Round
	case consensus.FinalVoteTask:
		return value.Round
	case consensus.DelphiQuestionnaireTask:
		return value.Round
	case consensus.DelphiRevisionTask:
		return value.Round
	case consensus.DelphiFacilitatorSummaryTask:
		return value.Round
	case consensus.ReviseTask:
		return value.Round
	case consensus.SemanticDedupTask:
		return value.Round
	}
	return 0
}

func phase(task consensus.Task) string {
	switch task.Kind() {
	case consensus.TaskKindPropose:
		return "propose"
	case consensus.TaskKindInitialProposal:
		return "initial"
	case consensus.TaskKindDebateRound:
		if value, ok := task.(consensus.DebateRoundTask); ok && value.SynthesisReview {
			return "amend"
		}
		return "debate"
	case consensus.TaskKindSemanticDedup:
		return "debate"
	case consensus.TaskKindFinalVote:
		return "final_vote"
	case consensus.TaskKindSemanticVerify:
		return "verify"
	case consensus.TaskKindArbitrate:
		return "adjudicate"
	case consensus.TaskKindDelphiQuestionnaire:
		return "questionnaire"
	case consensus.TaskKindDelphiRevision:
		return "revision"
	case consensus.TaskKindDelphiFacilitatorSummary:
		return "anonymous_summary"
	}
	return string(task.Kind())
}

func phaseLabel(task consensus.Task) string {
	switch phase(task) {
	case "propose":
		return "初稿起草"
	case "initial":
		return "独立评审"
	case "debate":
		return "本轮讨论"
	case "synthesis":
		return "方案综合"
	case "amend":
		return "综合稿复评"
	case "final_vote":
		return "冻结版本投票"
	case "challenge":
		return "证据质疑"
	case "verify":
		return "语义验证"
	case "revise":
		return "方案修订"
	case "adjudicate":
		return "主张裁决"
	case "questionnaire":
		return "匿名独立问卷"
	case "revision":
		return "匿名复评"
	case "anonymous_summary":
		return "匿名汇总"
	case "report":
		return "结果报告"
	}
	return string(task.Kind())
}
