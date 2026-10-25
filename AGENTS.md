# 项目协作约束

本文件根据用户已确认的规划及补齐要求建立。

- 使用纯 Go 与 egoist/mygo 原生 `ui`；禁止应用引入 WebView、HTML、JS、TS、Vue。
- 当前只执行 Phase 0，依据 docs/features/git-viewer/04-implementation-plan.md 与 05-tasks.md；后续阶段需另行授权。
- Phase 0 UI 使用模拟数据验证分栏、虚拟列表、Graph、Diff 和后台任务，不实现正式 Git/Workspace 产品流程。
- Git 通过系统 CLI，SSH 通过系统 OpenSSH。命令固定白名单、明确 argv；远端参数独立安全编码。严格校验 host key，不保存凭据。
- 固定 Go/MyGo 版本；联网可使用本机已有代理，工具链放在 .tools/，不要更改系统工具链或用户 SSH 配置。
- 依赖下载可包含 MyGo 官方仓库自带的其他模式代码，但应用入口、导入和运行流程必须仅使用原生 UI。
- 每完成 Task，运行对应测试并更新 05-tasks.md 与执行记录。实测、交叉构建与未验证项分别记录，不将编译成功等同于 GUI/远程实测。
- schema 是 JSON DTO 契约；内部执行上下文可以包含不序列化的执行信息。契约修改同步 Go、schema 和测试。
- 对当前目录新建 Git 仓库即可跟踪变更，不自动提交或推送。用户随后已明确确认本次 Phase 0 A/B 编排：允许建立基线、工作线及集成提交；不推送远端。工作线文件边界和冻结接口见 docs/features/git-viewer/evidence/parallel/contract.md。
- 修改前阅读相关代码；不得覆盖用户无关修改。错误输出和日志不得泄露凭据。
