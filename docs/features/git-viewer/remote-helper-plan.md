# 远端 Git Helper 实施计划（性能优先）

> 状态：RH01–RH12 已实施；验证边界见[执行记录](remote-helper-execution.md)与[协议说明](remote-helper-protocol.md)。本文档保存 2026-11-02 的规划结论。
> 用户已确认：第一版覆盖读 + 写；部署方式为首次连接自动上传，失败自动回退现有 CLI。

## Context

远端 SSH 仓库的提交慢，根因不是 Go 算法，而是每条 Git 命令都启动一个新的本地 `ssh` 客户端进程，写入（commit）还使用隔离连接、每次重新认证。局域网实测：

| 场景 | 耗时 |
|---|---:|
| 共享 ControlMaster 执行 `git status` | ~72 ms |
| 每次新建连接执行 `git status` | ~527 ms |
| 远端一次 commit（写前 status + commit + 写后 status） | ~670 ms |

目标：每台 SSH 主机一个常驻 helper 进程，复用现有 ControlMaster 的一条通道；所有 Git 操作走帧协议。预期单条命令降到 ~10–20 ms，commit 降到 ~60–100 ms。

该方案取代之前讨论的“专用写 master + 写前索引检查”，因为 helper 同时消除了每命令 SSH 进程和写入握手。

## 目标与非目标

目标：
- 远端仓库的 status/history/refs/diff/commit/stage/push 等全部走 helper。
- 打开远端仓库的 `rev-parse` 探针也走 helper。
- helper 不可用时无感回退 CLI；写入失败不回退、不重试，保持 `OperationStateUnknown` 语义。
- 保持现有安全约束：固定 argv 白名单、系统 Git、系统 OpenSSH、严格 host key、不保存凭据。

非目标（v1）：
- 远端文件监听/推送（继续轮询）。
- 远端终端、SFTP、LSP 改造（继续共用 master）。
- 多用户/多租户 helper 协议；helper 只服务当前 SSH 用户。

## 架构

### 接缝

`git.LocalReader.Transport` / `Batch` 是唯一需要替换的接缝（`internal/infrastructure/git/reader.go:22,25`）。客户端保留全部参数校验、解析和 DTO；helper 只做“固定 argv 的远端执行 + 帧复用”。

`ssh.Executor.Open` 在接缝之外，单独改为走同一 helper；`WorktreeCheckNewPath`（`realpath`/`test`）与旧 POC `Execute` 保留 CLI。

### 组件

```
本机 Git Viewer
  └─ ssh.Executor
       ├─ Connections（现有共享 ControlMaster）
       └─ HelperPool（新增，每主机一个常驻进程）
            └─ ssh -S <master> host ~/.local/share/gitviewer/helper/v1/<buildID>/gitviewer-helper --serve
                 └─ 帧协议（stdin/stdout）→ 固定 argv 执行系统 git
```

新增：
- `internal/gitargs/`：共享的 `HistoryFormat`、`IsWriteArgs`、`BasePrefix`、环境策略、OID/path/ref/remote 校验器。
- `internal/remotehelper/`：帧编解码、请求/响应类型、白名单 `Validate`、执行引擎、`Serve` 循环（可在进程内测试）。
- `cmd/gitviewer-helper/`：薄 main，`--version` / `--serve`，信号处理。
- `internal/infrastructure/ssh/helper_client.go`、`helper_pool.go`、`helper_install.go`、`helper_remote_ops.go`。
- `resources/helper/{linux,darwin}-{amd64,arm64}/gitviewer-helper`（构建产物，gitignore）。

### 协议

4 字节大端长度 + JSON 帧，`MaxFrame = 16 MiB`。所有字节字段用 `[]byte`/`[][]byte`（JSON 自动 base64），避免非 UTF-8 路径被替换。

请求：`hello`、`exec{id,dir,limit,timeoutMs,args,input}`、`batch{id,dir,limit,commands}`、`cancel{id}`、`ping`、`shutdown`。
响应：`ready{proto,platform,pid}`、`result{id,exit,stdout,stderr,error}`、`batchResult`、`pong`、`fatal`。

语义：
- `hello` 必须第一；`proto` 不匹配则 `fatal` 退出。
- 每个 `exec`/`batch` 恰好一个终态响应（含取消），除非连接断开。
- `cancel` 按 id 取消 ctx、杀掉 Git 进程组、回 `OperationCancelled`。
- 客户端 stdin EOF → helper 取消全部、杀组、退出；helper stdout EOF → 客户端把所有在途请求置 `RemoteUnavailable`。
- `ping` 用于 30s 空闲心跳；`shutdown` 优雅退出。
- 错误码：`UnsafeOperation`、`InvalidRepository`、`OperationCancelled`、`OperationTimedOut`、`OutputLimitExceeded`、`GitUnavailable`、`CommandFailed`、`ProtocolError`；`RemoteUnavailable`/`OperationStateUnknown` 只在客户端。
- 限制：`limit` 默认 2 MiB、最大 8 MiB；`input` ≤ 2 MiB；每个 arg ≤ 16 KiB；总 args ≤ 1 MiB；batch 1–4 条只读命令，聚合 ≤ 8 MiB；`dir` ≤ 4096 字节。

### 执行语义

helper 自己构造与 `ssh.runGit` 相同的固定前缀：

```
git --no-pager --no-optional-locks --literal-pathspecs
    -c core.fsmonitor=false -c log.showSignature=false -c color.ui=false
    -C <dir> <args...>
```

- 读：`CleanEnvironment`（`GIT_CONFIG_NOSYSTEM=1`、`GIT_CONFIG_GLOBAL=/dev/null`）。
- 写：`WriteEnvironment`（恢复用户配置，identity/hooks 生效）；由 `IsWriteArgs(args)` 判定，不由客户端声明。
- 输出上限、超时复用 `process.RunInput`；新增 `RunInputGroup`（`Setpgid`，SIGTERM→SIGKILL 进程组），保证取消 commit 时 hook/子进程一起结束。
- 写请求在 helper 内按 host 串行（容量 1 的信号量）；读并发最多 8。
- 白名单：helper 用共享校验器逐条验证 argv，拒绝任意子命令、全局选项、NUL、相对 `dir`、超长参数；不匹配返回 `UnsafeOperation`，客户端 fail closed，不回退 CLI。
- 白名单覆盖 Reader 全部 argv 形状（status/history/for-each-ref/show/log/diff/cat-file/ls-tree/blame/ls-files/config --get/check-ref-format/worktree/stash 及全部写入形状），并加“Reader 覆盖测试”和 fuzz。

### 部署与版本

1. 通过共享 master 的 SFTP `RealPath(".")` 解析远端 HOME（不经过 shell）。
2. 固定探针 `uname -s` / `uname -m`，映射 `linux|darwin` × `amd64|arm64`；其他平台永久回退 CLI。
3. 本地二进制从 `mygo.PathResources/helper/<goos>-<goarch>/gitviewer-helper` 等候选路径解析。
4. 远端路径：`<home>/.local/share/gitviewer/helper/v1/<buildID>/gitviewer-helper-<goos>-<goarch>`，`buildID` = 本地二进制 SHA-256 前 12 位；同目录写 `.sha256` sidecar。
5. 通过 SFTP 检查/上传：逐级 `Lstat` 拒绝符号链接，写临时文件 → `Chmod 0700` → `PosixRename`；sidecar 同样原子写。校验大小/哈希。
6. 用 `Executor.Command(context.Background(), host, SharedSession, []string{remotePath, "--serve"}, "")` 启动，`hello` 握手 10s 超时；失败分类 SSH 错误。
7. 失败策略：网络/主连接问题指数退避 500ms→30s；平台不支持/SFTP 不可用/权限/只读/noexec 永久回退；SSH 认证/host key 直接返回。
8. 旧版本目录 7 天后异步清理，临时文件 1 小时后清理；绝不删当前 buildID。
9. 启动等待最多 3s；超时先回退 CLI，上传在后台继续。

### 回退与安全

- `Executor.Helpers == nil` 或 `-remote-helper=off` → 全部走现有 CLI。
- helper 启动失败 → 读/写都回退 CLI（请求尚未发出，安全）。
- 请求已发出后失败：
  - 读：可回退 CLI 一次。
  - 写：不回退、不重试，返回 `RemoteUnavailable`/原错误，由 `Write` 置 `OperationStateUnknown`。
- helper 返回 `UnsafeOperation`/`InvalidRepository` → 直接返回，不回退（fail closed）。
- 保留：`ValidHost`、`Encode`、`StrictHostKeyChecking=yes`、BatchMode/askpass、2 MiB 输出预算、严格 host key、无凭据存储。
- helper 只接受固定子命令集合，不接受 shell、任意 argv、`-c` 全局选项或远端路径参数注入。

## 代码改动与任务

每个任务结束：运行对应测试，更新 `docs/features/git-viewer/05-tasks.md` 与执行记录，本地提交（不推送）。

### RH01 协议编解码
- 文件：`internal/remotehelper/protocol.go`、`protocol_test.go`
- 内容：常量、`[]byte` 类型、`ReadFrame`/`WriteFrame`、严格 JSON。
- 测试：往返、0/超大/截断帧、部分写、非法 JSON/base64、大 payload。
- 验收：`go test ./internal/remotehelper`。

### RH02 共享 argv/环境/校验器与白名单
- 文件：`internal/gitargs/args.go`、`args_test.go`；`internal/remotehelper/whitelist.go`、`whitelist_test.go`；现有 `internal/infrastructure/git/*` 保留薄包装避免大改。
- 内容：`HistoryFormat`、`IsWriteArgs`、`BasePrefix`、`CleanEnvironment`/`WriteEnvironment`、OID/path/ref/remote 校验、完整 `Validate`。
- 测试：接受/拒绝表、fuzz、`IsWriteArgs` 与白名单写分类一致、Reader 全部 argv 形状通过校验。
- 验收：`go test ./internal/gitargs ./internal/remotehelper ./internal/infrastructure/git ./internal/infrastructure/ssh`。

### RH03 进程组执行引擎
- 文件：`internal/infrastructure/process/run_group*.go`、`run_group_test.go`；`internal/remotehelper/exec.go`、`exec_test.go`
- 内容：`RunInputGroup`（Setpgid、组 SIGTERM→SIGKILL、WaitDelay）、helper 执行引擎（环境、写串行、输入、上限）。
- 测试：临时仓库 status/log/commit/apply；读写环境差异；输出上限；超时；取消杀掉 hook 及其孙进程；git 缺失/非零退出。
- 验收：`go test -race ./internal/infrastructure/process ./internal/remotehelper`。

### RH04 Serve 循环与 helper 二进制
- 文件：`internal/remotehelper/serve.go`、`serve_test.go`；`cmd/gitviewer-helper/main.go`、`main_test.go`
- 内容：读循环、在途表、cancel/batch/ping/shutdown、信号、`--version`/`--serve`。
- 测试：进程内管道、并发乱序、重复/未知 id、EOF 杀子进程、坏帧退出、未知 flag。
- 验收：`go build ./cmd/gitviewer-helper`；`go test ./internal/remotehelper ./cmd/gitviewer-helper`。

### RH05 Helper 客户端
- 文件：`internal/infrastructure/ssh/helper_client.go`、`helper_client_test.go`
- 内容：管道帧、在途表、cancel、写信号量、心跳、Close、死亡传播。
- 测试：并发乱序、取消确认、EOF、协议错误、写串行、Close 幂等。
- 验收：`go test -race ./internal/infrastructure/ssh -run TestHelperClient`。

### RH06 远端安装/上传/平台/HOME
- 文件：`internal/infrastructure/ssh/helper_install.go`、`helper_remote_ops.go`、`helper_install_test.go`
- 内容：SFTP `RealPath(".")`、`uname` 映射、本地二进制解析、buildID 路径、sidecar 哈希、原子上传、符号链接拒绝、清理。
- 测试：各分支 fake ops；loopback SFTP 上传；权限/noexec/不支持/SFTP 关闭；原子改名；旧目录清理。
- 验收：`go test -race ./internal/infrastructure/ssh -run TestHelperInstall`。

### RH07 连接池/单飞/退避/回退
- 文件：`internal/infrastructure/ssh/helper_pool.go`、`helper_pool_test.go`
- 内容：entry 状态机、Acquire/Ensure/Close、单飞、重连 watcher、永久/临时失败分类。
- 测试：N 个并发 Acquire 只启动一次；启动等待；退避；永久回退；死亡重连；Close；值拷贝共享；写串行。
- 验收：`go test -race ./internal/infrastructure/ssh -run TestHelperPool`。

### RH08 接入 Reader/Open/Batch 与回退
- 文件：`internal/infrastructure/ssh/executor.go`、`reader.go`、`batch.go`、`reader_transport_test.go`、`helper_fallback_test.go`；`internal/infrastructure/git/` 覆盖测试。
- 内容：`Helpers *HelperPool` 字段；`runGitPreferred`/`runReadBatchPreferred`；`Open` 改用同一前缀与 helper；回退规则；写不重试。
- 测试：helper 可用时不启动 CLI 进程；不可用回退；读死亡回退一次；写死亡不回退且 `Write` 置未知；Open 走 helper；Reader 全部 argv 通过白名单。
- 验收：`go test -race ./internal/infrastructure/ssh ./internal/infrastructure/git`；`TestOpenSSHIntegration` 在 helper nil/启用两种模式下通过。

### RH09 生命周期接线
- 文件：`cmd/git-viewer/main.go`、`manager.go`、`launch.go`、`remote_install_test.go`
- 内容：创建/赋值/关闭 HelperPool，`-remote-helper=auto|off`，打开远端时异步预热，关闭顺序（先 helper 后 Connections），本地二进制解析。
- 测试：退出时关闭池；预热非阻塞；纯本地模式不触碰池；候选路径解析。
- 验收：`go test ./cmd/git-viewer`；`make build`；`-smoke`。

### RH10 打包
- 文件：`Makefile`、`.gitignore`、`README.md`；`mygo.json` 不改（顶层 `resources/` 自动打包）。
- 内容：`make helper` 交叉编译四个平台到 `resources/helper/<goos>-<goarch>/`；`build`/`package` 依赖它；`helper-clean`。
- 验收：四个二进制存在且架构正确；`make package` 后 `Contents/Resources/helper` 完整；`codesign --verify --deep --strict`；`bin/git-viewer -smoke`；开发态 `go run` 能解析或回退。

### RH11 Loopback sshd 集成
- 文件：`internal/infrastructure/ssh/helper_integration_test.go`（新 fixture，启用 `Subsystem sftp internal-sftp`）
- 测试：上传/启动/status/history/diff/commit；取消 hook commit → 未知结果且 helper 仍可读；读期间杀 helper → CLI 回退；写期间杀 helper → 不重试；哈希不匹配重传；SFTP 关闭/不支持平台回退；无遗留 helper/git 进程。
- 验收：`go test -race -count=1 ./internal/infrastructure/ssh -run TestHelperOpenSSHIntegration`。

### RH12 性能测量、文档与全量验证
- 文件：`internal/infrastructure/ssh/helper_latency_test.go`（可选运行，只记录不设阈值）；`docs/features/git-viewer/remote-helper-execution.md`、`remote-helper-protocol.md`、`05-tasks.md`、`03-technical-design.md`、`ssh-performance-execution.md`。
- 内容：loopback 下 status/batch/commit 的 helper vs CLI p50/p95；记录实测、交叉构建、未验证项。
- 验收：`go test -race -count=1 ./...`、`go vet ./...`、`go tool mygo vet`、`make build`、`make package` 全部通过。

## 验证命令

```sh
make helper
.tools/go/bin/go test ./internal/remotehelper ./internal/gitargs -count=1
.tools/go/bin/go test -race ./internal/infrastructure/process ./internal/infrastructure/ssh ./internal/infrastructure/git -count=1
.tools/go/bin/go test ./... -count=1 -timeout=120s
.tools/go/bin/go vet ./...
.tools/go/bin/go tool mygo vet
make build
make package
./bin/git-viewer -smoke
```

## 风险与未验证项

- 真实 n1 局域网的 10–20 ms / 60–100 ms 目标需在实现后实测；loopback 只能证明开销结构。
- arm64 远端、macOS 远端、musl/Alpine、老 glibc 仅交叉构建，未实机运行。
- GUI 物理交互未验证；只做 smoke 与单元/集成测试。
- 只读 HOME、noexec、NFS root squash、无 SFTP、`ForceCommand`、低 `MaxSessions` 会回退 CLI。
- helper 被 SIGKILL 时 macOS 无 `Pdeathsig`，极端情况下可能遗留 Git 子进程；显式取消用进程组杀，写入仍报未知。
- 远端 GPG/凭据/editor 交互无 tty，可能直到写超时；与现有 CLI 行为一致。
- ELF 资源进入 macOS 公证未验证；备选方案是把 Linux helper 以 `.gz` 打包、上传前解压。
