# 三模式工作区布局（用户 ASCII 草图）

## Diff Preview 配色

参考 VS Code 的差异编辑器层级：新增行使用柔和浅绿底、删除行使用浅红底，正文使用主题正常文字色，替换此前整行亮蓝/红字；hunk 使用低饱和蓝底，文件头/元信息弱化。暗色主题使用对应的深绿、深红、深蓝底。行背景占满列表宽度，等宽正文调为 12px / 22px 行高。仅执行 5 项 Diff/只读交互相关检查（空态、提交详情与复制、查找、长行虚拟化、真实差异选择），均通过；原生构建和 diff 空白检查通过。未跑全量测试，未进行两主题实机视觉验收。


## 行内暂存与文件夹图标

后续调整：行内暂存图标使用“文件＋”轮廓、取消暂存使用“文件−”，并使用无边框/无底色的 ButtonBase，避免单独 SVG 加号仍像原来的加号按钮。保留 28×28 点击区域和操作提示；3 项相关操作/选择检查与构建通过，未跑全量测试。

按截图反馈，文件行的暂存/取消暂存改用 SVG 加号/减号图标按钮，沿用统一居中构造及既有确认流程；调整行内边距确保按钮不超出行高。Tree 目录行增加文件夹 SVG，并随折叠状态显示关闭/打开样式，整行保持可点击折叠。仅运行 6 项相关 UI 检查（行内操作、目录折叠、选择映射与布局），全部通过，原生构建与 diff 空白检查通过，未跑全量测试。


## Local Changes 折叠与同步图标

- Local Changes 标题、状态组标题与 Tree 目录均可点击折叠/展开；箭头跟随状态改变。折叠状态互相独立，同一路径在不同状态组中分别保存。
- 折叠只控制可见行，保留原始选中文件与 Git 暂存状态。Tree 模式每个目录只建一个节点，避免根目录文件与子目录交错时产生重复节点。
- Fetch、Pull、Push 换为云端下载、向下收取、向上推送三个 SVG 图标，保留 Tooltip、可访问名称、禁用条件及写入确认；所有图标共用清零内边距并居中的按钮构造。
- 仅运行 8 项相关 UI race 检查（新增整块/分组/目录折叠、同名目录独立折叠和根目录去重；既有选择、暂存、按钮状态与位置），均通过。六个图标按钮均验证尺寸与居中位置；局部 Go vet、MyGo vet、原生构建及 diff 空白检查通过。未跑全量测试，未新增 GUI 实机外观验收。


## 图标按钮与 Git Log 宽度修复

后续截图发现图标偏右并裁切：28×28 按钮继承了文字按钮默认内边距。现显式清除 Padding/Gap，设置水平/垂直居中并固定图标尺寸。两种窗口宽度下验证三个按钮均为 28×28、图标为 16×16，四边各留 6 像素；布局与目录切换相关检查和构建通过，未跑全量测试。

根据截图反馈，将 Local Changes 的 List、Tree、刷新操作改为 28×28 的原生 SVG 图标按钮，保留可访问名称、Tooltip 与 List/Tree 当前状态颜色。

Git Log 原先在横向滚动容器里把列表宽度固定为 `laneCount * 16 + 460`，导致面板放大后选中背景、滚动条与提交行仍只占原宽度。现改为直接由虚拟列表占满可用宽度；提交行 FillWidth，中间标题可伸缩并裁切，图形 lane 和右侧 revision/date 保持固定宽度。长标题保留全文 Tooltip。

验证：相关 UI 检查通过（图标切换、文件选择、搜索选择、一万条历史虚拟化、面板位置及新增长标题/长分支名称的 500→1000→700 宽度往返测试）；Graph 命中/merge 渲染检查、局部 Go vet、MyGo vet 与原生构建通过。未跑全量测试，未新增 GUI 截图或物理输入验收。


## VS Code 风格原生实施（HTML 预览后）

本轮按用户“开始吧，不执行全量测试”落实原生 UI，未改变 Go/MyGo 运行入口或 Git DTO。

- Local Changes 改为虚拟分组列表：Merge Changes、Staged Changes、Changes、Untracked Files 与计数；文件名突出、目录弱化、右侧显示状态字母和 + / − 操作，保留 List / Tree。普通路径不显示引号，控制字符路径继续安全转义，完整路径和重命名来源通过提示展示。
- 暂存/取消暂存继续使用已有确认、精确路径和写入复核。新增独立列表行索引，避免组标题/目录行导致文件选择偏移；相同目录在不同组使用不同虚拟 key，键盘可跨越标题选择下个文件。
- Commit 页移除提交 ID 输入与 View commit diff 按钮；差异面板使用 Diff Preview 标题和横排 Copy / Find。底部显示 staged 数量，无暂存文件或消息为空时禁用提交按钮。
- Git Log 首次进入自动读取历史与分支；等待已有操作完成后加载，缓存已读取的数据。失败/取消与空历史均不在重绘时循环重试，用户可使用 Refresh history 重试；读取完成不抢回已切换离开的页面。
- Git Log 增加已加载提交的本地搜索，并正确将过滤行映射回原始提交。保持虚拟列表与分页，提交行区分标题、作者、分支、短哈希和日期；详情按标题、作者、时间、短哈希、消息及文件清单组织。
- 历史区单独使用宽度状态，避免与 Commit 的 Local Changes 宽度相互挤占。Fetch / Pull / Push 继续在顶栏常驻。

验证：仅运行相关 UI 测试（包含新增自动加载、空仓库/失败无重试循环、搜索选择映射、双区同名目录与键盘跨组导航，以及已有一万条历史和长 Diff 虚拟化等）并开启 race，全部通过；`go vet ./internal/ui/local`、MyGo vet、原生构建及 `bin/git-viewer -smoke` 通过。**未执行全量测试或 make check。** 原生 smoke 证明本机窗口创建、Status 主线程更新和任务退出，不代表两页完整视觉或物理输入验收；未新增跨架构证据。


本轮以用户提供的 Commit Workspace、Git / Log、Worktrees 三张 ASCII 草图为布局依据。沿用纯 Go / MyGo native ui 与已有 Git 服务，未修改 JSON DTO/schema。

## 已实现

- 最左侧常驻 Commit / Git Log / Worktree 导航；在挂载内容前同步模式，避免从旧模式先挂载侧栏。
- 顶部显示 Workspace 文件名、Repository 名称和当前分支三个菜单。Workspace 管理单独放入 Workspace 菜单，最近仓库与路径打开位于 Repository 菜单。路径、SSH 连接和同步操作保留在 Connection / Git 菜单中，避免所有操作横向挤占主窗口。
- Commit：Local Changes 与 Diff 横向分栏，Commit Message 表单横跨两栏并置于底部；保留 Stage/Unstage、列表/树、Diff 查找与复制。
- Git Log：左侧 Local/Remote 分支，中间虚拟历史图，右侧 Changed Files / Commit Details / 提交 Diff。分支加载可保留当前页面；提交详情不再在历史与 Diff 中重复显示。
- Worktrees：列表与详情两栏，点击条目改变详情；显示 Branch、Path、HEAD、detached、locked、prunable 信息。Open 打开选定的本地或 SSH worktree 并读取其状态。
- Commit and Push：修复原先只提交的行为；首次确认提交，只有提交成功且完成状态复核后，才单独确认 Push。失败或取消提交均不进入 Push。取消 Push 保留已创建的本地提交。

## 验证

- `go test ./internal/ui/local` 通过。
- `make check` 通过：格式、Go vet、MyGo vet、全项目 race 测试及原生二进制构建。
- `./bin/git-viewer -smoke` 通过：本机原生窗口创建、数据主线程更新与后台任务退出。
- `layout_test.go` 使用 ui.Tester 在 1100×850、1400×850 下检查导航/文件/Diff 的左右位置、提交表单位于下方、Log 三栏位置及 Worktree 两栏位置；检查非当前模式的表单不泄漏。
- Worktree mock 交互测试验证选中第二项后 Branch/HEAD 随选择更新。
- 保留一万条历史记录的虚拟化测试。
- 隔离本地仓库和 bare 测试远端验证 Commit and Push 的成功、取消提交、取消 Push、提交失败四条路径；仅成功路径更新测试远端。未对项目仓库执行产品写入或推送。

## 草图中尚未完成的功能与验证边界

- `+ Add Worktree`、`Remove` 为禁用按钮并附功能尚未提供的说明；现有受控 GitOperation 不支持创建/删除 worktree。本轮未新增对应写入协议。未提供与 Open 重复的 Switch 按钮。
- 其他 worktree 的修改文件数量未额外读取，详情明确提示打开该 worktree 后加载状态；detached、locked、prunable 来自已读取的 Git 数据。
- 草图中的 ↑/↓ 数量尚无对应数据，本轮未显示模拟数字。
- 本机原生 smoke 只覆盖窗口、Status 更新及退出，不代表三页外观逐像素验收、真实鼠标/IME 操作或另一架构真机验证。未新增截图或交叉构建证据。

复现：`make run`，从最左侧切换三模式；打开本地仓库后在 Log 使用 Load history/Load branches，在 Worktree 使用 Load branches 读取列表并选择条目。Workspace 与 SSH 控件分别位于顶部对应菜单。

## HTML 样式预览（原生界面实施前）

用户要求先看 HTML 样式稿。新增独立 [预览文件](evidence/ui-workspaces-preview.html)，不导入原生应用入口，不改变 Go/MyGo 运行流程。

- 轻色 VS Code 风格：Staged Changes / Changes 分组、文件名和弱化目录、状态字母与行内暂存操作；Commit 页面不展示提交 ID 输入框。
- Git Log 用分支树、紧凑的提交图列表与提交详情三栏；首次进入自动展示模拟加载态和历史，支持搜索及选择提交。
- HTML 中的暂存、差异、历史和 Git 操作均为演示；尚未将本轮样式和自动加载逻辑实施到原生 UI。
- 仅检查 HTML ID 唯一性、必需区域及 `node --check` 脚本语法，通过；未跑全量测试，未进行浏览器实际渲染验收。

## 左侧导航与同步按钮修正

根据用户截图，Sidebar 原先只取内容高度，在父 Row 中垂直居中；120 宽度也使 Commit 与计数同时显示时截断。

- 导航增加 FillHeight，撑满主体左栏，并将宽度调为 180，条目从顶部排列。
- Fetch、Pull (fast-forward)、Push 恢复为顶栏常驻按钮，继续使用已有禁用条件与确认流程；连接菜单保留路径、SSH 和视图加载操作。
- 用户要求不跑全量测试，本次仅执行 `TestWorkspacePanelPositions`、`TestModeRailSwitchesPanes`、`TestOperationsLayoutLabels`、`TestOperationsDisabledWhileLoading`，全部通过；原生二进制重建通过。
- 布局检查增加导航与主体的 X/Y/高度一致断言，以及同步按钮无需展开菜单即出现在主体上方的断言。本次未运行全量测试或 GUI 实机验收。
