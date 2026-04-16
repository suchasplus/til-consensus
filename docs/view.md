# 终端 `view` 用法

`til-consensus view` 是统一查看入口。它会根据 `result.json` 里的 `mode` 自动渲染对应的结构。

默认行为：

- 如果传了 `--result`，直接读取指定的 `result.json`
- 如果传了 `--request-id`，按配置里的输出模板定位该次运行
- 如果都没传，自动读取最新一次完成的运行

## 最常用命令

查看最新一次：

```bash
til-consensus view --config ./til-consensus.yaml
```

查看指定 request：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --request-id tc_1710000000000_abcd12
```

直接看某个 `result.json`：

```bash
til-consensus view --result ./out/tc_1710000000000_abcd12/result.json
```

## 输出格式

默认是 `text`：

```bash
til-consensus view --config ./til-consensus.yaml --format text
```

也支持：

- `--format markdown`
- `--format json`

## 通用 section

对所有 mode 都可用：

- `overview`
- `artifacts`

示例：

```bash
til-consensus view --config ./til-consensus.yaml --section overview --section artifacts
```

## `adjudication` 专用 section

- `claims`
- `challenges`
- `verifications`

示例：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --section claims \
  --section verifications
```

## `free_debate` 专用 section

- `rounds`
- `votes`

示例：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --section rounds \
  --section votes
```

## `delphi` 专用 section

- `rounds`
- `statements`
- `convergence`

示例：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --section statements \
  --section convergence
```

## 常用过滤

只对 `adjudication` 的 claims 生效：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --claim-verdict undetermined
```

限制展示条数：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --limit 5
```

展开 rationale、evidence refs 和 artifact 路径：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --verbose
```

## 文本输出会显示什么

默认 `text` 输出会按 mode 渲染：

- `adjudication`
  - 运行头部
  - 任务摘要
  - terminal state / task type / risk level
  - 裁决统计
  - 关键 Claims
  - 风险与未决项
  - 相关文件
- `free_debate`
  - 运行头部
  - 任务摘要
  - 辩论轮次
  - 最终投票
  - 风险与未决项
  - 相关文件
- `delphi`
  - 运行头部
  - 任务摘要
  - Delphi 轮次
  - 候选结论与收敛度
  - 主要异议
  - 相关文件

对于新的 claim-centric 裁决结构，`view` 会优先展示：

- `verdict`
- `disposition`
- `claim type`
- `caveats`
- 打开的 challenge
- 失败或 inconclusive 的 verification
