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

func TestGrokEnvironmentOnlyRequiresXAIAPIKeyPassthrough(t *testing.T) {
	host := snapshotHostEnvironment([]string{"PATH=/usr/bin", "XAI_API_KEY=secret", "OPENAI_API_KEY=unrelated", "XAI_BASE_URL=https://example.invalid"}, false)
	provider := config.ProviderConfig{CLIType: config.CLITypeGrok}
	built := BuildEnvironment(host, provider, environmentTestTask{}, "agent", "participant", "grok-4.7-build")
	if strings.Join(built.MissingKnownKeys, ",") != "XAI_API_KEY" {
		t.Fatalf("unexpected Grok required passthrough: %v", built.MissingKnownKeys)
	}
	provider.EnvPassthrough = &[]string{"XAI_API_KEY"}
	built = BuildEnvironment(host, provider, environmentTestTask{}, "agent", "participant", "grok-4.7-build")
	if len(built.MissingKnownKeys) != 0 || environmentValuesMap(built.Values)["XAI_API_KEY"] != "secret" {
		t.Fatal("explicit Grok credential passthrough was not applied")
	}
	for _, unrelated := range []string{"OPENAI_API_KEY", "XAI_BASE_URL"} {
		if _, exists := environmentValuesMap(built.Values)[unrelated]; exists {
			t.Fatalf("undeclared %s leaked into Grok environment", unrelated)
		}
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

func TestBuildEnvironmentRecordsDroppedCredentialKeys(t *testing.T) {
	host := snapshotHostEnvironment([]string{
		"PATH=/usr/bin",
		"MY_CUSTOM_TOKEN=super-secret",
		"CORP_API_KEY=another-secret",
		"LEGACY_BASE_URL=https://user:pw@internal.example.com/v1?k=v",
		"OPENAI_API_KEY=passed-through",
		"EDITOR=vim",
	}, false)
	provider := config.ProviderConfig{
		Type:           config.ProviderTypeCLI,
		CLIType:        config.CLITypeGeneric,
		EnvPassthrough: &[]string{"OPENAI_API_KEY"},
	}
	built := BuildEnvironment(host, provider, consensus.ProposalTask{}, "agent-1", "role", "model-x")

	want := []string{"CORP_API_KEY", "LEGACY_BASE_URL", "MY_CUSTOM_TOKEN"}
	if strings.Join(built.Audit.DroppedCredentialKeys, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected dropped credential keys: %#v", built.Audit.DroppedCredentialKeys)
	}
	for _, key := range built.Audit.DroppedCredentialKeys {
		if strings.Contains(key, "=") {
			t.Fatalf("dropped credential entry must be a bare key name, got %q", key)
		}
	}
	// Values must never leak into the audit record.
	for _, secret := range []string{"super-secret", "another-secret", "user:pw"} {
		if strings.Contains(strings.Join(built.Audit.DroppedCredentialKeys, " "), secret) {
			t.Fatalf("audit leaked secret value %q", secret)
		}
	}
	// Passed-through and non-credential keys must not be reported as dropped.
	for _, key := range []string{"OPENAI_API_KEY", "EDITOR", "PATH"} {
		for _, dropped := range built.Audit.DroppedCredentialKeys {
			if dropped == key {
				t.Fatalf("key %q should not be reported as dropped", key)
			}
		}
	}
	// Diagnostic only: a generic provider must still dispatch.
	if err := EnvironmentMigrationError("generic-local", built.MissingKnownKeys); err != nil {
		t.Fatalf("dropped credential keys must not block dispatch: %v", err)
	}
}
