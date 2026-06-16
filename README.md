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
til-consensus ask "判断这个 patch 是否真正修复了竞态问题" --config ./til-consensus.yaml
```

如果任务文本已经写在文件里，也可以直接读取整份文本内容：

```bash
til-consensus ask ./task.md --config ./til-consensus.yaml
```

`--task-file` 会读取文件全部内容作为任务文本。它可以和 `--input` 一起用，用来覆盖 `run.yaml` 里的 `task_spec.goal`；但不能和 `--task` 同时使用。

底层 `run` 仍然保留，适合 CI 或需要精确覆盖所有角色 / policy 的场景：

```bash
til-consensus run --config ./til-consensus.yaml --task-file ./task.md
```

如果你想把最近一天的真实 run 汇总成 markdown：

```bash
til-consensus telemetry daily --config ./til-consensus.yaml
```

也可以直接指定扫描根目录和输出文件：

```bash
til-consensus telemetry daily \
  --root ./logs/out \
  --since 24h \
  --output ./reports/daily-telemetry.md
```

3. 查看最新一次结果：

```bash
til-consensus last --config ./til-consensus.yaml
```

也可以直接启动本地只读 Web viewer：

```bash
til-consensus open --config ./til-consensus.yaml
```

如果希望启动后自动打开默认浏览器：

```bash
til-consensus view --config ./til-consensus.yaml --web --open
```

## 高频短命令

为了避免日常命令过长，顶层提供 task-first 快捷入口：

```bash
til-consensus ask "是否应该合并这个 patch？"
til-consensus debate "monorepo 和 polyrepo 如何取舍？"
til-consensus delphi ./docs/decision.md
```

规则：

- `ask` 固定使用 `adjudication`
- `debate` 固定使用 `free_debate`
- `delphi` 固定使用 `delphi`
- 第一个位置参数如果是存在的文件，会按 `--task-file` 读取全文；否则按一段任务文本处理
- `debate` / `delphi` 没有显式 `--participants` 时，会优先使用配置里的 `roles.participants`，否则从 proposer/challenger 去重推导

查看相关也有快捷入口：

```bash
til-consensus last --config ./til-consensus.yaml
til-consensus inspect tc_xxx --config ./til-consensus.yaml
til-consensus logs tc_xxx --config ./til-consensus.yaml --type raw
til-consensus open tc_xxx --config ./til-consensus.yaml
```

这些命令只是 `run`、`view`、`artifact` 的薄包装；底层正交命令仍然保留给脚本和调试。

如果你已经装了具体 CLI，也可以直接生成对应模板：

```bash
til-consensus config init --mode adjudication --provider-profile codex --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile claude --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile gemini --config ./til-consensus.yaml
til-consensus config init --mode adjudication --provider-profile generic --config ./til-consensus.yaml
```

默认输出会写到当前执行目录下的 `./out/{requestId}/`。相对 `output.directory` 按当前执行目录解析，而不是按配置文件所在目录解析。最重要的文件是：

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

如果你不想维护一份很大的 YAML，可以直接生成 split config：

```bash
til-consensus setup --mode adjudication --provider-profile mock --dir .
```

它会写出：

- `til-consensus.yaml`
- `conf/providers.yaml`
- `conf/profiles.yaml`

主配置只保留 `include`、`profile` 和 `output`；provider、agent、roles 和 policy 放在片段里。等价入口是：

```bash
til-consensus config wizard --mode delphi --provider-profile claude --dir .
```

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

其中 provider profile 的当前默认模型是：

- `codex`
  - `gpt-5.4`
- `claude`
  - `claude-opus-4-6`
- `gemini`
  - `gemini-3.1-pro-preview`

常见可复制样例：

- [provider 配置与 `run.yaml` 示例](docs/examples.md)
- [generic 组合包](docs/examples/generic.config.yaml)
- [codex 组合包](docs/examples/codex.config.yaml)
- [claude 组合包](docs/examples/claude.config.yaml)
- [gemini 组合包](docs/examples/gemini.config.yaml)
- [openai-compatible API 组合包](docs/examples/openai-compatible.config.yaml)
- [anthropic-compatible API 组合包](docs/examples/anthropic-compatible.config.yaml)
- [gemini API 组合包](docs/examples/gemini-api.config.yaml)
- [OpenRouter 组合包](docs/examples/openrouter.config.yaml)
- [Kimi 组合包](docs/examples/kimi.config.yaml)
- [DeepSeek 组合包](docs/examples/deepseek.config.yaml)
- [Qwen Max 百炼组合包](docs/examples/qwen-max.config.yaml)
- [文档完善输入样例](docs/examples/document-refinement.run.yaml)
- [架构选择输入样例](docs/examples/architecture-decision.run.yaml)
- [coding review 输入样例](docs/examples/coding-review.run.yaml)
- [事实冲突输入样例](docs/examples/factual-conflict.run.yaml)

如果模板起步不够用，再用下面两个命令做增量修改：

- `til-consensus config add-provider`
- `til-consensus config add-agent`

它们的定位是“在模板基础上补 provider / agent”，不是第一次上手的首选入口。

`config add-provider --protocol` 现在支持三种 API 协议：

- `openai-compatible`
- `anthropic-compatible`
- `gemini-api`

例如：

```bash
til-consensus config add-provider \
  --config ./til-consensus.yaml \
  --id gemini-api \
  --type api \
  --protocol gemini-api \
  --base-url https://generativelanguage.googleapis.com/v1beta \
  --api-key-env GEMINI_API_KEY \
  --model-id default \
  --provider-model gemini-3.5-flash
```

如果你要接兼容网关，当前推荐这样理解：

- `OpenAI 官方 / OpenRouter / Kimi / DeepSeek / Qwen 百炼兼容模式 / 其他 OpenAI 风格网关`
  - 用 `openai-compatible`
- `Anthropic 官方 / 其他 Anthropic 风格网关`
  - 用 `anthropic-compatible`
- `Gemini 官方 generateContent`
  - 用 `gemini-api`

这三种 API provider 现在都支持细配：

- `base_url`
- `api_key_env`
- `models.<id>.provider_model`
- `models.<id>.max_output_tokens`
- `models.<id>.temperature`
- `headers`
- `options`

`options` 里当前最有用的键：

- 通用
  - `endpoint_path`
  - `structured_output_mode`
  - `api_key_header`
  - `api_key_prefix`
  - `api_key_query_param`
  - `extra_body`
  - `timeout_ms`
- `openai-compatible`
  - `max_output_tokens_field`
  - `reasoning_field`
  - `response_format_name`
- `anthropic-compatible`
  - `anthropic_version`
- `gemini-api`
  - `response_mime_type`
  - `response_schema_field`

## 常用命令

- `til-consensus run`
  - 运行一次 workflow
  - 加 `--dry-run` 时只解析最终 plan，不调用 provider，不写运行产物
- `til-consensus ask|debate|delphi`
  - 面向任务的短命令，分别映射到三种 workflow
- `til-consensus view`
  - 用终端友好的方式阅读结果
- `til-consensus last|inspect|logs|open`
  - 高频查看短命令，分别用于最近结果、指定 run、raw/debug artifact、Web viewer
- `til-consensus doctor`
  - 检查配置、输出目录、API key 环境变量和 CLI 二进制；加 `--providers` 才真实调用 provider
- `til-consensus artifact list/show`
  - 快速定位和展开某次 run 的 raw/input/error/telemetry artifact
- `til-consensus setup`
  - 生成 split config 起步骨架
- `til-consensus act`
  - 基于现有 `result.json` 继续执行 action
- `til-consensus config init`
  - 生成带注释的配置模板
- `til-consensus config validate`
  - 校验配置是否可用
- `til-consensus config render`
  - 展开 include/overlay 后输出最终配置
- `til-consensus config explain`
  - 解释最终生效的 provider、agent、roles 和输出路径
- `til-consensus profile preflight`
  - 真实调用配置里的 CLI/API provider，检查最小非交互 JSON 输出，并写出 readiness artifact
- `til-consensus telemetry daily`
  - 扫描运行目录，输出 readiness / compliance / workflow 的 markdown 汇总
- `til-consensus followup run`
  - 执行 follow-up case artifact
- `til-consensus session list`
  - 按 request/session 查看持久化快照

## `profile preflight` 示例

`config validate` 检查完整 workflow 配置；`profile preflight` 只聚焦 provider / agent profile，会真实调用 provider，适合手动确认 CLI 登录态、API key、base url 和模型名是否可用。

preflight 会发起一次最小非交互 JSON 探测，要求 provider 返回 `{"ok": true}`。探测默认使用 `max_output_tokens=2048`；如果对应 model 配置了更小的 `max_output_tokens`，则尊重配置值。这个预算只影响 preflight，不会改正常 `run` 的输出预算。

因此，`profile preflight` 默认不要求 `roles.proposers / roles.challengers / participants` 等 workflow 角色完整；只要 `providers` 层级合法，且被指定的 `agents` 能正确引用 provider/model，就可以执行探测。要检查完整运行配置，仍然使用 `til-consensus config validate`。

多个 provider 会逐个探测并分块输出：每个 provider 完成后立即打印该 provider 的 readiness，最后再打印 `profile preflight completed ready=x/y` 和 artifact 路径。stdout 是真实终端时，最终 summary 全部 ready 会显示为绿色，否则显示为红色。

检查配置里的所有 provider：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --verbose
```

相对 `output.directory` 会按当前执行目录解析，而不是按配置文件所在目录解析。也可以临时覆盖输出目录：

```bash
til-consensus profile preflight \
  --config docs/examples/deepseek.config.yaml \
  --provider deepseek-api \
  --output ./out/{requestId} \
  --verbose
```

只检查某个 provider 或 agent：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --provider deepseek-api
til-consensus profile preflight --config ./til-consensus.yaml --agent arbiter-qwen-max
```

完成后直接打开本地 Web viewer：

```bash
til-consensus profile preflight --config ./til-consensus.yaml --all --web --open
```

preflight 会写出标准运行目录，最关键的是：

- `result.json`
  - 可用 `til-consensus view --result ...` 打开
- `artifacts/provider-readiness.json`
  - 记录 `ready / strictJSON / recoverableJSON / durationMs / error`
- `summary.md`
  - 人可读的 readiness 摘要

如果 Gemini 等 thinking 模型返回 `gemini response contains no text parts ... finishReason=MAX_TOKENS`，通常是思考阶段消耗了输出预算；提高该模型的 `max_output_tokens`，或按目标网关支持情况降低/关闭 thinking。

## `run` 示例

默认 `adjudication`：

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --task "判断这个 patch 是否真正修复了竞态问题"
```

只检查最终会怎么跑，不调用 provider、不创建 `out/`：

```bash
til-consensus run \
  --config ./til-consensus.yaml \
  --task-file ./task.md \
  --dry-run

til-consensus run \
  --config ./til-consensus.yaml \
  --input ./case.run.yaml \
  --dry-run \
  --format json
```

## CLI 诊断与审计

快速检查本机配置，不花 token：

```bash
til-consensus doctor --config ./til-consensus.yaml
```

连真实 provider 一起检查：

```bash
til-consensus doctor --config ./til-consensus.yaml --providers --verbose
```

查看 include/overlay 后的最终配置：

```bash
til-consensus config render --config ./til-consensus.yaml
til-consensus config render --config ./til-consensus.yaml --format json
```

解释最终角色和 provider/model 映射：

```bash
til-consensus config explain --config ./til-consensus.yaml
til-consensus config explain --config ./til-consensus.yaml --agent arbiter-a
```

列出并查看 artifact：

```bash
til-consensus artifact list --config ./til-consensus.yaml --type error
til-consensus artifact show --result ./out/tc_xxx/result.json --id 1
til-consensus artifact show --result ./out/tc_xxx/result.json --path artifacts/run-telemetry.json
```

## Exit code

CLI 会把常见失败映射成稳定 exit code：

- `0` success
- `1` internal_error
- `2` usage_error
- `3` config_not_found
- `4` config_invalid
- `5` input_invalid
- `6` provider_not_ready
- `7` provider_auth_failed
- `8` provider_rate_limited
- `9` provider_timeout
- `10` provider_schema_failed
- `11` run_failed
- `12` run_cancelled
- `13` artifact_not_found
- `14` artifact_invalid

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

排查真实 provider 的结构化输出时，建议直接开两级日志：

```bash
til-consensus run \
  --config ./codex.yaml \
  --task "Should we use a monorepo or polyrepo for our microservices?" \
  --verbose \
  --debug
```

其中：

- `--verbose`
  - 打印业务级摘要
  - 包括 phase summary、claim revised、claim adjudicated、observation recorded
- `--debug`
  - 在 `--verbose` 基础上，再打印完整 payload 和 provider artifact 路径提示

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

如果要直接在页面里看运行期 debug 事件和原始 verdict 词，也可以：

```bash
til-consensus view --config ./til-consensus.yaml --web --section debug --verbose --open
```

`debug` 区块现在除了运行事件，还会直接展示三层 telemetry：

- `Provider Readiness`
  - 当前上下文里各 provider 的最小非交互探测结果
  - 显示 `ready / strictJSON / recoverableJSON / duration / error`
- `Run Summary`
  - 当前 run 的聚合业务质量
  - 显示 `primaryResult / taskVerdict / terminalState / workflow summary / task summary`
- `Strict Compliance`
  - 单 task 的结构化输出合规度

- summary
  - 按 `provider × taskKind` 汇总
  - 显示 `strict / normalized / repaired / failed`
- reports
  - 每个 task 一条
  - 显示最终状态、是否触发 repair，以及相关 artifact 路径

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
- `artifacts/strict-compliance-summary.json`
  - strict compliance 汇总
  - 按 `provider / providerModel / taskKind` 聚合
- `artifacts/provider-readiness.json`
  - 真实 provider 预检结果
- `artifacts/run-telemetry.json`
  - 当前 run 的 workflow 级聚合统计
- `artifacts/compliance-report-*.json`
  - 单个 task 的 compliance 报告
  - 会标出本次是：
    - `strict`
    - `normalized`
    - `repaired`
    - `failed`

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
- `make pre-push`
  - 推送前的快速质量门禁
  - 执行格式检查、单元测试、`go vet`、`golangci-lint run` 和本地构建
- `make install-git-hooks`
  - 安装仓库内置 Git `pre-push` hook
  - 安装后每次 `git push` 前会自动运行 `make pre-push`
- `make test-e2e`
  - 执行 CLI 端到端测试矩阵
- `make test-e2e-real`
  - 执行真实 CLI provider 预检与 E2E 矩阵
- `make test-e2e-real-api`
  - 执行真实 API provider 预检与 E2E 矩阵
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

## 运行期日志

`run` / `followup run` 的实时终端日志现在支持两级详细度：

- 默认
  - phase 变化
  - task dispatched / retrying / failed
- `--verbose`
  - task completed
  - phase completed
  - claim revised
  - claim adjudicated
  - observation recorded
- `--debug`
  - 在 `--verbose` 基础上
  - 再打印完整事件 payload
  - 再打印 `input/raw/failure` artifact 路径提示

运行时还会同步记录 strict compliance telemetry：

- `strict`
  - 原始输出直接满足 schema 和 task 校验
- `normalized`
  - 只靠无歧义的类型转换后通过
  - 例如 `"0.8" -> 0.8`
- `repaired`
  - 首次 decode 失败后，经过同源 provider repair retry 修复成功
- `failed`
  - strict、normalize、repair 都失败

真实 CLI E2E 或 daily run 还会额外产出：

- `provider-readiness.json`
  - provider 是否 ready
  - strict JSON / recoverable JSON
  - 最小调用耗时
- `run-telemetry.json`
  - 这次 run 的整体质量
  - 包括 `workflow summary / verification summary / task summary`

如果输出连接到真实终端，关键字会自动着色；如果输出被重定向到文件，则保持纯文本。

可用环境变量：

- `NO_COLOR=1`
  - 关闭终端彩色输出
- `FORCE_COLOR=1`
  - 强制开启终端彩色输出

`view` 的终端文本输出现在也会使用同一套着色策略，但只对：

- `view --format text`

生效；如果你输出的是：

- `markdown`
- `json`

则保持纯文本，不插入 ANSI 颜色码。

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

如果想在推送前自动拦截 `golangci-lint` / `staticcheck` 这类问题，先安装本地 hook：

```bash
make install-git-hooks
```

之后每次 `git push` 前会自动运行：

```bash
make pre-push
```

`make pre-push` 比 `make ci` 快，不跑 race 测试；它用于日常提交前拦截格式、单测、`go vet`、`golangci-lint` 和构建问题。需要完全对齐 GitHub Actions 时仍然运行 `make ci`。

本地模拟单个平台发布：

```bash
make release-archive VERSION=v0.1.0 TARGET_GOOS=darwin TARGET_GOARCH=arm64 DIRTY=false
```

更完整说明见：

- [E2E 测试设计](docs/e2e.md)
- [CI 与发布](docs/release.md)
- [三种 workflow 与状态机](docs/workflow.md)

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

- `input-<agent>-<task>-<taskID>.json`
  - provider 实际收到的结构化任务输入
- `failure-<agent>-<task>-<taskID>.json`
  - provider 执行失败时的分类结果
  - 会带 `class`、`message`、可选 `statusCode`
- `raw-<agent>-<task>-<taskID>.*`
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
