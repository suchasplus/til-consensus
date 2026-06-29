# 配置说明

`til-consensus` 的配置文件默认叫 `til-consensus.yaml`。

查找顺序：

1. `--config`
2. 当前目录下的 `./til-consensus.yaml`
3. `~/.config/til-consensus/default.yaml`
4. `~/.config/til-consensus/config.yaml`

如果你显式设置了 `XDG_CONFIG_HOME`，对应路径会落到 `$XDG_CONFIG_HOME/til-consensus/...`。

## include / overlay

配置文件支持用 `include` 拆分片段，避免维护一份很大的总配置。

```yaml
schema_version: 1
include:
  - ./partials/providers.cli.yaml
  - ./partials/providers.gemini-api.yaml
  - ./partials/agents.tri-model.yaml
  - ./partials/roles.adjudication.yaml

defaults:
  mode: adjudication
output:
  directory: ./out/{requestId}
```

规则：

- `include` 路径相对当前配置文件所在目录解析。
- include 可以继续 include 其他文件。
- 如果出现循环 include，加载会直接失败。
- 多个 include 按顺序合并，后面的 include 覆盖前面的 include。
- 主配置文件最后覆盖所有 include。
- map 字段会深合并，例如 `providers`、`providers.<id>.models`、`headers`、`env`、`options`。
- `agents` 按 `id` 合并，同一个 agent 可以在主文件里只覆盖 `system_prompt`、`role` 或 `model`。
- slice 字段默认整体替换，例如 `roles.proposers`、`success_criteria`、`required_checks`。
- 标量字段只有非零值会覆盖；布尔字段目前只支持 `false -> true` 的覆盖，不适合用 include 把某个已启用的布尔值关掉。
- 最终合并后的配置仍然走同一套 `Normalize` 和 `Validate`。
- `config add-provider` / `config add-agent` 这类写回命令会写出合并后的完整配置，不会保留原始 include 结构；手工维护 include 配置时，建议直接编辑片段文件。

推荐目录：

```text
configs/
  adjudication.yaml
  free-debate.yaml
  delphi.yaml
  partials/
    providers.cli.yaml
    providers.gemini-api.yaml
    providers.openrouter.yaml
    agents.tri-model.yaml
    roles.adjudication.yaml
    roles.debate.yaml
    roles.delphi.yaml
```

建议把稳定的 provider、agent、roles 放在 include 片段里，把每次任务不同的内容放在 `run.yaml` 或 `--task-file` 里。

## profile / profiles overlay

如果一份配置里需要保留多套运行方式，可以用 `profile` 选择一个 overlay：

```yaml
schema_version: 1
include:
  - ./conf/providers.yaml

profile: fast

profiles:
  fast:
    defaults:
      mode: adjudication
      per_task_timeout: 5m
    roles:
      proposers: [proposer-fast]
      challengers: [challenger-fast]
      arbiter: arbiter-fast
  delphi-strong:
    defaults:
      mode: delphi
      per_task_timeout: 20m
    roles:
      participants: [participant-a, participant-b, participant-c]
      facilitator: facilitator-a
      reporter: reporter-a

output:
  directory: ./out/{requestId}
```

规则：

- `profile:` 是默认 active profile。
- 命令行可以用 `--profile <name>` 覆盖，例如 `til-consensus ask "..." --profile fast`。
- `profile preflight` 为避免命名混淆，使用 `--config-profile <name>`。
- profile overlay 支持覆盖 `defaults`、`output`、`providers`、`agents`、`roles`。
- overlay 会在最终 `Normalize` / `Validate` 前合并，所以 `config render --profile fast` 能看到最终生效配置。

## 推荐起步方式

第一次上手时，直接生成模板：

```bash
til-consensus config init --mode adjudication --provider-profile mock --config ./til-consensus.yaml
```

如果你希望默认就是 include + profile 的拆分结构，用：

```bash
til-consensus setup --mode adjudication --provider-profile mock --dir .
```

它会生成：

```text
til-consensus.yaml
conf/providers.yaml
conf/profiles.yaml
```

等价的子命令是：

```bash
til-consensus config wizard --mode delphi --provider-profile claude --dir .
```

`config init` 现在按 3 个维度生成模板：

- `--mode`
  - `adjudication`
  - `free-debate`
  - `delphi`
- `--provider-profile`
  - `mock`
  - `openai`
  - `generic`
  - `codex`
  - `claude`
  - `gemini`
  - `antigravity`
- `--task-profile`
  - `general`
  - `coding`

如果想先看看模板，不落盘：

```bash
til-consensus config init --mode free-debate --provider-profile mock --stdout
```

常见组合：

```bash
til-consensus config init --mode adjudication --provider-profile generic --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile claude --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile gemini --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile antigravity --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile mock --task-profile coding --config ./til-consensus.yaml
```

## `run` 的输入方式

`til-consensus run` 现在支持几种常用输入源：

- `--task`
  - 直接在命令行里传一段任务文本
- `--task-file`
  - 从文件中读取**全部文本内容**作为任务文本
- `--input`
  - 读取 `run.yaml` 或 `run.json`
- `--followup`
  - 直接执行 follow-up case artifact
- `--resume-session`
  - 从已持久化的 session 恢复
- `--replay-session`
  - 基于历史 session 生成 child run

最常见的两种方式：

```bash
til-consensus run --config ./til-consensus.yaml --task "判断这个 patch 是否真正修复了竞态问题"
til-consensus run --config ./til-consensus.yaml --task-file ./task.md
```

如果你的输入文件里已经有 `task_spec.goal`，也可以用 `--task-file` 覆盖它：

```bash
til-consensus run --config ./til-consensus.yaml --input ./run.yaml --task-file ./task.md
```

优先级是：

1. `--task` 或 `--task-file`
2. `--input` 里的 `task_spec.goal`

约束规则：

- `--task-file` 不能和 `--task` 同时使用
- `--task-file` 不能和：
  - `--followup`
  - `--resume-session`
  - `--replay-session`
  同时使用
- `--task-file` 可以和 `--input` 一起使用

`--task-file` 适合这些场景：

- 任务描述很长，不适合塞进 shell 一行
- 你想把整段需求、会议纪要、文档草稿直接交给 workflow
- 你希望任务文本可以放进版本控制或复用

如果已有配置文件，需要覆盖：

```bash
til-consensus config init --mode delphi --provider-profile mock --config ./til-consensus.yaml --force
```

旧的 `--preset` 仍然可用，但现在只是兼容别名。例如：

- `quickstart`
- `coding`
- `debate`
- `delphi`
- `generic`
- `codex`
- `claude`
- `gemini`
- `antigravity`
- `openai`

provider profile 的当前默认模型：

- `codex`
  - `gpt-5.4`
- `claude`
  - `claude-opus-4-6`
- `gemini`
  - `gemini-3.1-pro-preview`
- `antigravity`
  - `Gemini 3.5 Flash (High)`

## 配置结构

顶层固定包含：

- `schema_version`
- `defaults`
- `output`
- `providers`
- `agents`
- `roles`

### `defaults`

主要控制默认的 task 约束和 policy。

通用字段：

- `mode`
- `task_type`
- `success_criteria`
- `allowed_tools`
- `per_task_timeout`
- `task_retry_attempts`
- `global_deadline`
- `loop_policy`
- `ingest_policy`
- `fallback_policy`
- `observe_policy`
- `workspace_snapshot`
- `task_constraints`

`adjudication` 专用：

- `proposal_policy`
- `verification_policy`
- `arbiter_policy`

其中：

- `loop_policy.max_revision_rounds`
- `loop_policy.max_verification_rounds`
- `loop_policy.material_confidence_delta`
- `fallback_policy.max_fallback_rounds`
- `fallback_policy.on_insufficient_evidence`
- `fallback_policy.on_unresolved_conflict`
- `fallback_policy.on_unresolved_claims`
- `fallback_policy.on_keep_with_caveat`

用于限制 `verify -> revise -> challenge/verify` 以及 `adjudicate -> revise/ingest` 这样的受控闭环。

`ingest_policy.sources` 和 `observe_policy.sources` 都是命令源列表，单个 source 支持：

- `name`
- `command`
- `args`
- `workdir`
- `env`
- `source_type`
- `reference`
- `success_pattern`
- `failure_pattern`
- `parsing`

`parsing` 当前支持四种模式：

- `mode: text`
  - 默认模式
  - 继续使用 `success_pattern` / `failure_pattern` 做文本匹配
- `mode: json`
  - 把 stdout 解析成 JSON
  - 支持用字段路径提取结构化结果
  - 可用字段：
    - `success_path`
    - `failure_path`
    - `summary_path`
    - `excerpt_path`
    - `notes_path`
    - `metadata_paths`
- `mode: yaml`
  - 把 stdout 解析成 YAML
  - 然后复用和 JSON 相同的路径提取规则
- `mode: xml`
  - 把 stdout 解析成简单 XML 树
  - 然后用点路径提取字段

字段路径使用简单的点路径，支持数组下标，例如：

- `status.ok`
- `report.summary`
- `items[0].name`

如果要跨数组提取，也支持简单的 `[*]`：

- `items[*].name`

如果你希望先做最基础的 schema 校验，再继续处理外部源，可以加：

- `required_paths`

示例：

```yaml
parsing:
  mode: yaml
  required_paths:
    - report.summary
    - report.publishedAt
  summary_path: report.summary
  excerpt_path: report.excerpt
  metadata_paths:
    publishedAt: report.publishedAt
```

适用语义：

- `ingest_policy`
  - 在 `ingest` 阶段执行
  - 适合补抓证据、归一化外部材料、追加 tool 输出
- `observe_policy`
  - 在 `observe` 阶段执行
  - 适合健康检查、回归观察、外部状态确认

`free_debate` 专用：

- `debate_policy`
  - `min_rounds`
  - `max_rounds`
  - `vote_threshold`
  - `enable_early_stop`
  - `peer_context_mode`

`delphi` 专用：

- `delphi_policy`
  - `min_rounds`
  - `max_rounds`
  - `convergence_threshold`
  - `rating_scale_min`
  - `rating_scale_max`
  - `anonymous`
  - `facilitator_summary_style`

### `providers`

当前支持：

- `type: mock`
- `type: api`
- `type: cli`
- `type: sdk`

常见字段：

- `protocol`
- `base_url`
- `api_key_env`
- `models`
- `command`
- `args`
- `env`

### `agents`

每个 agent 至少要有：

- `id`
- `provider`
- 可选 `model`
- 可选 `role`
- 可选 `system_prompt`

### `roles`

角色分配取决于当前 workflow。

`adjudication`：

- `proposers`
- `challengers`
- `arbiter`
- `semantic_verifier`
- `reporter`
- `actor`

`free_debate`：

- `participants`
- 可选 `reporter`
- 可选 `actor`

`delphi`：

- `participants`
- 可选 `facilitator`
- 可选 `reporter`
- 可选 `actor`

## 命令行覆盖

`run` 支持通过 CLI flags 覆盖配置和输入文件。

当前最重要的 mode 相关 flags：

- `--mode adjudication|free-debate|delphi`
- `--participants`
- `--facilitator`
- `--min-rounds`
- `--max-rounds`
- `--vote-threshold`
- `--convergence-threshold`

`adjudication` 相关输入里还建议显式提供：

- `task_spec.task_type`
- `task_spec.out_of_scope`
- `action_policy.risk_gate`

优先级始终是：

`CLI flags > input file > config defaults > built-in defaults`

如果只想确认最终 plan，不调用 provider、不写运行产物，可以用：

```bash
til-consensus run --config ./til-consensus.yaml --input ./case.run.yaml --dry-run
til-consensus run --config ./til-consensus.yaml --task-file ./task.md --dry-run --format json
```

`--dry-run` 会展示：

- 最终 `mode`
- 角色映射
- agent -> provider/model 映射
- 输出路径
- workflow phase 顺序
- timeout / retry / verification / debate / delphi policy 摘要

## render / explain

`include` 和 overlay 多了之后，建议用 `config render` 看最终配置：

```bash
til-consensus config render --config ./til-consensus.yaml
til-consensus config render --config ./til-consensus.yaml --format json
til-consensus config render --config ./til-consensus.yaml --profile delphi-strong
```

如果配置还没填完整 workflow roles，只想渲染 provider/profile 层：

```bash
til-consensus config render --config ./conf/providers.yaml --profiles-only
```

`config explain` 输出更适合人读：

```bash
til-consensus config explain --config ./til-consensus.yaml
til-consensus config explain --config ./til-consensus.yaml --provider gemini-api
til-consensus config explain --config ./til-consensus.yaml --agent arbiter-a
til-consensus config explain --config ./til-consensus.yaml --profile fast
```

它会展示：

- include trace
- provider 列表
- agent -> provider/model 映射
- roles
- 按当前执行目录解析后的 output 路径

## 最小示例

### 1. `adjudication`

```yaml
defaults:
  mode: adjudication
  fallback_policy:
    max_fallback_rounds: 1
    on_insufficient_evidence: ingest
    on_unresolved_conflict: ingest
    on_unresolved_claims: revise
    on_keep_with_caveat: revise
  observe_policy:
    on_contradiction: reopen
    sources:
      - name: health
        command: sh
        args: ["-c", "printf HEALTHY"]
        success_pattern: HEALTHY

roles:
  proposers: [proposer-a]
  challengers: [challenger-a]
  arbiter: arbiter-a
  semantic_verifier: verifier-a
```

如果希望 `adjudicate` 自动补抓新证据，可以继续加：

```yaml
defaults:
  ingest_policy:
    sources:
      - name: fresh-evidence
        command: sh
        args:
          - -c
          - printf '{"status":{"ok":true},"report":{"summary":"fresh evidence captured","excerpt":"fresh evidence excerpt","score":0.92}}'
        source_type: external_command
        reference: sh -c printf fresh-evidence
        parsing:
          mode: json
          success_path: status.ok
          summary_path: report.summary
          excerpt_path: report.excerpt
          metadata_paths:
            score: report.score
```

### 2. `free_debate`

```yaml
defaults:
  mode: free_debate
  debate_policy:
    min_rounds: 2
    max_rounds: 3
    vote_threshold: 0.75

roles:
  participants: [debater-a, debater-b, debater-c]
  reporter: reporter-a
```

### 3. `delphi`

```yaml
defaults:
  mode: delphi
  delphi_policy:
    min_rounds: 2
    max_rounds: 4
    convergence_threshold: 0.8

roles:
  participants: [participant-a, participant-b, participant-c]
  facilitator: facilitator-a
  reporter: reporter-a
```

如果你同时使用 `--input ./run.yaml`：

- `run.yaml` 里的 `roles` 会覆盖 config 里的 `roles`
- 如果你希望同一份 `run.yaml` 能复用不同 provider/profile，建议把角色映射放在 config 里，把 `run.yaml` 只用于 `task_spec` 和 policy

## 增量编辑

如果模板已经生成，后面只想补一个 provider 或 agent，可以用：

```bash
til-consensus config add-provider --help
til-consensus config add-agent --help
```

`config add-agent --assign` 目前支持：

- `proposer`
- `challenger`
- `arbiter`
- `semantic-verifier`
- `reporter`
- `actor`
- `participant`
- `facilitator`

这两个命令适合增量修改，不适合替代模板初始化。

`config add-provider --protocol` 目前支持：

- `openai-compatible`
- `openai-responses`
- `anthropic-compatible`
- `gemini-api`

如果你想手动接入 Gemini API，最小 provider 配置可以写成：

```yaml
providers:
  gemini-api:
    type: api
    protocol: gemini-api
    base_url: https://generativelanguage.googleapis.com/v1beta
    api_key_env: GEMINI_API_KEY
    models:
      default:
        provider_model: gemini-3.5-flash
```

如果你要接兼容网关，例如 OpenRouter、Kimi、DeepSeek、Qwen 百炼兼容模式或公司内代理，建议记住：

- `OpenAI Chat Completions 风格网关`
  - 用 `openai-compatible`
- `OpenAI Responses API / Qwen 百炼 Responses 兼容模式`
  - 用 `openai-responses`
- `Anthropic 风格网关`
  - 用 `anthropic-compatible`
- `Gemini generateContent`
  - 用 `gemini-api`

这些 API provider 当前都支持下面这些细配能力：

- `base_url`
- `api_key_env`
- `headers`
- `models.<id>.provider_model`
- `models.<id>.context_window`
- `models.<id>.max_output_tokens`
- `models.<id>.temperature`
- `models.<id>.reasoning`
- `options`

`max_output_tokens` 只适用于 API provider。CLI provider 当前没有稳定的一等 output-token 参数；如果在 CLI provider 的 `models.*.max_output_tokens`、`options.max_output_tokens_field`、`args: --max-output-tokens` 等位置声明 token budget，`profile preflight` / `config validate` 会直接报错，避免误以为该限制已经传给 CLI。

CLI provider 的 `models.<id>.reasoning` 是 provider-specific 映射：`claude` 会生成 `--effort <value>`，`codex` 会生成 `-c model_reasoning_effort=<value>`。`gemini` / `antigravity` 当前本机 CLI 未暴露稳定 thinking-level 参数，因此不会声明 `reasoning` 已生效。

API provider 中，`gemini-api` 使用官方 `google.golang.org/genai` 的 `Models.GenerateContent`。`models.<id>.reasoning` 会映射到 Gemini `generationConfig.thinkingConfig.thinkingLevel`，支持 `minimal / low / medium / high`；如果你在 `options.extra_body.generationConfig.thinkingConfig` 中显式配置 thinking，则显式配置优先。

`config add-provider` 里也可以直接写：

- `--context-window`
- `--max-output-tokens`
- `--header KEY=VALUE`
- `--option KEY=VALUE`

`options` 目前支持的常用键：

- 通用
  - `endpoint_path`
  - `structured_output_mode`
  - `api_key_header`
  - `api_key_prefix`
  - `api_key_query_param`
  - `extra_body`
  - `timeout_ms`
- `openai-compatible`
  - `max_output_tokens_field`
  - `reasoning_field`
  - `response_format_name`
- `openai-responses`
  - `response_format_name`
- `anthropic-compatible`
  - `anthropic_version`
- `gemini-api`
  - `response_mime_type`
  - `response_schema_field`
  - `api_version`

`openai-responses` 使用官方 `github.com/openai/openai-go/v3` 的 `Responses.New`。`endpoint_path` 只支持 `/responses`，代理路径前缀请放到 `base_url`；`max_output_tokens` 会固定映射为 Responses API 的 `max_output_tokens`，结构化输出会写入 `text.format`。

`gemini-api` 的 `endpoint_path` 只支持默认 `models/{model}:generateContent`，或带 API version 前缀的 `v1beta/models/{model}:generateContent` 形式。自定义代理路径请优先放在 `base_url` 中；SDK 会负责发送 camelCase payload，例如 `maxOutputTokens / responseMimeType / responseJsonSchema / thinkingConfig`。

可直接复制的完整样例：

- [openai-compatible.config.yaml](examples/openai-compatible.config.yaml)
- [anthropic-compatible.config.yaml](examples/anthropic-compatible.config.yaml)
- [gemini-api.config.yaml](examples/gemini-api.config.yaml)
- [antigravity.config.yaml](examples/antigravity.config.yaml)
- [openrouter.config.yaml](examples/openrouter.config.yaml)
- [kimi.config.yaml](examples/kimi.config.yaml)
- [deepseek.config.yaml](examples/deepseek.config.yaml)
- [qwen-max.config.yaml](examples/qwen-max.config.yaml)

## profile preflight

`config validate` 检查完整 workflow 配置；如果你要确认 API key、base url、CLI 登录态和模型名真的能用，使用 `profile preflight`。

常用命令：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
til-consensus profile preflight --config ./til-consensus.yaml --provider deepseek-api
til-consensus profile preflight --config ./til-consensus.yaml --agent arbiter-qwen-max
til-consensus profile preflight --config ./til-consensus.yaml --config-profile fast --all
til-consensus profile preflight --config ./til-consensus.yaml --all --web --open
til-consensus profile preflight --config docs/examples/deepseek.config.yaml --provider deepseek-api --output ./out/{requestId} --verbose
```

行为：

- `--all` 检查配置里的所有 provider；不传 `--provider/--agent` 时默认等价于 `--all`。
- `--provider` 按 provider id 过滤，可重复传入，也可以逗号分隔。
- `--agent` 按 agent id 检查，会使用该 agent 的 provider、model、temperature、reasoning 覆写。
- `--output` 只覆盖本次 preflight 的 `output.directory`，不会写回配置文件。
- 相对 `output.directory` 按当前执行目录解析，而不是按配置文件所在目录解析。
- `profile preflight` 默认只校验 provider / agent profile，不要求 `roles.proposers / roles.challengers / participants` 等 workflow 角色完整。
- 如果传了 `--agent`，该 agent 仍必须正确引用已存在的 provider 和 model。
- API provider 会先检查 `api_key_env` 对应环境变量是否存在；不会把 key 写进 artifact。
- 每个 provider 会执行带 schema 的最小非交互 JSON 探测：要求返回 `{"ok": true}`。
- 多个 provider 会逐个探测并分块输出：每个 provider 完成后立即打印该 provider 的 readiness，最后再打印 `profile preflight completed ready=x/y` 和 artifact 路径。
- stdout 是真实终端时，最终 summary 全部 ready 会显示为绿色，否则显示为红色。
- API provider 的 preflight 默认探测预算是 `max_output_tokens=2048`；如果该 API model 显式配置了更小的 `max_output_tokens`，则使用配置值。CLI provider 不支持在配置里声明 output-token budget。
- 结果会写到标准输出目录，并生成 `artifacts/provider-readiness.json`，可被 `view` 和 `telemetry daily` 读取。

推荐验证顺序：

```bash
til-consensus config validate --config ./til-consensus.yaml
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
til-consensus view --result ./out/tc_xxx/result.json --section debug --verbose
```

常见失败含义：

- `env XXX is not set`
  - `api_key_env` 指向的环境变量没有配置，先 `export XXX=...`
- `binary <name> not found`
  - CLI provider 的本地命令不存在或不在 `PATH`
- `request failed: status=401/403`
  - API key、base url 或 provider 账号权限不正确
- `request failed: status=429`
  - 当前 provider 被限流，换模型、降低频率或稍后重试
- `did not return a recoverable JSON object`
  - provider 可调用，但当前 CLI/API 输出不满足最小 JSON 契约，需要检查模型名、structured output 能力或 provider-specific 参数
- `gemini response contains no text parts ... finishReason=MAX_TOKENS`
  - Gemini API thinking 模型可能把输出预算消耗在思考阶段；提高该 API model 的 `max_output_tokens`，或在 provider `options.extra_body.generationConfig` 中按目标网关支持情况降低/关闭 thinking

如果只想验证新增 API profile，可以先导出对应 key：

```bash
export DEEPSEEK_API_KEY=...
til-consensus profile preflight --config docs/examples/deepseek.config.yaml --all --verbose

export BAILIAN_API_KEY=...
til-consensus profile preflight --config docs/examples/qwen-max.config.yaml --all --verbose
```

如果你想先做不消耗 token 的本机检查：

```bash
til-consensus doctor --config ./til-consensus.yaml
```

如果要把 provider preflight 也纳入 doctor：

```bash
til-consensus doctor --config ./til-consensus.yaml --providers --verbose
```

## follow-up 与 session store

CLI 会把 session snapshot 持久化到输出目录同级的 `_sessions/` 下，例如：

- `./out/_sessions/session_xxx.json`

常用命令：

```bash
til-consensus followup run --config ./til-consensus.yaml --artifact ./out/parent-run/artifacts/followups/case.json
til-consensus run --config ./til-consensus.yaml --followup ./out/parent-run/artifacts/followups/case.json
til-consensus run --config ./til-consensus.yaml --resume-session session_xxx
til-consensus run --config ./til-consensus.yaml --replay-session session_xxx
til-consensus session list --config ./til-consensus.yaml --request-id tc_xxx
til-consensus session show --config ./til-consensus.yaml --session-id session_xxx
```

行为区别：

- `run --resume-session`
  - 当前会对 `adjudication` workflow 执行 checkpoint 级恢复
  - 也就是从最近一次已完成 phase 继续，而不是从 `frame` 重新开始
- `run --replay-session`
  - 不复用旧 session 的 phase 进度
  - 会生成新的 request id，并把旧 session 挂成 lineage 父节点
