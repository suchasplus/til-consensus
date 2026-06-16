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
til-consensus last --config ./til-consensus.yaml
til-consensus view --config ./til-consensus.yaml
```

查看指定 request：

```bash
til-consensus inspect tc_1710000000000_abcd12 --config ./til-consensus.yaml
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
til-consensus open ./out/tc_1710000000000_abcd12/result.json
til-consensus view --result ./out/tc_1710000000000_abcd12/result.json --web
```

查看 raw/debug artifact：

```bash
til-consensus logs tc_1710000000000_abcd12 --config ./til-consensus.yaml --type raw
til-consensus logs --result ./out/tc_1710000000000_abcd12/result.json --type telemetry
til-consensus logs --result ./out/tc_1710000000000_abcd12/result.json --latest --type raw
```

`last`、`inspect`、`logs`、`open` 都是快捷入口；完整参数仍然可以用 `view` 和 `artifact list/show`。

`profile preflight` 也会写出标准 `result.json`，因此可以直接查看 readiness：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
til-consensus view --result ./out/tc_xxx/result.json --section debug --verbose
til-consensus view --result ./out/tc_xxx/result.json --web --open
```

preflight 的 Debug / Telemetry 区块会展示 provider readiness，包括 `ready / strictJSON / recoverableJSON / durationMs / error`。探测默认使用 `max_output_tokens=2048`；如果 Gemini 等 thinking 模型耗尽预算，错误里会带 `finishReason` 和 token usage，便于判断是模型名、结构化输出参数还是输出预算问题。

`profile preflight` 运行时会逐 provider 分块输出；`view` 负责事后读取同一 run 目录里的 `artifacts/provider-readiness.json`，因此适合把一次预检结果分享给别人或在 Web Debug 区块里展开查看。

相对 `output.directory` 会按当前执行目录解析，而不是按配置文件所在目录解析。要临时指定输出目录，可以加：

```bash
til-consensus profile preflight --config docs/examples/deepseek.config.yaml --output ./out/{requestId} --all --verbose
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

当 `--format text` 且 stdout 连接到真实终端时，`view` 现在也会自动给关键字加颜色，例如：

- 标题
  - `运行头部`
  - `关键 Claims`
  - `验证明细`
  - `风险与未决项`
  - `Debug Events`
- 关键状态词
  - `supported`
  - `keep`
  - `keep_with_caveat`
  - `refuted`
  - `reject`
  - `undetermined`
  - `inconclusive`
  - `requires_human_review`

如果你输出的是：

- `--format markdown`
- `--format json`

则保持纯文本，不插入 ANSI 颜色码。

你也可以用环境变量控制：

- `NO_COLOR=1`
  - 关闭终端彩色输出
- `FORCE_COLOR=1`
  - 强制开启终端彩色输出

## 通用 section

对所有 mode 都可用：

- `overview`
- `observations`
- `followups`
- `debug`
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

如果你要在查看结果时直接展开运行期 debug 事件：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --section debug \
  --verbose
```

`debug` 会读取同目录下的 `events.jsonl`，显示每条运行事件的时间、类型、payload，以及推导出的 provider artifact 路径提示。

如果同一次 run 里生成了 telemetry artifact，`debug` 还会继续显示：

- `Provider Readiness`
  - 各 provider 的最小非交互探测结果
  - 包括 `ready / strictJSON / recoverableJSON / duration / error`
- `Run Summary`
  - 当前 run 的聚合质量
  - 包括 `primaryResult / taskVerdict / terminalState / workflow summary / task summary`
- `Strict Compliance`
  - 单 task 的结构化输出合规度

- `Telemetry`
  - `summary`
    - 按 `provider / providerModel / taskKind` 聚合
    - 显示 `total / strict / normalized / repaired / failed`
  - `reports`
    - 每个 task 一条
    - 显示 `finalStatus`
    - 显示是否发生 `strict -> normalized -> repair`
    - verbose 模式下还会展示：
      - `rawArtifact`
      - `initialErrorArtifact`
      - `finalArtifact`

如果事件 metadata 里带有原始模型语义，`debug` 现在还会显式显示：

- `rawVerdict`
- `rawTaskVerdict`

这样排查模型把：

- `rejected`
- `upheld`
- `insufficient_for_claim`
- `taskVerdict: { verdict: ..., rationale: ... }`

这类外部词表映射成内部 verdict 的过程时，不需要再手动翻完整 payload。

如果你想把最近一段时间的 run 再聚合成 markdown 报告，可以直接跑：

```bash
til-consensus telemetry daily --config ./til-consensus.yaml
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
- `Debug`
- `Workflow`
- `Files`

页面内支持：

- section 切换
- claim verdict 过滤
- limit 调整
- verbose 开关
- `<details>` 折叠
- Debug 区块直接展示：
  - `rawVerdict`
  - `rawTaskVerdict`
  - provider readiness `Telemetry`
  - strict compliance `Telemetry`

Web 的 `Debug` 面板现在分成两部分：

- `Events`
  - 运行期事件、payload、provider artifact 路径提示
- `Telemetry`
  - `provider readiness`
    - 各 provider 的 `ready / strictJSON / recoverableJSON / duration / error`
  - `summary`
    - `provider / providerModel / taskKind`
    - `strict / normalized / repaired / failed`
  - `reports`
    - 每个 task 的最终状态
    - `strictError / finalError`
    - `rawArtifact / initialErrorArtifact / finalArtifact`

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
