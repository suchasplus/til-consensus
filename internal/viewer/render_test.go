package viewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestRenderDocumentGolden(t *testing.T) {
	bundle := loadSampleBundle(t)
	tests := []struct {
		name    string
		format  string
		verbose bool
		golden  string
	}{
		{name: "text", format: FormatText, verbose: true, golden: "view.text.golden"},
		{name: "markdown", format: FormatMarkdown, verbose: true, golden: "view.markdown.golden"},
		{name: "json", format: FormatJSON, verbose: false, golden: "view.json.golden"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := BuildDocument(bundle, RenderOptions{
				Format:  tc.format,
				Limit:   20,
				Verbose: tc.verbose,
			})
			got, err := RenderDocument(doc, RenderOptions{
				Format:  tc.format,
				Verbose: tc.verbose,
			})
			if err != nil {
				t.Fatalf("RenderDocument failed: %v", err)
			}
			assertGolden(t, tc.golden, got)
		})
	}
}

func TestBuildDocumentFiltersClaimsAndLimits(t *testing.T) {
	bundle := loadSampleBundle(t)
	doc := BuildDocument(bundle, RenderOptions{
		Format:       FormatText,
		ClaimVerdict: consensus.ClaimVerdictSupported,
		Limit:        1,
	})
	if len(doc.Claims) != 1 {
		t.Fatalf("expected one claim, got %d", len(doc.Claims))
	}
	if doc.Claims[0].Verdict != consensus.ClaimVerdictSupported {
		t.Fatalf("unexpected verdict: %s", doc.Claims[0].Verdict)
	}
}

func TestRenderDocumentSections(t *testing.T) {
	bundle := loadSampleBundle(t)
	tests := []struct {
		name     string
		sections []string
		contains []string
		excludes []string
	}{
		{
			name:     "claims only",
			sections: []string{SectionClaims},
			contains: []string{"关键 Claims", "Race fix"},
			excludes: []string{"运行头部", "相关文件"},
		},
		{
			name:     "artifacts only",
			sections: []string{SectionArtifacts},
			contains: []string{"相关文件", "artifacts/unit-tests.log"},
			excludes: []string{"关键 Claims", "任务摘要"},
		},
		{
			name:     "verifications only",
			sections: []string{SectionVerifications},
			contains: []string{"风险与未决项", "验证明细", "benchmark 样本不足"},
			excludes: []string{"关键 Claims", "相关文件"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := BuildDocument(bundle, RenderOptions{
				Format:   FormatText,
				Sections: tc.sections,
				Limit:    20,
				Verbose:  true,
			})
			got, err := RenderDocument(doc, RenderOptions{Format: FormatText, Verbose: true})
			if err != nil {
				t.Fatalf("RenderDocument failed: %v", err)
			}
			for _, needle := range tc.contains {
				if !strings.Contains(got, needle) {
					t.Fatalf("expected output to contain %q\n%s", needle, got)
				}
			}
			for _, needle := range tc.excludes {
				if strings.Contains(got, needle) {
					t.Fatalf("expected output to exclude %q\n%s", needle, got)
				}
			}
		})
	}
}

func TestLoadBundleMissingManifestDegrades(t *testing.T) {
	source := sampleRunDir(t)
	tmp := t.TempDir()
	for _, name := range []string{"result.json", "ledger.jsonl", "summary.md"} {
		body, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(tmp, name), body, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	bundle, err := LoadBundle(InferRunFiles(filepath.Join(tmp, "result.json")))
	if err != nil {
		t.Fatalf("LoadBundle failed: %v", err)
	}
	if len(bundle.Missing) != 1 || bundle.Missing[0] != "artifacts/manifest.jsonl" {
		t.Fatalf("unexpected missing files: %#v", bundle.Missing)
	}
}

func TestBuildDocumentSupportsFreeDebateAndDelphi(t *testing.T) {
	debateBundle := Bundle{
		Result: consensus.RunResult{
			SchemaVersion: 2,
			Mode:          consensus.WorkflowModeFreeDebate,
			RequestID:     "req-debate",
			SessionID:     "session-debate",
			TaskSpec:      consensus.TaskSpec{Goal: "debate goal"},
			Report:        consensus.AdjudicationReport{Summary: "debate summary"},
			FreeDebate: &consensus.FreeDebateResultSection{
				Outcome: consensus.FreeDebateOutcomeConsensus,
				Rounds: []consensus.DebateRoundRecord{
					{Round: 0, Phase: "initial", Summary: "initial"},
					{Round: 1, Phase: "debate", Summary: "debate"},
				},
				Votes: []consensus.DebateVoteRecord{{ClaimID: "c1", AgentID: "a1", Vote: consensus.DebateVoteAccept}},
			},
		},
		Files: RunFiles{RunDir: "sample-run", ResultPath: "sample-run/result.json", LedgerPath: "sample-run/ledger.jsonl", SummaryPath: "sample-run/summary.md", ManifestPath: "sample-run/artifacts/manifest.jsonl"},
	}
	debateDoc := BuildDocument(debateBundle, RenderOptions{Format: FormatText})
	if len(debateDoc.Rounds) != 2 || len(debateDoc.Votes) != 1 {
		t.Fatalf("unexpected debate doc: %#v", debateDoc)
	}

	delphiBundle := Bundle{
		Result: consensus.RunResult{
			SchemaVersion: 2,
			Mode:          consensus.WorkflowModeDelphi,
			RequestID:     "req-delphi",
			SessionID:     "session-delphi",
			TaskSpec:      consensus.TaskSpec{Goal: "delphi goal"},
			Report:        consensus.AdjudicationReport{Summary: "delphi summary"},
			Delphi: &consensus.DelphiResultSection{
				ConsensusLevel: 0.82,
				Recommendation: "Use monorepo",
				Rounds:         []consensus.DelphiRoundRecord{{Round: 1, Phase: "delphi_questionnaire"}},
				Statements:     []consensus.DelphiStatement{{StatementID: "s1", Statement: "Use monorepo", MeanRating: 4.5, ConsensusLevel: 0.82}},
			},
		},
		Files: RunFiles{RunDir: "sample-run", ResultPath: "sample-run/result.json", LedgerPath: "sample-run/ledger.jsonl", SummaryPath: "sample-run/summary.md", ManifestPath: "sample-run/artifacts/manifest.jsonl"},
	}
	delphiDoc := BuildDocument(delphiBundle, RenderOptions{Format: FormatText})
	if delphiDoc.Convergence == nil || delphiDoc.Convergence.Recommendation != "Use monorepo" {
		t.Fatalf("unexpected delphi doc: %#v", delphiDoc)
	}
}

func loadSampleBundle(t *testing.T) Bundle {
	t.Helper()
	root := sampleRunDir(t)
	bundle, err := LoadBundle(RunFiles{
		RunDir:       root,
		ResultPath:   filepath.Join(root, "result.json"),
		LedgerPath:   filepath.Join(root, "ledger.jsonl"),
		SummaryPath:  filepath.Join(root, "summary.md"),
		ManifestPath: filepath.Join(root, "artifacts", "manifest.jsonl"),
	})
	if err != nil {
		t.Fatalf("LoadBundle failed: %v", err)
	}
	return bundle
}

func sampleRunDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "view", "sample-run"))
	if err != nil {
		t.Fatalf("resolve sample dir: %v", err)
	}
	return root
}

func assertGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "view", "golden", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(want) != got {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
	}
}
