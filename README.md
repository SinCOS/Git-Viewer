# Git Viewer — MyGo Native Phase 1

纯 Go + `egoist/mygo/ui` 原生本地只读 Git 查看器。Phase 1 实现本地仓库打开与最近记录、分区 Status、工作区/暂存区/提交统一 Diff。SSH 产品入口、Workspace、History/Graph 与 Git 写操作按后续阶段实施；Phase 0 模拟组件保留独立探针。

固定版本：Go **1.27.3**，MyGo **v0.0.0-20261025001514-9c69cf829ece**。应用不使用 WebView、HTML、JavaScript、TypeScript 或 Bun。

## 本地运行

需要 macOS、Git、OpenSSH 和 Go 1.27.3。当前机器的 Go 已安装到 `.tools/go`，Makefile 会优先使用它。

```sh
make run
```

点击 `Choose repository…` 选择本地仓库，或在路径框输入目录后点击 `Open path`。支持仓库子目录、软链接和独立 worktree；最近记录自动保存在 `~/Library/Application Support/GitViewer/recent-repositories.json`。

Status 分为 Staged、Unstaged、Untracked、Conflicted，同一文件的双区修改会分别显示。点击文件或用列表方向键选择，右侧加载对应统一 Diff；支持中文、长行、文本选择和 `Copy diff`。输入完整提交 OID 后点击 `View commit diff` 查看该提交差异（merge 使用第一父提交）。二进制及超出 512 KiB / 10,000 行的预览会降级提示。

`Cmd+O` 或菜单 `Repository → Open local repository…` 打开仓库，`Cmd+R` 手动刷新。`Cancel` 中断读取；错误和取消会显示 stale，刷新成功后恢复。当前不自动监听文件变化。

也可从命令行打开仓库，验收时使用独立配置：

```sh
.tools/go/bin/go run ./cmd/git-viewer -repo /absolute/repository/path \
  -config "$PWD/.tools/manual-check/recent.json"
```

在另一台 Intel Mac 上，也可以使用项目的固定版本安装脚本。它只写入 `.tools/`，校验官方 SHA-256；可按本机环境配置代理：

```sh
HTTPS_PROXY=http://127.0.0.1:7897 HTTP_PROXY=http://127.0.0.1:7897 \
  python3 scripts/bootstrap-go.py
```

该脚本仅下载 darwin/amd64；Apple Silicon 应安装官方 darwin/arm64 Go 1.27.3，并使用系统 PATH 中的 `go`。

## 验证与打包

```sh
make check       # 格式、go vet、MyGo UI 生命周期检查、race 测试、无 cgo 构建
make smoke       # 原生窗口、本地 Status 主线程更新、退出清理
make package     # MyGo CLI 生成两个架构的 .app，ad hoc 签名，不制作 DMG
```

产物位于 `bin/git-viewer` 和 `bin/package/darwin-{amd64,arm64}/Git Viewer POC.app`。本机已通过 Intel 包启动及两个包的 codesign 验证；Apple Silicon 真机启动尚未验证。签名、公证、安装器和发布仍属后续阶段。

SSH 集成测试自动启动仅监听 loopback 的临时 `sshd`，使用临时密钥、配置与 known_hosts，不修改用户 SSH 文件，也不需要公网服务器。若环境没有 sshd 或禁止启动，测试会显式跳过该集成用例；本机执行没有跳过。

## Phase 0 补充验证

已完成独立 Worktree 的 [原生窗口探针](docs/features/git-viewer/evidence/native-ui/README.md) 和 [SSH 运行中生命周期验证](docs/features/git-viewer/evidence/ssh-runtime/README.md)，由主 Agent 审查并集成。这些是 Phase 0 历史证据；Phase 1 另行记录。

```sh
.tools/go/bin/go run ./cmd/phase0-ui-probe \
  -baseline 53e686f6cfdaae9d162395f49d6de6007b2acc8f \
  -output .tools/native-ui-probe
```

此探针复用模拟数据，在真实原生窗口中驱动状态并生成截图和指标；物理输入、输入法和 GPU 计数器未验证。集成提交、测试与剩余项见 [集成记录](docs/features/git-viewer/evidence/parallel/integration.md)。

## 规划与证据

- [项目约束](AGENTS.md)
- [实施计划](docs/features/git-viewer/04-implementation-plan.md)
- [任务状态](docs/features/git-viewer/05-tasks.md)
- [Phase 0 执行与测试记录](docs/features/git-viewer/phase-0-execution.md)
- [Phase 1 执行与验收记录](docs/features/git-viewer/phase-1-execution.md)
- [性能与冒烟证据](docs/features/git-viewer/evidence/README.md)

本轮止于 Phase 1。双栏行内高亮、最低 macOS/Git 支持范围、Apple Silicon 真机交互和 GPU 性能仍需后续验证。
