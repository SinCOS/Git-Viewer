# 全量剩余任务执行与验收

用户选择 B，授权全部后续阶段及未完成补充项。清单复核：原有 22 项已勾选、34 项未勾选；先前会话回复写作 40 是加总错误。此次按全部实际剩余范围实施，不缩减授权范围。

主 Agent 承担共享接口、集成与 Review。当前无独立子 Agent API，因此由主 Agent 分线实现，独立读取、测试、静态检查与构建并行执行；没有宣称启动多个 AI Agent，也未建立或合并新的工作线提交。工作区变更未提交或推送。分线边界见 [编排记录](remaining-orchestration.md)。

## 实现映射及阶段验收

| 阶段/Task | 实现 | 验证 |
|---|---|---|
| 2：T201–T204 | SSH Host 建议/输入、连接与 Git 版本/仓库探测、远程注册、共用 Status/Diff | 临时真实 loopback sshd；特殊路径/中文/换行/引号；未知/变更 host key、认证、无权限、非仓库、取消/超时/断线；Git 缺失及旧版本用受控进程测试错误映射 |
| W：TW001–TW006 | JSONC 保留编辑、Workspace DTO/schema、发现去重、revision 与原子保存、原生管理/最近记录 | 注释、尾逗号、未知字段、重复 key、软链接、非 Git/缺目录、外部编辑冲突、只读文件/目录写失败及临时文件清理；ui.Tester 创建/打开/冲突路径；应用保存后的文件由隔离 VS Code 原生启动并注册 Workspace |
| 3：T301–T304 | 固定 OID 集合分页、完整消息及文件详情、本地/远程分支及上游、Worktree 信息 | 初始空历史、提交期间分页稳定、真实 detached、双根、tag-only 根、锁定 worktree、共同 common-dir；本地与 SSH 读取 |
| 4：T401–T403 | 跨页 GraphLayout；原生绘制、虚拟列表和横向滚动 | 真实三父 octopus merge、独立根、detached；10,000 个真实提交首尾分页；10,000 行 UI 定位与构建行数限制；Graph benchmark |
| 5：T501–T505 | common-dir 队列、精确 Stage/Unstage、stdin Commit、safe Switch、失败后 Status 复核 | 12 个并发写请求跨 worktree 串行；初始提交取消暂存、删除、重命名后继续修改、特殊 pathspec；全局作者身份、hook 拒绝；脏工作区/已占用分支拒绝；真实 SSH Stage/Unstage/Commit/Switch |
| 6：T601–T604 | Fetch 运行中指示、Pull ff-only、显式当前分支普通 Push、未知状态复核 | 临时 bare 上游 Fetch/Pull/Push、真正 fast-forward 更新、分叉拒绝、push mirror/通配 force refspec 配置不扩大推送；真实 SSH 同步；等待远程 Commit hook 已运行后取消，返回 Unknown 并重读 Status；不重试写入 |
| 7：T701–T703 | 本地工作区/元数据监听、兜底、前后台远程轮询与退避、关闭清理 | 原生文件事件、监听失效兜底、poll/退避恢复、后台间隔策略、取消清理；自动刷新保留选择并更新 Diff；全量 race、10k fixture 与 UI 虚拟化 |
| MY007、NU007 | 已将监听/原子 JSON POC 衔接至 JSONC 与原生 Workspace 对话框 | 同 W 阶段及原 storage watcher 回归 |
| T704、MY008、NU008 | 两架构 .app、ad hoc 签名、Intel 原生启动及安装副本、预览说明已准备 | arm64 真机、正式发布验收缺环境/尚未执行，保留未勾选 |

独立模块可先实现；正式集成按 2 → W → 3 → 4 → 5 → 6 → 7 的数据依赖串接。后阶段测试依赖前阶段的真实仓库、受控传输与 Workspace 能力，不以 POC 单独通过代替产品集成。

## 共享边界与安全审查

- `GitOperation` 现有 JSON 字段不变。分页 cursor、传输函数、输入流与执行上下文是内部类型。新增 Workspace DTO 与既有 schema 对齐，新增 SavedLocation schema；契约测试校验字段集合、必填项和实例。
- `LocalReader` 暴露固定业务操作；SSH 绑定器只接收内部构造的 argv。远端参数逐项 POSIX 编码，Host/绝对路径验证。应用没有 Shell/Git 命令输入框。
- 写入前重查 Status；同 common-dir 进程内排队，并保留 Git 自身锁。Stage 使用 literal pathspec 和 `--`。已暂存重命名后继续修改时不对已不存在的旧 index 路径再执行 add；Unstage 恢复两个路径。
- Commit 消息经本地/SSH stdin；写命令恢复正常系统/全局 Git 配置，以支持身份、hooks、credential helper，同时移除继承的 GIT_* 覆盖。只读路径继续禁用外部 diff/textconv 等行为。
- Fetch/Pull/Push 的 Git 内部 SSH 调用也固定 BatchMode/StrictHostKeyChecking 等安全选项。Push 指定一个当前分支到其上游 ref，显式禁用 mirror、force、follow-tags、递归 submodule push，避免用户 Git 配置把操作扩大。
- 失败后用独立、受限 context 重读 Status；远程中断及复核失败标 Unknown。测试证明不重复发起写入。UI 显示固定错误说明，原始 stderr/argv 不写日志，避免泄露认证信息。
- Workspace 编辑保留数组外全部字节；数组内注释保留但可能移位。未知 folder 属性不能无损修改时拒绝保存。保存前两次 revision 校验、同目录临时文件/fsync/rename；对非协作外部进程不提供操作系统级 compare-and-swap，最终校验到 rename 之间仍存在极短并发窗口。测试覆盖保存前已发生的外部修改，未宣称跨进程事务隔离。

## 本机检查

环境：Intel/x86_64 macOS，项目 Go 1.27.3，MyGo `9c69cf829ece`，系统 Git 2.39.5 (Apple Git-154)。最低 Git 门槛 2.38 是代码约束；尚未在 2.38 或其他 macOS 版本做兼容矩阵。

执行命令：

```sh
make check
.tools/go/bin/go mod verify
.tools/go/bin/go test -v -count=1 ./internal/infrastructure/ssh -run TestOpenSSHIntegration
.tools/go/bin/go test -run '^$' -bench BenchmarkGraph10000 -benchmem ./internal/domain
make package
codesign --verify --deep --strict 'bin/package/darwin-amd64/Git Viewer.app'
codesign --verify --deep --strict 'bin/package/darwin-arm64/Git Viewer.app'
```

全量 race、Go vet、MyGo vet、编译和依赖校验通过。新增真实 SSH 产品读写、运行中写入取消场景无 skip。最终 [make check 日志](evidence/remaining/final-check.log)、[安装副本原生 smoke](evidence/remaining/installed-native-smoke.log)、[Workspace 原生 smoke](evidence/remaining/workspace-native-smoke.log) 已归档；原始文件也保存在 `.tools/acceptance/`。

GraphLayout 10,000 节点 benchmark：约 **2.76 ms/op、2.88 MB/op、20,082 allocs/op**（本机一次测量）；这是 CPU 算法测试，不是 GUI 帧率。真实 10k Git history 和 UI 虚拟化有独立测试，不以此 benchmark 代替它们。

产物：`bin/package/darwin-{amd64,arm64}/Git Viewer.app`，分别为 x86_64/arm64 Mach-O，均通过严格 codesign 校验。安装副本位于 `.tools/acceptance/install/Git Viewer.app`，未写入用户 Applications。

## VS Code 兼容性证据

应用 `workspace.Save` 编辑了 `.tools/acceptance/workspace/native.code-workspace`，保留注释、settings/extensions、尾逗号和未知字段。随后启动已安装的 VS Code，使用独立临时 user-data/extensions 目录、禁用扩展，通过 `User/workspaceStorage/*/workspace.json` 确认文件被注册；读取后源文件字节未改变。已关闭测试进程并清理临时 profile，没有改动用户 VS Code 设置。

一次探针因 user-data-dir 太长而触发 VS Code 的 macOS Unix socket 路径限制；改用短临时目录后通过。最终 [VS Code 报告](evidence/remaining/vscode-workspace-report.json) 已归档，原始输出在 `.tools/acceptance/workspace/`。没有把单纯进程启动返回值当作解析验收，也没有宣称物理输入已测试。

## 尚未验收

- **MY008、NU008**：缺 Apple Silicon 真机，arm64 包虽已构建/签名校验，不能勾选真机启动验收。
- **T704**：当前为内部开发预览；正式目标架构安装验收、Developer ID/公证及公开发布未完成。已有预览包、Intel 安装副本和发布说明，未上传任何产物。
- 物理鼠标/键盘/IME、真实公网 SSH/credential helper 交互、GPU 计数器及跨 macOS/Git 版本兼容性没有冒充实测。远程写入取消不保证远端进程已经终止；测试刻意等待 hook 收尾并使用失败 hook 避免误提交。

本轮关闭 34 个原未完成条目中的 **31 项**，剩余 **3 项**保留未勾选。补充条目与主任务存在重叠，数量不是独立工作量估算。
