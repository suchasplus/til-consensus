package consensus

import (
	"strings"
	"testing"
)

func TestBuildRunSummaryGroupsFinalVoteAndFoldsMerged(t *testing.T) {
	result := &RunResult{
		RequestID: "tc_layout",
		Mode:      WorkflowModeFreeDebate,
		TaskSpec:  TaskSpec{Goal: "layout goal"},
		Report: AdjudicationReport{
			Summary:        "final conclusion text",
			RetainedClaims: []string{"claim-accepted"},
		},
		FreeDebate: &FreeDebateResultSection{
			Outcome: FreeDebateOutcomePartialConsensus,
			Claims: []DebateClaim{
				{ClaimID: "claim-accepted", Statement: "the accepted statement", Active: true},
				{ClaimID: "claim-low", Statement: "the low-support statement", Active: true},
				{ClaimID: "claim-silent", Statement: "the unvoted statement", Active: true},
				{ClaimID: "claim-merged", Statement: "the merged statement", Active: false, MergedInto: "claim-accepted"},
			},
			ClaimResolutions: []DebateClaimResolution{
				// deliberately unsorted: merged first, low support before accepted
				{ClaimID: "claim-merged", MergedInto: "claim-accepted", FinalStatement: "the merged statement"},
				{ClaimID: "claim-low", Accepted: false, SupportScore: 0.30, VoteCount: 2, OpposingVoters: []string{"a", "b"}, FinalStatement: "the low-support statement"},
				{ClaimID: "claim-silent", Accepted: false, FinalStatement: "the unvoted statement"},
				{ClaimID: "claim-accepted", Accepted: true, SupportScore: 0.90, VoteCount: 2, SupportingVoters: []string{"a", "b"}, FinalStatement: "the accepted statement"},
			},
		},
	}
	summary := BuildRunSummary(result)

	for _, fragment := range []string{
		"### Accepted (1)",
		"### Not Accepted (1)",
		"### No Votes (1)",
		"### Merged (1)",
		"- claim-merged → merged into claim-accepted",
		"- claim-accepted | support=0.90 | votes=2 (accept 2 / reject 0 / abstain 0)",
		"- claim-accepted — the accepted statement",
	} {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("summary missing fragment %q:\n%s", fragment, summary)
		}
	}

	// Merged claims must not render as rejected-with-zero-support lines.
	if strings.Contains(summary, "claim-merged | support=") {
		t.Fatalf("merged claim leaked into a voted-claim line:\n%s", summary)
	}

	// Conclusion and retained claims come before the vote detail.
	conclusionAt := strings.Index(summary, "## Conclusion")
	retainedAt := strings.Index(summary, "## Retained Claims")
	voteAt := strings.Index(summary, "## Final Vote")
	if conclusionAt < 0 || retainedAt < 0 || voteAt < 0 {
		t.Fatalf("expected all sections present, got positions %d/%d/%d:\n%s", conclusionAt, retainedAt, voteAt, summary)
	}
	if conclusionAt >= retainedAt || retainedAt >= voteAt {
		t.Fatalf("expected conclusion before retained claims before final vote, got positions %d/%d/%d", conclusionAt, retainedAt, voteAt)
	}

	// Accepted claims sort above lower-support ones inside the vote detail.
	acceptedLineAt := strings.Index(summary, "- claim-accepted | support=0.90")
	lowLineAt := strings.Index(summary, "- claim-low | support=0.30")
	if acceptedLineAt < 0 || lowLineAt < 0 || acceptedLineAt > lowLineAt {
		t.Fatalf("expected accepted claim rendered before low-support claim, got positions %d/%d", acceptedLineAt, lowLineAt)
	}
}

func TestBuildRunSummaryRendersRetainedClaimStatementsForDelphi(t *testing.T) {
	result := &RunResult{
		RequestID: "tc_delphi_layout",
		Mode:      WorkflowModeDelphi,
		TaskSpec:  TaskSpec{Goal: "delphi goal"},
		Report: AdjudicationReport{
			Summary:          "delphi conclusion",
			RetainedClaims:   []string{"claim_004", "claim-unknown"},
			DowngradedClaims: []string{"claim_005"},
		},
		Delphi: &DelphiResultSection{
			Statements: []DelphiStatement{
				{StatementID: "claim_004", Statement: "the retained delphi statement"},
				{StatementID: "claim_005", Statement: "the downgraded delphi statement"},
			},
		},
	}
	summary := BuildRunSummary(result)
	if !strings.Contains(summary, "- claim_004 — the retained delphi statement") {
		t.Fatalf("expected retained claim to carry statement text:\n%s", summary)
	}
	if !strings.Contains(summary, "- claim_005 — the downgraded delphi statement") {
		t.Fatalf("expected downgraded claim to carry statement text:\n%s", summary)
	}
	// Unknown IDs stay as bare IDs instead of breaking the render.
	if !strings.Contains(summary, "- claim-unknown") {
		t.Fatalf("expected unknown claim id to render bare:\n%s", summary)
	}
}
