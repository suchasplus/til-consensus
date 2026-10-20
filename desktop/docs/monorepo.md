# 同仓开发、CI 与发布边界

TCView 源码位于 til-consensus 仓库的 `desktop/`。公共 CLI、Go SDK 与桌面一起评审接口、测试和实现，保持各自的依赖边界。

## 仓库与模块

```text
til-consensus/
├── go.mod                    # github.com/suchasplus/til-consensus
├── go.work                   # use . 与 ./desktop/services/host
├── cmd/til-consensus/         # 公共 CLI
├── consensus/、runtime/ …    # 公开 Go 内核
├── .github/workflows/
└── desktop/
    ├── package.json          # tcview；Electron、React 与 TypeScript
    ├── package-lock.json
    ├── apps/desktop/
    ├── packages/protocol/
    ├── services/host/go.mod   # 独立 Go host module
    ├── scripts/
    └── tests/
```

根 module 的公开导入路径不变，SQLite 等桌面专属依赖留在 host 子模块。普通 Go SDK 使用者只下载根模块，不需要安装 Electron 或 Node.js。独立检查根模块可执行 `GOWORK=off go test ./...`；它不会运行嵌套 module 的 host 测试。

## 克隆与启动

准备 Go 1.27.1、macOS，以及 `.nvmrc` 指定的 Node.js 版本后，从本仓库根目录执行：

```bash
cd desktop
npm ci
npm start
```

无需另行克隆兄弟目录、编辑用户全局 CLI 配置或设置全局 Go workspace。正常运行默认使用真实 CLI，会使用对应服务额度；新讨论要求至少三家 CLI 启用并可用。测试或离线检查须明确选择 `TCVIEW_PROVIDER_MODE=mock`，真实失败不会自动回退。

## 同一 checkout 与构建身份

桌面构建和 host 测试脚本显式使用根 `go.work`，校验公开内核是 workspace 主模块，并且目录恰为当前仓库根目录。外部遗留 `GOWORK` 不参与解析，禁止借助 sibling checkout 或未声明的 `replace` 绕过同仓来源检查。host `go.mod` 保留公开版本基线供独立模块解析；它不决定默认桌面构建所用内核。

仓库提交用于追踪整套交付源码；内核兼容身份根据根模块与 host 的生产 Go 源码、两份 `go.mod`／`go.sum` 和根 `go.work` 计算。它们分别记录，避免只改 CSS、图标或 README 就导致已冻结讨论无法恢复。运行、恢复、CLI 版本、模型、请求预算、策略和 ballot 校验继续执行，内核行为发生变更时仍拒绝不兼容恢复。[身份清单](../scripts/engine-lock.json) 保留精确迁移基线：只有执行源码 hash 完全一致时才沿用旧 `e9121310e997…` 身份，其余返回 `source-sha256:<完整hash>`；不会把不同执行源码伪装成旧提交。测试文件、前端和文档不进入执行源码 hash。

`dist/host-build-info.json` 及包内 `build-info.json` 记录 `repositoryRevision`、`sourceHash`、`engineVersion` 和 `dirty`。具体散列范围见 [源码身份实现](../scripts/source-identity.mjs) 与 [构建环境回归](../tests/go-env.test.mjs)。

```bash
TCVIEW_RELEASE=1 npm run build
TCVIEW_RELEASE=1 npm run verify:dependency
```

发布来源检查要求可追踪且干净的 Git checkout。`verify:dependency` 复制独立完整 checkout，在隔离 HOME、空 Go 模块缓存与空 Go 编译缓存中验证当前源码，核对 workspace 与构建身份；它不调用真实模型，仅复用 Node 依赖，第三方 Go 依赖仍可能需要下载。显式 `TCVIEW_RELEASE=1` 时执行干净源码发布检查；普通命令也可验证未提交的开发源码镜像，结果会如实记录 dirty。

普通开发允许未提交修改并记录实际源码身份。`TCVIEW_RELEASE=1` 只检查构建来源；它不会创建 tag、推送代码、签名公证或发布正式版本。

## 自动化检查

从 `desktop/` 执行：

```bash
npm run check
npm run test:build-env
npm run test:host
npm run build
npm run test:rpc
npm run test:e2e
```

`npm test` 串联上述检查。RPC 与 Electron 自动化使用显式 Mock 或脱敏协议回放；窗口和 Go 进程是真的，不表示请求了真实模型。真实 CLI 目录及请求验收继续使用 `verify:cli-settings:live`、`verify:providers:batch:live` 等显式命令，并单独记录服务额度与失败。

根目录提供 `make desktop-install`、`make desktop-start`、`make desktop-check`、`make desktop-test` 和 `make desktop-package` 作为同等入口。

## CI

[公共内核 CI](../../.github/workflows/ci.yml) 保留 Linux 质量／race／vet／lint、macOS 构建、Windows 进程控制和 CLI 烟测，显式 `GOWORK=off`，验证根 SDK 与 CLI 独立可用。

[桌面 CI](../../.github/workflows/desktop.yml) 在所有 PR 和 main 推送时执行，不使用路径过滤。因此内核或 CLI 修改也会触发桌面集成：

- Linux：`npm ci`、TypeScript、构建环境契约、Go host race／vet、当前 checkout 构建与 stdio RPC。
- macOS：构建、全部 Electron 回归、打包、ad-hoc 签名校验与离线安装包烟测。
- 离线安装包烟测通过实际 Electron UI 发起显式 Mock 讨论，检查 Go 结果、事件、重连与退出重开持久化；隔离数据与进程环境，不要求安装四家 CLI，不读取认证，也不发起真实模型请求。

离线烟测入口为：

```bash
npm run package:mac
node scripts/verify-package-offline.mjs
```

本地 `npm run verify:package` 继续检查已安装真实 CLI 的可执行文件、版本、设置页和重连；它不发推理请求，但不适用于没有四家 CLI 的 CI。两种烟测各自明确来源，不能互相冒充模型验收。

## 发布边界

根 CLI／Go SDK 使用 `v*` 标签及现有 [CLI 发布流程](../../.github/workflows/release.yml)。CLI 构建的自动版本发现只匹配 `v[0-9]*`，桌面标签不会被误选为 CLI 版本。

桌面版本单独维护于 `desktop/package.json`，预留 `desktop-v*` 标签空间。当前没有按桌面标签自动正式发布的流程，也没有创建本次 tag／release。手工触发桌面 workflow 只上传七天保留的 macOS 开发包 artifact，供下载检查，不发布 GitHub Release。

当前包为构建架构的 macOS ad-hoc 开发包，未加入 Developer ID、公证、DMG、自动更新或正式跨架构分发。四家 CLI、运行环境和认证不随安装包分发。

## 导入范围与历史证据

导入 TCView 相关源码、测试、依赖锁、文档、字体与 OFL 许可，保留历史接口补丁及元数据。运行数据库、草稿、日志、截图、导出、`node_modules`、安装包和独立远程代码调查服务需求文档保留在原项目目录，没有导入。

旧文档中的私有证据已改为注明原本地记录的文字，避免仓库内出现不存在的可点击路径。历史实测版本、模型、失败、额度和构建指纹保持执行时的语义；旧记录不能作为合仓后构建或真实模型验证的证据。当前用户标识继续使用 `TCVIEW_*`、`window.tcview`、`tcview.*` 和 TCView 数据目录，不增加旧 NewTC 兼容。
