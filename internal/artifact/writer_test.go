package artifact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestWriteRunArtifacts(t *testing.T) {
	tmp := t.TempDir()
	resultPath := filepath.Join(tmp, "result.json")
	summaryPath := filepath.Join(tmp, "summary.md")
	result := &consensus.ConsensusResult{
		ResultVersion: 1,
		RequestID:     "req-1",
		SessionID:     "session-1",
		Task:          consensus.ConsensusTask{Prompt: "task prompt", Title: "task title"},
		Status:        consensus.ConsensusStatusConsensus,
		FinalClaims: []consensus.Claim{
			{ClaimID: "c1", Title: "Claim 1", Statement: "Statement 1", Status: consensus.ClaimStatusActive, ProposedBy: []string{"a"}},
		},
		ClaimResolutions: []consensus.ClaimResolution{
			{ClaimID: "c1", Status: consensus.ClaimResolutionResolved, AcceptCount: 2, TotalVoters: 2},
		},
		Representative: consensus.Representative{ParticipantID: "a", Reason: consensus.RepresentativeReasonTopScore, Score: 88.5, Speech: "speech"},
		Scoreboard: []consensus.ParticipantScore{
			{ParticipantID: "a", Total: 88.5, Breakdown: &consensus.ParticipantScoreBreakdown{Correctness: 80}},
		},
		Report:  consensus.FinalReport{FinalSummary: "summary", RepresentativeSpeech: "speech"},
		Metrics: consensus.Metrics{ElapsedMs: 1000, TotalRounds: 3, TotalTurns: 6},
	}
	if err := WriteRunArtifacts(result, resultPath, summaryPath); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed consensus.ConsensusResult
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.ResultVersion != 1 {
		t.Fatalf("unexpected result version: %d", parsed.ResultVersion)
	}
	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(summary)
	if !strings.Contains(text, "## Conclusion") || !strings.Contains(text, "## Scoreboard") || !strings.Contains(text, "## Metrics") {
		t.Fatalf("unexpected summary content:\n%s", text)
	}
}
