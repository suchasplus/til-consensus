# 配置与输入样例

这页分成两部分：

- 可直接落盘的 provider 配置样例
- 可直接复制的 `run.yaml`

如果你刚起步，建议直接：

1. `til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml`
2. 或者从下面挑一个完整配置文件直接复制
3. 再配一份对应场景的 `run.yaml`

## Provider 样例

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

### 多 CLI 交叉论证

推荐直接用下面的组合：

- `codex.config.yaml` 负责 proposer / reporter
- `claude.config.yaml` 负责 challenger / arbiter
- `gemini.config.yaml` 负责 semantic verifier

如果你想把三者合并到一份配置里，可以从这三份文件里直接拷贝 `providers / agents / roles`。

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

## `run.yaml` 样例

- [文档完善](examples/document-refinement.run.yaml)
- [架构选择](examples/architecture-decision.run.yaml)
- [coding review](examples/coding-review.run.yaml)
- [事实冲突与 freshness](examples/factual-conflict.run.yaml)
- [observe 否定 action 后 reopen](examples/observe-reopen.run.yaml)

推荐搭配：

- 文档完善 / 架构选择：
  - `codex.config.yaml` 或 `claude.config.yaml`
- coding review：
  - `--mode adjudication --provider-profile mock --task-profile coding` 或 `codex.config.yaml`
- factual conflict：
  - `generic.config.yaml` 或 `gemini.config.yaml`

如果你想一步到位，优先用上面的“一对一组合包”。
