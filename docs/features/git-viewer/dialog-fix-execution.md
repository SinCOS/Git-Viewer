# 原生对话框卡死修复（0.1.1 内部预览）

用户反馈 Open Workspace 等按钮点击后卡住，窗口也无法关闭。此前 ui.Tester 将系统面板替换为立即返回的回调；原生 smoke 只覆盖窗口启动与数据加载，未覆盖按钮等待真实 AppKit 面板的路径。这些检查没有发现该问题。

## 原因与修复

打开目录、打开 Workspace、另存为及 Git 确认都曾在 UI 回调中同步调用 MyGo Dialog.Open/Save/Message。API 会等待面板结果；在主线程上还会嵌套处理原生事件，可能重入尚未完成的 UI 构建。

统一使用 `internal/ui/local/dialogs.go`：按钮先返回，独立 goroutine 等待系统面板，结果通过 Dispatch 回主线程。DialogOpen 单独记录状态、阻止重复弹窗及期间启动其他读写；取消恢复之前的状态，关闭后的结果不再打开文件或执行已确认写入。

真实面板探针又确认了第二条故障：窗口带着 sheet 关闭后，原生 API 的回调可能不再返回；直接将该调用加入 App.Wait 会阻塞进程退出。现使用可取消的协调器等待其结果，Close 取消协调器，App.Wait 不再依赖系统面板返回。底层 OS 调用没有 context API，等待该调用的 goroutine 可能保留到进程结束；不会阻塞退出或在关闭后执行业务操作。没有修改 MyGo 版本或系统配置。

## 回归验证

```sh
make check
make dialog-smoke
make package
```

阻塞回调测试覆盖目录选择、Workspace 打开/另存、Git 确认：按钮返回后，仍可构建下一帧；不重复弹窗；即使系统回调尚未返回，Close/Wait 也必须完成；迟到的选择/确认不会启动业务操作。既有真实 Git 写入和 Workspace 完整流程测试适配异步等待，继续验证成功路径。

`cmd/native-dialog-probe` 是独立 macOS 原生探针，复用正式 App 控制器与相同的文件过滤/对话框配置，在真实 UI 构建路径触发上述操作。它验证 AppKit attachedSheet 实际出现、面板等待期间主线程心跳可执行、系统取消后状态恢复，以及带未完成面板关闭窗口后进程正常退出。所有确认均自动取消，不执行 Git 或 Workspace 写入。它使用原生 API 自动操作，不代表物理鼠标/键盘验收。

探针首次发现退出等待不返回；修复后上述六项全部通过。最终检查输出见 [check 日志](evidence/dialog-fix/check.log) 与 [真实面板探针日志](evidence/dialog-fix/native-dialog-probe.log)。

0.1.1 已重新生成本机二进制与两个架构的 .app，ad hoc 签名校验；Apple Silicon 真机和正式发布仍未完成。之前的 0.1.0 执行记录是历史证据，不构成修复前按钮可用性的证明。
