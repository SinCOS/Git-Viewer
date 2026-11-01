# Source Control 与 VS Code 对齐执行记录

用户要求截图中的 Source Control 功能和排版与 VS Code 对齐，遵循 ub-mode；以本机 `/Applications/Visual Studio Code.app/Contents/Resources/app/extensions/git/package.json` 的内置 Git 菜单及默认配置为参考。

## 操作与原因

1. 读取用户截图、项目约束、提交表单、变化列表、快捷键、既有交互测试和工作区状态；保存本轮修改前文本，以便提交仅包含本轮差异。保留原有 UI/diff/终端及已暂存改动。
2. 核对 VS Code：Git 默认混合显示未跟踪文件，行内提供暂存/取消暂存/丢弃操作。围绕紧凑表单、Staged 树形视图和直接文件操作先写行为测试；三项均按预期失败。
3. 提交框由 84px 调至 52px，显式覆盖 MyGo TextArea 的默认最小高度。表单边距 12px；提交按钮加入图标；Amend 移到 Commit actions 与标题菜单，Reset form 可退出模式。保留提交说明、错误与写入禁用状态；长消息可在原生多行输入框内滚动。
4. 文件行移除暂存复选框及其可变状态映射，直接使用 + / − 和丢弃按钮；更多操作继续从右键菜单访问。保持 Git index 为事实来源，写入继续走受控流程。未跟踪文件不提供现有 tracked-file writer 无法处理的删除操作。
5. Staged/Changes 共用树形层级，分别维护折叠状态；列表模式显示父目录以区分同名文件。分组增加数量 badge，操作靠右，去掉 Staged 下无效的丢弃按钮和跨分组的 Stage all。分组头去除横向 Scroll 容器，避免背景仅铺到内容宽度。
6. 为消息框 ⌘Enter 先写真实隔离 Git fixture 测试并见其失败，再补焦点内快捷提交；普通 Enter 保持换行，不可提交状态不能被快捷键绕过。
7. 运行真实 Git fixture 验证目标文件暂存、取消暂存与提交排除未选文件；验证丢弃确认捕获点击行路径，不使用另一条选中行。更新旧复选框/更多按钮测试为新交互。
8. 导出浅色/深色、300px/420px 宽度的确定性 renderer 截图并检查；400px/850px 窗口高度的布局测试通过。截图为程序渲染证据，不宣称物理 GUI 点击验收。
9. 全量测试、针对性 race、Go vet、MyGo vet、差异检查、本机 build 和 Intel 打包通过。按本轮差异提交本地 Git，不推送远端。

## 验证

```sh
.tools/go/bin/go test ./... -count=1 -timeout=60s
.tools/go/bin/go test -race ./internal/ui/local -run 'TestSourceControl|TestChanges|TestGroupedTree|TestInlineStage|TestCommitForm|TestOperations|TestMultiRepository|TestNativeWriteHistoryWorkspaceFlow' -count=1 -timeout=45s
.tools/go/bin/go vet ./...
.tools/go/bin/go tool mygo vet
git diff --check
make build
.tools/go/bin/go tool mygo build -platform darwin/amd64 -skip-dmg
GITVIEWER_CAPTURE_SCM=/tmp/scm-alignment-evidence .tools/go/bin/go test ./internal/ui/local -run '^TestCaptureSourceControlAlignment$' -count=1
```

渲染证据：[浅色窄栏](evidence/source-control-alignment/source-control-light-narrow.png)、[深色窄栏](evidence/source-control-alignment/source-control-dark-narrow.png)、[浅色宽栏](evidence/source-control-alignment/source-control-light.png)、[深色宽栏](evidence/source-control-alignment/source-control-dark.png)。

## 边界

- 本轮对齐截图所在的提交/变化区域；标题中已有 GitLens 扩展入口及原有不支持菜单项保留，未声称复制 VS Code 的全部 Git/扩展能力。
- 文件操作按钮保持可见以兼顾原生 UI 可发现性；未实现 VS Code 的所有 hover、主题与用户配置组合。
- 未运行真实 SSH 主机或物理输入验收；所有产品 Git 写入均在临时 fixture 内完成。
- crapper、dryer、mutator 在当前工具集合和本机 PATH 中均未提供；没有运行这些工具，也没有作 CRAP/重复率/变异覆盖达标声明。已删除复选框冗余状态并人工核对本轮重复决策。
- 完整测试与构建针对当前工作区。额外用临时 index 导出本轮提交候选作隔离检查时，发现原 HEAD 的 `git/lists.go` 调用的 `parseBranches/parseTags/parseStashes` 定义仍在原有未提交 `history.go` 中；隔离检出因此在基础 Git 包编译阶段失败。该既有提交依赖不在本轮 UI 改动中修复，未把不相关 history 改动纳入；临时检查目录与 index 在记录后删除。
- 应用包位于 `bin/package/darwin-amd64/Git Viewer.app`，构建与签名步骤通过，不等同正式分发验收。
