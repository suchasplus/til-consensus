# til-consensus Go v1

## 兼容边界

- v1 兼容的是多 agent 共识编排语义，不追求对参考 TypeScript 项目的 CLI 和配置完全兼容。
- v1 保留文件产物：
  - `result.json`
  - `events.jsonl`
  - `summary.md`
- v1 不提供 viewer 打开能力，也不保证现有 viewer 能直接消费结果 JSON。

## 当前 provider

- `mock`
- `openai`
- `command`

## 当前限制

- 暂不提供 E2E
- 暂不提供公共 SDK
- 暂不实现外部 reporter / actor 的独立路由
