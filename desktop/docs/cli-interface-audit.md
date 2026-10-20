# 四种 CLI 的真实接口审计与本地内核升级

审计日期：2026-10-20，Asia/Kuala_Lumpur（UTC+8）。

四家 CLI 均已完成真实模型推理，升级后的 Go Runner 也已分别通过真实项目事实核对。本地 `til-consensus` 接口已据此升级。
初始验证针对独立 CLI 请求及独立 Runner 任务，当时桌面应用处于 M0；后续产品完整运行暴露了更多接口差异，均保留原始失败。修复后已完成真实四方自由辩论、停止恢复及三张有效完整票验收，见 [实现与验收说明](implementation.md)。本次最终判定为 `no_consensus`，不能将流程通过当作意见一致。

当前 `0.5.0` 的模型／推理设置、目录读取和请求／报告边界另见 [CLI 设置说明](cli-settings.md)。下文保留早期审计输入快照，不改写历史版本。

## 证据与核对范围

原始输入取自 [package.json](../package.json) 和 [services/host/go.mod](../services/host/go.mod)，没有让模型自行猜测项目依赖。
输入内容、SHA-256、期望值和 schema 见 input-manifest.json（原本地私有记录 `.newtc/cli-audit/2026-10-20/input-manifest.json`，未导入） 与 schema.json（原本地私有记录 `.newtc/cli-audit/2026-10-20/schema.json`，未导入）。

- 初次 Runner 审计：til-runner-live-summary.json（原本地私有记录 `.newtc/cli-audit/2026-10-20/til-runner-live-summary.json`，未导入）。
- 修复后的补验：til-runner-repaired-summary.json（原本地私有记录 `.newtc/cli-audit/2026-10-20/til-runner-repaired-summary.json`，未导入）。
- CLI 原生审计：Codex（原本地私有记录 `.newtc/cli-audit/2026-10-20/codex/summary.json`，未导入）、Claude（原本地私有记录 `.newtc/cli-audit/2026-10-20/claude/summary.json`，未导入）、已登录 Grok（原本地私有记录 `.newtc/cli-audit/2026-10-20/grok/summary.logged-in.json`，未导入）、AGY（原本地私有记录 `.newtc/cli-audit/2026-10-20/agy/summary.json`，未导入）。

核对六组字段：项目名、Electron 版本、React 版本、Go 声明版本、共识库版本及完整脚本集合；审计输入快照中的脚本共 9 项。后续实现新增的打包依赖和脚本不属于这份历史快照。

| 字段 | 真实输入中的期望值 |
| --- | --- |
| `project_name` | `newtc` |
| `electron_version` | `44.9.0` |
| `react_version` | `19.3.1` |
| `go_version` | `1.26.1` |
| `consensus_version` | `v0.0.0-20260803125524-b505773800a4` |
| `scripts` | `dev`、`start`、`build`、`build:host`、`check`、`test:host`、`test:rpc`、`test:e2e`、`test` |

四家最终输出均通过上述核对。`risk` 只检查存在且非空；模型关于 SDK、密钥、版本漂移或计费的风险表述属于推断，不能据此认定项目实际走 API、泄露凭据或发生版本漂移。

## 版本、实际模型与 Runner 后验结果

| CLI | 本轮版本 | 实际模型及来源 | 升级后真实 Runner 耗时 |
| --- | --- | --- | ---: |
| Codex | `0.163.0` | app-server 的 `thread/start` 返回 `gpt-6.1-sol`；exec 本身未标明模型 | 17.666 秒 |
| Claude Code | `2.1.303` | `system/init` / `modelUsage`：`claude-opus-5-5` | 7.544 秒 |
| Grok Build | `1.0.62 (1474f9cbdd91)` | 默认请求的 `modelUsage` canonical：`grok-4.7-build`；不是 CLI selector | 42.348 秒（修复后） |
| AGY | `1.4.2` | CLI `/model`：`gemini-3.8-flash-medium` | 26.258 秒（修复后） |

表中时间来自 Runner 审计脚本的实际进程计时，包含启动、认证、推理与解析，不能直接作为稳定性能指标。
Codex exec 的实际模型没有独立元数据证明，不能把 app-server 观测值直接冒充 exec 已确认的模型。
Grok 早期 `1.0.34` 请求因未登录失败；该失败记录仍保留，不能与后续已登录的 `1.0.62` 成功请求混算。
以上 Grok 原生与 Runner 成功请求均未传 `--model`，只证明默认模型可用，不能视为显式 `--model grok-4.7-build` 已成功。

## 实测协议差异

### Codex

`exec --json` 返回 JSONL 的 `thread.started`、`turn.started`、`item.completed`、`turn.completed`；这是完整 item 事件，并非逐 token 正文流。
原生 schema 请求使用 `--output-schema`，最终正文可从 `--output-last-message` 读取；读取答案文件前仍须确认任务终态，避免把失败前的内容当成成功。
这些参数的用途见 [Codex 官方非交互文档](https://developers.openai.com/codex/noninteractive)。

另行实测了 app-server：收到首个公开正文片段后，通过 `turn/interrupt` 取消，6 毫秒后收到 `turn/completed`，状态为 `interrupted`，进程退出码为 0。
初始审计只验证了该协议；后续 M1 已将桌面 Codex 主通道接为 app-server，并使用每任务独立的临时 thread/turn。模型、schema、公开增量和中断均经过该通道；并未复用含历史任务的长期 thread。
证据见 app-server-summary.json（原本地私有记录 `.newtc/cli-audit/2026-10-20/codex/app-server-summary.json`，未导入），协议参考 [Codex App Server 文档](https://developers.openai.com/codex/app-server)。

### Claude Code

原生 JSON 成功响应是 `type: result` 的 envelope，优先读取 `structured_output`，否则读取非空 `result`；本次原生 schema 与完整 stream-json 均通过事实核对。
流式参数为 `--output-format stream-json --verbose --include-partial-messages`，公开正文来自 `stream_event.event.delta.text` 的 `text_delta`。
这些入口和 envelope 字段见 [Claude 官方非交互文档](https://code.claude.com/docs/en/headless)。

本机实测在首个正文片段后发送 SIGINT，进程仍以 **0** 退出、stderr 为空，但最终 `is_error=true`、`subtype=error_during_execution`、`terminal_reason=aborted_streaming`。
因此完成判定必须检查协议终态；只看退出码会把取消误判为成功。原生 schema 成功的 `stop_reason=tool_use` 也不能单独作为失败依据。
多个 assistant 内容块重复携带 usage；应使用最终汇总，不能把所有事件的 token 数相加。取消时汇总为 0 也不能证明没有消耗。
本次禁用了工具、MCP 和自定义扩展；`--safe-mode` 保留现有认证，不能默认换成不读取订阅 OAuth/keychain 的 `--bare`。

### Grok Build

原生 JSON 使用驼峰字段 `structuredOutput`，并带 `text`、`stopReason`；成功终态为 `end_turn`。本机输出是多行缩进 JSON，不能逐行当 JSONL 解析。
原生流的 CLI 参数是 `streaming-json`，正文增量位于 `type: text` 的 `data`；最终 `type: end` 带终态与汇总，没有正文快照。
`streaming-messages-json` 是另一套协议；统一配置里的 `stream-json` 必须映射为原生 `streaming-json`。参数说明见 [Grok 官方非交互文档](https://docs.x.ai/build/cli/headless-scripting)。

`--json-schema` 会强制 JSON 输出；使用原生增量流时，适配器改为在输入中提供 schema，并由 Runner 在成功终态后使用 JSON Schema 严格验证业务输出。
首个正文后 SIGINT 的真实结果为退出码 **130**，只留下部分 `text`，没有 `end`；不能用部分正文推定完成，也不能要求取消时一定有终态事件。
初次 Runner 的 OnEvent 把缩进 JSON 当 JSONL，实际失败；修复后真实 Runner 42.348 秒完成，全部事实通过，原失败产物未删除。
随后真实产品完整运行把返回 canonical `grok-4.7-build` 当作 `--model` selector，CLI 明确报 `unknown model id`；这是独立接口审计未覆盖显式模型选择暴露的真实失败。
原失败见 Grok 显式模型选择产物（原本地私有记录 `.newtc/live-validation/discussion-2026-10-19T17-54-07-835Z/host/runs/run-5797a4e6b58d3329e2d6feae/artifacts/attempt-8d2e9cbcaf07945ef05ec8e9/attempt-8d2e9cbcaf07945ef05ec8e9/attempt-8d2e9cbcaf07945ef05ec8e9-inv-0004.output.json`，未导入）。
后续只读 `grok models` 确认默认 selector 为 `grok-4.7`，另有 `grok-4.7-build-fast` 等选项。本地 profile、示例和配置测试已改为 `grok-4.7`；正确 selector 的完整全场验证由后续运行单独记录，不能沿用此前默认请求的成功结论。

第三轮产品运行已使用正确 selector，返回 canonical `grok-4.7-build`，但流式 Runner 的本地 schema 编译器不接受 Go 构造的嵌套 `[]string`。修复将 schema 先序列化、再解码为标准 JSON 类型，并保留数值精度；没有放宽 schema。保存的同一真实输出离线通过原 schema、invocationId 与 contextHash 校验，证实该次故障位于本地校验器。新增回归覆盖嵌套 enum/required、组合约束和无效输出。

### AGY / Antigravity

原生 stdin 契约为 `{"event":"user","message":{"content":"..."}}`，配合 `--input-format stream-json --output-format stream-json`，不是要求模型读取临时文件。
schema 通过 `--json-schema` 提供，答案优先取唯一最终 `result.structured_output`；`response` 可能拼接多个 JSON 对象，不保证可直接单对象解析。
官方入口见 [AGY Headless 文档](https://www.antigravity.google/docs/cli/headless/)；本地版本的实际 envelope 仍以捕获结果为准。

真实多任务流曾出现第二个 result 的 `response.risk` 已更新，但 `structured_output.risk` 仍为前一任务值。即使项目版本事实正确，也不能宣称结构化输出具有当前任务的新鲜度。
因此当前使用全新进程、单个 user 输入后 EOF，并拒绝多 result 或终态后活动；暂不复用携带 schema 的多任务会话。
但 **`num_turns=2` 不等于两个人类任务**：CLI 内部 schema 修复会增加计数。修复后的 Runner 就以唯一、fresh、`num_turns=2` 的结果真实通过了核对。
初次以 `num_turns=1` 为硬条件的 Runner 失败已保留；修复只读入同一独立任务的唯一终态，不能靠取最后一个 result 掩盖陈旧数据。
取消实测为退出码 **1**、`status=ERROR`、`error=interrupted`；应结合本地取消意图归为 canceled，而非只匹配某个文档状态字面值。
计划模式加终端 sandbox 不等于系统级只读；桌面应用仍需私有 cwd，并按实际要求增加文件系统或容器隔离。

## 本地内核改动与配置

升级位置：[til-consensus/runtime/cli](../../runtime/cli)。新增 Grok profile，修正 AGY 原生传输，并统一各 CLI 的 envelope、错误、取消和公开进度解析。
公开事件过滤 thinking/reasoning，正文增量与最终快照分开；usage 表示快照，不代表可累加的增量。失败与取消保留原始输出，不能仅靠进程退出码判定业务成功。
新增 `workdir`、`permission_mode`、`output_format`；后两者仅支持 Codex、Claude、Grok、AGY，未实现的 CLI 显式配置会校验失败。

下面保留审计时的 provider 片段及历史工作目录；当前应用的临时产物写入 `.tcview/`，目录须提前创建；完整示例见 [grok.config.yaml](../../docs/examples/grok.config.yaml)。

```yaml
providers:
  grok-cli:
    type: cli
    cli_type: grok
    command: grok
    workdir: ./.tcview/sessions/manual
    permission_mode: read-only
    output_format: json
    env_passthrough: [XAI_API_KEY]
    models:
      default:
        provider_model: grok-4.7
```

另补齐裁决从 created/frame 检查点恢复时遗漏的前奏，并以 7 个持久化阶段检查点及实际 SIGKILL RPC 回归验证已提交任务不重做。

审计阶段以 `commit+local.<hash>` 记录未提交源码，保留了本轮真实运行的原始构建指纹。后续修复已固化为 [cb9149af032e723e12b972d8790c5a97de2c0511](https://github.com/suchasplus/til-consensus/commit/cb9149af032e723e12b972d8790c5a97de2c0511)，其 [核心升级 PR #1](https://github.com/suchasplus/til-consensus/pull/1) 已合并。后续 [格式兼容 PR #2](https://github.com/suchasplus/til-consensus/pull/2) 通过等价局部返回值写法兼容 Go 1.26.1／1.27.1，当时桌面锁定可直接下载的远端模块版本 `v0.0.0-20261019184227-f4e2dbad63e4`，完整提交为 `f4e2dbad63e409ef8dcf490125f06e522ba886b9`。
上述独立项目审计阶段使用远端固定模块与显式本地覆盖，原始验收记录中的旧指纹保持不变。当前 [go-env.mjs](../scripts/go-env.mjs) 已改为同仓 workspace 与独立构建身份，见 [同仓说明](monorepo.md)。

## 保留证据与复验

私有历史完整记录保存在 .newtc/cli-audit/2026-10-20（原本地私有记录 `.newtc/cli-audit/2026-10-20`，未导入），包括认证失败、解析失败、取消和修复后的成功，不能只保留成功样本。
脱敏真实 fixture 位于 [fixtures/cli](../fixtures/cli)，来源、删改范围和摘要见各目录的 provenance/metadata；已删除会话标识、路径与推理内容等不必要字段。
这些 fixture 用于回放协议回归，不代表测试时重新调用模型，也没有用构造内容冒充真实成功。

```bash
cd desktop
node scripts/cli-audit/verify-til.mjs --providers codex,claude,grok,antigravity --label manual-recheck-1
```

审计脚本会发起真实推理并消耗各 CLI 额度，仅供手动复验，禁止接入默认 CI 自动执行。再次运行应换用新的 label，避免覆盖既有证据。

## 测试状态

- 已通过：本地 `til-consensus` 的 `go test ./...`、`go test -race ./...`、`make pre-push`（格式、覆盖率报告、vet、lint、构建）。
- 当时已通过：NewTC 的 `npm test`，包含 Go 后台 race 检查、类型检查、构建、RPC 与 Electron 测试。默认回归中的模型响应为显式测试 Mock，实际 Go 共识引擎和桌面桥接仍真实执行。
- 已通过：桌面 Codex app-server 原生 schema 的独立真实调用。完整四方讨论的结果在产品验收记录中单独列出，不与上述独立审计混算。
