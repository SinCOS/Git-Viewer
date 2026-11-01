# SSH 连接复用与远端架构执行记录

本轮依据用户对“本地原生 UI + 可替换远端执行后端”的确认实施第一阶段，保留系统 OpenSSH、系统 Git CLI 和现有业务接口。

## 已实现

- `ssh.Connections` 持有前台 `ssh -N` master，按 SSH 可执行文件、配置路径、known-hosts 路径、密码策略和 host alias 合并首次连接。应用退出关闭连接池；测试可注入独立池。
- master 使用 `/tmp/gv-ssh-*/s`，父目录由 MkdirTemp 创建为 0700，避免 macOS TMPDIR 的 Unix socket 长度限制。不更改用户 SSH 配置。
- 用 `ssh -O check` 确认就绪；认证 stderr 有界保存，不打印。启动上限 30 秒，连接心跳 15 秒/2 次。请求取消不拥有 master 生命周期；进程退出后下次请求重新建立连接，清理自己的 socket 目录。
- Git 读取与 SFTP 共用 master；SFTP 保持独立 subsystem channel。Git 读取每条连接最多 4 个并发请求。
- 停用 Shell/Base64/临时文件批处理协议。原 Batch 回调保留为内部调度接口，每个固定 Git 读取使用独立 SSH 客户端/channel，保持 stdout/stderr、退出码、取消及输出上限。失败后取消同组其余请求并等待回收。
- Reader 写入按固定 Git 子命令分流，显式 `-S none -o ControlMaster=no`，覆盖用户配置中的复用设置。`stash list` 是读取，其余 stash 操作保守视为写入。原有结果未知与写后复核语义保留。
- CommitDetails 的三项固定读取统一分组；UI 同一提交切换文件时复用当前详情快照，仓库/提交切换沿用已有状态清理。未增加跨仓库或持久化缓存。
- 远端仓库先由远端 Git 解析顶层路径，再读取 GitDir/CommonDir；不再读取本机同名 commondir 或解析本机符号链接来决定远端身份。

## 验证

已通过：

```sh
.tools/go/bin/go test ./... -count=1 -timeout=45s
.tools/go/bin/go test -race ./internal/infrastructure/ssh ./internal/infrastructure/sshfs ./internal/ui/local -run 'TestConnections|TestReadBatch|TestRemoteReader|TestOpenSSHIntegration|TestSFTP|TestCommitFileNavigation|TestCommitDetailsFileNavigation' -count=1 -timeout=45s
.tools/go/bin/go vet ./...
git diff --check
make build
.tools/go/bin/go tool mygo build -platform darwin/amd64 -skip-dmg
```

包含真实 loopback sshd/SFTP、特殊路径、Git 工作树生命周期、运行中写入取消和结果复核；新增 fixture 覆盖首次连接并发合并、认证失败后重试、读取取消后继续使用 master、master 退出重建与目录回收、NUL/空输出/输出限额/非零退出。另验证用户配置指定现有 master 时，读取仍能复用，而隔离操作必须重新认证。

集成测试在断开 sshd 前先关闭持有的 master，并对日志管道退出设定 WaitDelay，避免持久会话使 server.Wait 无期限等待。测试后检查未见本轮 master/test sshd 遗留。

打包产物：`bin/package/darwin-amd64/Git Viewer.app`，MyGo 编译与签名步骤通过。不将构建/签名成功等同于 GUI 物理交互或正式发布验收。

## 后续边界

- 本轮没有连接真实 `n1:/root/prj/lotos`，历史五查询 0.446 秒为先前基线，不是本轮结果。须重新测首次打开、首屏历史、切分支、选提交到 diff 和 Source Control 刷新。
- 池按配置路径区分连接，不自动侦测配置文件内容修改；活跃 master 保持已有认证，关闭池/重启应用后重新认证。
- OpenSSH 在 master 与客户端连接之间失效时可能回退独立连接；读取仍受严格 host-key 校验及超时限制。已执行的写入不自动重试。
- SFTP channel 生命周期仍由 Client 管理；本轮没有重写其初始化取消行为。
- 下一阶段若真实高延迟交互仍慢，再实现只提供 OpenRepository/ReadHistory/ReadCommit/ReadDiff/ReadStatus 的远端 Go helper；经系统 SSH stdin/stdout 通信，显式协议版本、帧长度、request ID、错误、取消和限流，并提供 CLI 后端回退。helper 部署、远端通知、跨请求缓存和业务接口抽象替换均未在本轮实现。
- 工作区原有大量 UI、diff、终端、Makefile 改动保留；本轮本地提交按文件/差异块限定，不纳入无关改动。
