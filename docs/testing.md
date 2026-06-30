# 测试、CI 与发布

这篇覆盖本地质量门禁、E2E、真实 provider 验证、coverage treemap 和发布流程。

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

## Git pre-push hook

安装本地 hook：

```bash
make install-hooks
```

安装后，`.git/hooks/pre-push` 会在每次 `git push` 前调用 `make pre-push`。这个 hook 只在当前 clone 生效，不会提交到 Git。

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

本地构建：

```bash
make build
```

清理：

```bash
make clean
```

版本信息由构建注入；本地开发版本通常会显示当前 commit 和 dirty 状态。
