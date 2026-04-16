package artifact_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/artifact"
	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestScenarioExpectedSummaryFragments(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "scenarios")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read scenarios: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			input, err := config.LoadRunInput(filepath.Join(root, name, "run.yaml"))
			if err != nil {
				t.Fatalf("LoadRunInput failed: %v", err)
			}
			expectedBody, err := os.ReadFile(filepath.Join(root, name, "expected-summary.txt"))
			if err != nil {
				if os.IsNotExist(err) {
					t.Skip("no static expected-summary fixture")
				}
				t.Fatalf("read expected summary: %v", err)
			}
			summary := artifact.BuildSummary(&consensus.RunResult{
				SchemaVersion: 2,
				Mode:          consensus.WorkflowModeAdjudication,
				RequestID:     "req-1",
				TaskSpec: consensus.TaskSpec{
					Goal: input.TaskSpec.Goal,
				},
				Adjudication: &consensus.AdjudicationResultSection{
					TaskVerdict: consensus.TaskVerdictUndetermined,
					ClaimGraph: []consensus.ClaimNode{
						{
							ClaimID:   "claim-1",
							Title:     "Sample claim",
							Statement: "Sample statement",
							Verdict:   consensus.ClaimVerdictUndetermined,
						},
					},
				},
				Report: consensus.AdjudicationReport{
					Summary: strings.Join([]string{
						"benchmark evidence available",
						"semantic verifier used",
						"task verdict: undetermined",
					}, "\n"),
				},
				Metrics: consensus.Metrics{
					ClaimsProposed:   1,
					ChallengesOpened: 1,
					VerificationsRun: 1,
					TasksDispatched:  1,
				},
			})
			for _, line := range strings.Split(strings.TrimSpace(string(expectedBody)), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if !strings.Contains(summary, line) {
					t.Fatalf("summary for %s missing fragment %q\n%s", name, line, summary)
				}
			}
		})
	}
}
