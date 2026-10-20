# til-consensus 历史接口升级补丁

当前 TCView 位于本仓库 `desktop/`，通过根 `go.work` 使用同一 checkout 的公开 Go 内核，**不需要手动应用本目录的补丁**。构建来源与兼容身份见 [同仓开发说明](../docs/monorepo.md)。

## 历史材料

`til-consensus-cli.patch` 保留真实 CLI 审计早期的接口升级，包括原生结构化输出、公开事件、取消、隔离配置、脱敏协议回放及裁决检查点恢复修复。它用于复核当时的源码和差异，不是当前版本的安装步骤。

补丁基础提交为 `b505773800a46c54d4e32a2c0f79f13b1b416087`，曾在该提交的干净源码上通过 `git apply --check`；基础提交、文件大小和 SHA-256 见 [metadata.json](metadata.json)。原补丁与元数据不因合仓而改写，也不应重复套用到已包含升级的源码。

独立项目阶段最终引用远端模块 `v0.0.0-20261019194904-e9121310e997`，完整提交 [`e9121310e9971179d3d902ee5b2a36bd61eb1553`](https://github.com/suchasplus/til-consensus/commit/e9121310e9971179d3d902ee5b2a36bd61eb1553)。[核心升级 PR #1](https://github.com/suchasplus/til-consensus/pull/1) 与[跟进 PR #2](https://github.com/suchasplus/til-consensus/pull/2) 已合并。

历史真实验收中的 `+local.<hash>` 是执行当时的源码指纹，不能改标为新的提交或当前同仓身份。私有原始记录留在原独立项目，没有导入仓库。

## 当前构建

从本仓库根目录执行：

```bash
cd desktop
npm ci
npm run build
```

发布来源检查使用 `TCVIEW_RELEASE=1 npm run build`，要求干净且可追踪的同仓 checkout。无需 `TIL_CONSENSUS_DIR` 或本地补丁。更多运行、测试及打包说明见 [实现与验收文档](../docs/implementation.md)。
