# UL021 历史与 Diff 加载期间保留画面

## 修改

用户反馈切换 Git Log 分支时界面闪白。原先 selectHistoryRef 在读取开始前立即清空 GraphRows、HistoryPage、提交选择与详情，导致下一帧移除提交列表，直到 Git 请求完成才重新创建。

现在将请求范围作为 loadHistory 参数传入，读取期间保留当前 HistoryRef、分页、提交列表、图形、选择和详情。结果成功后在同一 UI 回调中更新范围及内容；失败或取消保留原画面和原分页，沿用状态栏显示错误。加载期间仍使用已有禁用/取消/generation 机制。

同时移除 loadDiff 在请求前对 Preview/CommitDetails/lines 的清空，选择另一个提交或文件时保留已有预览，准备好后整体替换。首次加载和切换仓库仍使用原有空态/重置逻辑；成功切换历史范围后清理旧范围的提交选择和详情。

## 验证

新增真实隔离仓库回归测试，UI Dispatch 队列控制结果应用时机：

- 分支读取未应用期间，原范围、分页、提交文字和详情持续显示，不进入 Loading history 空态。
- 取消及迟到回调不改变原画面；读取失败保留内容并显示 Stale；重试成功时范围与列表一起切换。
- 提交详情和 Diff 等待替换时不清空，失败后保留最后成功预览。

实际通过：

- `go test -race ./internal/ui/local ./cmd/git-viewer -count=1`。
- `go vet ./internal/ui/local ./cmd/git-viewer`。
- `go tool mygo vet`。
- `make build`。
- `git diff --check`。

本轮未重复基础设施全量测试、原生窗口冒烟或人工视觉验收；验证证明加载期间不再主动清空视图，不能替代真机对所有绘制闪烁的观察。
