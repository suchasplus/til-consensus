# til-consensus

`til-consensus` 是一个面向一次性论证、架构选择和代码裁决任务的 CLI。它用确定性的 coordinator 编排多种工作流，把过程和结论落成可审计的本地产物。

默认 `adjudication` 是 claim-centric 的：系统不会只选“哪篇完整答案看起来更像对的”，而是围绕 claim 做 challenge、verification、revision 和 adjudication。

如果你只想先跑通一次，先看下面的 quickstart 即可。更完整的 provider 配置和 `run.yaml` 样例在 [docs/examples.md](docs/examples.md)。

当前支持 3 种 workflow：

- `adjudication`
  - 裁决式流程
  - `frame -> ingest -> propose -> challenge -> verify -> revise -> adjudicate -> report -> action -> observe`
  - 适合 patch、benchmark、设计结论是否成立
- `free_debate`
  - 多轮自由辩论
  - `initial -> debate* -> final_vote -> report -> action`
  - 适合多 CLI 交叉讨论，再做最终投票
- `delphi`
  - 匿名 Delphi
  - 多轮匿名问卷、聚合摘要、修订评分，直到收敛或达到轮数上限
  - 适合架构选择、方案取舍、文档结论收敛

## 5 分钟快速开始

1. 生成一份可直接运行的配置：

```bash
til-consensus config init --mode adjudication --provider-profile mock --config ./til-consensus.yaml
```

2. 运行一次默认 `adjudication`：

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --task "判断这个 patch 是否真正修复了竞态问题"
```

3. 查看最新一次结果：

```bash
til-consensus view --config ./til-consensus.yaml
```

也可以直接启动本地只读 Web viewer：

```bash
til-consensus view --config ./til-consensus.yaml --web
```

如果你已经装了具体 CLI，也可以直接生成对应模板：

```bash
til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile claude --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile gemini --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile generic --config ./til-consensus.yaml
```

默认输出会写到 `./out/{requestId}/`。最重要的文件是：

- `result.json`
  - 统一结果壳，包含 `mode` 和对应的 mode-specific section
- `ledger.jsonl`
  - append-only 证据账本
- `summary.md`
  - 适合快速阅读的摘要
- `artifacts/`
  - 原始 worker 输出、命令日志、diff、benchmark 结果

同时还会把 session snapshot 持久化到：

- `./out/_sessions/`

`adjudication` 结果里最关键的新对象有：

- `caseManifest`
- `claimGraph`
- `challengeTickets`
- `verificationResults`
- `revisionRecords`
- `adjudicationRecords`
- `observations`

默认情况下，单个 task 在超时或失败后会自动重试 1 次。

如果你希望裁决后自动补抓新证据或做真实外部观测，可以在配置里加入：

```yaml
defaults:
  fallback_policy:
    max_fallback_rounds: 1
    on_insufficient_evidence: ingest
    on_unresolved_claims: revise
    on_keep_with_caveat: revise
  ingest_policy:
    sources:
      - name: fresh-evidence
        command: sh
        args: ["-c", "printf fresh-evidence"]
  observe_policy:
    on_contradiction: reopen
    sources:
      - name: health
        command: sh
        args:
          - -c
          - printf '{"status":{"ok":true},"report":{"summary":"healthy","excerpt":"post-action checks are healthy"}}'
        parsing:
          mode: json
          success_path: status.ok
          summary_path: report.summary
          excerpt_path: report.excerpt
```

## 三种模式怎么选

- `adjudication`
  - 目标是高置信度裁决
  - 支持 verifier、revise 和 claim-level arbiter
  - 默认 mode
- `free_debate`
  - 目标是让多个 participant 先充分讨论，再对 active claims 做 final vote
  - 不走当前 verifier / arbiter
- `delphi`
  - 目标是匿名多轮收敛
  - 不暴露 participant 身份给其他 participant
  - 不走 arbiter，最终结论来自 Delphi 汇总结果

## 配置模板

`config init` 现在按 3 个正交维度生成模板：

- `--mode`
  - `adjudication`
  - `free-debate`
  - `delphi`
- `--provider-profile`
  - `mock`
  - `openai`
  - `generic`
  - `codex`
  - `claude`
  - `gemini`
- `--task-profile`
  - `general`
  - `coding`

最常用的组合：

- 第一次跑通 CLI：
  - `--mode adjudication --provider-profile mock`
- 接 Codex CLI：
  - `--mode adjudication --provider-profile codex`
- 接 Claude CLI：
  - `--mode adjudication --provider-profile claude`
- 接 Gemini CLI：
  - `--mode adjudication --provider-profile gemini`
- 做代码裁决：
  - `--mode adjudication --provider-profile mock --task-profile coding`
- 做自由辩论：
  - `--mode free-debate --provider-profile mock`
- 做 Delphi：
  - `--mode delphi --provider-profile mock`

最常用的初始化方式：

```bash
til-consensus config init --mode adjudication --provider-profile mock --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile mock --task-profile coding --stdout
til-consensus config init --mode free-debate --provider-profile mock --stdout
til-consensus config init --mode delphi --provider-profile mock --config ./til-consensus.yaml --force
```

旧的 `--preset` 仍然可用，但现在只是兼容别名：

- `quickstart` = `--mode adjudication --provider-profile mock`
- `coding` = `--mode adjudication --provider-profile mock --task-profile coding`
- `debate` = `--mode free-debate --provider-profile mock`
- `delphi` = `--mode delphi --provider-profile mock`
- `codex|claude|gemini|generic|openai` = `--mode adjudication --provider-profile <对应值>`

常见可复制样例：

- [provider 配置与 `run.yaml` 示例](docs/examples.md)
- [generic 组合包](docs/examples/generic.config.yaml)
- [codex 组合包](docs/examples/codex.config.yaml)
- [claude 组合包](docs/examples/claude.config.yaml)
- [gemini 组合包](docs/examples/gemini.config.yaml)
- [文档完善输入样例](docs/examples/document-refinement.run.yaml)
- [架构选择输入样例](docs/examples/architecture-decision.run.yaml)
- [coding review 输入样例](docs/examples/coding-review.run.yaml)
- [事实冲突输入样例](docs/examples/factual-conflict.run.yaml)

如果模板起步不够用，再用下面两个命令做增量修改：

- `til-consensus config add-provider`
- `til-consensus config add-agent`

它们的定位是“在模板基础上补 provider / agent”，不是第一次上手的首选入口。

## 常用命令

- `til-consensus run`
  - 运行一次 workflow
- `til-consensus view`
  - 用终端友好的方式阅读结果
- `til-consensus act`
  - 基于现有 `result.json` 继续执行 action
- `til-consensus config init`
  - 生成带注释的配置模板
- `til-consensus config validate`
  - 校验配置是否可用
- `til-consensus followup run`
  - 执行 follow-up case artifact
- `til-consensus session list`
  - 按 request/session 查看持久化快照

## `run` 示例

默认 `adjudication`：

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --task "判断这个 patch 是否真正修复了竞态问题"
```

运行 `free_debate`：

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --mode free-debate \
  --participants debater-a,debater-b,debater-c \
  --min-rounds 2 \
  --max-rounds 3 \
  --vote-threshold 0.75 \
  --task "Should we use a monorepo or polyrepo for our microservices?"
```

运行 `delphi`：

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --mode delphi \
  --participants participant-a,participant-b,participant-c \
  --facilitator facilitator-a \
  --min-rounds 2 \
  --max-rounds 4 \
  --convergence-threshold 0.8 \
  --task "评估未来 6 个月内是否应将当前单体服务演进为事件驱动架构"
```

## `view` 示例

查看最新一次：

```bash
til-consensus view --config ./til-consensus.yaml
```

按模式常用 section：

- adjudication：

```bash
til-consensus view --config ./til-consensus.yaml --section claims --section verifications
```

看 observation / follow-up：

```bash
til-consensus view --config ./til-consensus.yaml --section observations --section followups --verbose
```

同样的结果也可以用浏览器 viewer 看：

```bash
til-consensus view --config ./til-consensus.yaml --web --section observations --section followups --verbose
```

- free_debate：

```bash
til-consensus view --config ./til-consensus.yaml --section rounds --section votes
```

- delphi：

```bash
til-consensus view --config ./til-consensus.yaml --section rounds --section convergence
```

## 输出产物

一次完整运行默认会写出：

- `result.json`
  - 顶层统一字段：
    - `schemaVersion`
    - `mode`
    - `requestId`
    - `sessionId`
    - `taskSpec`
    - `caseManifest`
    - `terminalState`
    - `report`
    - `action`
    - `observations`
    - `metrics`
    - `error`
  - 并按 `mode` 挂载：
    - `adjudication`
    - `freeDebate`
    - `delphi`
- `ledger.jsonl`
  - 证据账本
  - 每条记录单调递增，便于审计
- `summary.md`
  - 人可读摘要
- `artifacts/manifest.jsonl`
  - artifact 与 ledger entry 的反向索引

## 构建与安装

仓库根目录已经带了常用 `Makefile` target：

- `make build`
  - 本地构建到 `./bin/til-consensus`
- `make build-debug`
  - 生成便于调试的 `./bin/til-consensus-debug`
- `make build-release`
  - 生成发布版 `./dist/til-consensus`
- `make install`
  - 安装到本机
  - 在 macOS 下默认安装到 `~/.local/bin/til-consensus`
- `make run ARGS="..."`
  - 直接运行 CLI
- `make cover`
  - 生成并打印单元测试覆盖率
- `make test-e2e`
  - 执行 CLI 端到端测试矩阵
- `make ci`
  - 本地对齐 GitHub CI 的质量门禁
- `make release-archive`
  - 生成单个平台的发布压缩包

第一次本地安装常用命令：

```bash
make install
```

如果运行命令时不显式传 `--config`，当前默认查找顺序是：

1. 当前目录下的 `./til-consensus.yaml`
2. `~/.config/til-consensus/default.yaml`
3. `~/.config/til-consensus/config.yaml`

`til-consensus config init` 在不传 `--config` 时，也会默认写到 `~/.config/til-consensus/default.yaml`。

如果 `~/.local/bin` 还没进 `PATH`，可以在 shell 配置里补上：

```bash
export PATH="$HOME/.local/bin:$PATH"
```

也可以指定自定义安装目录：

```bash
make install INSTALL_DIR=/usr/local/bin
```

## CI 与发布

仓库使用 GitHub Actions 做两条流水线：

- `ci`
  - PR / push 到 `main` 时触发
  - 运行：
    - `gofmt -l .`
    - `go test ./...`
    - `go test -race ./...`
    - `go vet ./...`
    - `golangci-lint run`
    - `make build`
- `release`
  - 推送 `v*` tag 时触发
  - 产出：
    - `linux/amd64`
    - `linux/arm64`
    - `darwin/amd64`
    - `darwin/arm64`
    的发布压缩包和 `checksums.txt`

本地可直接对齐 CI：

```bash
make ci
```

本地模拟单个平台发布：

```bash
make release-archive VERSION=v0.1.0 TARGET_GOOS=darwin TARGET_GOARCH=arm64 DIRTY=false
```

更完整说明见：

- [E2E 测试设计](docs/e2e.md)
- [CI 与发布](docs/release.md)

## follow-up / observe / structured parsing

如果 `observe_policy.sources` 发现新的矛盾证据，系统会：

1. 把当前 run 的 terminal state 升级为 `requires_human_review`
2. 在 `artifacts/followups/` 下生成真实的 follow-up case JSON
3. 在 `observations` 和 `ledger.jsonl` 里挂上 child request / artifact 关系

然后你可以直接执行：

```bash
til-consensus followup run --config ./til-consensus.yaml --artifact ./out/parent-run/artifacts/followups/case.json
```

或者：

```bash
til-consensus run --config ./til-consensus.yaml --followup ./out/parent-run/artifacts/followups/case.json
```

如果你想基于历史 session 继续处理：

```bash
til-consensus run --config ./til-consensus.yaml --resume-session session_xxx
til-consensus run --config ./til-consensus.yaml --replay-session session_xxx
til-consensus session list --config ./til-consensus.yaml --request-id tc_xxx
til-consensus session show --config ./til-consensus.yaml --session-id session_xxx
```

说明：

- `run --resume-session`
  - 会读取 `_sessions/` 里的 checkpoint snapshot
  - 对 `adjudication` 模式执行 phase 级恢复，而不是从头重跑整条 request
- `run --replay-session`
  - 会基于历史 request 生成新的 child run
  - 适合做“同一输入重新审理”，并保留 parent/child lineage

provider 执行过程中还会把关键审计文件落到 `artifacts/`：

- `input-<agent>-<task>.json`
  - provider 实际收到的结构化任务输入
- `failure-<agent>-<task>.json`
  - provider 执行失败时的分类结果
  - 会带 `class`、`message`、可选 `statusCode`
- `raw-<agent>-<task>.*`
  - provider 的原始输出或 parse error 原文

外部源解析现在支持：

- `text`
- `json`
- `yaml`
- `xml`

并支持：

- `required_paths`
- `items[0].name`
- `items[*].name`

更完整的 provider 配置和 `run.yaml` 样例见：

- [配置与输入样例](docs/examples.md)

## 版本信息

构建时会通过 `ldflags` 注入：

- `version`
- `commit`
- `build time`
- `dirty`

## 深入阅读

- [文档首页](docs/index.md)
- [工作流与状态机](docs/workflow.md)
- [多工作流技术设计](docs/rewrite.md)

查看方式：

```bash
til-consensus --version
til-consensus version
```

其中：

- `--version`
  - 适合快速看版本号
- `version`
  - 会输出完整构建信息，包括 commit、构建时间和 dirty 状态

## 深入阅读

- [文档首页](docs/index.md)
- [配置说明](docs/config.md)
- [输出产物说明](docs/output.md)
- [CI 与发布](docs/release.md)
- [终端 view 用法](docs/view.md)
- [浏览器 Viewer](docs/viewer.md)
- [多工作流技术设计](docs/rewrite.md)
