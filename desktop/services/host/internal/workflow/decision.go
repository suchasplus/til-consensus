package workflow

import (
	"fmt"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

// Finalize only reads finished engine output; it never asks a model to decide.
func Finalize(s *model.RunSnapshot, result *consensus.RunResult) {
	d := &model.Decision{Outcome: "no_consensus", CoreOutcome: string(result.TerminalState), Mock: s.ProviderMode == "mock", Votes: []model.Vote{}, MinorityOpinions: []string{}, Assumptions: []string{}, Summary: "讨论完成。"}
	s.Claims = []model.ClaimView{}
	if result.Error != nil {
		d.Outcome = "failed"
		d.CoreOutcome = "failed"
		d.Summary = result.Error.Message
		s.Decision = d
		return
	}
	if f := result.FreeDebate; f != nil {
		plan := SnapshotRolePlan(*s)
		reviewerSet := map[string]bool{}
		for _, id := range plan.Reviewers {
			reviewerSet[id] = true
		}
		expectedVoters := len(plan.Reviewers)
		d.CoreOutcome = string(f.Outcome)
		resolutions := map[string]consensus.DebateClaimResolution{}
		for _, r := range f.ClaimResolutions {
			resolutions[r.ClaimID] = r
		}
		allSupported := len(f.Claims) > 0
		canonical := ""
		for _, c := range f.Claims {
			if !c.Active {
				continue
			}
			r := resolutions[c.ClaimID]
			score, accepted := r.SupportScore, r.Accepted
			s.Claims = append(s.Claims, model.ClaimView{ID: c.ClaimID, Statement: c.Statement, Category: string(c.Category), SupportScore: &score, Accepted: &accepted})
			independentSupport := map[string]bool{}
			for _, id := range r.SupportingVoters {
				if reviewerSet[id] {
					independentSupport[id] = true
				}
			}
			if !r.Accepted || len(independentSupport) < 2 || r.VoteCount < expectedVoters {
				allSupported = false
			}
			if !r.Accepted {
				d.MinorityOpinions = append(d.MinorityOpinions, c.Statement)
			}
			if c.Category == consensus.DebateClaimCategorySynthesis {
				canonical = c.Statement
			}
		}
		for _, v := range f.Votes {
			d.Votes = append(d.Votes, model.Vote{AgentID: v.AgentID, ClaimID: v.ClaimID, Vote: string(v.Vote), Support: v.Confidence, Rationale: v.Rationale})
		}
		if canonical != "" {
			setProposal(s, canonical)
		}
		if f.Outcome == consensus.FreeDebateOutcomeQuorumNotMet {
			d.Outcome = "quorum_not_met"
		} else if allSupported && canonical != "" && exactReviewers(f.Voters, plan.Reviewers) && len(f.AbsentVoters) == 0 {
			d.Outcome = "supported"
		}
		d.Summary = fmt.Sprintf("自由辩论完成：%d 位评审，%d 条冻结主张；内核结论：%s。", len(f.Voters), len(s.Claims), f.Outcome)
	}
	if a := result.Adjudication; a != nil {
		d.CoreOutcome = string(a.TaskVerdict)
		all := len(a.ClaimGraph) > 0
		for _, c := range a.ClaimGraph {
			accepted := c.Verdict == consensus.ClaimVerdictSupported
			s.Claims = append(s.Claims, model.ClaimView{ID: c.ClaimID, Statement: c.Statement, Category: string(c.ClaimType), Accepted: &accepted, EvidenceRefs: c.EvidenceRefs, Adjudication: string(c.Disposition)})
			if c.ClaimType == consensus.ClaimTypeAssumption {
				d.Assumptions = append(d.Assumptions, c.Statement)
			}
			if len(c.Caveats) > 0 {
				d.Assumptions = append(d.Assumptions, c.Caveats...)
			}
			if !accepted {
				d.MinorityOpinions = append(d.MinorityOpinions, c.Statement)
				all = false
			}
		}
		if a.TaskVerdict == consensus.TaskVerdictSupported && all && result.TerminalState == consensus.TerminalStateCompleted {
			d.Outcome = "supported"
		} else {
			d.Outcome = "insufficient_evidence"
		}
		d.Summary = "裁决完成；模型语义判断与可复现证据分别记录。"
		if result.Report.Summary != "" {
			setProposal(s, result.Report.Summary)
		}
	}
	if f := result.Delphi; f != nil {
		s.DelphiDistribution = f.RatingDistributions
		d.CoreOutcome = "not_converged"
		if f.Converged {
			d.CoreOutcome = "converged"
		}
		for _, c := range f.Statements {
			score, accepted := c.MeanRating, c.MeanRating >= 4
			s.Claims = append(s.Claims, model.ClaimView{ID: c.StatementID, Statement: c.Statement, Category: "candidate", SupportScore: &score, Accepted: &accepted})
		}
		if !f.ResponseQuorumMet || !f.MinResponsesMet {
			d.Outcome = "quorum_not_met"
		} else if f.Converged && len(f.Statements) > 0 {
			if f.Statements[0].MeanRating >= 4 {
				d.Outcome = "supported"
			} else {
				d.Outcome = "rejected"
			}
		}
		d.Summary = fmt.Sprintf("德尔菲完成：共识度 %.0f%%，匿名响应与最低支持分分别检查。", f.ConsensusLevel*100)
		d.MinorityOpinions = append(d.MinorityOpinions, f.DissentSummary...)
		if f.Recommendation != "" {
			setProposal(s, f.Recommendation)
		}
	}
	for _, i := range s.Reviews {
		if i.Severity == "blocking" && i.Status != "resolved" {
			d.Outcome = "needs_human"
			d.Summary += " 仍有未解决的阻塞意见。"
		}
	}
	if result.TerminalState != consensus.TerminalStateCompleted && d.Outcome == "supported" {
		d.Outcome = "needs_human"
		d.Summary += " 内核终态要求人工复核。"
	}
	if d.Mock {
		d.Summary = "[Mock 测试] " + d.Summary
	}
	s.Decision = d
}

func exactReviewers(actual, expected []string) bool {
	if len(expected) < 2 || len(actual) != len(expected) {
		return false
	}
	seen := map[string]bool{}
	for _, id := range actual {
		if seen[id] {
			return false
		}
		seen[id] = true
	}
	for _, id := range expected {
		if !seen[id] {
			return false
		}
	}
	return true
}

func setProposal(s *model.RunSnapshot, content string) {
	if s.Proposal != nil && s.Proposal.Content == content {
		return
	}
	version := 1
	if s.Proposal != nil {
		version = s.Proposal.Version + 1
	}
	s.Proposal = &model.Proposal{ID: fmt.Sprintf("%s-p%d", s.ID, version), Title: "当前综合方案", Content: content, Hash: Hash(content), Version: version, Kind: "final"}
	s.ProposalHistory = append(s.ProposalHistory, *s.Proposal)
}
