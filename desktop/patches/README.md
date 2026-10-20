# til-consensus 历史接口升级补丁

当前 TCView 正常构建使用已固定升级提交的远程 Go 模块，默认 `GOWORK=off`，不依赖 `~/agentic/til-consensus`，**不需要手动应用本目录的补丁**。构建来源由 [scripts/engine-lock.json](/Users/suchasplus/devs/agentic/newtc/scripts/engine-lock.json) 的 `module`、`moduleVersion`、`commit` 与 [services/host/go.mod](/Users/suchasplus/devs/agentic/newtc/services/host/go.mod) 共同锁定。

当前模块版本为 `v0.0.0-20261019194904-e9121310e997`，固定完整提交 [`e9121310e9971179d3d902ee5b2a36bd61eb1553`](https://github.com/suchasplus/til-consensus/commit/e9121310e9971179d3d902ee5b2a36bd61eb1553)。[核心升级 PR #1](https://github.com/suchasplus/til-consensus/pull/1) 与[跟进 PR #2](https://github.com/suchasplus/til-consensus/pull/2) 均已合并；后者包含格式兼容、Go 1.27、原生 CLI 参数契约测试及慢 CI 等待修复，远端 Linux、macOS、Windows 与 CLI 烟测已全部通过。正常构建直接消费已推送的固定提交，不是正式 tag/release。

## 历史材料

`til-consensus-cli.patch` 保留本轮真实 CLI 审计早期的接口升级，包括原生结构化输出、公开事件、取消、隔离配置、脱敏协议回放及裁决检查点恢复修复。它用于复核当时的开发源码和差异，不是当前版本的安装步骤。

补丁基础提交仍为 `b505773800a46c54d4e32a2c0f79f13b1b416087`。原补丁已在该提交的干净源码上通过 `git apply --check`；基础提交、文件大小和 SHA-256 保留于 [metadata.json](/Users/suchasplus/devs/agentic/newtc/patches/metadata.json)。原补丁和元数据不因当前构建切换而改写，也不应重复套用到已经包含升级的锁定提交上。

历史真实验收产物中的 `+local.<hash>` 是当时源码指纹，不能改标为新的远程提交。当前构建与历史实测分别留存来源。

## 当前构建方式

在 TCView 项目目录正常构建即可：

```bash
npm install
npm run build
```

如需执行发布来源检查，可用：

```bash
env -u TIL_CONSENSUS_DIR TCVIEW_RELEASE=1 npm run build
```

只有开发内核改动时才显式设置 `TIL_CONSENSUS_DIR`，且本地 HEAD 必须等于当前锁文件中的 `commit`。普通开发允许本地修改并记录源码指纹；`TCVIEW_RELEASE=1` 仍拒绝脏的本地覆盖。旧基础提交加历史补丁的 checkout 不等于当前锁定提交，不能沿用早期“应用补丁后直接作为当前本地覆盖”的构建步骤。

更多运行与打包说明见 [实现与验收文档](/Users/suchasplus/devs/agentic/newtc/docs/implementation.md)。
