# 远端 code 安装提示修复（2026-10-28）

从本地窗口的 File > Open remote… 打开远程目标时，local.App 已更新 SSHHost/SSHPath，但 windowManager 的 windowState.host/path 未更新。OnRemoteOpened 启动 ensureHost 后，ensureRemoteCLI 使用 mostRecent(host) 查找窗口，得到 nil 并提前返回，因此不会弹出安装提示。命令行指定远程目标会预先登记窗口位置，不受同一遗漏影响。

OnRemoteOpened 现在先在管理器锁内登记已成功打开的主机和路径，再启动 bridge 与安装检查。远程目录、文件、Workspace 和切换主机共用这一通知流程；登记同时恢复远程窗口匹配及复用。

新增 TestRemoteOpenedMakesGUIWindowEligibleForCLIInstall：模拟无远程目标的本地窗口，登记 GUI 打开的目标后通过真实 ensureRemoteCLI 和隔离 fake SSH 验证安装确认回调被调用；确认取消后不执行安装。另验证远程窗口路径匹配和切换主机后不再匹配旧主机。

用户主机 n1 的临时处理：通过系统 SSH 执行项目现有安装脚本，将 code 与 gitviewer 安装到 /root/.local/bin，记录 host=n1。远端 sh -n、可执行权限和加入 PATH 后的命令查找已通过。未改远端 shell 配置；新终端可执行 export PATH="$HOME/.local/bin:$PATH" 后使用 code。

验证通过：cmd/git-viewer 与 internal/remotecli 的 race 测试、cmd/git-viewer 的 go vet，以及 CGO_ENABLED=0 原生入口构建。使用构建后的 bin/git-viewer -ssh-host n1 -remote-repo /root -auto-refresh=false -smoke 连接真实主机，原生窗口主线程数据更新及关闭通过。真实 GUI 安装弹窗及用户终端 code . 唤起窗口仍需单独验收；命令行远程窗口冒烟不代表已完成该交互验收。
