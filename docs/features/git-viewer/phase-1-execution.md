# Phase 1 执行记录

用户明确授权“进入 Phase 1”。已完成 T101–T104，本轮止于本地只读；后续阶段仍需单独授权。原 Phase 0 的 Apple Silicon 真机、物理输入/IME、GPU 计数器及发布验证缺口保留，不视为本阶段已验证。

## 环境

本机 macOS x86_64，项目工具链 Go 1.27.3 darwin/amd64，MyGo 固定 `9c69cf829ece`，系统 Git `2.39.5 (Apple Git-154)`。本轮没有修改系统工具链、SSH 配置或引入新依赖；最低支持版本仍未以兼容性矩阵验收。

## T101：本地仓库打开与最近记录 — 完成

`git.LocalReader.Open` 以固定 argv 分别探测工作区根目录、git-dir/common-dir；子目录与软链接归一，worktree 单独身份，拒绝非仓库、bare、Git 内部目录。最近记录复用 Repository DTO 与原子 JSON 保存，最多 20 项；损坏记录报错且拒绝覆盖，持久化失败仍允许查看已验证仓库。

主入口改为 `internal/ui/local`，提供原生 MyGo 目录选择器、路径输入、最近仓库按钮及 Cmd+O。默认配置在 `os.UserConfigDir()/GitViewer/recent-repositories.json`，可通过 `-config` 隔离。

测试：`.tools/go/bin/go test ./internal/infrastructure/git ./internal/infrastructure/storage ./internal/application` 首轮通过；后续全量 race 通过。真实 Git 测试覆盖子目录、中文/空格/换行子目录、软链接、bare、gitfile/worktree 身份、最近记录去重/排序/20 项限制/损坏拒绝以及持久化 schema 校验。目录选择器调用由 ui.Tester 注入回调验证，系统目录面板的真实鼠标选择未单独验证。

## T102：真实 Status 与分区 UI — 完成

复用 porcelain v1 NUL 解析器。`domain.StatusEntries` 按 Staged/Unstaged/Untracked/Conflicted 分组，同一文件的 index/worktree 修改各有一行，冲突只进入冲突分区。虚拟列表支持点击与键盘选择，含重命名旧路径。控制器提供 loading、空仓库/clean、错误、stale、刷新与取消；后台结果经主线程调度，代次校验阻止旧结果覆盖。

测试：领域分区测试、真实仓库双区修改与未跟踪文件、初始空仓库、真实 merge 冲突及 ui.Tester 交互通过。无效仓库打开保留既有状态并标记 stale，刷新恢复；取消后晚到结果不会改写状态。原生窗口 Status 更新另有本机冒烟验证。

## T103：Diff 服务与查看器 — 完成

实现工作区/暂存区文件 Diff、完整提交 OID Diff；commit 对象通过 `^{commit}` 限定，初始提交可查看，merge 使用第一父提交语义。文件路径来自当前 Status，重命名包含旧路径，使用 literal pathspec 和 `--`，禁用外部 diff/textconv。未跟踪文件使用 `os.Root` 限定读取，symlink 仅展示链接文字。

统一视图按行虚拟化，支持横向滚动、中文/长行、单行文字选择与复制完整预览。Git 输出最多 512 KiB；未跟踪文件超过 512 KiB、展示超过 10,000 行时降级，二进制单独提示，超限不展示截断补丁。`DiffPreview` 和最近记录新增 schema 并有实际校验测试；DiffRequest 为内部非序列化执行类型。

测试：`.tools/go/bin/go test ./internal/ui/local ./internal/infrastructure/git ./internal/domain ./internal/infrastructure/storage` 通过；覆盖无 HEAD 暂存区、初始提交、双区差异、重命名、中文/空格/换行/引号/pathspec 元字符、二进制、文件/行数/命令输出限额、外部 diff 禁用、路径穿越/选项注入拒绝、symlink 不读取目标内容及取消。5,000 行原生虚拟 Diff 在 ui.Tester 中每帧构建少于 100 行，可定位第 4,900 行，复制与完整预览一致。

## T104：本地只读端到端与集成验收 — 完成

真实临时仓库 → 点击 Open path → Status 分区 → 点击文件 → 工作区/暂存区/未跟踪 Diff → 输入提交 OID → 提交 Diff → Refresh → 错误/stale → 取消 → 重读最近记录，已通过。流程前后比较 `status --porcelain=v1 -z` 和 `ls-files --stage -z`，确认工作区状态及索引条目未变。只有测试 fixture 创建仓库并执行写命令，应用入口仅调用只读方法。

最终检查已执行并通过：

```sh
make check
./bin/git-viewer -smoke -config "$PWD/.tools/phase1-smoke/recent.json"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 .tools/go/bin/go build \
  -o bin/git-viewer-phase1-arm64 ./cmd/git-viewer
.tools/go/bin/go mod verify
git diff --check
```

`make check` 包含 go fmt、go vet、MyGo 生命周期检查、全量 `go test -race -count=1 ./...`、无 cgo 本机构建；本机 Git/SSH/Phase 0 回归也通过。`file` 确认本机产物为 x86_64、交叉构建产物为 arm64 Mach-O。原生冒烟输出：

```text
native local repository window created
native local status main-thread update passed
background workers stopped
```

区分证据边界：ui.Tester 在内存软件渲染器中驱动真实服务与原生控件；原生窗口冒烟仅验证本机打开、Status 主线程更新和退出清理。尚未验证本阶段完整流程的物理鼠标/键盘/IME、arm64 真机运行或 Phase 1 应用包签名/公证；没有把交叉编译等同于实机验收。

## 手动复现

```sh
make build
./bin/git-viewer -repo /absolute/repository/path \
  -config "$PWD/.tools/manual-check/recent.json"
```

选择一个包含已暂存、未暂存和新文件的现有仓库，检查各分区并点击查看差异；同一文件双区修改应展示不同基准。输入 `git rev-parse HEAD` 的完整 OID 查看提交 Diff。外部修改文件后 Cmd+R；读取中点击 Cancel，检查取消/stale 提示及再次刷新恢复。所有 Git 写入均由用户在测试仓库外部完成。

当前限制：单栏统一 Diff；无历史选择器（提交 Diff 输入 OID）、自动监听、SSH 产品入口、Workspace 或 Git 写操作。最近列表进程内更新串行化，不提供多进程合并。Phase 2 及之后未开始。
