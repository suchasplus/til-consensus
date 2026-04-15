package runtime

import "testing"

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

func TestParseJSONObjectFailure(t *testing.T) {
	if _, err := ParseJSONObject("not json"); err == nil {
		t.Fatal("expected parse failure")
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
