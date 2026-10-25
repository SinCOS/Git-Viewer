# 全量剩余任务并行编排

授权：用户在本会话选择 B，授权全部剩余 34 个未勾选条目（此前回复合计 40 为算术错误，实际清单为 34）；覆盖 Phase 2、W、3–7 和 MY007/MY008/NU007/NU008。此前 Phase 1 限制已由此授权更新。不得自动推送或发布；产品功能开发中的 Git 写操作在隔离 fixtures 中验收。

执行结果：五条工作线已集成，34 个原未完成条目关闭 31 项，T704/MY008/NU008 保留待验收；见 [执行记录](remaining-execution.md)。独立验证实际并行运行，开发由主 Agent 执行。

执行方式：会话未提供独立子 Agent 调用接口。主 Agent 执行各工作线，互不依赖的读取及验证并行运行，不宣称独立 AI Agent。既有 Phase 0 parallel/contract.md 仅约束旧 Phase 0 工作，不适用于此次新任务。

| 线 | 任务 | 文件所有权 | 前置条件 |
|---|---|---|---|
| A SSH | T201–T204 | infrastructure/ssh 新增 reader/config；对应测试 | 主 Agent 冻结传输接口 |
| B Workspace | TW001–TW006、MY007、NU007 | infrastructure/workspace、application/workspace；对应测试 | JSONC POC 可并行；正式验收在 A 后 |
| C History/Graph | T301–T304、T401–T403 | infrastructure/git/history、domain/graph；对应测试 | 数据层可并行；正式验收在 W 后 |
| D 写入/同步 | T501–T505、T601–T604 | infrastructure/git/write、application/write；对应测试 | 受控执行与队列；正式验收在 Graph 后 |
| E 刷新/发布 | T701–T704、MY008、NU008 | infrastructure/watch、发布脚本/证据 | 刷新协调器可并行；全量验收在 D 后 |
| 主 Agent | 契约、集成、Review | domain 共享 DTO、schemas、现有读执行器、UI、cmd、规划文档、AGENTS | 共享文件单一所有权 |

依赖：Phase 2 → W → 3 → 4 → 5 → 6 → 7。可提前实现独立模块，不跳过阶段验收。每个任务仅在对应产品路径和测试证据满足后打勾；部分完成保留未勾选。

接口原则：现有 GitOperation JSON 不增加未同步 schema 字段；内部分页/执行参数使用不序列化类型。UI 只接收受控业务方法；本地与 SSH 共用固定 argv 构造器。远程 shell 参数逐项编码，严格校验 host key。Workspace 原始 JSONC 与派生模型分离。写入按 common-dir 串行；失败后读取真实状态，远端中断后状态未知不得自动重试。

验收：每线定向测试 → 集成 race/vet/build → 原生 smoke → 双架构构建/打包。真实远程、真机物理输入、arm64 启动、发布签名及公证分别记录；无法验证时不得以编译通过替代。
