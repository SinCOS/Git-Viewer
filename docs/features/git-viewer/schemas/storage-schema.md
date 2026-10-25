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



## Phase 1 已实现的最近仓库存储

默认路径为 `os.UserConfigDir()/GitViewer/recent-repositories.json`（macOS 为 `~/Library/Application Support/GitViewer/`）。命令行 `-config /absolute/path/recent.json` 可指定隔离配置用于验收。

实际格式为按最近打开排序的 `Repository[]`，仅有 `repositoryId/location/path/displayName`，契约见 `recent-repositories.schema.json`，复用 `repository.schema.json`。Phase 1 仅接受 local，按 ID 去重，最多 20 项；不保存 host、凭据、Git 状态或 Diff。尚未引入以上建议集合中的 `lastOpenedAt/hostId/preferences`。

保存使用同目录 0600 临时文件、fsync、rename；新建应用目录为 0700。进程内更新串行化；不提供跨进程最近列表合并。加载限制 128 KiB 并校验类型、数量与重复 ID，损坏时提示且拒绝覆盖，用户仍可打开仓库查看。

## Workspace 持久化扩展
【原文明确】支持读取、创建、保存 `.code-workspace`。
【建议】`workspaces`: workspaceId, filePath, displayName, lastOpenedAt, lastKnownRevision；`workspaceAssociations`: workspaceId, repositoryId, optionalHostId（Git Viewer 私有关联，不写入 `.code-workspace`）。
原始 Workspace 文件是用户的独立源文件，不复制为权威配置；保存时保留 VS Code 原字段，私有 SSH 关联和 UI 偏好只存在 Git Viewer 的应用配置中。
