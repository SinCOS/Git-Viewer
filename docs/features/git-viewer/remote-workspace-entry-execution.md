# 远程 Workspace 入口修复

## 根因

“Open workspace…” 走的是 `ChooseWorkspaceFile`，它只调用 `mygo.Dialog.Open` 的原生文件面板。原生面板看不到远端文件系统，所以即使当前窗口已经是 SSH 会话，选出来的也只能是本地路径；远端 `.code-workspace` 完全无法进入这条入口。

功能本身早已存在：`OpenRemoteWorkspace`、`ssh-workspace` 最近记录、CLI `--ssh-host HOST --workspace PATH`、深链 `gitviewer://workspace?host=…` 都能打开远端 workspace，缺的只是从菜单/命令面板到远端选择器的路径。另外远端选择器的主按钮只在路径以 `.code-workspace` 结尾时才叫 “Open workspace”，一旦选中普通目录就没有任何方式把它按 workspace 打开。

## 行为

- `ChooseWorkspaceFile` 分流：当前会话是远端（`SSHHost` 非空或已有远端选择器）时直接打开远端选择器，不再弹本地面板；纯本地会话保持原来的原生面板。
- 新增 `OpenRemoteWorkspacePicker`：有 SSH 会话就复用，否则用已配置的 host 直接连接，连接失败/无 host 时才落到远端面板让用户选 host。切换前检查脏编辑器标签，避免覆盖未保存内容。
- 新增菜单项 “Open workspace on SSH host…” 与命令面板项 “Open Workspace on SSH Host…”，任何状态下都能显式进入远端入口。
- 远端选择器在选中普通目录时额外提供 “Open workspace” 按钮（主按钮仍是 “Open folder”），对应 `submitRemoteWorkspace`：走同一条 `openRemoteWorkspace` 流程，不复制打开逻辑。路径本身是 `.code-workspace` 时主按钮改成 “Open workspace”，不再重复显示两个同样的按钮。
- 未改动：本地 Open workspace、保存 workspace、远端 workspace 的读写与多根发现逻辑。

## 操作日志

1. 阅读 `ChooseWorkspaceFile`、`remote_browser.go`、`remote_dialog.go`、`remote_editor.go` 与 window manager 的 target 路由，确认远端 workspace 能力存在但入口缺失。
2. 改 `dialogs.go`：远端会话分流、脏标签检查、`OpenRemoteWorkspacePicker`；本地分支保留原生面板。
3. 远端选择器增加 “Open workspace” 次按钮（仅目录时出现）并抽出 `submitRemoteWorkspace`。
4. 菜单与命令面板各加一项显式远端入口。
5. 补三组回归：远端会话不再弹本地面板、本地会话仍走原生面板、目录选择时出现 “Open workspace”。
6. `make fmt lint`、`go test ./...`、原生构建与 `-smoke` 通过。

## 验证

```sh
.tools/go/bin/go test ./internal/ui/local -run 'TestRemote|TestOpenWorkspace|TestLocalWorkspace|TestNativeMenu|TestWorkspace' -count=1
.tools/go/bin/go test ./...
make fmt lint
make build
./bin/git-viewer -smoke
```

## 未验证

- 没有真实 SSH 主机可用于端到端点击验收；“远端会话分流”的回归只断言进入了连接流程（无可用 host 时连接失败是预期结果），`OpenRemoteWorkspace` 的 SFTP 读取沿用既有已测路径。
- 远端选择器里 “Open workspace” 按路径直接打开，若该文件不是合法 JSONC workspace，错误仍由 `service.OpenRemoteWorkspace` 上报，UI 不额外预校验。
