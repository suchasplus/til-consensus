# 三种讨论模式

`til-consensus` 支持三种 mode：`adjudication`、`free_debate`、`delphi`。选择 mode 的关键不是“哪个更高级”，而是你需要哪种讨论机制。

## 怎么选

| 场景 | 推荐 mode | 原因 |
| --- | --- | --- |
| 判断一个 claim、patch、benchmark 或设计结论是否成立 | `adjudication` | claim-centric，有 challenge、verify、revise、arbiter，适合需要审计的裁决 |
| 希望多个参与者充分碰撞观点，然后投票形成共识 | `free_debate` | participant 地位平等，多轮辩论、可选 rapporteur 合成后按支持分数 final vote |
| 希望减少权威偏见和从众效应，做匿名多轮收敛 | `delphi` | 匿名问卷、聚合摘要、修订评分，直到收敛或达到轮数上限 |

默认使用 `adjudication`。它最稳，也最适合做代码裁决、事实核查和架构决策复核。

不确定怎么选时，可以先运行分类器：

```bash
til-consensus classify "monorepo 和 polyrepo 如何取舍？" --config ./til-consensus.yaml
til-consensus classify --file ./case.md --config ./til-consensus.yaml
```

`classify` 默认使用 `gemini-api/default` 做一次轻量判断。它不会启动完整 workflow，也不会写 run artifact；输出会说明推荐 mode、置信度、理由、缺失信息和建议改写后的任务。如果问题缺少上下文或评价标准，它会返回 `needs_clarification`，并同时预估用户补齐信息后大概率适合的 mode 及理由；如果任务太简单、不需要多 agent 讨论，它会返回 `not_suitable`。

## `adjudication`

固定宏观阶段：

```text
frame -> ingest -> propose -> challenge -> verify -> revise -> adjudicate -> report -> action -> observe
```

核心角色：

- `proposers`：提出 claim。
- `challengers`：针对 claim 打开 challenge。
- `semantic_verifier`：做语义验证，输出 claim 级 verification。
- `arbiter`：做 claim 级最终裁决。
- `reporter`：生成面向人的报告。
- `actor`：可选，执行后续 action。

适合：

- patch 是否真正修复 bug。
- benchmark 或性能结论是否成立。
- 某个架构建议是否有足够证据。
- 需要保留 unresolved、caveat 和审计链路的判断。

典型配置片段：

```yaml
defaults:
  mode: adjudication
  success_criteria:
    - 给出 claim 级裁决
    - 明确 caveat、风险和 unresolved

roles:
  adjudication:
    proposers: [proposer-a]
    challengers: [challenger-a]
    semantic_verifier: verifier-a
    arbiter: arbiter-a
    reporter: reporter-a
```

## `free_debate`

固定宏观阶段：

```text
frame -> ingest -> initial -> debate* -> [synthesis -> amend*] -> final_vote -> report -> action -> observe
```

`[synthesis -> amend*]` 仅在启用 `debate_policy.synthesis` 时插入，见下文合成阶段。

核心角色：

- `participants`：平等参与者，先独立提出观点，再互相审阅和辩论。
- `semantic_deduper`：可选；启用 `semantic_dedup` 时必须配置，产出跨参与者的 claim 合并建议。
- `synthesizer`：可选；启用 `debate_policy.synthesis` 时必须配置，作为 rapporteur 起草唯一的综合推荐。
- `reporter`：汇总最终共识、分歧和投票结果。
- `actor`：可选，执行后续 action。

适合：

- 多个模型或多个立场之间做观点碰撞。
- 希望看到“为什么不同意”，而不是只看最终答案。
- 团队策略、架构方案、产品方向的开放式讨论。

典型配置片段：

```yaml
defaults:
  mode: free_debate
  success_criteria:
    - 形成最终可投票的 active claims
    - 保留主要分歧和少数派意见
  debate_policy:
    min_rounds: 2
    max_rounds: 3
    support_threshold: 0.67
    vote_aggregation: median
    vote_quorum: 0.6
    max_new_claims_per_round: 5
    max_active_claims: 30
    enable_early_stop: true
    semantic_dedup:
      enabled: true
      similarity_threshold: 0.85
      cadence: per_round
    synthesis:
      enabled: true
      amendment_rounds: 1

roles:
  free_debate:
    participants: [participant-a, participant-b, participant-c]
    semantic_deduper: deduper-a
    synthesizer: synthesizer-a
    reporter: reporter-a
```

`semantic_dedup` 是外部语义去重环节。启用后必须配置 `roles.free_debate.semantic_deduper`，该 agent 可以绑定任意 CLI/API provider。deduper 只输出 `sourceClaimId -> targetClaimId` 合并建议和 `similarity`，低于阈值的合并会被拒绝。合并后的 canonical claim 会保留 `proposedBy` 和 `mergedClaimIds`，例如“此 claim 由 Agent A 和 Agent C 独立提出”。默认 `cadence: per_round`——每轮辩论结束就去重一次，下一轮参与者看到的是合并后的 canonical 集合，冗余不会滚雪球，单轮去重失败也只损失一轮的合并（记入 degradations）；`cadence: final` 恢复旧行为（只在投票前去重一次）。

冗余的另一半来自数量失控：`max_new_claims_per_round`（默认 5）限制每轮每参与者的新 claim，超出即丢弃；`max_active_claims`（默认 30）达到后当轮完全禁止新增。两者配合把投票 ballot 控制在可辨析的规模——ballot 上全是彼此改写的句子时，投票只能沦为橡皮图章。

claim 还必须自带 `category` 自分类：`domain`（针对用户任务的实质主张）、`process`（对本次辩论运行本身的观察，如"claim 数量过多建议去重"）或 `synthesis`（总结全场的综合推荐）。`process` 类 claim 会被记录为该参与者的 `processNotes`（保留为协调反馈），但**永远不进入去重和 final vote**——依赖模型自分类而不是关键词黑名单，措辞再有创意的元评论也拦得住；旧的关键词启发式降级为兜底，命中时同样转为 processNotes 而不是静默丢弃。一条元评论也不再作废整个响应：provider 边界只校验 `category` 枚举值本身。

`synthesis` 类 claim 走独立的**合成阶段**（`debate_policy.synthesis`，需配置 `roles.free_debate.synthesizer`）：辩论轮结束后，synthesizer 作为 rapporteur 把原子 claim 和参与者的综合稿改写合成为**唯一一条 canonical 综合推荐**（被消费的综合稿 `mergedInto` 它、保留 provenance），随后进行 `amendment_rounds` 轮修正评审——每个参与者对草案给出 agree / revise（携带完整替换文本）/ disagree，synthesizer 整合修正意见——最终草案与原子 claim 一起进入 final vote。投票通过即"批准"（summary 的 `### Synthesis` 组显示 ratified / not ratified）。这解决了"每个参与者末轮各写一份综合推荐、丢弃式去重无法合并超集"的结构性冗余：改写式合并只发生在这个被授权的角色身上，而它的产物要经过全员修正与投票的双重审计。synthesizer 失败时自动降级回旧行为（参与者综合稿直接进投票）并记入 degradations。

final vote 不是纯二元多数票。每个 participant 会为每个 active claim 输出 `vote` 粗标签和连续 `confidence` 支持分数，且两者必须一致（accept 要求 ≥0.5，reject 要求 ≤0.5，违反会被校验拒绝并进入 repair）。系统把各票支持分数按 `vote_aggregation`（默认 `median`）聚合成 `supportScore`，用 `supportScore >= support_threshold` 判断 claim 是否 accepted；`confidenceVariance` / `confidenceStdDev` 仍然输出，帮助区分“高分低方差的强共识”和“中等分数高方差的真实争议”。summary 里 `support=` 显示的就是判定用的 `supportScore`。

投票是有法定人数的：成功返回 final vote 的参与者比例低于 `vote_quorum` 时，outcome 会被降级为 `quorum_not_met` 而不是伪装成 consensus / no_consensus，同时 result 的 `degradations` 和 summary 的 `## Degradations` 段会记录缺席者。`vote_quorum: 0`（默认）关闭该检查。`freeDebate` section 的 `voters` / `absentVoters` 字段无论是否启用 quorum 都会填充，summary 里对应 `- voters: N/M (absent: ...)` 行。

## `delphi`

固定宏观阶段：

```text
frame -> ingest -> delphi_questionnaire -> delphi_summary -> delphi_revision -> ... -> report -> action -> observe
```

核心角色：

- `participants`：匿名专家，独立评分和给出理由。
- `facilitator`：生成匿名聚合摘要和分歧点。
- `reporter`：输出最终收敛结论。
- `actor`：可选，执行后续 action。

适合：

- 希望避免先发锚定、权威偏见和从众心理。
- 多轮评分收敛，例如技术选型、迁移计划、风险评估。
- 需要明确“未收敛在哪里”的决策。

典型配置片段：

```yaml
defaults:
  mode: delphi
  success_criteria:
    - 给出匿名聚合结论
    - 明确评分分布、收敛程度和未收敛议题
  delphi_policy:
    min_rounds: 2
    max_rounds: 4
    convergence_threshold: 0.8
    rating_scale_min: 1
    rating_scale_max: 5
    anonymous: true

roles:
  delphi:
    participants: [participant-a, participant-b, participant-c]
    facilitator: facilitator-a
    reporter: reporter-a
```

## 如何串起来

三种 mode 可以作为一条决策链使用：

1. 用 `free_debate` 扩散观点，暴露主要立场、冲突和候选 claim。
2. 用 `adjudication` 对关键 claim 做证据化裁决，明确哪些成立、哪些需要 revision、哪些 unresolved。
3. 用 `delphi` 对仍有多方案取舍的问题做匿名收敛，减少权威偏见。

如果只想最小化复杂度，直接从 `adjudication` 开始。只有当你明确需要观点碰撞或匿名收敛时，再引入 `free_debate` 或 `delphi`。

## 命令入口

```bash
til-consensus classify "是否应该采用 monorepo？" --config ./til-consensus.yaml
til-consensus ask "是否应该采用 monorepo？" --config ./til-consensus.yaml
til-consensus debate "monorepo 和 polyrepo 如何取舍？" --config ./til-consensus.yaml
til-consensus delphi ./decision.md --config ./til-consensus.yaml
```

底层也可以用 `run --mode`：

```bash
til-consensus run --config ./til-consensus.yaml --mode free-debate --task-file ./case.md
til-consensus run --config ./til-consensus.yaml --mode delphi --input ./case.run.yaml
```
