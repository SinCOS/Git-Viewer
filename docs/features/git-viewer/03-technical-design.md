# 03 — 技术设计

## 全量实现映射（用户已授权后续阶段）

当前正式入口仍使用 MyGo 原生 `ui.View`。`LocalService.ReaderFor` 根据已验证 RepositoryContext 绑定本地或 SSH Reader；UI 只调用受控 Open/Status/Diff/History/CommitDetails/Branches/Worktrees/Write 方法。`GitOperation` JSON 字段保持原样，内部 cursor/输入流/Transport 不序列化。

- WorkspaceDocument/WorkspaceFolder/WorkspaceView 对齐既有 JSON Schema，SavedLocation 新增独立 schema。原始 JSONC 与 revision 位于内部 workspace.Document；持久化只保存最近别名与路径，不保存凭据或 Git 状态。
- History cursor 是 HEAD/分支/tag 解析得到的完整 OID 集合加 offset，后续页不重新解析浮动 refs；GraphLayout 保留跨页父 lane，绘制与布局分离。
- 写队列按 location、SSH alias、common-dir 组合键串行。每次前置读取 Status，按类型构造 Git argv。Commit 消息使用 stdin；写入尊重正常用户 Git 身份、hooks 与 credential helper。Git 内部的上游 SSH 也固定 BatchMode/严格 host key 检查。
- Push 显式指定当前分支到其上游完整 ref，覆盖 mirror、禁止 force/follow-tags/submodule 扩大范围。取消或断线后不重试，Status 复核失败/远端写入结果不确定时返回 OperationStateUnknown。
- 本地工作区与 GitDir/CommonDir 使用 MyGo HintsOnly watch；500ms 去抖、60s 兜底。远程前台 15s/后台 60s、退避上限 5min。后台状态经 Window.Update 应用，自动刷新保留所选 Diff。

细节、测试及 Workspace 乐观保存的并发边界见 [全量执行记录](remaining-execution.md)。Phase 0/1 的下列映射保留为历史记录，不能据此认为正式入口仍只读。

## 1. 技术选型
- 【建议】MyGo + Go 原生 UI（`ui` 包，无 WebView）；首发 macOS；后续跨平台需隔离 OS 特性。
- Git CLI 是唯一 Git 状态真相；初期不引入 libgit2，不安装远端 Agent。
- OpenSSH 读取用户 ~/.ssh/config，利用 ssh-agent；不要自己解析密钥文件或保存密码。

## 2. 分层与职责
1. UI：仓库选择器、Status、Diff、History、Branch、Worktree、Graph、操作确认与任务进度。
2. 原生 UI 控制器：Go 类型调用、输入校验、context 取消、统一错误类型；无需 Web IPC。
3. Application：RepositoryService、StatusService、DiffService、HistoryService、BranchService、WorktreeService、SyncService、RefreshCoordinator。
4. Domain：RepositoryContext、GitOperation、GitError、CommitGraph、ChangeState。
5. Infrastructure：LocalExecutor、SshExecutor、GitOutputParsers、RepositoryStore、LocalWatcher、RemotePoller。

## 3. 统一执行接口（概念）

以下为概念示例；实际冻结接口见 `internal/domain/git.go`：`GitCommand` 持有 `Repository RepositoryContext`、`Operation GitOperation`、`Timeout time.Duration`，RepositoryID 在 Repository/Operation 中显式携带并由执行器校验。Phase 0 补充验证保持该实际接口不变。
```go
type GitCommand struct {
    RepositoryID string
    Operation GitOperation
    Timeout time.Duration
}
type GitResult struct {
    ExitCode int
    Stdout, Stderr []byte
}
type GitExecutor interface {
    Execute(context.Context, GitCommand) (GitResult, error)
}
```
- GitOperation 是封闭枚举 + 类型化参数，UI 不能直接提交任意 shell 字符串。
- Go 在本地使用 `exec.CommandContext` 和分离参数；SSH 端应有独立安全参数编码器，避免字符串拼接和命令注入。
- 【建议】可选进阶方案：远端只调用受控 shell wrapper；本期先完成编码器安全验证。

## 4. 关键 Git 协议
- Status：`git status --porcelain=v1 -z`；区分 index/worktree 状态，处理重命名双路径。
- Diff：`git diff --no-ext-diff --binary` 的展示应过滤/限流；staged 用 `git diff --cached`；初始提交无 HEAD 需单独处理。
- History：`git log` 使用机器可解析字段分隔符，避免按人类可读文本分割；分页并稳定排序。
- Graph：每条提交解析 oid、parents、refs；前端虚拟列表 + DAG lane 分配；不能仅凭时间顺序连线。
- Branch：优先 `git for-each-ref` 与格式化字段；Worktree：`git worktree list --porcelain -z` 需先做目标 Git 版本兼容验证，不支持时使用非 -z porcelain 并严格处理边界。
- Stage：显式 pathspec 与 `--`，确保不会误处理选项；untracked 与删除路径分别验证。
- Commit：提交消息通过 stdin 或临时受保护文件传入，避免命令行泄露/转义问题；尊重 Git hooks。
- Pull：【建议】`git pull --ff-only`；不提供隐式 merge/rebase。
- Push：默认普通 push，禁止 `--force`、`--force-with-lease`，无上游需明确提示。

## 5. 刷新与一致性
- 本地：watch 工作区及 Git 元数据（兼容 gitfile/common-dir），去抖 300–800ms；事件丢失后全量刷新；操作后强制刷新。
- 远程：前台 15 秒、后台 60 秒轮询，指数退避、同仓库去重；用户手动刷新优先；远端读取可按需分层加载。
- 版本戳：每个结果附 fetchedAt 与 connectionState；离线结果显式标记 stale。
- 写操作：同仓库写队列 + Git 自身锁机制；不能仅依赖 UI 按钮禁用。

## 6. 安全设计
- 禁止执行任意 UI 输入的 Git 命令；固定 Git 操作集。
- SSH StrictHostKeyChecking 不得自动关闭；密钥和凭据不写入应用数据库。
- 路径经仓库根目录验证；操作必须显式指定 repositoryId；文件操作防路径穿越、符号链接误用。
- 不自动执行破坏性 Git 命令；所有远程同步失败保留原始错误（脱敏后展示）。
- SSH 超时/取消后如远端操作状态不确定，必须重新探测，不能重试可能重复的写操作。

## 7. 数据存储与契约
- 【建议】MVP 配置使用本地 JSON 文件（原子写入），无需 SQLite；如后期需要搜索/复杂缓存再引入 SQLite migration。
- 独立 schema 见 `schemas/`：`repository.schema.json`、`git-status.schema.json`、`git-operation.schema.json`、`storage-schema.md`。
- Schema 是 Git/Workspace 数据交换与持久化契约；Go 结构体与 JSON Schema 通过契约测试保持一致，无 TypeScript 客户端。
- Phase 0 明确契约边界：Repository DTO 使用现有 schema 字段；RepositoryContext 为内部类型，包含 SSH Host 与 git-dir/common-dir，不隐式改变持久化结构。GitOperation DTO 保留既有字段，执行器另外验证操作参数并仅开放只读 POC。GitError 为内部错误类型，本阶段不新增传输 schema。

## 8. 错误类型
`InvalidRepository`, `GitUnavailable`, `SshAuthFailed`, `HostKeyRejected`, `RemoteUnavailable`, `PermissionDenied`, `GitConflict`, `UnsafeOperation`, `OperationCancelled`, `OperationStateUnknown`, `UnsupportedGitVersion`, `ParseError`。

## 9. 待确认
- Git Graph 采用 §12 的原生 `Draw` / `ui.Painter` / `ui.Path`，与原生虚拟列表组合；旧 Canvas/SVG 待选条款由 §12 覆盖。具体性能通过 Phase 0 POC 验证。
- 【待确认】是否支持交互式 SSH 密码、Git credential helper 弹窗；默认优先非交互密钥认证。
- 【待确认】最低 Git 版本、最低 macOS 版本；在 POC 阶段验证。


## Phase 1 实现映射

用户已授权本地只读 T101–T104，主入口 `cmd/git-viewer` 现使用 `internal/ui/local`；Phase 0 的模拟窗口组件和 `cmd/phase0-ui-probe` 保留作技术回归。

- `application.LocalService` 编排仓库探测、最近记录与读取；`git.LocalReader` 使用固定系统 Git argv、10 秒超时、取消和输出上限。原有 LocalExecutor/SSH POC 保持独立，正式远程 UI 留 Phase 2。
- 仓库探测通过 `rev-parse` 分别读取根目录、git-dir/common-dir，规范化软链接；工作目录与 git-dir 共同生成稳定本地 ID。支持子目录、gitfile/worktree；bare 仓库拒绝。最近记录是 Repository DTO 数组，按打开顺序去重，最多 20 项，原子替换；损坏记录不自动覆盖，保存失败不阻止查看仓库。
- Status 沿用 porcelain v1 NUL 协议，`domain.StatusEntries` 将 index/worktree 独立投影到 Staged/Unstaged，另列 Untracked/Conflicted。同一文件可出现两次；冲突只进入冲突分区。
- `DiffRequest` 是内部类型化请求；`DiffPreview` 为 JSON 展示契约，见 `diff-preview.schema.json` 与契约测试。工作区/暂存区只接受当前 Status 中的文件，重命名补充旧路径；全部使用 `--literal-pathspecs`、`--`、禁用外部 diff/textconv。提交只接受完整 SHA-1/SHA-256 OID，并以 `^{commit}` 限定对象类型；根提交展示新增，merge 展示第一父提交差异。不提前实现历史分页 UI。
- 未跟踪文件以 `os.Root` 约束读取，symlink 展示链接目标文字，二进制提示；未跟踪文件超过 512 KiB、Git diff 超过 512 KiB 或展示超过 10,000 行均降级，不显示截断补丁。统一 Diff 文本按行虚拟化，保留横向滚动、单行文本选择与复制完整预览。双栏行内高亮仍为独立里程碑。
- 控制器将后台结果经 `Window.Update` 应用，代次检查丢弃过期/取消/关闭结果。错误和取消保留已有 Status 并标记 stale；手动刷新成功清除。自动监听/轮询留 Phase 7。目录选择为原生 MyGo Dialog；`Cmd+O` 打开、`Cmd+R` 刷新。

验收证据及运行步骤见 [Phase 1 执行记录](phase-1-execution.md)。

## 10. Workspace 设计
【原文明确】读取、创建、保存 VS Code `.code-workspace`。

【建议】组件边界：`WorkspaceService`（业务与保存策略）、`VscodeWorkspaceParser`（JSONC 解析及无损编辑）、`WorkspacePathResolver`（以 Workspace 文件为基准解析 folder）、`WorkspaceRepositoryDiscovery`（识别/去重 Git 仓库）、`WorkspaceStore`（最近打开列表及 Git Viewer 私有元数据）。

API：`openWorkspace(path) -> WorkspaceView`；`createWorkspace(path, folders, optionalSettings) -> WorkspaceView`；`saveWorkspace(id, folderChanges, expectedRevision) -> WorkspaceView`；`closeWorkspace(id)`。`expectedRevision` 为打开时的文件指纹，保存前验证；文件变更则返回 `WorkspaceConflict`。

模型：`WorkspaceDocument` 表示原始 VS Code 文件（JSONC 文本与 revision）；`WorkspaceFolder` 表示 name/path；`WorkspaceView` 是派生视图，包含 resolvedPath、repoId?、diagnostics[]。Git Viewer 私有 SSH 仓库关联与 UI 偏好单独存储，不写入 VS Code 文件。

保存策略：创建文件时输出规范 JSON；编辑已有文件时使用 JSONC edit/modify 算法，仅编辑 folders 节点，尽量保留注释与未知字段；不支持无损修改时阻止保存并解释原因。保存流程校验 revision、临时文件写入、fsync（可行时）、原子 rename、重新解析。文件路径必须经过权限和目标文件类型检查，防止意外覆盖。

契约：`schemas/workspace.schema.json` 为解析后的标准结构（不用于直接验证原始 JSONC 文本）；`schemas/workspace-view.schema.json` 为 UI DTO；`schemas/storage-schema.md` 记录最近 Workspace 与私有关联。

测试：JSONC round-trip、未知字段/注释保留、尾随逗号、相对路径、空 folders、重复仓库、文件权限、外部修改、原子写入失败恢复。

## 11. MyGo 技术适配（v1.2）
- 【原文明确】使用 https://github.com/egoist/mygo 作为桌面框架，macOS 优先；保留 OpenSSH、safe、hybrid。
- 【建议】选用 MyGo `ui` 纯 Go 原生 UI；Graph、Diff、三栏布局由 Go 组件与原生绘图实现。Go 实现全部 Git、SSH、Workspace 与持久化逻辑。
- 【建议】Go 原生视图控制器调用 `RepositoryService`、`WorkspaceService`、`GitReadService`、`GitWriteService`；界面仅使用受控业务方法。严格限定可调用方法与参数，不暴露通用 shell 执行。
- 【建议】Go 包划分 `internal/domain`、`internal/application`、`internal/infrastructure/{git,ssh,workspace,storage,watch}`、`internal/desktop`；`internal/ui` 按 feature 分包。
- 【建议】本地用 `exec.CommandContext`、明确 argv、context timeout、stderr/stdout 限额；远程使用系统 `ssh`，严禁拼接不可信路径到 shell，先做参数编码与注入 POC。
- 【建议】MyGo 文件监听用于本地变化，轮询协调器用于远程变化；UI 通过 Go 状态订阅/事件调度接收变更通知，断线时标记 stale。
- 【待确认】MyGo 当前版本在目标 macOS Intel/Apple Silicon 的构建、签名、公证、原生布局、列表、绘图、文本编辑与文件监听 API 均需 POC 实测，不能以 README 代替验收。
- 【待确认】MyGo 最低 Go 版本与 CLI 用法以锁定的实际版本文档为准；依赖版本需固定，不直接使用 latest 作为可复现构建依据。


## 12. v1.3 原生 UI 架构（覆盖 v1.2 WebView 决策）
- 【原文明确】仅使用 MyGo `ui`，不得引入 WebView、HTML、JavaScript、TypeScript、Vue。
- 【建议】调用链：`ui.View` → `ViewModel/Controller` → `Application Service` → `Git Operation` → `LocalExecutor | SSHExecutor` → Git CLI → 解析器 → Go 领域模型 → 主 UI 线程安全刷新。
- 【建议】组件：`RepositorySidebar`、`WorktreeTree`、`CommitGraphCanvas`、`CommitVirtualList`、`ChangesPanel`、`DiffViewer`、`WorkspaceDialog`、`GitOperationDialog`、`ConnectionStatus`。
- 【建议】Git Graph：后台 Go 计算 DAG lane、节点及连线；原生画布只负责绘制、滚动、命中测试；按批加载，避免全量布局。
- 【建议】Diff：Go 解析统一 diff，采用虚拟化行视图、文本选择、双栏/单栏模式；高亮和长行处理独立组件。
- 【建议】后台 Git/SSH 调用采用 `context.Context` 与 goroutine；状态变更按 MyGo 当前版本规定调度到 UI 线程，避免直接跨线程改控件。
- 【建议】原生菜单、快捷键、文件打开对话框与 macOS 外观适配统一在 `internal/ui/shell`。
- 【待确认】MyGo 原生绘图/虚拟列表/文本选择的具体 API、稳定性与性能通过 Phase 0 POC 验证，失败则先缩小高级 UI 范围，不自动改回 WebView。

Phase 0 实测映射：固定 Go 1.27.3、MyGo `9c69cf829ece`；`WindowOptions.Content: ui.View`、嵌套 `ui.Split`、`ui.List/ListState`、`Draw/ui.Painter`、`Selectable`、`Window.Update`、`OnClosed/OnBeforeQuit`、`mygo.WatchFiles` 已在代码及测试中验证。Graph 使用模拟节点验证绘制，不包含正式 DAG 布局；Diff 使用静态可选中文本验证，不包含 Phase 1 的加载/解析业务。当前性能为 headless 基线，真实 GPU 与 arm64 真机表现待验证，详情见 [执行记录](phase-0-execution.md)。

## UI 分区与模块契约（v1.4 UI 重构，2026-10-25 确认）

布局基线见 [UI 简稿图](evidence/ui-layout-sketch.svg)。六大区域映射七个模块：Shell（Global Toolbar+Status Bar+挂载）、Repository、Changes、History、Graph、Diff（Inspector）、Operations。

- 挂载契约：`type Panel func(c *ui.Context, s *UIState)`；Shell 独占 `shell.go` 构建 View，面板只读本模块状态、经 App 既有方法发起动作；业务数据仍由 App 拥有。
- 主工作区三模式（左竖栏 Sidebar 切换）：Commit（Changes+Diff+Commit 区）、Git Log（Graph+History 合一）、Worktree（仅查看）。
- 已拍板：状态栏 ahead/behind（↑↓）本期不做；Worktree 仅查看，分支切换走顶栏 Branch 下拉；`Commit and Push` 为 UI 组合动作（成功后走确认流再 Push）。
- MyGo API 已核对：Sidebar/SidebarItem、MenuButton/Select/Popover、Checkbox、Split/SplitPercent、FindBar、OnShortcut、Focus/AutoFocus/KeepFocus 均存在；窗口 bounds 无框架级持久化，由应用层 prefs 原子 JSON 保存。
- 文件所有权与并行边界见 `evidence/parallel/contract-ui.md`（UL001 完成后冻结）；app.go/features.go 方法签名为共享冻结接口。

## v1.5 组件拆分与新增写操作设计（用户已确认）

文件所有权（互斥，支持多 Agent 并行）：

| 模块 | 独占文件 | 说明 |
|---|---|---|
| 框架 | `shell.go` `rail.go`(新) `toolbar.go`(新) `layout_test.go` `shell_test.go` | rail 为自定义 icon 按钮列（52–60px）；`operations.go` 拆分为 toolbar.go + commit_form.go |
| Commit 页 | `commit_page.go`(新) `changes.go` `commit_form.go`(新) | 三栏编排；Changes/Staged 两段＋右键＋批量；表单含 Amend 与内联校验 |
| Diff | `diff.go` `panels.go` | Unified/Side-by-Side 切换；Log 详情文件行点击加载单文件 Diff |
| Git Log | `history.go` `history_workspace.go` `branch_navigation.go` | 树默认 240px 等微调 |
| Worktree | `worktree_page.go`(新) `repo_explorer.go` | 创建/删除/状态，从 repo_explorer 迁出 branchesView |
| Git 写层 | `domain/git.go` `git/{write,local,write_environment}.go` `schemas/*` `application/features.go` | 见下 |

新增写操作（固定 argv，全部经确认对话框，沿用失败复核/Stale 流）：

- Discard：`restore --worktree -- <paths>`（仅限 status 中出现的未暂存路径，rename 两半一并丢弃需 `checkout --` 处理旧路径时以实测为准）；DTO 复用 Paths。
- Amend：`commit --amend --file=-`（消息 stdin；保持 hooks/身份环境；不改动作者信息）。
- 批量 Stage/Unstage：DTO 不变（Paths 多值），UI 一次传入组内全部路径，确认文案列出数量与首末路径。
- Worktree 创建：`worktree add <path> <ref>`（ref 必须为已存在的本地分支或完整 OID，拒绝自由文本参数注入面）；删除：`worktree remove <path>`（拒绝主工作目录；危险确认）。

共享不变式：App 字段与 `RunWrite` 签名不变（新增枚举值向后兼容）；`ReadArgs` 白名单同步扩展；schema 与 Go DTO 契约测试同步。
