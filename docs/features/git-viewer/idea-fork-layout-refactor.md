# JetBrains/Fork 风格布局重构 — 设计与执行记录

日期：2026-10-27。目标：在不改动 Git 行为、数据模型和操作流程的前提下，重构 `internal/ui/local` 的窗口结构，参考 JetBrains IDEA Git 工具窗口与 Fork 的信息架构。

## 现状（重构前）

```
Column（窗口）
├── featureToolbar（顶部：Workspace 弹层 / Repository 弹层 / 当前 Branch 弹层 / Fetch·Pull·Push / Connection 弹层）
├── Row "Workspace body"
│   ├── modeRail（左侧 Sidebar，180px：Commit / Git Log / Worktree）★导航重复来源
│   └── Column mainView（+ 多仓库 workspaceReposBar）
│       ├── commit：标题行 + Split(StatusWidth)[statusView | diffView] + commitSidebar
│       ├── log：Split(BranchTreeWidth, 默认 280)[branchNavigation | historyWorkspace]
│       │   historyWorkspace = Row[history(手写横向分隔条) | historyInspector 右侧详情]
│       └── worktree：branchesView
└── statusBar（28px 固定）
```

主要问题：左侧 180px 导航栏与顶部工具栏重复且浪费空间；Branch Tree 默认 280px 偏宽且不能隐藏；Commit Details 在右侧、易被挤占，且要求是选中 commit 后在**底部**展示；Branch Tree 缺 Tags/Stashes 分类和右键菜单。

## 目标区域划分

```
Column（窗口，macOS 原生）
├── 统一顶部工具栏（唯一导航入口，ui.Toolbar 行）
│   ├── 模式切换：Commit｜Git Log｜Worktree（保持 1/2/3 顺序与快捷键）
│   ├── Workspace 弹层、Repository 弹层、当前 Branch 弹层（沿用既有实现）
│   ├── 面板开关：Branch Tree 显隐（log 页）、Commit Details 显隐（log 页）
│   └── Fetch / Pull (fast-forward) / Push、Connection / Git 弹层
├── 主区域（按模式挂载，同一时刻只挂一个）
│   ├── Commit 页：Local Changes(可折叠) | Diff 预览 + 底部 Commit Message 表单（不变）
│   ├── Git Log 页：
│   │   ├── Branch Tree 面板（可拖宽、可隐藏、可搜索、可折叠、右键菜单）
│   │   │   └── All Branches / HEAD / Local / Remote / Tags / Stashes
│   │   ├── Commit History 表（Graph|Description|Date|Author|Hash，Grow）
│   │   └── 底部 Commit Details 面板（元数据 + 文件变更 + Diff，可拖高、可隐藏）
│   └── Worktree 页：Worktree List | Worktree Details（不变）
└── statusBar（不变）
```

## 交互状态

| 状态 | 位置 | 说明 |
| --- | --- | --- |
| `Mode` / `Pane` / `railPane` | App | 模式映射保持：commit→Status、log→""、worktree→Branches；rail 移除后 `railPane` 仍用于防振荡 |
| `BranchTreeWidth` / `BranchTreeHidden` | App + prefs | 树宽拖动（既有手写分隔条模式）；显隐开关进 ui-prefs.json |
| `HistoryDetailsOpen` / `HistoryDetailsHeight` | App + prefs | 详情改为表格下方纵向面板；高度纵向拖动；显隐进 prefs |
| `branchCollapsed` / `BranchQuery` | App | 树折叠/搜索沿用；新增 Tags/Stashes 分区同样可折叠、参与搜索 |
| 新增 `Tags []TagRecord` / `Stashes []StashRecord` | App | 随 Branches 一起加载；`resetHistory` 清空 |

## 重构影响范围（分阶段，避免同文件并行）

| 阶段 | 文件 | 内容 |
| --- | --- | --- |
| S1 骨架 | shell.go、operations.go、layout_test.go、shell_test.go | 移除左侧 modeRail，模式切换并入工具栏；三页挂载结构不变 |
| S2 Git Log | history_workspace.go、history.go、prefs.go、相关测试 | 详情移至底部纵向面板（沿用"保住 Table 原生控件身份"的 key/手写分隔条模式）；树显隐开关；prefs 持久化 |
| S3 树分类 | branch_navigation.go、infrastructure/git/history.go、application/features.go、相关测试 | Tags/Stashes 只读查询（固定 argv，for-each-ref refs/tags/ 与 stash list）；树分区渲染与右键菜单（切换分支走既有确认写流程，其余仅复制名称） |
| S4 样式 | icons.go / 各面板小改 | 统一面板标题 12 Bold、说明文字 10–11 muted、行高 30、图标 16×16、选中态 SurfacePressed+Text（沿用 Git Log 表格的 neutral 方案） |

不变式：`ui.Table`/`ui.List` 的行状态、`HistoryList`/`branchList` 的 Key/Label 契约、所有 Git 读写流程、confirm 对话框、后台同步与 reveal 重建逻辑均不动。

## 验收口径

每阶段 `go build` + `go test ./internal/ui/local`（race 由最终 make check 覆盖）；最终 `make check` + `bin/git-viewer -smoke`（只读打开当前仓库）。Tester 帧断言验证布局几何与交互状态；物理鼠标/键盘交互与真实 GUI 外观不在本轮宣称实测。
