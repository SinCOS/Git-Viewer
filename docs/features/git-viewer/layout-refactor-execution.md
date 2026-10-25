# UI 布局重构（UL001–UL008）执行记录

按用户授权"拆分多 agent 开干"执行。设计与契约见 [03 补充章](03-technical-design.md)、[contract-ui.md](evidence/parallel/contract-ui.md)、[布局图](evidence/ui-layout-sketch.svg)。

## 分工与提交

- UL001（主 Agent）：基线 `798e1c0`——app.go/features.go 瘦身为冻结接口，拆出 shell/prefs/repo_explorer/changes/history/graph/diff/operations 八个文件；左竖栏三模式（ui.Sidebar，与按钮/⌘1/2/3 同步）、prefs 原子 JSON。
- UL002（Agent）：`29e16f0` patch 应用——当前仓库 `●` 选中态、Load branches 按钮。
- UL003（Agent）：`e8d2bd1` 合并——文件行 Stage/Unstage Checkbox（经 RunWrite 确认流，状态随 Entry.Group 派生）。
- UL004（Agent）：`7ac5be4` 合并——Git Log 标题、COMMIT DETAILS 小节、万行虚拟化保持。
- UL005（Agent）：`8f6c7b0` 合并——第二父边 Danger 色、graphHitTest、冻结布局参数常量化。
- UL006（Agent）：`2e1cfda` 合并——COMMIT DETAILS 分组、FindBar（per-App 状态存 sync.Map，未改冻结字段）。
- UL007（Agent）：`a83f9fc` 合并——Commit and Push 按钮（因 start() 写串行拦截实现为两步：先 Commit，Push 仍走既有按钮，说明文本标注）、Connect 分组。
- UL008（主 Agent，集成）：合并 5 个 worktree 分支 + UL002 patch；修复 UL003 上报的共享缺陷——`&a.StatusList` 被 sidebar 与 statusView 两个 List 同帧共用导致整窗 panic；修复方式：statusView 改为计数概览，交互列表（含 Checkbox）统一在 explorer sidebar，并加回归测试 TestStatusPaneWithEntriesDoesNotPanic；变更 UL003 测试改为整窗渲染。

## 验证

- 每条工作线自检 go build/vet/test 全绿后提交，白名单核对无交集。
- 集成后 `make check` 全绿、`go test -race ./internal/ui/local/` 通过、bin/git-viewer 重建。
- 未验证项：物理鼠标/键盘输入、实机观感（Accent 对比度、FindBar 快捷键 ⌘G/F3）、SSH 远端勾选流程、窗口 bounds 持久化（需 main.go WindowOptions，未在本轮范围）。Commit and Push 为两步实现，与契约原文差异已记录。
