# 输出产物说明

每次运行默认写到当前执行目录下的 `./out/{requestId}/`。相对 `output.directory` 按当前执行目录解析，而不是按配置文件所在目录解析。

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

## provider readiness 快速入口

`profile preflight` 和真实 E2E 会写出这个文件，用来记录 provider 是否能在当前环境完成最小非交互 JSON 调用。

`profile preflight` 的探测默认使用 `max_output_tokens=2048`；如果 model 配置了更小的 `max_output_tokens`，则使用配置值。这个预算只影响 readiness 探测，不代表正式 workflow 的输出预算。

运行时的完整字段说明见本文后面的 [provider readiness telemetry](#provider-readiness-telemetry)。终端执行 `profile preflight` 时会逐个 provider 分块输出；`artifacts/provider-readiness.json` 仍然会在全部探测结束后写出，供 `view` 和 `telemetry daily` 读取。

Gemini API 的无文本错误会尽量带上 `finishReason`、`promptBlockReason` 和 token usage，例如 `thoughtsTokenCount`，用于区分安全拦截、输出截断和 thinking 预算消耗。

这个文件会被 `view --section debug --verbose`、`view --web` 和 `telemetry daily` 读取。

## `events.jsonl`

运行事件日志。

它主要服务于观察执行过程，不承担核心裁决语义。更关注审计时，优先看 `ledger.jsonl`。

常见事件类型：

- `session_started`
- `phase_changed`
- `task_dispatched`
- `task_retrying`
- `task_completed`
- `task_failed`
- `ledger_appended`
- `claim_revised`
- `claim_adjudicated`
- `observation_added`
- `session_finalized`
- `session_failed`

如果事件 payload 的 metadata 里带有模型原始语义，当前也会保留：

- `rawVerdict`
- `rawTaskVerdict`

这两个字段会在 `view --section debug` 和 `view --web` 的 Debug 区块里直接显示。

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

终端里可以直接用 artifact 快捷命令查看，不必手动翻目录：

```bash
til-consensus artifact list --result ./out/tc_xxx/result.json
til-consensus artifact list --result ./out/tc_xxx/result.json --type error
til-consensus artifact show --result ./out/tc_xxx/result.json --id 1
til-consensus artifact show --result ./out/tc_xxx/result.json --path artifacts/run-telemetry.json
til-consensus artifact show --result ./out/tc_xxx/result.json --type raw --latest
```

高频调试也可以用更短的 `logs`：

```bash
til-consensus logs tc_xxx --config ./til-consensus.yaml --type raw
til-consensus logs --result ./out/tc_xxx/result.json --type error
til-consensus logs --result ./out/tc_xxx/result.json --latest --type raw
```

`artifact show` 默认只允许读取当前 run 目录内的路径，避免误读任意本地文件；确实需要读取外部路径时必须显式传 `--allow-outside-run-dir`。

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

- `input-<agent>-<task>-<taskID>.json`
  - provider 实际收到的结构化任务输入
- `failure-<agent>-<task>-<taskID>.json`
  - provider 执行失败时的分类结果
  - 常见 `class`：
    - `timeout`
    - `auth`
    - `rate_limited`
    - `unavailable`
    - `network`
    - `command_not_found`
    - `command_exit`
- `raw-<agent>-<task>-<taskID>.*`
  - provider 的原始输出
  - 如果输出无法规范化，还会保留 parse error 原文
- `decode-error-<agent>-<task>-<taskID>.txt`
  - strict decode 或 normalize 失败时的原始错误文本
- `repair-request-<agent>-<task>-<taskID>.json`
  - 同源 provider repair retry 的请求记录
- `repair-report-<agent>-<task>-<taskID>.json`
  - repair 的输入、错误和结果摘要
- `compliance-report-<agent>-<task>-<taskID>.json`
  - 单个 task 的 strict compliance 报告
- `strict-compliance-summary.json`
  - 当前 run 的 compliance 汇总
- `provider-readiness.json`
  - 真实 provider 预检结果
- `run-telemetry.json`
  - 当前 run 的 workflow 级聚合统计

之所以带上 `<taskID>`，是为了避免同一个 agent / task kind 的多次执行互相覆盖，方便事后逐轮排查。

## strict compliance telemetry

strict compliance telemetry 用来回答一个更具体的问题：

- 某个 provider 的某类 task，有多少次是第一次就严格符合 schema
- 又有多少次需要 normalize 或 repair 才能通过

### `compliance-report-*.json`

每个 task 一份，典型字段包括：

- `provider`
- `providerType`
- `providerModel`
- `agentID`
- `taskKind`
- `taskID`
- `strict`
- `normalized`
- `repairAttempted`
- `finalStatus`
- `strictError`
- `finalError`
- `rawArtifact`
- `initialErrorArtifact`
- `finalArtifact`

`finalStatus` 目前固定是四种之一：

- `strict`
  - 原始输出直接通过 strict decode 和 task 校验
- `normalized`
  - 只靠无歧义的类型转换后通过
  - 例如 `"0.8" -> 0.8`
- `repaired`
  - strict/normalize 失败后，经过同源 provider repair retry 修复成功
- `failed`
  - strict、normalize、repair 都失败

### `strict-compliance-summary.json`

按下面维度聚合：

- `provider`
- `providerType`
- `providerModel`
- `taskKind`

## provider readiness telemetry

### `provider-readiness.json`

这份文件位于：

```text
artifacts/provider-readiness.json
```

它主要回答：

- 当前测试 / 运行上下文里，`claude / gemini / antigravity / codex` 这类 provider 到底是不是 ready
- 失败是 provider 自身不可用，还是 workflow 本身出问题

典型字段：

- `provider`
- `providerType`
- `protocol`
- `model`
- `baseUrl`
- `apiKeyEnv`
- `agent`
- `command`
- `ready`
- `strictJSON`
- `recoverableJSON`
- `durationMs`
- `stdoutPreview`
- `stderrPreview`
- `error`

`profile preflight` 的终端输出是逐 provider 分块打印的：每个 provider 完成后立即显示 readiness，最后再打印 `profile preflight completed ready=x/y` 和 artifact 路径。stdout 是真实终端时，全部 ready 的最终摘要会显示为绿色；只要有一个 provider 不 ready，最终摘要会显示为红色。

## run telemetry

### `run-telemetry.json`

这份文件把单 task 的 compliance 汇总成 run 级信号，主要回答：

- 这次 run 的主结果是什么
- 还有多少 `unresolved`
- `keep_with_caveat` 有多少
- 哪个 task kind 在拖后腿

典型字段：

- `requestId`
- `sessionId`
- `mode`
- `providers`
- `taskSummary`
- `workflowSummary`
- `verificationSummary`
- `result`
- `timing`

其中：

- `taskSummary`
  - 按 `taskKind` 聚合 `total / strict / normalized / repaired / failed`
- `workflowSummary`
  - 聚合 `claims / keep / keep_with_caveat / unresolved / reject`
- `result`
  - 聚合 `primaryResult / taskVerdict / terminalState`

## daily markdown 聚合

如果你想把最近一段时间的 telemetry 聚合成 markdown：

```bash
til-consensus telemetry daily --config ./til-consensus.yaml
```

也可以直接指定扫描根目录和输出路径：

```bash
til-consensus telemetry daily \
  --root ./logs/out \
  --since 24h \
  --output ./reports/daily-telemetry.md
```

统计字段：

- `total`
- `strict`
- `normalized`
- `repaired`
- `failed`

这些汇总会直接出现在：

- `til-consensus view --section debug`
- `til-consensus view --web --section debug`

也就是说，排查 provider 漂移时，不需要只靠翻 raw artifact；终端和 Web 都能直接看到本次 run 的 compliance 分布和每个 task 的最终状态。

## 终端运行日志

`run` 和 `followup run` 的实时输出支持：

- 默认
  - phase 变化
  - task dispatched / retrying / failed
- `--verbose`
  - task completed
  - phase completed
  - claim revised
  - claim adjudicated
  - observation recorded
- `--debug`
  - 完整事件 payload
  - provider artifact 路径提示

如果 stdout/stderr 连接到真实终端，关键字会自动着色；如果输出被重定向到文件，则不会插入 ANSI 码。

环境变量：

- `NO_COLOR=1`
  - 关闭彩色输出
- `FORCE_COLOR=1`
  - 强制开启彩色输出
