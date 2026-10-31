# SCM 与终端深化执行记录

日期：2026-10-31

本轮补齐 SCM 的冲突解决、分支图交互和提交图深层操作，并把集成终端从单本地会话扩展为本地/SSH 多标签与可恢复会话。应用仍为纯 Go + MyGo 原生 UI。

## 已实现

- **三路冲突解决**：新增 `ConflictFile` 读取 base/ours/theirs 三个 Git stage，并读取当前 worktree 结果。冲突面板提供三栏只读对照、可编辑 Result、冲突块 Previous/Next、Take Ours/Theirs/Both；保存时原子写 worktree，再只 Stage 该路径。二进制、超限和非普通文件拒绝应用内解决。
- **安全写入边界**：解决冲突前重新确认路径仍处于 unmerged 状态；写入保留原权限，拒绝 NUL、超大内容和根目录逃逸；远程仓库通过受限 SFTP 写回，再由固定 Git 写入通道执行 Stage。
- **分支图交互**：分支行继续单击筛选历史，右键新增 Reveal HEAD in Graph、Create branch here、Copy full ref、Copy commit hash；分支头可在已加载提交图中定位并选中。
- **提交图深层操作**：提交右键菜单新增 Open Changes、Create Branch Here、Cherry-pick、Revert、Copy short hash、Copy subject。新增 `createBranch`、`cherryPick`、`revert` 三个受控 GitOperation；只接受完整 commit OID、合法分支名和干净工作区，创建分支后显式切换。
- **多终端标签**：终端改为标签模型，可新建本地终端、选择 SSH Host 新建远程终端、切换前后标签、关闭/重启当前标签。每个标签独立保存标题、状态和终端实例。
- **远程 SSH 终端**：使用系统 OpenSSH，固定 `-tt` 与现有严格 HostKey/Config/KnownHosts 参数，仅接受 SSH Config Host 别名，不拼接任意 shell 命令。
- **终端会话恢复**：`terminal-sessions.json` 保存类型、Host、cwd、标题和最多 256 KiB 终端快照；重启应用后恢复标签元数据，打开终端时重连并回放屏幕。远端重连失败会保留标签并显示错误，不会静默启动本地 shell。
- **命令与快捷键**：新增 New Terminal、Close Terminal、Next/Previous Terminal 命令，默认快捷键为 `Cmd+Shift+\``、`Cmd+Shift+]`、`Cmd+Shift+[`。

## VS Code 对齐评估

这里区分“Git 为核心的编辑器子集”和“完整 VS Code 产品”。完整 VS Code 还包含扩展市场、语言服务、调试器、任务系统、Dev Containers 等大型平台能力，当前项目不在同一产品范围内。

- Git 为核心的编辑工作流：约 **65%–70%**。编辑、Explorer、搜索替换、Source Control、历史/Graph、本地/SSH 终端、远程工作区已有可用闭环。
- 完整 VS Code 功能面：约 **35%–40%**。主要差距不是 Git UI，而是扩展、语言服务、调试和任务生态。
- Source Control：约 **75%**。已覆盖 Status、Stage/Unstage、Hunk、Commit/Amend、分支、历史、冲突解决、cherry-pick、revert；仍缺完整 Rebase 交互、合并编辑器中的逐行/逐块接受、Tag 管理、Submodule 管理、force-with-lease 等。
- 终端：约 **70%**。已覆盖本地/SSH、多标签和屏幕恢复；仍缺分屏、任务终端、Profile 管理、进程真正保活、shell integration 装饰与命令导航。
- 设置/命令系统：约 **55%**。已有可持久化设置、动态快捷键与命令面板；仍缺 VS Code 的 JSON/UI 双层设置、上下文键、when 条件、扩展贡献点和多套 Profile。
- 扩展/语言/调试：约 **5%–10%**。没有扩展宿主、LSP/DAP 客户端、问题面板、调试控制台和任务运行器。

## 仍需补齐

- 冲突解决：把当前三栏 + Result 面板升级为逐行三方 diff、按块接受、保存前校验和 merge conflict 文件类型覆盖。
- Git 历史：Rebase interactive、merge/cherry-pick conflict continuation、tag CRUD、submodule、reflog、blame 行级交互。
- 终端：split pane、任务终端、shell profiles、真实进程保活；当前恢复是“重连 + 屏幕回放”，不是恢复原进程。
- 编辑器平台：LSP、DAP、问题/测试面板、任务系统、扩展 API。

## 验证

- `go test ./...`：通过。
- 新增覆盖：三路冲突读取与解决、干净路径拒绝、CreateBranch/CherryPick/Revert、终端多标签、SSH 命令参数、会话持久化和快照上限、冲突块接受逻辑、提交定位。
- 全仓 race、vet 与原生构建在交付前执行。

## 边界

- 终端真实 PTY/SSH 交互仍需在原生窗口中用真实 SSH 主机验证；自动化测试验证命令构造、状态和持久化，不宣称替代真实交互。
- 会话恢复只恢复屏幕和标签，不恢复运行中的 shell 进程。
- Cherry-pick/Revert 产生冲突时会进入现有 Merge Changes；冲突 continuation（`--continue` / `--abort`）尚未提供独立 UI。
