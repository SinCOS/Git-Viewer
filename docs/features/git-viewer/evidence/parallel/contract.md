# Phase 0 补充验证：共享契约与文件所有权

用户已确认执行 A/B；仅限 Phase 0，不进入 Phase 1。初始代码基线：bfc5bdc。工作分支必须从本契约提交建立。

执行方式：当前会话无独立 Coding Agent 调用接口，由主 Agent 在两个独立 Worktree 中执行已确认的 A/B 工作线，独立提交，并行运行互不依赖的检查。不宣称存在独立子 Agent。

## 冻结接口

- `domain.GitExecutor.Execute(context.Context, domain.GitCommand) (domain.GitResult, error)`。
- `GitCommand` 实际字段为 `Repository RepositoryContext`、`Operation GitOperation`、`Timeout time.Duration`；03 的 RepositoryID 示例为概念接口，实际以此为准。
- GitResult：ExitCode、Stdout、Stderr。GitError 使用既有 Code；OperationCancelled、OperationTimedOut、RemoteUnavailable 区分。
- `ssh.Executor` 的 SSHPath、ConfigPath、KnownHostsFile、OutputLimit 不变；严格主机密钥校验不变。
- `poc.New()`、App 现有 View/HistoryView/GraphView/DiffView、List、Tasks 与 ScrollState 不变；UI 状态只在主线程写入。
- Go 1.27.3、MyGo v0.0.0-20261025001514-9c69cf829ece、现有 DTO/schema 不变；子工作线不得更改共享接口或依赖。
- 发现共享实现缺陷时，由主 Agent 单独修复、测试并提交，不混入 A/B 的文件范围。

## 文件所有权

| 工作线 | 可写文件 |
|---|---|
| A | cmd/phase0-ui-probe/**；internal/ui/poc/native_probe_test.go；docs/features/git-viewer/evidence/native-ui/** |
| B | internal/infrastructure/ssh/runtime_integration_test.go；internal/infrastructure/ssh/runtime_fixture_test.go；docs/features/git-viewer/evidence/ssh-runtime/** |
| 主 Agent | 其余文件，尤其共享实现、依赖、CI、规划状态及证据总索引 |

子工作线不修改现有 view.go、tasks.go、executor_test.go 或 Makefile。报告写各自独立目录，不修改 05-tasks.md。只提交本工作线文件。

## 报告契约

报告 JSON 采用同目录 report.schema.json。必须记录冻结提交、机器与工具版本、执行命令及退出码、场景结果、带单位的指标、跳过/未验证项。输出不得包含测试私钥或认证信息。报告与日志中的 SHA 标明测试对象；不能把基线或工作分支的测试冒充集成后验证。

## 验收

A：复用模拟数据，在真正 MyGo 原生窗口中运行列表定位/滚动、Graph 绘制/滚动和 Diff 场景；统计 view 构建时间、主线程调度延迟、内存等可测指标。图形输入、复制和命中由官方 Tester 补充，不能冒充真实 OS 输入自动化。无 GPU 计数器则明确 GPU 指标未验证；不设未经确认的 FPS 产品门槛。

B：使用真实 loopback SSH 连接，明确等待远端测试任务进入执行后再取消/超时/断线。取消或 deadline 后 3 秒内返回；隔离测试用总超时，退出需回收 fixture 资源。不得仅靠预取消 context 或连接前拒绝推断运行中结果；不声称本地取消能保证远端任务终止。

集成：逐提交检查文件白名单及冲突；make check、依赖校验、native smoke、双架构打包与签名检查；文档与 Task 状态由主 Agent更新。C（MY008/NU008 的 Apple Silicon 真机）缺设备，保持未完成。
