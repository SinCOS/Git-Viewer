# 01 — Git Viewer 需求规格说明

版本：1.0 | 状态：已确认设计方向，细节待确认

## 1. 目标与决策
- 【原文明确】桌面 Git Viewer：本地 Git 仓库、SSH 远程仓库、Status、Diff、Commit History、Branch/Worktree、Git Graph、Stage/Commit、Push/Pull/Fetch。
- 【原文明确】首发 macOS；SSH 使用系统 OpenSSH；写入采用安全模式；本地监听、远程轮询的混合刷新。
- 【建议】首版只管理已有仓库，不提供完整代码编辑器、仓库托管、服务器文件浏览器、强制推送、hard reset、清理未跟踪文件。
- 【待确认】最低 macOS 版本、目标 CPU 架构（Intel/Apple Silicon）、发布签名/公证方式、是否支持密码交互认证。

## 2. 用户故事及验收
### REQ-01 本地仓库（P0）
- 选择本地路径并验证是 Git 仓库（支持子目录定位根目录）；记住最近仓库。
- 验收：普通仓库、裸仓库（应提示不支持工作区操作）、无效路径、路径包含空格/中文均正确处理。
### REQ-02 SSH 仓库（P0）
- 使用 ~/.ssh/config Host 别名、系统密钥及 ssh-agent 连接；输入远程仓库绝对路径并验证 Git 环境。
- 验收：连接超时、认证失败、主机密钥不匹配、Git 不存在、目录无权限都有明确错误；不得自动忽略 host key 校验。
### REQ-03 Status（P0）
- 区分 staged、unstaged、untracked、conflicted；显示重命名和删除；支持刷新。
- 验收：文件同时已暂存又有新修改时分别展示两类状态；路径包含换行也能解析。
### REQ-04 Diff（P0）
- 显示工作区相对索引、索引相对 HEAD、指定提交差异；行级标识、文件名、二进制占位、过大 Diff 提示。
- 验收：新增/删除/重命名、无 HEAD 的初始仓库、二进制文件不会崩溃。
### REQ-05 History（P0）
- 按批分页展示 commit hash、作者、时间、subject；选择提交展示详情和变更文件。
- 验收：包含 merge commit；大历史分页不阻塞 UI。
### REQ-06 Branch/Worktree（P1）
- 展示本地/远程分支、HEAD、上游追踪；Worktree 路径、分支、锁定和 detached 状态；安全切换分支。
- 验收：切换前检测未提交变更、分支被其他 worktree 占用，危险情形拒绝或要求用户明确选择安全处理方式。
### REQ-07 Graph（P1）
- 基于 commit parents 构建 DAG；展示分叉、合并、refs、当前 HEAD；增量加载。
- 验收：octopus merge、多根历史、孤立分支、detached HEAD 可展示。
### REQ-08 Stage/Commit（P1）
- 按文件暂存/取消暂存，填写提交消息，提交当前索引内容；展示 Git hook 错误。
- 验收：不意外暂存其它文件；commit 失败不宣称成功；初始提交可用。
- 【建议】首版不做行级 stage、amend、签名设置界面。
### REQ-09 Fetch/Pull/Push（P2）
- 显示上游、执行 fetch、pull、push；进度/错误信息；阻止默认 force push。
- 验收：无上游、拒绝推送、认证失败、网络中断、merge/rebase 冲突有可解释反馈；不自动丢弃改动。
- 【建议】pull 默认仅允许 fast-forward (`--ff-only`)；不自动设置 upstream。

## 3. 非功能需求
- 安全：只允许经过校验的 Git 操作；禁止 Shell 字符串直接拼接不可信路径；SSH 严格校验主机密钥；不保存明文凭据；日志脱敏。
- 性能：【建议】历史分页默认 100 条；单次 Diff 软限制 2 MiB；远程轮询前台 15 秒、后台 60 秒，失败指数退避；这些阈值均应可配置并经性能测试调整。
- 一致性：Git 仓库是真实来源；操作完成后重新读取；SSH 离线时显式标注缓存过期。
- 可访问性：键盘导航、文本可复制、颜色之外辅以状态文字。

## 4. 待确认清单
- 【待确认】分支是否允许创建/删除；Worktree 是否仅查看还是允许新增/移除（当前按仅查看设计）。
- 【待确认】远程交互式 SSH 密码、2FA 与 Git credential helper 的支持范围。
- 【待确认】是否允许创建新仓库、clone；当前不包含。
- 【待确认】pull 遇到非快进时提供哪些显式后续操作；当前仅提示并中止。


## REQ-10 VS Code Workspace（P1）
【原文明确】支持读取、创建、保存 `.code-workspace` 文件；支持从 Workspace 中识别多个本地 Git 仓库。

【建议】
- 打开已有 `.code-workspace`：支持 VS Code 常见 JSONC（注释、尾随逗号）；解析 `folders[].path`、`folders[].name`，保留 `settings`、`extensions`、`launch`、`tasks` 及未知字段。
- 路径基于 Workspace 文件所在目录解析；支持相对与绝对路径；目录不存在时显示诊断，不阻止打开其他目录。
- 创建 Workspace：选择多个本地目录，指定文件名及保存路径，生成 VS Code 可识别的 `.code-workspace`。
- 保存：新增/调整目录条目时保留原有配置；保存前检查文件是否已被外部修改；冲突时禁止静默覆盖，提示重新加载或另存为。
- Workspace 与 Repository 解耦：同一目录可以被多个 Workspace 引用；同一 Git 仓库可由多个子目录指向，列表需按规范化仓库根目录去重，并保留 folder 归属。
- SSH 远程仓库仍由 Git Viewer 自身配置管理；本阶段不往标准 `folders` 中写入非标准 SSH URI，不将 `gitViewer` 字段写进 VS Code 文件。
- 保存应使用同目录临时文件 + 原子替换；读取或解析失败时不写回。新建文件使用标准 JSON；编辑已有 JSONC 应尽可能进行最小文本编辑，保留注释、字段顺序及未知内容，不能做到无损时提示用户，不静默整体重排。

验收：A. 带注释/尾逗号/未知字段的现有 Workspace 可打开；B. 创建文件可被 VS Code 打开；C. 保存后原 settings/extensions 和注释不丢失；D. 相对路径正确；E. 外部修改冲突不覆盖；F. 缺失目录和非 Git 目录显示可理解状态。

【待确认】是否支持 UI 删除文件夹条目、重排顺序和“另存为”；第一版建议允许添加/移除条目并提供另存为，但不删除磁盘上的实际文件夹。

## v1.2 框架决策
【原文明确】Git Viewer 使用 egoist/mygo；保留 macOS 优先、系统 OpenSSH、safe 写操作、hybrid 刷新及 VS Code Workspace 读取/创建/保存。
【建议】采用 MyGo WebView + Vue 3；不改变已有 Git 功能验收标准。


## v1.3 原生 UI 决策
- 【原文明确】桌面框架使用 `egoist/mygo` 的 **纯 Go 原生 UI**，不使用 WebView、Vue、TypeScript、HTML 或 JavaScript。
- 【建议】界面参考 IDEA Git 工具窗口，采用原生可调整三栏布局：仓库树/Worktree、Graph/History、Diff/Changes；Git 写入功能遵循 safe 模式。
- 【待确认】是否要求首版原生 Diff 具备语法高亮、行内差异和虚拟滚动的全部高级体验；暂按基本高亮与可滚动文本验收。
