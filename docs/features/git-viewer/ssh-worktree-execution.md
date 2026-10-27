# SSH Worktree 接入记录

- D2/D7：将受控 Worktree 创建、删除与状态查看扩展至 SSH。使用现有固定 Git argv、SSH 参数独立编码、仓库 common-dir 写入队列和确认流程；对远端仓库及目标 Worktree 重新打开并核对身份，删除前拒绝锁定、主 Worktree、变更、未跟踪和忽略文件，操作后重新读取列表。
- 创建前通过远端 `realpath` 检查父目录存在且无符号链接，并通过 `test -e`/`test -L` 拒绝已存在路径及悬空符号链接；远端必须提供兼容的 `realpath` 和 `test`。确认对话框显示 SSH host。
- 按用户后续要求，`make build` 本机原生构建通过；测试、静态检查和真实 SSH 验收尚未进行，D2/D7 保持未完成。后续须使用隔离 SSH fixture 检查创建、删除、状态、特殊路径、符号链接、脏目录、连接中断、复核、串行写入，以及实际远端 `realpath` 兼容性；并运行相关 UI 测试与完整 `make check`。
- D4 真实窗口交互、D8 视觉与物理交互、Apple Silicon 真机和正式分发验收仍待对应环境。
