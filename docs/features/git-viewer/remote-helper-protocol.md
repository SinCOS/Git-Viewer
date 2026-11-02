# 远端 Git Helper 协议 v1

本协议由 `internal/remotehelper` 与 `cmd/gitviewer-helper --serve` 实现；系统 OpenSSH 提供认证、加密与严格 host key 校验。它只服务当前 SSH 用户，不提供独立监听端口或多用户授权。

## 帧与数据类型

每帧为 4 字节无符号大端 JSON 字节长度，紧跟一个 UTF-8 JSON 对象。长度必须在 1–16 MiB，空帧、超限、截断、非法 JSON 使连接终止。写入处理部分写，不能通过重发修复半帧。

`dir`、`args` 中每个参数、`commands` 中每个参数、`input`、`stdout`、`stderr` 都是标准且规范的 base64 字符串；Go 分别使用 `[]byte`、`[][]byte`、`[][][]byte`。目录和 Git 输出本身不要求 UTF-8。不接受数字数组、null、重复 key、未知字段、字段大小写替代、当前消息无关字段或尾随 JSON。

请求与响应 DTO/schema 同步维护，并由 `internal/remotehelper` 契约测试检查：

- [请求 schema](schemas/remote-helper-request.schema.json)
- [响应 schema](schemas/remote-helper-response.schema.json)

Schema 描述 JSON 形状；解码后的字节预算、规范目录、命令白名单、握手和请求关联由执行器检查。

## 消息

| 方向 | type | 字段及含义 |
|---|---|---|
| 客户端 → helper | `hello` | `proto: 1`，必须第一条且只出现一次 |
| helper → 客户端 | `ready` | `proto: 1`、`platform`（如 `linux/amd64`）、`pid` |
| 客户端 → helper | `exec` | 非零 `id`、`dir`、`args`，可选 `limit`、`timeoutMs`、`input` |
| helper → 客户端 | `result` | 原 `id`、`exit`（成功 0 可省略）、可选 `stdout`、`stderr`、`error` |
| 客户端 → helper | `batch` | 非零 `id`、`dir`、`commands`，可选 `limit`、`timeoutMs`；只读命令 |
| helper → 客户端 | `batchResult` | 原 `id`、按请求顺序排列的 `results`；或顶层 `error` |
| 客户端 → helper | `cancel` | 非零 `id`；取消该请求，未知或已完成 id 无操作 |
| 客户端 → helper | `ping` | 无其他字段 |
| helper → 客户端 | `pong` | 无其他字段 |
| 客户端 → helper | `shutdown` | 取消全部请求并退出 |
| helper → 客户端 | `fatal` | `error`，随后终止连接 |

`error` 为 `{ "code": "固定错误码", "message": "可选固定说明" }`。命令错误码包括 `UnsafeOperation`、`InvalidRepository`、`OperationCancelled`、`OperationTimedOut`、`OutputLimitExceeded`、`GitUnavailable`、`CommandFailed`、`ProtocolError`。客户端仅接受此集合，不直接展示远端任意 message。`RemoteUnavailable` 与 `OperationStateUnknown` 由客户端/受控写入层产生，不是 helper 命令响应码。

每个接纳的 exec/batch 恰好一个终态响应，连接断开时除外；不同请求可以乱序返回。重复在途 id、零 id、未知响应 id、终态重复、响应类型不匹配和 batch 结果数量不匹配均作为协议错误关闭连接。客户端使用递增 id，不在活跃连接上抛弃取消中的 pending。

## 限制与调度

| 项目 | 限制 |
|---|---|
| 帧 JSON | 16 MiB |
| 单命令 stdout + stderr | 默认 2 MiB，最大 8 MiB；负数拒绝 |
| stdin | 2 MiB |
| 单个参数 / 单条命令参数总量 | 16 KiB / 1 MiB |
| dir | 4096 字节，绝对、规范、无 NUL 的 POSIX 路径 |
| batch | 1–4 条，全部预校验，只读；聚合最多 8 MiB |
| 并发执行 | 读最多 8，写最多 1；读写槽分别管理 |
| 在途请求 | 最多 64，另有有界响应队列 |

batch 并行执行，单命令实际输出预算为 `min(limit, 8 MiB / 命令数)`；默认每条 2 MiB 在最多四条时保持不变。显式大预算会保守均分，不借用其他命令的剩余额度。输出溢出返回顶层 `OutputLimitExceeded`。

默认读超时 15 秒、写超时 2 分钟；客户端发送的超时最多 2 分钟。服务端从接纳请求开始计时，覆盖读写槽排队与整个 batch。负数和时长转换溢出拒绝。服务端按 argv 自行判定写入，客户端不能通过声明改变环境或调度。

## 固定 Git 执行

系统 Git 通过明确 argv 启动，前缀为：

```text
git --no-pager --no-optional-locks --literal-pathspecs
    -c core.fsmonitor=false -c log.showSignature=false -c color.ui=false
    -C <dir> <已校验的产品参数>
```

白名单覆盖应用实际 Reader 的读、hunk、索引、commit/amend、branch、stash、cherry-pick/revert、worktree 和 fetch/pull/push 形状。产品 argv 覆盖测试在隔离仓库和本地 bare 上游执行这些操作。任意子命令、危险全局选项、额外参数、NUL 或任意 `-c` 被拒绝。仅固定网络操作可携带固定 `core.sshCommand`，push 必须携带与目标 remote 匹配的 `mirror=false` 并显式限制 refspec。

读清除继承的 Git 环境并忽略系统/全局配置。写恢复正常用户 Git 配置，使身份、hooks 和凭据助手保持可用；禁止终端提示。Git 内部上游 SSH 同样启用严格 host key、非交互与连接超时策略。

## 取消、死亡与回退

取消通过 `cancel` 触发 context，Unix 进程组先 TERM、再 KILL；即使 Git 父进程先退出，仍等待对子孙进程的升级清理。EOF、shutdown、信号或协议失败都会取消全部工作、关闭流并等待工作协程。Unix 继承的 pipe/socket 使用可中断的 Go poller 适配，避免 stdin 未关闭或 stdout 阻塞造成 SIGTERM 挂住。

客户端取消后等待最多 3 秒终态确认；无法确认则废弃连接并失败所有在途请求。发送半帧或关闭时不重发。正常关闭先请求 shutdown，再有界等待与回收本地 SSH 进程。空闲心跳间隔 30 秒。

启动前部署不可用允许 CLI 回退，认证/host key/非法输入/协议错误直接返回。请求发出后仅 `RemoteUnavailable` 的读取允许一次 CLI 回退；命令拒绝、协议错误和普通 Git 非零退出均不重试。发出的写入始终不回退，受控 Write/worktree 层在取消、超时、断线、输出超限或协议错误后报告 `OperationStateUnknown` 并独立读取验证。

SIGKILL 无法执行 helper 清理；尤其 macOS 没有 Linux `Pdeathsig`，极端情况下可能残留 Git 子进程。该限制不允许客户端重试写入。真实部署、测试结果与未验证边界见 [执行记录](remote-helper-execution.md)。
