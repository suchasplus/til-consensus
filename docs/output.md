# 输出产物说明

每次运行默认写到 `./out/{requestId}/`。

## `result.json`

这是最终结果，适合程序消费。

顶层统一字段：

- `schemaVersion`
- `mode`
- `requestId`
- `sessionId`
- `lineage`
- `taskSpec`
- `caseManifest`
- `terminalState`
- `report`
- `action`
- `observations`
- `metrics`
- `error`

然后按 `mode` 挂一个 section：

- `adjudication`
  - `claimGraph`
  - `challengeTickets`
  - `verificationResults`
  - `revisionRecords`
  - `adjudicationRecords`
  - `arbiterReport`
  - `taskVerdict`
- `freeDebate`
  - `rounds`
  - `claims`
  - `claimResolutions`
  - `votes`
  - `outcome`
- `delphi`
  - `rounds`
  - `statements`
  - `ratingDistributions`
  - `consensusLevel`
  - `recommendation`
  - `dissentSummary`

## `ledger.jsonl`

这是最重要的审计产物。

特点：

- append-only
- 每条都有 `seq`
- 每条都可以挂接 artifact
- 适合回看 worker 输出、verification 结果、投票或 Delphi 聚合过程

常见 `kind`：

- `case_framed`
- `task_ingested`
- `claim_proposed`
- `challenge_opened`
- `deterministic_check`
- `semantic_verification`
- `claim_revised`
- `arbiter_decision`
- `adjudication_fallback`
- `follow_up_case_created`
- `observation_recorded`
- `debate_round_opened`
- `debate_round_output`
- `debate_vote_cast`
- `delphi_round_opened`
- `delphi_response_recorded`
- `delphi_round_summary`
- `delphi_convergence_reached`

## `events.jsonl`

运行事件日志。

它主要服务于观察执行过程，不承担核心裁决语义。更关注审计时，优先看 `ledger.jsonl`。

## `summary.md`

给人快速阅读的摘要。

会根据 `mode` 渲染不同内容：

- `adjudication`
  - 任务 verdict、terminal state、关键 claims、主要验证结论、revision/observe 摘要
- `free_debate`
  - 轮次摘要、最终投票、共识结果
- `delphi`
  - 收敛水平、推荐结论、异议摘要

## `artifacts/`

原始产物目录。

常见内容：

- verifier 命令 stdout / stderr
- benchmark 输出
- diff 或 patch
- worker 原始返回
- parse 错误文本

其中 `artifacts/manifest.jsonl` 会把 artifact 反向索引回 `ledger.jsonl` 的 `entryId`。

和这次闭环增强最相关的两类记录是：

- `source_material`
  - 既可能来自静态 `task_spec.materials`
  - 也可能来自 `ingest_policy.sources`
  - fallback 补抓的证据会在 metadata 里带 `reason=fallback-N`
- `observation_recorded`
  - 既可能是 coordinator 的基础观察
  - 也可能来自 `observe_policy.sources`
  - 如果命中矛盾证据，会带 `reopen=true` 和 `followUpCaseId`
- `follow_up_case_created`
  - 只在 `observe_policy.on_contradiction=reopen` 时出现
  - artifact 会指向真实生成的 follow-up case JSON
  - metadata 会带 `followUpRequestId`

## session store

除了 run 目录里的产物，CLI 还会把 session snapshot 持久化到 `_sessions/` 目录。

单个 snapshot 至少包含：

- `sessionId`
- `requestId`
- `request`
- `phase`
- `checkpoint`
- `claimGraph`
- `challengeTickets`
- `result`

这使得 CLI 可以支持：

- `run --resume-session`
- `run --replay-session`
- `session list`
- `session show`

其中：

- `resume-session`
  - 读取 snapshot 里的 `checkpoint.lastCompletedPhase`
  - 对 `adjudication` 模式执行 phase 级恢复
- `replay-session`
  - 复用旧 request，但生成新的 child run
  - 更适合做“同一问题重新跑一次”而不是断点恢复

## provider 审计 artifact

当 provider 被调用时，`artifacts/` 目录下还会额外生成一组审计文件：

- `input-<agent>-<task>.json`
  - provider 实际收到的结构化任务输入
- `failure-<agent>-<task>.json`
  - provider 执行失败时的分类结果
  - 常见 `class`：
    - `timeout`
    - `auth`
    - `rate_limited`
    - `unavailable`
    - `network`
    - `command_not_found`
    - `command_exit`
- `raw-<agent>-<task>.*`
  - provider 的原始输出
  - 如果输出无法规范化，还会保留 parse error 原文
