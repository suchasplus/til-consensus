package runtime

import (
	"strings"
	"testing"
)

func TestExtractJSONObject(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", `{"a":1}`, `{"a":1}`},
		{"code_fence", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"noise", "hello\n{\"a\":1}\nworld", `{"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractJSONObject(tt.input); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestParseJSONObjectRecovery(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"trailing_comma", `{"a":1,}`},
		{"stray_quote", `{"a":"hello "world""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseJSONObject(tt.input); err != nil {
				t.Fatalf("expected recovery, got %v", err)
			}
		})
	}
}

func TestParseJSONObjectDoesNotEnforceTaskSchema(t *testing.T) {
	value, err := ParseJSONObject(`{"summary":"proposal","claims":[{"claim":"alias field","confidence":"medium"}]}`)
	if err != nil {
		t.Fatalf("expected syntax-only parse to succeed, got %v", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected parsed object, got %T", value)
	}
	claims, ok := object["claims"].([]any)
	if !ok || len(claims) != 1 {
		t.Fatalf("unexpected parsed claims: %#v", object["claims"])
	}
	first, ok := claims[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected parsed claim type: %T", claims[0])
	}
	if first["claim"] != "alias field" || first["confidence"] != "medium" {
		t.Fatalf("expected raw alias fields to survive syntax parsing, got %#v", first)
	}
}

func TestStrictJSONObjectBytes(t *testing.T) {
	if _, err := StrictJSONObjectBytes(`{"a":1}`); err != nil {
		t.Fatalf("expected strict object to pass, got %v", err)
	}
	for _, input := range []string{
		"```json\n{\"a\":1}\n```",
		"noise\n{\"a\":1}",
		"{\"a\":1}\ntrailing",
	} {
		if _, err := StrictJSONObjectBytes(input); err == nil {
			t.Fatalf("expected strict parser to reject %q", input)
		}
	}
}

func TestParseJSONObjectFailure(t *testing.T) {
	_, err := ParseJSONObject("not json")
	if err == nil {
		t.Fatal("expected parse failure")
	}
	parseErr, ok := err.(*JSONParseError)
	if !ok {
		t.Fatalf("expected JSONParseError, got %T", err)
	}
	if parseErr.RawText != "not json" {
		t.Fatalf("unexpected raw text: %#v", parseErr)
	}
	if parseErr.ExtractedCandidate != "" {
		t.Fatalf("expected empty extracted candidate, got %#v", parseErr)
	}
}

func TestParseJSONObjectFailureKeepsCandidate(t *testing.T) {
	_, err := ParseJSONObject("prefix\n{\"a\":1 trailing")
	if err == nil {
		t.Fatal("expected parse failure")
	}
	parseErr, ok := err.(*JSONParseError)
	if !ok {
		t.Fatalf("expected JSONParseError, got %T", err)
	}
	if !strings.Contains(parseErr.RawText, "prefix") {
		t.Fatalf("expected raw text in parse error, got %#v", parseErr)
	}
}

func FuzzParseJSONObject(f *testing.F) {
	seeds := []string{
		`{"a":1}`,
		"```json\n{\"a\":1}\n```",
		"noise {\"a\":1} tail",
		`{"a":1,}`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = ParseJSONObject(input)
	})
}
