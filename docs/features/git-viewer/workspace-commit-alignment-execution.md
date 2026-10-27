# Workspace Commit / Changes 对齐

2026-10-27：Changes 分组标题为 40 DIP，Commit Diff 标题此前为 48 DIP，导致左右标题下沿错位。Diff 标题统一为 40 DIP 并垂直居中；多仓库 workspace 切换栏禁止压缩，内部按钮、当前仓库标记及计数垂直居中。

新增多仓库布局回归，在 1000×550 和 1000×850 下检查：仓库栏与主体不重叠、Changes/Diff 顶边相同、标题高度一致、提交表单不超出右侧面板底边。

验证：TestMultiRepositoryCommitAlignment、TestWorkspacePanelPositions、TestWorkspaceBarListsRepositoriesWithCounts 通过；make build 通过并重建 bin/git-viewer。未做物理 GUI 验收。保留任务开始前 app.go、changes.go 和 layout-three-pane-execution.md 的已有修改，不纳入本次提交。

后续按截图反馈：移除主体顶部独立的 workspace 仓库按钮栏，将变更计数与刷新入口并入现有 Repository 下拉的 Workspace folders 分组；切换仓库后关闭下拉。主体 Changes/Diff 直接从模式栏顶部开始，保持左右标题对齐。更新多仓库布局、仓库标记和切换回归，覆盖计数与刷新入口。相关五项测试通过，原生构建通过；未做物理 GUI 验收。
