# 操作手册

这篇覆盖日常 CLI 操作：运行、诊断、查看结果、查看 artifact、telemetry、follow-up 和 exit code。

## 运行任务

短命令优先：

```bash
til-consensus ask "判断这个 patch 是否真正修复了竞态问题" --config ./til-consensus.yaml
til-consensus debate "monorepo 和 polyrepo 如何取舍？" --config ./til-consensus.yaml
til-consensus delphi ./decision.md --config ./til-consensus.yaml
```

底层 `run`：

```bash
til-consensus run --config ./til-consensus.yaml --task "是否推进这个方案？"
til-consensus run --config ./til-consensus.yaml --task-file ./task.md
til-consensus run --config ./til-consensus.yaml --input ./case.run.yaml
```

输入优先级：

1. `--task` 或 `--task-file`
2. `--input` 中的 `task_spec.goal`
3. config defaults

约束：

- `--task` 和 `--task-file` 不能同时使用。
- `--task-file` 会读取文件全部文本内容。
- `--task-file` 可以和 `--input` 一起使用，用于覆盖 `run.yaml` 里的任务文本。

## Dry-run

只看计划，不调用 provider：

```bash
til-consensus run --config ./til-consensus.yaml --input ./case.run.yaml --dry-run
til-consensus ask ./task.md --config ./til-consensus.yaml --dry-run --format json
```

`--dry-run` 会展示最终 mode、roles、agent/provider/model 映射、输出路径、phase 顺序和 policy 摘要。

## 诊断

静态诊断：

```bash
til-consensus doctor --config ./til-consensus.yaml
```

连真实 provider 一起检查：

```bash
til-consensus doctor --config ./til-consensus.yaml --providers --verbose
```

Provider 专项预检：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
```

## 配置查看

展开 include/profile 后的最终配置：

```bash
til-consensus config render --config ./til-consensus.yaml
til-consensus config render --config ./til-consensus.yaml --format json
til-consensus config render --config ./til-consensus.yaml --profile delphi
```

如果只想渲染 provider/profile 层，不要求完整 workflow roles：

```bash
til-consensus config render --config ./conf/providers.yaml --profiles-only
```

解释最终角色和 provider/model 映射：

```bash
til-consensus config explain --config ./til-consensus.yaml
til-consensus config explain --config ./til-consensus.yaml --provider gemini-api
til-consensus config explain --config ./til-consensus.yaml --agent arbiter-a
```

## 查看结果

最近一次：

```bash
til-consensus last --config ./til-consensus.yaml
```

指定 run：

```bash
til-consensus inspect tc_xxx --config ./til-consensus.yaml
```

底层 view：

```bash
til-consensus view --config ./til-consensus.yaml
til-consensus view --result ./out/tc_xxx/result.json
```

常用 section：

```bash
til-consensus view --result ./out/tc_xxx/result.json --section summary
til-consensus view --result ./out/tc_xxx/result.json --section claims
til-consensus view --result ./out/tc_xxx/result.json --section debug --verbose
til-consensus view --result ./out/tc_xxx/result.json --section telemetry
```

Web viewer：

```bash
til-consensus open tc_xxx --config ./til-consensus.yaml
til-consensus view --config ./til-consensus.yaml --web --open
```

Web viewer 是本地只读页面，适合展开每轮 debug 日志、payload、raw verdict 和 telemetry。

## Artifact 快捷查看

列出 artifact：

```bash
til-consensus artifact list --config ./til-consensus.yaml
til-consensus artifact list --config ./til-consensus.yaml --type raw
til-consensus artifact list --config ./til-consensus.yaml --type error
```

查看 artifact：

```bash
til-consensus artifact show --result ./out/tc_xxx/result.json --id 1
til-consensus artifact show --result ./out/tc_xxx/result.json --path artifacts/run-telemetry.json
```

raw/debug 快捷入口：

```bash
til-consensus logs tc_xxx --config ./til-consensus.yaml --type raw
til-consensus logs tc_xxx --config ./til-consensus.yaml --type input
til-consensus logs tc_xxx --config ./til-consensus.yaml --type error
```

## Verbose / debug

运行时两级日志：

- `--verbose`：业务级摘要，例如 phase、task dispatch、claim/revision/adjudication。
- `--debug`：原始 provider 输入输出、完整 payload、artifact 路径。

```bash
til-consensus run --config ./til-consensus.yaml --task-file ./task.md --verbose --debug
```

`view` 也支持 debug/telemetry 展示：

```bash
til-consensus view --result ./out/tc_xxx/result.json --section debug --verbose
til-consensus view --result ./out/tc_xxx/result.json --section telemetry
```

## Telemetry

汇总最近一天：

```bash
til-consensus telemetry daily --config ./til-consensus.yaml
```

指定扫描目录和输出：

```bash
til-consensus telemetry daily \
  --root ./logs/out \
  --since 24h \
  --output ./reports/daily-telemetry.md
```

重点看：

- provider readiness。
- provider × task strict compliance。
- schema repair 是否触发。
- normalize 规则是否触发。
- unresolved / undetermined 比例。

## Follow-up / session

执行 follow-up artifact：

```bash
til-consensus followup run --config ./til-consensus.yaml --artifact ./out/parent-run/artifacts/followups/case.json
til-consensus run --config ./til-consensus.yaml --followup ./out/parent-run/artifacts/followups/case.json
```

查看 session：

```bash
til-consensus session list --config ./til-consensus.yaml --request-id tc_xxx
til-consensus session show --config ./til-consensus.yaml --session-id session_xxx
```

恢复或 replay：

```bash
til-consensus run --config ./til-consensus.yaml --resume-session session_xxx
til-consensus run --config ./til-consensus.yaml --replay-session session_xxx
```

## Exit code

CLI 会把常见失败映射成稳定 exit code：

| Code | 含义 |
| --- | --- |
| `0` | 成功 |
| `1` | 通用失败 |
| `2` | 参数错误 |
| `3` | 配置无效 |
| `4` | 输入无效 |
| `5` | 输出目录不可用 |
| `6` | provider 不可用 |
| `7` | provider 认证失败 |
| `8` | provider 限流 |
| `9` | provider 超时 |
| `10` | provider schema 失败 |
| `11` | workflow 失败 |
| `12` | action 失败 |
| `13` | artifact 不存在 |
| `14` | artifact 无效 |
