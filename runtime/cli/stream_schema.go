package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/suchasplus/til-consensus/config"
)

func streamingSchemaPrompt(provider config.ProviderConfig, prompt string, schema map[string]any) (string, error) {
	if provider.CLIType != config.CLITypeGrok || provider.OutputFormat != "stream-json" || len(schema) == 0 {
		return prompt, nil
	}
	body, err := json.Marshal(schema)
	if err != nil {
		return "", err
	}
	return prompt + "\nJSON Schema for the final answer:\n" + string(body), nil
}

func validateStreamingSchema(schema map[string]any, output string) error {
	// 调用方可使用 []string、[]map[string]any 等 Go 容器构建 schema；
	// 编译器要求解码后的 JSON 类型。经过序列化边界也避免修改调用方的对象。
	body, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("CLI output schema: encode JSON: %w", err)
	}
	normalized, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("CLI output schema: decode JSON: %w", err)
	}
	if _, ok := normalized.(map[string]any); !ok {
		return fmt.Errorf("CLI output schema: expected a JSON object")
	}
	compiler := jsonschema.NewCompiler()
	const location = "urn:til-consensus:cli-output"
	if err := compiler.AddResource(location, normalized); err != nil {
		return fmt.Errorf("CLI output schema: %w", err)
	}
	compiled, err := compiler.Compile(location)
	if err != nil {
		return fmt.Errorf("CLI output schema: %w", err)
	}
	value, err := jsonschema.UnmarshalJSON(strings.NewReader(output))
	if err != nil {
		return fmt.Errorf("CLI structured stream must return JSON: %w", err)
	}
	if err := compiled.Validate(value); err != nil {
		return fmt.Errorf("CLI structured stream violates output schema: %w", err)
	}
	return nil
}

func hasOutputFormat(args []string, format string) bool {
	for i, arg := range args {
		if arg == "--output-format" && i+1 < len(args) && args[i+1] == format {
			return true
		}
		if arg == "--output-format="+format {
			return true
		}
	}
	return false
}

func validateOneShotArgs(provider config.ProviderConfig) error {
	switch provider.CLIType {
	case config.CLITypeCodex, config.CLITypeClaude, config.CLITypeGrok, config.CLITypeAntigravity:
	default:
		return nil
	}
	for index, arg := range provider.Args {
		name, _, _ := strings.Cut(arg, "=")
		for _, blocked := range []string{"--continue", "--resume", "--conversation", "--input-format", "--output-format", "--json-schema", "--output-schema", "--output-last-message", "-o", "--prompt-file"} {
			if name == blocked {
				return fmt.Errorf("%s one-shot transport manages %s; use the provider execution fields and a fresh task process", provider.CLIType, blocked)
			}
		}
		if (provider.CLIType == config.CLITypeAntigravity || provider.CLIType == config.CLITypeGrok) && (name == "-c" || name == "-p" || name == "--prompt" || name == "--print") {
			return fmt.Errorf("%s transport does not allow overriding %s", provider.CLIType, name)
		}
		if provider.Workdir != "" && (name == "--cwd" || name == "--cd" || name == "-C" || name == "--project") {
			return fmt.Errorf("provider workdir cannot be overridden by %s", name)
		}
		if provider.PermissionMode == "read-only" {
			if provider.CLIType == config.CLITypeCodex && strings.HasPrefix(arg, "-c") && !strings.HasPrefix(arg, "--") && len(arg) > 2 {
				key, _, _ := strings.Cut(arg[2:], "=")
				if strings.HasPrefix(key, "sandbox") || key == "approval_policy" {
					return fmt.Errorf("read-only profile cannot override config %s", key)
				}
			}
			if strings.HasPrefix(name, "--dangerously-") {
				return fmt.Errorf("read-only profile cannot use %s", name)
			}
			for _, blocked := range []string{"--sandbox", "--permission-mode", "--mode", "--tools", "--allowedTools", "--allowed-tools", "--allow", "--always-approve", "--no-plan", "--yolo", "--full-auto", "--disable-slash-commands"} {
				if name == blocked {
					return fmt.Errorf("read-only profile cannot be overridden by %s", name)
				}
			}
			if provider.CLIType == config.CLITypeCodex && (name == "-c" || name == "--config") {
				value := ""
				if _, after, ok := strings.Cut(arg, "="); ok {
					value = after
				} else if index+1 < len(provider.Args) {
					value = provider.Args[index+1]
				}
				key, _, _ := strings.Cut(value, "=")
				if strings.HasPrefix(key, "sandbox") || key == "approval_policy" {
					return fmt.Errorf("read-only profile cannot override config %s", key)
				}
			}
		}
	}
	return nil
}
