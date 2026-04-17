# 工作流与状态机

这份文档描述 `til-consensus` 当前支持的 3 种 workflow：

- `adjudication`
- `free_debate`
- `delphi`

如果你是第一次上手，先看：

- [README](../README.md)
- [配置说明](config.md)
- [终端 `view` 用法](view.md)

## 工作流总览

### `adjudication`

固定宏观阶段：

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

特点：

- 默认模式
- claim-centric
- evidence-driven
- 支持 verifier / revise / fallback / observe
- 支持 `adjudicate -> revise/ingest` 的受控闭环

### `free_debate`

固定宏观阶段：

1. `frame`
2. `ingest`
3. `initial`
4. `debate`
5. `final_vote`
6. `report`
7. `action`
8. `observe`

特点：

- participant 先独立提出初始立场
- 再做若干轮自由辩论
- 最后通过 `final_vote` 对 active claims 投票
- 不走当前 `verifier / arbiter` 路径

### `delphi`

固定宏观阶段：

1. `frame`
2. `ingest`
3. `delphi_questionnaire`
4. `delphi_summary`
5. `delphi_revision`
6. 重复直到收敛或达到 `max_rounds`
7. `report`
8. `action`
9. `observe`

特点：

- participant 之间保持匿名
- coordinator 或 facilitator 只产出匿名聚合摘要
- 最终结论来自收敛结果，而不是 arbiter 判决

## 统一结果与产物

三种 workflow 共用一套顶层结果壳和审计入口：

- `result.json`
- `ledger.jsonl`
- `events.jsonl`
- `summary.md`
- `artifacts/`

统一顶层结果至少包含：

- `mode`
- `requestId`
- `sessionId`
- `taskSpec`
- `caseManifest`
- `terminalState`
- `report`
- `action`
- `observations`

然后按 `mode` 挂一个 mode-specific section：

- `adjudication`
- `freeDebate`
- `delphi`

## `adjudication`

### 核心 artifact

#### `CaseManifest`

把原始任务收敛为可审计的 case 定义，至少包含：

- `case_id`
- `canonical_problem_statement`
- `task_type`
- `constraints`
- `success_criteria`
- `out_of_scope`
- `risk_level`
- `required_evidence_level`
- `allowed_tools`
- `unresolved_questions`

#### `EvidenceLedger`

账本是 append-only 的，claim、challenge、verification、report、action、observe 都会写入 `ledger.jsonl`。

常见字段：

- `entryId`
- `kind`
- `claimId`
- `challengeId`
- `verificationId`
- `source`
- `sourceType`
- `sourceLocator`
- `summary`
- `contentExcerpt`
- `provenanceQuality`
- `firstHandVsSecondHand`
- `artifact`
- `metadata`

#### `Claim`

claim 是裁决的一等对象，不是整段 prose 的副产物。

关键字段：

- `claim_id`
- `claim_text`
- `claim_type`
- `parent_claim_ids`
- `supporting_evidence_ids`
- `opposing_evidence_ids`
- `confidence`
- `boundary_conditions`
- `status`
- `disposition`

#### `AttackRecord`

challenge 必须针对具体 claim。

关键字段：

- `attack_id`
- `target_claim_id`
- `attack_type`
- `severity`
- `attack_text`
- `suggested_falsification_method`

#### `VerificationResult`

verification 结果会明确挂到 claim 上，而不是只停留在 worker prose。

关键字段：

- `verification_id`
- `target_claim_id`
- `method`
- `result`
- `raw_output_reference`
- `confidence_delta`
- `notes`

#### `AdjudicationRecord`

最终 claim 级裁决。

关键字段：

- `target_claim_id`
- `disposition`
- `rationale`
- `final_confidence`
- `blocking_risks`
- `actionability`

### 阶段语义

#### 1. `frame`

coordinator 先生成 `CaseManifest`，把任务标准化。

这个阶段不会让 proposer 直接自由发挥，避免下游 reasoning 全都锚定在同一份模糊题意上。

#### 2. `ingest`

把材料、上下文、工具输入规范化写入 `EvidenceLedger`。

除了静态 `task_spec.materials`，现在还支持 `ingest_policy.sources`：

- coordinator 调用外部命令抓取或归一化证据
- stdout/stderr 会落到 `artifacts/`
- 结果写成 `source_material` ledger entry
- metadata 会记录 `reason=initial` 或 `reason=fallback-N`

#### 3. `propose`

proposer 生成 claim set。

要求：

- claim 尽量原子化
- 多 proposer 时先 blind proposal，再暴露他人输出
- 保留 proposer provenance

#### 4. `challenge`

challenger 不能只挑战整篇答案，必须明确：

- target claim
- attack type
- severity
- falsification method

#### 5. `verify`

优先 deterministic verification，再用 semantic verifier 兜底。

当前 task type 策略：

- `factual`
  - 优先来源质量、冲突检测、时效性
- `coding`
  - 优先 workspace snapshot、路径约束、命令检查、git diff、benchmark
- `strategy`
  - 优先暴露隐含假设、替代解释、执行风险
- `operational`
  - 优先 dry-run、依赖检查、权限和回滚风险

#### 6. `revise`

对每条被 challenge 或 verification 打到的 claim，proposal 侧必须显式做一种处理：

- revise
- downgrade confidence
- withdraw
- mark unresolved
- unchanged

如果 proposer 没给出 revision，系统会走 builtin fallback revision。

#### 7. `adjudicate`

裁决在 claim 级完成，而不是 essay 级。

每条 claim 会得到：

- `keep`
- `keep_with_caveat`
- `unresolved`
- `reject`

同时保留：

- `ClaimVerdict`
- `AdjudicationRecord`
- evidence refs
- blocking risks

#### 8. `report`

最终报告明确分开：

- retained claims
- downgraded claims
- unresolved questions
- next actions

#### 9. `action`

默认只允许低风险且可审计的 action。

风险门禁由 `action_policy.risk_gate` 控制：

- `low_only`
- `allow_medium`
- `allow_high`

#### 10. `observe`

记录 post-action 观察结果。

当前至少会写：

- `pending`
- `no_action`
- `follow_up_recommended`
- `held_up`
- `contradicted`

`observe` 支持真实外部观测源：`observe_policy.sources`。

### 受控循环

允许的主要转移：

- `propose -> challenge`
- `challenge -> verify`
- `verify -> revise`
- `revise -> challenge`
- `revise -> verify`
- `revise -> adjudicate`
- `adjudicate -> revise`
- `adjudicate -> ingest`
- `action -> observe`

### 停止条件与终态

系统不会无限循环。主要限制：

- `loop_policy.max_revision_rounds`
- `loop_policy.max_verification_rounds`
- `fallback_policy.max_fallback_rounds`
- `loop_policy.material_confidence_delta`
- `waiting_policy.global_deadline`

常见非成功终态：

- `insufficient_evidence`
- `unresolved_conflict`
- `requires_human_review`
- `action_blocked_by_risk`

## `free_debate`

### 阶段语义

#### 1. `frame`

先把问题规范化成 case 视角，避免 participant 围绕模糊题意展开。

#### 2. `ingest`

把输入材料、外部源和上下文整理成可共享的结构化输入。

#### 3. `initial`

每个 participant 独立提出初始 claim / position。

要求：

- 初始立场先独立生成
- 不直接暴露其他 participant 身份
- 尽量保留 claim provenance

#### 4. `debate`

每轮 participant 会收到：

- 匿名化/去重后的 peer claims
- 上轮摘要
- 当前 active claims

每轮输出通常包括：

- judgement
- revised claim
- merge suggestion
- disagreement reason

#### 5. `final_vote`

所有 active claims 进入最终投票。

是否纳入最终结论，取决于：

- `debate_policy.vote_threshold`
- 是否达到最少轮数
- 是否满足 early stop 条件

#### 6. `report`

输出：

- 辩论过程摘要
- 最终被接受的 claims
- 主要分歧点
- 未达成共识的部分

#### 7. `action`

仍然遵守 action 风险门禁；如果风险超限，不会偷偷执行。

#### 8. `observe`

和其他 workflow 一样，可以记录 post-action observation，并触发 reopen / follow-up。

### 循环与停止条件

主要由 `debate_policy` 控制：

- `min_rounds`
- `max_rounds`
- `vote_threshold`
- `enable_early_stop`
- `peer_context_mode`

early stop 的典型条件是：

- 已达到 `min_rounds`
- 没有新 claim
- 没有 merge suggestion
- judgement 全部为 `agree` 或 `no_change`

### 结果重点

最终主要看：

- `rounds`
- `claims`
- `claimResolutions`
- `votes`
- `outcome`

常见 outcome：

- `consensus`
- `partial_consensus`
- `no_consensus`

## `delphi`

### 阶段语义

#### 1. `frame`

先收敛出规范化问题定义，避免 participant 对题目理解漂移过大。

#### 2. `ingest`

将材料、约束和外部信息整理成匿名问卷的统一输入。

#### 3. `delphi_questionnaire`

每个 participant 独立回答同一问题，通常会给出：

- 候选 statement
- rating
- reason

这一轮不会暴露 participant 身份或原文归属。

#### 4. `delphi_summary`

coordinator 或 facilitator 生成匿名聚合摘要。

典型内容：

- 候选结论
- 评分分布
- 主要理由
- 主要分歧

#### 5. `delphi_revision`

participant 基于匿名汇总再次修订：

- rating
- reason
- 对候选 statement 的支持或反对

#### 6. 收敛判断

每一轮 revision 后，系统会判断：

- 是否达到 `min_rounds`
- 是否达到 `convergence_threshold`
- 如果未收敛，是否继续到下一轮

#### 7. `report`

输出：

- recommendation
- consensus level
- dissent summary
- 仍未解决的问题

#### 8. `action`

仍受 action 风险门禁约束。

#### 9. `observe`

和其他 workflow 一样，可以记录后验证据和 follow-up。

### 循环与停止条件

主要由 `delphi_policy` 控制：

- `min_rounds`
- `max_rounds`
- `convergence_threshold`
- `rating_scale_min`
- `rating_scale_max`
- `anonymous`
- `facilitator_summary_style`

停止条件通常是：

- 至少完成 `min_rounds`
- 若收敛度达到阈值，提前停止
- 否则运行到 `max_rounds`

### 结果重点

最终主要看：

- `rounds`
- `statements`
- `ratingDistributions`
- `consensusLevel`
- `recommendation`
- `dissentSummary`

## `view` 如何展示三种 workflow

`view` 会按 `mode` 自动决定优先渲染哪些 section。

- `adjudication`
  - `claims`
  - `challenges`
  - `verifications`
  - `observations`
  - `followups`
- `free_debate`
  - `rounds`
  - `votes`
  - `observations`
  - `followups`
- `delphi`
  - `rounds`
  - `statements`
  - `convergence`
  - `observations`
  - `followups`

## 设计取舍

- 没有用“多数 proposer 赞同就算真”作为真值机制。
- verifier 优先 deterministic，LLM verifier 只作为补强。
- worker 输出可以是 prose，但 workflow 传递与裁决必须落到结构化 artifact 上。
- 保留多工作流，但默认 workflow 仍然最强调证据链、claim traceability 和可复盘性。
