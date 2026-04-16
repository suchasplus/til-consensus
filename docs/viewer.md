# 浏览器 Viewer 二期规划

当前版本的主查看入口仍然是 `til-consensus view` 的终端输出。

浏览器 Viewer 不在这一轮交付里，但实现路线已经固定。

## 目标

- 保持 Go 单二进制
- 不引入独立前端工程
- 不依赖远程后端
- 只读取本地输出产物
- 按 `mode` 渲染 `adjudication`、`free_debate`、`delphi`

## 入口形式

预留命令：

```bash
til-consensus view --web
```

二期会由这个命令启动一个本地只读 HTTP 服务，再自动打开浏览器。

## 技术路线

- Go `html/template`
- 少量原生 JS
- 数据源固定为：
  - `result.json`
  - `ledger.jsonl`
  - `artifacts/manifest.jsonl`

## 页面结构

一期页面固定包含：

- `Overview`
  - 当前 mode
  - 任务结论
  - 关键统计
  - 主要风险
- `Claims`
  - `adjudication` 的 claim graph
- `Debate`
  - `free_debate` 的轮次与 votes
- `Delphi`
  - `delphi` 的 statements、收敛度、异议摘要
- `Evidence`
  - ledger 时间线
  - verification 结果
  - artifact 引用
- `Artifacts`
  - log
  - diff
  - benchmark
  - raw worker output

## 测试要求

二期实现时至少要有：

- HTTP handler 测试
- 模板渲染 golden tests
- view model 构造测试
- 缺失 ledger / manifest / artifact 的降级测试
