# 配置

这篇说明配置文件的结构、include/overlay、profile、roles 和常用校验方式。Provider 细节见 [Provider 配置与预检](providers.md)，日常命令见 [操作手册](operations.md)。

## 查找顺序

`til-consensus` 默认配置文件名是 `til-consensus.yaml`。查找顺序：

1. `--config`
2. 当前目录下的 `./til-consensus.yaml`
3. `~/.config/til-consensus/default.yaml`
4. `~/.config/til-consensus/config.yaml`

如果设置了 `XDG_CONFIG_HOME`，用户配置会落到 `$XDG_CONFIG_HOME/til-consensus/...`。

## 推荐结构

推荐使用 split config：

```yaml
schema_version: 1
include:
  - ./conf/providers.yaml
  - ./conf/agents-adjudication.yaml
  - ./conf/roles-adjudication.yaml

profile: adjudication

output:
  directory: ./out/{requestId}

profiles:
  adjudication:
    defaults:
      mode: adjudication
```

一个更适合手工维护的目录：

```text
conf/
  providers.yaml
  agents-adjudication.yaml
  agents-free-debate.yaml
  agents-delphi.yaml
  roles-adjudication.yaml
  roles-free-debate.yaml
  roles-delphi.yaml
  tc.yaml
```

建议：

- 稳定 provider/model 放 `providers.yaml`。
- 不同 mode 的 agent 和 roles 分开。
- 主配置只保留 `include`、`profile`、`profiles` 和 `output`。
- 每次任务变化的文本放 `--task-file` 或 `run.yaml`。

## Include / overlay 规则

```yaml
schema_version: 1
include:
  - ./partials/providers.yaml
  - ./partials/agents.yaml
  - ./partials/roles.yaml
```

规则：

- `include` 路径相对当前配置文件所在目录解析。
- include 可以继续 include 其他文件。
- 循环 include 会直接失败。
- 多个 include 按顺序合并，后面的 include 覆盖前面的 include。
- 主配置文件最后覆盖所有 include。
- map 字段深合并，例如 `providers`、`providers.<id>.models`、`headers`、`env`、`options`。
- `agents` 按 `id` 合并。
- slice 字段整体替换，例如 `roles.*.participants`、`success_criteria`、`required_checks`。
- 可关闭/可清零的 policy 字段区分“未指定”和显式零值：省略字段表示继承；写出 `false` 或 `0` 表示覆盖父配置。适用于 `allow_semantic_verifier`、arbiter 布尔项、`enable_early_stop`、`semantic_dedup.enabled`、`synthesis.enabled`、`synthesis.amendment_rounds`、`vote_quorum` 和 Delphi `anonymous`。
- 最终合并后的配置仍会执行 `Normalize` 和 `Validate`。

注意：`output.directory` 的相对路径按当前执行目录解析，不按配置文件所在目录解析。`include` 路径才按配置文件所在目录解析。

## Profile overlay

一份配置可以内置多套运行方式：

```yaml
profile: fast

profiles:
  fast:
    defaults:
      mode: adjudication
      per_task_timeout: 5m
    roles:
      adjudication:
        proposers: [proposer-fast]
        challengers: [challenger-fast]
        arbiter: arbiter-fast

  strong-delphi:
    defaults:
      mode: delphi
      per_task_timeout: 20m
    roles:
      delphi:
        participants: [participant-a, participant-b, participant-c]
        facilitator: facilitator-a
        reporter: reporter-a
```

规则：

- `profile:` 是默认 active profile。
- 运行时可用 `--profile <name>` 覆盖。
- `profile preflight` 为避免和 provider profile 混淆，使用 `--config-profile <name>`。
- profile overlay 支持覆盖 `defaults`、`output`、`providers`、`agents`、`roles`。
- overlay 会在最终 normalize/validate 前合并。

例如，下面的 profile 会明确关闭父配置中的自动步骤和 quorum；这些值不会被当成“未配置”丢弃：

```yaml
profiles:
  manual:
    defaults:
      debate_policy:
        vote_quorum: 0
        enable_early_stop: false
        semantic_dedup:
          enabled: false
        synthesis:
          enabled: false
          amendment_rounds: 0
      delphi_policy:
        anonymous: false
```

**mode 与 profile 错位时的 success_criteria**：如果运行时用 `--mode`（或 `debate`/`delphi` 快捷命令、run 输入文件）把 mode 改成了与 active profile 不同的模式，profile 继承下来的 `success_criteria` 不会被原样注入 prompt——另一个模式的协议词汇会误导参与者（典型案例：adjudication 的"必须标注 keep、revise、reject"会让 debate 参与者把"裁决：keep"写进 claim 正文）。此时 plan 会自动改用 `profiles` 里 mode 匹配的 profile 的 criteria；找不到匹配 profile 就用该 mode 的内置 criteria，并在 `run` 输出和 `--dry-run` 里打一行 `notice:` 说明替换来源。通过 CLI `--success-criteria` 或 run 输入文件显式给出的 criteria 永远原样保留、不做替换。

## 顶层字段

常用顶层字段：

- `schema_version`
- `include`
- `profile`
- `profiles`
- `defaults`
- `output`
- `providers`
- `agents`
- `roles`

## Defaults

通用字段：

- `mode`
- `task_type`
- `success_criteria`
- `allowed_tools`
- `per_task_timeout`
- `task_retry_attempts`
- `global_deadline`
- `loop_policy`
- `ingest_policy`
- `fallback_policy`
- `observe_policy`
- `workspace_snapshot`
- `task_constraints`

`adjudication` 常用 policy：

- `proposal_policy`
- `verification_policy`
- `arbiter_policy`

`free_debate` 常用 policy：

- `debate_policy.min_rounds`
- `debate_policy.max_rounds`
- `debate_policy.support_threshold`（旧名 `vote_threshold` 仍可用；两者同时设置时 `support_threshold` 优先）
- `debate_policy.vote_aggregation`（`median`（默认）或 `mean`）
- `debate_policy.vote_quorum`
- `debate_policy.max_new_claims_per_round`（每轮每参与者的新 claim 上限，默认 5，超出部分被丢弃）
- `debate_policy.max_active_claims`（active claim 总量上限，默认 30，达到后该轮禁止新增、只允许评判与合并）
- `debate_policy.enable_early_stop`
- `debate_policy.peer_context_mode`
- `debate_policy.semantic_dedup.enabled`
- `debate_policy.semantic_dedup.similarity_threshold`
- `debate_policy.semantic_dedup.cadence`（`per_round`（默认，每轮结束去重一次）或 `final`（只在投票前去重一次，即旧行为））
- `debate_policy.synthesis.enabled`（合成阶段开关，需同时配置 `roles.free_debate.synthesizer`）
- `debate_policy.synthesis.amendment_rounds`（合成稿的修正评审轮数，0–3，0 = 草案直接进投票）

启用 `semantic_dedup` 时，还必须配置 `roles.free_debate.semantic_deduper`。这个 agent 会走正常的 CLI/API provider 调用链路，并输出结构化的 claim merge 建议；系统不会使用本地文本相似度 fallback。如果希望强制外部 API 依赖，把 semantic deduper agent 绑定到 `type: api` provider 即可。

`free_debate` 的 final vote 使用连续 `confidence` 支持分数聚合：每个 voter 必须对每个 active claim 恰好投一票，返回 claim ID 集合与 ballot 不等价时整张 ballot 无效，且该参与者不计入 `voters`。accept/reject 的 `confidence`（0=强烈拒绝，1=强烈支持）由 `vote_aggregation` 聚合为 `supportScore`；abstain 的分数只作诊断，不进入 supportScore、均值或方差。标签与分数矛盾的票（accept<0.5 或 reject>0.5）会在 provider 边界被拒绝；计票层仍会剔除直接注入的矛盾票和同一参与者的重复票。`supportRatio` 仅作诊断展示。

`vote_quorum` 是逐 claim 的投票法定人数：每条 active claim 的唯一投票者数都必须达到 `ceil(vote_quorum × participants)`；未达标的 claim 强制不接受，整体 outcome 降级为 `quorum_not_met`（task verdict 为 `undetermined`），并在 degradation 和 ledger 中记录缺额。取值范围 `[0, 1]`，`0`（默认）关闭检查；生成的 free-debate 模板默认 `0.6`。

claim 预算控制冗余膨胀：`max_new_claims_per_round` 限制每个参与者每轮的新 claim 数量（超出部分被 coordinator 丢弃，round output 的 ledger metadata 记 `truncatedNewClaims`）；`max_active_claims` 是总量保险丝，达到后当轮所有参与者被告知 claim 预算已满、只能通过 judgements 修订与合并。revise 型 judgement 产生的修订 claim 不计入预算——合并与修订正是预算希望鼓励的行为。

final vote 还有一条校准约束：support-bearing（accept/reject）票达到 5 条时，如果一个 voter 给它们完全相同的 confidence，该输出会被判为无效并进入 repair。abstain 不参与这项分数校准。

`delphi` 常用 policy：

- `delphi_policy.min_rounds`
- `delphi_policy.max_rounds`
- `delphi_policy.convergence_threshold`
- `delphi_policy.rating_scale_min`
- `delphi_policy.rating_scale_max`
- `delphi_policy.response_quorum`（默认 `0.6`，每轮有效响应者比例）
- `delphi_policy.min_responses_per_statement`（默认 `2`，每个 active statement 的独立响应数）
- `delphi_policy.stability_rounds`（默认 `2`，连续满足样本量和离散度要求的轮数）
- `delphi_policy.anonymous`
- `delphi_policy.facilitator_summary_style`

Delphi 的 `rating` 是必填数值，且必须落在配置量表的闭区间内。空响应、缺失/越界评分以及同一参与者对同一 statement 的重复评分不会进入聚合，也不会计入 `response_quorum`。`consensus_level` 只描述评分离散度；最终收敛还要求本轮响应 quorum、每个 statement 的最小样本量全部达标，并对同一最高候选连续稳定至少两轮。单个响应不会产生可测量的 consensus。

## Providers / agents / roles

Provider 定义调用方式：

```yaml
providers:
  gemini-api:
    enabled: true
    type: api
    protocol: gemini-api
    base_url: https://generativelanguage.googleapis.com/v1beta
    api_key_env: GEMINI_API_KEY
    models:
      default:
        enabled: true
        provider_model: gemini-3.5-flash
```

CLI provider 默认只继承运行命令所需的基础环境，不会继承整个宿主环境。需要使用宿主凭据或自定义 endpoint 时必须精确声明：

```yaml
providers:
  pi-cli:
    type: cli
    cli_type: pi
    command: pi
    args: [--provider, google-vertex]
    env_passthrough: [GOOGLE_APPLICATION_CREDENTIALS, GOOGLE_CLOUD_PROJECT]
    env:
      GOOGLE_CLOUD_LOCATION: global
    models:
      default:
        provider_model: gemini-3.6-flash
```

`env_passthrough` 不支持 glob；profile overlay 未写该字段时继承父配置，显式 `[]` 表示清空。`provider.env` 继续用于显式设置或模板化变量，优先级高于 passthrough。`CLAUDECODE` 永久禁止注入 CLI 子进程。

Agent 引用 provider/model：

```yaml
agents:
  - id: arbiter-gemini
    provider: gemini-api
    model: default
    role: arbiter
```

Roles 决定 workflow 使用哪些 agent：

```yaml
roles:
  adjudication:
    proposers: [proposer-a]
    challengers: [challenger-a]
    semantic_verifier: verifier-a
    arbiter: arbiter-a
    reporter: reporter-a
```

Mode 对应 roles：

- `adjudication`
  - `proposers`
  - `challengers`
  - `semantic_verifier`
  - `arbiter`
  - `reporter`
  - `actor`
- `free_debate`
  - `participants`
  - `semantic_deduper`
  - `synthesizer`
  - `reporter`
  - `actor`
- `delphi`
  - `participants`
  - `facilitator`
  - `reporter`
  - `actor`

`enabled` 规则：

- provider/model 未写 `enabled` 时默认启用。
- agent 不能引用 `enabled: false` 的 provider/model。
- `profile preflight --all` 会跳过 disabled provider/model。

## 输出目录

```yaml
output:
  directory: ./out/{requestId}
```

`{requestId}` 会替换成本次 request id。相对路径按当前执行目录解析。自定义
`request_id` 必须是单一路径段，不能包含 `/`、`\\` 或 `..`；不合法的值会在
解析 run plan 时被拒绝，不会进入输出路径模板。

命令行可覆盖本次输出：

```bash
til-consensus profile preflight \
  --config ./conf/providers.yaml \
  --output ./tmp/provider-lab/out/{requestId}
```

## 输入方式

`run` 支持：

- `--task`
- `--task-file`
- `--input`
- `--followup`
- `--resume-session`
- `--replay-session`

常用：

```bash
til-consensus run --config ./til-consensus.yaml --task "判断这个 patch 是否修复 bug"
til-consensus run --config ./til-consensus.yaml --task-file ./task.md
til-consensus run --config ./til-consensus.yaml --input ./case.run.yaml
```

优先级：

1. `--task` 或 `--task-file`
2. `--input` 里的 `task_spec.goal`

## 校验和解释

完整校验：

```bash
til-consensus config validate --config ./til-consensus.yaml
```

查看最终配置：

```bash
til-consensus config render --config ./til-consensus.yaml
til-consensus config render --config ./til-consensus.yaml --format json
til-consensus config render --config ./til-consensus.yaml --profile delphi
```

只渲染 provider/profile 层：

```bash
til-consensus config render --config ./conf/providers.yaml --profiles-only
```

解释配置：

```bash
til-consensus config explain --config ./til-consensus.yaml
til-consensus config explain --config ./til-consensus.yaml --provider gemini-api
til-consensus config explain --config ./til-consensus.yaml --agent arbiter-gemini
```

真实连通性验证见 [Provider 配置与预检](providers.md#profile-preflight)。
