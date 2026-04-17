# E2E 测试设计

本文描述 `til-consensus` 当前的端到端测试分层、覆盖矩阵和执行方式。

## 目标

E2E 关注的不是单个函数是否正确，而是用户真实命令链路是否仍然可用：

- `config init`
- `config validate`
- `run`
- `view`
- `view --web`
- `followup run`
- `run --followup`
- `session list`
- `session show`
- `run --resume-session`
- `run --replay-session`

同时要覆盖 3 类 workflow：

- `adjudication`
- `free_debate`
- `delphi`

## 分层

当前 E2E 分成两层。

### 1. 命令链路层

目的：验证 CLI 入口、默认路径、输出产物和主要子命令之间的串联是否可用。

重点覆盖：

- `quickstart` 配置初始化后能否直接 `run`
  - 当前默认用法是 `config init --mode adjudication --provider-profile mock`
- `view` 是否能正确读取最新 run
- `view --web` 是否能启动本地只读服务并返回正确的 `Document`
- `resume / replay / followup / session` 是否能串起来

### 2. 场景夹具层

目的：用真实 `run.yaml` 和工作区夹具验证 workflow 语义、外部源解析和 follow-up 行为。

当前优先覆盖：

- `coding-composite`
- `factual-conflict`
- `fallback-reversal`
- `observe-negates-action`

这些测试会：

- 把 `testdata/scenarios/<name>` 复制到临时目录
- 生成一份最小可运行的 mock 配置
- 真正调用 CLI `run`
- 读取生成的 `summary.md`
- 校验 `expected-run-summary.txt` 或 `expected-summary.txt` 里的关键片段

## 当前矩阵

### 命令链路

- `TestE2EQuickstartCommandChainAndWeb`
  - `config init --mode adjudication --provider-profile mock`
  - `config validate`
  - `run`
  - `view`
  - `view --web`
- `TestE2EResumeSessionFromCheckpoint`
  - 失败 run 生成 checkpoint
  - `session list`
  - `session show`
  - `run --resume-session`
  - `run --replay-session`
- `TestE2EFollowupAndObservationSections`
  - 生成 follow-up artifact
  - `run --followup`
  - `followup run`
  - `view --section observations --section followups`
- `TestE2EMultiModeSmoke`
  - `free_debate`
  - `delphi`

### 场景夹具

- `TestE2EScenarioFixtureMatrix`
  - `coding-composite`
  - `factual-conflict`
  - `fallback-reversal`
- `TestE2EObserveNegatesActionFixtureFollowupChain`
  - `observe-negates-action`
  - follow-up artifact
  - child run lineage

## 执行方式

本地执行整套 E2E：

```bash
make test-e2e
```

或者直接跑：

```bash
go test ./internal/app -run '^TestE2E' -count=1
```

如果要和全量质量门禁一起跑：

```bash
go test ./...
go test -race ./...
go vet ./...
golangci-lint run
```

## 设计原则

- 优先用 mock provider 跑命令链路，保证测试稳定。
- 场景夹具尽量覆盖真实文件、外部命令、git 工作区、follow-up artifact。
- E2E 只校验用户真正能看到的重要行为，不校验内部实现细节。
- 对输出采用“关键片段”断言，而不是把整份终端输出完全 golden 化，避免过脆。

## 已知边界

- 当前 phase 级 `resume` 主要覆盖 `adjudication`。
- 真实外部 provider (`codex / claude / gemini / generic`) 还没有纳入 CI 的 E2E 矩阵。
- `view --web` 目前只做只读 MVP，不覆盖自动刷新或多用户访问。

## 下一步建议

- 增加真实 provider 的 smoke matrix
- 把 `free_debate` / `delphi` 的 resume/replay 也纳入 E2E
- 补一组 `generic` adapter 的完整命令级回归
