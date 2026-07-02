# 技术架构

这篇面向维护者，描述 `til-consensus` 的 library / CLI 边界。第一次使用不需要先读这里。

## 核心目标

`til-consensus` 是一个确定性 coordinator，加上可插拔 provider 执行层。它既可以通过 CLI 使用，也可以作为 Go library 嵌入。它的目标不是让模型自由聊天，而是把多 agent 讨论过程落成可审计产物：

- task 被标准化为 case。
- provider 输出被结构化约束。
- claim、challenge、verification、revision、adjudication 都进入 ledger。
- raw input/output 和 schema 失败单独落盘。
- telemetry 衡量 provider 合规性和系统兜底行为。

## 包边界

面向外部 Go 项目的包位于模块根目录：

- `consensus`：核心 engine、workflow、request/result/task 类型。
- `config`：配置 schema、include/profile 加载、run plan 解析、render/explain 报告。
- `runner`：高层执行入口，封装 config 解析、engine 创建、run/resume/replay/action/classify。
- `preflight`：provider readiness 预检。
- `telemetry`：readiness、strict compliance、run telemetry、daily report 类型和聚合。
- `doctor`：配置、输出目录和 provider 可用性诊断。
- `runtime`：provider delegate、schema enforcement、repair、normalize。
- `runtime/api`、`runtime/cli`、`runtime/mock`、`runtime/sdk`：provider runner。
- `store/file`、`store/memory`：session store 实现。
- `observer`：JSONL event/ledger observer。

`internal/*` 仅保留 CLI 专用能力：

- `internal/app`：CLI 命令，运行类命令应尽量只是 `runner.Executor` 的薄封装。
- `internal/artifact`：CLI 输出文件、summary、error artifact。
- `internal/viewer`：终端/Web 展示。
- `internal/buildinfo`：CLI build metadata。

## Engine 边界

`Engine.Start()` 的职责：

1. 规范化 `StartRequest`。
2. 初始化 session、ledger、observer、artifact writer。
3. 发射公共 lifecycle event。
4. 按 `mode` 分发到 workflow runner。
5. 统一执行 report/action/observe。
6. 写出 result、summary、ledger、events、artifacts。

Engine 是 workflow dispatcher，不应该把 provider-specific 逻辑写进 workflow。

## Workflow runner

当前 workflow：

- `adjudication`
  - claim-centric 裁决链路。
  - 支持 verifier、revise、fallback、observe。
- `free_debate`
  - participant 平等辩论，最后 final vote。
- `delphi`
  - 匿名问卷、聚合摘要、评分修订和收敛。

Workflow runner 只关心阶段语义和领域对象，不关心某个 provider 是 CLI 还是 API。

## Provider 执行层

Provider 执行层负责：

- 构造 provider-specific request。
- 调用 CLI/API/SDK。
- 保存 input/raw/failure artifact。
- 做 schema enforcement。
- 把原始输出 decode 为 task output。

CLI/API 能提供结构化输出能力时，应优先使用结构化接口，而不是只靠 prompt 要求 JSON。

当前边界：

- 语法修复和 schema enforcement 分离。
- 严格 schema 优先。
- 原始错误单独落盘。
- 同源 provider repair 优先于额外 refiner 模型。

## Schema 与 normalize

Schema 防线分层：

1. Provider-specific prompt / tool / schema contract。
2. 语法级 recovery：只处理 code fence、前后文本、trailing comma 等包裹性问题。
3. Schema enforcement：强类型 decode、枚举和必填字段校验。
4. 同源带错重试：把具体错误反馈给原 provider 修正结构。
5. Normalize：只保留无歧义转换，例如字符串数字到数字、百分号到小数、大小写/命名风格归一。
6. Hard fail：仍不合规则失败，并保留 raw error artifact。

Normalize 不应成为隐式 spec。语义映射、枚举拍平和信号丢失都应该进入 taxonomy/schema/prompt 或 hard fail 路径。

## Telemetry

Telemetry 关注：

- provider readiness。
- provider × task strict compliance。
- schema repair 是否触发。
- normalize 规则是否触发。
- raw verdict / raw task verdict / raw disposition。
- unresolved / undetermined 分布。
- provider failure 分类。

目标是区分：

- provider 严格按 schema 输出。
- provider 漂移但被 repair/normalize 修好。
- provider 无法修复，需要 schema/prompt/provider 配置加固。

## Result / ledger / artifact

统一输出壳：

- `result.json`：最终结构化结果。
- `summary.md`：人工阅读摘要。
- `ledger.jsonl`：append-only 审计账本。
- `events.jsonl`：运行期事件。
- `artifacts/`：输入、原始输出、失败、telemetry、readiness。

Artifact 命名必须避免覆盖，尤其是 semantic/raw artifact。每轮 provider 调用都应该可以被独立追踪。

## 当前取舍

- 默认 workflow 仍是 `adjudication`，因为它最适合审计。
- CLI provider 没有稳定 token budget 参数时，不伪装支持。
- API provider 支持 base url、model、headers、options，方便接 OpenRouter、DeepSeek、Kimi、DashScope 等网关。
- 真实 provider E2E 不进入默认 CI，避免把外部服务状态引入基础质量门禁。
