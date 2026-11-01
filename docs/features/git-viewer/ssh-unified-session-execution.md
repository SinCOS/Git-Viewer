# SSH 会话统一执行记录

承接用户对 Changes/Staged 少量文件延迟、复用 History 技术，以及“所有 SSH，包括文件编辑器、文件列表，都应统一”的要求。本轮遵循 ub-mode，以连接与会话入口收敛为目标。

## 操作日志与结果

1. 读取截图、项目约束与各 SSH 调用入口，保存修改前文本；保留工作区原有 UI/diff/终端等改动及暂存内容。
2. 先验证暂存新握手、untracked 重复读取、普通与分组请求参数不统一的问题；原代码三个回归均按预期失败。Git 普通/分组统一到 runGit，窄范围索引写入可复用 master；取消仍返回未知并复核。详见 [Git 通道记录](source-control-shared-transport-execution.md)。
3. 新增 SessionMode（Shared/Isolated/Terminal）、SessionArgs 和 Command；同一执行器负责固定 SSH 参数、认证环境、连接池和远端参数编码。调用方保留自己需要的流式 I/O、输出上限和进程 Wait。Git 请求继续使用既有每连接并发限制，长生命周期文件/终端 channel 不长期占据 Git 的短请求槽位。
4. SFTP Config 改为 SSH Executor 的别名，应用服务、文件浏览器、冲突编辑直接传递同一 Remote，消除逐字段拷贝漏传连接池的问题。文件列表与编辑器继续复用既有 SFTP Client，增加统一子系统启动入口。
5. SFTP 初始化挂接调用方取消；初始化结束后会话生命周期仍归 Client。增加握手停滞 fixture，确认取消能返回且不关闭 master。认证诊断有界、同步读写，错误改用统一 SSH 分类。
6. 终端认证与主连接准备移到后台，SessionArgs(TerminalSession) 获得同一 master；UI 线程只验证 host、准备占位命令，不阻塞认证。PTY 与终端生命周期仍由终端组件负责。
7. 远端 code 安装/检查、桥接 endpoint 写入/清理改用统一 Command；配置透传同一连接池。反向转发经过统一严格配置但显式隔离，以保持关闭进程撤销转发的所有权，避免误关文件或 Git channel。
8. Git、SFTP、code、桥接共用 SSH 错误分类；不把原始 stderr 作为产品错误日志泄露。安装检查的 exit 3 仍表示尚未安装。
9. 增加同一 socket 的终端/SFTP 准备断言；真实 SFTP 中把新认证改为无效身份后，第二个会话仍能列目录和读取编辑器文件，证明复用已认证 master。真实桥接测试注入并关闭自己的连接池，修复持久 master 下 fixture sshd 的退出等待。
10. 全量测试与针对性 race 通过。并发全量+race 压力下原 hook 启动 3 秒门限出现一次失败，改为 8 秒等待已认证任务开始；取消触发后的返回上限保持 3 秒，未放宽产品超时。重新运行 race、静态检查、build、Intel 打包通过。
11. 操作日志、任务状态及验证边界随本轮改动提交本地 Git，不推送远端。按照差异限制纳入，不提交旧工作区其他改动。

## 验证

```sh
.tools/go/bin/go test ./... -count=1 -timeout=60s
.tools/go/bin/go test -race ./internal/infrastructure/ssh ./internal/infrastructure/sshfs ./internal/bridge ./internal/remotecli ./internal/ui/local -run 'TestConnectionsLifecycle|TestOpenSSHIntegration|TestRemoteReader|TestSFTP|TestReverseUnix|TestInspect|TestRemoteTerminal|TestOpenCancellation' -count=1 -timeout=45s
.tools/go/bin/go vet ./...
.tools/go/bin/go tool mygo vet
git diff --check
make build
.tools/go/bin/go tool mygo build -platform darwin/amd64 -skip-dmg
```

本轮统一的是传输与认证会话，不是把 Git、SFTP、PTY 合并成一种业务协议。应用默认共享一个连接池，自定义/测试可显式注入池；应用退出关闭池，各消费者关闭自己的 channel。

## 边界

- 没有部署远端 helper；实际文件系统操作继续走 SFTP，Git 继续系统 Git CLI，终端继续 PTY。
- 未测真实 n1 网络和物理 GUI 交互；不能从 loopback 成功推导全部网络延迟已消除。
- SFTP 请求仍按既有 Client 串行执行，本轮处理初始化取消，未全面重写已打开会话的逐请求取消和断线恢复。
- 主连接在认证后保持该会话身份；改 SSH 配置不会强制重认证。
- 当前工作区全量验证通过；原 HEAD 的 history 解析器等未提交依赖问题沿用前轮记录。临时提交 index 只纳入本轮差异，未伪称独立检出通过。
- crapper/dryer/mutator 当前未提供，未宣称通过这些工具检查。减少重复参数构造、配置拷贝，并保留行为与真实 SSH 回归证据。
- Intel 应用包位于 `bin/package/darwin-amd64/Git Viewer.app`，构建/签名通过不等同正式分发验收。
