package config

import (
	"strings"
	"testing"
)

func TestValidateCLIEnvironment(t *testing.T) {
	values := func(items ...string) *[]string { return &items }
	tests := []struct {
		name    string
		pass    *[]string
		env     map[string]string
		wantErr string
	}{
		{name: "valid", pass: values("GEMINI_API_KEY", "CLAUDE_CODE_USE_BEDROCK"), env: map[string]string{"CUSTOM_ENDPOINT": "https://example.com"}},
		{name: "empty", pass: values(""), wantErr: "not a valid"},
		{name: "equals", pass: values("KEY=value"), wantErr: "not a valid"},
		{name: "glob", pass: values("OPENAI_*"), wantErr: "not a valid"},
		{name: "leading digit", pass: values("1TOKEN"), wantErr: "not a valid"},
		{name: "duplicate", pass: values("TOKEN", "token"), wantErr: "duplicate"},
		{name: "denied passthrough", pass: values("CLAUDECODE"), wantErr: "permanently denied"},
		{name: "denied configured", env: map[string]string{"claudecode": "1"}, wantErr: "permanently denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCLIEnvironment("cli", ProviderConfig{EnvPassthrough: tt.pass, Env: tt.env})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
