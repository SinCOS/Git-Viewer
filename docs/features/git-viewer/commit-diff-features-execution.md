# Commit / Diff 功能入口实现记录

用户要求实现原先禁用的 File History、Blame、编辑器、终端、Console、Search、Diff 设置、Reset、Discard All 和多选。四条独立 worktree 工作线分别实现读取、Diff 交互、受控操作和系统程序入口，主工作线接入、审查并整合。

- File History 固定读取 HEAD 可达的该路径最近 100 条提交，不跟随重命名；Blame 显示 HEAD 文件的 Git blame。两者使用固定 Git argv、字面路径、512 KiB 输出限制和现有 SSH transport；无 HEAD、未跟踪、二进制或非普通文件会报错。已限制为 HEAD 中存在的普通文件，避免目录路径匹配到后代。
- Files Search 过滤真实变更路径；多选按仓库和状态条目标识维护，支持确认后 Stage、Unstage 和受控 Discard。全选作用于当前搜索结果，状态变化与切换仓库时清除失效选择。
- Diff settings 可切换行号与空白字符显示；复制和查找仍使用原始 Diff。Console 显示最多 100 条本次会话中受控写操作的起止摘要，不保存 Git/SSH 原始输出或凭据。
- Reset form 只清除提交消息、Amend 和表单错误，不执行 Git。Discard All 经确认后对安全的已跟踪工作区变更调用既有 `domain.Discard`，遇到可识别的不安全状态时拒绝，后端继续逐路径检查和复核；不删除未跟踪文件。
- Diff 查看器继续对齐 VS Code：悬停变更行时行号栏出现 + 按钮暂存（已暂存侧显示 −），悬停或定位到变更时显示 Stage/Unstage/Discard Hunk，工具栏可对当前变更执行同一操作；设置区增加 Word wrap。补丁由完整旧/新文本生成，只包含该变更及最多 3 行上下文；写入继续经过固定参数、单文件单 hunk 校验、仓库状态复核和统一写队列。Discard Hunk 只改工作区，需确认。SSH 仓库通过同一固定补丁写入路径支持 Stage/Unstage/Discard Hunk；提交历史、部分/二进制预览保持只读。
- Commit 侧栏文件菜单补齐 Open Changes、Open File、Reveal in Finder、Copy Path 和 Copy Absolute Path；提交按钮下拉增加 Undo Last Commit（soft reset，保留改动在暂存区）；GitLens 风格菜单增加 Stash Changes、Stash Changes (Include Untracked)、Pop Latest Stash；Pull, Push 子菜单按 Sync、Pull、Pull (Rebase)、Pull from...、Push、Push to...、Fetch、Fetch (Prune)、Fetch From All Remotes 的顺序实现，并保留远端分支选择。
- Open in Editor 在当前窗口的内置编辑器中打开 Commit/Diff 面板所选文件的工作区版本，行为与 VS Code 从源代码管理打开文件一致；仅支持有效本地仓库中的普通文本文件（最大 8 MiB），校验路径和符号链接，避免打开仓库外文件。Terminal 使用系统 `/usr/bin/open` 和 Terminal 的固定 bundle identifier 请求打开本地仓库目录；是否按预期进入目录取决于系统应用行为，尚未实测。SSH 仓库不能调用这两个本地入口。

后续已准备文件历史与 Blame 的隔离仓库回归，以及多选的确认路径、拒绝危险组合和状态清理回归。`make build` 本机原生构建通过。按用户要求这些测试、静态检查和原生窗口/终端实测尚未运行；新功能、批量确认与丢弃边界、取消/断线、长路径、窗口布局及已有回归均须在隔离 fixtures 和真实窗口中验证。真实 SSH 只在授权环境验证；不将编译成功等同功能验收。CD003–CD007 仍未勾选。
