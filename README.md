# Git Viewer — MyGo Native Phase 0

纯 Go + `egoist/mygo/ui` 原生技术原型。当前完成工程、Git/SSH 执行与解析 POC，以及模拟数据 UI 验证；正式仓库管理、Status/Diff/History/Graph 业务和 Workspace 功能按后续阶段实施。

固定版本：Go **1.27.3**，MyGo **v0.0.0-20261025001514-9c69cf829ece**。应用不使用 WebView、HTML、JavaScript、TypeScript 或 Bun。

## 本地运行

需要 macOS、Git、OpenSSH 和 Go 1.27.3。当前机器的 Go 已安装到 `.tools/go`，Makefile 会优先使用它。

```sh
make run
```

界面提供模拟仓库三栏、10,000 条虚拟提交、Graph/Diff 切换、静态 Diff 复制、后台 Git 版本探测、任务取消和断线模拟。`Cmd+R` 或菜单 `Prototype → Health / Version` 执行只读 Git 健康检查。

在另一台 Intel Mac 上，也可以使用项目的固定版本安装脚本。它只写入 `.tools/`，校验官方 SHA-256；可按本机环境配置代理：

```sh
HTTPS_PROXY=http://127.0.0.1:7897 HTTP_PROXY=http://127.0.0.1:7897 \
  python3 scripts/bootstrap-go.py
```

该脚本仅下载 darwin/amd64；Apple Silicon 应安装官方 darwin/arm64 Go 1.27.3，并使用系统 PATH 中的 `go`。

## 验证与打包

```sh
make check       # 格式、go vet、MyGo UI 生命周期检查、race 测试、无 cgo 构建
make smoke       # 原生窗口、后台任务主线程更新、退出清理
make package     # MyGo CLI 生成两个架构的 .app，ad hoc 签名，不制作 DMG
```

产物位于 `bin/git-viewer` 和 `bin/package/darwin-{amd64,arm64}/Git Viewer POC.app`。本机已通过 Intel 包启动及两个包的 codesign 验证；Apple Silicon 真机启动尚未验证。签名、公证、安装器和发布仍属后续阶段。

SSH 集成测试自动启动仅监听 loopback 的临时 `sshd`，使用临时密钥、配置与 known_hosts，不修改用户 SSH 文件，也不需要公网服务器。若环境没有 sshd 或禁止启动，测试会显式跳过该集成用例；本机执行没有跳过。

## Phase 0 补充验证

已完成独立 Worktree 的 [原生窗口探针](docs/features/git-viewer/evidence/native-ui/README.md) 和 [SSH 运行中生命周期验证](docs/features/git-viewer/evidence/ssh-runtime/README.md)，由主 Agent 审查并集成；未进入 Phase 1。

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
- [性能与冒烟证据](docs/features/git-viewer/evidence/README.md)

本轮止于 Phase 0。最低 macOS/Git 支持范围、Apple Silicon 真机交互和 GPU 性能仍需补充实测。
