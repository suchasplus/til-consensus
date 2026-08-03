package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMergeProviderEnvPassthroughPresence(t *testing.T) {
	baseValues := []string{"BASE_TOKEN"}
	emptyValues := []string{}
	overrideValues := []string{"CHILD_TOKEN"}
	tests := []struct {
		name    string
		overlay *ProviderConfig
		want    []string
	}{
		{name: "unset inherits", overlay: &ProviderConfig{}, want: []string{"BASE_TOKEN"}},
		{name: "explicit empty clears", overlay: &ProviderConfig{EnvPassthrough: &emptyValues}, want: []string{}},
		{name: "non-empty replaces", overlay: &ProviderConfig{EnvPassthrough: &overrideValues}, want: []string{"CHILD_TOKEN"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeProvider(ProviderConfig{EnvPassthrough: &baseValues}, *tt.overlay)
			if got.EnvPassthrough == nil {
				t.Fatal("expected env_passthrough presence")
			}
			if len(*got.EnvPassthrough) != len(tt.want) {
				t.Fatalf("env_passthrough: got=%v want=%v", *got.EnvPassthrough, tt.want)
			}
			for idx := range tt.want {
				if (*got.EnvPassthrough)[idx] != tt.want[idx] {
					t.Fatalf("env_passthrough: got=%v want=%v", *got.EnvPassthrough, tt.want)
				}
			}
		})
	}
}

func TestProviderEnvPassthroughYAMLRoundTripPreservesExplicitEmpty(t *testing.T) {
	empty := []string{}
	original := ProviderConfig{Type: ProviderTypeCLI, CLIType: CLITypeGeneric, Command: "adapter", EnvPassthrough: &empty}
	body, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var decoded ProviderConfig
	if err := yaml.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.EnvPassthrough == nil || len(*decoded.EnvPassthrough) != 0 {
		t.Fatalf("expected explicit empty env_passthrough, got %#v\nyaml:\n%s", decoded.EnvPassthrough, body)
	}
}
