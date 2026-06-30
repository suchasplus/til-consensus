# 三种讨论模式

`til-consensus` 支持三种 mode：`adjudication`、`free_debate`、`delphi`。选择 mode 的关键不是“哪个更高级”，而是你需要哪种讨论机制。

## 怎么选

| 场景 | 推荐 mode | 原因 |
| --- | --- | --- |
| 判断一个 claim、patch、benchmark 或设计结论是否成立 | `adjudication` | claim-centric，有 challenge、verify、revise、arbiter，适合需要审计的裁决 |
| 希望多个参与者充分碰撞观点，然后投票形成多数共识 | `free_debate` | participant 地位平等，多轮辩论后 final vote |
| 希望减少权威偏见和从众效应，做匿名多轮收敛 | `delphi` | 匿名问卷、聚合摘要、修订评分，直到收敛或达到轮数上限 |

默认使用 `adjudication`。它最稳，也最适合做代码裁决、事实核查和架构决策复核。

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
frame -> ingest -> initial -> debate* -> final_vote -> report -> action -> observe
```

核心角色：

- `participants`：平等参与者，先独立提出观点，再互相审阅和辩论。
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
    vote_threshold: 0.67
    enable_early_stop: true
    semantic_dedup:
      enabled: true
      similarity_threshold: 0.85

roles:
  free_debate:
    participants: [participant-a, participant-b, participant-c]
    semantic_deduper: deduper-a
    reporter: reporter-a
```

`semantic_dedup` 是进入 final vote 前的外部语义去重环节。启用后必须配置 `roles.free_debate.semantic_deduper`，该 agent 可以绑定任意 CLI/API provider。deduper 只输出 `sourceClaimId -> targetClaimId` 合并建议和 `similarity`，低于阈值的合并会被拒绝。合并后的 canonical claim 会保留 `proposedBy` 和 `mergedClaimIds`，例如“此 claim 由 Agent A 和 Agent C 独立提出”。

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
til-consensus ask "是否应该采用 monorepo？" --config ./til-consensus.yaml
til-consensus debate "monorepo 和 polyrepo 如何取舍？" --config ./til-consensus.yaml
til-consensus delphi ./decision.md --config ./til-consensus.yaml
```

底层也可以用 `run --mode`：

```bash
til-consensus run --config ./til-consensus.yaml --mode free-debate --task-file ./case.md
til-consensus run --config ./til-consensus.yaml --mode delphi --input ./case.run.yaml
```
