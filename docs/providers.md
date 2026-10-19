# Provider 配置与预检

这篇只讲 provider、model、agent 以及如何确认它们真的可用。workflow roles 和 policy 见 [配置](config.md)，日常命令见 [操作手册](operations.md)。

桌面等嵌入宿主的模型目录、原生推理参数及请求/实际报告边界见 [CLI 模型与推理参数契约](cli-model-settings.md)。

## Provider 类型

当前支持：

- `mock`：本地测试用，不访问外部模型。
- `cli`：调用本机 CLI，例如 Codex、Claude Code、Grok Build、Gemini CLI、Antigravity CLI、Pi。
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
  - 通过 `--json-schema` 请求结构化输出，读取最终 `result` 事件中的 `structured_output`，缺省时才读取非空 `result` 文本。
  - JSON 与 JSONL 都检查 `is_error`、`subtype` 和 `terminal_reason`；进程退出码为 0 不能单独证明请求成功。
  - `models.<id>.reasoning` 会映射为 `--effort <value>`。
- `grok`
  - 通过 Grok Build 的非交互模式调用，`cli_type: grok`，默认命令为 `grok`。
  - 原生 JSON 使用 `structuredOutput`（驼峰）和 `text`；通过 `stopReason` 判断终态。原生流中的文本为 `type: text` 的 `data`，最终事件为 `type: end`。
  - `output_format: stream-json` 映射为 Grok 原生的 `--output-format streaming-json`；不混用独立的 `streaming-messages-json` 协议。
  - 内置 profile 使用本机 `grok models` 列出的默认模型 selector `grok-4.7`，并显式配置 `permission_mode: read-only`。此前真实推理成功的是未传 `--model` 的默认请求，不能据此认定某个显式 selector 已验证。
  - `models.*.provider_model` 是传给 CLI `--model` 的 requested selector；`modelUsage` 的 reported canonical 是返回统计名称，两者不能混用。本机默认请求返回 canonical `grok-4.7-build`，但显式 `--model grok-4.7-build` 在真实完整运行中报 `unknown model id`；正确 selector 为 `grok-4.7`，可选值以本机 `grok models` 为准。
- `gemini`
  - 通过 Gemini CLI 非交互调用。
  - 本机 CLI 的 thinking level 通常由 CLI 自身配置管理，不在 `til-consensus` 中声明已生效。
- `antigravity`
  - 通过 `agy --input-format stream-json --output-format stream-json` 非交互调用，prompt 以原生 `event: user` JSONL 从 stdin 传入，并用 `--model` 指定模型。
  - 使用原生 JSONL 事件流；不能将 `-p` 当成通用的 print 参数，也不能直接把完整 stdout 当成业务 JSON。
  - `read-only` 使用计划模式和终端 sandbox；这不等于操作系统级只读。桌面应用仍应为每次讨论提供私有 cwd，并按需要增加文件系统或容器隔离，防止计划文件、会话记录或 CLI 自身行为写入项目目录。
  - thinking level 通常编码在 `agy models` 返回的完整 ID 中；`models.<id>.reasoning` 也可映射到原生 `--effort`，但不能与 ID 后缀冲突。宿主按目录选择具体变体时应留空独立 reasoning，避免重复配置。
- `pi`
  - 通过 `pi --mode json --print --no-session` 做一次性调用，prompt 从 stdin 传入。
  - JSONL 只提取 assistant `text_delta`；assistant `errorMessage`、空文本或损坏事件流都会失败。
  - 内置模板使用 `--provider google-vertex` 和 `gemini-3.6-flash`，通过 ADC/服务账号 JSON 认证，location 固定为 `global`。

CLI provider 可显式设置以下执行字段；它们不适用于 `api`、`sdk` 或 `mock` provider：

```yaml
providers:
  grok-cli:
    type: cli
    cli_type: grok
    command: grok
    workdir: ./project
    permission_mode: read-only
    output_format: json
    env_passthrough: [XAI_API_KEY]
    models:
      default:
        provider_model: grok-4.7
```

- `workdir`：子进程工作目录。省略时继承调用进程目录；相对路径按调用进程当前目录解析。目录存在性在实际运行时检查。
- `permission_mode`：仅 `codex`、`claude`、`grok`、`antigravity` 支持，允许省略，或填写 `read-only` / `workspace-write`。讨论和评审建议使用 `read-only`；需要修改工作区时再明确使用 `workspace-write`。模式会映射为各 CLI 的权限或工具限制，其隔离能力取决于该 CLI，不能等同于统一的操作系统沙箱。
- `output_format`：仅上述四种 CLI 支持，允许省略，或填写统一名称 `json` / `stream-json`；不要把 Grok 原生的 `streaming-json` 写入该字段。默认行为由适配器决定，实际输出格式会转换成对应 CLI 的参数。其他 CLI 显式设置权限或输出模式会在配置校验时失败，避免配置被静默忽略；`workdir` 可用于所有 CLI。

省略 `permission_mode` 保留已有适配行为，例如 Codex 原有的 `workspace-write`。这三个字段会随 include/profile 合并保留，并显示在 `config explain` 中。既有配置中的模型选择不会因此自动更新。

四种原生适配器管理输入、输出和单次任务会话参数，`args` 不能再次覆盖 `--output-format`、schema、prompt-file、resume 等受控参数；旧配置应改用上述执行字段。显式 `read-only` 也不允许在 `args` 中追加跳过权限或覆盖 sandbox 的参数。AGY 每个任务启动全新进程，只发送一个原生 user 消息后关闭 stdin，不能通过 `--continue` / `--conversation` 复用 schema 会话；CLI 内部修复可能增加 `num_turns`，因此该计数不是外部任务数。

Grok 的原生 `--json-schema` 会强制 JSON 输出。选择 `stream-json` 时，schema 放入任务上下文，Runner 在成功终态后使用 JSON Schema 验证最终业务 JSON；不是仅靠提示词声明满足约束。

嵌入应用可使用 `runtime/cli.NewRunnerWithOptions` 的 `OnEvent` 接收公开正文与进度；回调错误会取消该调用。完整 JSON 模式在文档接收完后发出事件，JSONL 模式按完整行解析并保留 UTF-8 分片。`CLIEvent.Usage` 是服务方快照，不能直接累加；`RunTaskResult.RawOutput` 属于诊断数据，可能包含推理内容，不应直接渲染到聊天窗口。

Claude 的 `--safe-mode` 会禁用自定义指令、技能、hook 和 MCP 等定制，同时保留当前登录和模型选择；它与 `--bare` 不同，后者不会读取订阅 OAuth 或系统 keychain，不能直接拿来替换本机登录。详见 [Claude 非交互文档](https://code.claude.com/docs/en/headless)。Grok 的原生格式见 [Grok 非交互文档](https://docs.x.ai/build/cli/headless-scripting)。

读取流式结果时，以最终终态确认完成。Claude 实测在首个文本片段后收到 SIGINT，仍以退出码 0 结束，但终态为 `is_error: true`、`subtype: error_during_execution`、`terminal_reason: aborted_streaming`；这种情况必须归为取消。已经收到的文本不能当成完成的方案。各事件中的 token 使用量也不一定是增量：Claude 的多个 assistant 内容块会重复携带 usage，应使用最终汇总；取消后的汇总为 0 时，实际消耗仍可能不完整。

CLI 环境默认采用 allowlist，而不是完整继承 `os.Environ()`：

- `PATH`、`HOME`、临时目录、locale、证书路径和代理等运行基础变量默认保留。
- 凭据和自定义 endpoint 必须通过 `env_passthrough` 精确列出，不支持 `*`。
- `provider.env` 可显式设置固定值或 `{requestId}` 等模板值，并覆盖 passthrough。
- `TIL_CONSENSUS_*` 任务 metadata 最后写入，不能被 provider 配置覆盖。
- `CLAUDECODE` 永久拒绝；`CLAUDE_CODE_*` 只有显式声明才会透传。
- input artifact 只记录实际使用的环境变量名和脱敏 endpoint，不保存 key、token 或 secret 值。

例如：

```yaml
providers:
  pi-cli:
    type: cli
    cli_type: pi
    command: pi
    args: [--provider, google-vertex]
    env_passthrough: [GOOGLE_APPLICATION_CREDENTIALS, GOOGLE_CLOUD_PROJECT]
    env:
      GOOGLE_CLOUD_LOCATION: global
```

CLI 非零退出会记录为结构化的 `command_exit`，failure artifact 同时保留实际 `exitCode`。仅对已知 CLI 的 stderr 做保守分类：明确的认证、限流或服务不可用错误会分别归类为 `auth`、`rate_limited` 或 `unavailable`；Grok 的真实 `Not signed in` 错误归为 `auth`。stdout 不参与这种文本分类；有原生协议的适配器另行检查其终态，不能将协议错误当成答案。

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
- [grok.config.yaml](examples/grok.config.yaml)
- [gemini.config.yaml](examples/gemini.config.yaml)
- [antigravity.config.yaml](examples/antigravity.config.yaml)
- [pi.config.yaml](examples/pi.config.yaml)
- [deepseek.config.yaml](examples/deepseek.config.yaml)
- [qwen-max.config.yaml](examples/qwen-max.config.yaml)
- [gemini-api.config.yaml](examples/gemini-api.config.yaml)
- [openrouter.config.yaml](examples/openrouter.config.yaml)
- [kimi.config.yaml](examples/kimi.config.yaml)
