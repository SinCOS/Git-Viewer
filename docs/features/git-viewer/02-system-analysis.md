# 02 — 新项目系统预分析

状态：Phase 0 已建立真实工程并完成 T001–T005；原生 UI、Git/SSH、解析、监听/保存 POC 已验证。原始核对见 [核对记录](phase-0-review.md)，当前结果见 [执行记录](phase-0-execution.md)。

## 1. 系统边界
- 【原文明确】本地 macOS 桌面应用，管理本地与通过 OpenSSH 可访问的远程 Git 仓库。
- 【建议】MyGo 原生 Go UI，Go 应用层，Git CLI 执行器；SSH 远端不安装守护进程。
- 外部依赖：本机 Git、系统 OpenSSH、ssh-agent/Keychain、远程 sshd、远程 Git、远端仓库权限、远端 Git credential helper。

## 2. 调用链（拟定）
原生 UI 事件 → Go 应用服务 → 应用服务（权限/状态/任务调度）→ Git 操作构建器 → LocalExecutor 或 SshExecutor → Git CLI → 字节输出解析器 → Go 视图状态。

## 3. 本地与远程差异
| 维度 | Local | Remote |
|---|---|---|
| Git 执行 | Go 子进程 argv | OpenSSH exec；远端 shell 需安全编码 |
| 仓库路径 | 本机路径 | 远端路径；不能当作本机路径 |
| 认证 | 本机文件权限 | SSH host key/密钥；远端 Git 认证独立 |
| 刷新 | 文件监听 + 操作后刷新 + 低频兜底 | 定时轮询 + 操作后刷新 + 失败退避 |
| 中断 | 终止子进程 | 终止 ssh 进程；远端执行状态可能未知 |

## 4. 风险与验证原型
1. **远程命令安全**：OpenSSH 远程执行经过远端 shell；构建参数编码器与命令白名单，并测试包含空格、单引号、换行、前导连字符的路径。
2. **二进制协议**：Git -z 输出是字节序列，不可按行或仅按 UTF-8 字符串解析；设计字节级 parser。
3. **SSH 非交互性**：GUI 环境下 ssh-agent、host key 确认、密码提示与 Git credential helper 的行为需 macOS 原型验证。
4. **文件监听**：Git 状态受工作区文件、索引、HEAD、refs 影响；监听失效/溢出时需完整刷新兜底。
5. **写操作并发**：同仓库串行化写操作；不同 Worktree 共享 Git common dir 的并发锁与状态刷新要验证。
6. **远端 Git 版本**：探测版本并对不支持的特性降级/提示。
7. **仓库标识**：worktree git-dir 与 common-dir 不同；缓存键使用 host + canonical path + worktree 身份，不能仅按分支名。

## 5. 原型检查点（代码开发前）
- POC-A：本地 status -z、diff、log、worktree porcelain 解析正确。
- POC-B：使用系统 SSH Config 连接，安全执行远端 Git，并捕获 stdout/stderr/exit status。
- POC-C：无 TTY 情况下的 SSH/Git 认证及失败反馈。
- POC-D：本地文件监听在修改、checkout、commit 后的刷新正确性。

## 6. 代码探索待办
- 当前入口 `cmd/git-viewer` → `internal/ui/poc`（模拟数据 UI/任务控制）→ `internal/application.Health` → `internal/infrastructure/process` → 系统 Git。UI 后台结果经 `Window.Update` 调度回主线程；取消/关闭通过 context 和 worker 等待清理。
- Git POC：`internal/domain` 定义命令/DTO/错误；`infrastructure/git.LocalExecutor` 与 `infrastructure/ssh.Executor` 共用固定只读 argv 与 `process.Run`；解析器位于 `infrastructure/git/parsers.go`。SSH 调用系统 OpenSSH 并对远端 shell 参数独立编码。它们通过测试运行，不连接正式仓库管理 UI。
- `infrastructure/storage` 仅为原子 JSON 配置 POC；官方 `mygo.WatchFiles` 通过集成测试验证原子替换通知。Workspace JSONC、业务刷新协调器和正式服务均未实现。
- 测试覆盖：schema 真实校验、子进程边界、真实 Git fixtures、临时 loopback sshd、解析器 fuzz、官方 ui.NewTester 交互、原生窗口与打包冒烟。已建立 macOS CI，未在远端触发。详见执行记录和 evidence/README.md。


## 7. Workspace 新项目预分析（待代码验证）
【原文明确】`.code-workspace` 读取、创建、保存。
【建议】新增 WorkspaceService 与 JSONC 文本编辑器，不直接将 Workspace 视为 Git 仓库。拟定调用链：原生 UI Open Workspace → WorkspaceService → JSONC parse → folder path resolve → Git repository discovery → Workspace View；保存链：UI 编辑 folders → 校验当前文件指纹 → JSONC 最小编辑 → 临时文件 → 原子替换 → 重新读取验证。

风险：JSONC 注释/尾随逗号与 JSON Schema 不同；VS Code 文件可能含未知属性；相对路径、软链接、嵌套仓库、子目录重复引用；外部编辑竞态；跨设备 SSH URI 非标准。

POC：验证 JSONC 解析与保留注释的编辑库；构造带 settings、extensions、注释的真实示例进行 round-trip；测试修改冲突与原子保存。
【待确认】正式编码前验证所选 JSONC 库对保留注释、缩进、尾逗号的实际支持程度。

## v1.2 新框架预分析
【原文明确】框架切换为 egoist/mygo。
【建议】优先验证 MyGo `ui` 原生窗口生命周期、布局/虚拟列表/绘制/事件处理、构建链，再做 Git 业务开发。
- 已锁定 Go 1.27.3 与 MyGo 9c69cf829ece；当前 Intel macOS 的构建、原生窗口启动和 .app 打包通过。arm64 构建/打包通过，Apple Silicon 真机启动及发布签名/公证仍待验证。


## v1.3 原生 UI 预分析与技术风险
- 【原文明确】所有 UI 均采用 MyGo `ui` 包，禁止 WebView/HTML/JS/TS。
- 【建议】优先验证可拖拽三栏、长列表虚拟化、可选中文本 Diff、Git Graph 自绘连线、键盘快捷键与输入法、主线程更新和后台任务取消。
- 【建议】Git Graph 的拓扑计算独立于 UI 绘制，数据层只暴露节点、边、分支标签和分页结果。
- 【待确认】原生组件的精确 API 与性能必须基于锁定 MyGo 版本实际编译、测试和测量，不直接假设所有设计都已受支持。
