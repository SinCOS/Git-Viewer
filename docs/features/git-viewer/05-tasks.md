# 05 — Agent 可执行任务清单

状态：用户已授权全部剩余阶段，本轮关闭原 34 个未完成条目中的 31 项；当前共 54 项已勾选、3 项未勾选（T704、MY008、NU008）。实现、测试及未验证边界见 [全量执行记录](remaining-execution.md) 和 [并行编排](remaining-orchestration.md)。先前回复合计 40 为算术错误，实际剩余条目为 34。Phase 0/1 历史证据分别见 [Phase 0](phase-0-execution.md)、[Phase 1](phase-1-execution.md)。

## 阶段 0：工程与技术验证

执行顺序：T001 → T002 → T003 → T004 → T005。原始核对保留于 [Phase 0 核对记录](phase-0-review.md)；当前以执行记录为准。

补充任务归属：MY001–MY006、NU001、NU003、NU004、NU006、NU009 属 Phase 0；NU002 在 Phase 0 仅做模拟仓库三栏/菜单/快捷键 POC，正式切换器留 Phase 1/2。NU005 在 Phase 0 仅做静态文本 POC，正式 Diff 留 Phase 1。MY007 分拆为 Phase 0 的监听/原子 JSON 技术 POC 与 W 阶段 JSONC；NU007 完整归 W。MY008/NU008 记录本机启动与双架构构建，另一架构真机启动及完整发布验收留待对应环境。
- [x] T001 初始化 MyGo 原生 UI + Go；设置 lint/test/build CI。验收：macOS 本地运行、构建成功。
- [x] T002 定义 GitOperation、RepositoryContext、Error、DTO；建立 schema 契约测试。验收：Go JSON DTO 与 JSON Schema 一致。
- [x] T003 本地 Git 子进程 POC；捕获 stdout/stderr/exit、超时与取消。验收：命令错误可区分。
- [x] T004 OpenSSH POC；支持 SSH Config、host key 校验、安全远端命令编码。验收：恶意/特殊路径不触发注入。
- [x] T005 Git status -z / history / worktree parser POC。验收：特殊文件名、空仓库和合并历史测试通过。

用户确认后的 A/B 补充验证已完成并集成，仅收尾 Phase 0：
- A（NU003/NU004/NU005）：独立原生窗口探针、四场景截图、view CPU/主线程调度/Go heap 指标；原生探针及 race 执行通过。程序驱动状态与真实物理输入分开记录。
- B（MY004/MY006/NU006）：真实 SSH 认证后确认远端任务运行，再触发取消、超时和已建立连接断开；三轮九场景、资源清理检查通过。
- 主 Agent：检查文件白名单无交集，合并两个独立 Worktree 提交；make check、报告 schema 契约、依赖校验、原生 smoke、双架构打包/签名验证通过。
- C（MY008/NU008）：缺 Apple Silicon 真机，继续未完成；GPU 计数器、物理输入/IME 和真实网络兼容性未验证。

共享冻结契约、分支提交及原始证据见 [补充验证集成记录](evidence/parallel/integration.md)。当前没有独立子 Agent 调用接口，A/B 由主 Agent 在独立 Worktree 执行，未宣称实际启动独立 AI Agent。

## 阶段 1：本地只读
- [x] T101 本地仓库选择、验证、最近记录。原生目录选择/路径输入、子目录与软链接归一、worktree、最多 20 项原子记录。
- [x] T102 Git Status 解析与 UI，区分 staged/unstaged/untracked/conflicted。真实状态、虚拟列表、双区修改、刷新/取消/stale。
- [x] T103 工作区、暂存区、提交 Diff API 与查看器；二进制/大文件降级。单栏虚拟行、复制、完整提交 OID、初始提交、512 KiB / 10,000 行限额。
- [x] T104 本地只读端到端测试。真实 Git fixtures + 原生 ui.Tester 交互、race、原生窗口 Status 冒烟；实机物理输入及 arm64 启动未宣称验证。

## 阶段 2：SSH 远程
- [x] T201 SSH Host 配置、连接测试、远端 Git 探测。
- [x] T202 SshExecutor、命令白名单、安全参数编码、取消/超时。
- [x] T203 远程仓库注册与路径校验，复用 Status/Diff UI。
- [x] T204 SSH 断线、认证、host key、权限、特殊路径测试。

## 阶段 3：History/Branch/Worktree
- [x] T301 历史分页与提交详情。
- [x] T302 本地/远程分支与 upstream 解析。
- [x] T303 Worktree 列表、锁定、detached 状态及 common-dir 关系。
- [x] T304 分支/Worktree 场景测试。

## 阶段 4：Graph
- [x] T401 构建 commit DAG 和 lane 布局算法。
- [x] T402 图形渲染、虚拟列表、分页加载。
- [x] T403 merge、octopus、孤立分支、大仓库性能测试。

## 阶段 5：安全写入
- [x] T501 同仓库写操作队列、前置检查、失败后状态复核。
- [x] T502 按文件 Stage/Unstage；精确 pathspec。
- [x] T503 Commit 表单、消息安全传递、hook 失败处理。
- [x] T504 安全分支切换；拒绝覆盖未提交改动及已占用分支。
- [x] T505 本地/SSH 写操作集成测试。

## 阶段 6：远程同步
- [x] T601 Fetch 及进度/认证/超时处理。
- [x] T602 Pull --ff-only 与冲突/非快进反馈。
- [x] T603 普通 Push、上游检查、拒绝推送反馈；禁止强制推送。
- [x] T604 远端状态未知与失败后复核测试。

## 阶段 7：刷新与发布
- [x] T701 本地文件监听、去抖、Git 元数据监听、兜底刷新。
- [x] T702 远程轮询、前后台策略、退避、离线 stale 标记。
- [x] T703 大仓库、网络波动、资源占用、回归测试。
- [ ] T704 macOS 构建、安装、签名/公证决策与发布说明。**部分完成：双架构开发预览包、ad hoc 签名校验、Intel 安装副本启动及发布说明已准备；arm64 真机、正式分发与发布验收待完成。**

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
- [x] TW001 JSONC 解析与最小文本编辑 POC；验证注释、尾逗号、未知字段 round-trip。验收：不会丢失 settings/extensions 或注释。
- [x] TW002 定义 WorkspaceDocument/WorkspaceFolder/WorkspaceView DTO 和 JSON Schema；Go/JSON Schema 契约测试。
- [x] TW003 实现 Workspace 打开、路径解析、目录存在性检查、Git 仓库发现与去重。验收：多根目录、相对路径、非 Git 目录、软链接。
- [x] TW004 实现 Workspace 创建及保存；revision 冲突检测、原子写入、失败恢复。验收：外部编辑不被静默覆盖。
- [x] TW005 实现 Workspace 选择/管理 UI、最近 Workspace；与 Local/SSH 仓库切换集成。
- [x] TW006 端到端测试：真实 `.code-workspace` 文件、特殊路径、JSONC、外部修改冲突、权限错误及 VS Code 兼容性。

依赖：T001–T005、T101–T104、T201–T204。TW001 的 POC 可提前并行验证，但正式功能在阶段 2 后验收。

## v1.2 MyGo 技术任务（覆盖旧框架任务）
- [x] MY001 检查 MyGo 版本、文档与环境要求，固定 Go/MyGo 版本；记录兼容性矩阵。
- [x] MY002 初始化 MyGo `ui` 原生 Go 项目，macOS 打开主窗口。
- [x] MY003 原生 Go 视图调用 `Health/Version` 服务，验证错误反馈与状态更新。
- [x] MY004 验证 goroutine 到原生 UI 的安全状态调度、取消、加载态与窗口关闭清理。
- [x] MY005 用 `exec.CommandContext` 实现本地 Git 只读 POC，含超时、特殊文件名和输出限额。
- [x] MY006 通过系统 OpenSSH 验证远程 Git 只读 POC，测试路径转义、断线、host key 校验。
- [x] MY007 验证 MyGo 文件监听、本地 JSON 原子保存、`.code-workspace` JSONC 读写。**监听/原子 JSON 历史验证及本轮正式 JSONC/Workspace 保存测试通过。**
- [ ] MY008 完成 macOS Intel/Apple Silicon 构建和打包冒烟测试，并更新系统预分析。**部分完成：双架构构建/打包/签名校验、Intel 启动通过；arm64 真机启动待验证。**


## v1.3 原生 UI 专项任务（替代 WebView/前端任务）
- [x] NU001 使用官方 native 模板建立最小窗口，确认无 WebView/HTML/JS/TS 依赖；锁定版本。
- [x] NU002 实现可调整三栏原生布局、工具栏、菜单、快捷键、仓库切换器。**本阶段仅模拟仓库切换，正式入口留 Phase 1/2。**
- [x] NU003 验证虚拟列表：至少 10,000 条模拟提交可滚动、选择、定位，记录性能。
- [x] NU004 验证原生 Graph 画布：节点、边、分支标签、滚动与命中测试。
- [x] NU005 验证原生 Diff 视图：长行、中文、可选择文本、单栏差异、二进制提示。**仅静态文本 POC，不包含正式 Git Diff 加载。**
- [x] NU006 后台 Git/SSH 任务取消、UI 线程安全刷新、错误/断线/过期状态。**Git 版本探测验证实际 UI 调度；SSH 取消/失败由 executor 集成验证，断线界面使用模拟状态。**
- [x] NU007 原生 Workspace 打开/创建/保存对话框，保留 JSONC 注释与未知字段。**原生回调及真实服务 ui.Tester 路径通过；用户反馈后补齐异步面板与关闭修复，真实 AppKit 面板探针通过，见 [修复记录](dialog-fix-execution.md)。物理输入未单独验收。**
- [ ] NU008 macOS Intel 与 Apple Silicon 冒烟测试；确认可打包、可启动、无 WebView。**部分完成，同 MY008；arm64 真机待验证。**
- [x] NU009 更新文档和测试证据；未验证的 MyGo API 必须标注待确认。

## Commit / Diff 截图对齐任务（用户指定参考图）

参考图：`/Users/richard/Desktop/Git Viewer Commit Diff Interface.png`。详细分解、范围与验收见 [截图对齐任务](commit-diff-reference-tasks.md)，实施进度见 [首轮记录](commit-diff-parallel-execution.md)、[补齐记录](commit-diff-completion-execution.md)、[功能入口记录](commit-diff-features-execution.md)及[回归记录](commit-diff-validation-execution.md)；本组尚未完成。

- [ ] CD001 对齐全局框架：已接入图文导航、原生同一行标题栏、Git 标识、上下文选择、连体 Fetch/Pull/Push、刷新、终端和设置按钮以及真实状态计数；截图像素对照、窗口拖动、布局与窄窗待验证。
- [ ] CD002 对齐左侧 Changes / Staged Changes：默认目录树将未跟踪文件并入 Changes，暂存区平铺；对齐目录计数、状态徽标与行尾菜单。截图对照及相关程序驱动检查通过，物理交互待验证。
- [ ] CD003 对齐提交表单：已接入 Amend 勾选及其提交/推送流程、表单重置和受控 Discard All；写操作、视觉和交互待验证。
- [ ] CD004 对齐右侧文件标题、Diff 工具栏、统一/双栏预览及空状态：单文件预览隐藏重复 Git 头、分列行号与标记，调整文件状态徽标与工具栏；截图及相关回归通过，深色主题和真实交互待验证。见 [对齐记录](commit-diff-visual-alignment.md)。
- [ ] CD005 对齐右下文件列表面板：已接入真实状态、文件筛选、多选及确认后批量操作；待验证。
- [ ] CD006 截图入口功能：已接入文件历史、HEAD Blame、默认编辑器、Terminal、操作摘要 Console、文件 Search、Diff 显示设置及受控批量操作；远程外部应用、真实运行和安全边界仍待验证。
- [ ] CD007 响应式、浅/深主题和交互验收：此前全量 `make check`（含 race）与原生 smoke 已通过；本轮原生截图检查包含 1000×550，顶部和提交表单需滚动；深色主题及真实窗口物理交互仍待检查。见 [对齐记录](commit-diff-visual-alignment.md)。

## v1.5 布局与 Commit 工作区任务（D 组，方案已确认）
- [x] D0 更新 01–05 规划文档（v1.5 章节）。
- [x] D1 全局布局：左侧 56px icon 导航栏替换顶栏分段按钮，顶栏 46px；本地 UI 测试和构建通过，真实窗口物理交互待验证。
- [ ] D2 Git 写层：Discard 与 Amend 已接入确认流程及受控 Git 写入，隔离 fixtures 验证通过；Discard 拒绝未跟踪、冲突、重命名、类型变化和子模块等复杂状态。Worktree 写入已扩展至 SSH，新增实现及安全边界待测试；见 [本轮记录](ssh-worktree-execution.md)。
- [x] D3 Commit 页三栏：Changes 与表单在可调整宽度的左列，Diff 位于右列；本地 UI 测试与构建通过。分组交互属于 D4。
- [ ] D4 Changes/Staged 交互：表单显示暂存和消息提示，支持确认后批量 Stage/Unstage；文件行显示图标和路径，右键支持暂存、取消暂存、受控 Discard 及复制路径。提交、Amend、暂存、取消暂存与 Discard 失败在表单内显示，刷新不会清除，下一次确认执行或切换仓库时清除；隔离仓库 hook 失败及重试测试、全量 `make check` 通过。真实窗口交互验收待完成。
- [x] D5 Diff：Git Log 选择提交自动打开详情，详情文件点击加载单文件 Diff；Unified/Side-by-Side 基于真实 diff 内容渲染，保留查找和复制。子任务及集成本地 UI 测试、构建通过；combined diff 保留原始行，物理交互和视觉待验收。Hunk 级操作范围外。
- [x] D6 Git Log 微调：分支树默认 240px，已有用户宽度偏好继续保留；本地 UI 测试和构建通过。
- [ ] D7 Worktree 页：本地仓库已接入创建（已有未检出本地分支）、打开、确认删除和状态查看；后端隔离测试及 UI 测试、构建通过。SSH 创建、删除及状态查看已接入，待隔离远程环境测试及真实窗口验收；见 [本轮记录](ssh-worktree-execution.md)。
- [ ] D8 视觉统一：针对窄窗口限制顶栏长名称并保留完整提示，多仓库栏、提交批量操作和 Diff 操作改为横向滚动；部分间距与文件操作图标尺寸统一。后续 `make build` 通过；测试、静态检查、1000×550、浅/深主题、Hover/Selected/Disabled 和真实窗口物理交互仍待验收。见 [D8 执行记录](visual-polish-execution.md)。

## v1.4 UI 布局重构任务（UL 组，用户已授权）
- [x] UL026 按 `Git Viewer Commit Graph and Diff.png` 对齐 Git Log：提交表改为 Graph / Description / Branches / Author / Date / Commit 六列，分支标签独立显示；底部改为左侧提交摘要与文件列表、右侧 Diff 工具栏和虚拟差异视图。自动打开首个变更文件，修复切换范围后的旧预览残留。窄宽列序回归、全量 `make check`（含 race）和原生构建通过；真实鼠标键盘及深色主题待人工验收。见 [执行记录](git-log-alignment-execution.md)及[视觉快照](evidence/git-log-aligned.png)。
- [x] UL025 修复嵌套分支右键菜单使用末级名称的问题：切换和复制使用完整分支名；相关分支／历史 UI 测试及构建通过。

- [x] UL024 参考 JetBrains/Fork 重构桌面布局：移除左侧模式导航栏，Commit/Git Log/Worktree 改为统一顶栏首组入口（顺序与 Cmd+1/2/3 不变）；Workspace/Repository/当前 Branch 弹层与 Fetch/Pull/Push 保留在顶栏；Git Log 页分支树可拖宽、可隐藏可恢复（宽度保留）并新增 Local/Remote/Tags/Stashes 四分区（折叠/搜索/右键菜单，分支右键复用既有确认切换流程，Tag/Stash 仅复制），选中 Commit 后详情改为历史表格下方可拖高/可隐藏的底部面板（元数据＋文件变更＋Diff）；面板标题 12 Bold、说明 10–11 muted、行高 30、图标 16×16，选中态统一 SurfacePressed；BranchTreeHidden/HistoryDetailsOpen/HistoryDetailsHeight 进 ui-prefs.json。新增只读 for-each-ref refs/tags/ 与 stash list 查询，Git 行为/数据模型/操作流程未改。完整 make check（含全量 race）、原生 smoke 通过；真实物理鼠标/键盘交互与真机观感未宣称实测。见 [设计与执行记录](idea-fork-layout-refactor.md)。

- [x] UL023 修复点击提交/自动刷新导致无关面板变灰与整窗位移：读取不再禁用分支树与工具图标、自动刷新静默化、状态栏高度固定、读取可被新读取替换；新增像素级回归测试，完整 `make check` 通过；真机观感待用户确认。见 [执行记录](history-flash-fix-execution.md)。

- [x] UL022 继续修正分支树与详情/刷新区域：保留原生历史列表实例、列宽、刷新时的滚动锚点/提交选择与已加载页数；详情开关保持列表父层级不变，保留拖动与键盘分隔线；分支/搜索切换不重建列表。120 提交多页交互回归、完整 `make check` 及补充后的相关 race/vet/构建通过；真机无闪烁仍待人工观察。见 [执行记录](history-flash-fix-execution.md)。

- [x] UL021 修复 Git Log 分支切换与详情/Diff 加载前清空造成的闪白：读取期间保留原列表/范围/分页/预览，成功后一次性替换，失败或取消保留画面。新增等待/失败/取消/迟到结果回归测试，UI 与入口全量 race、相关 Go vet/MyGo vet、原生构建通过；人工视觉验收未完成。见 [执行记录](history-flash-fix-execution.md)。

- [x] UL020 按截图移除最上方 Git Viewer/Ready/Refresh/Cancel/Choose repository 整条栏；状态与加载提示移至底部，选择仓库移入 Repository 菜单。UI 与正式入口全量 race 测试、原生构建通过，未重复仓库全量测试或人工 GUI 验收。见 [执行记录](topbar-removal-execution.md)。

- [x] UL019 Git Log 左侧增加原生分支导航树：分支搜索、Local/Remote/remote/路径分组、折叠、当前分支星标、跟踪状态、All Branches/HEAD、可调宽度与持久化；按所选分支 OID 加载固定分页，删除分支刷新回退全部历史。完整 `make check`（含全量 race）、原生隔离仓库 Log 模式启动冒烟通过；真实物理交互与远程主机实测未完成。见 [执行记录](branch-navigation-execution.md)。后续按截图及位置澄清，All Branches 使用顶部图标，HEAD 改为 Local 上方的独立整行入口（金色描边标签图标＋文字），保留提示和高亮；分支搜索框限制最大 224 px，Git Log 与 Commit / Local Changes 选中行均改为灰底与正常文字色；相关 UI race 测试与构建通过。

- [x] UL018 按参考截图将 Git Log 改为紧凑五列表格、内联彩色分支标签、多色连续图形、分支/远程筛选，默认全宽且详情可切换。相关 History/Graph/布局 race 检查、静态检查、构建与原生 smoke 通过，未跑全量测试。见 [执行记录](layout-workspaces-execution.md)。

- [x] UL017 Diff Preview 参考 VS Code 调整亮/暗主题：柔和增删背景、正常正文色、弱化 hunk/元信息；5 项相关检查与构建通过，未跑全量测试。见 [执行记录](layout-workspaces-execution.md)。

- [x] UL016 文件行暂存/取消暂存改为 SVG 图标，Tree 目录增加随折叠变化的文件夹图标；6 项相关 UI 检查与构建通过，未跑全量测试。见 [执行记录](layout-workspaces-execution.md)。后续按反馈将行内操作改为无边框“文件＋/文件−”图标，3 项相关检查与重建通过。

- [x] UL015 Local Changes 整块/状态组/目录支持独立点击折叠；Fetch / Pull / Push 改为 SVG 图标并保留提示、禁用与确认逻辑。8 项相关 UI race 检查、静态检查与构建通过，未跑全量测试；见 [执行记录](layout-workspaces-execution.md)。

- [x] UL014 修复图标按钮内边距导致的偏右和裁切：清除默认 Padding/Gap、双向居中，相关布局/视图切换测试与构建通过；未跑全量测试。见 [执行记录](layout-workspaces-execution.md)。

- [x] UL013 截图反馈：Local Changes 的 List/Tree/刷新改为 SVG 图标；修复 Git Log 固定宽度，列表、选中背景与提交行随面板伸缩，revision/date 固定右侧。相关 UI/Graph 测试、宽度往返测试、静态检查及构建通过；未跑全量测试。见 [执行记录](layout-workspaces-execution.md)。

- [x] UL012 实施预览中的原生 Commit / Git Log 样式：分组文件列表、行内暂存/取消暂存、移除提交 ID 控件、自动加载历史与分支、提交搜索及更清晰的历史/详情层级。按用户要求仅运行相关 UI race 测试、静态检查、构建与原生 smoke，均通过，未跑全量测试。边界见 [执行记录](layout-workspaces-execution.md)。

- [x] UL011 按用户要求先提供独立 HTML 样式预览：VS Code 风格 Local Changes、去除 Commit 页提交 ID 控件、Git Log 三栏与自动加载演示；HTML 结构与脚本语法检查通过。预览文件见 [样式稿](evidence/ui-workspaces-preview.html)。本项仅完成预览，原生样式与自动加载尚未实施。

- [x] UL010 截图反馈修正：左侧导航撑满主体高度并加宽，Fetch / Pull / Push 恢复顶栏常驻。按用户要求仅运行 4 项相关 UI 测试与原生构建，均通过；见 [执行记录](layout-workspaces-execution.md)。

- [x] UL001 Shell 迁移：shell.go（View 主干、UIState、Panel 契约、左竖栏三模式、顶栏下拉）、prefs.go（布局/窗口偏好原子 JSON）；app.go/features.go 按模块拆分到独占文件。验收：make check、mock 布局测试、模式切换。
- [x] UL002 repo_explorer.go：仓库发现/最近/Workspace 控制/Worktree+Branch 摘要。依赖 UL001。
- [x] UL003 changes.go：文件列表行加 Stage/Unstage Checkbox、键盘选择。依赖 UL001。
- [x] UL004 history.go：分页列表+提交详情、GitLog 模式合并视图。依赖 UL001。
- [x] UL005 graph.go：DAG 绘制独立化+命中测试，1 万行虚拟化保持。依赖 UL004。
- [x] UL006 diff.go：FindBar、Copy diff、提交详情头。依赖 UL001。
- [x] UL007 operations.go：Commit 区迁入 Commit 模式底部、Commit and Push 组合动作、SSH 控件。依赖 UL001。
- [x] UL008 集成：白名单核对、make check+race、布局冒烟截图、evidence 更新。依赖 UL002–UL007。
- [x] UL009 按三张 ASCII 草图重排 Commit / Git Log / Worktrees：最左导航、顶部上下文菜单、底部提交表单、Log 三栏与 Worktree 列表/详情；修复 Commit and Push 的两次确认与成功后衔接。验收：两种宽度面板位置、模式隔离、Worktree 详情选择、隔离远端四路径、make check、原生 smoke 通过。边界见 [三模式执行记录](layout-workspaces-execution.md)：新增/删除 Worktree、其他 worktree 修改计数和 ahead/behind 数字尚未实现，真实外观与物理输入待验收。

- [x] 2026-10-27 Workspace 加载后的 Commit/Changes 对齐：统一 Changes 与 Diff 标题高度，仓库切换栏固定高度并居中；多仓库窄窗口布局回归与原生构建通过。见 workspace-commit-alignment-execution.md。
- [x] 2026-10-27 按截图将独立 workspace 仓库切换栏并入 Repository 下拉，保留计数、选中标记、切换与刷新；相关回归及构建通过。
- [x] 2026-10-27 修复 Repository 等工具栏下拉无法点击的问题；为 Changes 增加基于 Git index 的文件复选框，支持选择文件后仅提交暂存区中的所选文件。
- [x] 2026-10-28 按截图反馈修正三项：Git Log 图形在提交 lane 变化的行上半段补画 lane 连接线，消除选中行等处竖线中断错位；移除 Git Log 头部的 Toggle branch tree 按钮，分支树固定显示；移除窗口底部状态栏。go vet、全量 race 测试与原生 smoke 通过。
- [x] 2026-10-28 UI 改为 VS Code 观感与布局：新增 VS Code 亮/暗主题、36px 标题栏、48px 图标活动栏、统一可拖宽主侧栏（Commit 表单/分支树/Worktree 列表）与右侧编辑区（Diff/Git Log/Worktree 详情），Git Graph 配色对齐 VS Code。go vet、全量 race 测试、原生 smoke 通过。
- [x] 2026-10-28 编辑区新增 VS Code 风格代码编辑器模式（⌘4）：多标签页、脏标记与关闭、打开文件/变更文件侧栏、⌘S 保存、语法高亮与撤销/多光标/补全（复用 mygo editor）、Ln/Col 状态行；编辑器模式随布局偏好恢复。go vet、全量 race 测试、原生 smoke 通过。
- [x] 2026-10-28 按反馈重做 EXPLORER 与 Source Control：Explorer 增加 VS Code 风格根目录、文件夹/文件图标、紧凑工具栏、目录优先排序、根目录折叠与长名称省略；Source Control 移除重复标题，提交框/Amend/提交按钮改为 VS Code 紧凑布局，变更组与文件行改用标题式分组、右侧状态字母和窄栏文件名省略。go build、go vet 与真实窗口截图检查通过；未跑测试。
- [x] 2026-10-28 修复 EXPLORER 同级文件夹未对齐：行按钮默认水平居中导致短名称右移，统一改为左对齐；go build、go vet 与真实窗口截图检查通过，未跑测试。
- [x] 2026-10-28 Diff Preview 对齐 VS Code：Commit/Git Log 共用单层文件标题栏，移除重复工具行；Side-by-Side 增加 Original/Modified 双栏标题、独立行号 gutter、分隔线与 VS Code 增删背景，统一/双栏切换、设置、查找、复制与文件历史功能保留。Workspace 入口接入标题栏，Explorer 支持 `.code-workspace` 多根目录与跨根 Quick Open，标题栏显示当前 workspace。相关 Diff/Workspace 定向测试、go vet、go build 与真实窗口双根工作区验证通过；未跑全量测试。
- [x] 2026-10-28 按截图继续收紧 Diff 页面：移除底部重复的 Files/批量操作面板，Diff 编辑器占满右侧高度；增删行背景铺满编辑器宽度；Unified/Side-by-Side 改为 VS Code 风格图标切换并保留提示与键盘可访问标签。相关 Diff/Workspace 定向测试、go vet、go build 与真实窗口 unified/side-by-side 检查通过；未跑全量测试。
- [x] 2026-10-28 按反馈收紧仓库上下文入口：编辑器保存文件后立即请求 Source Control 后台刷新，并与现有 watcher/调度器合并；标题栏移除 Workspace 与 Repository 下拉，Workspace 操作保留在原生菜单栏，原 Repository 菜单改为 VS Code 风格 File 菜单。相关保存/Workspace/菜单定向测试、go vet、go build 与真实窗口 clean→dirty 检查通过；未跑全量测试。
- [x] 2026-10-28 移除 Worktree 功能入口与页面，活动栏只保留 Editor / Git Log / Commit，旧 `worktree` 布局偏好回退到 Commit；Source Control 增加切回 Commit 页和窗口重新获得焦点时的兜底刷新，覆盖外部编辑器保存后 watcher 未及时通知的情况。相关模式、偏好、保存、焦点与外部编辑定向测试、go vet、go build 及真实窗口检查通过；未跑全量测试。
- [x] 2026-10-28 Source Control 常规操作对齐 VS Code：Stage/Unstage/Commit/Amend/Fetch/Pull/Push 直接执行，不再弹出系统确认框；Discard 与分支切换仍保留确认。相关写入、批量选择、Commit & Push 与原生对话框定向测试通过；未跑全量测试。
- [x] 2026-10-28 合并重复的仓库打开入口：File 菜单只保留 `Open folder…`（⌘O），选择目录后同时建立 Explorer 根目录并自动探测 Git 仓库；移除独立 `Open local repository…`。本地 watcher 的 5 秒兜底改为始终刷新状态，避免遗漏工作区文件事件时长期不更新 Source Control。
