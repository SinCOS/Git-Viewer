# 远程连接与目录选择 UI 执行记录

日期：2026-10-28。对应 [已确认方案](remote-ui-implementation-plan.md)。

## 已实现

- 连接面板重排为左对齐标题、说明、主机输入与最近连接。最近连接采用主机/路径两行、图标、整行悬停与完整路径提示。
- 密码面板统一左对齐、紧凑间距与单层边框；保留凭据掩码、提交、取消和原有内存生命周期。
- 远程目录采用左对齐图标列表、名称/类型列、整行选中与悬停态；单击选择，目录双击/回车进入。工作区文件选中后主操作切换为 Open workspace。
- 路径导航与待打开目标分离：输入路径回车或 Go 导航；上级目录、刷新、当前目录筛选、加载与空结果提示可用。底部明确显示实际待打开路径。
- 保留先连接后选目录、目录/工作区过滤、请求代际保护、认证会话复用和未保存文件切换保护。未添加不可靠的修改时间元数据。
- 窗口左下角新增 24px SSH 区域，所有模式位置一致；菜单提供连接主机、打开/切换远程目录及断开操作。本地、连接中、已有会话、连接失败和目录浏览失败具有文本区分。
- 断开关闭当前窗口的远程浏览/文件会话和远程标签，清理远程工作区及远程仓库状态，并清除窗口管理器的远程目标。远程脏缓冲区会阻止断开，不静默丢弃修改；共享窗口管理器持有的主机桥接连接仍按原有应用生命周期管理。
- 面板宽高按窗口尺寸收缩，最近连接可滚动；640×480 下主要面板和操作按钮保持可见。

## 验证与证据

通过：

- `go test -race ./internal/ui/local ./cmd/git-viewer -run 'TestRemote|TestDisconnectRemote|TestPassword|TestModeRail|TestWorkspacePanelPositions' -count=1`。
- 新增回归覆盖：亮暗主题文件名对齐、单击选择/回车导航、工作区操作切换、当前目录筛选、左下角位置与菜单断开、脏缓冲区保护、窄窗口可见性。
- `go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go build -o bin/git-viewer ./cmd/git-viewer`。
- 隔离配置与单实例运行目录的原生 `-smoke`：输出 `native data main-thread update passed`、`background workers stopped`，退出码 0。
- 隔离的原生预览窗口人工点击验证：取消面板 → 左下角 Remote connection 菜单 → Connect to Host… → 连接面板重新出现。没有连接任何真实主机；预览进程和临时源文件已清理。
- MyGo 原生 UI 渲染器导出的亮/暗主题截图：连接、目录、密码共六张。检查标题、表单与目录名左对齐，以及目录选中态和按钮布局。这些截图使用模拟数据，不代表真实 SSH 登录。

截图：

- [连接·亮色](evidence/remote-ui/login-light.png) / [暗色](evidence/remote-ui/login-dark.png)
- [目录·亮色](evidence/remote-ui/picker-light.png) / [暗色](evidence/remote-ui/picker-dark.png)
- [密码·亮色](evidence/remote-ui/password-light.png) / [暗色](evidence/remote-ui/password-dark.png)

截图可重建（不会在普通测试中写文件）：

```sh
GITVIEWER_CAPTURE_REMOTE_UI="$PWD/docs/features/git-viewer/evidence/remote-ui" .tools/go/bin/go test ./internal/ui/local -run '^TestCaptureRemoteUI$' -count=1
```

## 广域测试限制

首次运行 `go test ./internal/ui/local ./cmd/git-viewer`，CLI 包通过；UI 包以下四项失败。已对提交 `95c931a` 的隔离源码副本执行同名测试，四项均原样复现：

- `TestGroupedTreeSelectionMapsToGitEntries`：重复文件名选择预期不匹配。
- `TestOpenEditorFileDialogReturnsBeforeBlocking`：对话框结果调度超时。
- `TestOperationsDisabledWhileLoading`：仍查找已不存在的 Connect SSH 按钮。
- `TestCommitReadDoesNotFadeUnrelatedPanels`：仍查找已移除的 Fetch 区域。

全项目 `go test -race -count=1 ./...`（跳过上述四项）中，其他包通过，UI 包在 `TestChangesContextMenuUsesFileOperations` 的 `changes_test.go:132/169` 报测试变量数据竞争。本次未修改该文件；仅单独重跑同名基线测试（含 count=10）未复现，不能宣称全量 race 通过。随后在 `95c931a` 隔离源码上运行同样的 UI 全包 race 测试（跳过上述四项），同一测试在相同行号再次报告 DATA RACE，确认也是基线问题。本次保留该问题记录，不混入无关测试修复。

## 未验证边界

- 未进行真实主机密码/验证码登录、网络中断及重连实测。
- SSH 标签基于窗口已有会话与已知请求错误，不做后台心跳，也不承诺持续可达；未将保存的主机名直接显示为已连接。
- 此次未更新正在运行的旧版正式应用包；已生成本地 `bin/git-viewer`，启动新版后可看到界面变化。
- 没有修改系统工具链、SSH 配置、凭据策略或远端命令协议；提交仅包含本任务相关文件。
