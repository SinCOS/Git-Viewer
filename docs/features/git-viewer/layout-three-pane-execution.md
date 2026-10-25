# 三栏布局改版（按设计稿）

用户给出设计稿：顶栏（标题、Workspace 名、SSH 徽标、刷新）、左栏 Changes/Worktrees/Branches/最近仓库、中栏 Git Log、右栏 Diff + Commit Details、底部状态栏。

## 改动（internal/ui/local/app.go、features.go）

- View 重排为：顶栏（Git Viewer 标题、Workspace 文件名、加载 Spinner、消息、SSH 徽标、Refresh/Cancel/Choose）、featureToolbar、主体 Split、底部状态栏。
- 左栏固定为 Changes 区：分组计数（仅非零组）+ 可选中的文件列表（原 Status 列表逻辑迁入），下方滚动区放最近仓库、Workspace、Worktrees、Branches 摘要。
- 中栏默认 Git Log（历史图），保留 Status 与 Branches 视图切换；历史为空时显示 "Load history" 按钮与空态提示。
- 状态栏：左侧 `分支 · N changes`（分支来自已加载的 Branches，未加载时提示），右侧 SSH Connected·host 或 Local、Stale 标记。
- 提交信息改入 `ui.Scroll().MaxHeight(100)` 修复溢出重叠（见 diff-copy-overlap-fix-execution.md）。

按用户要求按钮一个未删："Status" 按钮恢复且状态信息同时常驻左栏；路径输入/Open path 移入 featureToolbar；History / Graph、Branches / Worktrees、全部 Git 写入按钮保持原标签与位置语义。

## 验证

- `go build ./...`、`go test ./internal/ui/local/`、`make check` 全部通过；bin/git-viewer 已重建。
- GUI 实机观感（配色、图标、提交图圆点样式为设计稿效果，当前用主题色圆点近似）未逐项对照，待用户验收。
