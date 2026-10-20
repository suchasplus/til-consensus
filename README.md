# til-consensus

`til-consensus` 是一个面向一次性论证、架构选择、代码裁决和多模型讨论的 CLI + Go library，并包含桌面客户端 [TCView](desktop/README.md)。它用确定性的 coordinator 编排多种 workflow，把过程和结论落成可审计的结构化结果。

默认 mode 是 `adjudication`：系统不会只选“哪篇完整答案看起来更像对的”，而是围绕 claim 做 challenge、verification、revision 和 adjudication。

## 桌面客户端 TCView

TCView 在同一个桌面聊天窗口内编排 Codex、Claude Code、Grok Build 和 AGY。支持三种讨论模式、话题自动选择、文本材料、模型与推理配置、停止恢复和可审计结果。界面与 CLI 共用本仓库的公开 Go 内核，详细操作见 [桌面说明](desktop/README.md)。

macOS 开发需要 Go 1.27.1，以及 [desktop/.nvmrc](desktop/.nvmrc) 固定的 Node.js 版本：

```bash
cd desktop
npm ci
npm start
```

真实讨论还需要至少三家已安装、已登录并启用的 CLI，会使用对应服务额度；CI 和默认自动化使用明确的 Mock，不调用真实模型。

根 `go.mod` 继续提供 `github.com/suchasplus/til-consensus` SDK 与 CLI。`desktop/` 保留独立 npm 项目和 Go host 子模块，由根 `go.work` 连接当前 checkout；克隆这一个仓库即可构建，无需兄弟目录或全局 Go workspace。[同仓开发、构建来源与发布边界](desktop/docs/monorepo.md)。

## 什么时候使用哪种模式

| Mode | 什么时候用 | 例子 |
| --- | --- | --- |
| `adjudication` | 已有明确、可证伪的主张，需要依据证据逐条判断成立、修订还是驳回 | 审查“这个 patch 已彻底修复竞态问题”：让 agent 拆出关键 claim，质疑边界条件，运行或复核验证，最后给出 claim 级裁决 |
| `free_debate` | 存在多个合理方案，需要不同立场公开交锋、互相回应，再通过投票和综合报告形成共识 | 讨论“团队应继续使用 polyrepo，还是渐进迁移到 monorepo”：让平台、业务、CI 等视角比较成本、风险和迁移边界 |
| `delphi` | 结论依赖专家判断或预测，希望通过匿名多轮评分降低权威偏见、从众效应和会议压力 | 在收益和风险都不确定时，为下一季度的候选项目排序：参与者独立评分，阅读匿名汇总后修订判断，直到分布稳定 |

简单说：能围绕证据判断“对不对”，用 `adjudication`；需要观点正面碰撞，用 `free_debate`；需要匿名估计逐轮收敛，用 `delphi`。更完整的选择指南见 [三种讨论模式](docs/modes.md)。

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

不知道该用哪种 mode 时，先分类：

```bash
til-consensus classify "monorepo 和 polyrepo 如何取舍？" --config ./til-consensus.yaml
til-consensus classify --file ./task.md --config ./til-consensus.yaml
cat ./task.md | til-consensus classify --stdin --config ./til-consensus.yaml
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
| `free_debate` | `til-consensus debate ...` | 多参与者充分碰撞观点，可选 rapporteur 合成，最后按支持分数投票形成共识 |
| `delphi` | `til-consensus delphi ...` | 匿名多轮评分和摘要收敛，降低权威偏见和从众效应 |

选择建议见 [三种讨论模式](docs/modes.md)。

如果问题还不够明确，`classify` 会返回 `needs_clarification` 或 `not_suitable`，并列出需要补充的信息，而不是强行推荐某个 mode。`needs_clarification` 还会给出用户补齐信息后大概率适合的 mode 及理由。默认分类 provider 是 `gemini-api`，可用 `--provider` / `--model` 覆盖。

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

- CLI：Codex、Claude Code、Gemini CLI、Antigravity CLI、Pi、generic。
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

## Go Library

如果要嵌入到其它 Go 项目中，可以直接导入根级包：

- `github.com/suchasplus/til-consensus/runner`
- `github.com/suchasplus/til-consensus/consensus`
- `github.com/suchasplus/til-consensus/config`
- `github.com/suchasplus/til-consensus/preflight`
- `github.com/suchasplus/til-consensus/telemetry`
- `github.com/suchasplus/til-consensus/doctor`
- `github.com/suchasplus/til-consensus/runtime`
- `github.com/suchasplus/til-consensus/store/memory`
- `github.com/suchasplus/til-consensus/observer`

推荐从 `runner.Executor` 起步；它支持 run/resume/replay/action，也支持 `Classify` 做任务路由。配置解释、provider preflight、telemetry 和 doctor 也都有公开包。嵌入示例见 [Go Library 嵌入](docs/library.md)。

## 常用命令

```bash
# 传统模板初始化
til-consensus config init --mode adjudication --provider-profile mock --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile pi --config ./til-consensus.yaml
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

- `result.json`：统一结果壳（含 `degradations` 降级清单）。
- `summary.md`：结论优先的人工阅读摘要。
- `ledger.jsonl`：append-only 审计账本。
- `ledger.root.json`：ledger hash chain 的最终 root checkpoint。
- `events.jsonl`：运行事件流。
- `artifacts/`：provider input/raw/failure、readiness、telemetry、manifest。

详细说明见 [输出产物](docs/outputs.md)。

## 开发与质量门禁

源码开发需要 Go `1.27.1` 或更高版本；质量门禁使用 `golangci-lint v2.13.2`。CI 的 Linux、macOS、Windows 和发布构建统一从 `go.mod` 读取 Go 版本。根 SDK／CLI 的 CI 显式使用 `GOWORK=off` 验证独立模块；桌面 CI 使用本仓库 workspace 验证同一 checkout 的集成。

本地提交前：

```bash
make pre-push
```

它会执行格式检查、覆盖率测试、coverage treemap、`go vet`、`golangci-lint` 和构建。覆盖率图会写到：

```text
tmp/coverage/coverage.svg
```

桌面入口为 `make desktop-install`、`make desktop-start`、`make desktop-check`、`make desktop-test` 和 `make desktop-package`；完整 Electron 回归与 macOS 开发包检查见 [同仓开发说明](desktop/docs/monorepo.md)。

更多见 [测试、CI 与发布](docs/testing.md)。

发布包示例：

```bash
make ci
make release-archive VERSION=v0.1.0 TARGET_GOOS=darwin TARGET_GOARCH=arm64 DIRTY=false
```

## 文档入口

- [TCView 桌面客户端](desktop/README.md)
- [同仓开发与桌面 CI](desktop/docs/monorepo.md)

- [快速开始](docs/quickstart.md)
- [三种讨论模式](docs/modes.md)
- [配置](docs/config.md)
- [Provider 配置与预检](docs/providers.md)
- [操作手册](docs/operations.md)
- [输出产物](docs/outputs.md)
- [测试、CI 与发布](docs/testing.md)
- [技术架构](docs/architecture.md)
- [配置与输入样例](docs/examples.md)

常用样例文件：

- [generic.config.yaml](docs/examples/generic.config.yaml)
- [codex.config.yaml](docs/examples/codex.config.yaml)
- [claude.config.yaml](docs/examples/claude.config.yaml)
- [gemini.config.yaml](docs/examples/gemini.config.yaml)
- [antigravity.config.yaml](docs/examples/antigravity.config.yaml)
- [pi.config.yaml](docs/examples/pi.config.yaml)
- [document-refinement.run.yaml](docs/examples/document-refinement.run.yaml)
- [coding-review.run.yaml](docs/examples/coding-review.run.yaml)
