# 项目协作约束

本文件根据用户已确认的规划及补齐要求建立。

- 使用纯 Go 与 egoist/mygo 原生 `ui`；禁止应用引入 WebView、HTML、JS、TS、Vue。
- 用户已在全量剩余任务编排会话选择 B，授权 Phase 2、W、3–7 及剩余 MyGo/原生 UI 补充项；按 docs/features/git-viewer/remaining-orchestration.md 分线实施，阶段依赖与验收仍须遵守。
- Phase 0 UI 保留为独立模拟探针；正式入口从 Phase 1 本地 Status/Diff 扩展至已授权功能。Git 写入只通过受控操作实现，开发验收使用隔离 fixtures，不对本项目仓库自动执行产品写入或推送。
- Git 通过系统 CLI，SSH 通过系统 OpenSSH。命令固定白名单、明确 argv；远端参数独立安全编码。严格校验 host key，不保存凭据。
- 固定 Go/MyGo 版本；联网可使用本机已有代理，工具链放在 .tools/，不要更改系统工具链或用户 SSH 配置。
- 依赖下载可包含 MyGo 官方仓库自带的其他模式代码，但应用入口、导入和运行流程必须仅使用原生 UI。
- 每完成 Task，运行对应测试并更新 05-tasks.md 与执行记录。实测、交叉构建与未验证项分别记录，不将编译成功等同于 GUI/远程实测。
- schema 是 JSON DTO 契约；内部执行上下文可以包含不序列化的执行信息。契约修改同步 Go、schema 和测试。
- 对当前目录新建 Git 仓库即可跟踪变更，不自动提交或推送。用户随后已明确确认本次 Phase 0 A/B 编排：允许建立基线、工作线及集成提交；不推送远端。工作线文件边界和冻结接口见 docs/features/git-viewer/evidence/parallel/contract.md。
- 用户最新要求：每次完成修改及对应验证后，提交本次任务相关代码和文档到本地 Git；不自动推送远端，不纳入用户无关修改。该要求覆盖此前“不自动提交”的约定。
- 修改前阅读相关代码；不得覆盖用户无关修改。错误输出和日志不得泄露凭据。
