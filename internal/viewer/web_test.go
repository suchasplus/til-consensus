package viewer

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestRenderWebPageGolden(t *testing.T) {
	got, err := renderWebPage(WebPageModel{
		Title:          "til-consensus viewer",
		RequestID:      "req-1",
		Mode:           "adjudication",
		PrimaryResult:  "supported",
		TerminalState:  "completed",
		Goal:           "判断 patch 是否真正修复竞态问题",
		URL:            "http://127.0.0.1:43123",
		APIPath:        "/api/document",
		InitialSection: "all",
		ClaimVerdict:   "",
		Limit:          20,
		Verbose:        true,
	})
	if err != nil {
		t.Fatalf("renderWebPage failed: %v", err)
	}
	assertGolden(t, "web.index.golden", got)
}

func TestWebHandlerServesIndexAndDocumentAPI(t *testing.T) {
	bundle := bundleWithObservationData(t)
	handler, err := newWebHandler(bundle, RenderOptions{
		Format:  FormatText,
		Limit:   20,
		Verbose: true,
	}, "http://127.0.0.1:43123")
	if err != nil {
		t.Fatalf("newWebHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	indexResp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer func() { _ = indexResp.Body.Close() }()
	indexBody, err := ioReadAll(indexResp)
	if err != nil {
		t.Fatalf("read index body: %v", err)
	}
	for _, needle := range []string{"Overview", "Claims", "Evidence", "Observations", "Follow-ups", "Files"} {
		if !strings.Contains(indexBody, needle) {
			t.Fatalf("expected index to contain %q\n%s", needle, indexBody)
		}
	}

	healthResp, err := http.Get(server.URL + "/api/healthz")
	if err != nil {
		t.Fatalf("GET /api/healthz failed: %v", err)
	}
	defer func() { _ = healthResp.Body.Close() }()
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected healthz status: %d", healthResp.StatusCode)
	}

	docResp, err := http.Get(server.URL + "/api/document?section=claims&claim_verdict=supported&limit=1&verbose=true")
	if err != nil {
		t.Fatalf("GET /api/document failed: %v", err)
	}
	defer func() { _ = docResp.Body.Close() }()
	var doc Document
	if err := json.NewDecoder(docResp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode document: %v", err)
	}
	if len(doc.RequestedSections) == 0 || doc.RequestedSections[0] != SectionClaims {
		t.Fatalf("unexpected requested sections: %#v", doc.RequestedSections)
	}
	if len(doc.Claims) != 1 || doc.Claims[0].Verdict != consensus.ClaimVerdictSupported {
		t.Fatalf("unexpected claims: %#v", doc.Claims)
	}
}

func TestWebHandlerMissingManifestDegrades(t *testing.T) {
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
	handler, err := newWebHandler(bundle, RenderOptions{Limit: 20}, "http://127.0.0.1:43123")
	if err != nil {
		t.Fatalf("newWebHandler failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/document")
	if err != nil {
		t.Fatalf("GET /api/document failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var doc Document
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode document: %v", err)
	}
	if len(doc.Files.Missing) != 1 || doc.Files.Missing[0] != "artifacts/manifest.jsonl" {
		t.Fatalf("unexpected missing files: %#v", doc.Files.Missing)
	}
}

func TestWebHandlerSupportsDebateAndDelphi(t *testing.T) {
	tests := []struct {
		name   string
		bundle Bundle
		check  func(t *testing.T, doc Document)
	}{
		{
			name: "debate",
			bundle: Bundle{
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
			},
			check: func(t *testing.T, doc Document) {
				if len(doc.Rounds) != 2 || len(doc.Votes) != 1 {
					t.Fatalf("unexpected debate web doc: %#v", doc)
				}
			},
		},
		{
			name: "delphi",
			bundle: Bundle{
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
			},
			check: func(t *testing.T, doc Document) {
				if len(doc.Statements) != 1 || doc.Convergence == nil || doc.Convergence.Recommendation != "Use monorepo" {
					t.Fatalf("unexpected delphi web doc: %#v", doc)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := newWebHandler(tc.bundle, RenderOptions{Limit: 20}, "http://127.0.0.1:43123")
			if err != nil {
				t.Fatalf("newWebHandler failed: %v", err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			resp, err := http.Get(server.URL + "/api/document")
			if err != nil {
				t.Fatalf("GET /api/document failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			var doc Document
			if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
				t.Fatalf("decode document: %v", err)
			}
			tc.check(t, doc)
		})
	}
}

func ioReadAll(resp *http.Response) (string, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
