# til-consensus Fast Path

这份文档只讲高频路径：少记命令、少写 YAML、快速跑通、快速查看、需要时再下钻到底层命令。

底层正交命令仍然存在，例如 `run`、`view`、`artifact`、`config render`。Fast Path 是它们上面的一层薄入口，适合日常手动使用。

## 1. 一次性初始化

推荐从 split config 开始：

```bash
til-consensus setup --mode adjudication --provider-profile mock --dir .
```

它会生成：

```text
til-consensus.yaml
conf/providers.yaml
conf/profiles.yaml
```

职责分层：

- `til-consensus.yaml`
  - 只保留 `include`、默认 `profile`、`output`
- `conf/providers.yaml`
  - provider、model、agent
- `conf/profiles.yaml`
  - profile overlay、默认 mode、roles、policy

如果要生成 Delphi 起步配置：

```bash
til-consensus setup --mode delphi --provider-profile mock --dir .
```

等价子命令：

```bash
til-consensus config wizard --mode adjudication --provider-profile mock --dir .
```

如果只想预览，不落盘：

```bash
til-consensus setup --mode adjudication --provider-profile mock --stdout
```

## 2. 日常运行：ask / debate / delphi

### adjudication

用 `ask`，它等价于“用 adjudication 处理这个任务”：

```bash
til-consensus ask "判断这个 patch 是否真正修复了竞态问题"
```

指定配置：

```bash
til-consensus ask "是否应该迁移到 monorepo？" --config ./til-consensus.yaml
```

读取文件全文作为任务：

```bash
til-consensus ask ./task.md --config ./til-consensus.yaml
```

### free_debate

用 `debate`：

```bash
til-consensus debate "monorepo 和 polyrepo 如何取舍？" --config ./til-consensus.yaml
```

没有显式 `--participants` 时：

- 优先使用配置里的 `roles.participants`
- 如果没有，则从 `roles.proposers` 和 `roles.challengers` 去重推导

需要手动指定参与者时：

```bash
til-consensus debate "是否采用 monorepo？" \
  --config ./til-consensus.yaml \
  --participants participant-a,participant-b,participant-c
```

### delphi

用 `delphi`：

```bash
til-consensus delphi ./decision.md --config ./til-consensus.yaml
```

常用覆盖：

```bash
til-consensus delphi "是否推进 CI 迁移？" \
  --config ./til-consensus.yaml \
  --min-rounds 2 \
  --max-rounds 4 \
  --convergence-threshold 0.82
```

## 3. 先看计划，不花 token

所有短命令都支持 `--dry-run`：

```bash
til-consensus ask "是否应该合并这个 patch？" --dry-run
til-consensus debate ./case.md --dry-run --format json
til-consensus delphi ./decision.md --dry-run
```

`--dry-run` 会展示：

- active config / profile
- requestId
- mode
- task preview
- role assignments
- agent -> provider -> model 映射
- phase 顺序
- output 路径
- timeout / retry / policy 摘要

它不会：

- 调用 provider
- 创建 run 目录
- 写 `result.json`
- 写 artifact

## 4. 用 profile 切换配置

`til-consensus.yaml` 可以只写默认 profile：

```yaml
schema_version: 1
include:
  - conf/providers.yaml
  - conf/profiles.yaml

profile: fast

output:
  directory: ./out/{requestId}
```

`conf/profiles.yaml` 可以放多套 overlay：

```yaml
schema_version: 1

profiles:
  fast:
    defaults:
      mode: adjudication
      per_task_timeout: 5m
    roles:
      proposers: [proposer-fast]
      challengers: [challenger-fast]
      arbiter: arbiter-fast
      reporter: reporter-fast

  strong-delphi:
    defaults:
      mode: delphi
      per_task_timeout: 20m
    roles:
      participants: [participant-a, participant-b, participant-c]
      facilitator: facilitator-a
      reporter: reporter-a
```

运行时覆盖：

```bash
til-consensus ask "是否推进这个方案？" --profile fast
til-consensus delphi ./decision.md --profile strong-delphi
```

provider preflight 使用 `--config-profile`，避免和 provider profile 概念混淆：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --config-profile fast --all --verbose
```

查看最终配置：

```bash
til-consensus config explain --config ./til-consensus.yaml --profile fast
til-consensus config render --config ./til-consensus.yaml --profile strong-delphi
```

## 5. 快速查看结果

### 看最近一次

```bash
til-consensus last --config ./til-consensus.yaml
```

等价底层命令：

```bash
til-consensus view --config ./til-consensus.yaml
```

### 看指定 run

```bash
til-consensus inspect tc_xxx --config ./til-consensus.yaml
```

或者直接给 `result.json`：

```bash
til-consensus inspect ./out/tc_xxx/result.json
```

### 打开 Web viewer

```bash
til-consensus open --config ./til-consensus.yaml
til-consensus open tc_xxx --config ./til-consensus.yaml
til-consensus open ./out/tc_xxx/result.json
```

### 看 raw / debug / telemetry artifact

列出 raw 类 artifact：

```bash
til-consensus logs tc_xxx --config ./til-consensus.yaml --type raw
```

看最新 raw：

```bash
til-consensus logs tc_xxx --config ./til-consensus.yaml --type raw --latest
```

看 telemetry：

```bash
til-consensus logs tc_xxx --config ./til-consensus.yaml --type telemetry
```

看指定 artifact：

```bash
til-consensus logs tc_xxx --config ./til-consensus.yaml --id 1
til-consensus logs tc_xxx --config ./til-consensus.yaml --path artifacts/run-telemetry.json
```

底层命令仍然可用：

```bash
til-consensus artifact list --config ./til-consensus.yaml
til-consensus artifact show --config ./til-consensus.yaml --id 1
```

## 6. 诊断路径

本地配置和命令检查：

```bash
til-consensus doctor --config ./til-consensus.yaml
```

真实调用 provider 做 readiness preflight：

```bash
til-consensus doctor --config ./til-consensus.yaml --providers --verbose
```

只检查 provider：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
```

查看 preflight 结果：

```bash
til-consensus last --config ./til-consensus.yaml --section debug --verbose
til-consensus open --config ./til-consensus.yaml
```

## 7. 什么时候回到底层命令

继续用 Fast Path：

- 手动提问
- 快速比较方案
- 看最近结果
- 看 raw/debug artifact
- 切 profile

回到底层命令：

- CI / cron / automation 需要完全显式参数
- 要执行 `--followup`、`--resume-session`、`--replay-session`
- 要精确覆盖 `actor`、`workspace-snapshot`、`action`
- 要生成 telemetry daily report
- 要做 artifact 安全边界外读取

对应底层命令：

```bash
til-consensus run ...
til-consensus view ...
til-consensus artifact list/show ...
til-consensus telemetry daily ...
til-consensus followup run ...
til-consensus session list/show ...
```

## 8. 推荐日常流程

第一次：

```bash
til-consensus setup --mode adjudication --provider-profile mock --dir .
til-consensus doctor --config ./til-consensus.yaml
til-consensus ask "先跑一个 mock 任务" --config ./til-consensus.yaml
til-consensus last --config ./til-consensus.yaml
```

接真实 provider 后：

```bash
til-consensus doctor --config ./til-consensus.yaml --providers --verbose
til-consensus ask ./task.md --config ./til-consensus.yaml --dry-run
til-consensus ask ./task.md --config ./til-consensus.yaml --verbose
til-consensus open --config ./til-consensus.yaml
```

排查 provider 漂移：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
til-consensus logs --config ./til-consensus.yaml --type raw
til-consensus last --config ./til-consensus.yaml --section debug --verbose
```
