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

当前 E2E 分成三层。

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

当前优先覆盖 `testdata/e2e/` 下的三种 workflow fixture：

- `adjudication`
- `free_debate`
- `delphi`

同时保留历史场景夹具：

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

### 3. 真实 provider 层

目的：在本机或日常任务里验证真实 provider 配置、登录态、API key、base url 和模型名是否仍然可用。

真实 provider 层分成：

- 真实 CLI
  - `claude / gemini / antigravity / codex`
  - 使用当前机器上的真实命令和登录态
- 真实 API
  - `openai-compatible / anthropic-compatible / gemini-api`
  - 默认读取对应 API key 和可选 base url / model 覆盖

真实矩阵开始前会先做 provider readiness preflight。单个 provider 不 ready 时，该 provider 会在矩阵里按 provider 级降级跳过；只要当前模式还能由其他 ready provider 组成角色映射，测试会继续跑。如果没有任何 provider ready，则直接 fail。

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

- `TestE2EFixtureCatalog`
  - 检查 `testdata/e2e` fixture 元数据和输入文件是否完整
- `TestE2EAPIFixtureMatrix`
  - 使用本地 `httptest` 模拟 `openai-compatible / anthropic-compatible / gemini-api`
  - 覆盖 `adjudication / free_debate / delphi`
- `TestE2EScenarioFixtureMatrix`
  - `coding-composite`
  - `factual-conflict`
  - `fallback-reversal`
- `TestE2EObserveNegatesActionFixtureFollowupChain`
  - `observe-negates-action`
  - follow-up artifact
  - child run lineage

### 真实 provider

- `TestE2ERealCLIProviderReadinessPreflight`
  - 探测 `claude / gemini / antigravity / codex` 是否能完成最小非交互 JSON 调用
- `TestE2ERealCLIFixtureMatrix`
  - 用 ready 的真实 CLI provider 跑三种 mode fixture
- `TestE2ERealAPIProviderReadinessPreflight`
  - 探测真实 API provider 是否能完成最小非交互 JSON 调用
- `TestE2ERealAPIFixtureMatrix`
  - 用 ready 的真实 API provider 跑三种 mode fixture

## 执行方式

本地执行整套 E2E：

```bash
make test-e2e
```

默认 `make test-e2e` 不访问线上 provider；它会跑 mock 命令链路、场景夹具、本地模拟 API 协议矩阵和多 mode smoke。

执行真实 CLI provider 预检与矩阵：

```bash
TIL_CONSENSUS_E2E_REAL=1 make test-e2e-real
```

执行真实 API provider 预检与矩阵：

```bash
TIL_CONSENSUS_E2E_REAL_API=1 make test-e2e-real-api
```

真实 CLI 默认依赖这些本机命令可非交互调用：

- `claude`
- `gemini`
- `agy`
- `codex`

真实 API 默认会读取这些环境变量：

- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`
- `GEMINI_API_KEY`

可选覆盖：

- `TIL_CONSENSUS_E2E_OPENAI_MODEL`
- `TIL_CONSENSUS_E2E_ANTHROPIC_MODEL`
- `TIL_CONSENSUS_E2E_GEMINI_MODEL`
- `TIL_CONSENSUS_E2E_OPENAI_BASE_URL`
- `TIL_CONSENSUS_E2E_ANTHROPIC_BASE_URL`
- `TIL_CONSENSUS_E2E_GEMINI_BASE_URL`

超时可以按全局或按 mode 覆盖：

- `TIL_CONSENSUS_E2E_REAL_TIMEOUT`
- `TIL_CONSENSUS_E2E_REAL_TIMEOUT_ADJUDICATION`
- `TIL_CONSENSUS_E2E_REAL_TIMEOUT_FREE_DEBATE`
- `TIL_CONSENSUS_E2E_REAL_TIMEOUT_DELPHI`
- `TIL_CONSENSUS_E2E_REAL_API_TIMEOUT`
- `TIL_CONSENSUS_E2E_REAL_API_TIMEOUT_ADJUDICATION`
- `TIL_CONSENSUS_E2E_REAL_API_TIMEOUT_FREE_DEBATE`
- `TIL_CONSENSUS_E2E_REAL_API_TIMEOUT_DELPHI`
- `TIL_CONSENSUS_E2E_REAL_PREFLIGHT_TIMEOUT`
- `TIL_CONSENSUS_E2E_REAL_API_PREFLIGHT_TIMEOUT`

或者直接跑：

```bash
go test ./internal/app -run '^TestE2E' -count=1
```

如果要和全量质量门禁一起跑：

```bash
make pre-push
```

`make pre-push` 会执行：

```bash
gofmt -l .
go test ./...
go vet ./...
golangci-lint run
make build
```

## 设计原则

- 优先用 mock provider 跑命令链路，保证测试稳定。
- 默认 E2E 不依赖线上 provider；线上 provider 只在显式环境变量开启时运行。
- 真实 provider 先 preflight，再按 ready provider 生成角色映射；单 provider 不可用不应导致整组全局 fail。
- 场景夹具尽量覆盖真实文件、外部命令、git 工作区、follow-up artifact。
- E2E 只校验用户真正能看到的重要行为，不校验内部实现细节。
- 对输出采用“关键片段”断言，而不是把整份终端输出完全 golden 化，避免过脆。

## 已知边界

- 当前 phase 级 `resume` 主要覆盖 `adjudication`。
- 真实外部 provider 不纳入默认 CI；它们通过 `make test-e2e-real` 和 `make test-e2e-real-api` 做手动或 daily 验证。
- `view --web` 目前只做只读 MVP，不覆盖自动刷新或多用户访问。

## 下一步建议

- 把 `free_debate` / `delphi` 的 resume/replay 也纳入 E2E
- 补一组 `generic` adapter 的完整命令级回归
- 把真实 API 矩阵扩展到更多 OpenAI-compatible 网关，例如 DeepSeek、Qwen、Kimi 或 OpenRouter
