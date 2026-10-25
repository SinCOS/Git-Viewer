# 05 — Agent 可执行任务清单

状态：Phase 0 的 T001–T005 已完成；补充原生 POC 已实施，跨架构实机与发布项保留未完成。逐任务测试证据见 [Phase 0 执行记录](phase-0-execution.md)。

## 阶段 0：工程与技术验证

执行顺序：T001 → T002 → T003 → T004 → T005。原始核对保留于 [Phase 0 核对记录](phase-0-review.md)；当前以执行记录为准。

补充任务归属：MY001–MY006、NU001、NU003、NU004、NU006、NU009 属 Phase 0；NU002 在 Phase 0 仅做模拟仓库三栏/菜单/快捷键 POC，正式切换器留 Phase 1/2。NU005 在 Phase 0 仅做静态文本 POC，正式 Diff 留 Phase 1。MY007 分拆为 Phase 0 的监听/原子 JSON 技术 POC 与 W 阶段 JSONC；NU007 完整归 W。MY008/NU008 记录本机启动与双架构构建，另一架构真机启动及完整发布验收留待对应环境。
- [x] T001 初始化 MyGo 原生 UI + Go；设置 lint/test/build CI。验收：macOS 本地运行、构建成功。
- [x] T002 定义 GitOperation、RepositoryContext、Error、DTO；建立 schema 契约测试。验收：Go JSON DTO 与 JSON Schema 一致。
- [x] T003 本地 Git 子进程 POC；捕获 stdout/stderr/exit、超时与取消。验收：命令错误可区分。
- [x] T004 OpenSSH POC；支持 SSH Config、host key 校验、安全远端命令编码。验收：恶意/特殊路径不触发注入。
- [x] T005 Git status -z / history / worktree parser POC。验收：特殊文件名、空仓库和合并历史测试通过。

## 阶段 1：本地只读
- [ ] T101 本地仓库选择、验证、最近记录。
- [ ] T102 Git Status 解析与 UI，区分 staged/unstaged/untracked/conflicted。
- [ ] T103 工作区、暂存区、提交 Diff API 与查看器；二进制/大文件降级。
- [ ] T104 本地只读端到端测试。

## 阶段 2：SSH 远程
- [ ] T201 SSH Host 配置、连接测试、远端 Git 探测。
- [ ] T202 SshExecutor、命令白名单、安全参数编码、取消/超时。
- [ ] T203 远程仓库注册与路径校验，复用 Status/Diff UI。
- [ ] T204 SSH 断线、认证、host key、权限、特殊路径测试。

## 阶段 3：History/Branch/Worktree
- [ ] T301 历史分页与提交详情。
- [ ] T302 本地/远程分支与 upstream 解析。
- [ ] T303 Worktree 列表、锁定、detached 状态及 common-dir 关系。
- [ ] T304 分支/Worktree 场景测试。

## 阶段 4：Graph
- [ ] T401 构建 commit DAG 和 lane 布局算法。
- [ ] T402 图形渲染、虚拟列表、分页加载。
- [ ] T403 merge、octopus、孤立分支、大仓库性能测试。

## 阶段 5：安全写入
- [ ] T501 同仓库写操作队列、前置检查、失败后状态复核。
- [ ] T502 按文件 Stage/Unstage；精确 pathspec。
- [ ] T503 Commit 表单、消息安全传递、hook 失败处理。
- [ ] T504 安全分支切换；拒绝覆盖未提交改动及已占用分支。
- [ ] T505 本地/SSH 写操作集成测试。

## 阶段 6：远程同步
- [ ] T601 Fetch 及进度/认证/超时处理。
- [ ] T602 Pull --ff-only 与冲突/非快进反馈。
- [ ] T603 普通 Push、上游检查、拒绝推送反馈；禁止强制推送。
- [ ] T604 远端状态未知与失败后复核测试。

## 阶段 7：刷新与发布
- [ ] T701 本地文件监听、去抖、Git 元数据监听、兜底刷新。
- [ ] T702 远程轮询、前后台策略、退避、离线 stale 标记。
- [ ] T703 大仓库、网络波动、资源占用、回归测试。
- [ ] T704 macOS 构建、安装、签名/公证决策与发布说明。

## Agent 协作约束
- Planner：仅调整需求、依赖与验收，不直接改业务代码。
- Explorer：只读探索现有实现，先更新 02-system-analysis.md。
- Architect：维护 03-technical-design.md 与 schemas；变更契约需同步两端。
- Implementer：一次领取一个或一组无冲突任务，使用独立分支/worktree。
- Reviewer：审查安全、Git 边界、SSH 注入风险与测试证据。
- 禁止多个 Agent 同时编辑同一文件；合并前完成 lint、test、build。

## 完成定义（DoD）
- 实现通过测试；有错误路径测试；更新文档和契约；用户能复现验收步骤；不引入未确认的破坏性 Git 操作。


## 阶段 W：VS Code Workspace（在阶段 2 后、阶段 3 前执行）
- [ ] TW001 JSONC 解析与最小文本编辑 POC；验证注释、尾逗号、未知字段 round-trip。验收：不会丢失 settings/extensions 或注释。
- [ ] TW002 定义 WorkspaceDocument/WorkspaceFolder/WorkspaceView DTO 和 JSON Schema；Go/JSON Schema 契约测试。
- [ ] TW003 实现 Workspace 打开、路径解析、目录存在性检查、Git 仓库发现与去重。验收：多根目录、相对路径、非 Git 目录、软链接。
- [ ] TW004 实现 Workspace 创建及保存；revision 冲突检测、原子写入、失败恢复。验收：外部编辑不被静默覆盖。
- [ ] TW005 实现 Workspace 选择/管理 UI、最近 Workspace；与 Local/SSH 仓库切换集成。
- [ ] TW006 端到端测试：真实 `.code-workspace` 文件、特殊路径、JSONC、外部修改冲突、权限错误及 VS Code 兼容性。

依赖：T001–T005、T101–T104、T201–T204。TW001 的 POC 可提前并行验证，但正式功能在阶段 2 后验收。

## v1.2 MyGo 技术任务（覆盖旧框架任务）
- [x] MY001 检查 MyGo 版本、文档与环境要求，固定 Go/MyGo 版本；记录兼容性矩阵。
- [x] MY002 初始化 MyGo `ui` 原生 Go 项目，macOS 打开主窗口。
- [x] MY003 原生 Go 视图调用 `Health/Version` 服务，验证错误反馈与状态更新。
- [x] MY004 验证 goroutine 到原生 UI 的安全状态调度、取消、加载态与窗口关闭清理。
- [x] MY005 用 `exec.CommandContext` 实现本地 Git 只读 POC，含超时、特殊文件名和输出限额。
- [x] MY006 通过系统 OpenSSH 验证远程 Git 只读 POC，测试路径转义、断线、host key 校验。
- [ ] MY007 验证 MyGo 文件监听、本地 JSON 原子保存、`.code-workspace` JSONC 读写。**部分完成：监听、原子 JSON 保存及清理测试通过；JSONC 留 W。**
- [ ] MY008 完成 macOS Intel/Apple Silicon 构建和打包冒烟测试，并更新系统预分析。**部分完成：双架构构建/打包/签名校验、Intel 启动通过；arm64 真机启动待验证。**


## v1.3 原生 UI 专项任务（替代 WebView/前端任务）
- [x] NU001 使用官方 native 模板建立最小窗口，确认无 WebView/HTML/JS/TS 依赖；锁定版本。
- [x] NU002 实现可调整三栏原生布局、工具栏、菜单、快捷键、仓库切换器。**本阶段仅模拟仓库切换，正式入口留 Phase 1/2。**
- [x] NU003 验证虚拟列表：至少 10,000 条模拟提交可滚动、选择、定位，记录性能。
- [x] NU004 验证原生 Graph 画布：节点、边、分支标签、滚动与命中测试。
- [x] NU005 验证原生 Diff 视图：长行、中文、可选择文本、单栏差异、二进制提示。**仅静态文本 POC，不包含正式 Git Diff 加载。**
- [x] NU006 后台 Git/SSH 任务取消、UI 线程安全刷新、错误/断线/过期状态。**Git 版本探测验证实际 UI 调度；SSH 取消/失败由 executor 集成验证，断线界面使用模拟状态。**
- [ ] NU007 原生 Workspace 打开/创建/保存对话框，保留 JSONC 注释与未知字段。
- [ ] NU008 macOS Intel 与 Apple Silicon 冒烟测试；确认可打包、可启动、无 WebView。**部分完成，同 MY008；arm64 真机待验证。**
- [x] NU009 更新文档和测试证据；未验证的 MyGo API 必须标注待确认。
