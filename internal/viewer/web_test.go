package viewer

import (
	"encoding/json"
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
	indexReq := httptest.NewRequest("GET", "/", nil)
	indexResp := httptest.NewRecorder()
	handler.ServeHTTP(indexResp, indexReq)
	indexBody := indexResp.Body.String()
	for _, needle := range []string{"Overview", "Claims", "Evidence", "Observations", "Follow-ups", "Debug", "Files"} {
		if !strings.Contains(indexBody, needle) {
			t.Fatalf("expected index to contain %q\n%s", needle, indexBody)
		}
	}

	healthReq := httptest.NewRequest("GET", "/api/healthz", nil)
	healthResp := httptest.NewRecorder()
	handler.ServeHTTP(healthResp, healthReq)
	if healthResp.Code != 200 {
		t.Fatalf("unexpected healthz status: %d", healthResp.Code)
	}

	docReq := httptest.NewRequest("GET", "/api/document?section=claims&claim_verdict=supported&limit=1&verbose=true", nil)
	docResp := httptest.NewRecorder()
	handler.ServeHTTP(docResp, docReq)
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

	debugReq := httptest.NewRequest("GET", "/api/document?section=debug&verbose=true", nil)
	debugResp := httptest.NewRecorder()
	handler.ServeHTTP(debugResp, debugReq)
	var debugDoc Document
	if err := json.NewDecoder(debugResp.Body).Decode(&debugDoc); err != nil {
		t.Fatalf("decode debug document: %v", err)
	}
	if len(debugDoc.DebugEvents) == 0 {
		t.Fatalf("expected debug events in web document: %#v", debugDoc)
	}
	if debugDoc.DebugEvents[0].PayloadPretty == "" {
		t.Fatalf("expected pretty payload in debug event: %#v", debugDoc.DebugEvents[0])
	}
	var sawRawVerdict bool
	var sawRawTaskVerdict bool
	for _, item := range debugDoc.DebugEvents {
		if item.RawVerdict == "rejected" {
			sawRawVerdict = true
		}
		if strings.Contains(item.RawTaskVerdict, "\"verdict\":\"undetermined\"") {
			sawRawTaskVerdict = true
		}
	}
	if !sawRawVerdict || !sawRawTaskVerdict {
		t.Fatalf("expected raw verdict fields in web debug document: %#v", debugDoc.DebugEvents)
	}
	if debugDoc.Telemetry == nil || len(debugDoc.Telemetry.Summary) == 0 || len(debugDoc.Telemetry.Reports) == 0 {
		t.Fatalf("expected telemetry in web debug document: %#v", debugDoc.Telemetry)
	}
	if debugDoc.Telemetry.Reports[0].FinalStatus == "" {
		t.Fatalf("expected telemetry report final status: %#v", debugDoc.Telemetry.Reports[0])
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
	req := httptest.NewRequest("GET", "/api/document", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
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
			req := httptest.NewRequest("GET", "/api/document", nil)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			var doc Document
			if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
				t.Fatalf("decode document: %v", err)
			}
			tc.check(t, doc)
		})
	}
}
