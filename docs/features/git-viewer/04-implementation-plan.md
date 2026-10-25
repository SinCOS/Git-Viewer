# 04 — 实施计划

## 实施原则
- 【原文明确】macOS 优先、OpenSSH、safe 写操作、hybrid 刷新。
- 【建议】先 POC、后只读、再写入；每阶段均有测试、文档和可演示产物；不以估计天数替代验收。
- 每阶段必须经过用户确认才能进入下一阶段的较大范围开发。**授权更新：用户已在剩余任务编排会话选择 B，授权 Phase 2、W、3–7 及全部未完成补充项；无需逐阶段重复确认，依赖与验收不变。**

| 阶段 | 工作范围 | 交付与验收 | 依赖 |
|---|---|---|---|
| 0 | 初始化、技术 POC、风险验证、schema | 可运行 MyGo 纯 Go 原生窗口；本地/SSH Git 原型；关键技术决策记录 | 无 |
| 1 | 本地仓库、Status、Diff | 本地仓库可打开；双区状态正确；Diff 正确 | 0 |
| 2 | SSH Config、远程仓库、统一 Executor | 同一界面切换本地/远程，错误和超时可识别 | 1 |
| W | VS Code Workspace 读取、创建、保存 | VS Code 文件兼容；JSONC 保留；外部修改不覆盖 | 2 |
| 3 | History、Branch、Worktree | 可分页查看历史、分支、worktree | W |
| 4 | Git Graph | 分叉/合并正确，历史可增量加载 | 3 |
| 5 | Stage/Unstage/Commit、安全切换分支 | 无误暂存；hook 错误正确；危险操作拒绝 | 4 |
| 6 | Fetch/Pull/Push | 上游/冲突/认证/失败反馈正确 | 5 |
| 7 | 混合刷新、性能、回归、macOS 发布 | 监听/轮询稳定；大仓库不卡 UI；可安装包 | 6 |

## 横向质量门槛
1. 每个命令：输入校验、超时、取消、错误映射、脱敏日志。
2. 每个解析器：空仓库、中文/空格/换行文件名、重命名、冲突、异常输出测试。
3. 每个 UI 模块：加载中、空状态、错误、过期缓存、手动刷新。
4. 每个写操作：操作前检查、串行化、操作后刷新、失败状态复核。
5. 每阶段补充真实代码映射与决策记录，不允许规划文档与代码长期分叉。

## 发布验收
- macOS Intel 与 Apple Silicon 的目标支持范围确认并测试。
- 签名、公证、自动更新策略在发布前决定。
- SSH host key 验证、凭据保护、危险命令黑名单、安全测试通过。

## 不在首版范围
仓库 Clone、Git 托管服务 API、交互式 merge/rebase 冲突解决器、行级 stage、完整代码编辑器、远程守护进程。


## Workspace 插入阶段：W（P1）
【原文明确】支持 `.code-workspace` 的读取、创建、保存。
【建议】阶段 W 位于阶段 2（本地和 SSH Git 基础能力可用）之后、阶段 3（History/Branch/Worktree）之前；与 Git Graph 和写入功能解耦。

| 子阶段 | 工作范围 | 交付验收 |
|---|---|---|
| W0 | JSONC 解析与编辑 POC、schema/DTO | 现有 VS Code 文件 round-trip 无配置损失 |
| W1 | 打开 Workspace、路径解析、仓库发现 | 多目录与非 Git 目录可显示 |
| W2 | 创建 Workspace、保存及冲突检测 | VS Code 可打开；保留已有设置、注释、未知字段 |
| W3 | Workspace UI、最近列表、集成测试 | Local/SSH 仓库入口与 Workspace 不混淆 |

阶段 3 开始前必须通过 W 的验收。此阶段不实现非标准 SSH URI 写入 `.code-workspace`。

## v1.2 MyGo 迁移门槛
- Phase 0 首先完成 MyGo 原生 `ui.View` 最小窗口、原生分栏、虚拟列表、Graph 绘制、异步任务与 macOS 构建验证。
- 原 Tauri/Rust 工程任务全部由 Go/MyGo 等价任务替代，不再建立 `src-tauri`。
- Phase 1–7、W 阶段的业务范围、依赖和验收条件不变；先跑通框架与 SSH POC，再进入业务开发。


## v1.3 原生 UI 阶段验收补充
- 【原文明确】Phase 0 起全部采用 Go 原生 UI，不生成任何 Vue/TS/WebView 项目。
- 【建议】Phase 0 先做三个独立 POC：三栏可调整布局、1 万条提交的虚拟列表、200 个可见节点的 Git Graph 绘制及滚动；记录机器配置和性能。
- 【建议】Phase 1 原生 Status/Diff；Phase W 原生 Workspace 打开/新建/保存；Phase 4 原生 Git Graph；Phase 5/6 原生确认对话框及操作进度。
- 【待确认】首版 Diff 是否需要双栏行内高亮，作为独立 UI 里程碑决定。
