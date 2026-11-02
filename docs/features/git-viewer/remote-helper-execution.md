# 远端 Git Helper 执行记录

本轮按 [实施计划](remote-helper-plan.md) 完成 RH01–RH12。实现保持纯 Go/MyGo 原生 UI、系统 Git CLI、系统 OpenSSH、严格 host key 与固定 argv 白名单；未引入 WebView、HTML、JS 或远端任意 shell。协议细节见 [协议说明](remote-helper-protocol.md)。

## 任务结果

### RH01 协议编解码

`internal/remotehelper/protocol.go` 实现 4 字节大端长度 + JSON 帧、`[]byte` 字节字段、16 MiB 上限、严格解码与编码。解码拒绝未知/大小写替代字段、重复 key（含嵌套错误）、null、数字字节数组、非规范 base64、非法 UTF-8、尾随内容和截断帧。请求/响应新增 JSON Schema，契约测试同步 Go DTO 与 schema。

### RH02 共享参数、环境与白名单

新增 `internal/gitargs`，集中 `HistoryFormat`、`BasePrefix`、`CleanEnvironment`/`WriteEnvironment`、`IsWriteArgs` 和 OID/path/ref/remote/blob 校验。`IsWriteArgs` 覆盖 cherry-pick 与 revert；Reader 的既有薄包装委托该实现。helper 白名单按完整产品 argv 形状验证，覆盖读取、diff/conflict、全部 hunk apply、索引、commit/amend、branch、stash、cherry-pick/revert、worktree、fetch/pull/push。产品 argv 覆盖测试在真实临时仓库和本地 bare 上游执行。

### RH03 进程组执行引擎

新增 `process.RunInputGroup`：Unix `Setpgid`、取消时 TERM→KILL 整个进程组、有界 `WaitDelay`，父进程先退出仍升级清理；非 Unix 提供等价普通执行路径。`remotehelper.Engine` 固定 Git 前缀、读写环境分流、写串行、读并发 8、输入/输出/超时限制。测试覆盖 hook 及忽略 TERM 的子孙、继承/重定向输出、读写环境差异、缺失 Git、超时与输出上限。

### RH04 Serve 与 helper 二进制

`Serve` 要求 hello 第一且一次，按 id 管理在途请求，支持 exec/batch/cancel/ping/shutdown；EOF、信号、坏帧、重复 id、超过 64 在途均安全终止并杀组。Unix 继承 pipe/socket 经 `dup` + 非阻塞 + Go poller 包装，修复真实子进程收到 SIGTERM 时因 stdin 未关闭或 stdout 阻塞而挂住的问题。`cmd/gitviewer-helper` 提供 `--serve`/`--version`，拒绝未知参数及二者并用。

### RH05 Helper 客户端

`HelperClient` 在现有 ControlMaster 通道上发送帧，维护 pending、写/发送/心跳信号量、30 秒 ping、3 秒取消确认、5 秒关闭边界。读取循环拒绝未知/重复 id、类型不匹配、错误 batch 数量和未知错误码；死亡向所有在途请求发布同一失败。半帧或取消无法确认时废弃连接，绝不重发。普通 Git 非零退出现在与 CLI 一样依据 stderr 分类，保留 `PermissionDenied` 等映射。

### RH06 远端安装

SFTP `RealPath(".")` 解析 HOME，固定 `uname` 探针映射 Linux/Darwin × amd64/arm64。本地二进制从 `mygo.PathResources/helper/<goos>-<goarch>/gitviewer-helper` 等候选解析；buildID 为实际文件 SHA-256 前 12 位。远端路径为 `<home>/.local/share/gitviewer/helper/v1/<buildID>/gitviewer-helper-<goos>-<goarch>`。

逐级 `Lstat` 拒绝符号链接/非目录，目录 0700；临时文件独占创建、检测短写、chmod 0700、原子重命名。复用前重新哈希实际远端文件，不信任 sidecar；上传后也校验实际字节。SFTP 每操作 10 秒取消守卫，认证/host key 与部署失败区分。

### RH07 连接池、单飞、退避与回退

每 host 一个 entry，单飞启动，最多等待 3 秒，后台继续安装；临时失败 500ms→30s 退避，永久失败（平台、SFTP、权限、noexec、命令不可执行）停止重试。死亡 watcher 关闭并回收 client，即使 entry 已被替换；心跳失败触发重建。并发/重复 Close 幂等并等待 worker。

旧版本目录 7 天、临时文件 1 小时后清理。清理在握手成功后交给池跟踪的后台 worker，独占 SFTP 会话；不阻塞 helper 就绪，池关闭时取消并 join 该 worker。实现是“每次成功启动后的一次后台保守清扫”，不是独立周期定时器。

### RH08 Reader/Open/Batch 接入

`Executor.Helpers` 存在时，Reader、Open 探针与分组读取优先 helper；否则或 `-remote-helper=off` 使用 CLI。启动前部署不可用回退 CLI；认证、host key、非法输入、取消、协议错误不回退。请求发出后仅读取的 `RemoteUnavailable` 回退一次；写入从不回退或重试。helper 拒绝不安全 argv 时 fail closed，不启动 CLI。

受控 Write 在远端取消、超时、断线、输出超限或协议错误后报告 `OperationStateUnknown` 并独立读取验证。受控 worktree 创建/移除采用同样语义。原有 `TestOpenSSHIntegration` 现以 `cli` 与 `helper` 两种独立 fixture 运行。

### RH09 应用生命周期

应用入口注入 `HelperResourcesDir = mygo.App.Path(mygo.PathResources)`，启动参数新增 `-remote-helper=auto|off`。打开远端目标时非阻塞预热，纯本地路径不触碰池。退出先关闭 helper、等待 worker，再关闭 Connections。资源候选包含开发态回退；未找到本地 helper 时部署失败并回退 CLI。

### RH10 打包

`make helper` 交叉构建四个平台到 `resources/helper/<goos>-<goarch>/`，`build`/`package` 依赖它，另有 `helper-clean`。生成目录已加入 `.gitignore`。README 记录 auto/off、部署、回退、资源与构建方式。

### RH11 Loopback SSH 集成

独立临时 sshd（loopback、新主机/用户密钥、私有 known_hosts/config、`internal-sftp`）验证：

- 自动上传、启动、Open、status、stage、commit、history、CommitDetails batch、diff；
- 含 Unicode、换行、引号、shell 元字符的仓库/文件路径；
- Linux 非 UTF-8 文件名；APFS 拒绝非法 UTF-8 时改用有效 Unicode + 换行；
- 取消 pre-commit hook 的 commit → `OperationStateUnknown`，helper 之后仍可读；
- 读取中 helper 死亡 → 真实 OpenSSH CLI 回退一次；
- 写入中 helper 收到 SIGTERM → 未知结果、hook 仅执行一次、不重试；
- 池关闭后远端 helper PID 消失；
- 真实 SFTP 篡改等长二进制且保留原 sidecar → 再次 Ensure 重新上传实际匹配字节并启动；
- SFTP 关闭、不支持平台 → CLI 回退。

读死亡子用例使用本地 re-exec 帧 helper + 假 Git 制造在途慢读，CLI 回退仍经真实 OpenSSH 和系统 Git；不是远端 helper 进程被杀。

### RH12 性能、文档与全量验证

可选用例 `GITVIEWER_HELPER_LATENCY=1` 在 loopback 各取 15 次，不设阈值。最近一次结果（并行 batch 修复后）：

| 模式 | 操作 | p50 | p95 |
|---|---|---:|---:|
| helper | status | 26.303 ms | 30.481 ms |
| helper | ListRefs batch | 30.335 ms | 38.316 ms |
| helper | 完整 Write commit | 97.146 ms | 116.891 ms |
| CLI | status | 45.691 ms | 49.401 ms |
| CLI | ListRefs batch | 48.096 ms | 50.428 ms |
| CLI | 完整 Write commit | 206.327 ms | 254.393 ms |

commit 包含写前 status、commit、写后 status。loopback 只证明进程/握手开销结构，不能代替真实局域网。

## 验证记录

已完成并通过：

```sh
.tools/go/bin/go test -race ./internal/remotehelper ./internal/gitargs ./internal/infrastructure/process ./cmd/gitviewer-helper ./internal/infrastructure/git -count=1 -timeout=120s
.tools/go/bin/go test -race ./internal/infrastructure/ssh -run 'TestHelperClient|TestHelperOpenSSHIntegration' -count=1 -timeout=100s
.tools/go/bin/go test -race ./internal/infrastructure/ssh -run 'TestHelperPool|TestHelperInstall|TestOpenSSHIntegration' -count=1 -timeout=120s
GITVIEWER_HELPER_LATENCY=1 .tools/go/bin/go test ./internal/infrastructure/ssh -run '^TestHelperLatency' -count=1 -v
.tools/go/bin/go vet ./...
.tools/go/bin/go tool mygo vet
make build
make package
```

`make package` 生成 `bin/package/darwin-amd64/Git Viewer.app` 与 `bin/package/darwin-arm64/Git Viewer.app`，MyGo 编译与签名步骤通过；`codesign --verify --deep --strict` 对两个包均通过，两个包的 `Contents/Resources/helper/` 都包含 Linux/Darwin × amd64/arm64 四个 helper（Linux 为静态 ELF）。使用独立临时 Git 仓库与临时配置运行 `./bin/git-viewer -smoke -repo <fixture> -config <temp>/recent.json -auto-refresh=false`，输出 `native data main-thread update passed` 与 `background workers stopped`。

全量 race 首轮暴露并已修复：helper 子进程 SIGTERM 压力测试、Reader 覆盖测试残留文件、引擎测试过短超时、batch 预算旧断言、终端加载测试读取包级函数的数据竞争。修复后 `.tools/go/bin/go test -race -count=1 ./... -timeout=180s` 全部通过（`internal/infrastructure/ssh` 约 51 秒、`internal/ui/local` 约 67 秒），无 `DATA RACE` 或失败包。

## 未验证与限制

- 未连接真实 `n1` 或其他用户远端；真实局域网 10–20 ms / commit 60–100 ms 目标未实测。
- Linux/arm64、Darwin/arm64、musl/Alpine、老 glibc 仅交叉构建，未实机运行。
- 未做 GUI 物理点击、远端登录窗口或真实凭据交互；只完成原生 smoke 与自动化集成。
- 只读 HOME、noexec、NFS root squash、`ForceCommand`、低 `MaxSessions` 由分类/单测覆盖，未在真实异常远端挂载验证。
- SIGKILL 无法执行进程组清理；macOS 无 `Pdeathsig`，可能遗留 Git 子进程。该情形下客户端仍不重试写入。
- SFTP path API 不能消除同一用户并发重命名的恶意竞争；v1 假设单用户、私有 0700 目录。
- 公证、Linux 桌面打包与 ELF 资源进入公证流程未验证。
- 真实 LAN 上的 helper 版本升级、旧目录清理时机与长期心跳稳定性未做长时运行。
