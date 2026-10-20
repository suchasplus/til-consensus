# TCView · til-consensus 桌面客户端

TCView 是 til-consensus 的桌面客户端，原名 NewTC。`til-consensus` 用于共识内核及主项目名称，`TCView` 用于桌面应用、窗口和安装包名称。当前桌面代码仍位于本项目目录，内核通过固定的远程 Go 模块引用。

在一个桌面聊天窗口内，由启用的 CLI 起草、独立评审和讨论，再由确定性程序检查是否收敛。默认由 Codex 起草，Claude Code、Grok Build 和 agy 评审；可以停用不可用的一家。界面采用 Electron、React 和 TypeScript，Go 后台复用 `til-consensus` 的三种模式。

当前版本 `0.6.0` 在 M1–M3、话题自动选择、文本材料分析和模型／推理设置基础上，增加 CLI 启用选择、动态角色、历史搜索与可恢复删除、三栏宽度调整与自动保存。**正常启动默认调用真实 CLI，会使用各 CLI 对应的模型额度；真实调用失败不会回退为 Mock。** 此前已完成一次真实四方自由辩论及停止恢复验收：24 次 CLI 请求、三张有效完整票，最终如实保留 `no_consensus`。自动化回归与真实模型实测分别记录。

默认界面采用 IBM Carbon 启发的商务主题：石墨色导航、灰白工作区、蓝色主操作、方角网格与本地 IBM Plex 字体。所有下拉选择器及展开菜单统一由应用绘制，支持键盘选择、长目录滚动和空间不足时向上展开。主题说明、字体许可与验证范围见 [桌面主题](/Users/suchasplus/devs/agentic/newtc/docs/desktop-theme.md)。

设计见 [设计文档](/Users/suchasplus/devs/agentic/newtc/docs/design.md)，当前交付范围、限制与验收记录见 [实现与验收说明](/Users/suchasplus/devs/agentic/newtc/docs/implementation.md)，四家 CLI 的独立实测见 [接口审计](/Users/suchasplus/devs/agentic/newtc/docs/cli-interface-audit.md)。[M0 开发说明](/Users/suchasplus/devs/agentic/newtc/docs/m0.md) 保留为历史原型记录。

## 启动

需要 macOS、Node.js 24 或更高版本、Go 1.27.1，以及至少三家已安装并完成各自登录的 CLI（`codex`、`claude`、`grok`、`agy`）。首次默认启用四家，可在设置停用不可用项。当前开发环境使用 Node.js 26.8.2。

内核通过**固定升级提交的远程 Go 模块**提供，默认构建使用 `GOWORK=off`，不查找或依赖 `~/agentic/til-consensus`。首次构建由 Go 下载锁定的模块，或使用已有模块缓存；正常构建不需要手动应用历史补丁。

```bash
npm install
npm start
```

`npm start` 构建 Go 后台和桌面应用后打开原生窗口。界面开发可使用 `npm run dev`；修改 Go 或 Electron 主进程后需要重新启动。

只有需要修改内核源码时，才显式使用本地开发覆盖：

```bash
TIL_CONSENSUS_DIR=/path/to/til-consensus npm start
```

[内核版本锁](/Users/suchasplus/devs/agentic/newtc/scripts/engine-lock.json) 记录 `module`、`moduleVersion` 和完整 `commit`，并与 [Go 模块依赖](/Users/suchasplus/devs/agentic/newtc/services/host/go.mod) 保持一致。显式本地覆盖要求 checkout 位于锁定提交；开发改动用 `+local.<hash>` 标识，仅为该覆盖生成 `.tcview/local-engine/go.work`。未设置覆盖时，即使机器上存在其他源码或旧 `go.work`，也使用远程固定模块。

默认模块路径可以使用 `TCVIEW_RELEASE=1 npm run build`。此标志仍拒绝脏的显式本地覆盖，不执行签名、公证或发布。

当前固定版本为 `github.com/suchasplus/til-consensus` 的 `v0.0.0-20261019194904-e9121310e997`，对应[提交 `e9121310e997`](https://github.com/suchasplus/til-consensus/commit/e9121310e9971179d3d902ee5b2a36bd61eb1553)，最低 Go 版本为 `1.27.1`。源码已推送并经 Go 模块机制实际下载、校验。[核心升级 PR #1](https://github.com/suchasplus/til-consensus/pull/1) 与[跟进 PR #2](https://github.com/suchasplus/til-consensus/pull/2) 均已合并；后者包含 Go 1.27、四家模型／推理参数契约回归及慢 CI 测试等待修复，远端四项 CI 已通过。当前伪版本不是正式 tag/release。

## 使用

- 在“CLI 与连接检查”中选择模型和适用推理档位，保存或恢复默认；可读取四家真实目录，也可手工填写未验证模型 ID。优先级为环境变量 > 应用保存设置 > 应用默认。
- 可逐家启用／停用。新讨论至少三家：一位提案者及至少两位独立评审；按 Codex、Claude、Grok、AGY 顺序选择首位启用者提案。当前有效角色在发送前显示并随讨论冻结，停用设置不会改变运行或恢复中的讨论。
- 真实连接检查使用编辑中的配置并叠加环境覆盖，不自动保存，会使用额度；版本探测和模型目录读取不发模型推理。请求值和 CLI 实际报告分开显示，未报告实际强度保持未知。详见 [CLI 设置说明](/Users/suchasplus/devs/agentic/newtc/docs/cli-settings.md)。
- “一键真实检查（使用额度）”逐家检查全部四家（含停用项），显示进度和通过／失败汇总；单家失败继续检查，其余项可停止。每家最多一个任务，不重复派发或自动保存。
- 输入目标，默认使用“自动选择”。发送后优先通过启用的 agy / Gemini CLI 检查话题；AGY 停用时显式改用启用的分析提供方。显示推荐理由和信心，信息充分时开始对应模式，也可手动选择三种模式。
- 可粘贴材料或选择 UTF-8 文本文件，点击“检查话题与材料”先看摘要、原文引文和信息缺口。最多 8 份，每份 64 KiB，目标与材料合计 128 KiB；暂不读取 PDF、图片或整个目录。
- 缺少必要信息时先补充；简单任务会提示无需多方讨论。检查提供方可换为 Codex、Claude 或 Grok；更改话题、材料或提供方后需重新检查。原文会随讨论冻结并提供给各参与者，摘要不会替代原文。
- 查看方案版本、争议、投票或匿名评分。讨论完成不一定代表方案通过，未解决的阻塞意见会保留。
- 停止后可在原 15 分钟期限内继续；补充目标或约束会创建关联的新讨论，旧投票不会继承。
- 通过原生保存对话框导出 Markdown 或 JSON；诊断清理保留聊天、结果、版本和账本。
- 左侧可搜索并删除最近讨论，删除后在“最近删除”恢复；运行中的讨论需先停止并等待 CLI 退出。删除保留审计和关联链额度。
- 拖动两条分栏边界调整宽度，自动保存；分隔条支持方向键和双击复位。窄窗口保留讨论产物抽屉。详细操作见 [界面与参与者说明](/Users/suchasplus/devs/agentic/newtc/docs/desktop-interactions.md)。
- 上方讨论信息默认收起，下方输入区在发起／恢复讨论和查看历史时自动收起；点击各自开关可展开。折叠保留草稿与材料，停止、恢复、继续讨论和取消分析仍可直接操作。
- 最近讨论和讨论产物也可独立收起，折叠状态自动保存；两边都收起时，主讨论区铺满窗口。展开后恢复原栏宽和当前内容，顶栏始终保留开关。

每个讨论最多允许后台发起 **30 次 CLI 任务请求**；同一根讨论及其全部关联分支共用 **90 次链额度**，包含采用的话题检查、P0、内核重试与恢复后的请求。额度在调用前原子预留，不能通过创建兄弟分支绕过。未采用的检查作为独立分析保存，不计入某个讨论额度。一次 CLI 任务内部可能进行多次服务端 API 调用，因此这不是 API 调用次数、token 或费用的硬限额。自动选择和材料边界详见 [话题准备说明](/Users/suchasplus/devs/agentic/newtc/docs/topic-preparation.md)。

## 验证与打包

```bash
npm test
npm run package:mac
```

`npm test` 包含 Go race、TypeScript、构建、stdio RPC 和真实 Electron 窗口测试；讨论使用**显式 Mock 或已录制响应回放**，默认不发起真实模型推理。真实 Electron 窗口与真实 Go 内核测试不等于真实模型测试。

显式执行 `npm run verify:preparation:live` 会通过已登录的 AGY 发出五次真实话题分析请求，以实际项目文件验证三种模式、信息不足和简单任务；不启动完整多方讨论，原始结果保存到 `.tcview/preparation-validation/`。

手动检查固定远程模块的冷缓存构建：

```bash
npm run verify:dependency
```

此命令会下载 Go 模块，在隔离 HOME、独立空模块缓存和 `TCVIEW_RELEASE=1` 下构建并检查后台版本；复用已安装的 Node 依赖及 Go 编译缓存，不调用模型。改名前 `e9121310e997` 的独立冷缓存发布构建已通过，[验收记录](/Users/suchasplus/devs/agentic/newtc/.newtc/dependency-upgrade/2026-10-19T19-53-11.804Z/summary.json)保留实际模块路径、校验和与后台版本；先前 `cb9149af032e` 的记录保留于实现说明。

`npm run package:mac` 生成 `release/TCView-darwin-<当前架构>/TCView.app`，包含 Go sidecar。当前产物使用本机 ad-hoc 签名，属于本机开发包；没有 Developer ID 签名、公证、DMG 或自动更新，也不包含四家 CLI、认证或额度。

仅在需要检查旧 M0 流程时显式启用模拟模式，并使用独立数据目录：

```bash
TCVIEW_PROVIDER_MODE=mock TCVIEW_DATA_DIR="$PWD/.tcview/mock-host" npm start
```

旧 Mock 历史会保留来源标记，不能作为真实讨论结果，也不会自动转换为真实 CLI 执行。

## 名称与数据目录

桌面产品与窗口名为 `TCView`，npm 包名为 `tcview`，共识内核及主项目名称为 `til-consensus`。当前标识统一如下：

| 项目 | 当前值 |
| --- | --- |
| macOS 应用 | `TCView.app` |
| Bundle ID | `local.tcview.desktop` |
| 用户数据和浏览器会话目录 | `~/Library/Application Support/TCView` |
| 默认后台数据目录 | `~/Library/Application Support/TCView/host` |
| 环境变量前缀 | `TCVIEW_*` |
| 受限桌面桥接 | `window.tcview`，IPC 使用 `tcview:*` |
| 布局和折叠本地存储键 | `tcview.*` |
| 后台二进制／入口 | `tcview-host`／`services/host/cmd/tcview-host` |
| 后台 Go module | `github.com/suchasplus/til-consensus/desktop/services/host` |
| 本地开发覆盖与新验收产物 | `.tcview/` |

本次不保留旧 NewTC 标识兼容：应用不自动读取或迁移旧用户数据、CLI 设置和布局，不识别旧环境变量或桥接别名。首次使用 TCView 目录时从默认设置开始；旧目录和数据保留在原位置，不自动删除。此调整不修改用户全局 CLI 配置和认证。

独立身份调整已通过类型检查、Go 全包 race、8 项 RPC、7 项构建环境和完整 40 项 Electron 回归。新包以 `TCVIEW_RELEASE=1`、无本地内核覆盖重建并验收，包内仅包含 `tcview-host`。隔离测试确认旧环境、IPC、桥接和存储键不生效，旧目录逐文件内容保持不变；新配置、讨论与布局独立持久化。[本次验收记录](/Users/suchasplus/devs/agentic/newtc/.tcview/branding-validation/2026-10-20T08-21-04.223Z/summary.json)。

本机项目物理目录仍为 `/Users/suchasplus/devs/agentic/newtc`。旧 `.newtc/` 中的历史截图、导出、验收文件和[前次改名记录](/Users/suchasplus/devs/agentic/newtc/.newtc/branding-validation/2026-10-20T08-07-16.691Z/summary.json)保留原始路径与内容，不作为当前运行目录；保留其 Git 忽略规则以免误提交私有记录。本文当前操作示例使用 `TCVIEW_*` 和 `.tcview/`，旧证据反映执行当时的标识。
