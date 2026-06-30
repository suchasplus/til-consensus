# Provider 配置与预检

这篇只讲 provider、model、agent 以及如何确认它们真的可用。workflow roles 和 policy 见 [配置](config.md)，日常命令见 [操作手册](operations.md)。

## Provider 类型

当前支持：

- `mock`：本地测试用，不访问外部模型。
- `cli`：调用本机 CLI，例如 Codex、Claude Code、Gemini CLI、Antigravity CLI。
- `api`：调用 HTTP/API provider，例如 OpenAI-compatible、OpenAI Responses、Anthropic-compatible、Gemini API。
- `sdk`：给 SDK 型运行时预留。

Provider 只描述“怎么调用模型”。Agent 描述“用哪个 provider/model，以什么角色参与 workflow”。

## CLI provider

常见 CLI profile：

- `codex`
  - 通过 `codex exec` 非交互调用。
  - 结构化输出优先使用 `--output-schema`，最终答案从 `--output-last-message` 文件读取。
  - `models.<id>.reasoning` 会映射为 `-c model_reasoning_effort=<value>`。
- `claude`
  - 通过 `claude --print` 非交互调用。
  - 可使用 Claude CLI 的结构化输出能力时优先走 schema 约束。
  - `models.<id>.reasoning` 会映射为 `--effort <value>`。
- `gemini`
  - 通过 Gemini CLI 非交互调用。
  - 本机 CLI 的 thinking level 通常由 CLI 自身配置管理，不在 `til-consensus` 中声明已生效。
- `antigravity`
  - 通过 `agy -p` 非交互调用，并用 `--model` 指定模型。
  - thinking level 通常体现在 Antigravity 的模型名或 CLI 自身配置中。

CLI provider 当前没有稳定的一等 output-token 参数。不要在 CLI provider 中配置：

- `models.*.max_output_tokens`
- `options.max_output_tokens_field`
- `args: --max-output-tokens`

这些配置会在 `config validate` 或 `profile preflight` 中报错，避免误以为 token 限制已经传给 CLI。

## API provider

支持的 API 协议：

- `openai-compatible`
  - Chat Completions 风格网关。
  - 适合 DeepSeek、Kimi、OpenRouter 或公司内 OpenAI-compatible 代理。
- `openai-responses`
  - OpenAI Responses API 风格。
  - 使用 `github.com/openai/openai-go/v3` 的 `Responses.New`。
  - `max_output_tokens` 固定映射为 Responses API 的 `max_output_tokens`。
- `anthropic-compatible`
  - Anthropic Messages API 风格网关。
- `gemini-api`
  - Gemini `generateContent`。
  - 使用官方 `google.golang.org/genai` 的 `Models.GenerateContent`。
  - SDK 负责发送 camelCase payload，例如 `maxOutputTokens`、`responseMimeType`、`responseJsonSchema`、`thinkingConfig`。

API provider 常用字段：

```yaml
providers:
  gemini-api:
    enabled: true
    type: api
    protocol: gemini-api
    base_url: https://generativelanguage.googleapis.com/v1beta
    api_key_env: GEMINI_API_KEY
    models:
      default:
        enabled: true
        provider_model: gemini-3.5-flash
        max_output_tokens: 2048
        temperature: 0.2
        reasoning: high
```

通用能力：

- `base_url`
- `api_key_env`
- `headers`
- `models.<id>.provider_model`
- `models.<id>.context_window`
- `models.<id>.max_output_tokens`
- `models.<id>.temperature`
- `models.<id>.reasoning`
- `options`

`models.<id>.reasoning` 是 provider-specific 映射。Gemini API 中会映射到 `generationConfig.thinkingConfig.thinkingLevel`，支持 `minimal / low / medium / high`，最终发送为 Gemini SDK 的枚举值。

## `enabled`

provider 和 model 都支持 `enabled`：

```yaml
providers:
  deepseek-api:
    enabled: true
    models:
      default:
        enabled: true
        provider_model: deepseek-v4-pro
```

规则：

- 未写 `enabled` 时默认等价于 `true`。
- `enabled: false` 的 provider/model 不允许被 agent 引用。
- `profile preflight --all` 会跳过 disabled provider/model。
- 显式 `--provider disabled-id` 或 agent 引用 disabled model 会报错。

## Agent 引用

Agent 负责把 workflow 角色映射到 provider/model：

```yaml
agents:
  - id: arbiter-gemini
    provider: gemini-api
    model: default
    role: arbiter
    system_prompt: 你是严格的 claim-level arbiter。
```

如果 `model` 省略，使用 provider 的默认模型。

## Profile preflight

`profile preflight` 的目标是验证 providers 配置正确性和真实可连通性，而不是验证完整 workflow roles。

检查全部 provider：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
```

不传 `--provider/--agent` 时默认也会检查全部 provider：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --verbose
```

只检查某个 provider：

```bash
til-consensus profile preflight \
  --config ./til-consensus.yaml \
  --provider deepseek-api \
  --output ./out/{requestId} \
  --verbose
```

只检查某个 agent：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --agent arbiter-gemini --verbose
```

行为：

- API provider 会检查 `api_key_env` 对应环境变量是否存在。
- CLI provider 会检查二进制是否存在，并执行最小非交互调用。
- 每个 provider 会被要求返回 `{"ok": true}`。
- 多 provider 会逐个分块输出，不会等全部结束才打印。
- `--output` 只覆盖本次 preflight 输出目录，不写回配置。
- 相对 `output.directory` 按当前执行目录解析，不按配置文件所在目录解析。
- API preflight 默认使用 `max_output_tokens=2048`；如果 model 配置了更小值，则尊重配置。
- `--verbose` 会显示 provider 类型、协议、base url、api key env、CLI 命令参数和 stdout。

典型输出：

```text
- gemini-api/gemini-3.5-flash ready=true strict=true recoverable=true duration=2680ms
  provider: type=api protocol=gemini-api
  base_url: https://generativelanguage.googleapis.com/v1beta
  api_key_env: GEMINI_API_KEY
  command: gemini-api gemini-3.5-flash "https://generativelanguage.googleapis.com/v1beta"
  stdout: {"ok":true}
```

结果会写入：

- `result.json`
- `summary.md`
- `artifacts/provider-readiness.json`

## 常见失败

- `env XXX is not set`
  - `api_key_env` 指向的环境变量没设置。
- `binary <name> not found`
  - CLI 不在 `PATH`。
- `status=401/403`
  - API key、base url、账号权限或模型权限错误。
- `status=429`
  - 被限流。
- `did not return a recoverable JSON object`
  - provider 可调用，但当前输出不满足最小 JSON 契约。
- `gemini response contains no text parts ... finishReason=MAX_TOKENS`
  - Gemini thinking 模型可能把预算消耗在思考阶段；提高 `max_output_tokens` 或降低 thinking。

## 样例

完整样例索引见 [配置与输入样例](examples.md)。常用文件：

- [all-providers.fill-in.config.yaml](examples/all-providers.fill-in.config.yaml)
- [codex.config.yaml](examples/codex.config.yaml)
- [claude.config.yaml](examples/claude.config.yaml)
- [gemini.config.yaml](examples/gemini.config.yaml)
- [antigravity.config.yaml](examples/antigravity.config.yaml)
- [deepseek.config.yaml](examples/deepseek.config.yaml)
- [qwen-max.config.yaml](examples/qwen-max.config.yaml)
- [gemini-api.config.yaml](examples/gemini-api.config.yaml)
- [openrouter.config.yaml](examples/openrouter.config.yaml)
- [kimi.config.yaml](examples/kimi.config.yaml)
