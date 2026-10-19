package config

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValidateCLIExecutionSettings(t *testing.T) {
	base := ProviderConfig{Type: ProviderTypeCLI, CLIType: CLITypeGrok, Models: map[string]ProviderModelConfig{"default": {ProviderModel: "grok-4.7"}}}
	for _, permission := range []string{"", "read-only", "workspace-write"} {
		for _, output := range []string{"", "json", "stream-json"} {
			provider := base
			provider.PermissionMode, provider.OutputFormat = permission, output
			// Config validation deliberately does not require a local directory.
			provider.Workdir = "/not-present-on-this-machine"
			if err := ValidateProfiles(Config{SchemaVersion: 1, Providers: map[string]ProviderConfig{"grok": provider}}); err != nil {
				t.Fatalf("permission=%q output=%q: %v", permission, output, err)
			}
		}
	}
	for _, test := range []struct {
		name string
		edit func(*ProviderConfig)
	}{
		{"invalid permission", func(p *ProviderConfig) { p.PermissionMode = "bypassPermissions" }},
		{"invalid output", func(p *ProviderConfig) { p.OutputFormat = "text" }},
		{"native spelling is not config spelling", func(p *ProviderConfig) { p.OutputFormat = "streaming-json" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := base
			test.edit(&provider)
			if err := ValidateProfiles(Config{SchemaVersion: 1, Providers: map[string]ProviderConfig{"grok": provider}}); err == nil {
				t.Fatal("invalid CLI execution setting was accepted")
			}
		})
	}
}

func TestCLIExecutionSettingsAreCLIOnly(t *testing.T) {
	for _, providerType := range []string{ProviderTypeAPI, ProviderTypeSDK, ProviderTypeMock} {
		for _, field := range []string{"workdir", "permission_mode", "output_format"} {
			t.Run(providerType+"/"+field, func(t *testing.T) {
				provider := ProviderConfig{Type: providerType, Protocol: APIProtocolOpenAICompatible, Adapter: "test", Models: map[string]ProviderModelConfig{"default": {ProviderModel: "model"}}}
				switch field {
				case "workdir":
					provider.Workdir = "."
				case "permission_mode":
					provider.PermissionMode = "read-only"
				case "output_format":
					provider.OutputFormat = "json"
				}
				err := ValidateProfiles(Config{SchemaVersion: 1, Providers: map[string]ProviderConfig{"test": provider}})
				if err == nil || !strings.Contains(err.Error(), field+" is only supported by cli") {
					t.Fatalf("expected CLI-only validation, got %v", err)
				}
			})
		}
	}
}

func TestCLIExecutionSettingsRejectUnsupportedCLIMappings(t *testing.T) {
	for _, cliType := range []string{CLITypeGemini, CLITypeOpenCode, CLITypePi, CLITypeGeneric, "custom"} {
		for _, field := range []string{"permission_mode", "output_format"} {
			t.Run(cliType+"/"+field, func(t *testing.T) {
				provider := ProviderConfig{Type: ProviderTypeCLI, CLIType: cliType, Command: "adapter", Models: map[string]ProviderModelConfig{"default": {ProviderModel: "model"}}}
				if field == "permission_mode" {
					provider.PermissionMode = "read-only"
				} else {
					provider.OutputFormat = "stream-json"
				}
				err := ValidateProfiles(Config{SchemaVersion: 1, Providers: map[string]ProviderConfig{"test": provider}})
				if err == nil || !strings.Contains(err.Error(), field+" is not supported for cli_type") {
					t.Fatalf("unsupported mapping was not rejected: %v", err)
				}
			})
		}
	}
	for _, cliType := range []string{CLITypeCodex, CLITypeClaude, CLITypeGrok, CLITypeAntigravity, CLITypeGemini, CLITypeOpenCode, CLITypePi, CLITypeGeneric, "custom"} {
		provider := ProviderConfig{Type: ProviderTypeCLI, CLIType: cliType, Command: "adapter", Workdir: "./project", Models: map[string]ProviderModelConfig{"default": {ProviderModel: "model"}}}
		if err := ValidateProfiles(Config{SchemaVersion: 1, Providers: map[string]ProviderConfig{"test": provider}}); err != nil {
			t.Fatalf("workdir should remain available for %s: %v", cliType, err)
		}
	}
	for _, cliType := range []string{CLITypeCodex, CLITypeClaude, CLITypeGrok, CLITypeAntigravity} {
		provider := ProviderConfig{Type: ProviderTypeCLI, CLIType: cliType, PermissionMode: "read-only", OutputFormat: "stream-json", Models: map[string]ProviderModelConfig{"default": {ProviderModel: "model"}}}
		if err := ValidateProfiles(Config{SchemaVersion: 1, Providers: map[string]ProviderConfig{"test": provider}}); err != nil {
			t.Fatalf("implemented mapping should be accepted for %s: %v", cliType, err)
		}
	}
}

func TestCLIExecutionSettingsSurviveMergeRoundTripAndExplain(t *testing.T) {
	base := ProviderConfig{Type: ProviderTypeCLI, CLIType: CLITypeGrok, Workdir: "./project", PermissionMode: "read-only", OutputFormat: "json"}
	inherited := mergeProvider(base, ProviderConfig{})
	if inherited.Workdir != base.Workdir || inherited.PermissionMode != base.PermissionMode || inherited.OutputFormat != base.OutputFormat {
		t.Fatalf("unset overlay lost execution settings: %#v", inherited)
	}
	provider := mergeProvider(base, ProviderConfig{Workdir: "./other", PermissionMode: "workspace-write", OutputFormat: "stream-json"})
	body, err := yaml.Marshal(provider)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ProviderConfig
	if err := yaml.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Workdir != "./other" || decoded.PermissionMode != "workspace-write" || decoded.OutputFormat != "stream-json" {
		t.Fatalf("roundtrip lost overlay: %#v", decoded)
	}
	report := BuildExplainReport(LoadedConfig{Config: Config{Providers: map[string]ProviderConfig{"grok": decoded}}}, ExplainOptions{})
	if len(report.Providers) != 1 || report.Providers[0].Workdir != decoded.Workdir || report.Providers[0].PermissionMode != decoded.PermissionMode || report.Providers[0].OutputFormat != decoded.OutputFormat {
		t.Fatalf("explain omitted execution settings: %#v", report.Providers)
	}
	text := RenderExplainText(report)
	for _, value := range []string{"workdir=./other", "permissionMode=workspace-write", "outputFormat=stream-json"} {
		if !strings.Contains(text, value) {
			t.Errorf("explain text missing %q", value)
		}
	}
	body, err = json.Marshal(report)
	if err != nil || !strings.Contains(string(body), `"permissionMode":"workspace-write"`) {
		t.Fatalf("explain JSON omitted settings: body=%s err=%v", body, err)
	}
}

func TestGrokTemplateSelectionAndPreset(t *testing.T) {
	for _, mode := range []string{"adjudication", "free_debate", "delphi"} {
		_, selection, err := RenderTemplateRequest("", mode, "grok", "general")
		if err != nil {
			t.Fatalf("grok %s template: %v", mode, err)
		}
		cfg, err := BuildTemplateConfig(selection)
		if err != nil {
			t.Fatal(err)
		}
		provider := cfg.Providers["grok-cli"]
		if provider.CLIType != CLITypeGrok || provider.Command != "grok" || provider.Models["default"].ProviderModel != "grok-4.7" || provider.PermissionMode != "read-only" || provider.OutputFormat != "json" {
			t.Fatalf("unexpected Grok profile: %#v", provider)
		}
		if provider.EnvPassthrough == nil || strings.Join(*provider.EnvPassthrough, ",") != "XAI_API_KEY" {
			t.Fatalf("unexpected Grok credential allowlist: %#v", provider.EnvPassthrough)
		}
	}
	body, selection, err := RenderTemplateRequest(TemplatePresetGrok, "", "", "")
	if err != nil || selection.Alias != TemplatePresetGrok || !strings.Contains(body, "provider_profile=grok") {
		t.Fatalf("grok preset not resolved: %#v err=%v", selection, err)
	}
}

func TestBuildProviderKeepsCLIExecutionSettings(t *testing.T) {
	provider, err := BuildProvider(AddProviderInput{ID: "grok", Type: ProviderTypeCLI, CLIType: CLITypeGrok, ModelID: "default", ProviderModel: "grok-4.7", Workdir: "./project", PermissionMode: "read-only", OutputFormat: "stream-json"})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Workdir != "./project" || provider.PermissionMode != "read-only" || provider.OutputFormat != "stream-json" {
		t.Fatalf("BuildProvider dropped settings: %#v", provider)
	}
}

func TestGrokExampleLoads(t *testing.T) {
	loaded, err := Load("../docs/examples/grok.config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	provider := loaded.Config.Providers["grok-cli"]
	if provider.PermissionMode != "read-only" || provider.OutputFormat != "json" || provider.Models["default"].ProviderModel != "grok-4.7" {
		t.Fatalf("example does not use the Grok discussion profile: %#v", provider)
	}
}
