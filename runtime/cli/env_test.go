package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
)

func TestBuildEnvironmentAppliesAllowlistAndPrecedence(t *testing.T) {
	passthrough := []string{"OPENAI_API_KEY", "CUSTOM_ENDPOINT"}
	host := snapshotHostEnvironment([]string{
		"PATH=/usr/bin",
		"HOME=/home/test",
		"OPENAI_API_KEY=host-secret",
		"OPENAI_BASE_URL=https://gateway.example.com/v1",
		"UNDECLARED_SECRET=must-not-pass",
		"CUSTOM_ENDPOINT=https://user:password@example.com/v1?token=secret#fragment",
		"CLAUDECODE=must-never-pass",
	}, false)
	task := environmentTestTask{TaskMeta: consensus.TaskMeta{
		RequestID: "request-1",
		SessionID: "session-1",
		AgentID:   "task-agent",
	}}
	built := BuildEnvironment(host, config.ProviderConfig{
		CLIType:        config.CLITypeCodex,
		EnvPassthrough: &passthrough,
		Env: map[string]string{
			"OPENAI_API_KEY":           "configured-secret",
			"CUSTOM_VALUE":             "{requestId}/{agentId}/{providerModel}",
			"TIL_CONSENSUS_REQUEST_ID": "must-be-overridden",
			"CLAUDE_CODE_USE_BEDROCK":  "1",
		},
	}, task, "agent-1", "proposer", "gpt-test")

	values := environmentValuesMap(built.Values)
	for key, want := range map[string]string{
		"PATH":                         "/usr/bin",
		"HOME":                         "/home/test",
		"OPENAI_API_KEY":               "configured-secret",
		"CUSTOM_VALUE":                 "request-1/task-agent/gpt-test",
		"TIL_CONSENSUS_REQUEST_ID":     "request-1",
		"TIL_CONSENSUS_SESSION_ID":     "session-1",
		"TIL_CONSENSUS_AGENT_ID":       "agent-1",
		"TIL_CONSENSUS_ROLE":           "proposer",
		"TIL_CONSENSUS_PROVIDER_MODEL": "gpt-test",
		"CLAUDE_CODE_USE_BEDROCK":      "1",
	} {
		if values[key] != want {
			t.Fatalf("env %s: got=%q want=%q\nall=%#v", key, values[key], want, values)
		}
	}
	for _, denied := range []string{"UNDECLARED_SECRET", "CLAUDECODE"} {
		if _, ok := values[denied]; ok {
			t.Fatalf("unexpected denied env %s in %#v", denied, values)
		}
	}
	if len(built.MissingKnownKeys) != 1 || built.MissingKnownKeys[0] != "OPENAI_BASE_URL" {
		t.Fatalf("unexpected missing known keys: %#v", built.MissingKnownKeys)
	}
	if got := built.Audit.Endpoints["CUSTOM_ENDPOINT"]; got != "https://example.com/v1" {
		t.Fatalf("sanitized endpoint: got=%q", got)
	}
	auditBody, err := json.Marshal(built.Audit)
	if err != nil {
		t.Fatalf("marshal audit: %v", err)
	}
	for _, secret := range []string{"host-secret", "configured-secret", "password", "token=secret"} {
		if strings.Contains(string(auditBody), secret) {
			t.Fatalf("audit leaked %q: %s", secret, auditBody)
		}
	}
}

func TestBuildEnvironmentWindowsKeysAreCaseInsensitive(t *testing.T) {
	passthrough := []string{"custom_token"}
	host := snapshotHostEnvironment([]string{
		"Path=C:\\Windows\\System32",
		"SYSTEMROOT=C:\\Windows",
		"Custom_Token=secret",
	}, true)
	built := BuildEnvironment(host, config.ProviderConfig{CLIType: config.CLITypeGeneric, EnvPassthrough: &passthrough}, environmentTestTask{}, "agent", "", "model")
	values := environmentValuesMap(built.Values)
	if values["Path"] != `C:\Windows\System32` || values["SYSTEMROOT"] != `C:\Windows` || values["Custom_Token"] != "secret" {
		t.Fatalf("unexpected Windows environment: %#v", values)
	}
}

func TestEnvironmentMigrationErrorDoesNotContainValues(t *testing.T) {
	err := EnvironmentMigrationError("pi-cli", []string{"GEMINI_API_KEY"})
	if err == nil || !strings.Contains(err.Error(), "pi-cli") || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Fatalf("unexpected migration error: %v", err)
	}
}

func TestKnownEnvironmentKeysDistinguishesPiGoogleProviders(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "gemini api default", want: []string{"GEMINI_API_KEY"}},
		{name: "vertex pair", args: []string{"--provider", "google-vertex"}, want: []string{"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION"}},
		{name: "vertex equals", args: []string{"--provider=google-vertex"}, want: []string{"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := knownEnvironmentKeys(config.ProviderConfig{CLIType: config.CLITypePi, Args: tt.args})
			if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
				t.Fatalf("known keys: got=%v want=%v", got, tt.want)
			}
		})
	}
}

func environmentValuesMap(values []string) map[string]string {
	out := make(map[string]string, len(values))
	for _, value := range values {
		key, item, _ := strings.Cut(value, "=")
		out[key] = item
	}
	return out
}

type environmentTestTask struct {
	consensus.TaskMeta
}

func (environmentTestTask) Kind() consensus.TaskKind { return consensus.TaskKind("env-test") }
func (t environmentTestTask) Meta() consensus.TaskMeta {
	return t.TaskMeta
}
