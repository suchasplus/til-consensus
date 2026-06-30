# 配置与输入样例

这篇只做样例索引。Provider 配置解释见 [Provider 配置与预检](providers.md)，配置结构见 [配置](config.md)，运行方式见 [操作手册](operations.md)。

## Provider 样例

多 provider 总模板：

- [all-providers.fill-in.config.yaml](examples/all-providers.fill-in.config.yaml)

CLI provider：

- [codex.config.yaml](examples/codex.config.yaml)
- [claude.config.yaml](examples/claude.config.yaml)
- [gemini.config.yaml](examples/gemini.config.yaml)
- [antigravity.config.yaml](examples/antigravity.config.yaml)
- [generic.config.yaml](examples/generic.config.yaml)

API provider：

- [openai-compatible.config.yaml](examples/openai-compatible.config.yaml)
- [anthropic-compatible.config.yaml](examples/anthropic-compatible.config.yaml)
- [gemini-api.config.yaml](examples/gemini-api.config.yaml)
- [openrouter.config.yaml](examples/openrouter.config.yaml)
- [deepseek.config.yaml](examples/deepseek.config.yaml)
- [qwen-max.config.yaml](examples/qwen-max.config.yaml)
- [kimi.config.yaml](examples/kimi.config.yaml)

多模型组合：

- [free-debate-tri-model.config.yaml](examples/free-debate-tri-model.config.yaml)
- [delphi-tri-model.config.yaml](examples/delphi-tri-model.config.yaml)

## Run 输入样例

- [architecture-decision.run.yaml](examples/architecture-decision.run.yaml)
- [document-refinement.run.yaml](examples/document-refinement.run.yaml)
- [coding-review.run.yaml](examples/coding-review.run.yaml)
- [factual-conflict.run.yaml](examples/factual-conflict.run.yaml)
- [free-debate-monorepo.run.yaml](examples/free-debate-monorepo.run.yaml)
- [delphi-ci-migration.run.yaml](examples/delphi-ci-migration.run.yaml)
- [observe-reopen.run.yaml](examples/observe-reopen.run.yaml)
- [generic.run.yaml](examples/generic.run.yaml)
- [codex.run.yaml](examples/codex.run.yaml)
- [claude.run.yaml](examples/claude.run.yaml)
- [gemini.run.yaml](examples/gemini.run.yaml)
- [antigravity.run.yaml](examples/antigravity.run.yaml)

## 复制后先做什么

```bash
til-consensus config validate --config ./til-consensus.yaml
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
til-consensus run --config ./til-consensus.yaml --input ./docs/examples/architecture-decision.run.yaml --dry-run
```

如果只想验证 provider 文件，不要求 roles 完整：

```bash
til-consensus profile preflight --config ./conf/providers.yaml --all --verbose
```

## `run.yaml` 还是 `--task-file`

用 `--task-file`：

- 只有一段长任务文本。
- 希望把会议纪要、需求或设计文档全文作为任务。
- 不需要单独配置 policy/roles。

用 `run.yaml`：

- 任务有结构化字段。
- 需要指定 mode、success criteria、constraints、policy。
- 希望同一个 case 在不同 provider/profile 下复跑。
