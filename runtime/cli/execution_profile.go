package cli

import "github.com/suchasplus/til-consensus/config"

func applyExecutionProfile(provider config.ProviderConfig, input []string) []string {
	args := append([]string(nil), input...)
	replace := func(flag, value string) {
		for i := 0; i < len(args)-1; i++ {
			if args[i] == flag {
				args[i+1] = value
				return
			}
		}
		args = append(args, flag, value)
	}
	if provider.OutputFormat == "stream-json" {
		switch provider.CLIType {
		case config.CLITypeClaude:
			replace("--output-format", "stream-json")
			args = append(args, "--verbose", "--include-partial-messages")
		case config.CLITypeGrok:
			replace("--output-format", "streaming-json")
		}
	}
	switch provider.PermissionMode {
	case "read-only":
		switch provider.CLIType {
		case config.CLITypeCodex:
			replace("--sandbox", "read-only")
		case config.CLITypeClaude:
			args = append(args, "--safe-mode", "--tools", "", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--permission-mode", "dontAsk", "--permission-prompts", "none")
		case config.CLITypeGrok:
			args = append(args, "--tools", "", "--deny", "MCPTool(*)", "--disable-web-search")
		}
	case "workspace-write":
		switch provider.CLIType {
		case config.CLITypeClaude:
			args = append(args, "--permission-mode", "acceptEdits")
		case config.CLITypeAntigravity:
			replace("--mode", "accept-edits")
		case config.CLITypeGrok:
			replace("--permission-mode", "default")
		}
	}
	return args
}
