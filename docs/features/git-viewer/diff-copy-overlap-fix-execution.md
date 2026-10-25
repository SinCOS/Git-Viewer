# Diff 视图提交信息文字重叠修复

用户截图显示 commit diff 视图中多行提交信息与下方元素（Changed files、scope 行、Copy diff 按钮等）重叠绘制。

## 原因

`internal/ui/local/app.go` 的 diffView 中提交信息写作
`ui.Text(...).Selectable().MaxHeight(100)`。MyGo 的 MaxHeight 只收缩布局占位框，不裁剪子内容绘制（需要显式 Clip 或滚动容器）。多行提交信息的文本按自然行高完整绘制，溢出元素框，压在按收缩后布局摆放的后续兄弟元素上。

## 修复

把提交信息包进 `ui.Scroll(c).MaxHeight(100).FillWidth()` 垂直滚动容器：容器裁剪溢出内容并支持滚动查看全文，Selectable 保留。

## 验证

- `go build ./...` 通过。
- `go test ./internal/ui/local/` 通过。
- `make check` 全部通过（含 UI 本地测试套件），并重建 bin/git-viewer。

未做 GUI 实机复核；需用户在应用中查看同一 commit 确认重叠消失、信息可滚动。
