# 原生窗口补充验证（A 工作线）

冻结基线：`53e686f6cfdaae9d162395f49d6de6007b2acc8f`。新增独立探针 `cmd/phase0-ui-probe`，复用现有 poc.App，不修改共享 UI 实现或应用入口。

```sh
go run ./cmd/phase0-ui-probe \
  -baseline 53e686f6cfdaae9d162395f49d6de6007b2acc8f \
  -output docs/features/git-viewer/evidence/native-ui
```

探针在真实 MyGo 原生窗口中顺序运行四个场景，各执行 50 次主线程状态更新，保存原生内容截图和 JSON 指标。窗口关闭或场景未完成会返回非零。测试测量期间不同时运行 SSH 测试。

- [列表](list.png)：10,000 行，脚本定位和选择，实际观察到最后一行进入可见范围；最多每 pass 构建 48 行。
- [Graph](graph.png)：200 节点及分支标签，缩小窗口后观察到双向滚动偏移，恢复尺寸后完整显示。
- [Diff](diff.png)：中文长行横向滚动；[二进制占位](binary.png) 单独渲染。

报告基于工作线新增探针及冻结业务代码；不是声称冻结基线本身已有该命令。[report.json](report.json) 保留机器、命令、场景与指标。另用 `go run -race` 执行同一探针通过，race 产物不用于性能基线；原有 `ui.NewTester` 的选择/指针命中/复制测试也通过。

本次原生 view 构建 p95：列表 0.151 ms、Graph 0.009 ms、Diff 0.031 ms。主线程回调排队 p95 0.087 ms，最大约 35 ms，完整数据保留在报告中。

这些指标不包含完整布局、GPU 绘制和显示提交，不是 FPS 或物理输入延迟。探针用程序驱动状态，不注入真实鼠标/键盘；物理输入、输入法与 GPU 计数器仍未验证。内存指标仅覆盖 Go heap，不代表整个进程或 GPU 内存。Apple Silicon 实机验收仍缺环境。
