# 远程连接与目录选择（2026-10-28）

## 分支合并

将“添加远程 SSH 编辑器支持”聊天的独立 checkout 提交 bac1c30（Add GUI SSH password prompts）合入 orchestrator/phase0-validation，合并提交 38f282c。保留本聊天先前的远端 code 安装窗口登记修复：回调接受 host/path 并通过管理器锁更新窗口，再启动 bridge。合并前临时保存工作区及 index，合并后按原 staged/unstaged 状态恢复；未提交用户无关文件。

## 行为

- File > Open remote… 首屏仅选择主机并 Connect，登录成功再显示目录选择器，从 SFTP 服务返回的初始目录开始。
- 目录选择器具有子目录列表、Parent folder、绝对路径输入及 Go，保留 .code-workspace 选择。浏览使用独立 SFTP 会话，编辑文件仍限制在所打开的工作区根目录内；不把浏览会话作为编辑会话。
- Explorer 的 SSH 标记和 Open folder…、File > Switch remote folder…、远程模式 Cmd+O 重新打开选择器，复用已认证的浏览会话。File > Open local folder… 保留本地入口。
- 登录失败返回主机表单；目录不可读保留上一次列表与路径；取消选择不替换工作区。切换文件夹/Workspace 前阻止丢失未保存编辑，成功后关闭旧编辑标签，失败不清空当前标签。
- 应用关闭取消浏览上下文并关闭浏览会话；新的登录请求取代旧请求，旧结果不得覆盖当前状态。
- 修复 Explorer 构造远程文件引用时把父目录当作会话根的问题。现在沿用节点的工作区 root，因此 /root 下展开 source/src 后点击文件也能找到原 SFTP 会话。

## 验证

- 子目录回归测试在修复前复现 source/README.md 和 source/src/RedisConnection.php 点击失败；修复后根目录、一层、两层文件点击和保存均通过。
- 远程专项 race 测试覆盖主机首屏不要求目录、已连接目录浏览/失败回退/重复选择、登录失败返回主机表单、未保存编辑阻止切换。
- 合并后 cmd/git-viewer、askpass、remotecli、SSH、SFTP、bridge 的 race 测试通过；另增加真实 loopback SFTP 主目录发现和取消测试。
- 全量 local UI race 测试未全绿：输出包含 TestChangesContextMenuUsesFileOperations 的测试回调数据竞争，以及既有界面断言失败（如 TestCommitReadDoesNotFadeUnrelatedPanels 缺少 Fetch）。远程专项通过不代表全量测试通过；基线复核与原生交互结果见后续补记。

## 原生实测及基线复核

- go vet（cmd/git-viewer、local UI、sshfs）、MyGo vet、原生构建、darwin/amd64 应用打包及签名均通过。
- 将合并后的 HEAD（38f282c，不含本次目录选择器改动）导出到临时目录，复现 TestCommitReadDoesNotFadeUnrelatedPanels 缺少 Fetch 的失败；测试回调数据竞争单测重跑未稳定复现，不将其描述为已经修复。
- 在真实打包应用中通过 File > Open remote… 连接 n1：首屏只有主机，连接成功后显示 /root 的目录列表；Go 到 /root/source，打开文件夹，展开 src 并点击 RedisConnection.php，编辑器显示真实 PHP 内容（42 行）。随后通过侧栏 Open folder… 进入 src 并打开，Explorer 根切换为 /root/source/src，旧的干净标签关闭。
- 真实主机执行 code --reuse-window . 返回成功，沿用已建立的 bridge。真实文件只做读取，未保存或改写用户源码。
- 首次 GUI 连接出现一次 RemoteUnavailable，重试后完成以上操作；相同主机及 GUI 环境下的 SFTP 主目录发现、列表读取均成功，暂未稳定复现该瞬时失败。认证失败有可重试表单，不把成功打包等同于网络始终可用。
- 浏览选择器再次打开时跟随当前工作区路径（包括 code 命令切换目录后的路径）；不把之前一次浏览的位置误当作当前工作区。
