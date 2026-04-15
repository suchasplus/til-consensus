# til-consensus 裁决式引擎

## 定位

`til-consensus` 不再做“多 agent 自由辩论直到收敛”的流程。

当前实现采用固定状态机：

1. `ingest`
2. `propose`
3. `challenge`
4. `verify`
5. `adjudicate`
6. `report`
7. `action`

系统目标不是“必须共识”，而是“基于证据账本给出高置信度裁决”，并且允许 `undetermined`。

## 核心对象

- `TaskSpec`
  - `goal`
  - `materials`
  - `constraints`
  - `successCriteria`
  - `allowedTools`
  - `workspaceSnapshot`
- `ClaimNode`
  - 原子 claim
  - 依赖关系
  - 证据引用
  - 裁决结果
- `ChallengeTicket`
  - 对 claim 的质疑
  - 请求的 verification checks
- `EvidenceRecord`
  - append-only ledger 条目
  - 指向原始 artifact
- `VerificationResult`
  - deterministic 或 semantic verifier 的输出
- `ArbiterReport`
  - claim 级裁决
  - task 级裁决

## 输出产物

每次运行默认写到 `./out/{requestId}/`：

- `result.json`
  - 顶层 schema 为 `schemaVersion: 1`
  - 包含 `taskSpec`、`taskVerdict`、`claimGraph`、`challengeTickets`、`arbiterReport`、`report`
- `ledger.jsonl`
  - 核心证据账本
  - append-only
  - `seq` 单调递增
- `events.jsonl`
  - 运行事件日志
  - 只用于观察执行过程
- `summary.md`
  - 人可读摘要
- `artifacts/`
  - 原始 worker 输出
  - verifier 命令日志
  - parse 错误文本
- `artifacts/manifest.jsonl`
  - artifact 反向索引
  - 每条记录都指回对应的 ledger `entryId`

## Claim 裁决语义

claim verdict：

- `supported`
- `refuted`
- `insufficient_evidence`
- `undetermined`

task verdict：

- `supported`
- `partially_supported`
- `undetermined`
- `failed`

## CLI

当前命令：

- `til-consensus run`
- `til-consensus view`
- `til-consensus act`
- `til-consensus config init`
- `til-consensus config validate`
- `til-consensus config add-provider`
- `til-consensus config add-agent`

### `run` 示例

```bash
til-consensus run \
  --task "判断这个 patch 是否真正修复了 race condition" \
  --proposers proposer-a \
  --challengers challenger-a \
  --arbiter arbiter-a \
  --reporter reporter-a \
  --semantic-verifier verifier-a \
  --success-criteria "只按 claim 级裁决" \
  --success-criteria "证据不足时输出 undetermined" \
  --workspace-snapshot .
```

### `view` 示例

```bash
til-consensus view --request-id tc_1710000000000_abcdef
```

### `act` 示例

```bash
til-consensus act \
  --result ./out/tc_1710000000000_abcdef/result.json \
  --task "基于裁决结果生成下一步修复计划"
```

## 配置文件示例

```yaml
schema_version: 1

defaults:
  success_criteria:
    - 给出 claim 级裁决
    - 证据不足时允许 undetermined
  allowed_tools:
    - repo
    - tests
    - benchmarks
  per_task_timeout: 20m
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 3
    dedupe_strategy: normalized-statement
  verification_policy:
    allow_semantic_verifier: true
    max_parallel_checks: 4
    required_checks:
      - name: workspace
        kind: workspace_snapshot
      - name: allowed-paths
        kind: allowed_paths
      - name: unit-tests
        kind: command
        command: go
        args: ["test", "./..."]
  arbiter_policy:
    allow_undetermined: true
    blind_review: true

output:
  directory: ./out/{requestId}

providers:
  mock:
    type: mock
    behavior: deterministic
    models:
      default:
        provider_model: mock-default

agents:
  - id: proposer-a
    provider: mock
    model: default
    role: proposer
  - id: challenger-a
    provider: mock
    model: default
    role: challenger
  - id: arbiter-a
    provider: mock
    model: default
    role: arbiter
  - id: reporter-a
    provider: mock
    model: default
    role: reporter

roles:
  proposers: [proposer-a]
  challengers: [challenger-a]
  arbiter: arbiter-a
  reporter: reporter-a
```

## Verifier 说明

当前 verifier 框架支持两类执行器：

- deterministic verifier
  - `workspace_snapshot`
  - `allowed_paths`
  - `command`
  - `git_diff_paths`
  - `benchmark_threshold`
- semantic verifier
  - 通过独立 agent 对 claim 做语义验证
  - 输出仍然落到 ledger

其中 `command` check 会自动注入这些环境变量：

- `TIL_CONSENSUS_REQUEST_ID`
- `TIL_CONSENSUS_SESSION_ID`
- `TIL_CONSENSUS_CLAIM_ID`
- `TIL_CONSENSUS_WORKSPACE_ROOT`

`benchmark_threshold` 支持：

- `pattern`
  - 从 stdout/stderr artifact 中提取数值
- `threshold`
- `threshold_mode`
  - `max`
  - `min`

`git_diff_paths` 会用 `base_revision` 或 `workspace_snapshot.revision` 作为基线，对当前工作区执行 `git diff --name-only`，并按允许路径做裁决。

## 样例

仓库内置了可直接复用的样例输入，位于：

- [patch-fix](/Users/suchasplus/agentic/til-consensus/testdata/scenarios/patch-fix/run.yaml)
- [benchmark-claim](/Users/suchasplus/agentic/til-consensus/testdata/scenarios/benchmark-claim/run.yaml)
- [architecture-claim](/Users/suchasplus/agentic/til-consensus/testdata/scenarios/architecture-claim/run.yaml)

这些样例会被单元测试直接加载，用来约束新 schema 和计划解析逻辑。

## 当前边界

- 不保留任何旧 debate/round/final-vote 兼容层
- 不提供 E2E
- 不提供公共 SDK
- `view` 只读取当前 adjudication schema
