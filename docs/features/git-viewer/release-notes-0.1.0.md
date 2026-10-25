# Git Viewer 0.1.0 内部开发预览

纯 Go/MyGo 原生桌面版本，加入本地及 SSH 仓库、Workspace、提交历史与 Graph、安全暂存/提交/分支切换、Fetch/Pull/Push，以及自动刷新。

## 新增能力

- 本地仓库及 SSH Host 别名连接、最近位置、真实 Status 与统一 Diff。
- `.code-workspace` 打开、创建、目录管理、保存；保留 JSONC 注释与未知配置，外部修改时报告冲突。
- 按批 History、提交消息及文件详情、原生 Git Graph、本地/远程分支和 Worktree 信息。
- 按所选文件 Stage/Unstage、提交当前索引、安全切换分支；操作前确认，失败后刷新。
- Fetch、仅 fast-forward 的 Pull、只针对当前分支上游的普通 Push；不支持 force。
- 本地监听与兜底刷新；远程前后台轮询、失败退避及 stale/未知状态反馈。

## 安装与签名决策

此版本选择**内部预览、ad hoc 签名**，不启用自动更新，不生成 DMG，不做 Developer ID 分发或公证。产物仅保存在项目 `bin/package/`，没有公开发布。

Intel 产物：`bin/package/darwin-amd64/Git Viewer.app`。Apple Silicon 产物：`bin/package/darwin-arm64/Git Viewer.app`。两包均构建并通过严格 codesign 校验；只有 Intel 包在本机运行和安装副本中验证。不要把 arm64 交叉构建解释为真机验证。

需要 Git >= 2.38、系统 OpenSSH；本机实测 Git 2.39.5。最低 macOS 版本及多版本 Git 兼容矩阵尚未验收。固定工具链见 README。

## 验证与限制

全量 race、Go/MyGo 静态检查、契约测试、真实本地 Git/loopback SSH、10k 历史/Graph 虚拟化、原生窗口及 VS Code Workspace 兼容性探针通过。完整命令和证据边界见 [执行记录](remaining-execution.md)。

Fetch/同步目前显示运行中指示，未提供实时百分比或原始 stderr 流。错误使用固定说明，提交消息经 stdin 传递；正常 Git hooks、身份和 credential helper 配置继续有效。交互式认证体验、公网网络兼容性、物理输入/IME 和 GPU 指标没有完成实测。

远程写入取消/超时不能证明远端任务已停止；应用标记状态未知、重新读取 Status，不自动重试。Workspace 使用保存前 revision 检查与原子替换，不提供非协作外部编辑器之间的跨进程事务。

Apple Silicon 真机、正式分发签名/公证和最终发布验收保留为未完成任务。无 clone、行级 stage、强制推送、交互式 merge/rebase 或完整代码编辑器。
