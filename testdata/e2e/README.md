## E2E 夹具

这里存放真实端到端矩阵测试使用的输入夹具。

当前目录只保留与任务本身相关的内容：

- `fixture.yaml`
  - 描述 mode、默认的 `view` section，以及最小断言片段
- `run.yaml`
  - 真实 `til-consensus run --input` 使用的输入

测试时不会直接使用仓库内固定 config，而是动态生成：

- `cli` 配置
  - 真实调用本机已安装的 `claude / gemini / codex`
- `api` 配置
  - 默认使用本地 `httptest` server 模拟 `openai-compatible / anthropic-compatible / gemini-api`
  - 显式开启真实 API E2E 时，改为调用线上 API provider

这样做的原因：

- `cli` 需要使用真实命令和当前机器上的登录态
- `api` 需要动态注入本地测试 server 的 `base_url`
- 输出目录需要指向临时目录，避免污染工作区

默认 `go test ./internal/app -run '^TestE2E'` 只会跑：

- 夹具发现
- mock 命令链路和历史场景夹具
- 本地模拟 API provider × 三种 mode 的矩阵

真实 provider 矩阵需要显式开启。测试开始前会先执行 readiness preflight；单个 provider 不 ready 时按 provider 级降级跳过，只有当前环境没有任何 provider ready 时才 fail。

真实 CLI × 三种 mode：

```bash
TIL_CONSENSUS_E2E_REAL=1 go test ./internal/app -run '^TestE2ERealCLIFixtureMatrix$' -count=1
```

真实 API × 三种 mode：

```bash
TIL_CONSENSUS_E2E_REAL_API=1 go test ./internal/app -run '^TestE2ERealAPIFixtureMatrix$' -count=1
```

对应的 `Makefile` 入口：

```bash
make test-e2e-real
make test-e2e-real-api
```
