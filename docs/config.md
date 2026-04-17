# 配置说明

`til-consensus` 的配置文件默认叫 `til-consensus.yaml`。

查找顺序：

1. `--config`
2. 当前目录下的 `./til-consensus.yaml`
3. `~/.config/til-consensus/default.yaml`
4. `~/.config/til-consensus/config.yaml`

如果你显式设置了 `XDG_CONFIG_HOME`，对应路径会落到 `$XDG_CONFIG_HOME/til-consensus/...`。

## 推荐起步方式

第一次上手时，直接生成模板：

```bash
til-consensus config init --mode adjudication --provider-profile mock --config ./til-consensus.yaml
```

`config init` 现在按 3 个维度生成模板：

- `--mode`
  - `adjudication`
  - `free-debate`
  - `delphi`
- `--provider-profile`
  - `mock`
  - `openai`
  - `generic`
  - `codex`
  - `claude`
  - `gemini`
- `--task-profile`
  - `general`
  - `coding`

如果想先看看模板，不落盘：

```bash
til-consensus config init --mode free-debate --provider-profile mock --stdout
```

常见组合：

```bash
til-consensus config init --mode adjudication --provider-profile generic --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile claude --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile gemini --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile mock --task-profile coding --config ./til-consensus.yaml
```

如果已有配置文件，需要覆盖：

```bash
til-consensus config init --mode delphi --provider-profile mock --config ./til-consensus.yaml --force
```

旧的 `--preset` 仍然可用，但现在只是兼容别名。例如：

- `quickstart`
- `coding`
- `debate`
- `delphi`
- `generic`
- `codex`
- `claude`
- `gemini`
- `openai`

provider profile 的当前默认模型：

- `codex`
  - `gpt-5.4`
- `claude`
  - `claude-opus-4-6`
- `gemini`
  - `gemini-3.1-pro-preivew`

## 配置结构

顶层固定包含：

- `schema_version`
- `defaults`
- `output`
- `providers`
- `agents`
- `roles`

### `defaults`

主要控制默认的 task 约束和 policy。

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

`adjudication` 专用：

- `proposal_policy`
- `verification_policy`
- `arbiter_policy`

其中：

- `loop_policy.max_revision_rounds`
- `loop_policy.max_verification_rounds`
- `loop_policy.material_confidence_delta`
- `fallback_policy.max_fallback_rounds`
- `fallback_policy.on_insufficient_evidence`
- `fallback_policy.on_unresolved_conflict`
- `fallback_policy.on_unresolved_claims`
- `fallback_policy.on_keep_with_caveat`

用于限制 `verify -> revise -> challenge/verify` 以及 `adjudicate -> revise/ingest` 这样的受控闭环。

`ingest_policy.sources` 和 `observe_policy.sources` 都是命令源列表，单个 source 支持：

- `name`
- `command`
- `args`
- `workdir`
- `env`
- `source_type`
- `reference`
- `success_pattern`
- `failure_pattern`
- `parsing`

`parsing` 当前支持四种模式：

- `mode: text`
  - 默认模式
  - 继续使用 `success_pattern` / `failure_pattern` 做文本匹配
- `mode: json`
  - 把 stdout 解析成 JSON
  - 支持用字段路径提取结构化结果
  - 可用字段：
    - `success_path`
    - `failure_path`
    - `summary_path`
    - `excerpt_path`
    - `notes_path`
    - `metadata_paths`
- `mode: yaml`
  - 把 stdout 解析成 YAML
  - 然后复用和 JSON 相同的路径提取规则
- `mode: xml`
  - 把 stdout 解析成简单 XML 树
  - 然后用点路径提取字段

字段路径使用简单的点路径，支持数组下标，例如：

- `status.ok`
- `report.summary`
- `items[0].name`

如果要跨数组提取，也支持简单的 `[*]`：

- `items[*].name`

如果你希望先做最基础的 schema 校验，再继续处理外部源，可以加：

- `required_paths`

示例：

```yaml
parsing:
  mode: yaml
  required_paths:
    - report.summary
    - report.publishedAt
  summary_path: report.summary
  excerpt_path: report.excerpt
  metadata_paths:
    publishedAt: report.publishedAt
```

适用语义：

- `ingest_policy`
  - 在 `ingest` 阶段执行
  - 适合补抓证据、归一化外部材料、追加 tool 输出
- `observe_policy`
  - 在 `observe` 阶段执行
  - 适合健康检查、回归观察、外部状态确认

`free_debate` 专用：

- `debate_policy`
  - `min_rounds`
  - `max_rounds`
  - `vote_threshold`
  - `enable_early_stop`
  - `peer_context_mode`

`delphi` 专用：

- `delphi_policy`
  - `min_rounds`
  - `max_rounds`
  - `convergence_threshold`
  - `rating_scale_min`
  - `rating_scale_max`
  - `anonymous`
  - `facilitator_summary_style`

### `providers`

当前支持：

- `type: mock`
- `type: api`
- `type: cli`
- `type: sdk`

常见字段：

- `protocol`
- `base_url`
- `api_key_env`
- `models`
- `command`
- `args`
- `env`

### `agents`

每个 agent 至少要有：

- `id`
- `provider`
- 可选 `model`
- 可选 `role`
- 可选 `system_prompt`

### `roles`

角色分配取决于当前 workflow。

`adjudication`：

- `proposers`
- `challengers`
- `arbiter`
- `semantic_verifier`
- `reporter`
- `actor`

`free_debate`：

- `participants`
- 可选 `reporter`
- 可选 `actor`

`delphi`：

- `participants`
- 可选 `facilitator`
- 可选 `reporter`
- 可选 `actor`

## 命令行覆盖

`run` 支持通过 CLI flags 覆盖配置和输入文件。

当前最重要的 mode 相关 flags：

- `--mode adjudication|free-debate|delphi`
- `--participants`
- `--facilitator`
- `--min-rounds`
- `--max-rounds`
- `--vote-threshold`
- `--convergence-threshold`

`adjudication` 相关输入里还建议显式提供：

- `task_spec.task_type`
- `task_spec.out_of_scope`
- `action_policy.risk_gate`

优先级始终是：

`CLI flags > input file > config defaults > built-in defaults`

## 最小示例

### 1. `adjudication`

```yaml
defaults:
  mode: adjudication
  fallback_policy:
    max_fallback_rounds: 1
    on_insufficient_evidence: ingest
    on_unresolved_conflict: ingest
    on_unresolved_claims: revise
    on_keep_with_caveat: revise
  observe_policy:
    on_contradiction: reopen
    sources:
      - name: health
        command: sh
        args: ["-c", "printf HEALTHY"]
        success_pattern: HEALTHY

roles:
  proposers: [proposer-a]
  challengers: [challenger-a]
  arbiter: arbiter-a
  semantic_verifier: verifier-a
```

如果希望 `adjudicate` 自动补抓新证据，可以继续加：

```yaml
defaults:
  ingest_policy:
    sources:
      - name: fresh-evidence
        command: sh
        args:
          - -c
          - printf '{"status":{"ok":true},"report":{"summary":"fresh evidence captured","excerpt":"fresh evidence excerpt","score":0.92}}'
        source_type: external_command
        reference: sh -c printf fresh-evidence
        parsing:
          mode: json
          success_path: status.ok
          summary_path: report.summary
          excerpt_path: report.excerpt
          metadata_paths:
            score: report.score
```

### 2. `free_debate`

```yaml
defaults:
  mode: free_debate
  debate_policy:
    min_rounds: 2
    max_rounds: 3
    vote_threshold: 0.75

roles:
  participants: [debater-a, debater-b, debater-c]
  reporter: reporter-a
```

### 3. `delphi`

```yaml
defaults:
  mode: delphi
  delphi_policy:
    min_rounds: 2
    max_rounds: 4
    convergence_threshold: 0.8

roles:
  participants: [participant-a, participant-b, participant-c]
  facilitator: facilitator-a
  reporter: reporter-a
```

## 增量编辑

如果模板已经生成，后面只想补一个 provider 或 agent，可以用：

```bash
til-consensus config add-provider --help
til-consensus config add-agent --help
```

`config add-agent --assign` 目前支持：

- `proposer`
- `challenger`
- `arbiter`
- `semantic-verifier`
- `reporter`
- `actor`
- `participant`
- `facilitator`

这两个命令适合增量修改，不适合替代模板初始化。

## follow-up 与 session store

CLI 会把 session snapshot 持久化到输出目录同级的 `_sessions/` 下，例如：

- `./out/_sessions/session_xxx.json`

常用命令：

```bash
til-consensus followup run --config ./til-consensus.yaml --artifact ./out/parent-run/artifacts/followups/case.json
til-consensus run --config ./til-consensus.yaml --followup ./out/parent-run/artifacts/followups/case.json
til-consensus run --config ./til-consensus.yaml --resume-session session_xxx
til-consensus run --config ./til-consensus.yaml --replay-session session_xxx
til-consensus session list --config ./til-consensus.yaml --request-id tc_xxx
til-consensus session show --config ./til-consensus.yaml --session-id session_xxx
```

行为区别：

- `run --resume-session`
  - 当前会对 `adjudication` workflow 执行 checkpoint 级恢复
  - 也就是从最近一次已完成 phase 继续，而不是从 `frame` 重新开始
- `run --replay-session`
  - 不复用旧 session 的 phase 进度
  - 会生成新的 request id，并把旧 session 挂成 lineage 父节点
