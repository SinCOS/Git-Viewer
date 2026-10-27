# Git Log 界面对齐执行记录

日期：2026-10-28

参考图：`/Users/richard/Desktop/Git Viewer Commit Graph and Diff.png`。

## 实现

- Git Log 提交表调整为 `Graph / Description / Branches / Author / Date / Commit` 六列。Description 使用剩余宽度，Branches、Author、Date 和 Commit 使用紧凑固定宽度并在窄窗口中保持最小可用宽度。
- Description 只显示提交主题；本地与远程分支标签移入独立 Branches 列，分支切换箭头显示为 `→`。Graph 只在相邻行的 lane 连续时补画纵向连接线，避免换列时出现错误连线。
- 底部详情改为横向分栏：左侧固定摘要区显示提交主题、作者时间、短哈希、完整消息、变更文件及统计；右侧 Diff 区显示 `Diff / File History / Blame`、Unified/Side-by-Side 切换、文件路径与状态。
- 选择提交后保留原预览直到新数据返回，再自动加载首个变更文件。切换历史范围且没有可保留的提交选择时，清除旧详情、预览和滚动状态。
- 详情面板默认展开，继续支持显式折叠与高度偏好；最低高度提高到 140 px，避免摘要区和 Diff 工具条被压扁。

## 验证

- 六列在 480 px 窄宽下保持 `Graph → Description → Branches → Author → Date → Commit` 顺序，单元格未越出提交表。
- `TestHistoryHeaderAndCommitDetails`、`TestHistoryRowsFollowViewportWidth`、`TestHistoryColumnsStayAlignedAtNarrowWidths`、Graph 与偏好测试通过。
- 完整 `make check` 通过，包括 Go/MyGo vet、全量 race 测试和本机原生构建。
- 程序驱动原生视图生成[视觉快照](evidence/git-log-aligned.png)，确认六列表头与内容列对齐、分支标签独立显示，底部摘要与 Diff 分栏符合参考结构。

## 边界

快照使用隔离 fixture 数据，仅验证结构、列序和可见布局。真实窗口中的鼠标拖列、键盘导航、浅/深主题对比度和高 DPI 观感仍需人工验收。此次没有扩大 Git 写入权限，也没有引入 WebView 或前端技术。
