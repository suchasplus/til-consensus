package consensus

import "testing"

func TestParseJSONExternalOutput(t *testing.T) {
	parsed, err := parseExternalSourceOutput(ExternalCommandParsing{
		Mode:        ExternalCommandParseModeJSON,
		SuccessPath: "status.ok",
		SummaryPath: "report.summary",
		ExcerptPath: "report.excerpt",
		NotesPath:   "report.notes",
		MetadataPaths: map[string]string{
			"score": "report.score",
		},
	}, `{"status":{"ok":true},"report":{"summary":"all good","excerpt":"detailed excerpt","notes":["n1","n2"],"score":0.9}}`)
	if err != nil {
		t.Fatalf("parseExternalSourceOutput failed: %v", err)
	}
	if !parsed.HasSuccess || !parsed.Success {
		t.Fatalf("expected success=true, got %#v", parsed)
	}
	if parsed.Summary != "all good" || parsed.Excerpt != "detailed excerpt" {
		t.Fatalf("unexpected summary/excerpt: %#v", parsed)
	}
	if len(parsed.Notes) != 2 || parsed.Metadata["score"] != float64(0.9) {
		t.Fatalf("unexpected notes/metadata: %#v", parsed)
	}
}

func TestParseJSONExternalOutputFailurePath(t *testing.T) {
	parsed, err := parseExternalSourceOutput(ExternalCommandParsing{
		Mode:        ExternalCommandParseModeJSON,
		FailurePath: "health.failed",
		SummaryPath: "message",
	}, `{"health":{"failed":true},"message":"probe reported broken"}`)
	if err != nil {
		t.Fatalf("parseExternalSourceOutput failed: %v", err)
	}
	if !parsed.HasFailure || !parsed.Failure {
		t.Fatalf("expected failure=true, got %#v", parsed)
	}
	if parsed.Summary != "probe reported broken" {
		t.Fatalf("unexpected summary: %#v", parsed)
	}
}

func TestJSONPathLookupSupportsArrayIndex(t *testing.T) {
	root := map[string]any{
		"items": []any{
			map[string]any{"name": "first"},
			map[string]any{"name": "second"},
		},
	}
	value, ok := jsonPathLookup(root, "items[1].name")
	if !ok || value != "second" {
		t.Fatalf("unexpected lookup result: ok=%t value=%#v", ok, value)
	}
}
