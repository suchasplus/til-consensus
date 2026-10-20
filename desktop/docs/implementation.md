# M1–M3 实现与验收说明

更新日期：2026 年 10 月 20 日，Asia/Kuala_Lumpur。TCView 应用版本 `0.6.0`，桌面协议 `1.4`，SQLite 数据版本 `6`。

桌面应用现名 TCView，共识内核为 til-consensus。应用标识使用 `TCVIEW_*`、`window.tcview`、`tcview.*`、`tcview-host` 和 `local.tcview.desktop`，默认用户数据目录为 `~/Library/Application Support/TCView`。不读取或自动迁移旧 NewTC 目录、设置和偏好，也不删除旧数据。当前桌面项目继续通过固定远程 Go 模块引用内核，仓库结构与依赖来源保持现状。

本文说明当前代码已实现的功能及其验证边界。产品目标和原始里程碑保留在 [设计文档](/Users/suchasplus/devs/agentic/newtc/docs/design.md)；M0 的历史原型记录见 [M0 开发说明](/Users/suchasplus/devs/agentic/newtc/docs/m0.md)。

**当前状态：M1–M3 功能已接入，真实四方自由辩论及停止恢复验收通过。** 本次流程产出三张有效完整票，桌面判定为 `no_consensus`，没有将流程完成冒充意见一致。此前真实运行暴露的 schema、Grok 模型选择、本地校验及恢复问题已修复，失败记录保留。

本次另接入[话题自动选择与补充材料分析](/Users/suchasplus/devs/agentic/newtc/docs/topic-preparation.md)：默认通过 agy / Gemini CLI 检查后选择三种模式，支持文本材料摘要、逐字引文、信息缺口及完整原文冻结。

本轮增加 CLI 启用选择与动态角色、可恢复删除、历史搜索及三栏宽度保存，详见 [桌面交互说明](/Users/suchasplus/devs/agentic/newtc/docs/desktop-interactions.md)。下文真实四方运行及 0.5.0 的实测证据保持原版本、请求次数及 `.newtc` 历史路径；当前操作示例已同步 TCView 标识。

## 已实现范围

| 阶段 | 已接入功能 | 验证边界 |
| --- | --- | --- |
| M1 | 四家真实 CLI；Codex P0；三方独立评审；自由辩论、综合修订、最终投票；方案历史；Markdown/JSON 导出 | 独立审计与真实全场通过：24 次请求、3 张有效完整票、P0/P1/P2 与双格式导出 |
| M2 | 原内核三模式；裁决证据视图；匿名德尔菲和真实评分分布；关联讨论与补充目标；attempt 隔离；冻结配置恢复；缺额与未收敛状态 | 三模式 SIGKILL／恢复、匿名边界、旧票及幂等回归通过；自由辩论已做真实恢复，另两种模式未分别做完整真实多轮实测 |
| M3 | 公开正文增量；任务进度；取消与进程清理路径；按调用保存 usage；运行约束；分支上下文确定性选材；数据迁移；诊断清理；后台重连；macOS 开发包脚本 | 拆包、背压、迁移及错误路径回归通过；真实停止 1.27 秒、无残留；开发包签名／启动／精简 PATH 探测通过 |

Codex 使用 app-server 接收公开正文增量和取消；Claude、Grok、agy 使用各自原生非交互协议。存在可用公开正文增量时，界面逐字展示 `coreOutput.summary`，完成后替换成完整可读消息；原生结构化模式若只在终态给出正文，则先显示任务阶段，完成后显示正文。不会伪造逐字动画，也不会将推理事件、JSON 键或 schema 当聊天正文。

P0 流式摘要和最终正文关联同一 invocation，在提交方案时更新原消息并结束生成状态。三个模式都由同一套 `til-consensus` Engine 驱动，前端不自行调度下一轮。

## 讨论与判定

自由辩论的主要流程为：冻结的提案者起草 P0，独立评审提出主张，交叉辩论，提案者综合，修正评审，最终冻结投票。默认四家为 Codex 提案、其余三家评审；停用一家后为一位提案者、两位独立评审。默认最少两轮、最多三轮辩论，支持分数使用 median 聚合及 `0.67` 阈值；所有冻结评审都必须提交有效完整票，关键主张仍至少需要两位独立评审支持。

桌面判定还检查方案与 manifest 的 hash、独立模型身份、关键主张的支持、未解决阻塞问题和声明的互斥关系。旧版本的票不能批准新内容；存在未解决的阻塞意见时保留 `needs_human`。投票完成后不在同一个 run 内回跳讨论，补充要求通过关联 run 重新起草和投票。

裁决模式保留主张、质疑、证据引用、语义验证与处置状态。模型认可与可复现检查分别记录；事实主张若只有语义支持，不会因此标为事实验证通过。当前讨论不自动修改业务代码或运行实施任务。

德尔菲向参与者提供匿名任务和摘要，父讨论材料和桌面问题上下文也移除已知来源字段、CLI／模型／路径标识；本机所有者的原始审计记录仍能追踪真实来源。用户原始目标中的产品名称保留，确定性文本投影不能保证消除任意隐晦自报身份或文风推断。评分分布来自内核真实响应，不从均分反推。高一致性和高支持分分别检查，低分一致不会判为通过。

## CLI、模型与权限

设置页已支持四家模型与适用推理档位、保存／恢复默认、目录刷新和编辑配置真实检查。环境覆盖优先于保存设置，保存设置优先于默认值；新讨论使用当前有效配置，恢复使用冻结配置。请求与实际报告分开记录，详细界面、变量和限制见 [CLI 设置说明](/Users/suchasplus/devs/agentic/newtc/docs/cli-settings.md)。

正常启动的 `providerMode` 为 `cli`。版本探测只检查可执行文件和 `--version`，不能证明已经登录或某模型可用；连通检查由用户点击触发，会发出真实模型请求。认证继续由各 CLI 管理，应用不读取并导出认证文件。

当前新讨论的默认 CLI 选择 ID 为：

| CLI | 冻结的模型选择 ID | 环境变量覆盖 |
| --- | --- | --- |
| Codex | `gpt-6.1-sol` | `TCVIEW_CODEX_MODEL` |
| Claude Code | `claude-opus-5-5` | `TCVIEW_CLAUDE_MODEL` |
| Grok Build | `grok-4.7` | `TCVIEW_GROK_MODEL` |
| agy | `gemini-3.8-flash-medium` | `TCVIEW_AGY_MODEL` |

CLI 选择 ID 和实际返回的 canonical 模型名可能不同。Grok 的 `grok-4.7` 是有效选择 ID；`grok-4.7-build` 是已观察到的实际报告值，不能拿它替代 `--model` 参数。

每次 run 冻结命令路径、CLI 版本、模型选择 ID、权限范围、允许传递的环境变量名、内核版本及任务材料 hash。`requestedModel` 与 `reportedModel` 分开记录；首次验证成功的 CLI 报告在 `modelBindings` 中绑定 canonical 身份，不修改原冻结配置。之后的 CLI 版本或已绑定 canonical 漂移会中断讨论并提示新建关联 run。实际 canonical 相同的参与者不能产生多张独立票；缺失信息按来源保留，不伪造实际模型或费用。

模型环境变量只影响新 run；恢复使用原快照。将选择 ID 设为空字符串表示使用 CLI 默认选择，后续仍需依据实际报告确认模型身份。

Grok 默认推理强度选择 `low`，目的是在单任务 3 分钟、整次讨论 15 分钟的窗口内减少等待；可用 `TCVIEW_GROK_REASONING` 覆盖，设为空表示使用 CLI 默认强度。推理强度随配置冻结，只影响新 run。参数入口见 [Grok CLI Reference](https://docs.x.ai/build/cli/reference)，模型支持的强度见 [Grok Reasoning](https://docs.x.ai/developers/model-capabilities/text/reasoning)。原默认强度的完整运行曾出现单任务 180 秒超时，原始记录保留；`low` 配置已完成本次完整运行，不能据此保证其他任务的耗时。

| CLI | 当前执行约束 |
| --- | --- |
| Codex | read-only sandbox，禁止审批提权 |
| Claude | 禁用工具，使用隔离工作目录；这不等于 OS sandbox |
| Grok | 禁用工具，使用隔离工作目录；这不等于 OS sandbox |
| agy | `--mode plan`、CLI sandbox 和空工作目录，依赖 CLI 自身约束；不提供 OS 级只读保证 |

环境变量按 provider 白名单传递，避免将另一家的凭据传给当前 CLI。运行记录保存允许的变量名，不保存完整环境变量值。

## 期限、预算与上下文

每个 run 有原始 15 分钟期限；采用准备分析时自该分析开始计时，手动直接开始时自 run 开始计时，停止和等待时间也计入。恢复不会重置期限、预算或已绑定模型身份。

后台最多为一个 run 发起 **30 次 CLI 任务请求**，同一根讨论及其所有后代、兄弟分支共用 **90 次关联链额度**。请求在调用前于数据目录的独占 lease 和 manager 锁内原子预留并持久化，包含采用的准备分析、P0、内核重试及恢复后的请求；重复的 invocation 通知只计一次，预算不足时不执行对应任务。显式 Mock 也走同一预算路径，便于回归边界。创建分支前同样检查整条链，不能用新分支清零额度。独立根讨论分别计数。

新运行将链 ID 和链限额冻结在配置内，恢复和构建版本变化不会重新分配额度；快照公开 `chainCalls`、`chainLimit`。当前策略版本为 `m6-provider-selection-v1`，兼容 `m4-topic-preparation-v1`、`m3-cli-v2-chain-budget` 及缺少链字段的 `m3-cli-v1`：缺省链限额为 90，保留原冻结配置及 hash，已有 invocation 和历史调用总数继续计入，不能恢复归零。原 M0 来源与只读边界不变。

一个 CLI 请求内部可能包含多个服务端 API 请求或内部重试；这些额度不是 API 调用次数、token 上限或费用硬限额。单家连通检查是独立请求，不计入某个讨论或关联链额度。usage 使用每个 invocation 的最新汇总，避免重复累加流事件；CLI 未报告的价格保持未知。

关联 run 冻结父讨论的当前方案、目标和声明的硬约束、关键决策、少数意见、假设、未决主张、全部未解决问题及证据索引；旧消息、旧方案和旧票不会整体塞进新上下文。选材 manifest 记录来源 hash、选择范围和省略范围，保留的必要正文不做静默截断。父材料和补充目标合计超过 128 KiB 时明确报错，需要缩小目标或拆分讨论。

这部分压缩由确定性代码完成，没有额外模型摘要调用。模式内部继续使用内核的活跃主张、轮数和匿名摘要机制；当前没有针对所有任务的通用 token 预算或模型自动压缩。关联 run 有新的期限和自己的 30 次请求上限，同时继续消耗同一根讨论剩余的 90 次链额度；达到链上限后不能再创建关联分支或预留新请求。

## 停止、恢复与持久化

renderer 只通过受限 preload API 调用 Electron 主进程，主进程通过 stdio JSON-RPC 启动 Go sidecar；后台不监听本机 HTTP 端口。事件先持久化再通知，推送缺失时可以按游标补读。UI 断开不会自行取消已经启动的讨论；“重新连接后台”会重启 sidecar，原活动任务会进入可恢复状态。

`run.stop` 异步取消任务，完整退出后再允许继续。恢复使用最后提交的完整阶段或轮次检查点；中断的并行轮次可能重跑，会生成新的 attempt 和产物。已提交的 P0 不重复生成。已完成的内核只补最终判定；运行配置错误不会被最终判定或重启恢复改成成功。

默认后台数据目录为 `~/Library/Application Support/TCView/host`，保存 TCView 自身的数据：

| 位置 | 内容 |
| --- | --- |
| `desktop.sqlite` | 讨论快照、聊天、配置、模型绑定、持久事件、幂等键和准备分析记录 |
| `preparations/<id>/` | 准备分析私有输入／输出诊断 |
| `engine-sessions/` | 内核检查点与恢复状态 |
| `runs/<runId>/` | 冻结配置、内核结果、桌面决策、账本和 artifact manifest |
| `runs/<runId>/artifacts/` | 按 attempt 保存的输入、schema、CLI 输出与诊断信息 |

数据迁移为增量 JSON 字段和 SQLite `user_version=6`。旧 Mock 记录保持 Mock 来源，旧版本不兼容的运行可以查看，不能悄悄改成真实 CLI 继续执行。数据目录有独占 lease，禁止两个后台同时写入。

“清理诊断”要求讨论已停止，删除已知 invocation 的输入/输出诊断文件；聊天、结果、方案版本、检查点、决策、账本和索引保留。清理不等于删除会话，已删除原文的索引也无法再读取原文。导出使用系统保存对话框：Markdown 提供方案、结论、少数意见、假设、投票和证据引用；JSON 另保留内核结果、冻结配置、实际元数据、模型绑定和父上下文 manifest。

在 macOS 上关闭最后一个窗口时，应用留在 Dock，后台可继续工作；Cmd+Q 或退出菜单会关闭后台并保留恢复状态。

## 运行与构建

需要 macOS、Node.js 24 或更高版本和 Go 1.27.1。当前依赖锁定 Electron 44.9.0、React 19.3.1；开发机 Node.js 为 26.8.2。四家 CLI 及登录状态需要在本机提前准备。

```bash
npm install
npm start
```

开发窗口使用 `npm run dev`；`npm run build` 只构建，产物包括 `dist/bin/tcview-host`、Electron 主进程/preload 和 renderer。

默认构建从已固定升级提交的远程 `til-consensus` Go 模块解析依赖。桌面后台 module 为 `github.com/suchasplus/til-consensus/desktop/services/host`，二进制入口为 `services/host/cmd/tcview-host`。构建和 Go 测试脚本使用 `GOWORK=off`，不自动查找 `~/agentic/til-consensus`，也不会使用机器上遗留的 `go.work`。首次构建需要可用的 Go 模块下载源或已有模块缓存；不需要另行准备参考仓库或手动应用历史补丁。

[版本锁](/Users/suchasplus/devs/agentic/newtc/scripts/engine-lock.json) 包含 `module`、`moduleVersion`、`commit`，其中模块版本必须与 [services/host/go.mod](/Users/suchasplus/devs/agentic/newtc/services/host/go.mod) 一致；构建脚本同时核对伪版本提交后缀，拒绝不一致的锁定或依赖覆盖。

| 锁定字段 | 当前值 |
| --- | --- |
| `module` | `github.com/suchasplus/til-consensus` |
| `moduleVersion` | `v0.0.0-20261019194904-e9121310e997` |
| `commit` | [`e9121310e9971179d3d902ee5b2a36bd61eb1553`](https://github.com/suchasplus/til-consensus/commit/e9121310e9971179d3d902ee5b2a36bd61eb1553) |

该提交已推送，Go 已实际解析出上述伪版本，模块声明 Go `1.27.1`。默认构建通过 Go 模块机制下载固定提交。[核心升级 PR #1](https://github.com/suchasplus/til-consensus/pull/1) 与[跟进 PR #2](https://github.com/suchasplus/til-consensus/pull/2) 均已合并；后者包含 Go 1.27、四家 CLI 模型／推理参数契约文档与回归，以及旧诊断产物测试的等待修复。现有生产参数接口已足够，未为桌面设置新增一套不兼容接口。当前提交的四项远端 CI 全部通过。固定版本不是正式 tag 或 release。

`TIL_CONSENSUS_DIR` 仅用于显式本地开发。该路径必须是模块名称正确、HEAD 等于锁定提交且具备所需升级接口的 Git checkout。设置后才在 `.tcview/local-engine/go.work` 中连接源码；干净 checkout 使用锁定 commit 标识，开发修改记录为 `+local.<hash>`。这不会修改远程依赖或自动更新本地仓库。

默认远程模块路径支持发布检查构建：

```bash
env -u TIL_CONSENSUS_DIR TCVIEW_RELEASE=1 npm run build
```

`TCVIEW_RELEASE=1` 不再要求机器必须存在本地参考目录；显式本地覆盖仍必须位于固定提交且工作区干净，脏覆盖会被拒绝。它只约束构建来源，不代替发行签名、公证或发布。

可单独执行固定依赖的冷缓存验收；它会下载真实远程 Go 模块，不调用模型：

```bash
npm run verify:dependency
```

脚本复制独立项目，使用隔离 HOME、空 `GOMODCACHE`、`TCVIEW_RELEASE=1` 和故意无效的旧 workspace，验证正常构建仍走 `GOWORK=off`；随后核对模块目录及版本、执行 `go mod verify`，并只通过后台 `hello` 检查固定 commit 与默认 CLI 模式。已安装的 Node 依赖和 Go 编译缓存会复用，因此这里的冷缓存特指 Go 模块缓存。

先前提交 `cb9149af032e723e12b972d8790c5a97de2c0511` 的独立冷缓存发布来源构建已经通过，记录见 [依赖隔离验收 summary](/Users/suchasplus/devs/agentic/newtc/.newtc/dependency-upgrade/2026-10-19T18-38-17.496Z/summary.json)：无本地 checkout 覆盖、旧 workspace 被忽略，模型请求为 0。当时 pin `f4e2dbad63e409ef8dcf490125f06e522ba886b9` 的同项复验也已通过，独立记录见 [最终模块冷缓存验收](/Users/suchasplus/devs/agentic/newtc/.newtc/dependency-upgrade/2026-10-19T18-44-54.800Z/summary.json)：使用真实远端下载的模块、保留 Go 校验和、发布检查构建成功，后台 `hello.engineVersion` 与锁定提交一致。复验没有调用模型；此前记录和下文真实模型讨论证据分别保留。

| 环境变量 | 用途 |
| --- | --- |
| `TIL_CONSENSUS_DIR` | 仅用于显式本地开发覆盖；未设置时使用固定远程模块及 `GOWORK=off` |
| `TCVIEW_DATA_DIR` | Go 后台数据目录 |
| `TCVIEW_USER_DATA_DIR` | Electron 用户数据目录 |
| `TCVIEW_PROVIDER_MODE` | 默认 `cli`；仅测试显式设为 `mock` |
| `TCVIEW_CODEX_ENABLED` 等四个启用变量 | 新讨论参与选择，默认四家启用，`true/false` 或 `1/0` |
| `TCVIEW_CODEX_MODEL` 等四个模型变量 | 新 run 使用的模型选择 ID |
| `TCVIEW_GROK_REASONING` | Grok 新 run 推理强度，默认 `low`，空值使用 CLI 默认 |
| `TCVIEW_MOCK_DELAY_MS` | 显式 Mock 模式的新 run 延迟 |
| `TCVIEW_RELEASE=1` | 默认固定模块可用；显式本地覆盖须为干净固定提交，不会自动签名、公证或发布 |

改名前 `e9121310e997` 的独立空模块缓存、无本地覆盖、`NEWTC_RELEASE=1` 构建已通过，保留执行时记录：[0.5.0 来源验收](/Users/suchasplus/devs/agentic/newtc/.newtc/dependency-upgrade/2026-10-19T19-53-11.804Z/summary.json)。

## macOS 本机开发包

```bash
npm run package:mac
npm run verify:package
```

打包脚本先构建应用，将 Electron 和 Go sidecar 打入 `release/TCView-darwin-<当前架构>/TCView.app`，写入构建信息，并执行本机 ad-hoc 签名及签名校验。`verify:package` 在独立数据目录及精简 PATH 下启动开发包，验证四家 CLI 的版本探测、设置窗口和后台重连，不发起模型推理。包内不包含四家 CLI、它们的运行环境或认证数据；目标机器仍需自行准备。

当前只生成运行构建命令的 macOS 架构。本机开发包没有 Developer ID 签名、公证、DMG、跨架构构建或自动更新能力，不能作为已完成正式分发的安装包。内核接口升级现已通过模块版本及完整提交锁定；正式分发仍需完成发行签名和公证。

## 验证方式与当前结论

```bash
npm test
```

默认测试包括：Go `-race`、TypeScript 检查、生产构建、真实 stdio 二进制和真实 Electron 窗口。讨论场景显式使用 `providerMode=mock`；CLI 协议解析还使用脱敏录制响应回放和测试子进程。真实 Codex 测试须显式设置 `TCVIEW_LIVE_CODEX=1`；普通 `npm test` 不主动启用它。

自动化覆盖三模式、停止/恢复、幂等、attempt 隔离、旧 Mock 迁移、匿名上下文、P0 单消息提交、投票版本、重复 canonical、模型/CLI 版本变化、请求预算、并发分支链预算及重启不清零、usage 去重、上下文范围、导出和诊断清理；另覆盖自动选模式门禁、准备复用与取消、材料原文冻结、输入变更失效和准备诊断清理。窗口测试的“真实”是指 Electron 窗口与 Go 后台，不代表测试发起了真实模型讨论。

以下操作会发起真实模型请求，不属于默认 CI：设置中的单家连通检查，以及显式执行完整桌面验收脚本。

```bash
npm run build
TCVIEW_LIVE_STOP_RESUME=1 node scripts/live-desktop-smoke.mjs
```

完整脚本使用独立 `.tcview/live-validation/discussion-<时间>/` 数据目录，检查 `providerMode=cli`，保存版本信息、过程快照、截图及 Markdown/JSON 导出。目录名的时间戳使用 UTC，查看记录时应换算为本地 UTC+8。

| 验收项 | 当前结论 |
| --- | --- |
| 四个 CLI 的独立真实请求与 Runner 接口审计 | 已有真实证据，详见接口审计；不能代替完整产品验收 |
| Go race、协议、类型与桌面自动化 | 已通过；构建环境 7 项、RPC 7 项、Electron 17 项；模型使用显式 Mock/录制回放 |
| 话题自动模式与文本材料分析 | 已通过真实 AGY 五类场景；原文指纹和引文核对通过，信息不足和无需讨论被阻止自动开跑；详见话题准备说明 |
| 修复后的四方自由辩论，从 P0 到最终有效票与导出 | 已通过完整流程及离线一致性检查；24 次请求、3 张有效完整票；结果 `no_consensus` |
| 停止、崩溃恢复、无残留子进程 | 三模式进程 SIGKILL 回归通过；自由辩论实际 CLI 停止恢复通过；另两种模式完整真实恢复未单独实测 |
| macOS 开发包构建、签名校验与实际启动 | 已通过；精简 PATH 四 CLI 版本探测、设置加载及后台重连通过；未在开发包内重复发起模型讨论 |

## 本次真实全场证据

- 讨论 ID：`run-3dee8b172f5539ecc9d0fef6`，模式 `free_debate`，来源 `cli`；总耗时约 585.7 秒。
- 24 次 CLI 请求，21 个任务完成，3 个任务在停止验收中取消；恢复后未重做已提交的 P0 和三方独立评审。
- 停止前观测到 9 个 CLI 及其子进程，停止耗时 1270 毫秒；停止后的子进程列表和原 PID 存活列表均为空。这是本轮实测结果，不是所有任务的时延保证。
- 保存 P0、P1、P2 三个方案版本；三方完整 ballot 绑定当前 P2 和同一 manifest，共 11 条冻结主张、33 个逐主张评价。
- 实际模型来源均为 CLI 报告：Codex `gpt-6.1-sol`、Claude `claude-opus-5-5`、Grok `grok-4.7-build`、agy `gemini-3.8-flash-medium`。
- 内核结论 `partial_consensus`，桌面严格判定 `no_consensus`；保留分歧、假设和未解决问题，没有改成自动通过。

[离线验收结果](/Users/suchasplus/devs/agentic/newtc/.newtc/live-validation/discussion-2026-10-19T18-15-50-851Z/validation.json)、[停止恢复记录](/Users/suchasplus/devs/agentic/newtc/.newtc/live-validation/discussion-2026-10-19T18-15-50-851Z/stop-resume.json)、[Markdown 报告](/Users/suchasplus/devs/agentic/newtc/.newtc/live-validation/discussion-2026-10-19T18-15-50-851Z/discussion.md)、[完整 JSON 导出](/Users/suchasplus/devs/agentic/newtc/.newtc/live-validation/discussion-2026-10-19T18-15-50-851Z/discussion.json)、[真实桌面截图](/Users/suchasplus/devs/agentic/newtc/.newtc/live-validation/discussion-2026-10-19T18-15-50-851Z/desktop.png)。

复核保存证据不会再次调用模型：

```bash
node scripts/validate-live-run.mjs .newtc/live-validation/discussion-2026-10-19T18-15-50-851Z
```

[开发包启动验证](/Users/suchasplus/devs/agentic/newtc/.newtc/package-validation/2026-10-19T18-21-21.175Z/summary.json) 与 [交付日志和构建指纹](/Users/suchasplus/devs/agentic/newtc/.newtc/delivery-validation/summary.json) 单独保存。当时本地开发阶段的补丁及干净基线应用检查保留在 [历史补丁说明](/Users/suchasplus/devs/agentic/newtc/patches/README.md)。这些真实验收记录及其构建指纹维持原样；当前正常构建已改用固定远程模块，不再手动应用该补丁。


本次已打开的验收窗口使用上述独立目录中的 `host/` 与 `electron/`。常规 `npm start` 使用默认用户数据目录；原 M0 历史继续保留。
