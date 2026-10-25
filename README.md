# Git Viewer — MyGo Native

纯 Go + `egoist/mygo/ui` 原生 Git 桌面应用，支持本地与系统 OpenSSH 远程仓库、Status/Diff、Workspace、History/Graph、Branch/Worktree、安全写入和同步。Phase 0 模拟 UI 保留为独立探针。

固定 Go **1.27.3**、MyGo **v0.0.0-20261025001514-9c69cf829ece**。应用入口不使用 WebView、HTML、JavaScript、TypeScript 或 Vue。Git 版本门槛为 2.38，本机实测 2.39.5；最低 macOS 兼容性矩阵尚未验证。

## 运行

```sh
make run
```

点击 `Choose repository…`，或输入路径后点击 `Open path`。支持子目录、软链接和独立 worktree；最近仓库保存在 `os.UserConfigDir()/GitViewer/recent-repositories.json`。`Cmd+O` 打开本地仓库，`Cmd+R` 刷新，`Cancel` 取消当前操作。

SSH 栏使用用户 SSH Config 的 Host 别名与远端绝对路径。OpenSSH 负责身份和连接配置；应用固定非交互认证与严格 host key 检查，不修改用户 SSH 配置或保存凭据。Host 下拉建议仅列当前主配置的字面别名，Include/Match 和通配 Host 仍由系统 OpenSSH 解析，可直接输入别名。

```sh
.tools/go/bin/go run ./cmd/git-viewer -repo /absolute/repository/path
.tools/go/bin/go run ./cmd/git-viewer -ssh-host my-server -remote-repo /absolute/remote/path
.tools/go/bin/go run ./cmd/git-viewer -workspace /absolute/project.code-workspace
```

`-config /absolute/path/recent.json` 指定隔离应用配置。相邻的 `recent.json.locations.json` 保存最近 SSH 别名/路径与 Workspace 路径，共最多 20 项，不包含认证材料。

## 查看与 Workspace

Status 区分 Staged、Unstaged、Untracked、Conflicted；同一文件的双区修改分别展示。选择文件查看统一 Diff；支持文本选择、复制、中文和长行。超过 512 KiB / 10,000 行及二进制内容降级提示。完整提交 OID 或 History 选择可查看提交消息、变更文件和 Diff；merge Diff 使用第一父提交。

`History / Graph` 按 100 条分页，以首批完整提交 OID 集合固定游标；覆盖 HEAD、本地/远程分支和 commit tags。原生 Graph 保留跨页父节点，支持多根、octopus merge 与 detached HEAD。列表虚拟化并支持横向滚动。`Branches / Worktrees` 展示上游、当前分支、锁定、detached 与 prunable 状态。

侧栏打开、创建或保存 `.code-workspace`，可添加/移除目录并打开发现的 Git 仓库。路径基于 Workspace 文件目录解析，规范化后去重。编辑只替换 folders 值，保留外部字段及注释；数组内部注释保留在新数组内。不能安全保留未知 folder 属性时拒绝改写。保存校验 revision、同目录临时写入、fsync、原子替换；外部修改冲突提示重新打开或另存。创建/另存不覆盖已有文件，也不删除目录。

## 写入与刷新

写入按钮需明确确认；Stage/Unstage 只针对所选文件，Commit 只提交当前索引。提交消息通过 stdin 传入，读取正常用户 Git 身份、credential helper 和 hooks。安全切换分支会拒绝脏工作区和已被其他 worktree 占用的分支。

Fetch 显示运行中指示；Pull 只允许 fast-forward；Push 检查上游并显式推送当前分支，禁用 force、mirror、隐式额外 refspec 和自动推送 tag。Git 自身连接 SSH 上游时也强制非交互和严格 host key 校验。不同 worktree 按 common-dir 共用进程内写队列；完成或失败后重读真实 Status。SSH 写入取消、超时或断线后可能显示“状态未知”，不自动重试。

本地监听工作区、git-dir/common-dir，去抖约 500ms，监听失败时用 60 秒兜底刷新。远程前台约 15 秒、后台约 60 秒轮询，失败指数退避至 5 分钟；离线保留数据并标 stale。自动刷新保留所选文件并更新 Diff。可用 `-auto-refresh=false` 关闭。

## 验证与打包

```sh
make check       # 格式、Go/MyGo vet、全量 race、无 cgo 构建
make smoke       # 本机原生窗口、Status 主线程更新和退出清理
make dialog-smoke # macOS 真实打开/保存/确认面板、取消、关闭窗口与退出
make package     # Intel/Apple Silicon .app，ad hoc 签名，不制作 DMG
```

产物为 `bin/git-viewer` 与 `bin/package/darwin-{amd64,arm64}/Git Viewer.app`。本机 Intel 启动、安装副本启动及双架构签名校验通过；Apple Silicon 真机启动、物理输入/IME、Developer ID、公证及正式发布尚未完成。当前包是内部开发预览。

SSH 测试使用临时 loopback sshd、临时密钥/config/known_hosts；不修改系统或用户 SSH 配置。本机真实 SSH 集成没有跳过。其他环境缺 sshd 时会明确 skip。所有测试写入、提交及推送均在隔离 fixtures，不操作开发者项目仓库的 Git 状态。

工具链只安装到 `.tools/`。首次 Intel 环境可用 `python3 scripts/bootstrap-go.py` 下载并校验固定版本；代理沿用本机设置。Apple Silicon 需准备对应官方 Go 工具链。

## 记录

- [原生对话框卡死修复与验证](docs/features/git-viewer/dialog-fix-execution.md)
- [全部剩余任务编排](docs/features/git-viewer/remaining-orchestration.md)
- [本轮执行与验收记录](docs/features/git-viewer/remaining-execution.md)
- [内部预览发布说明](docs/features/git-viewer/release-notes-0.1.0.md)
- [任务状态](docs/features/git-viewer/05-tasks.md)
- [Phase 0 执行记录](docs/features/git-viewer/phase-0-execution.md)及[补充验证](docs/features/git-viewer/evidence/parallel/integration.md)
- [Phase 1 历史记录](docs/features/git-viewer/phase-1-execution.md)

Phase 0 原生探针仍可运行 `go run ./cmd/phase0-ui-probe`。其模拟数据与历史性能证据不等同于正式产品或物理输入验收。
