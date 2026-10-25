# UI 布局重构（UL 组）：共享契约与文件所有权

用户已授权"拆分多 agent 开干"。基线提交：UL001 完成后的 Shell 迁移提交（见 git log：`refactor(ui): UL001 shell split…`）。各工作线从该提交建立独立 worktree，只提交白名单内文件。

## 冻结接口（任何工作线不得更改）

- `internal/ui/local/app.go` 与 `features.go`：App 字段、Open/refresh/Select/CommitDiff/loadDiff/applyStatus/applyPreview/Cancel/Close/start/OpenSSH/LoadHistory/LoadBranches/RunWrite 等方法签名。
- `shell.go`：View/modeRail/mainView/statusBar 与 UIState；prefs.go。
- `domain` DTO、schema、application.LocalService、infrastructure 各包。
- Go 1.27.3、MyGo v0.0.0-20261025001514-9c69cf829ece。
- 事件流：UI 回调 → App.start → Dispatch 回主线程；面板不得直接调用其他面板的 View。
- 按钮文案不得删除或改名（用户要求按钮一个不能少；测试依赖现有文案）。

## 文件所有权（每线只写自己的文件 + 自己的 *_test.go）

| 工作线 | 可写文件 |
|---|---|
| UL002 Repository | internal/ui/local/repo_explorer.go、repo_explorer_test.go |
| UL003 Changes | internal/ui/local/changes.go、changes_test.go |
| UL004 History | internal/ui/local/history.go、history_test.go |
| UL005 Graph | internal/ui/local/graph.go、graph_test.go（依赖 UL004 的行结构，与 UL004 串行或同组） |
| UL006 Diff | internal/ui/local/diff.go、diff_test.go |
| UL007 Operations | internal/ui/local/operations.go、operations_test.go |
| 主 Agent（集成） | 其余全部文件；发现共享缺陷时单独修复提交 |

现有 app_test.go、features_test.go、diff_test.go、dialogs_test.go、shell_test.go 为共享测试基线，工作线不得修改；若其改动导致基线测试失败，说明越界，须回调。共享 Go module（go.mod/go.sum）不得更改。

## 模块职责边界

- UL002：仓库发现/最近列表/Workspace 控制/Worktree+Branch 摘要；Worktree 模式（Pane=="Branches" 的 branchesView 也归此线，已在本文件）。
- UL003：Changes 文件行加 Stage/Unstage Checkbox（勾选=Stage、取消=Unstage，经 RunWrite），保留行点击选 diff 与现有行标签文案。
- UL004：History 分页列表+提交详情；Git Log 模式合并视图微调仅限本文件。
- UL005：graph.go 绘制独立化、命中测试；不得改 history.go 的行构建调用方式（graphRow(c,i) 签名冻结）。
- UL006：Diff FindBar（ui.FindBar）、Copy diff 保持、提交详情头。
- UL007：Commit 区布局（消息表单+按钮）、Commit and Push 组合（Commit 成功后经确认流再 RunWrite(Push)，失败不静默）、SSH 控件归位；不得删除任何现有按钮。

## 验收（每线自检后才报告）

1. `go build ./...`、`go vet ./internal/ui/local/`、`go test ./internal/ui/local/` 全绿。
2. 新增 mock 布局测试（ui.NewTester + 手工填充状态，不依赖真实 Git）。
3. 提交信息注明任务 ID；只提交白名单文件。
4. 明确列出未验证项（如物理输入、实机观感），不得宣称 GUI 实测。
