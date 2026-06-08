# til-consensus 新手教程

下面是一份面向**第一次上手 `til-consensus`** 的详细教程。默认你已经在仓库根目录：

```bash
cd /Users/suchasplus/agentic/til-consensus
```

可配合阅读：

- [README.md](/Users/suchasplus/agentic/til-consensus/README.md)
- [docs/config.md](/Users/suchasplus/agentic/til-consensus/docs/config.md)
- [docs/view.md](/Users/suchasplus/agentic/til-consensus/docs/view.md)
- [docs/examples.md](/Users/suchasplus/agentic/til-consensus/docs/examples.md)

## 1. 先理解它是什么

`til-consensus` 不是普通问答 CLI。它是一个“多 agent / 多 workflow 的裁决器”：

- `adjudication`
  - 适合代码审查、patch 是否修好 bug、事实结论是否站得住。
- `free_debate`
  - 适合几个 CLI 自由辩论后再投票。
- `delphi`
  - 适合架构选择、方案收敛、文档完善这种匿名多轮判断。

对新手，先用 `adjudication` 就够了。

## 2. 安装与验证

先确认 Go/Make 环境正常，然后构建或安装：

```bash
make build
./bin/til-consensus --version
```

如果想安装到本机：

```bash
make install
til-consensus --version
```

默认会装到：

```bash
~/.local/bin/til-consensus
```

如果命令找不到，补一下 `PATH`：

```bash
export PATH="$HOME/.local/bin:$PATH"
```

## 3. 第一次跑通：用 mock 模板

这是最重要的一步。先不要接真实模型，先把流程跑通。

生成配置：

```bash
til-consensus config init --mode adjudication --provider-profile mock --config ./til-consensus.yaml
```

直接运行一个任务：

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --task "判断这个 patch 是否真正修复了竞态问题"
```

如果你的任务已经写在文件里，也可以直接读文件全文：

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --task-file ./task.md
```

`--task-file` 会把文件全部内容当作任务文本。它可以和 `--input ./run.yaml` 一起使用，用来覆盖 `task_spec.goal`；但不能和 `--task` 同时使用。

查看结果：

```bash
til-consensus view --config ./til-consensus.yaml
```

如果你想用浏览器看：

```bash
til-consensus view --config ./til-consensus.yaml --web
```

## 4. 它会生成什么文件

默认输出在：

```bash
./out/{requestId}/
```

这个路径按当前执行命令的目录解析，不按 `--config` 文件所在目录解析。比如你在仓库根目录执行：

```bash
til-consensus run --config docs/examples/codex.config.yaml --task "..."
```

默认输出仍然会落到仓库根目录的 `./out/{requestId}/`，而不是 `docs/examples/out/{requestId}/`。

里面最重要的是：

- `result.json`
  - 最终结构化结果
- `ledger.jsonl`
  - 证据账本，append-only
- `summary.md`
  - 人类可读摘要
- `artifacts/`
  - 原始输入输出、日志、follow-up 等

另外 session 会落到：

```bash
./out/_sessions/
```

## 5. 看懂第一次结果

最常用的查看方式：

```bash
til-consensus view --config ./til-consensus.yaml
```

如果只想看 claims 和 verification：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --section claims \
  --section verifications \
  --verbose
```

如果只想看 observation / follow-up：

```bash
til-consensus view \
  --config ./til-consensus.yaml \
  --section observations \
  --section followups \
  --verbose
```

浏览器模式会给你：

- Overview
- Claims
- Evidence
- Observations
- Follow-ups
- Debug
- Workflow
- Files

## 6. 配置文件默认放哪

如果你不传 `--config`，默认查找顺序是：

1. `./til-consensus.yaml`
2. `~/.config/til-consensus/default.yaml`
3. `~/.config/til-consensus/config.yaml`

所以你也可以这样初始化默认配置：

```bash
til-consensus config init --mode adjudication --provider-profile mock
```

它会默认写到：

```bash
~/.config/til-consensus/default.yaml
```

之后可以直接运行：

```bash
til-consensus run --task "Should we use a monorepo or polyrepo for our microservices?"
til-consensus view
```

## 7. 换成真实 provider

跑通 mock 之后，再接真实 CLI。

比如你用 Codex：

```bash
til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml
```

用 Claude：

```bash
til-consensus config init --mode adjudication --provider-profile claude --config ./til-consensus.yaml
```

用 Gemini：

```bash
til-consensus config init --mode adjudication --provider-profile gemini --config ./til-consensus.yaml
```

如果你有自己的脚本或代理，选 `generic`：

```bash
til-consensus config init --mode adjudication --provider-profile generic --config ./til-consensus.yaml
```

接真实 provider 后，先不要直接跑完整 workflow，先做一次 provider 预检：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
```

`profile preflight` 会真实调用 provider 做最小 JSON 探测，默认 `max_output_tokens=2048`。它只聚焦 provider / model / agent profile 是否可用，不要求 `roles.proposers / roles.challengers / participants` 已经完整可运行；如果你加 `--agent`，才会校验这个 agent 的 provider/model 引用。

如果同一份配置里有多个 provider，`--all` 会逐个探测并分块输出。最后的 `ready=x/y` 在真实终端中全部 ready 为绿色，否则为红色。

如果你想临时改变输出目录：

```bash
til-consensus profile preflight \
  --config ./til-consensus.yaml \
  --all \
  --output ./out/{requestId} \
  --verbose
```

完整样例在：

- [docs/examples/codex.config.yaml](/Users/suchasplus/agentic/til-consensus/docs/examples/codex.config.yaml)
- [docs/examples/claude.config.yaml](/Users/suchasplus/agentic/til-consensus/docs/examples/claude.config.yaml)
- [docs/examples/gemini.config.yaml](/Users/suchasplus/agentic/til-consensus/docs/examples/gemini.config.yaml)
- [docs/examples/generic.config.yaml](/Users/suchasplus/agentic/til-consensus/docs/examples/generic.config.yaml)

## 8. 怎么加 provider 和 agent

如果模板不够，再增量改配置。

加 provider：

```bash
til-consensus config add-provider --config ./til-consensus.yaml ...
```

加 agent：

```bash
til-consensus config add-agent \
  --config ./til-consensus.yaml \
  --id proposer-b \
  --provider codex-cli \
  --model default \
  --role proposer \
  --assign proposer
```

然后校验配置：

```bash
til-consensus config validate --config ./til-consensus.yaml
```

## 9. 三种 workflow 怎么用

默认 `adjudication`：

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --task "判断这个结论是否有足够证据支持"
```

自由辩论 `free_debate`：

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

匿名 Delphi：

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

## 10. 推荐的新手使用顺序

建议严格按这个顺序来：

1. `quickstart` 跑通一次
2. 学会 `view` 看结果
3. 再切 `codex/claude/gemini/generic`
4. 再尝试 `free_debate` 或 `delphi`
5. 再去用 `followup / resume / replay`

## 11. follow-up、resume、replay 是干什么的

如果 observation 发现矛盾证据，系统可能生成 follow-up case artifact。你可以直接继续跑：

```bash
til-consensus followup run --config ./til-consensus.yaml --artifact ./out/parent-run/artifacts/followups/case.json
```

或者：

```bash
til-consensus run --config ./til-consensus.yaml --followup ./out/parent-run/artifacts/followups/case.json
```

如果你想从历史 session 继续：

```bash
til-consensus session list --config ./til-consensus.yaml
til-consensus session show --config ./til-consensus.yaml --session-id session_xxx
til-consensus run --config ./til-consensus.yaml --resume-session session_xxx
til-consensus run --config ./til-consensus.yaml --replay-session session_xxx
```

## 12. 最实用的几个命令

日常最常用就这些：

```bash
til-consensus --version
til-consensus config init --mode adjudication --provider-profile mock --config ./til-consensus.yaml
til-consensus config validate --config ./til-consensus.yaml
til-consensus run --config ./til-consensus.yaml --task "..."
til-consensus view --config ./til-consensus.yaml
til-consensus view --config ./til-consensus.yaml --web
til-consensus session list --config ./til-consensus.yaml
```

## 13. 什么时候用哪个 mode / provider-profile

- `--mode adjudication --provider-profile mock`
  - 第一次跑通流程
- `--mode adjudication --provider-profile codex|claude|gemini`
  - 你已经装了对应 CLI
- `--mode adjudication --provider-profile generic`
  - 你有自定义脚本或代理
- `--mode free-debate --provider-profile mock`
  - 你要做自由辩论 workflow
- `--mode delphi --provider-profile mock`
  - 你要做匿名收敛 workflow
- `--mode adjudication --provider-profile mock --task-profile coding`
  - 你主要是代码仓库验证、benchmark、tests、git diff

旧的 `--preset` 还可用，但现在只是兼容别名，不再是主概念。

provider profile 的当前默认模型：

- `codex`
  - `gpt-5.4`
- `claude`
  - `claude-opus-4-6`
- `gemini`
  - `gemini-3.1-pro-preview`

## 14. 出错时先看哪里

先看这几个地方：

- `til-consensus view --verbose`
- `./out/{requestId}/ledger.jsonl`
- `./out/{requestId}/artifacts/`
- `./out/_sessions/`
- provider 失败审计文件：
  - `failure-<agent>-<task>-<taskID>.json`
  - `input-<agent>-<task>-<taskID>.json`

如果要看运行期事件和原始模型 verdict，建议：

```bash
til-consensus run --config ./codex.yaml --task "..." --verbose --debug
til-consensus view --config ./codex.yaml --section debug --verbose
til-consensus view --config ./codex.yaml --web --section debug --verbose --open
```

其中 Debug 区块现在会直接显示：

- `rawVerdict`
- `rawTaskVerdict`

## 15. 最后给你一个最短上手路径

如果你只想现在立刻成功一次，就执行这三条：

```bash
til-consensus config init --mode adjudication --provider-profile mock --config ./til-consensus.yaml
til-consensus run --config ./til-consensus.yaml --task "判断这个 patch 是否真正修复了竞态问题"
til-consensus view --config ./til-consensus.yaml --web --open
```

如果你愿意，下一步可以继续补两份更具体的教程：

1. “文档完善 / 架构选择”专用上手教程
2. “代码审查 / patch 验证”专用上手教程
