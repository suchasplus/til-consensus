# CLI 模型与推理参数契约

本文记录 2026-10-20 对本机 Claude Code 2.1.303、AGY 1.4.2、Grok Build 1.0.62 的接口核对。可用模型随账号和 CLI 版本变化，本文列举的目录不是永久白名单。

## 现有接口

通用配置继续使用 `ProviderModelConfig.ProviderModel` / `Reasoning`；单次调用继续使用 `RunTaskDetailed` / `PreviewCommand` 的 `providerModel` / `reasoning` 参数，无需为桌面设置增加另一套参数结构。

| CLI | 非空模型参数 | 非空推理参数 | 两字段为空时 |
| --- | --- | --- | --- |
| Claude Code | `--model <selector>` | `--effort <level>` | 不传对应覆盖参数 |
| Codex exec | `-m <selector>` | `-c model_reasoning_effort=<level>` | 不传对应覆盖参数 |
| AGY | `--model <目录中的完整 ID>` | `--effort <level>` | 不传对应覆盖参数 |
| Grok Build | `--model <selector>` | `--reasoning-effort <level>` | 不传对应覆盖参数 |

空字符串表示继承 CLI 自身的有效设置，不是某个确认过的模型或推理值。CLI 自身仍可能读取用户或组织设置；库不改写全局 CLI 配置。`RunTaskResult.Command` 记录实际启动参数，`PreviewCommand` 使用相同构造路径，但它们只能证明请求参数，不能证明服务端实际执行值。

模型目录、应用设置持久化、环境变量优先级和讨论配置冻结由宿主应用负责。库保留字符串参数以兼容不同供应商的语义，不把所有 CLI 的档位合并成一个跨模型通用枚举。对于 CLI 会降档的已知组合，宿主应在请求前按模型能力拒绝；未知模型的可用性须经真实调用验证，不能静默回退到 Mock。

Codex app-server 是另一套协议：使用该协议的宿主必须显式设置协议中的模型和推理字段。修改这里的 `codex exec` 映射不会自动改变 app-server 请求。

## Claude Code

本机 `claude --help` 暴露 `--model` 和 `--effort`。可不发送用户推理消息，仅用 SDK 控制协议初始化读取模型目录：启动 `claude --print --input-format stream-json --output-format stream-json --verbose --safe-mode`，向 stdin 写入单个初始化请求后关闭输入：

```json
{"type":"control_request","request_id":"model-catalog","request":{"subtype":"initialize"}}
```

本机响应路径为 `control_response.response.response.models`，各项包括 `value`、`resolvedModel`、`displayName`、`supportsEffort` 和 `supportedEffortLevels`。**初始化响应还含账号等信息，宿主只能提取所需模型字段，不应整体存储、转发或记日志。** 这次目录包含 `default` / `opus`、`sonnet`、`haiku` 和 Fable；实际支持范围须优先采用该次响应。

官方说明模型档位有差异，例如 Opus 4.6 / Sonnet 4.6 不支持 `xhigh`，CLI 会将它降为 `high`。组织上限也可能限制请求档位。不能用请求参数替代实际报告值。详见 [Claude Code 模型与 effort 配置](https://code.claude.com/docs/en/model-config)。

## AGY

优先运行 `agy models` 读取当前账号目录。本机返回的系列包括 Gemini 4 Argon、Gemini 3.8 Flash、Claude Opus 5.5、Claude Sonnet 5.5 的 `low` / `medium` / `high` 变体，以及 `gpt-oss-120b-medium`。目录原文作为有日期和版本的测试资料保存于 `runtime/cli/testdata/agy/models-1.4.2.txt`；不能据此臆造 `max` 或其他未返回 ID。

适合宿主的保守策略是：按目录中的“系列 + 档位”选择一个**实际存在的完整 ID**，仅传 `--model`，将独立 `reasoning` 留空。Runner 原样传递 selector，既不删后缀，也不自动追加 `--effort`。

官方文档说明 `--effort` 仅覆盖当前调用，支持档位取决于模型发布的变体；交互 `/effort` 可保存全局默认，宿主不可用它写设置。只读 `agy --print /effort --output-format json` 可检查当前档位。详见 [AGY effort 命令](https://www.antigravity.google/docs/cli/commands/effort) 和 [非交互模式](https://www.antigravity.google/docs/cli/headless/)。

本机 1.4.2 已通过只读 `/effort` 实测：

| 请求配置 | 原生结果 |
| --- | --- |
| `--model gemini-3.8-flash-medium` | `current: medium`，可选 `low` / `medium` / `high` |
| 同一完整 ID 加 `--effort medium` | 同样成功，重复配置不改变选择 |
| 同一完整 ID 加 `--effort low` | 退出 1，`ERROR`：`--model ...-medium conflicts with --effort=low` |
| `--model gemini-3.8-flash --effort low` | 成功解析系列与档位，`current: low` |

这些命令的全部 token 使用量和 `num_turns` 均为 0，不是推理成功凭据。相应原始 fixture 与 SHA-256 保存在 `runtime/cli/testdata/agy/model-settings-provenance.json`。冲突**不会由 `--effort` 自动覆盖模型后缀**，宿主无需依赖这种重复表达方式。

## Grok Build

`grok models` 本机返回 `grok-4.7`（默认）、`grok-4.7-build-fast`、`grok-4.6`、`grok-4.5`。这些是请求 selector。真实返回统计中的 `grok-4.7-build` 是 canonical 名称，不能反向替换 `--model grok-4.7`；Runner 不做这类重写。

本机帮助确认 `--reasoning-effort`，其别名为 `--effort`。官方当前列出 `grok-4.7` / `grok-4.6` 的 `low` / `medium` / `high` / `xhigh`，`grok-4.5` 仅前三档；4.5 的 `xhigh` 会被当成 `high`。`build-fast` 未经确认的独立推理档位应保持禁用。参见 [Grok CLI 参数](https://docs.x.ai/build/cli/reference) 和 [模型 reasoning 支持表](https://docs.x.ai/developers/model-capabilities/text/reasoning)。

## 请求、报告与验证边界

`CLIEvent.Model` 只来自协议中实际出现的 model 字段；`RawOutput` 可能包含推理内容或其他私有数据，只能用于受控诊断，不能直接进入聊天、导出或下一轮 prompt。没有返回独立推理强度时应显示“未报告”，不能从请求档位、token 数或模型后缀推断。

新增回归通过本地明确标注的进程替身，检查完整 Runner 路径最终 argv、CLI 默认值省略、AGY 完整 ID 和 Grok selector 不被重写；真实 AGY 错误 fixture 验证零轮模型冲突不会被解释成成功。它们不代替四家真实推理验收，也不宣称所有目录模型均已成功调用。
