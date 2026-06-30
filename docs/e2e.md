# E2E 测试设计

这篇内容已收敛到 [测试、CI 与发布](testing.md)。

请优先阅读：

- [测试、CI 与发布](testing.md)
- [Provider 配置与预检](providers.md)

常用入口：

```bash
make test-e2e
TIL_CONSENSUS_E2E_REAL_CLI=1 make test-e2e-real
TIL_CONSENSUS_E2E_REAL_API=1 make test-e2e-real-api
```
