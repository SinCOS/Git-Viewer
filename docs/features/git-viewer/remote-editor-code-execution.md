# Git Viewer 远程 SSH 编辑与 `code` 命令集成

## 已实现

- 多窗口入口改为 `windowManager`：每个窗口拥有独立 `local.App`、编辑器标签、远程 SFTP 客户端和后台任务；关闭单个窗口只结束该会话，最后一个窗口关闭后退出。菜单按目标窗口路由，单实例锁和 `gitviewer` URL scheme 已接入。
- `File > Open remote…` 提供应用内远程目标表单：SSH Host 支持从用户 SSH Config 建议和手工输入，远端路径必须是绝对路径；`.code-workspace` 自动按 Workspace 打开，其他路径按文件夹打开，并展示最近远程位置。应用仍不收集或保存认证信息。
- 新增 `EditorLocation`、`EditorRoot`、`RemoteTarget`、`RemoteFileInfo`、`RemoteFileSystem`。远程路径始终使用 POSIX `path`，不经过本机 `filepath`。
- 远程文件通过系统 `ssh -s <host> sftp` 和固定 `github.com/pkg/sftp v1.13.12` 访问。复用严格 OpenSSH 参数：`BatchMode=yes`、`StrictHostKeyChecking=yes`、连接超时、禁止本地命令；认证只使用 SSH Config、密钥和 ssh-agent。
- SFTP 层限制到打开时的规范化根目录，拒绝符号链接读取/目录遍历逃逸，文件读取上限 8 MiB。保存使用同目录临时文件、关闭、权限设置和 `PosixRename`；失败不清理编辑器缓冲区。
- Explorer 对远程目录按展开懒加载；Quick Open 在后台跨根遍历，取消/新请求会通过 generation 丢弃过期结果，全局上限 20,000 文件。远程文件标签、脏标记、关闭和保存与本地标签共用编辑器状态。
- 打开远程文件记录 size/mtime；保存前发现外部变化时提示覆盖、重新加载或取消。保存失败或断线时保留未保存内容并标记 stale。
- `workspace.Store` 支持 `LocalStore` 与 `RemoteStore`。远程 `.code-workspace` 保留 JSONC 注释、未知字段和 revision；相对路径按 Workspace 文件目录解析；同一 Host 多根、重复根去重和 revision 冲突检测已实现。
- 远程 Git 复用现有固定 argv、Status/Diff/History/Branch/Worktree/Stage/Commit/Fetch/Pull/Push 和 common-dir 写队列。远程多根按 repository ID 去重；保存文件时只刷新对应仓库，其他根目录通过 Workspace 计数刷新。
- `internal/bridge` 建立 token 保护的本地 Unix listener，并通过 `ssh -R` 把临时远端 socket 反向前向到本机。endpoint 文件以 0600 原子写入，只包含 socket、token 和 Host 别名；HTTP 协议只接受 NUL 分隔 argv，且校验 Host、token、绝对 cwd、参数长度和 NUL。
- `internal/codecmd` 支持 `--new-window`、`--reuse-window`、`--add`、`--goto`、`--diff`、`--wait`、`--folder-uri`、`--file-uri` 和普通文件/目录/Workspace 参数；未知命令或扩展管理命令委托给原 `code`。
- `internal/remotecli` 安装 `~/.local/bin/code` 与 `gitviewer` 别名。已有可写 `code` 先备份再替换；不可写时安装到用户目录并报告 PATH 顺序，不执行 sudo、不修改 shell 配置。远端安装记录用于区分首次提示和后续自动更新。
- 通道不可用时远端脚本输出 OSC 8 可点击深链和普通 `gitviewer://open?...` 文本。路径使用 Base64 URL-safe 编码，Host 和绝对路径均严格校验。

## 验证结果

已通过：

```sh
.tools/go/bin/go test -race -count=1 \
  ./cmd/git-viewer ./internal/bridge ./internal/remotecli \
  ./internal/deeplink ./internal/codecmd ./internal/domain \
  ./internal/infrastructure/storage ./internal/infrastructure/sshfs \
  ./internal/infrastructure/workspace

.tools/go/bin/go test -race -count=1 ./internal/ui/local \
  -run 'Test(Remote|EditorTabs|SaveEditor|OpenFolderDialog|Workspace|CommitForm|Amend|CommitAndPush)'

.tools/go/bin/go vet ./...
.tools/go/bin/go tool mygo vet
make build
make smoke
```

loopback SSH 集成测试使用临时 sshd、`Subsystem sftp internal-sftp`、临时密钥/config/known_hosts 和 `AllowStreamLocalForwarding yes`，覆盖 SFTP 读写/权限/特殊路径/符号链接逃逸、反向 Unix socket、token 拒绝和断线退出。远端安装测试在隔离 HOME 中执行真实 shell 脚本，覆盖可写 `code` 备份替换、只读 `code` 回退、委托和幂等记录读取。

## 已知验证边界

- 全量 `go test ./...` 在本次改动前和改动后都会失败于 4 个既有 `internal/ui/local` 测试：`TestGroupedTreeSelectionMapsToGitEntries`、`TestOpenEditorFileDialogReturnsBeforeBlocking`、`TestOperationsDisabledWhileLoading`、`TestCommitReadDoesNotFadeUnrelatedPanels`。已在 HEAD 独立 worktree 复现，不属于本次远程功能回归；本轮未修改这些无关测试或 UI 行为。
- `internal/infrastructure/process/run.go` 在本轮开始前已有未格式化改动，因此没有运行会写文件的 `make fmt`/完整 `make check`。所有本轮 Go 文件已单独 `gofmt`，`git diff --check` 通过。
- 尚未连接真实 Linux/macOS SSH 主机完成 `code .`、`--new-window`、`--reuse-window`、`--add`、`--goto`、`--diff`、`--wait` 的人工验收；相关行为由参数、bridge、安装和 loopback socket 测试覆盖。
- 未验证真实多窗口焦点、macOS URL scheme 冷启动/运行中唤醒、远端安装 PATH 冲突后的实际 shell 行为，以及远端主机无 `curl`/反向 forwarding 被 sshd 禁用时的终端深链点击体验。
- 首版不实现密码/2FA 交互、常驻远端服务、Windows 远端、本地与远程 Workspace 混排，也不接管扩展管理或其他非编辑器命令。
