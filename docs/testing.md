# 测试、CI 与发布

这篇覆盖本地质量门禁、E2E、真实 provider 验证、coverage treemap 和发布流程。

## 工具版本

最低 Go 版本为 `1.27.1`，与 `go.mod` 一致；本地和 CI 的质量检查使用 `golangci-lint v2.13.2`。可先运行 `go version` 与 `golangci-lint version` 核对工具。

Linux 质量检查、macOS 构建、Windows 进程测试及 release 矩阵均通过 `actions/setup-go` 的 `go-version-file: go.mod` 选择同一 Go 版本。工作流设置 `GOTOOLCHAIN=local`，避免测试或构建期间自动切换工具链，运行日志会输出实际 Go 版本。

## 本地质量门禁

日常提交前运行：

```bash
make pre-push
```

`make pre-push` 当前会执行：

- `fmt-check`
- `coverage-treemap`
- `go vet`
- `golangci-lint`
- `build`

其中 `coverage-treemap` 会运行：

```bash
go test ./... -coverprofile=./tmp/coverage/cover.out
go-cover-treemap -coverprofile ./tmp/coverage/cover.out > ./tmp/coverage/coverage.svg
```

生成物：

- `tmp/coverage/cover.out`
- `tmp/coverage/coverage.svg`

`tmp/` 默认不提交。

`fmt` 和 `fmt-check` 只处理 Git 已跟踪以及未跟踪但未被忽略的 Go 文件。这样既会检查尚未 `git add` 的新源码，也不会进入 `tmp/` 等忽略目录中的外部项目或临时材料。

## Git pre-push hook

安装本地 hook：

```bash
make install-git-hooks
```

安装后，`.git/hooks/pre-push` 会在每次 `git push` 前调用 `make pre-push`。这个 hook 只在当前 clone 生效，不会提交到 Git。

`README.md` 和 `docs/**/*.md` 的 shell 代码块中出现的 `make` 命令会由文档 smoke test 自动核对，确保目标在 Makefile 中真实存在。

## CI 对齐

更完整地对齐 GitHub Actions：

```bash
make ci
```

CI 通常覆盖：

- Go 单元测试。
- race 或扩展测试。
- `go vet`。
- `golangci-lint`。
- 构建。
- release 相关检查。

## E2E 分层

E2E 分三层：

### 命令链路层

验证 CLI 入口、默认路径、输出产物和主要子命令串联。

常见覆盖：

- `config init`
- `setup`
- `run`
- `ask / debate / delphi`
- `view / last / inspect / logs / open`
- `artifact list/show`

### 场景夹具层

用 mock provider 跑稳定的 mode fixture：

- `adjudication`
- `free_debate`
- `delphi`

重点验证 result/ledger/artifacts/session/follow-up 等结构，不依赖线上 provider。

### 真实 provider 层

真实 provider 层用于 daily 或手动验证：

- 真实 CLI provider readiness。
- 真实 CLI provider × mode 矩阵。
- 真实 API provider readiness。
- 真实 API provider × mode 矩阵。

真实矩阵开始前会先做 provider readiness preflight。单个 provider 不 ready 时按 provider 级降级跳过；只要当前 mode 还能由其他 ready provider 组成角色映射，测试继续跑。

## 常用测试命令

默认 E2E，不访问线上 provider：

```bash
make test-e2e
```

真实 CLI provider：

```bash
TIL_CONSENSUS_E2E_REAL_CLI=1 make test-e2e-real
```

真实 CLI fixture 的三个 mode 默认各有 `25m` 的外层运行预算，单个 CLI task 的超时为 `5m`。可通过以下环境变量覆盖外层预算：

- `TIL_CONSENSUS_E2E_REAL_TIMEOUT`：统一覆盖所有 mode。
- `TIL_CONSENSUS_E2E_REAL_TIMEOUT_ADJUDICATION`：只覆盖 `adjudication`。
- `TIL_CONSENSUS_E2E_REAL_TIMEOUT_FREE_DEBATE`：只覆盖 `free_debate`。
- `TIL_CONSENSUS_E2E_REAL_TIMEOUT_DELPHI`：只覆盖 `delphi`。

变量值使用 Go duration 格式，例如 `30m`、`1h15m`。通用覆盖优先于 mode 专用覆盖；无法解析、为零或为负数的值会被忽略并继续回退到下一层，最终使用 `25m` 默认值。

真实 API provider：

```bash
TIL_CONSENSUS_E2E_REAL_API=1 make test-e2e-real-api
```

如果只跑 Go 测试：

```bash
go test ./...
```

## 真实 provider 环境变量

真实 CLI 默认依赖本机命令可非交互调用：

- `codex`
- `claude`
- `gemini`
- `agy`

真实 API 默认读取：

- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`
- `GEMINI_API_KEY`
- `GOOGLE_APPLICATION_CREDENTIALS`
- `GOOGLE_CLOUD_PROJECT`（Pi Vertex；location 由测试固定为 `global`）
- `DEEPSEEK_API_KEY`
- `BAILIAN_API_KEY`
- `KIMI_API_KEY`

具体可用性以配置中的 `api_key_env` 为准。

## 真实 E2E 原则

- 默认 CI 不访问线上 provider。
- 线上 provider 只在显式环境变量开启时运行。
- 所有真实 provider 先 preflight。
- provider 不可用时按 provider 级降级，不做全局 fail。
- 如果没有任何 provider ready，真实矩阵应明确 fail 或 skip，并打印原因。
- daily run 关注 provider/schema 漂移、unresolved 比例和 strict compliance。

## 发布

发布矩阵构建 Linux 与 macOS 的 amd64、arm64 归档。每个归档都会解包，并通过 `go version -m` 核对编译器版本、GOOS 和 GOARCH；只有目标系统及架构与 runner 一致时才直接执行 `--version`，交叉构建产物不在异构 runner 上运行。

本地构建：

```bash
make build
```

清理：

```bash
make clean
```

版本信息由构建注入；本地开发版本通常会显示当前 commit 和 dirty 状态。
