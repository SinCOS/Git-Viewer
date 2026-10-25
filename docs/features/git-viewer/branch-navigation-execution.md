# UL019 Git Log 分支导航树执行记录

## 范围与结果

按用户提供的分支树截图，在 Git Log 的模式导航与提交列表之间加入 MyGo 原生分支导航。应用入口仍为纯 Go/native UI。

- 顶部独立分支搜索、All Branches 与 HEAD 入口。搜索按不区分大小写的分支完整路径匹配，保留祖先目录，临时展开匹配路径，清空后保留原折叠状态。
- Local / Remote 分组，远程按 remote 再按 `/` 分层，本地同样按 `/` 分层。目录优先、名称排序，排除 `origin/HEAD` 等符号引用；目录用文件夹图标，普通分支用 ref 图标，当前分支用金色星标与粗体。
- 虚拟列表独立滚动；行高 30，长名称与跟踪信息单行省略，悬停提供完整 ref/跟踪信息。选中行使用主题 Surface 与当前分支星标区分。
- 左侧默认 280 px，可拖动分隔线，保存宽度至现有 `ui-prefs.json`。分支搜索/折叠/选择只保留当前仓库会话，打开另一仓库时重置。
- 当前分支的跟踪信息来自 `for-each-ref` 的 upstream 与 upstream:track，展示 ↑领先、↓落后、gone；数量基于已有远程跟踪引用，不自动 fetch。
- 移除旧分支下拉框与 Show Remote Branches 复选框，历史顶部显示所选范围并保留提交搜索。远程标签正常展示。

## 历史读取与安全

旧实现只对已加载的全部历史页计算分支可达性，较旧分支的 tip 在首屏外时会得到空结果。现在首次加载所选分支时，先刷新分支列表，以完整 ref 精确匹配获取 OID，再复用只允许合法 OID 的 History 分页。分页固定首次 OID；分支后续移动不改变已有分页位置。刷新重新解析当前引用，所选分支已删除时回退 All Branches。

HEAD 通过固定 `rev-parse --verify HEAD` 解析，支持 detached HEAD；未出生分支返回空历史。加载期间禁用分支切换，通过现有 generation/cancellation 机制避免旧请求跨仓库覆盖结果。切换浏览范围时清空旧提交详情与选择。单击分支不执行 checkout 或其他 Git 写入。

本地与 SSH 共用 ReaderFor 返回的 LocalReader 与现有受控 Transport；新增读取仅使用固定 argv。BranchRecord 是内部读取结果，不修改 GitOperation/公开 JSON DTO。UI 偏好新增可选 `branchTreeWidth`，旧配置缺失时使用默认值。

## 验证

实际通过：

- `make check`：Go 格式、`go vet ./...`、`go tool mygo vet`、全量 `go test -race -count=1 ./...`、`CGO_ENABLED=0` 原生构建。
- 分支树模型与 UI：多个 remote、多级分组、符号引用隐藏、大小写搜索、祖先路径保留、搜索后恢复折叠、10,000 分支虚拟化。
- 隔离 Git fixture：首屏 100 个提交外的旧本地/远程分支可以直接打开；浏览不改变 HEAD；分页期间新增提交不漂移；HEAD 重新解析；删除所选分支后刷新回退；跨仓库清理状态。
- 跟踪状态读取：ahead、gone、上游名称、symbolic ref；HEAD 空仓库与 detached 状态。
- 1100 / 1400 宽度下分支树与日志左右排列、详情开关、模式隔离；分支面板宽度持久化 round-trip。
- 原生启动冒烟：临时仓库含 main、feat/local、origin/feat/delivery，隔离配置以 log 模式打开；输出 `native local repository window created`、`native data main-thread update passed`、`background workers stopped`。

验证中还发现原有 TestEmptyAndConflict 仍查找已移除的 `Conflicted: 1` 文本。在 `git archive HEAD` 导出的未修改基线中复现同一失败后，修正为验证现有 `Merge Changes` 标题、独立计数与 Conflicted 数据分组。未更改冲突业务行为。

## 验收步骤与边界

1. 打开有本地和远程跟踪分支的仓库，进入 Git Log，检查左侧树、星标与跟踪信息。
2. 展开目录并搜索分支完整路径的一部分，清空搜索确认折叠恢复；选择旧分支检查提交范围，确认实际工作分支未改变。
3. 选择 HEAD/All Branches，加载更多；拖动左侧分隔线并重启，检查宽度恢复。
4. 打开详情并缩放窗口，检查树、提交列表和详情相邻排列。

原生冒烟只验证窗口创建、数据更新与退出，不等同于人工外观/鼠标键盘操作验收。远程读取由现有 SSH fixture 回归覆盖，未连接真实远程主机；本轮未做双架构交叉构建或 arm64 真机验证。提交搜索仍明确限于已加载提交。Tags、收藏、右键分支管理不在本轮范围。
