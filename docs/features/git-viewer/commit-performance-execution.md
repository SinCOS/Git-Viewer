# Commit 区域 Git 操作性能修复

用户反馈 Commit 区域操作 Git 慢、卡。本轮遵循 ub-mode，先复现用户可见行为并测量，再作最小修复。

## 操作日志与原因

1. 读取任务约束、工作区状态、UI 后台调度、写入与验证链路、watch 刷新和变化列表；保存修改前文本，防止提交夹带已有改动。
2. 确认产品 Git 命令通过后台 goroutine 执行；写后复核仍需等待，不能把“未完成”当成成功。
3. 发现 Stage/Unstage 对每个请求路径都遍历全部状态以寻找重命名前路径，批量路径构造为 O(P×N)。添加排除磁盘/SSH 的基准并记录修改前结果。
4. 添加真实 Git fixture：暂存另一文件后当前预览、选择和已加载历史应保留；该测试在原实现按预期失败。另验证受影响预览清除及模拟慢 Git 期间 UI 仍能绘制。
5. 将重命名查找表在状态扫描时一次建立，按路径常数时间查询。保留两端路径、去重及安全校验，写前读取和写后独立复核不变。
6. Stage/Unstage/Discard 使用已有 applyStatusPatch 合并已验证状态，保留未涉及的预览和列表滚动；操作涉及或已消失的预览失效。其他写入保持原有 applyStatus 语义。
7. 索引/工作区写入不清空不可变提交历史和 refs；Commit、分支/远端等其他操作仍按原规则失效历史。
8. 文件行暂存改为直接构造含目标路径的操作，消除临时修改全局 Selected 再恢复的流程。
9. 全量测试、相关 race、静态检查与 Intel 打包通过；测试辅助函数整理后重新运行新增回归通过。本轮按差异块本地提交，不推送，已有无关暂存内容保留。

## 性能证据

同机 darwin/amd64，`BenchmarkBulkStagePreparation`，`-benchtime=3x`；Transport fixture 固定返回状态，包含状态解析、路径构造和验证状态解析，排除实际 Git 进程、磁盘和 SSH。

| 文件数 | 修改前 | 修改后 |
| --- | ---: | ---: |
| 1,000 | 5.91 ms/op | 0.84 ms/op |
| 10,000 | 559.66 ms/op | 7.27 ms/op |

10,000 文件场景约 77 倍；这是指定路径准备基准，不能解释为真实完整 Git 操作加速 77 倍。

## 验证命令

```sh
.tools/go/bin/go test ./... -count=1 -timeout=60s
.tools/go/bin/go test -race ./internal/ui/local ./internal/infrastructure/git -run 'TestStage|TestSlowStage|TestChanges|TestSourceControl|TestInlineStage|TestCommitForm|Test.*Rename|Test.*Write|Test.*Cancel' -count=1 -timeout=45s
.tools/go/bin/go test ./internal/infrastructure/git -run '^$' -bench '^BenchmarkBulkStagePreparation$' -benchtime=3x -count=1
.tools/go/bin/go vet ./...
.tools/go/bin/go tool mygo vet
git diff --check
make build
.tools/go/bin/go tool mygo build -platform darwin/amd64 -skip-dmg
```

## 边界

- 未连接真实 n1 主机，未测物理 GUI 交互、真实 hook 或磁盘等待。本轮消除已测出的路径准备开销和写后重置；不保证所有“慢”均已消除。
- SSH 写入继续独立连接，保留取消后的结果未知处理；没有为减少握手而改变其语义。
- 读取前状态和写后复核是安全边界，没有用乐观 UI 替代 Git 真相。
- 当前工作区通过验证；原 HEAD 的 lists.go 解析器依赖仍在已有未提交 history.go 中，单独检出基线的问题沿用此前执行记录，未夹带该 history 改动。
- crapper/dryer/mutator 未在当前工具集合或 PATH 中提供，没有宣称运行其指标检查。
