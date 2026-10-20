package workflow

import (
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"testing"
	"time"
)

func TestDynamicRolesAndTwoReviewersAcrossModes(t *testing.T) {
	plan := &model.RolePlan{Proposer: "claude", Reviewers: []string{"grok", "agy"}}
	for _, mode := range []string{"free_debate", "adjudication", "delphi"} {
		r := model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "recorded-mock", Goal: "显式 Mock 策略测试", Mode: mode, ProviderMode: "mock"}}, Frozen: model.FrozenConfig{ProviderMode: "mock", RolePlan: plan}}
		request, err := Request(r, model.Proposal{ID: "p0", Content: "冻结材料", Hash: Hash("冻结材料")}, time.Minute)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if request.Roles.Proposers[0] != "claude" || request.Roles.Synthesizer != "claude" || request.Roles.Facilitator != "claude" || request.Roles.Arbiter != "grok" || request.Roles.SemanticVerifier != "agy" || len(request.Roles.Participants) != 2 || request.DelphiPolicy.MinResponsesPerStatement != 2 || request.DebatePolicy.VoteQuorum != 1 || request.DelphiPolicy.ResponseQuorum != 1 {
			t.Fatalf("%s wrong dynamic policy: %#v", mode, request)
		}
	}
	legacy := RolePlanFor(model.FrozenConfig{})
	if legacy.Proposer != "codex" || len(legacy.Reviewers) != 3 {
		t.Fatal("legacy roles changed")
	}
}

func TestTwoReviewerDecisionRequiresBothDistinctSupporters(t *testing.T) {
	for _, test := range []struct {
		name            string
		support, voters []string
		count           int
		outcome         string
	}{
		{"both", []string{"grok", "agy"}, []string{"grok", "agy"}, 2, "supported"},
		{"one", []string{"grok"}, []string{"grok", "agy"}, 2, "no_consensus"},
		{"duplicate", []string{"grok", "grok"}, []string{"grok", "agy"}, 2, "no_consensus"},
		{"proposerCannotVote", []string{"grok", "claude"}, []string{"grok", "agy"}, 2, "no_consensus"},
		{"missing", []string{"grok", "agy"}, []string{"grok"}, 1, "no_consensus"},
		{"unexpected", []string{"grok", "agy"}, []string{"grok", "codex"}, 2, "no_consensus"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := model.RunSnapshot{RolePlan: &model.RolePlan{Proposer: "claude", Reviewers: []string{"grok", "agy"}}}
			r := consensus.RunResult{TerminalState: consensus.TerminalStateCompleted, FreeDebate: &consensus.FreeDebateResultSection{Outcome: consensus.FreeDebateOutcomeConsensus, Claims: []consensus.DebateClaim{{ClaimID: "synthesis", Statement: "综合方案", Category: consensus.DebateClaimCategorySynthesis, Active: true}}, ClaimResolutions: []consensus.DebateClaimResolution{{ClaimID: "synthesis", Accepted: true, VoteCount: test.count, SupportingVoters: test.support}}, Voters: test.voters}}
			Finalize(&s, &r)
			if s.Decision.Outcome != test.outcome {
				t.Fatalf("wanted %s got %#v", test.outcome, s.Decision)
			}
		})
	}
}
