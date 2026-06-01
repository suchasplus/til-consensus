# CI 与发布

`til-consensus` 目前使用 GitHub Actions 做两类自动化：

- `ci`
  - PR 和推送到 `main` 时触发
  - 负责质量门禁
- `release`
  - 推送 `v*` tag 时触发
  - 负责多平台构建与 GitHub Release

## CI 流水线

工作流文件：

- `.github/workflows/ci.yml`

触发条件：

- `pull_request`
- `push` 到 `main`

当前 CI 包含 3 个 job：

- `quality-linux`
  - `gofmt -l .`
  - `go test ./...`
  - `go test -race ./...`
  - `go vet ./...`
  - `golangci-lint run`
  - `make build`
- `build-macos`
  - macOS 上编译 smoke test
  - `make build`
  - `./bin/til-consensus --version`
- `cli-smoke`
  - 下载 Linux 构建产物
  - 验证：
    - `til-consensus --version`
    - `til-consensus version`

## 本地对齐 CI

本地可以直接运行：

```bash
make ci
```

它会顺序执行：

- 格式检查
- 单元测试
- race 测试
- `go vet`
- `golangci-lint`
- `make build`

## 推送前本地检查

日常开发不一定每次都要跑完整 `make ci`。为了在推送前提前发现 GitHub Actions 里的 `golangci-lint` / `staticcheck` 问题，可以运行：

```bash
make pre-push
```

它会顺序执行：

- 格式检查
- `go test ./...`
- `go vet ./...`
- `golangci-lint run`
- `make build`

如果希望每次 `git push` 前自动执行，可以安装仓库提供的 Git hook：

```bash
make install-git-hooks
```

安装后，`.git/hooks/pre-push` 会调用 `make pre-push`。这个 hook 不会提交到 Git，只在当前本地 clone 生效。

## 发布流水线

工作流文件：

- `.github/workflows/release.yml`

触发条件：

- 推送 tag，格式：

```text
v0.1.0
v0.2.3
```

当前发布覆盖 4 个目标平台：

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`

每个平台会：

- 注入版本信息
- 构建压缩包
- 校验 `--version` 输出

最终发布产物：

- `til-consensus_<version>_<goos>_<goarch>.tar.gz`
- `checksums.txt`

## 本地构建发布包

本地可以这样模拟单个平台发布：

```bash
make release-archive VERSION=v0.1.0 TARGET_GOOS=darwin TARGET_GOARCH=arm64 DIRTY=false
```

输出示例：

```text
dist/til-consensus_v0.1.0_darwin_arm64.tar.gz
```

## 版本注入

CI / release 都会通过 `ldflags` 注入：

- `version`
- `commit`
- `build time`
- `dirty`

发布流水线里会固定：

- `VERSION=<tag>`
- `DIRTY=false`

## 当前明确不做

这一版发布链路暂时不包含：

- Windows 构建
- 签名 / notarization
- Homebrew tap
- changelog 自动生成
- SBOM / provenance
