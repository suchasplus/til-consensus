# IBM Plex 本地字体

字体来自 IBM 官方发布的 npm 包，不在运行时请求字体服务，不添加新的 npm 运行依赖。

| 包 | 固定版本 | 本应用使用 |
| --- | --- | --- |
| `@ibm/plex-sans` | `1.1.0` | Regular 400、Medium 500、SemiBold 600 |
| `@ibm/plex-mono` | `2.5.0` | Regular 400 |

上游：[IBM/plex](https://github.com/IBM/plex)。使用原始 `fonts/complete/woff2/` 文件，没有修改字体或名称。各文件大小和 SHA-256 见 `sources.json`。

字体许可为 OFL-1.1，完整许可位于 `../../public/licenses/IBM-Plex-OFL-1.1.txt`，构建时复制为 `dist/renderer/licenses/IBM-Plex-OFL-1.1.txt`，随桌面包分发。中文使用系统字体回退。
