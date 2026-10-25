# Phase 0 执行记录

状态：T001–T005 完成，原生专项 POC 已验证。本轮止于 Phase 0，未实现正式仓库管理、Workspace、History/Graph 业务或 Git 写操作。原始核对见 [phase-0-review.md](phase-0-review.md)，最新任务状态见 [05-tasks.md](05-tasks.md)。

## 补齐与决策

用户授权补齐约束并推进后，建立 AGENTS.md；修正 03/05 的 TypeScript 遗留和补充任务阶段映射，保留原 Implementation Plan 与分层方案。

- Go 1.27.3 安装在 `.tools/go`，官方 archive SHA-256 为 `b6f8e931e4ab497fcde6da8ae4499a58f9d258e55c8a80824a516eae5950a750`，下载后已校验；未修改系统 Go 或 shell 配置。
- MyGo 固定为 `v0.0.0-20261025001514-9c69cf829ece`，提交 `9c69cf829ece196c5b3a3601f1c7bbe70526376f`；CLI 通过 go.mod 的 tool 指令使用同一版本。
- Repository JSON DTO 遵循既有 schema；内部 RepositoryContext 单独包含 SSH Host/git-dir/common-dir；GitError 为内部类型。本阶段不扩展持久化契约。
- 执行 POC 仅开放固定只读命令，拒绝写操作及未实现的可选参数。无 HEAD 的历史命令失败保留为明确错误，后续业务层再处理空仓库显示。
- 建立 Git 分支 `phase-0`，未提交或推送。MyGo CLI 在首次无提交仓库中会打印 VCS 元数据警告，构建与签名均成功。
- MY007 的 JSONC 及 NU007 正式 Workspace 功能留 W；跨架构真机和正式发布验收不冒充本机已完成。

## 逐 Task 测试与验收

| Task | 交付 | 测试及结果 |
|---|---|---|
| T001 | 官方 native 模板生命周期、原生入口、Makefile、macOS CI、固定版本 | 首次 `make check` 通过；Intel 原生窗口 smoke 退出码 0；arm64 交叉构建通过。后续 MyGo 双架构 .app 打包/ad hoc 签名校验也通过，Intel .app 启动通过 |
| T002 | domain 操作枚举、命令/Repository/Status DTO、内部 Context、GitError、解析记录 | `go test -race ./internal/domain`、vet 通过。使用 jsonschema/v6 v6.0.2 实际校验 draft 2020-12；测试字段集合、有效实例、必填缺失、操作枚举和 limit 越界。既有 Git schemas 无字段变更 |
| T003 | LocalExecutor、共享 process.Run、只读白名单、独立输出/退出码、合计输出预算、超时/取消 | `go test -race -count=1 ./internal/infrastructure/git ./internal/infrastructure/process`、vet 通过。真实仓库特殊文件名、非仓库错误、Git 缺失、输出限额；测试进程验证退出码 7、NUL 字节、stderr、超时、主动取消 |
| T004 | 系统 OpenSSH POC、Config 别名、严格 host key、POSIX 独立参数编码、错误映射 | `go test -race -v -count=1 ./internal/infrastructure/ssh`、vet 通过，无 skip。真实隔离 loopback sshd 验证 SSH Config/远程 Git、未知/变更 host key 拒绝、认证失败、取消、超时、断线；真实 shell round-trip 验证恶意/特殊参数不注入 |
| T005 | NUL 字节 Status/History/Worktree 解析器 | `go test -race -count=1 ./internal/infrastructure/git`、vet 通过。特殊文件名、重命名双路径、MM、删除、untracked、真实冲突索引、空状态/无 HEAD、merge parents、detached/locked worktree、bare 仓库、异常协议；5 秒双 worker fuzz 约 273,091 次输入通过 |

开发中测试发现的库 API 名称、缺失可执行文件错误映射、macOS canonical path 比较和测试日志复制锁问题均已修正，最终检查无失败。

本地执行使用 `exec.CommandContext`、明确 argv，禁用 pager/提示、fsmonitor、log 签名探测及外部 diff/textconv；清理继承的 Git 环境覆盖。超时默认 10 秒、输出默认合计 2 MiB，WaitDelay 限制退出后管道等待。错误消息不包含 argv/凭据，stdout/stderr 保留在结果中供受控展示。

SSH 使用临时测试密钥、known_hosts 和配置，只监听 loopback，结束时关闭 sshd；不读写用户密钥和配置。取消/超时集成用已取消 context/极短 deadline 验证入口，进行中的子进程取消由 T003 等待子进程覆盖。真实公网 SSH、交互密码/2FA 不在本次验收中。

## 原生专项 POC

- MY003/MY004、NU006：Go Health/Version 服务执行真实 Git 版本探测；`Window.Update` 主线程更新，UI 展示 loading/online/offline/stale/cancelled。generation 防止旧结果覆盖；窗口关闭取消，App.Run 返回后等待 worker。错误/断线 UI 由受控模拟与任务测试验证，SSH 后端失败另有真实集成覆盖。
- NU002：嵌套 `ui.Split` 三栏、工具栏、原生菜单、Cmd+R、模拟仓库切换。官方 UI 测试器验证拖拽、快捷键、切换与状态。
- NU003：10,000 模拟提交，选择、定位到第 9999 行、滚动，每 pass 约 29 行；headless 跳转/重建约 1.60 ms/op。
- NU004：200 模拟节点、边、标签，首尾命中、双向滚动；独立 900×450 viewport 可显示全部节点，测试帧约 0.071 ms/op。无正式 Git DAG 算法。
- NU005：静态单栏 Diff，中文/长行、横向滚动、可选中复制、二进制占位；不包含正式 Git Diff 服务。
- MY007 的 Phase 0 部分：`mygo.WatchFiles` 捕获原子 JSON 替换，0600 权限、序列化失败保留原文件、临时文件清理、符号链接/目录/相对路径拒绝与 watcher 资源释放测试通过。JSONC 留 W。

原始性能、机器配置、绘图截图及复现命令见 [evidence/README.md](evidence/README.md)。这些性能为官方 headless 测试器基线，不能解释为真实 GPU FPS。

## 最终检查与产物

- `make check`：格式、`go vet`、`go tool mygo vet`、全部 `go test -race -count=1 ./...`、`CGO_ENABLED=0` 构建通过。
- `bin/git-viewer -smoke`：原生窗口创建、真实后台探测经 UI 主线程更新、关闭清理通过，失败分支返回非零。
- `make package`：MyGo CLI 生成 Intel/arm64 `.app`，ad hoc 签名；两个包的 `codesign --verify --deep --strict` 通过；Intel 包执行 `-smoke` 通过。
- 应用入口仅 `WindowOptions.Content: ui.View`；无 URL/WebView 调用或应用 JS/TS/HTML。MyGo 官方模块自身包含其其他模式源码，应用只走原生路径。
- CI 配置已建立，尚未在远端触发。

## Phase 0 用户确认后的 A/B 验证收尾

已在独立分支/Worktree 完成 A 原生窗口探针、B 运行中 SSH 取消/超时/断线补充验证，并由主 Agent 集成。共享业务接口和依赖未变，文件白名单无重叠。完整结果与提交链见 [补充验证集成记录](evidence/parallel/integration.md)。原生探针另以 race 模式执行通过；SSH 先确认远端 helper 已执行再触发，三轮九场景通过，并显式检查 helper 回收。集成后完整 make check、报告 schema 契约、依赖、native smoke、双架构 .app/签名和 Intel 包 smoke 通过。

当前工具没有独立子 Agent 调用接口，由主 Agent 在两个独立 Worktree 中执行 A/B 工作线并独立提交；没有虚构多 Agent 执行。Apple Silicon 仍缺机器。真实窗口结果是程序驱动状态，不等于物理输入或 GPU 计数器验收；原始未验证项继续保留。

## 兼容性与未解决项

| 环境 | 构建/打包 | 启动/测试 |
|---|---|---|
| 本机 Darwin 24.6.0 x86_64，32 GiB，系统报告 Ryzen 7 2700X / 16 逻辑 CPU | Intel 无 cgo 二进制与 .app 通过 | 全部 race 测试、原生 smoke、Intel 包 smoke 通过 |
| macOS arm64 | 交叉构建、.app、签名校验通过 | 缺 Apple Silicon 真机，未验证 |
| Git 2.39.5 / OpenSSH 9.9p2 | 本机已验证 | 更早 Git 的 worktree -z 兼容/降级待确认 |

仍待验证：最低 macOS/Git 支持范围、Apple Silicon 真机交互、实际 GPU 性能与长时间交互、真实网络/认证环境。Developer ID、公证、DMG 与正式发布属于后续发布阶段。以上不影响已完成 POC 的本机复现，也不代表允许进入 Phase 1；本轮已在 Phase 0 停止。
