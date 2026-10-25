# Phase 0 A/B 补充验证集成记录

状态：A/B 已完成并集成；本轮没有进入 Phase 1。用户已确认建立基线、两个 Worktree 及分支提交；没有推送远端或修改 master。

## 执行方式与提交

当前会话没有独立 Coding Agent 调用接口，实际由主 Agent 按已确认的文件所有权在两个独立 Worktree 中执行 A/B 工作线，独立提交。互不依赖的静态/测试检查并行执行；原生性能测量独占执行，避免与 SSH 测试互相干扰。

| 用途 | 分支 | 提交 |
|---|---|---|
| 保存初始工程 | phase-0 | bfc5bdc |
| 冻结契约 | orchestrator/phase0-validation | 53e686f6cfdaae9d162395f49d6de6007b2acc8f |
| A 原生窗口验证 | agent/phase0-native-ui | 79592a6 |
| B SSH 生命周期验证 | agent/phase0-ssh-runtime | 139b95e |
| 主 Agent 集成 A | orchestrator/phase0-validation | 3ad4eea |
| 主 Agent 集成 B | orchestrator/phase0-validation | 85fa662 |

A Worktree：同级 `git-viewer-wt-native-ui`；B：同级 `git-viewer-wt-ssh-runtime`。均从冻结提交建立，两个分支保持独立可审查。未使用不存在的 master 分支。

## 契约和所有权审查

[contract.md](contract.md) 先冻结现有 Go 接口和依赖，以及 [report.schema.json](report.schema.json)。A 只提交独立探针及 native-ui 证据（7 文件），B 只提交新增 runtime 测试及 ssh-runtime 证据（5 文件）。用 git diff 检查所有路径均在白名单内，两组文件交集为空；共享 domain、UI、SSH/process 实现、schema、依赖均未由工作线修改。两个合并均无冲突。

03 的概念 GitCommand 示例与实际 RepositoryContext 字段关系已由主 Agent 补充说明，不改变实际接口。主 Agent 另增加 report_test.go，对实际报告执行 draft 2020-12 校验、基线/所有权/指标检查及缺失元数据负向测试，保留 GPU/OS 输入 unverified 声明。

## A：原生窗口验证

[原始报告](../native-ui/report.json) 和 [说明](../native-ui/README.md) 来自 A 工作线代码与冻结业务基线。真实 MyGo 窗口脚本驱动四个场景，保存列表、Graph、Diff、二进制占位截图；验证最终行可见、每 pass 少于 100 行、Graph 和 Diff 滚动偏移前进。最多构建 48 行。

原始 CPU view 构建 p95：列表 0.151 ms、Graph 0.009 ms、Diff 0.031 ms；主线程回调排队 p95 0.087 ms，最大约 35 ms。Go heap 指标另记。未声称完整帧耗时、GPU FPS、进程 RSS 或物理输入性能。

独立 `go vet`、mygo vet、既有 UI race 测试及探针执行通过。探针又以 `go run -race` 执行通过。没有将程序驱动状态或 Tester 输入冒充真实 OS 输入自动化。

## B：SSH 运行中生命周期验证

[报告](../ssh-runtime/report.json) 和 [三轮原始日志](../ssh-runtime/test.log) 基于冻结代码。真实 loopback sshd 的 ForceCommand helper 写出 PID 标记，确认认证和任务执行均已发生后，再触发取消、deadline 或已建立连接中断。

三个场景各运行三次，无失败、race 或 skip；最大触发后返回延迟分别为 0.956/0.764/1.069 ms，均低于 3 秒验收上界。测试显式清理 helper、检查进程已回收、关闭连接代理并停止 sshd。不把本地取消推断为真实远端任务一定终止。真实 Git 执行、参数编码、host key 的原有集成测试同步通过。

## 主 Agent 集成后验证

测试对象为集成提交 `85fa662` 的工作树，另含主 Agent 新增报告契约测试；最终记录提交不改变被测产品/探针/fixture 代码。

- `make check` 全部通过：fmt、go vet、mygo vet、所有包 race 测试和 CGO_ENABLED=0 构建，见 [integration-check.log](integration-check.log)。
- `go mod verify` 通过。
- 集成后的原生探针以 `go run -race` 完整运行通过，报告和截图保留在本机忽略目录 `.tools/native-ui-integration/`；该次不用于更新 A 的性能基线。
- `bin/git-viewer -smoke` 通过：创建原生窗口、主线程更新健康状态、停止后台任务。
- `make package` 生成 Intel 与 arm64 .app；两个包的 codesign --verify --deep --strict 通过；Intel .app 的 -smoke 通过。
- 文档链接、报告 JSON/schema、分支文件所有权和后续阶段任务未开始状态已检查。

## 保留未验证项

C（MY008/NU008）缺 Apple Silicon 真机，保持未完成；arm64 打包/签名不能替代启动。GPU 计数器、真实鼠标/键盘/IME、长时间交互、公网 SSH 和复杂认证仍未验证。JSONC/Workspace 留 W；正式本地 Git UI 留 Phase 1，未自动推进。
