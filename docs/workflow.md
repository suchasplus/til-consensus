# 工作流与状态机

这份文档专门描述默认 `adjudication` workflow 的阶段、artifact、循环条件和终态。

## 宏观流程

固定宏观阶段是：

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

其中 `free_debate` 和 `delphi` 仍然保留自己的专用流程；这里讲的是默认裁决模式。

## 核心 artifact

### `CaseManifest`

用于把原始任务收敛为可审计的 case 定义，至少包含：

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

### `EvidenceLedger`

账本是 append-only 的，所有 claim、challenge、verification、report、action、observe 都会写入 `ledger.jsonl`。

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

### `Claim`

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

### `AttackRecord`

challenge 必须针对具体 claim。

关键字段：

- `attack_id`
- `target_claim_id`
- `attack_type`
- `severity`
- `attack_text`
- `suggested_falsification_method`

### `VerificationResult`

verification 结果会明确挂到 claim 上，而不是只停留在 worker prose。

关键字段：

- `verification_id`
- `target_claim_id`
- `method`
- `result`
- `raw_output_reference`
- `confidence_delta`
- `notes`

### `AdjudicationRecord`

最终 claim 级裁决。

关键字段：

- `target_claim_id`
- `disposition`
- `rationale`
- `final_confidence`
- `blocking_risks`
- `actionability`

## 阶段语义

### 1. `frame`

coordinator 先生成 `CaseManifest`，把任务标准化。

这个阶段不会让 proposer 直接自由发挥，避免下游所有 reasoning 都锚定在同一份模糊题意上。

### 2. `ingest`

把材料、上下文、工具输入规范化写入 `EvidenceLedger`。

目标不是“塞更多上下文”，而是把证据来源和 provenance 记录清楚。

除了静态的 `task_spec.materials`，现在还支持 `ingest_policy.sources`：

- coordinator 调用外部命令抓取或归一化证据
- stdout/stderr 会落到 `artifacts/`
- 结果写成 `source_material` ledger entry
- metadata 会记录 `reason=initial` 或 `reason=fallback-N`

这意味着 `adjudicate -> ingest` 已经不是“重新读旧材料”，而是可以真正补抓一轮新证据。

### 3. `propose`

proposer 生成 claim set。

要求：

- claim 尽量原子化
- 多 proposer 时先 blind proposal，再暴露他人输出
- 保留 proposer provenance

### 4. `challenge`

challenger 不能只挑战整篇答案，必须明确：

- target claim
- attack type
- severity
- falsification method

### 5. `verify`

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

### 6. `revise`

这是新加入的关键阶段。

对每条被 challenge 或 verification 打到的 claim，proposal 侧必须显式做一种处理：

- revise
- downgrade confidence
- withdraw
- mark unresolved
- unchanged

如果 proposer 没给出 revision，系统会走 builtin fallback revision。

### 7. `adjudicate`

裁决在 claim 级完成，而不是 essay 级。

每条 claim 会得到：

- `keep`
- `keep_with_caveat`
- `unresolved`
- `reject`

同时保留对应的：

- `ClaimVerdict`
- `AdjudicationRecord`
- evidence refs
- blocking risks

`adjudicate` 之后现在支持自动闭环回退。

系统会结合：

- `terminal_state`
- unresolved claims
- keep_with_caveat claims
- open challenges

和 `fallback_policy` 来决定：

- `adjudicate -> revise`
- `adjudicate -> ingest`

如果触发回退，ledger 会追加 `adjudication_fallback` 记录，说明：

- 回退原因
- 目标阶段
- 关联 claim
- 第几次 fallback

### 8. `report`

最终报告必须明确分开：

- retained claims
- downgraded claims
- unresolved questions
- next actions

### 9. `action`

默认只允许低风险且可审计的 action。

风险门禁由 `action_policy.risk_gate` 控制：

- `low_only`
- `allow_medium`
- `allow_high`

如果风险超过门禁，系统不会偷偷执行，而是进入 `action_blocked_by_risk`。

### 10. `observe`

记录 post-action 观察结果。

当前至少会写：

- `pending`
- `no_action`
- `follow_up_recommended`
- `held_up`
- `contradicted`

`observe` 现在可以接真实外部观测源：`observe_policy.sources`。

每个 source 都是一个外部命令，系统会：

- 执行命令
- 把 stdout/stderr 写到 artifact
- 根据 `success_pattern` / `failure_pattern` 解释结果
- 生成 `observation_recorded` ledger entry

如果观测结果与 retained claims 矛盾，且 `observe_policy.on_contradiction=reopen`：

- 该 observation 会标记 `reopen=true`
- 写入 `followUpCaseId`
- 生成真实的 follow-up case artifact
- 当前 run 的 `terminalState` 会升级为 `requires_human_review`

follow-up case 不是一个纯字符串 ID。当前实现会在 `artifacts/followups/` 下落一个可重跑的 follow-up case JSON，请求里会自动带上：

- parent request/session/case 信息
- 导致 reopen 的 observation summary
- 对应 observation artifact 路径
- 更新后的 goal 和 success criteria

## 受控循环

宏观阶段固定，但中间允许受控微循环。

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

## 停止条件

系统不会无限循环。当前主要限制：

- `loop_policy.max_revision_rounds`
- `loop_policy.max_verification_rounds`
- `fallback_policy.max_fallback_rounds`
- `loop_policy.material_confidence_delta`
- `waiting_policy.global_deadline`

典型停止条件：

- 已达到 revision 上限
- 没有材料性变更
- claim 已 withdraw
- 证据不足进入终态
- action 被风险门禁阻止

## 非成功终态

支持的非成功终态：

- `insufficient_evidence`
- `unresolved_conflict`
- `requires_human_review`
- `action_blocked_by_risk`

这些终态会同时写入：

- `result.json.terminalState`
- `adjudication.terminalState`
- `summary.md`
- `view` 的 overview / 风险视图

## 设计取舍

- 没有用“多数 proposer 赞同就算真”作为真值机制。
- verifier 优先 deterministic，LLM verifier 只作为补强。
- worker 输出可以是 prose，但 workflow 传递与裁决必须落到结构化 artifact 上。
- 保留多工作流，但默认 workflow 更强调证据链、claim traceability 和可复盘性。
