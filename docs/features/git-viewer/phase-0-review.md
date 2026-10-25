# Phase 0 执行前核对记录

状态：历史执行前核对，记录当时的阻塞；随后用户授权补齐，已建立约束/工具链并完成 T001–T005，当前状态见 [执行记录](phase-0-execution.md) 和 05-tasks.md。以下保留原始发现及建议作为审计证据，不替代 04 的 Implementation Plan。

## 现有目录与环境

- 已阅读 README、01–05 全部规划文档、schemas 下全部五份 JSON Schema 及 storage-schema.md。
- 项目根目录没有 AGENTS.md；已检查的父目录也没有该文件，无法读取项目专属约束。建议补充原文件，不推测或代写其内容。
- 当前只有 README.md、docs/ 和系统 .DS_Store 文件，无 Go 源码、go.mod、CI 或已有应用。
- `git status --short` 返回 `fatal: not a git repository`；当前是规划目录，不能使用分支/worktree。
- 平台：Darwin 24.6.0、x86_64；Command Line Tools 位于 /Library/Developer/CommandLineTools。
- `go version` 返回 `go: command not found`；/usr/local/go/bin/go、/usr/local/bin/go、/opt/homebrew/bin/go 均不存在。建议准备符合选定 MyGo 版本的 Go 工具链后进行构建。
- Git：2.39.5 (Apple Git-154)；OpenSSH：9.9p2。
- 系统 HTTP/HTTPS/SOCKS 代理已启用，均为 127.0.0.1:7897；显式设置 HTTP_PROXY/HTTPS_PROXY 后可读取官方资料。未修改系统代理或 shell 配置。

## Phase 0 范围、依赖与验收核对

04 定义 Phase 0 为初始化、技术 POC、风险验证和 schema，无前置阶段；交付包括可运行原生窗口、本地/SSH Git 原型和技术决策记录。

05 明确列出的 Phase 0 任务如下。以下依赖是依据任务内容作出的执行顺序解释，原文没有逐项依赖图。

| Task | 执行前提/顺序 | 必须提供的验收证据 | 本轮状态 |
|---|---|---|---|
| T001 | 核对 MyGo/Go 版本与 native 模板后先执行 | macOS 本机窗口启动、lint/test/build 和 CI 配置 | 阻塞：工具链缺失，版本尚未锁定 |
| T002 | T001 工程基础；先解决契约表述冲突 | 领域类型、DTO、实际 JSON Schema 契约测试 | 阻塞：原文要求 Go/TS 一致，与纯 Go 决策冲突 |
| T003 | 工程基础及命令/错误契约 | stdout/stderr/exit、超时、取消、错误区分；MY005 还要求输出限额和特殊文件名 | 未开始，依赖前项 |
| T004 | 工程基础及命令/错误契约；在本地执行 POC 后验证 | SSH Config、严格 host key、安全编码、恶意及特殊路径测试；真实 SSH 验收需可用测试目标 | 未开始，尚未确认测试目标 |
| T005 | 工程基础、本地 Git POC | status -z/history/worktree 字节解析；特殊文件名、空仓库、合并历史测试 | 未开始，依赖前项 |

04 额外明确 Phase 0 应验证：三栏拖拽、10,000 条模拟提交虚拟列表、200 个可见 Graph 节点绘制与滚动、异步任务与 macOS 构建，并记录机器配置和性能。它们是模拟数据技术 POC，不等于提前实现 History/Graph 业务。

MY001–MY008、NU001–NU009 未逐项标注阶段。MY007 的 JSONC POC 可对应 TW001 的提前验证许可，但 NU007 的 Workspace 打开/创建/保存完整对话框与 W 阶段功能重叠。建议明确 Phase 0 仅做技术 POC、接口可行性和构建验证，Workspace 正式功能留在 W；不要将补充任务全部作为 Phase 0 功能实施。当前仅有 Intel 主机证据，Apple Silicon 真机启动不能通过交叉编译宣称通过。

## 官方 MyGo API 核对

本次读取 GitHub main 对应提交 `9c69cf829ece196c5b3a3601f1c7bbe70526376f`；这是审计基线，尚未作为项目依赖锁定。引用采用固定提交，避免后续 main 漂移。

| 规划能力 | 官方 API/资料 | 结论 |
|---|---|---|
| 纯 Go 原生窗口、native 模板 | [官方模板](https://github.com/egoist/mygo/blob/9c69cf829ece196c5b3a3601f1c7bbe70526376f/cmd/mygo/template/native/main.go.tmpl)：`WindowOptions.Content: ui.View(a.view)`、`App.WhenReady`、`App.Run` | 方向一致，无需 WebView |
| Go 版本 | [go.mod](https://github.com/egoist/mygo/blob/9c69cf829ece196c5b3a3601f1c7bbe70526376f/go.mod)：`go 1.27.3` | 当前工具链不满足，须安装或选定兼容的固定版本，不能擅自假定旧版本兼容 |
| 可调整三栏 | [split](https://github.com/egoist/mygo/blob/9c69cf829ece196c5b3a3601f1c7bbe70526376f/docs/ui/split.md)：`ui.Split`、`SplitPercent` 等双 pane 分栏 | API 可支持嵌套实现；三栏交互仍需 POC |
| 虚拟列表 | [list](https://github.com/egoist/mygo/blob/9c69cf829ece196c5b3a3601f1c7bbe70526376f/docs/ui/list.md)：`ui.List`、`ListState`、`ScrollTo`、`Visible` | 官方明确仅构建可见及附近行；10,000 行性能未实测 |
| Graph 自绘 | [drawing](https://github.com/egoist/mygo/blob/9c69cf829ece196c5b3a3601f1c7bbe70526376f/docs/ui/drawing.md)：`Draw`、`ui.Painter`、`ui.Path`，另有 GPU 位图 `ui.Canvas` | 支持原生绘图；命中测试与滚动仍由 POC 验证，不能把 Canvas 自动理解为 HTML Canvas |
| goroutine 更新 UI | [views](https://github.com/egoist/mygo/blob/9c69cf829ece196c5b3a3601f1c7bbe70526376f/docs/ui/views.md)：`Window.Update(func(){...})` | 官方明确在主线程执行；`ui.Element` 不应存入后台任务或持久状态 |
| 原生 Diff | [diff-viewer](https://github.com/egoist/mygo/blob/9c69cf829ece196c5b3a3601f1c7bbe70526376f/docs/ui/diff-viewer.md)：`ui/editor.DiffView`、`DiffState`、`ParsePatch` | 官方已有原生组件；是否采用需在现有方案范围内验证，不自行替换规划的实现 |
| 文件监听 | [native](https://github.com/egoist/mygo/blob/9c69cf829ece196c5b3a3601f1c7bbe70526376f/docs/native.md)、[watch](https://github.com/egoist/mygo/blob/9c69cf829ece196c5b3a3601f1c7bbe70526376f/docs/plugins/watch.md)：`mygo.WatchFiles` | native 可直接调用；不需要页面端 watch 插件或 JS 包 |

以上是官方文档/模板核对，不是编译、运行或性能测试通过的证据。

## 具体冲突及建议（尚未修改设计）

1. 03 §3 仍为 TypeScript interface、AbortSignal、Promise；T002、TW002 仍写 Go/TS 契约。与 03 §7、§12 的纯 Go 决策不一致。建议将概念接口改写为 Go/context/字节输出，将验收改为 Go JSON DTO 与 JSON Schema 一致，不引入 TS。
2. 03 §9 仍要求 Canvas/SVG 选择并建议 SVG + 虚拟滚动，而 §12 已规定原生绘图。建议明确旧条款被 §12 覆盖，并通过原生 Painter/Path POC 验证。原生 SVG 图标能力不等于已有 Git Graph 渲染器。
3. 05 的 MY/NU 补充任务缺少阶段映射，NU007 尤其与 W 阶段重叠。建议补充映射及 POC/正式功能边界，避免本轮越界。
4. Repository schema 只有 location/path 等字段，没有 SSH host 身份，而 storage 设计含 hostId；GitOperation schema 为操作枚举附可选通用字段，没有逐操作的必填/互斥约束；Error 没有独立 schema。建议 T002 前明确传输 DTO、内部 RepositoryContext 与 Error 的契约边界，以及操作参数是否需要按枚举验证。本轮未擅自增加字段或改变 schema。
5. 根目录 AGENTS.md 缺失，且未安装 Go；前者无法核对约束，后者阻止 T001 本地构建与所有 Go 测试。建议补充原约束文件并准备选定版本的工具链。

## 检查与测试记录

- 目录与 Git 检查：确认规划目录，无现有代码；Git 元数据检查失败，原因见上。
- 环境检查：Git/OpenSSH/Command Line Tools 可用；Go 不可用。
- 官方网络检查：首次直连 README 超时；使用本机代理后 README、固定提交文档、模板、go.mod 获取成功。
- Schema 静态检查：见本轮执行命令结果；这只验证 JSON 可解析与基本结构，不代表 JSON Schema 实例契约测试。
- T001–T005：没有完成任务，没有可运行 Go 测试；不能把环境检查当作 Task 测试通过。
- 未实施后续阶段功能，未引入 WebView/JavaScript，未生成替代 Implementation Plan。
