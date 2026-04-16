# 配置与输入样例

这页给出两类可直接复用的样例：

- provider / agent 配置片段
- 可直接复制的 `run.yaml`

如果你刚起步，建议先：

1. `til-consensus config init --preset quickstart --config ./til-consensus.yaml`
2. 从下面复制 provider 片段替换 `providers:` / `agents:` / `roles:`
3. 再从下面挑一个 `run.yaml`

## Provider 样例

### `generic`

适合包你自己的脚本、本地模型适配器或公司内部 CLI。

```yaml
providers:
  generic-local:
    type: cli
    cli_type: generic
    command: python3
    args:
      - ./scripts/generic_adapter.py
    models:
      default:
        provider_model: local-generic
        reasoning: medium

agents:
  - id: proposer-generic
    provider: generic-local
    model: default
    role: proposer
  - id: challenger-generic
    provider: generic-local
    model: default
    role: challenger
  - id: arbiter-generic
    provider: generic-local
    model: default
    role: arbiter
  - id: reporter-generic
    provider: generic-local
    model: default
    role: reporter

roles:
  proposers: [proposer-generic]
  challengers: [challenger-generic]
  arbiter: arbiter-generic
  reporter: reporter-generic
```

### `codex`

```yaml
providers:
  codex-cli:
    type: cli
    cli_type: codex
    command: codex
    models:
      default:
        provider_model: gpt-5
        reasoning: medium
```

### `claude`

```yaml
providers:
  claude-cli:
    type: cli
    cli_type: claude
    command: claude
    models:
      default:
        provider_model: claude-sonnet-4
        reasoning: medium
```

### `gemini`

```yaml
providers:
  gemini-cli:
    type: cli
    cli_type: gemini
    command: gemini
    models:
      default:
        provider_model: gemini-2.5-pro
        reasoning: medium
```

### 多 CLI 交叉论证

```yaml
providers:
  codex-cli:
    type: cli
    cli_type: codex
    command: codex
    models:
      default:
        provider_model: gpt-5
        reasoning: medium
  claude-cli:
    type: cli
    cli_type: claude
    command: claude
    models:
      default:
        provider_model: claude-sonnet-4
        reasoning: medium
  gemini-cli:
    type: cli
    cli_type: gemini
    command: gemini
    models:
      default:
        provider_model: gemini-2.5-pro
        reasoning: medium

agents:
  - id: proposer-codex
    provider: codex-cli
    model: default
    role: proposer
  - id: challenger-claude
    provider: claude-cli
    model: default
    role: challenger
  - id: verifier-gemini
    provider: gemini-cli
    model: default
    role: semantic-verifier
  - id: arbiter-claude
    provider: claude-cli
    model: default
    role: arbiter
  - id: reporter-codex
    provider: codex-cli
    model: default
    role: reporter

roles:
  proposers: [proposer-codex]
  challengers: [challenger-claude]
  arbiter: arbiter-claude
  semantic_verifier: verifier-gemini
  reporter: reporter-codex
```

## `run.yaml` 样例

- [架构选择](examples/architecture-decision.run.yaml)
- [observe 否定 action 后 reopen](examples/observe-reopen.run.yaml)
- [事实冲突与 freshness](examples/factual-conflict.run.yaml)
