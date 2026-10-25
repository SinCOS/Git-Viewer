# 本地持久化设计（非数据库表）

【建议】MVP 不引入关系型数据库，使用原子写入的 JSON 配置。未来引入 SQLite 时在本目录新建 migrations/。

逻辑集合：
- `hosts`: hostId, sshConfigAlias, displayName；不存密码、私钥或 host key。
- `repositories`: repositoryId, location(local|ssh), hostId?, path, displayName, lastOpenedAt。
- `preferences`: refreshIntervals, diffLimits, recentRepositoryIds, uiLayout。

安全与生命周期：
- 本地配置文件使用用户目录权限；写入采用临时文件+rename。
- Git Status、Commit、Diff 均是易变数据，不作为权威数据持久化。
- 不持久化明文认证材料；Git 真实状态以 Git CLI 查询为准。
- 【待确认】是否引入 SQLite 以支持跨仓库全文搜索与大规模缓存。



## Workspace 持久化扩展
【原文明确】支持读取、创建、保存 `.code-workspace`。
【建议】`workspaces`: workspaceId, filePath, displayName, lastOpenedAt, lastKnownRevision；`workspaceAssociations`: workspaceId, repositoryId, optionalHostId（Git Viewer 私有关联，不写入 `.code-workspace`）。
原始 Workspace 文件是用户的独立源文件，不复制为权威配置；保存时保留 VS Code 原字段，私有 SSH 关联和 UI 偏好只存在 Git Viewer 的应用配置中。
