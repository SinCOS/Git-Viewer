# Changes / Staged 与 Git Log / History 统一执行通道

用户补充指出少量文件的 Changes/Staged 仍慢，并明确要求复用 Git Log/History 的技术。本轮以统一传输入口为核心，遵循 ub-mode。

## 操作日志

1. 读取截图、当前工作区与 SSH/Git 调用链，保存修改前文本。截图仅少量文件，上一轮批量路径构造的基准不能解释此处延迟。
2. 检查发现读取已共用 master，但 Stage/Unstage 仍每次独立握手；History 的 Batch 和普通 Transport 各自拼 Git 环境及参数；远端 untracked 先生成 diff，再用同样命令重复获取完整内容。
3. 先修改/增加回归并观察预期失败：暂存日志含 `-S none`；untracked 请求为 `[status diff diff]`；普通 Status 与 History 分组调用生成不同 remote argv。
4. 新增统一 `runGit`，普通 Transport 与 Batch 都调用同一个入口。Changes、Staged、History、提交详情共享固定参数、Git 只读配置策略、连接池、每连接 4 个请求限制、输出上限、取消和错误分类。Batch 只承担本地固定读取分组调度。
5. 窄范围允许固定 `add --`、`restore --staged --`、`rm --cached --` 复用同一 master，去掉每次索引操作的认证握手。提交、hook、工作树修改、网络写入及其他形式继续独立连接。Git 写前读取、串行写队列和写后独立复核保留；取消仍报告结果未知，不自动重试。
6. 真实 loopback sshd 中将后续新认证配置成失败：Status、Stage、Unstage 仍通过既有 master 成功，而隔离执行器失败，证明索引操作无需新握手。
7. 新增真实 SSH slow clean filter 场景，在 add 运行后取消，断言 OperationStateUnknown、Verified、Unknown；等待 fixture filter 完成并确认 master 继续可读。另覆盖 hook 提交取消与主机校验的原有测试。
8. untracked 从已取得的 patch 恢复完整文本，保留末尾无换行；正常读取缩为 `[status diff]`，不再重复传输整个文件。断线错误仍正常返回。
9. 完整工作区全量测试、针对性 race、Go/MyGo vet、build、Intel 打包通过。保存操作日志并按差异提交本地，不推送。

## 验证

```sh
.tools/go/bin/go test ./... -count=1 -timeout=60s
.tools/go/bin/go test -race ./internal/infrastructure/ssh ./internal/infrastructure/git -run 'TestOpenSSHIntegration|TestRemoteReaderReads|TestRemoteUntracked|TestIndexConnection|TestReadBatch' -count=1 -timeout=45s
.tools/go/bin/go vet ./...
.tools/go/bin/go tool mygo vet
git diff --check
make build
.tools/go/bin/go tool mygo build -platform darwin/amd64 -skip-dmg
```

本轮验证的是共享通道、减少请求及取消语义，没有测真实 n1 的完整操作耗时；前文每次新握手 0.48–0.63 秒为历史实测，不作为本轮新性能数据。

当前包：`bin/package/darwin-amd64/Git Viewer.app`。截图/物理 GUI 与真实远端延迟待实机检查。crapper/dryer/mutator 未提供，没有宣称通过其指标检查。

完整验证针对当前工作区。既有未提交的完整 Diff DTO/文本读取及 history 解析器仍为工作区依赖；本轮不将其整体提交。此前 HEAD 单独检出缺少解析器的问题仍见前轮记录。
