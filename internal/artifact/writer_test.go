package artifact

import (
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
	result := &consensus.AdjudicationResult{
		SchemaVersion: 1,
		RequestID:     "req-1",
		SessionID:     "session-1",
		TaskSpec: consensus.TaskSpec{
			Goal: "verify patch",
		},
		TaskVerdict: consensus.TaskVerdictSupported,
		ClaimGraph: []consensus.ClaimNode{
			{
				ClaimID:   "claim-1",
				Title:     "Race fix",
				Statement: "Patch fixes the race",
				Verdict:   consensus.ClaimVerdictSupported,
			},
		},
		Report: consensus.AdjudicationReport{
			Summary: "裁决完成",
		},
		Metrics: consensus.Metrics{
			ClaimsProposed:   1,
			ChallengesOpened: 1,
			VerificationsRun: 1,
			TasksDispatched:  2,
		},
	}
	if err := WriteRunArtifacts(result, resultPath, summaryPath); err != nil {
		t.Fatalf("WriteRunArtifacts failed: %v", err)
	}
	if _, err := os.Stat(resultPath); err != nil {
		t.Fatalf("result artifact missing: %v", err)
	}
	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	text := string(summary)
	for _, needle := range []string{"task verdict: supported", "Race fix", "裁决完成", "claims proposed: 1"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("summary missing %q\n%s", needle, text)
		}
	}
}
