# SSH 运行中生命周期验证（B 工作线）

冻结基线：`53e686f6cfdaae9d162395f49d6de6007b2acc8f`。仅增加独立测试，不修改 Executor、process.Run、命令白名单、依赖或既有测试文件。

新增测试在真实 loopback sshd 上完成认证并执行测试 helper，读取远端 PID 标记确认已进入执行，再分别触发 context 取消、deadline 和已建立 TCP 会话断开。sshd 的 ForceCommand 只用于隔离 fixture，以便稳定阻塞任务；实际 Git 执行、参数编码和 host key 仍由原有集成测试覆盖。

命令：

```sh
go vet ./internal/infrastructure/ssh
go test -race -v -count=3 -timeout=40s ./internal/infrastructure/ssh
```

三轮通过，无 race、失败或 skip；新增三个场景共九次。错误类型依次为 OperationCancelled、OperationTimedOut、RemoteUnavailable，触发后最大返回延迟分别为 0.956 ms、0.764 ms、1.069 ms，均小于冻结的 3 秒上界。

测试结束显式清理远端 helper，并检查 PID 已被回收；关闭代理连接并停止 sshd。此清理检查与产品取消断言分开：本地 ssh 退出不能证明任意真实远端任务已终止。

原始执行记录见 [test.log](test.log)，机器/命令/场景/指标见 [report.json](report.json)。未测公网网络、交互认证、2FA 或远端 Git 写操作。
