package cli

import (
	"fmt"
	"net/url"
	"runtime"
	"sort"
	"strings"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
)

var baselineEnvironmentKeys = []string{
	"PATH", "HOME", "USER", "LOGNAME", "SHELL",
	"TMPDIR", "TMP", "TEMP",
	"LANG", "LC_ALL", "LC_CTYPE", "TZ",
	"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR",
	"SSL_CERT_FILE", "SSL_CERT_DIR", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE", "NODE_EXTRA_CA_CERTS", "GRPC_DEFAULT_SSL_ROOTS_FILE_PATH",
	"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
	"http_proxy", "https_proxy", "all_proxy", "no_proxy",
}

var windowsBaselineEnvironmentKeys = []string{
	"SystemRoot", "ComSpec", "PATHEXT", "USERPROFILE", "APPDATA", "LOCALAPPDATA",
}

var knownCLIEnvironmentKeys = map[string][]string{
	config.CLITypeCodex:       {"OPENAI_API_KEY", "OPENAI_BASE_URL"},
	config.CLITypeClaude:      {"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL"},
	config.CLITypeGrok:        {"XAI_API_KEY"},
	config.CLITypeGemini:      {"GEMINI_API_KEY"},
	config.CLITypeAntigravity: {"GEMINI_API_KEY"},
}

type hostEnvironmentValue struct {
	Key   string
	Value string
}

type HostEnvironment struct {
	values          map[string]hostEnvironmentValue
	caseInsensitive bool
}

type EnvironmentAudit struct {
	BaselineKeys    []string          `json:"baselineKeys,omitempty"`
	PassthroughKeys []string          `json:"passthroughKeys,omitempty"`
	ConfiguredKeys  []string          `json:"configuredKeys,omitempty"`
	MetadataKeys    []string          `json:"metadataKeys,omitempty"`
	Endpoints       map[string]string `json:"endpoints,omitempty"`
	// DroppedCredentialKeys names host variables that look like credentials or
	// endpoint overrides and were NOT passed to the CLI. It is diagnostic only
	// and never blocks a run: MissingKnownKeys is the curated, high-precision
	// check that fails dispatch, while this is the low-precision catch-all that
	// covers cli types we cannot enumerate keys for (generic, opencode).
	// Only key names are recorded — values never reach disk.
	DroppedCredentialKeys []string `json:"droppedCredentialKeys,omitempty"`
}

type BuiltEnvironment struct {
	Values           []string
	Audit            EnvironmentAudit
	MissingKnownKeys []string
}

type environmentValue struct {
	Key    string
	Value  string
	Source string
}

func SnapshotHostEnvironment(environ []string) HostEnvironment {
	return snapshotHostEnvironment(environ, runtime.GOOS == "windows")
}

func snapshotHostEnvironment(environ []string, caseInsensitive bool) HostEnvironment {
	host := HostEnvironment{values: make(map[string]hostEnvironmentValue, len(environ)), caseInsensitive: caseInsensitive}
	for _, item := range environ {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		host.values[host.canonicalKey(key)] = hostEnvironmentValue{Key: key, Value: value}
	}
	return host
}

func (h HostEnvironment) canonicalKey(key string) string {
	if h.caseInsensitive {
		return strings.ToUpper(key)
	}
	return key
}

func (h HostEnvironment) lookup(key string) (hostEnvironmentValue, bool) {
	value, ok := h.values[h.canonicalKey(key)]
	return value, ok
}

func BuildEnvironment(host HostEnvironment, provider config.ProviderConfig, task consensus.Task, agentID string, role string, providerModel string) BuiltEnvironment {
	values := map[string]environmentValue{}
	set := func(key string, value string, source string) {
		if strings.EqualFold(key, "CLAUDECODE") {
			return
		}
		values[host.canonicalKey(key)] = environmentValue{Key: key, Value: value, Source: source}
	}
	for _, key := range baselineEnvironmentKeys {
		if value, ok := host.lookup(key); ok {
			set(value.Key, value.Value, "baseline")
		}
	}
	if host.caseInsensitive {
		for _, key := range windowsBaselineEnvironmentKeys {
			if value, ok := host.lookup(key); ok {
				set(value.Key, value.Value, "baseline")
			}
		}
	}
	if provider.EnvPassthrough != nil {
		for _, key := range *provider.EnvPassthrough {
			if value, ok := host.lookup(key); ok {
				set(value.Key, value.Value, "passthrough")
			}
		}
	}
	for key, value := range provider.Env {
		set(key, renderTemplate(value, task, agentID, role, providerModel), "configured")
	}
	meta := task.Meta()
	metadata := map[string]string{
		"TIL_CONSENSUS_TASK_KIND":      string(task.Kind()),
		"TIL_CONSENSUS_REQUEST_ID":     meta.RequestID,
		"TIL_CONSENSUS_SESSION_ID":     meta.SessionID,
		"TIL_CONSENSUS_AGENT_ID":       agentID,
		"TIL_CONSENSUS_PROVIDER_MODEL": providerModel,
	}
	if role != "" {
		metadata["TIL_CONSENSUS_ROLE"] = role
	}
	for key, value := range metadata {
		set(key, value, "metadata")
	}

	ordered := make([]environmentValue, 0, len(values))
	for _, value := range values {
		ordered = append(ordered, value)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left := strings.ToUpper(ordered[i].Key)
		right := strings.ToUpper(ordered[j].Key)
		if left == right {
			return ordered[i].Key < ordered[j].Key
		}
		return left < right
	})
	result := BuiltEnvironment{Audit: EnvironmentAudit{Endpoints: map[string]string{}}}
	for _, value := range ordered {
		result.Values = append(result.Values, value.Key+"="+value.Value)
		switch value.Source {
		case "baseline":
			result.Audit.BaselineKeys = append(result.Audit.BaselineKeys, value.Key)
		case "passthrough":
			result.Audit.PassthroughKeys = append(result.Audit.PassthroughKeys, value.Key)
		case "configured":
			result.Audit.ConfiguredKeys = append(result.Audit.ConfiguredKeys, value.Key)
		case "metadata":
			result.Audit.MetadataKeys = append(result.Audit.MetadataKeys, value.Key)
		}
		upperKey := strings.ToUpper(value.Key)
		if strings.HasSuffix(upperKey, "_BASE_URL") || strings.HasSuffix(upperKey, "_ENDPOINT") {
			if endpoint := sanitizeEnvironmentEndpoint(value.Value); endpoint != "" {
				result.Audit.Endpoints[value.Key] = endpoint
			}
		}
	}
	if len(result.Audit.Endpoints) == 0 {
		result.Audit.Endpoints = nil
	}
	for canonical, hostValue := range host.values {
		if _, included := values[canonical]; included {
			continue
		}
		if looksLikeCredentialKey(hostValue.Key) {
			result.Audit.DroppedCredentialKeys = append(result.Audit.DroppedCredentialKeys, hostValue.Key)
		}
	}
	sort.Strings(result.Audit.DroppedCredentialKeys)
	for _, key := range knownEnvironmentKeys(provider) {
		if _, existsOnHost := host.lookup(key); !existsOnHost {
			continue
		}
		if _, included := values[host.canonicalKey(key)]; !included {
			result.MissingKnownKeys = append(result.MissingKnownKeys, key)
		}
	}
	sort.Strings(result.MissingKnownKeys)
	return result
}

func knownEnvironmentKeys(provider config.ProviderConfig) []string {
	if provider.CLIType != config.CLITypePi {
		return knownCLIEnvironmentKeys[provider.CLIType]
	}
	for idx, arg := range provider.Args {
		pairForm := arg == "--provider" && idx+1 < len(provider.Args) && provider.Args[idx+1] == "google-vertex"
		if pairForm || arg == "--provider=google-vertex" {
			return []string{"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CLOUD_PROJECT", "GOOGLE_CLOUD_LOCATION"}
		}
	}
	return []string{"GEMINI_API_KEY"}
}

// credentialKeyFragments are matched against the upper-cased variable name.
// The list is deliberately loose: a false positive only adds a name to the
// audit record, while a false negative hides a silently dropped credential.
var credentialKeyFragments = []string{
	"API_KEY", "_KEY", "_TOKEN", "_SECRET", "SECRET_", "_CREDENTIAL", "_PASSWORD",
	"_BASE_URL", "_ENDPOINT",
}

func looksLikeCredentialKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, fragment := range credentialKeyFragments {
		if strings.Contains(upper, fragment) {
			return true
		}
	}
	return false
}

func EnvironmentMigrationError(providerID string, missing []string) error {
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("CLI provider %s does not pass through host environment key(s) %s; add them to env_passthrough", providerID, strings.Join(missing, ", "))
}

func sanitizeEnvironmentEndpoint(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}
