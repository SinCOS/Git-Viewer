# Commit 表单写入错误提示执行记录

- D4：Commit、Amend、Stage、Unstage 和 Discard 失败时，在 Commit 表单显示操作名称及现有安全错误信息。保留提交消息；普通刷新不会清除提示，下一次确认执行相关写入或成功切换仓库时清除。底部状态栏仍显示操作结果。
- 隔离 Git 仓库使用失败的 pre-commit hook 验证错误提示、刷新后保留、移除 hook 后重试成功及表单清理；相关 race 测试通过。
- `make check` 通过：Go vet、MyGo vet、全量 race 测试及本机原生构建。
- 未进行真实窗口物理交互与视觉验收；D4 保持未完成。
