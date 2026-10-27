# Workspace Commit / Changes 对齐

2026-10-27：Changes 分组标题为 40 DIP，Commit Diff 标题此前为 48 DIP，导致左右标题下沿错位。Diff 标题统一为 40 DIP 并垂直居中；多仓库 workspace 切换栏禁止压缩，内部按钮、当前仓库标记及计数垂直居中。

新增多仓库布局回归，在 1000×550 和 1000×850 下检查：仓库栏与主体不重叠、Changes/Diff 顶边相同、标题高度一致、提交表单不超出右侧面板底边。

验证：TestMultiRepositoryCommitAlignment、TestWorkspacePanelPositions、TestWorkspaceBarListsRepositoriesWithCounts 通过；make build 通过并重建 bin/git-viewer。未做物理 GUI 验收。保留任务开始前 app.go、changes.go 和 layout-three-pane-execution.md 的已有修改，不纳入本次提交。

后续按截图反馈：移除主体顶部独立的 workspace 仓库按钮栏，将变更计数与刷新入口并入现有 Repository 下拉的 Workspace folders 分组；切换仓库后关闭下拉。主体 Changes/Diff 直接从模式栏顶部开始，保持左右标题对齐。更新多仓库布局、仓库标记和切换回归，覆盖计数与刷新入口。相关五项测试通过，原生构建通过；未做物理 GUI 验收。

## 下拉与 Commit 文件选择修复

2026-10-27：Repository 下拉按钮此前没有点击处理器，测试通过直接修改 `RepoMenuOpen` 绕过了真实交互。现在 Repository、Workspace、Branch、Connection/Git 下拉均由按钮点击切换。Workspace 仓库选择会关闭 Repository 下拉。

Changes 文件行加入复选框，复选框状态以 Git index 为准：勾选未暂存文件执行 Stage，取消勾选已暂存文件执行 Unstage；Commit 仍提交 index 中的文件，因此可只提交勾选文件。确认被拒绝或写入失败时，复选框会按实际 index 状态恢复。

验证：拒绝/接受暂存、取消暂存、仅提交所选文件且未选文件保持未跟踪的端到端测试通过；Repository 真实点击打开与切换后关闭测试通过；`go test ./internal/ui/local -count=1`、`go vet ./internal/ui/local`、`make build` 均通过。未做物理 GUI 验收。
