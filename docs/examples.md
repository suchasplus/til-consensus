# 配置与输入样例

这页分成两部分：

- 可直接落盘的 provider 配置样例
- 可直接复制的 `run.yaml`

如果你刚起步，建议直接：

1. `til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml`
2. 或者从下面挑一个完整配置文件直接复制
3. 再配一份对应场景的 `run.yaml`

## Provider 样例

### 六路总模板

如果你想一次把 `3 CLI + 3 API` 都先铺好，再慢慢填空和切角色，直接用：

- [all-providers.fill-in.config.yaml](examples/all-providers.fill-in.config.yaml)

这份模板已经包含：

- `codex / claude / gemini` 三个 CLI
- `openai-compatible / anthropic-compatible / gemini-api` 三个 API
- `adjudication / free_debate / delphi` 三种 mode 需要的角色位
- OpenRouter / Kimi / DeepSeek / Qwen 百炼这类兼容网关的填写说明

### `generic`

适合包你自己的脚本、本地模型适配器或公司内部 CLI。

- [完整配置：generic.config.yaml](examples/generic.config.yaml)
- 也可以直接生成：

```bash
til-consensus config init --mode adjudication --provider-profile generic --config ./til-consensus.yaml
```

### `codex`

- [完整配置：codex.config.yaml](examples/codex.config.yaml)
- 也可以直接生成：

```bash
til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml
```

### `claude`

- [完整配置：claude.config.yaml](examples/claude.config.yaml)
- 也可以直接生成：

```bash
til-consensus config init --mode adjudication --provider-profile claude --config ./til-consensus.yaml
```

### `gemini`

- [完整配置：gemini.config.yaml](examples/gemini.config.yaml)
- 也可以直接生成：

```bash
til-consensus config init --mode adjudication --provider-profile gemini --config ./til-consensus.yaml
```

### API 协议与兼容网关

下面这些样例都使用真实 API provider，不是 CLI：

- [openai-compatible.config.yaml](examples/openai-compatible.config.yaml)
- [anthropic-compatible.config.yaml](examples/anthropic-compatible.config.yaml)
- [gemini-api.config.yaml](examples/gemini-api.config.yaml)

如果你走兼容网关，也可以直接用下面这些组合包：

- [openrouter.config.yaml](examples/openrouter.config.yaml)
- [kimi.config.yaml](examples/kimi.config.yaml)
- [deepseek.config.yaml](examples/deepseek.config.yaml)
- [qwen-max.config.yaml](examples/qwen-max.config.yaml)

说明：

- `OpenRouter`、`Kimi`、`DeepSeek` 和 `Qwen 百炼兼容模式` 这类多数走 `openai-compatible`
- 真正需要改的核心字段通常只有：
  - `base_url`
  - `api_key_env`
  - `models.<id>.provider_model`
  - 以及可选的 `headers / options`
- `DeepSeek` 样例默认使用 `DEEPSEEK_API_KEY`，不会把明文 key 写进配置文件
- `Qwen Max` 样例默认使用 `BAILIAN_API_KEY`，并通过 `extra_body.enable_thinking` 打开思考模式
- 复制这些文件后，先用 `til-consensus config validate --config ./til-consensus.yaml` 做结构校验，再用 `til-consensus profile preflight --config ./til-consensus.yaml --all --verbose` 做真实连通性校验
- `profile preflight` 会执行带 schema 的 `{"ok": true}` 最小探测，默认 `max_output_tokens=2048`。如果你直接测试 `docs/examples/*.config.yaml`，建议加 `--output ./out/{requestId}`，避免输出落到 `docs/examples/out/`

### 多 CLI 交叉论证

推荐直接用下面的组合：

- `codex.config.yaml` 负责 proposer / reporter
- `claude.config.yaml` 负责 challenger / arbiter
- `gemini.config.yaml` 负责 semantic verifier

如果你想把三者合并到一份配置里，可以从这三份文件里直接拷贝 `providers / agents / roles`。

如果你要直接跑 Delphi 的三模型分工，也可以直接用：

- [Delphi 三模型综合配置：delphi-tri-model.config.yaml](examples/delphi-tri-model.config.yaml)
- [Free Debate 三模型综合配置：free-debate-tri-model.config.yaml](examples/free-debate-tri-model.config.yaml)

这份配置已经按模型特点分好了角色：

- `participant-claude`
  - 偏谨慎、擅长补 caveat 和边界条件
- `participant-gemini`
  - 偏发散、擅长扩展备选方案和比较路径
- `participant-codex`
  - 偏执行、强调迁移顺序、依赖和落地成本
- `facilitator-claude`
  - 负责匿名汇总和分歧归纳
- `reporter-codex`
  - 负责把最终建议压成可执行摘要

## 一对一组合包

下面这 4 组是“配置 + run 输入”一对一配套的最小可复制组合：

- `generic`
  - [generic.config.yaml](examples/generic.config.yaml)
  - [generic.run.yaml](examples/generic.run.yaml)
- `codex`
  - [codex.config.yaml](examples/codex.config.yaml)
  - [codex.run.yaml](examples/codex.run.yaml)
- `claude`
  - [claude.config.yaml](examples/claude.config.yaml)
  - [claude.run.yaml](examples/claude.run.yaml)
- `gemini`
  - [gemini.config.yaml](examples/gemini.config.yaml)
  - [gemini.run.yaml](examples/gemini.run.yaml)

使用方式统一是：

```bash
cp docs/examples/codex.config.yaml ./til-consensus.yaml
til-consensus run --config ./til-consensus.yaml --input ./docs/examples/codex.run.yaml
til-consensus view --config ./til-consensus.yaml
```

把 `codex` 替换成 `generic`、`claude` 或 `gemini` 即可。

如果你不想写 `run.yaml`，也可以把任务直接写进文件，再用 `--task-file`：

```bash
cp docs/examples/codex.config.yaml ./til-consensus.yaml
til-consensus run --config ./til-consensus.yaml --task-file ./task.md
til-consensus view --config ./til-consensus.yaml
```

`--task-file` 会读取文件全部内容作为任务文本，适合长问题、文档草稿或完整背景说明。

## `run.yaml` 样例

- [文档完善](examples/document-refinement.run.yaml)
- [架构选择](examples/architecture-decision.run.yaml)
- [Free Debate 仓库策略辩论](examples/free-debate-monorepo.run.yaml)
- [Delphi 决策收敛](examples/delphi-ci-migration.run.yaml)
- [coding review](examples/coding-review.run.yaml)
- [事实冲突与 freshness](examples/factual-conflict.run.yaml)
- [observe 否定 action 后 reopen](examples/observe-reopen.run.yaml)

推荐搭配：

- 文档完善 / 架构选择：
  - `codex.config.yaml` 或 `claude.config.yaml`
- Free Debate 仓库策略辩论：
  - `til-consensus config init --mode free-debate --provider-profile mock --config ./til-consensus.yaml`
  - 或者直接使用三模型分工：

```bash
cp docs/examples/free-debate-tri-model.config.yaml ./til-consensus.yaml
til-consensus run --config ./til-consensus.yaml --input ./docs/examples/free-debate-monorepo.run.yaml
til-consensus view --config ./til-consensus.yaml --section rounds --section votes
```
- Delphi 决策收敛：
  - `til-consensus config init --mode delphi --provider-profile mock --config ./til-consensus.yaml`
  - 或现有 `claude / gemini / codex` 配置，但 roles 需要包含 `participants / facilitator / reporter`
  - `delphi-ci-migration.run.yaml` 故意不内嵌 `roles`，默认继承 config 里的角色映射；如果你在 `run.yaml` 里写了 `roles`，它会覆盖 config
  - 如果你要直接用三模型分工：

```bash
cp docs/examples/delphi-tri-model.config.yaml ./til-consensus.yaml
til-consensus run --config ./til-consensus.yaml --input ./docs/examples/delphi-ci-migration.run.yaml
til-consensus view --config ./til-consensus.yaml --section rounds --section statements --section convergence
```
- coding review：
  - `--mode adjudication --provider-profile mock --task-profile coding` 或 `codex.config.yaml`
- factual conflict：
  - `generic.config.yaml` 或 `gemini.config.yaml`

如果你想一步到位，优先用上面的“一对一组合包”。

## 什么时候用 `run.yaml`，什么时候用 `--task-file`

建议这样区分：

- 只想提交一个问题或一段长文本：
  - 用 `--task` 或 `--task-file`
- 需要指定：
  - `roles`
  - `proposal_policy`
  - `verification_policy`
  - `debate_policy`
  - `delphi_policy`
  - `action`
  - `materials`
  - `constraints`
  - `workspace_snapshot`
  - 用 `run.yaml`

两者也可以组合：

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --input ./docs/examples/coding-review.run.yaml \
  --task-file ./task.md
```

这时 `--task-file` 会覆盖 `run.yaml` 里的 `task_spec.goal`，其余 policy 和 roles 仍然沿用 `run.yaml`。
