# til-consensus 多工作流引擎

这是 `til-consensus` 的技术设计说明，不是第一次上手的入口。

如果你想先跑通 CLI，先看：

- [README](../README.md)
- [文档首页](index.md)

## 目标

`til-consensus` 当前不是单一 workflow，而是一个多工作流引擎。

支持的 workflow：

- `adjudication`
  - 目标是基于证据做高置信度裁决
- `free_debate`
  - 目标是让多个 participant 充分讨论，再对 active claims 做最终投票
- `delphi`
  - 目标是通过匿名多轮问卷形成收敛结论

默认 `mode=adjudication`。

## 顶层架构

`Engine.Start()` 负责：

1. 规范化 `StartRequest`
2. 初始化 session / ledger / observer
3. 发射公共 lifecycle 事件
4. 按 `mode` 分发到具体 workflow runner
5. 统一执行 report / action / artifact 写出

也就是说，现在的 `Engine` 本质上是一个 workflow dispatcher。

## 三种 workflow

### 1. `adjudication`

固定宏观状态机：

1. `frame`
2. `ingest`
3. `propose`
4. `challenge`
5. `verify`
6. `revise`
7. `adjudicate`
8. `report`
9. `action`
10. `observe`

核心对象：

- `CaseManifest`
- `ClaimNode`
- `ChallengeTicket`
- `VerificationResult`
- `ClaimRevisionRecord`
- `AdjudicationRecord`
- `ArbiterReport`

适合：

- patch 是否真的修复了 bug
- benchmark 主张是否成立
- 文档或架构结论是否有足够证据

关键语义：

- `frame` 先把任务收敛成标准化 case
- `verify` 优先 deterministic checks
- `revise` 不是可选装饰，而是强制 proposal 侧对 challenge/verification 作出响应
- `adjudicate` 在 claim 级完成，输出 disposition，而不是只选最像对的整篇回答
- `observe` 用来记录 action 之后的后验证据

### 2. `free_debate`

固定流程：

1. `initial`
2. `debate` 若干轮
3. `final_vote`
4. `report`
5. `action`

关键语义：

- 每个 participant 独立提出初始 claims
- 每轮收到匿名化 / 去重后的 peer claims 与上轮摘要
- 输出 judgement、revised claim、merge suggestion
- 达到 `min_rounds` 后，若没有新 claim / merge 且 judgement 全为 `agree` 或 `no_change`，允许 early stop
- 最终由 `final_vote` 决定哪些 claims 进入结果

### 3. `delphi`

固定流程：

1. `delphi_questionnaire`
2. `delphi_summary`
3. `delphi_revision`
4. 重复直到收敛或达到 `max_rounds`
5. `report`
6. `action`

关键语义：

- 每轮 participant 独立且匿名回答同一问题
- 其他 participant 看不到身份和原文归属
- coordinator 或 facilitator 产出匿名聚合摘要
- 下一轮基于聚合摘要修订评分与理由
- 若收敛度达到 `convergence_threshold`，提前结束

## 统一请求模型

`StartRequest` 顶层统一包含：

- `mode`
- `taskSpec`
- `roles`
- `reportPolicy`
- `actionPolicy`
- `waitingPolicy`

并按 mode 挂不同 policy：

- `proposalPolicy`
- `verificationPolicy`
- `arbiterPolicy`
- `debatePolicy`
- `delphiPolicy`

角色模型也随 mode 变化：

- `adjudication`
  - `proposers`
  - `challengers`
  - `arbiter`
  - `semantic_verifier`
  - `reporter`
  - `actor`
- `free_debate`
  - `participants`
  - `reporter`
  - `actor`
- `delphi`
  - `participants`
  - `facilitator`
  - `reporter`
  - `actor`

## 统一结果壳

所有 workflow 都写成统一 `RunResult`：

- `schemaVersion`
- `mode`
- `requestId`
- `sessionId`
- `taskSpec`
- `caseManifest`
- `terminalState`
- `report`
- `action`
- `observations`
- `metrics`
- `error`

并且只挂一个 mode-specific section：

- `adjudication`
- `freeDebate`
- `delphi`

`view` 就是靠这个统一壳在运行时决定如何渲染。

## ledger 与 events

`ledger.jsonl` 是统一账本，但 `kind` 会随 workflow 扩展。

例如：

- adjudication：
  - `claim_proposed`
  - `challenge_opened`
  - `deterministic_check`
  - `semantic_verification`
  - `arbiter_decision`
- free_debate：
  - `debate_round_opened`
  - `debate_round_output`
  - `debate_vote_cast`
- delphi：
  - `delphi_round_opened`
  - `delphi_response_recorded`
  - `delphi_round_summary`
  - `delphi_convergence_reached`

`events.jsonl` 继续只做运行日志。

## 裁决式循环与终态

`adjudication` 不再是完全线性的。

允许的关键回路：

- `challenge -> verify -> revise`
- `revise -> challenge`
- `revise -> verify`
- `adjudicate -> revise`
- `adjudicate -> ingest`
- `action -> observe`

循环受这些策略限制：

- `loop_policy.max_revision_rounds`
- `loop_policy.max_verification_rounds`
- `loop_policy.material_confidence_delta`
- `waiting_policy.global_deadline`

非成功终态：

- `insufficient_evidence`
- `unresolved_conflict`
- `requires_human_review`
- `action_blocked_by_risk`

## CLI 与模板选择

`run` 新增了 mode 相关 flags：

- `--mode adjudication|free-debate|delphi`
- `--participants`
- `--facilitator`
- `--min-rounds`
- `--max-rounds`
- `--vote-threshold`
- `--convergence-threshold`

`config init` 现在把模板选择拆成 3 个正交维度：

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

旧的 `--preset` 仍保留为兼容别名，不再是主概念。

## 当前边界

- 保持 `adjudication` 默认行为不变
- `view` 同时支持统一结果壳和历史 adjudication 结果
- 仍然不提供 E2E
- 仍然不提供公共 SDK
