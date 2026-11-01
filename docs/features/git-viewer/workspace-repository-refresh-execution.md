# Workspace 仓库切换与 Git 面板刷新修复

## 根因和行为

本地 OpenWorkspace/saveWorkspace 只更新 Workspace 元数据，再等所有仓库计数读取结束；旧 openFirstWorkspaceRepository 在已有活动仓库时直接返回，导致 Git Log/Commit 继续绑定旧仓库。多仓库虽有发现与去重，没有可见的选择入口。

现在 Workspace 加载时先选定活动仓库、读取其 Status，再在同一次 UI 更新发布 Workspace/Repository/Status，并清理历史、详情、文件选择与预览。原活动仓库仍是成员时保留并刷新；否则选择第一项；没有 Git 仓库则清空，停止旧监听。跨仓库清除提交草稿和 Amend，防止写入错库。

Git Log 和 Commit 侧栏增加共用仓库选择器（多仓库时显示），菜单以完整路径区分同名根。通过已验证 RepositoryContext 切换，Status 走 ReaderFor，本地/SSH 共用规则；SSH 失败不回退本地读取。写操作继续只作用于当前仓库，未增加多仓库批量写入。

## 操作日志

1. 阅读 Workspace 打开/保存、远程 Workspace、监听、仓库面板及已有测试；确认旧仓库保护条件与缺失入口。
2. 先写新 Workspace 替换旧面板、非 Git 清空、双仓库选择三项回归，观察预期失败。
3. 用 workspaceRepository/workspaceGit/installWorkspaceGit 收敛选库及状态发布；移除依赖计数完成后再开库的旧逻辑。后台计数保留为辅助数据，不决定活动库。
4. 本地/远端 Workspace 打开和保存共用发布逻辑；监听切到新库，无库时不启动 watcher。跨库清除提交草稿，同库重载保留草稿。
5. 增加暂存只写所选仓库、保存移除活动根后回退、旧结果不能覆盖新 Workspace、SSH 失败不读取本地，以及 Commit 选择器可见性验证。
6. 相关普通测试、race、Go/MyGo vet、差异检查、本机构建及 Intel 打包通过。更新任务和记录，本地提交，不推送。

## 验证

```sh
.tools/go/bin/go test ./internal/ui/local -run 'TestWorkspace|TestOpenWorkspace|TestRemoteWorkspace|TestRemoteFolderOpened' -count=1 -timeout=35s
.tools/go/bin/go test -race ./internal/ui/local -run 'TestWorkspace|TestOpenWorkspace|TestRemoteWorkspace|TestRemoteFolderOpened|TestLateWorkspace|TestRepositorySwitch|TestLateResults' -count=1 -timeout=40s
.tools/go/bin/go vet ./...
.tools/go/bin/go tool mygo vet
git diff --check
make build
.tools/go/bin/go tool mygo build -platform darwin/amd64 -skip-dmg
```

本轮按修改范围验证，没有重复全量测试。未进行物理 GUI 或真实远程 Workspace 验收；SSH 边界测试使用失败执行器验证错误路由。多仓库按 Workspace 每个根发现其所属仓库，不新增递归查找根目录中所有嵌套 Git 仓库。当前只监听活动仓库，其他库切换时即时读取，未声称全部仓库实时监听。crapper/dryer/mutator 当前不可用，未宣称完成工具指标检查。
