# 输出产物

每次运行都会写出一个可审计目录。默认目录由配置里的 `output.directory` 决定，常见形式是：

```text
./out/{requestId}
```

相对路径按当前执行目录解析，不按配置文件所在目录解析。

默认新建的运行目录和 session store 使用 `0700`，session、provider
input/raw output、ledger、event 和结果文件使用 `0600`。所有运行产物写入都会
拒绝最终目标 symlink，避免将私有任务材料写到声明目录之外。

## 目录结构

典型输出：

```text
out/tc_xxx/
  result.json
  summary.md
  ledger.jsonl
  ledger.root.json
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
- `degradations`
- `error`

三种 mode 都通过同一条收尾链路写入 `caseManifest`、`terminalState`、`action` 和 `observations`，并在 `observe` 完成后保存最终 checkpoint。配置了 action 时一律先经过 `action_policy.risk_gate`；未显式配置 gate 时按 `low_only` 处理，只有实际完成的 action 才会设置 `executed: true`。`action_policy.include_result` 省略时默认为 `true`；显式设为 `false` 时，派发给 actor 的 task 不包含完整 `RunResult`。

mode-specific 结论会映射到统一 terminal state：`free_debate` 的 consensus 为 `completed`，partial/no consensus 为 `unresolved_conflict`，quorum 不足或无 ballot 为 `insufficient_evidence`；`delphi` 达到收敛阈值为 `completed`，未收敛为 `unresolved_conflict`，无有效 statement/recommendation 为 `insufficient_evidence`。risk gate 阻止 action 时统一覆盖为 `action_blocked_by_risk`，observe 发现矛盾并触发 reopen 时统一覆盖为 `requires_human_review`。

`degradations` 是结果级的降级清单：run 没有失败、但结论的产出基础发生了变化时，每条记录说明哪个环节（`phase`）、哪个 agent、原因（`reason`）以及对结论的影响（`impact`）。`kind` 取值：

- `participant_absent`：某阶段某个 agent 的任务重试耗尽后仍失败，该阶段基于其余 agent 的输出继续。
- `step_skipped`：可选步骤（semantic dedup、reporter）失败后被跳过或回退到内置实现。
- `quorum_not_met`：free_debate 至少一条 active claim 的唯一投票者数低于 `debate_policy.vote_quorum`。

原始错误仍在 ledger 和 `artifacts/failure-*` 中；`degradations` 只做人可读的汇总。`summary.md` 会在头部渲染同名 `## Degradations` 段落，free_debate 还会追加 `- voters: N/M (absent: ...)` 行。

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
- debate rounds。`rounds[].participantOutputs[].processNotes` 保存被分类为 `category=process` 的元观察（或关键词兜底命中的），它们不进入投票。
- final votes，包括每票的 `confidence` 连续支持分数。
- `claimResolutions[]`：每条 claim 的判定记录。`supportScore` 是 accept/reject 票的聚合支持分数（即 summary 中的 `support=`）；abstain 不进入该分数、均值或方差，只保留在 `abstainingVoters` 和总 `voteCount` 中。`supportRatio` 仅作诊断；`incoherentVotes` 计数被剔除的标签-分数矛盾票或重复票。
- 启用合成阶段时，`claims[]` 中 `category=synthesis` 且 active 的那条是 canonical 综合推荐（owner 为 synthesizer），summary 的 Final Vote 里单列 `### Synthesis` 组并标注 ratified / not ratified；`rounds[]` 中 `phase=amend` 的记录保存各参与者对草案的修正评审。
- consensus / minority positions。

`summary.md` 的布局为结论优先：头部元信息（含 `voters: N/M` 与 `accepted claims: X/Y ballot (Z%)`）→ Degradations → Conclusion → Retained/Downgraded Claims（`id — statement` 形式）→ Final Vote 明细。Final Vote 按 `Accepted / Not Accepted / No Votes / Merged` 分组，各组内按 `supportScore` 降序；被合并的 claim 折叠为一行 `id → merged into target`，不再与被否决的 claim 混淆。`freeDebate.ballotSize` 记录进入投票的 active claim 数量——通过率 100% 且 ballot 较大时，通常意味着 ballot 冗余或投票缺乏区分度，值得回看 claim 预算与去重配置。

`freeDebate.claimResolutions[]` 还保留一组诊断字段：`confidenceMean`、`confidenceVariance`、`confidenceStdDev` 只基于 accept/reject 票，用于判断支持强度与分歧；接受判定只看 `supportScore >= support_threshold`，同时还必须满足该 claim 的独立 quorum。

`delphi` 常见重点：

- questionnaire rounds。
- anonymous aggregate summaries。
- score distribution。
- convergence result。

`delphi.rounds[]` 还会输出匿名计数 `responderCount`、`absentResponderCount`，以及 `responseQuorumMet`、`minResponsesMet` 和 `stableRounds`；顶层 `delphi` 输出最终 gate 状态及 `converged`。因此 `consensusLevel` 达到阈值本身不再代表流程已经收敛：样本 quorum、每个 statement 的最小响应数和连续稳定轮数必须同时满足。参与者身份不会写入 Delphi response JSON。

## `summary.md`

面向人快速阅读的摘要。通常包含：

- 任务和 mode。
- 主要结论。
- 关键 caveat / unresolved。
- claim、投票或 Delphi 收敛摘要。
- 重要 artifact 路径。

## `ledger.jsonl`

证据账本的唯一 WAL。每条 entry 都包含：

- `prevHash`：上一条 entry 的 SHA-256；首条为空。
- `entryHash`：当前 entry（不含自身 `entryHash`）的 SHA-256。

`ledger.root.json` 保存当前 `entryCount` 和最终 `rootHash`。启动追加和 viewer
读取时都会验证完整 hash chain、连续 `seq` 和 root checkpoint；删除、重排或修改
完整 entry 会导致验证失败。旧版无 hash ledger 会在下一次追加时原子迁移。

常见 entry：

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
每次追加都会 `fsync` WAL，再原子更新并同步 root 和 manifest。若进程在 WAL 已提交、
派生文件更新前崩溃，下一次打开会以 ledger 为事实源重建 manifest；不完整的末尾
写入会被截断恢复。基于规范化 ledger 路径的用户级文件锁用于协调多个进程或多个
`LedgerWriter`，锁文件不会混入运行产物目录。

hash chain 和 root checkpoint 提供 tamper-evident 检测，不等同于数字签名：拥有整个
运行目录写权限的攻击者仍可能同时重写 ledger 和 root。需要对抗这种攻击时，应将
最终 `rootHash` 复制到外部不可变存储或签名系统中。

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
  - 从 ledger WAL 派生的 artifact 反向索引；每行包含对应的 `ledgerEntryHash`。
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
