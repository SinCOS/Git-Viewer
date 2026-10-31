# 编辑器生产力与命令系统执行记录

日期：2026-10-31

本轮补齐五个 VS Code 对齐缺口：全局搜索、Explorer 文件 CRUD、脏文件关闭保护、集成终端、设置/快捷键/命令系统。应用仍为纯 Go + MyGo 原生 UI，不引入 WebView、HTML 或 JavaScript。

## 已实现

- **命令系统**：新增单一 `appCommand` 注册表，Quick Open、Settings 快捷键列表和窗口快捷键都由同一份命令定义驱动。快捷键采用有界语法（Cmd/Ctrl/Alt/Shift + 字母、数字或命名键），非法值在设置页标红并停止注册。
- **设置持久化**：`ui-prefs.json` 增加编辑器字号/行号/换行、搜索默认项、终端字号/高度及每个命令的快捷键。Settings 面板覆盖 Editor、Search、Terminal、Keyboard shortcuts，支持重置快捷键。
- **全局搜索**：搜索本地与 SSH Explorer 根，支持大小写、正则、全词、include/exclude glob、结果定位与替换。并发读取本地文件，结果上限可配置；超过 2 MiB、二进制或非普通文件跳过。Replace All 使用原子写，跳过有未保存缓冲区的打开文件。
- **文件 CRUD**：Explorer 工具栏、文件夹/文件右键菜单和命令面板支持 New File、New Folder、Rename、Duplicate、Delete、Copy Path、Reveal。本地和远程操作均限制在根目录；删除确认，路径拒绝 `..`、符号链接逃逸和根目录删除。远程 `RemoteFileSystem` 增加 Mkdir、Rename、Remove，并更新 SFTP 实现与测试替身。
- **脏文件关闭**：关闭脏标签或窗口时显示 Save / Discard / Cancel。窗口 `OnClose` 可取消关闭；逐个处理所有脏标签后才允许窗口关闭。本地保存改为保留权限的临时文件 + fsync + rename。
- **集成终端**：接入 MyGo terminal 插件，在底部显示本地登录 shell，支持启动失败提示、重试、重启、字号和面板高度。远程终端不在本轮范围，远程工作区下回退到本机 home。

## 验证

- `go test ./internal/ui/local`：通过。
- `go test -race ./internal/ui/local`：通过。
- 新增回归覆盖快捷键解析、命令注册表唯一性、搜索全词/glob、原子写权限、本地路径逃逸、脏关闭保存/丢弃、设置持久化、远程 CRUD 调用和 Explorer 右键菜单不吞左键。
- 修正旧测试中与当前 UI 标签/菜单不一致的断言，并消除上下文菜单测试的变量竞争。
- 全仓 `go test ./...`、`go test -race ./...`、`go vet ./...`、`go build -o bin/git-viewer ./cmd/git-viewer` 在交付前执行。

## 边界

- 集成终端只验证编译与无窗口回退；真实 PTY/Ghostty 窗口交互、IME、剪贴板和全屏 TUI 仍需原生窗口实测。
- 远程终端、搜索取消后的增量 UI 更新、跨设备大文件索引和符号链接索引未实现。
- Replace All 只处理当前结果上限内的文件；有未保存缓冲区的文件会明确跳过。
