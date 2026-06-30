# til-consensus

`til-consensus` 是一个面向一次性论证、架构选择、代码裁决和多模型讨论的 CLI。它用确定性的 coordinator 编排多种 workflow，把过程和结论落成可审计的本地产物。

默认 mode 是 `adjudication`：系统不会只选“哪篇完整答案看起来更像对的”，而是围绕 claim 做 challenge、verification、revision 和 adjudication。

## 快速开始

生成一份可运行配置：

```bash
til-consensus setup --mode adjudication --provider-profile mock --dir .
```

运行一次任务：

```bash
til-consensus ask "判断这个 patch 是否真正修复了竞态问题" --config ./til-consensus.yaml
```

从文件读取完整任务：

```bash
til-consensus ask ./task.md --config ./til-consensus.yaml
```

查看最新结果：

```bash
til-consensus last --config ./til-consensus.yaml
```

更完整的起步路径见 [快速开始](docs/quickstart.md)。

## 三种模式

| Mode | 快捷命令 | 适合场景 |
| --- | --- | --- |
| `adjudication` | `til-consensus ask ...` | claim 级裁决、事实核查、patch/benchmark/架构结论是否成立 |
| `free_debate` | `til-consensus debate ...` | 多参与者充分碰撞观点，最后投票形成多数共识 |
| `delphi` | `til-consensus delphi ...` | 匿名多轮评分和摘要收敛，降低权威偏见和从众效应 |

选择建议见 [三种讨论模式](docs/modes.md)。

## Provider 预检

接真实 CLI/API 后，先做 preflight，不要直接跑完整 workflow：

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

Provider 支持 CLI 和 API：

- CLI：Codex、Claude Code、Gemini CLI、Antigravity CLI、generic。
- API：OpenAI-compatible、OpenAI Responses、Anthropic-compatible、Gemini API。

详细配置见 [Provider 配置与预检](docs/providers.md)。

## 配置结构

推荐使用 split config，而不是维护一个巨大的 YAML：

```text
til-consensus.yaml
conf/providers.yaml
conf/agents-adjudication.yaml
conf/agents-free-debate.yaml
conf/agents-delphi.yaml
conf/roles-adjudication.yaml
conf/roles-free-debate.yaml
conf/roles-delphi.yaml
```

主配置通常只保留：

```yaml
schema_version: 1
include:
  - ./conf/providers.yaml
  - ./conf/agents-adjudication.yaml
  - ./conf/roles-adjudication.yaml

profile: adjudication

output:
  directory: ./out/{requestId}
```

配置规则见 [配置](docs/config.md)。可复制样例见 [配置与输入样例](docs/examples.md)。

## 常用命令

```bash
# 传统模板初始化
til-consensus config init --mode adjudication --provider-profile mock --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml
til-consensus config init --mode free-debate --provider-profile mock --stdout
til-consensus config init --mode adjudication --provider-profile mock --task-profile coding --stdout

# 看计划，不调用 provider
til-consensus ask ./task.md --config ./til-consensus.yaml --dry-run

# 展开 include/profile 后的最终配置
til-consensus config render --config ./til-consensus.yaml

# 人类可读地解释 provider/agent/roles
til-consensus config explain --config ./til-consensus.yaml

# 静态诊断
til-consensus doctor --config ./til-consensus.yaml

# 打开本地 Web viewer
til-consensus open tc_xxx --config ./til-consensus.yaml

# 查看 raw/debug artifact
til-consensus logs tc_xxx --config ./til-consensus.yaml --type raw
til-consensus artifact list --config ./til-consensus.yaml --type error
```

完整命令说明见 [操作手册](docs/operations.md)。

## 底层 `run` 示例

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --task "是否采用 monorepo？"

til-consensus run \
  --config ./til-consensus.yaml \
  --mode free-debate \
  --task "monorepo 和 polyrepo 如何取舍？" \
  --participants debater-a,debater-b,debater-c

til-consensus run \
  --config ./til-consensus.yaml \
  --mode delphi \
  --task-file ./decision.md \
  --convergence-threshold 0.8

til-consensus view --config ./til-consensus.yaml
```

## 输出产物

每次运行默认写到：

```text
./out/{requestId}/
```

关键文件：

- `result.json`：统一结果壳。
- `summary.md`：人工阅读摘要。
- `ledger.jsonl`：append-only 审计账本。
- `events.jsonl`：运行事件流。
- `artifacts/`：provider input/raw/failure、readiness、telemetry、manifest。

详细说明见 [输出产物](docs/outputs.md)。

## 开发与质量门禁

本地提交前：

```bash
make pre-push
```

它会执行格式检查、覆盖率测试、coverage treemap、`go vet`、`golangci-lint` 和构建。覆盖率图会写到：

```text
tmp/coverage/coverage.svg
```

更多见 [测试、CI 与发布](docs/testing.md)。

发布包示例：

```bash
make ci
make release-archive VERSION=v0.1.0 TARGET_GOOS=darwin TARGET_GOARCH=arm64 DIRTY=false
```

## 文档入口

- [快速开始](docs/quickstart.md)
- [三种讨论模式](docs/modes.md)
- [配置](docs/config.md)
- [Provider 配置与预检](docs/providers.md)
- [操作手册](docs/operations.md)
- [输出产物](docs/outputs.md)
- [测试、CI 与发布](docs/testing.md)
- [技术架构](docs/architecture.md)
- [配置与输入样例](docs/examples.md)

保留的兼容入口：

- [旧版 CI 与发布入口](docs/release.md)
- [旧版输出产物入口](docs/output.md)
- [旧版终端 view 入口](docs/view.md)
- [旧版浏览器 Viewer 入口](docs/viewer.md)
- [旧版技术设计入口](docs/rewrite.md)

常用样例文件：

- [generic.config.yaml](docs/examples/generic.config.yaml)
- [codex.config.yaml](docs/examples/codex.config.yaml)
- [claude.config.yaml](docs/examples/claude.config.yaml)
- [gemini.config.yaml](docs/examples/gemini.config.yaml)
- [antigravity.config.yaml](docs/examples/antigravity.config.yaml)
- [document-refinement.run.yaml](docs/examples/document-refinement.run.yaml)
- [coding-review.run.yaml](docs/examples/coding-review.run.yaml)
