# 浏览器 Viewer

当前版本已经支持：

```bash
til-consensus view --web
```

它会启动一个**本地只读 HTTP 服务**，默认只绑定 `127.0.0.1`，并打印实际访问地址。默认不会自动打开浏览器；如果你明确需要，可以再加 `--open`。

## 当前范围

这轮实现的是 MVP，目标是把现有 `Document` 直接变成一个可读的本地页面，而不是再造一套 viewer schema。

当前页面固定包含：

- `Overview`
- `Claims`
- `Evidence`
- `Observations`
- `Follow-ups`
- `Debug`
- `Workflow`
- `Files`

其中：

- `Evidence`
  - 聚合 `Verifications`、`Challenges`、`Artifacts`、`Risks`
- `Workflow`
  - 按 mode 展示 `free_debate` 或 `delphi` 的额外结构化块
- `Debug`
  - 展示 `events.jsonl` 里的运行事件
  - 会显式显示：
    - `rawVerdict`
    - `rawTaskVerdict`
  - 也会提示关联的 provider artifact 路径

## HTTP 接口

- `GET /`
  - 页面 HTML
- `GET /api/document`
  - 当前 `Document` 的 JSON
  - 支持 query 参数：
    - `section`
    - `claim_verdict`
    - `limit`
    - `verbose`
- `GET /api/healthz`
  - 返回 `200 ok`

## 设计约束

当前 viewer 明确保持：

- 只读
- 单二进制 Go 实现
- 不依赖远程后端
- 不引入独立前端工程
- 不改已有 `Document` / `result.json` 读取逻辑

当前明确不做：

- artifact 内容在线预览
- 目录浏览器
- 自动刷新 / websocket
- 多用户访问控制
- 默认自动打开浏览器

如果你明确希望自动打开默认浏览器，可以显式加：

```bash
til-consensus view --web --open
```

## 后续更适合继续做的事

- claim 详情页
- artifact 内容预览
- lineage 图
- observation / follow-up 专门视图
- 基于 sqlite session store 的更强查询
