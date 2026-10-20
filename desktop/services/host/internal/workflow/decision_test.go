package workflow

import (
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"testing"
)

func TestLowScoreDelphiConvergenceDoesNotPass(t *testing.T) {
	s := model.RunSnapshot{Reviews: []model.ReviewIssue{}}
	Finalize(&s, &consensus.RunResult{Delphi: &consensus.DelphiResultSection{Converged: true, ResponseQuorumMet: true, MinResponsesMet: true, Statements: []consensus.DelphiStatement{{StatementID: "low", Statement: "不推荐", MeanRating: 1}}}})
	if s.Decision.Outcome != "rejected" || s.Decision.CoreOutcome != "converged" {
		t.Fatalf("低分收敛不能通过: %#v", s.Decision)
	}
}
func TestOpenBlockingIssuePreventsApproval(t *testing.T) {
	s := model.RunSnapshot{Reviews: []model.ReviewIssue{{Severity: "blocking", Status: "open"}}}
	Finalize(&s, &consensus.RunResult{Delphi: &consensus.DelphiResultSection{Converged: true, ResponseQuorumMet: true, MinResponsesMet: true, Statements: []consensus.DelphiStatement{{MeanRating: 5}}}})
	if s.Decision.Outcome != "needs_human" {
		t.Fatal("未解决阻塞问题不应通过")
	}
}

func TestNoncompletedTerminalStateCannotPass(t *testing.T) {
	s := model.RunSnapshot{}
	Finalize(&s, &consensus.RunResult{TerminalState: consensus.TerminalStateRequiresHumanReview, Delphi: &consensus.DelphiResultSection{Converged: true, ResponseQuorumMet: true, MinResponsesMet: true, Statements: []consensus.DelphiStatement{{MeanRating: 5}}}})
	if s.Decision.Outcome == "supported" {
		t.Fatal("非completed终态不允许通过")
	}
}
