# 终端 `view` 用法

`til-consensus view` 是统一查看入口。它会根据 `result.json` 里的 `mode` 自动渲染对应的结构。

除了终端输出，现在也支持本地只读 Web viewer：

```bash
til-consensus view --config ./til-consensus.yaml --web
```

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

启动本地 Web viewer：

```bash
til-consensus view --result ./out/tc_1710000000000_abcd12/result.json --web
```

默认只监听 `127.0.0.1`，并打印实际 URL，不会自动打开浏览器。

如果你要显式打开默认浏览器：

```bash
til-consensus view --result ./out/tc_1710000000000_abcd12/result.json --web --open
```

也可以自定义 host / port：

```bash
til-consensus view --config ./til-consensus.yaml --web --host 127.0.0.1 --port 8080
```

## 输出格式

默认是 `text`：

```bash
til-consensus view --config ./til-consensus.yaml --format text
```

也支持：

- `--format markdown`
- `--format json`

如果同时传了 `--web`，`--format` 会被忽略；页面固定返回 HTML，数据接口固定返回 JSON。

## 通用 section

对所有 mode 都可用：

- `overview`
- `observations`
- `followups`
- `artifacts`

示例：

```bash
til-consensus view --config ./til-consensus.yaml --section overview --section artifacts
```

如果你要专门排查 reopen / follow-up 链路：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --section observations \
  --section followups \
  --verbose
```

## `adjudication` 专用 section

- `claims`
- `challenges`
- `verifications`

其中 `observations` / `followups` 在 `adjudication` 下最有价值，因为它们会明确显示：

- 哪个 observation 导致 reopen
- follow-up artifact 路径
- parent/child request 关系

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
- 哪个 observation 导致 reopen
- parent/child request 关系

## Web viewer 页面

`view --web` 这轮是只读 MVP，固定提供这些块：

- `Overview`
- `Claims`
- `Evidence`
- `Observations`
- `Follow-ups`
- `Workflow`
- `Files`

页面内支持：

- section 切换
- claim verdict 过滤
- limit 调整
- verbose 开关
- `<details>` 折叠

数据接口：

- `GET /`
  - 页面 HTML
- `GET /api/document`
  - 当前 `Document` JSON
- `GET /api/healthz`
  - 健康检查

当前不支持：

- 目录浏览
- artifact 内容在线预览
- 自动刷新
- 远程开放监听
