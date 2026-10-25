# Phase 0 验收证据

平台：Darwin 24.6.0 x86_64；系统报告 CPU 为 AMD Ryzen 7 2700X Eight-Core Processor，16 个逻辑 CPU；内存 32 GiB。Go 1.27.3，MyGo 9c69cf829ece，Git 2.39.5，OpenSSH 9.9p2。

## UI 性能基线

执行命令：

```sh
.tools/go/bin/go test ./internal/ui/poc -run '^$' \
  -bench 'Benchmark(VirtualList10000|Graph200)$' -benchmem -benchtime=1s -count=1
```

原始结果：

```text
BenchmarkVirtualList10000-16  702    1597420 ns/op   73345 B/op   1582 allocs/op
BenchmarkGraph200-16          17430    70645 ns/op     129 B/op     11 allocs/op
PASS
```

列表每次构建约 29 行（600×700 viewport），验证选择、跳转到第 9999 行、滚动后仍保留虚拟化。Graph 独立 900×450 viewport 同时容纳 200 个模拟节点、连线、分支标签；验证首/末节点命中和缩小 viewport 后双向滚动。Graph benchmark 的重复帧允许框架复用布局和绘制缓存，不能解释为每帧重新计算 DAG 的耗时。

以上为 `ui.NewTester` 的 headless CPU/内存基线，不是实际 GPU 帧率，也未测试真实大仓库数据。原生窗口已启动验证，GPU 性能和长时间交互另行实测。

[200 节点原生绘图截图](graph-poc.png) 由官方 UI 测试器生成，可复现：

```sh
GITVIEWER_ARTIFACTS="$PWD/docs/features/git-viewer/evidence" \
  .tools/go/bin/go test ./internal/ui/poc -run TestGraphDrawingHitAndScroll -v -count=1
```

## 原生启动与关闭

Intel 二进制和 MyGo CLI 生成的 Intel `.app` 均通过 `-smoke` 启动。代表性日志：

```text
native window created
native main-thread background update passed
background workers stopped
```

`-smoke` 在窗口创建后执行 Git 版本探测，结果经 `Window.Update` 回到主线程，检查 online 状态后退出；失败返回非零。关闭时取消任务，返回 App.Run 后等待 worker 完成。

## 其他证据

- `make check`：格式、go vet、mygo vet、全部 race 测试、CGO_ENABLED=0 构建。
- T002：真正的 draft 2020-12 JSON Schema 校验，包含负向契约测试。
- T003：独立输出/退出码/超时/取消/限额的子进程测试；真实 Git 特殊文件名测试。
- T004：真实临时 sshd 集成，无 skip；host key 未知/变更拒绝、认证失败、断线及编码注入测试。
- T005：真实 Git status/history/worktree fixtures；5 秒 fuzz 约 273,091 次输入。
- MY007 的 Phase 0 部分：`mygo.WatchFiles` 捕获同目录临时文件原子替换，0600 JSON 配置、失败保留、符号链接拒绝和 watcher 清理通过。JSONC 编辑仍留 W。
- 两个架构的 MyGo `.app` 打包、ad hoc 签名及 `codesign --verify --deep --strict` 通过；arm64 真机启动未验证。
- CI 仅建立配置，尚无远端运行结果。

## 后续已集成的补充证据

原生真实窗口的程序驱动探针见 [native-ui/README.md](native-ui/README.md)，含四场景 PNG 和 JSON 指标；它与上面的 headless 基线分别记录。认证并启动远端测试任务后再触发取消/超时/断线的三轮测试见 [ssh-runtime/README.md](ssh-runtime/README.md)。独立 Worktree 提交、所有权检查与集成验证见 [parallel/integration.md](parallel/integration.md)。物理输入/IME、GPU 计数器和 Apple Silicon 真机仍未验证。
