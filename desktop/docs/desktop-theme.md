# IBM 商务风格主题

更新日期：2026 年 10 月 20 日。TCView 默认桌面主题参考 IBM Carbon 的配色、排版与界面密度，使用应用自有标识。TCView 是 til-consensus 的桌面客户端，原名 NewTC；历史截图和验收记录保留执行时的名称、环境标志及 `.newtc` 路径，当前构建命令使用 `TCVIEW_RELEASE=1`。

## 视觉约定

- 石墨色导航和产品顶栏，灰白交替的内容层级，IBM 蓝作为主要操作和焦点色。
- 方角控件、清晰的 1px 分界和网格，减弱装饰阴影。品牌和欢迎图改为正交节点与连接线。
- 正文保持中性色，提高辅助信息对比度；错误、警告、成功和匿名身份保留各自的语义颜色。
- IBM Plex Sans 用于拉丁界面文字，IBM Plex Mono 用于模型 ID、指纹和代码；中文使用系统字体回退。所有字体本地打包，不依赖在线字体服务。
- 统一覆盖历史、聊天、材料准备、讨论产物、投票和 CLI 设置，包括一键检查进度与错误提示。

主题改变视觉与部分标题文案，沿用原来的设置、拖宽持久化、键盘操作、讨论和检查调用链。没有新增主题选择字段、后台数据迁移或模型请求。

## 下拉选择器

讨论模式、话题检查提供方、方案版本、四家 CLI 的模型与推理档位统一使用应用绘制的 Carbon 风格选择器。触发器和展开菜单均采用方角、灰白层级、分隔线与蓝色焦点，不再弹出系统原生选择菜单。模型列表仍来自原有 CLI 目录，手工模型入口和未验证提示保持不变。

点击或按 Enter／空格打开菜单，上下方向键切换候选，Home／End 跳到首末项，Enter／空格确认；直接输入选项开头的文字可快速定位。浏览候选时不会改变已选配置。Escape 取消当前菜单并保留焦点，再次 Escape 才关闭外层设置或产物抽屉；Tab 关闭菜单并继续移动焦点。点击菜单外部可关闭。

菜单独立浮在内容上方，按可用空间向上或向下展开，长目录在菜单内滚动，长模型 ID 可换行。折叠对应区域或控件变为禁用时会关闭展开菜单。编辑目录外的手工模型时，即使清空原 ID，输入框也保持打开，保存前仍校验非空与格式。配置保存、环境覆盖、运行冻结和真实检查仍沿用既有行为。

下拉改造完成后，类型检查、默认远端依赖构建、全部 38 项 Electron 回归及 8 项 stdio RPC 检查通过。无本地源码覆盖的 `NEWTC_RELEASE=1` 开发包已重建、签名并通过精简 PATH 版本探测／重连验收。原生窗口实际检查模式菜单、Codex 真实模型目录、推理档位和底部提供方向上展开；键盘浏览后取消，没有保存配置或发送模型请求。原未发送草稿、AGY 检查提供方与左右折叠偏好均保持。[下拉验收与实际截图](/Users/suchasplus/devs/agentic/newtc/.newtc/dropdown-validation/2026-10-20T07-04-07.756Z/summary.json)。

## 文件和字体

主要样式位于 `apps/desktop/renderer/styles.css`，共用下拉组件为 `CarbonSelect.tsx`，字体声明在 `fonts.css`，品牌与欢迎 SVG 在 `App.tsx`。Electron 启动背景和 HTML theme-color 同步调整，CSP 保持原约束。

字体取自官方 `@ibm/plex-sans@1.1.0` 与 `@ibm/plex-mono@2.5.0`，只收录所需的四个 WOFF2，共 246,068 字节；来源、校验值与版权信息保存在 `assets/fonts/`，OFL-1.1 完整许可随开发包分发。

## 设计依据

- [Carbon 配色与层级](https://www.carbondesignsystem.com/building-blocks/foundations/color/overview)：中性色组织内容，蓝色标识主要操作，交互焦点明确可见。
- [Carbon 字体与排版](https://www.carbondesignsystem.com/building-blocks/foundations/typography/overview)：采用 IBM Plex 与适合产品界面的字号层级。
- [IBM Plex 官方字体](https://github.com/IBM/plex)：原字体与 OFL-1.1 许可来源。

## 验证

主题回归使用明确的 Mock 模式，覆盖默认窗口、窄窗口、三栏调宽、设置滚动、抽屉和键盘焦点；外部字体访问被阻断时，本地字体仍需正常加载。截图与最终验证记录见本轮交付记录。

本轮类型检查、默认构建和完整 **27 项 Electron 回归**通过。主题专项覆盖 `1440×920` 与 `820×760`，阻断外网后本地字体仍正常加载，无横向溢出。运行时 64 项文字采样均达到 4.5 对比度以上，最低约 4.55；该采样不等于完整无障碍认证。

macOS 开发包以 `NEWTC_RELEASE=1`、无本地内核覆盖重建，ad-hoc 签名及四个字体／OFL 许可均验证通过。精简 PATH 下 CLI 版本探测、设置页和后台重连通过，没有渲染错误。原用户数据目录已用新主题重新打开，三栏首选宽度及更新前的 Codex 分析选择已保留。本轮新增真实模型请求为 **0**。

- [主题验收汇总](/Users/suchasplus/devs/agentic/newtc/.newtc/theme-validation/2026-10-20T05-50-04.342Z/summary.json)
- [真实桌面窗口](/Users/suchasplus/devs/agentic/newtc/.newtc/theme-validation/2026-10-20T05-50-04.342Z/native-carbon-workspace.png)
- [配置页与讨论截图](/Users/suchasplus/devs/agentic/newtc/.newtc/theme-validation/2026-10-20T05-50-04.342Z)
- [最终文字对比度采样](/Users/suchasplus/devs/agentic/newtc/.newtc/theme-validation/2026-10-20T05-46-58.363Z/contrast-audit-final.json)
- [开发包验收](/Users/suchasplus/devs/agentic/newtc/.newtc/package-validation/2026-10-20T05-51-25.729Z/summary.json)
