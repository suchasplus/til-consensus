# 终端 `view` 用法

这篇内容已收敛到 [操作手册](operations.md)。

请优先阅读：

- [操作手册：查看结果](operations.md#查看结果)
- [操作手册：Verbose / debug](operations.md#verbose--debug)
- [输出产物](outputs.md)

常用入口：

```bash
til-consensus last --config ./til-consensus.yaml
til-consensus inspect tc_xxx --config ./til-consensus.yaml
til-consensus view --result ./out/tc_xxx/result.json --section debug --verbose
```
