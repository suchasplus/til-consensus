# CLI 模型、推理配置与真实连接检查

更新日期：2026 年 10 月 20 日。TCView `0.6.0`，协议 `1.4`，SQLite 数据版本 `6`。桌面原名 NewTC；下文“四家真实验收”保留 `0.5.0` 的实际记录，历史 `.newtc` 证据路径和执行时标识不改写；当前操作示例已同步 TCView。

“CLI 与连接检查”现支持四家模型选择、适用的推理档位、保存、恢复默认、模型目录重载及真实请求。设置属于本应用，保存在当前应用数据目录的 SQLite 中，不改用户全局 CLI 配置，不调用登录／登出命令，也不由应用读取或导出认证文件。

## 使用与生效范围

打开设置，等待目录加载，选择模型和可用档位，点击“保存配置”。四家配置一起保存；“恢复并保存默认”删除应用保存项，恢复下表默认值。环境覆盖仍然优先，因此恢复默认不等于移除环境变量。

| CLI | 应用默认模型 selector | 应用默认独立推理覆盖 |
| --- | --- | --- |
| Codex | `gpt-6.1-sol` | 空，使用 CLI 默认 |
| Claude Code | `claude-opus-5-5` | 空，使用 CLI 默认 |
| Grok Build | `grok-4.7` | `low`，沿用此前真实四方验收配置 |
| AGY | `gemini-3.8-flash-medium` | 空，档位包含在模型 ID 中 |

保存只影响之后开始的讨论和话题准备。已开始、停止后恢复的讨论继续使用冻结的 selector、推理请求、CLI 版本及来源；设置页能对照当前有效配置与所选讨论的冻结配置。更改配置会使编辑区已有的待用准备分析失效，避免旧分析与新配置混用。

“使用 CLI 默认”保存为空字符串，表示不传对应覆盖参数，不把它当成实际报告值。模型未知或选择 CLI 默认模型时，应用不能确认其独立推理档位，因此只允许默认推理；可改选目录里的具体模型后配置档位。CLI 本身的既有配置仍可能影响默认值。

四家默认均启用，可以逐家停用不可用的 CLI，再保存。停用项不参加新讨论，不自动读取模型目录，其缺失不会阻止新讨论；仍可手动重载目录或执行有额度提示的真实检查，以便修复后启用。可以保存少于三家、甚至全部停用，但新讨论须至少三家。角色分配与冻结规则见 [桌面交互说明](/Users/suchasplus/devs/agentic/newtc/docs/desktop-interactions.md)。

`0.6.0` 的启用状态、环境覆盖、动态角色、旧冻结兼容、删除和分栏验证已通过；当前验收结果在上述桌面交互说明末尾。下文保留此前四家原生参数和真实请求的完整证据，没有将先前测试改标为本轮三家真实讨论。

## 优先级与来源

同一字段按 **环境变量覆盖 > 已保存应用设置 > 应用默认值** 解析。设置页分别展示已保存值、当前有效值和本次编辑检查将使用的值，标明字段来自默认、保存设置或哪个环境变量。

| 变量 | 用途 |
| --- | --- |
| `TCVIEW_CODEX_ENABLED`、`TCVIEW_CLAUDE_ENABLED`、`TCVIEW_GROK_ENABLED`、`TCVIEW_AGY_ENABLED` | 启用状态覆盖，支持 `true` / `false` 或 `1` / `0`；空值或非法值报错 |
| `TCVIEW_CODEX_MODEL`、`TCVIEW_CLAUDE_MODEL`、`TCVIEW_GROK_MODEL`、`TCVIEW_AGY_MODEL` | 模型 selector 覆盖 |
| `TCVIEW_CODEX_REASONING`、`TCVIEW_CLAUDE_REASONING` | 独立推理请求，需属于所选模型确认支持的档位 |
| `TCVIEW_GROK_REASONING` | 推理请求覆盖，未设置时应用默认 `low` |
| `TCVIEW_AGY_REASONING` | 解析来源以发现冲突；当前桌面方案要求为空，非空会明确拒绝 |

模型或推理变量存在但值为空，同样是显式覆盖，含义为使用 CLI 默认。仅设置空模型但仍有非空推理覆盖时，会提示无法确认组合；不会暗中忽略推理值。被环境变量覆盖的字段仍可保存应用设置，但当前进程仍使用环境值，界面会明确提示。环境变化需要重新启动桌面后台才会继承。

Electron 向 Go 传递允许的环境及 `TCVIEW_*` 配置；Go 为每家 CLI 再按白名单传递该 provider 的认证环境。界面只显示公共模型／推理配置及变量名，不显示凭据值。错误和诊断进行凭据及认证 URL 脱敏，导出仅保留允许的实际元数据字段。

## 四家原生接口

### Codex

本机已核对 `codex-cli 0.163.0` 的帮助和生成的 JSON schema。模型目录通过 app-server `model/list` 获取，使用每个条目返回的 `supportedReasoningEfforts`，不硬编码统一档位。目录可能是客户端目录，不证明当前账号有权调用，真实成功 turn 才是本次访问凭据。

检查、正常讨论和准备分析共用 `codexserver.Run`：在 `thread/start` 指定模型，通过线程 `config.model_reasoning_effort` 和 `turn/start.effort` 传入同一推理选择；空时均不传。`thread/start` 的 `reasoningEffort` 作为 CLI 实际报告独立记录，若报告与显式请求不同则失败；模型重路由也不记为原选择验证成功。没有写入 `~/.codex/config.toml`。

til-consensus 的 exec 路径继续使用 `-m` 和 `-c model_reasoning_effort=...`，已有映射经进程参数回归验证。桌面连接检查已经统一到 app-server。依据：[OpenAI 官方 App Server 文档](https://developers.openai.com/codex/app-server)，以及本机版本生成的协议定义。

### Claude Code

本机 `2.1.303` 使用 `--model` 与 `--effort`。目录通过 SDK stdio 控制协议初始化获取，无需发送推理消息；只提取 `models` 白名单，包括 selector、resolvedModel、显示名及逐模型 effort 支持信息，丢弃初始化里的账号等其他字段。

目录中的 alias 和 CLI 报告的完整 resolved ID 可用于选择，但请求 selector 与实际响应模型仍分开记录。对于未确认模型不开放独立推理覆盖。旧模型或组织设置可能限制档位，不能把某模型的列表套到所有模型；应用也不会将已知会降档的组合视为支持。依据：[Claude 模型与 effort 配置](https://code.claude.com/docs/en/model-config)。

### AGY

本机 `1.4.2` 使用 `agy models` 读取账号目录。界面按实际存在的完整 ID 选择，系列内的档位切换也只引用返回条目，不拼接未知 ID。例如 `gemini-3.8-flash-low`、`-medium`、`-high` 是不同的真实选择。

已用零轮、零 token 的只读 `/effort` 核对：完整 medium ID 配 medium 可用，配 low 会因冲突退出；基础系列配 `--effort low` 能解析 low 变体，没有独立 flag 覆盖完整 ID 后缀的优先级。当前桌面采用一种表达：完整 ID 加空独立 reasoning，仅传 `--model`。材料中或模型名里含有 `low` 不会被记成 CLI 独立报告的实际 effort。

依据：[AGY effort 文档](https://www.antigravity.google/docs/cli/commands/effort)、[非交互模式](https://www.antigravity.google/docs/cli/headless/)，原始目录与只读证据保存在 til-consensus 的参数契约测试资料中。

### Grok Build

本机 `1.0.62` 的 `grok models` 返回可用 selector，模型使用 `--model`，独立强度使用 `--reasoning-effort`。当前已核对 `grok-4.7`／`grok-4.6` 的 low、medium、high、xhigh；`grok-4.5` 只开放前三档，避免把会降档的 xhigh 当成支持；build-fast 未确认独立推理档位时保持默认。

请求 `grok-4.7` 与实际报告的 `grok-4.7-build` 是不同字段。canonical 不写回 selector，不将它作为下一次 `--model` 参数。依据：[Grok CLI Reference](https://docs.x.ai/build/cli/reference)、[模型 reasoning 支持表](https://docs.x.ai/developers/model-capabilities/text/reasoning)。

## 目录、检查和错误

目录会显示加载时间并缓存公开模型字段。缓存关联 CLI 版本，版本变化或旧缓存缺版本时标为过期并提示重新加载，不据过期目录确认动态档位。刷新失败时保留旧成功时间及缓存来源，展示具体且已脱敏的错误；可手工填写模型 ID，但明确保持未验证。

目录查询不发起模型推理，仍需要 CLI 启动和可能的网络请求。Codex 上限 40 秒，Claude／Grok 45 秒，AGY 75 秒；同一家并发重载会复用请求。目录返回不是权限承诺。

“真实检查”始终检查**编辑中的配置 + 环境覆盖**，不自动保存修改。界面区分可执行文件、版本探测、是否派发模型请求、真实请求成功；校验失败不会冒称已经调用模型。不同配置不会共用旧成功结果，同家重复点击被阻止，后台也去重相同进行中的检查。

页面顶部的“一键真实检查（使用额度）”按 Codex、Claude、Grok、AGY 顺序检查全部四家，包含停用项，方便统一排查；不要求先保存编辑。点击时固定这一批的编辑配置，随后仍由同一个单家真实检查接口叠加环境覆盖。每家最多一个任务，串行执行，单家不可用、返回失败或抛出错误后继续检查其余项，无自动重试。汇总显示当前进度、通过和失败数量；各卡片继续展示请求与实际报告以及具体错误。

批次与单家检查共用互斥锁；执行期间禁用重复检查、模型／推理／启用编辑、目录重载、保存和恢复默认。关闭设置弹窗后批次继续，重新打开保留进度、点击时的编辑快照和检查结果。“停止后续检查”不会强杀当前请求，而是等当前调用结束后不再派发余下项，并显示跳过数量。讨论运行时不能发起批次。完整批次最多四次 CLI 任务，仍会消耗服务额度；没有派发请求的配置或版本失败不冒称调用成功。

每次检查最多发起一个外部 CLI 任务，120 秒超时，无自动重试，不替换模型、不降低强度、不忽略参数、不回退 Mock。它会使用服务额度；CLI 内部仍可能产生多次 API 调用。请求模型、实际模型、请求推理、实际报告推理、耗时和错误分别展示。未报告的模型、effort 或费用保持未知，不能用请求值或 token 数推断。

## 数据与标识

TCView 默认后台数据目录为 `~/Library/Application Support/TCView/host`，仅接收 `TCVIEW_*` 应用环境变量；窗口偏好使用 `tcview.*` 键。应用不自动读取或迁移旧 NewTC 目录、配置或偏好，不提供旧环境变量兼容别名；旧数据保留原处。用户全局 CLI 配置和认证不受影响。

配置与目录缓存在应用 SQLite，文件权限为 `0600`，目录受应用数据目录权限保护。升级增加设置表和可选字段，旧配置缺字段采用默认，旧记录继续可读；不会将 Mock 记录转换为真实记录。旧讨论恢复仍执行原来的引擎版本、CLI 版本、模型身份和冻结 hash 兼容检查，不重写旧来源。

Markdown 导出包含请求推理、实际报告推理及配置来源；JSON 保留冻结配置和经过白名单筛选的实际元数据，不包含 raw stdout、stderr、账号响应或凭据。诊断清理、预算、停止恢复和投票版本规则保持原有约束。

## 复验

自动化使用明确标注的 Mock 或协议回放：

```bash
npm test
npm run verify:dependency
```

`verify:dependency` 使用独立空模块缓存、隔离 HOME、无本地覆盖和 `TCVIEW_RELEASE=1` 验证可下载的固定远端模块，不发模型请求。

手动真实验收：

```bash
npm run build:host
npm run verify:cli-settings:live
```

脚本为每家最多发起一次真实检查，不创建整场讨论，使用独立数据目录并验证保存、重启读取、请求／报告区分及无效 AGY 组合拒绝。可以用 `-- --providers=codex` 指定本次提供方；`-- --catalogs-only --providers=agy` 只读取目录，模型请求数为零。原始白名单结果与失败保存在 `.tcview/cli-settings-validation/<时间>/`，不会自动重试或覆盖已有证据。

验证一键按钮的实际调度可以显式执行 `npm run verify:providers:batch:live`，需要先构建开发包。它在独立数据目录配置四家已验证的低强度组合，AGY 保持停用，然后只点击一次“一键真实检查”按钮；每家最多一次真实调用、不自动重试、不创建讨论。汇总与截图保存在 `.tcview/provider-batch-validation/<时间>/`。脚本分别记录批次是否完整执行和每家请求是否通过，单家失败不会被改写成成功。该命令会使用真实额度，不属于默认测试。

## 0.5.0 四家真实验收

四家各发起一次真实模型请求，全部通过；没有重新运行整场多方讨论：

| CLI | 请求 selector／独立推理 | 实际报告模型 | 独立推理实际报告 | 本次耗时 |
| --- | --- | --- | --- | ---: |
| Codex 0.163.0 | `gpt-6.1-sol` / `low` | `gpt-6.1-sol` | `low` | 7.678 秒 |
| Claude 2.1.303 | `claude-opus-5-5` / `low` | `claude-opus-5-5` | 未报告 | 3.233 秒 |
| Grok 1.0.62 | `grok-4.7` / `low` | `grok-4.7-build` | 未报告 | 6.373 秒 |
| AGY 1.4.2 | `gemini-3.8-flash-low` / 无额外参数 | `gemini-3.8-flash-low` | 未报告 | 20.767 秒 |

这些是本次实测，不是稳定时延或所有目录模型的可用性承诺。最初 AGY 目录读取曾失败，手工模型检查仍成功；补充具体错误处理后人工重读目录成功，未再发起模型推理。最终真实目录计数为 Codex 7、Claude 5、Grok 4、AGY 13。目录、失败、四家原始请求报告、构建和原生界面证据见[汇总索引](/Users/suchasplus/devs/agentic/newtc/.newtc/cli-settings-validation/validation.json)。

四次推理的构建使用 `ede1067`，原记录保持该标识；随后上游只修复测试等待，最终锁定 `e9121310e9971179d3d902ee5b2a36bd61eb1553`，模块版本为 `v0.0.0-20261019194904-e9121310e997`。til-consensus 的生产参数 API 已满足需求，本轮增加契约文档和有效回归，没有增加不必要的另一套配置 API。[PR #2](https://github.com/suchasplus/til-consensus/pull/2) 已更新并推送，最终四项远端 CI 全绿，未合并。

NewTC 全包 Go race、类型、7 项构建环境、7 项 stdio RPC 和最终 17 项 Electron 回归通过；最后的目录错误／版本缓存改动另有定向 race。固定远端提交的[空模块缓存发布构建](/Users/suchasplus/devs/agentic/newtc/.newtc/dependency-upgrade/2026-10-19T19-53-11.804Z/summary.json)通过，无本地源码覆盖、保留校验和，模型请求为零。macOS 开发包已重建，精简 PATH 的四家探测与重连通过；原生真实 CLI 模式也完成选择、保存、退出重启持久化及非法输入错误展示，未增加推理请求。此开发包仍为本机 ad-hoc 签名，未发布正式 release。

## 一键检查验收（2026 年 10 月 20 日）

一键检查复用上述原生调用接口，本轮无需修改 Go、协议或 til-consensus。新增 4 项明确 Mock IPC 的 Electron 测试，覆盖全部四家（含停用和不可用项）、编辑快照与环境优先级、请求和实际报告、返回失败或抛错后继续、串行去重、操作锁、关闭重开、停止后续及主动复查。完整 25 项 Electron 回归与类型检查通过。当时开发包以 `NEWTC_RELEASE=1`、无本地 til-consensus 覆盖重建并通过 ad-hoc 签名校验。

实际打包窗口在独立数据目录中只点击一次“一键真实检查”，四家各一次请求均通过，总验收约 42.3 秒；没有重试或发起讨论，renderer 错误为零。AGY 在验收设置中为停用状态，仍按一键按钮声明的范围接受检查。测试配置只写入隔离目录，未修改用户原有设置、全局 CLI 配置或登录状态。

| CLI 实测版本 | 请求模型／独立推理 | 实际模型 | 实际报告推理 | 耗时 |
| --- | --- | --- | --- | ---: |
| Codex 0.163.0 | `gpt-6.1-sol` / `low` | `gpt-6.1-sol` | `low` | 5.75 秒 |
| Claude 2.1.304 | `claude-opus-5-5` / `low` | `claude-opus-5-5` | 未报告 | 3.54 秒 |
| Grok 1.0.62 | `grok-4.7` / `low` | `grok-4.7-build` | 未报告 | 11.32 秒 |
| AGY 1.4.2 | `gemini-3.8-flash-low` / 无额外参数 | `gemini-3.8-flash-low` | 未报告 | 14.40 秒 |

[真实验收汇总](/Users/suchasplus/devs/agentic/newtc/.newtc/provider-batch-validation/2026-10-20T04-06-44.078Z/summary.json)与[打包窗口截图](/Users/suchasplus/devs/agentic/newtc/.newtc/provider-batch-validation/2026-10-20T04-06-44.078Z/desktop-final.png)保留本轮实际结果。用户默认数据目录的窗口已重新打开并显示新按钮，没有在该目录发起测试调用。
