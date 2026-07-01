# 快速开始

这篇只解决一个问题：如何尽快把 `til-consensus` 跑起来、确认 provider 可用、看到结果。需要完整配置细节时再看 [配置](config.md)、[Provider](providers.md) 和 [操作手册](operations.md)。

## 1. 确认 CLI 可用

```bash
til-consensus version
til-consensus --help
```

如果你在仓库内开发，也可以直接用本地构建产物：

```bash
bin/til-consensus version
```

## 2. 生成起步配置

推荐从 split config 开始，避免一个巨大的 YAML 文件：

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

- `til-consensus.yaml` 只放 `include`、默认 `profile` 和 `output`。
- `conf/providers.yaml` 放 provider、model 和 agent。
- `conf/profiles.yaml` 放 mode、roles 和 policy overlay。

如果只想预览模板，不落盘：

```bash
til-consensus setup --mode adjudication --provider-profile mock --stdout
```

## 3. 跑一次任务

日常优先使用 task-first 短命令：

```bash
til-consensus ask "判断这个 patch 是否真正修复了竞态问题" --config ./til-consensus.yaml
```

从文件读取完整任务文本：

```bash
til-consensus ask ./task.md --config ./til-consensus.yaml
```

三种短命令对应三种 mode：

```bash
til-consensus ask "是否应该合并这个 patch？" --config ./til-consensus.yaml
til-consensus debate "monorepo 和 polyrepo 如何取舍？" --config ./til-consensus.yaml
til-consensus delphi ./decision.md --config ./til-consensus.yaml
```

如果还不确定该选哪种 mode，先让 `classify` 判断：

```bash
til-consensus classify "monorepo 和 polyrepo 如何取舍？" --config ./til-consensus.yaml
til-consensus classify --file ./task.md --config ./til-consensus.yaml
cat ./task.md | til-consensus classify --stdin --config ./til-consensus.yaml
```

`classify` 默认使用 `gemini-api/default`，只要求 providers 配置可用，不要求 agents/roles 完整。它会返回 `adjudication`、`free_debate`、`delphi`、`needs_clarification` 或 `not_suitable`。当返回 `needs_clarification` 时，它也会预估用户补齐缺失信息后大概率适合的 mode，并说明理由。

底层 `run` 仍然保留，适合 CI、脚本或需要精确传入 `run.yaml` 的场景：

```bash
til-consensus run --config ./til-consensus.yaml --task-file ./task.md
til-consensus run --config ./til-consensus.yaml --input ./case.run.yaml
```

## 4. 先 dry-run，不花 token

在接真实 provider 前，先确认最终计划：

```bash
til-consensus ask "是否推进这个方案？" --config ./til-consensus.yaml --dry-run
til-consensus debate ./case.md --config ./til-consensus.yaml --dry-run --format json
```

`--dry-run` 会展示：

- 当前 config/profile。
- request id、mode、task preview。
- role assignment。
- agent -> provider -> model 映射。
- phase 顺序、输出目录、timeout/retry/policy 摘要。

它不会调用 provider，也不会创建 run 目录。

## 5. 验证真实 provider

真实 CLI/API 不建议直接跑完整 workflow，先做 provider preflight：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
```

只检查某个 provider：

```bash
til-consensus profile preflight \
  --config ./til-consensus.yaml \
  --provider gemini-api \
  --output ./out/{requestId} \
  --verbose
```

preflight 会做最小非交互 JSON 探测，要求 provider 返回 `{"ok": true}`。多个 provider 会逐个分块输出；全部 ready 时终端汇总会显示为绿色，否则显示为红色。

如果只想做本机静态检查，不调用 provider：

```bash
til-consensus doctor --config ./til-consensus.yaml
```

把真实 provider 也纳入 doctor：

```bash
til-consensus doctor --config ./til-consensus.yaml --providers --verbose
```

## 6. 查看结果

看最近一次：

```bash
til-consensus last --config ./til-consensus.yaml
```

看指定 run：

```bash
til-consensus inspect tc_xxx --config ./til-consensus.yaml
```

打开 Web viewer：

```bash
til-consensus open tc_xxx --config ./til-consensus.yaml
```

查看 raw/debug artifact：

```bash
til-consensus logs tc_xxx --config ./til-consensus.yaml --type raw
til-consensus artifact list --config ./til-consensus.yaml --type error
til-consensus artifact show --result ./out/tc_xxx/result.json --path artifacts/run-telemetry.json
```

## 7. 推荐顺序

1. 用 `mock` 模板跑通 `ask`。
2. 用 `--dry-run` 看清最终 roles 和 provider/model 映射。
3. 配置真实 provider。
4. 跑 `profile preflight --all --verbose`。
5. 小任务跑 `ask`。
6. 需要多方碰撞时再试 `debate`。
7. 需要匿名收敛时再试 `delphi`。
