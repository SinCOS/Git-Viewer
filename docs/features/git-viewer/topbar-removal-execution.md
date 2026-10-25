# UL020 移除冗余顶部栏

按用户截图要求，移除应用内最上方整条 `Git Viewer / Ready / Refresh / Cancel / Choose repository…` 栏，Workspace/Repository/分支/Fetch/Pull/Push 等上下文工具栏成为内容区首行。

操作状态与错误消息移至现有底部状态栏，单行省略并提供完整悬停文本；加载与原生对话框等待时在状态栏显示 spinner。SSH 连接信息沿用底部显示。Choose repository… 移入 Repository 弹出菜单，刷新继续通过面板按钮、Cmd+R 与原生菜单使用，取消继续通过原生 Repository 菜单使用。

验证：

- `go test -race ./internal/ui/local ./cmd/git-viewer -count=1` 通过，覆盖入口、模式布局、文件选择异步回调、读取/刷新、历史与变更列表。
- `make build` 原生构建通过。
- `git diff --check` 通过。

同步修改旧测试使用的新入口：目录选择先打开 Repository 菜单；刷新点击 Refresh changes；入口测试确认原有 Git Viewer 标题不再显示。未重复全量仓库测试或人工 GUI 验收。
