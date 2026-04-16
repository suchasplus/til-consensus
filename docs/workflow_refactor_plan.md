# Workflow Refactor Plan

## 当前设计

当前 `adjudication` workflow 位于：

- [engine.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/engine.go)
- [workflows.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/workflows.go)
- [state.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/state.go)

当前主路径仍然基本是单趟线性流程：

`ingest -> propose -> challenge -> verify -> adjudicate -> report -> action`

现有实现已经具备一些 claim-centric 基础：

- `ClaimNode`
- `ChallengeTicket`
- `VerificationResult`
- `EvidenceRecord`
- `ArbiterReport`

但这些结构仍然主要服务于“生成最终裁决结果”，而不是驱动一个严格的证据化工作流。

## 与目标设计的关键差距

### 1. 缺少 `frame / revise / observe`

- 当前没有 canonical case framing。
- 当前 challenge 和 verify 结束后直接进入 adjudicate，中间没有 proposal side 的结构化修订。
- 当前 action 之后没有 post-action observation。

### 2. 缺少 claim 生命周期控制

当前 claim 只有：

- `proposed`
- `challenged`
- `verified`
- `adjudicated`

但没有：

- 被 challenge/verify 驳回后的修订动作
- caveat / withdrawn / unresolved 这样的中间语义
- claim-level disposition

### 3. 缺少结构化 case artifact

当前没有真正的 `CaseManifest`。

`TaskSpec` 是输入，而不是 frame 后的 canonical case definition。当前也缺少：

- canonical problem statement
- task_type
- risk_level
- required_evidence_level
- out_of_scope
- unresolved_questions

### 4. 缺少 attack / adjudication 的正式 schema

当前 `ChallengeTicket` 已接近 `AttackRecord`，但缺少：

- `attack_type`
- `severity`
- `suggested_falsification_method`

当前 `ArbiterDecision` 也还停留在 verdict 级，不是：

- `keep`
- `keep_with_caveat`
- `unresolved`
- `reject`

这种 claim disposition 模型。

### 5. 缺少 controlled loop

当前 adjudication 基本是单趟执行。

还没有：

- `verify -> revise`
- `revise -> challenge`
- `revise -> verify`
- `adjudicate -> revise/ingest`

这样的受控回路。

也缺少显式 stop conditions：

- max revision rounds
- max verification rounds
- no material confidence change
- insufficient evidence terminal state
- requires human review terminal state

### 6. 缺少 task-type aware policy

当前 verification 主要靠：

- `VerificationPolicy.RequiredChecks`
- `CompositeVerifier`

但还没有把 task type 变成一等策略层。

目前没有明确区分：

- factual / research
- coding
- strategy / decision-support
- operational / execution

这几类任务的默认验证策略和 action gate。

### 7. action 仍然过于直接

当前只要配置了 `action_policy`，就会尝试执行 action。

缺少：

- 风险门禁
- blast-radius 风险控制
- 高风险只产出执行计划，不直接执行

### 8. report 还不够 claim-disposition-first

当前 report 能输出 summary / highlights / next actions，但没有显式区分：

- retained claims
- downgraded claims
- unresolved questions
- recommended next actions

## 拟议重构

### 核心思路

保持固定 macro-workflow，但把 adjudication workflow 改造成：

`frame -> ingest -> propose -> challenge -> verify -> revise -> adjudicate -> report -> action -> observe`

并引入 controlled micro-loops：

- `challenge -> verify`
- `verify -> revise`
- `revise -> challenge`
- `revise -> verify`
- `adjudicate -> terminal / requires_human_review / insufficient_evidence`

### 结构化 artifact

在现有结构上扩展，而不是另起一套平行模型：

- `CaseManifest`
  - 新增
- `EvidenceRecord`
  - 扩展 provenance / source / conflict 字段
- `ClaimNode`
  - 扩展 claim_type / parent_claim_ids / support/opposition / boundary_conditions / disposition
- `ChallengeTicket`
  - 扩展为 attack-centric
- `VerificationResult`
  - 扩展 method / result / raw output / confidence_delta
- `AdjudicationRecord`
  - 新增 claim disposition 结果
- `ObservationRecord`
  - 新增 action 后观察记录

### 策略层

新增 task-type aware policy abstraction：

- 识别 task type
- 生成 `CaseManifest`
- 决定默认 verification profile
- 决定 action gate
- 决定 evidence threshold

### 受控循环

新增 revise task 和 revision loop：

- 对 challenged / failed / inconclusive claims 做结构化修订
- 只对“发生材料性变化”的 claims 重新 challenge / verify
- 达到 stop condition 时结束

### 新终态

新增 terminal state：

- `completed`
- `insufficient_evidence`
- `unresolved_conflict`
- `requires_human_review`
- `action_blocked_by_risk`

## 计划修改的模块

- [internal/consensus/request.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/request.go)
  - 新增 manifest / policy / loop / observe 相关字段
- [internal/consensus/result.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/result.go)
  - 扩展 artifact schema、terminal state、adjudication records
- [internal/consensus/task.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/task.go)
  - 新增 revise task
- [internal/consensus/state.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/state.go)
  - 新增 `frame / revise / observe`
- [internal/consensus/workflows.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/workflows.go)
  - 重写 adjudication workflow 为 claim-centric loop
- [internal/consensus/claims.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/claims.go)
  - 扩展 claim / attack / disposition 操作
- [internal/consensus/verifier.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/verifier.go)
  - 接入 task-type policy 与 confidence delta
- [internal/consensus/arbiter.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/arbiter.go)
  - 改成 claim disposition 驱动
- [internal/consensus/report.go](/Users/suchasplus/agentic/til-consensus/internal/consensus/report.go)
  - report 按 retained / downgraded / unresolved 输出
- [internal/artifact/summary.go](/Users/suchasplus/agentic/til-consensus/internal/artifact/summary.go)
  - summary 适配新阶段与终态
- [internal/app/output.go](/Users/suchasplus/agentic/til-consensus/internal/app/output.go)
  - phase/event 文案更新
- [internal/viewer/render.go](/Users/suchasplus/agentic/til-consensus/internal/viewer/render.go)
  - 适配新字段与终态

## 兼容性考虑

- 保留现有 `RunResult` 外壳，不重写整个 CLI / artifact 管线。
- `free_debate` 和 `delphi` workflow 尽量不动主语义。
- 旧 adjudication result 的读取兼容继续保留。
- 旧字段尽量保留，但会被更严格的新字段覆盖。

## 风险

- adjudication workflow 的状态机会更复杂，测试必须同步补齐。
- 扩展 schema 后，viewer / summary / docs 很容易漂移，需要一起更新。
- task-type aware policy 先做最小可用版本，避免为了“通用性”引入过度抽象。

## 最小可行交付

这次优先确保：

1. `frame / revise / observe` 真正落地
2. adjudication 变成 claim-centric
3. 具备 revision loop 与终态
4. action 默认受风险门禁
5. 测试和文档跟上

更复杂的自动 re-ingest / reopen follow-up case 先做可审计占位，不强行做成完整自治系统。
