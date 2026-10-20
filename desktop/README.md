# TCView · til-consensus 桌面客户端

TCView 是 til-consensus 的桌面客户端，原名 NewTC。`til-consensus` 用于共识内核及主项目名称，`TCView` 用于桌面应用、窗口和安装包名称。桌面源码位于本仓库的 `desktop/`，通过根 `go.work` 直接使用当前 checkout 的公开 Go 内核。

在一个桌面聊天窗口内，由启用的 CLI 起草、独立评审和讨论，再由确定性程序检查是否收敛。默认由 Codex 起草，Claude Code、Grok Build 和 agy 评审；可以停用不可用的一家。界面采用 Electron、React 和 TypeScript，Go 后台复用 `til-consensus` 的三种模式。

当前版本 `0.6.0` 在 M1–M3、话题自动选择、文本材料分析和模型／推理设置基础上，增加 CLI 启用选择、动态角色、历史搜索与可恢复删除、三栏宽度调整与自动保存。**正常启动默认调用真实 CLI，会使用各 CLI 对应的模型额度；真实调用失败不会回退为 Mock。** 此前已完成一次真实四方自由辩论及停止恢复验收：24 次 CLI 请求、三张有效完整票，最终如实保留 `no_consensus`。自动化回归与真实模型实测分别记录。

默认界面采用 IBM Carbon 启发的商务主题：石墨色导航、灰白工作区、蓝色主操作、方角网格与本地 IBM Plex 字体。所有下拉选择器及展开菜单统一由应用绘制，支持键盘选择、长目录滚动和空间不足时向上展开。主题说明、字体许可与验证范围见 [桌面主题](docs/desktop-theme.md)。

设计见 [设计文档](docs/design.md)，当前交付范围、限制与验收记录见 [实现与验收说明](docs/implementation.md)，四家 CLI 的独立实测见 [接口审计](docs/cli-interface-audit.md)。[M0 开发说明](docs/m0.md) 保留为历史原型记录。

## 启动

需要 macOS、Go 1.27.1，以及 [.nvmrc](.nvmrc) 固定的 Node.js 26.8.2（支持范围 Node.js 24 或更高）。真实讨论需要至少三家已安装并完成各自登录的 CLI：`codex`、`claude`、`grok`、`agy`。首次默认启用四家，可在设置停用不可用项。

从仓库根目录执行：

```bash
cd desktop
npm ci
npm start
```

`npm start` 构建 Go 后台和桌面应用后打开原生窗口。界面开发可使用 `npm run dev`；修改 Go 或 Electron 主进程后需要重新启动。以下 npm 命令均在 `desktop/` 中执行。

构建脚本明确选择仓库根 [go.work](../go.work)，并验证内核解析到本仓库根模块。即使外部设置了 `GOWORK=off` 或其他 workspace，也不会悄悄下载旧内核或读取兄弟目录。根 SDK 与桌面 host 仍是两个独立 Go module；host `go.mod` 中的公开版本只是独立模块基线，桌面集成构建始终使用同仓源码。修改内核和桌面接口可在一个 PR 内一起验证，无需先发布内核再修改桌面依赖。

`TCVIEW_RELEASE=1 npm run build` 要求可追踪且干净的同仓源码，不执行正式发布、发行签名或公证。构建记录区分仓库提交与内核兼容身份，纯界面改动不会更换已冻结的内核身份。详见 [同仓开发与 CI](docs/monorepo.md)。

## 使用

- 在“CLI 与连接检查”中选择模型和适用推理档位，保存或恢复默认；可读取四家真实目录，也可手工填写未验证模型 ID。优先级为环境变量 > 应用保存设置 > 应用默认。
- 可逐家启用／停用。新讨论至少三家：一位提案者及至少两位独立评审；按 Codex、Claude、Grok、AGY 顺序选择首位启用者提案。当前有效角色在发送前显示并随讨论冻结，停用设置不会改变运行或恢复中的讨论。
- 真实连接检查使用编辑中的配置并叠加环境覆盖，不自动保存，会使用额度；版本探测和模型目录读取不发模型推理。请求值和 CLI 实际报告分开显示，未报告实际强度保持未知。详见 [CLI 设置说明](docs/cli-settings.md)。
- “一键真实检查（使用额度）”逐家检查全部四家（含停用项），显示进度和通过／失败汇总；单家失败继续检查，其余项可停止。每家最多一个任务，不重复派发或自动保存。
- 输入目标，默认使用“自动选择”。发送后优先通过启用的 agy / Gemini CLI 检查话题；AGY 停用时显式改用启用的分析提供方。显示推荐理由和信心，信息充分时开始对应模式，也可手动选择三种模式。
- 可粘贴材料或选择 UTF-8 文本文件，点击“检查话题与材料”先看摘要、原文引文和信息缺口。最多 8 份，每份 64 KiB，目标与材料合计 128 KiB；暂不读取 PDF、图片或整个目录。
- 缺少必要信息时先补充；简单任务会提示无需多方讨论。检查提供方可换为 Codex、Claude 或 Grok；更改话题、材料或提供方后需重新检查。原文会随讨论冻结并提供给各参与者，摘要不会替代原文。
- 查看方案版本、争议、投票或匿名评分。讨论完成不一定代表方案通过，未解决的阻塞意见会保留。
- 停止后可在原 15 分钟期限内继续；补充目标或约束会创建关联的新讨论，旧投票不会继承。
- 通过原生保存对话框导出 Markdown 或 JSON；诊断清理保留聊天、结果、版本和账本。
- 左侧可搜索并删除最近讨论，删除后在“最近删除”恢复；运行中的讨论需先停止并等待 CLI 退出。删除保留审计和关联链额度。
- 拖动两条分栏边界调整宽度，自动保存；分隔条支持方向键和双击复位。窄窗口保留讨论产物抽屉。详细操作见 [界面与参与者说明](docs/desktop-interactions.md)。
- 上方讨论信息默认收起，下方输入区在发起／恢复讨论和查看历史时自动收起；点击各自开关可展开。折叠保留草稿与材料，停止、恢复、继续讨论和取消分析仍可直接操作。
- 最近讨论和讨论产物也可独立收起，折叠状态自动保存；两边都收起时，主讨论区铺满窗口。展开后恢复原栏宽和当前内容，顶栏始终保留开关。

每个讨论最多允许后台发起 **30 次 CLI 任务请求**；同一根讨论及其全部关联分支共用 **90 次链额度**，包含采用的话题检查、P0、内核重试与恢复后的请求。额度在调用前原子预留，不能通过创建兄弟分支绕过。未采用的检查作为独立分析保存，不计入某个讨论额度。一次 CLI 任务内部可能进行多次服务端 API 调用，因此这不是 API 调用次数、token 或费用的硬限额。自动选择和材料边界详见 [话题准备说明](docs/topic-preparation.md)。

## 验证与打包

```bash
npm test
npm run package:mac
```

`npm test` 包含 Go race、TypeScript、构建、stdio RPC 和真实 Electron 窗口测试；讨论使用**显式 Mock 或已录制响应回放**，默认不发起真实模型推理。真实 Electron 窗口与真实 Go 内核测试不等于真实模型测试。

显式执行 `npm run verify:preparation:live` 会通过已登录的 AGY 发出五次真实话题分析请求，以实际项目文件验证三种模式、信息不足和简单任务；不启动完整多方讨论，原始结果保存到 `.tcview/preparation-validation/`。

手动检查同仓源码的隔离构建：

```bash
TCVIEW_RELEASE=1 npm run verify:dependency
```

上述命令在隔离 HOME、空 Go 模块缓存／编译缓存和独立完整 checkout 中验证当前提交的发布构建，确认内核来自同一 checkout，不调用模型。`TCVIEW_RELEASE=1` 要求已提交且干净的源码；不加该标志也可验收未提交的开发源码镜像。脚本只复用已安装的 Node 依赖；详情见 [构建来源说明](docs/monorepo.md)。历史独立远端模块验收保留在原本地私有目录，没有随合仓导入，也不能作为此次同仓构建成功的证据。

`npm run package:mac` 生成 `release/TCView-darwin-<当前架构>/TCView.app`，包含 Go sidecar。当前产物使用本机 ad-hoc 签名，属于本机开发包；没有 Developer ID 签名、公证、DMG 或自动更新，也不包含四家 CLI、认证或额度。

在没有本地 CLI／认证的环境下可显式启用 Mock，并使用独立数据目录：

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
| 私有运行与验收产物 | `.tcview/`，不入仓 |

本次不保留旧 NewTC 标识兼容：应用不自动读取或迁移旧用户数据、CLI 设置和布局，不识别旧环境变量或桥接别名。首次使用 TCView 目录时从默认设置开始；旧目录和数据保留在原位置，不自动删除。此调整不修改用户全局 CLI 配置和认证。

合仓前的独立身份调整已通过类型、Go race、RPC、构建环境和完整 40 项 Electron 回归；该结果属于历史验收，不表示合仓验证已经通过。历史截图、导出、构建指纹和真实 CLI 请求记录留在原独立项目的私有 `.newtc/`／`.tcview/` 中，未复制进本仓库。

本次仅导入 TCView 相关源码、锁文件、测试、字体及许可和文档；原独立目录中的安装包、运行数据、草稿、日志与独立的远程代码调查服务需求文档不导入。旧目录保持原样；当前源码与后续开发以本仓库 `desktop/` 为准。
