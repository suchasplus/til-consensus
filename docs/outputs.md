# 输出产物

每次运行都会写出一个可审计目录。默认目录由配置里的 `output.directory` 决定，常见形式是：

```text
./out/{requestId}
```

相对路径按当前执行目录解析，不按配置文件所在目录解析。

## 目录结构

典型输出：

```text
out/tc_xxx/
  result.json
  summary.md
  ledger.jsonl
  events.jsonl
  artifacts/
    manifest.jsonl
    input-*.json
    raw-*.json
    provider-readiness.json
    strict-compliance-summary.json
    run-telemetry.json
```

## `result.json`

统一结果壳，所有 mode 都包含：

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

然后按 mode 挂载一个 mode-specific section：

- `adjudication`
- `freeDebate`
- `delphi`

`adjudication` 常见重点：

- `claimGraph`
- `challengeTickets`
- `verificationResults`
- `revisionRecords`
- `adjudicationRecords`
- `observations`

`freeDebate` 常见重点：

- initial claims。
- debate rounds。
- final votes。
- consensus / minority positions。

`delphi` 常见重点：

- questionnaire rounds。
- anonymous aggregate summaries。
- score distribution。
- convergence result。

## `summary.md`

面向人快速阅读的摘要。通常包含：

- 任务和 mode。
- 主要结论。
- 关键 caveat / unresolved。
- claim、投票或 Delphi 收敛摘要。
- 重要 artifact 路径。

## `ledger.jsonl`

append-only 证据账本。常见 entry：

- `case_framed`
- `task_ingested`
- `worker_output`
- `claim_proposed`
- `challenge_opened`
- `semantic_verification`
- `claim_revised`
- `arbiter_decision`
- `report_generated`
- `observation_recorded`

账本用于把 claim、challenge、verification、report、action、observe 串成可追溯链路。

## `events.jsonl`

运行期事件流。主要用于：

- `--verbose` / `--debug` 输出回放。
- Web viewer 展示 phase 和任务级日志。
- 排查 provider 调用、schema repair、fallback 等行为。

## `artifacts/`

关键 artifact：

- `input-*.json`
  - provider 实际收到的结构化任务输入。
- `raw-*.json`
  - provider 原始输出或解析后的原始消息。
- `failure-*.json`
  - provider 或 schema 失败时的分类结果。
- `manifest.jsonl`
  - artifact 与 ledger entry 的反向索引。
- `provider-readiness.json`
  - `profile preflight` 的真实 provider 可用性结果。
- `strict-compliance-summary.json`
  - provider × task 的 strict compliance 汇总。
- `run-telemetry.json`
  - 单次运行 telemetry。

Raw artifact 不会被后续同名 semantic/raw 输出覆盖，便于回溯每一轮实际输入输出。

## Provider readiness

`artifacts/provider-readiness.json` 记录：

- provider id。
- provider type / protocol。
- model。
- base url。
- api key env。
- CLI/API command 摘要。
- ready 状态。
- strict JSON / recoverable JSON 状态。
- duration。
- error。

可以通过下面命令生成：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
```

## Strict compliance telemetry

strict compliance 用来衡量 provider 在不经过 schema repair / normalize 的情况下，是否直接输出合规结构。

重点维度：

- provider。
- provider model。
- task kind。
- strict decode 是否成功。
- repair 是否触发。
- normalize 是否触发。
- raw error artifact。

这部分用于判断“模型真的遵守 schema”还是“被系统兜底修好了”。

## Debug 信息

`view --debug` 和 Web viewer 的 Debug / Telemetry 区块会显式展示：

- provider 输入 artifact。
- raw 输出 artifact。
- schema error / repair 状态。
- raw verdict / raw task verdict / raw disposition。
- payload JSON。
- provider readiness。
- strict compliance。

## Session store

session snapshot 会写到输出目录同级的 `_sessions/`：

```text
out/_sessions/session_xxx.json
```

相关命令：

```bash
til-consensus session list --config ./til-consensus.yaml --request-id tc_xxx
til-consensus session show --config ./til-consensus.yaml --session-id session_xxx
```
