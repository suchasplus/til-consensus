package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (m *ProviderModelConfig) UnmarshalYAML(value *yaml.Node) error {
	if err := validateProviderModelFields(value); err != nil {
		return err
	}
	type plain ProviderModelConfig
	var decoded plain
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*m = ProviderModelConfig(decoded)
	m.MaxOutputTokensSet = yamlMappingHasKey(value, "max_output_tokens")
	return nil
}

func validateProviderModelFields(value *yaml.Node) error {
	if value == nil || value.Kind != yaml.MappingNode {
		return nil
	}
	allowed := map[string]struct{}{
		"enabled":           {},
		"provider_model":    {},
		"context_window":    {},
		"max_output_tokens": {},
		"temperature":       {},
		"reasoning":         {},
	}
	for i := 0; i+1 < len(value.Content); i += 2 {
		key := value.Content[i].Value
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown provider model field %q", key)
		}
	}
	return nil
}

func yamlMappingHasKey(value *yaml.Node, key string) bool {
	if value == nil || value.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(value.Content); i += 2 {
		if value.Content[i].Value == key {
			return true
		}
	}
	return false
}
