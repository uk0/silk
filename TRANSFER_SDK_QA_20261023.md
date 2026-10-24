# 流式传输 / 合并进度 / TransferProgress 验收 — 2026-10-23

> 后续 Windows 10 原生补测及修复结果见 [WINDOWS_TRANSFER_QA_20261023.md](WINDOWS_TRANSFER_QA_20261023.md)；本文下方环境与结果仍是当轮 macOS 验收记录。

## 交付范围

基线 HEAD：`1dd5c05432244057ecf67b0abbe6c91eb6a4ed0b`。保留工作树内上一轮 JSON 控件、字体与安全生成修复，不把它们重复计作本轮新增。本轮未 commit、push 或发布。

- `httpx/download.go`：同步／异步写入调用者提供的 Writer，固定 32 KiB 复制缓冲区、独立下载大小上限、超时／取消、SHA-256、节流进度、短写／截断处理。共用现有 HTTP URL 校验和传输错误脱敏入口。
- `async/latest.go`：高频快照合并；每个实例至多一条排队／执行中的派发，不创建 watcher goroutine 或计时器。中间值可丢弃，不是可靠事件总线。
- `gui/transfer_progress.go`：字节进度、已知／未知总量、显式成功／失败／取消状态；未知长度不产生动画心跳。运行时计数不写进设计文件。
- `ged/transfer_widget.go`、`decl/builder.go`：控件面板、拖放、属性、预览、类型化生成和 `OnStateChanged` 事件联动。
- `paint/icon_fallbacks.go`：与既有图标系统一致的矢量下载箭头／托盘，浅深主题使用精确调色板颜色；不是放大小位图。
- `examples/download-monitor`：本地 2 MiB 分块传输，已知／未知长度、错误／坏校验／取消入口；使用 io.Discard，**不创建下载文件**。
- 修复 `ProgressBar.SetValue(NaN)` 可能传播非法几何的问题；NaN 归零，正负无穷继续按边界钳制。
- 原生缩窗发现示例底部提示被裁切：改为动态分配高度，窄窗口按钮两列排列，空间不足时隐藏可选提示，不牺牲进度信息。

设计依据：[Qt QNetworkReply 下载进度](https://doc.qt.io/qt-6/qnetworkreply.html#downloadProgress)、[Go io.Writer 短写语义](https://pkg.go.dev/io#Writer)、[Go HTTP Response 长度／解压语义](https://pkg.go.dev/net/http#Response)。未引入 Qt 或新增第三方依赖。

## 自动化验证

环境：Go 1.26.10 / darwin arm64 / macOS 27.2 / Apple M2 Max。
本机证据（Git 忽略）：`bin/qa-artifacts/transfer-20261023/`。

| 检查 | 结果 | 证据 |
|---|---|---|
| 明确 13 个包，非 short | 4,159 pass / 10 skip / 0 fail | `packages-final.jsonl`、`packages-final.exit` |
| 明确 9 个包，race + short | 3,728 pass / 37 skip / 0 fail；无 race 报告 | `race-final.jsonl`、`race-final.exit` |
| SDK 默认 vet；GUI vet 禁用既有 composites 告警 | PASS | `vet-build-cross.log`、`vet-build-cross.exit` |
| macOS native IDE 与示例 | 构建 PASS，示例另做实窗验收 | 同上、`Silk Transfer QA.app/Contents/MacOS/download-monitor` |
| Windows amd64 IDE，silk_pure_go | 交叉构建 PASS，未在 Windows 执行 | `silkide-windows-purego.exe` |
| Windows / Linux amd64 HTTP SDK 测试二进制 | 交叉编译 PASS，未在对应系统执行 | `httpx-windows.test.exe`、`httpx-linux.test` |

计数含子测试与 fuzz seed，非独立函数数量。非 short 集成测试实际编译并运行设计器导出的 TransferUI，验证 int64 值与状态事件。race 的 short 模式跳过部分外部编译测试，但普通回归包含它们。

重点覆盖：

- 已知 Content-Length、未知分块、透明 gzip、空响应、HEAD、精确大小边界及超限。
- 非 2xx 不写目标；短写、Writer 错误、读错误、截断、连续空读、超时、取消、回调 panic 后 Body 关闭。
- SHA-256 正确／错误／格式错误；过长 expected hash 在 hex 解码分配之前拒绝。
- 异步请求 Header／Body 快照，Writer 生命周期；原有 buffered HTTP 回归未被流式入口改变。
- Latest 连续 100,000 次发布仅排队一次、并发发布、回调中再发布、关闭抑制、引用释放、消费 panic 后不锁死。
- 工作完成但 UI 尚未 drain 时控件未被后台修改；大量进度只占一条 UI 通知；取消后排队旧进度被丢弃。
- 控件 int64 精度、接近 MaxInt64 的百分比取整、空内容、未知总量、非法状态、同值不重复发信号。
- 绘制边界剪裁、语义颜色、主题矢量图标、紧凑布局、工厂默认值、设计／运行时状态隔离。

跳过项与上一轮相同类型：平台限定、非 widget 的 gui.Action、需 opt-in 的 native GL／图集／甘特图 fixture、当前字体无 U+20000、3 个 real-gopls 用例；不将它们记为通过。没有将 GUI 的 composites=false vet 称为全仓默认 vet 全绿。

复跑命令：

```sh
go test -count=1 -p=2 -json \
  ./async ./httpx ./jsonx ./gui ./ged ./decl ./paint \
  ./examples/download-monitor ./examples/json-inspector ./examples/http-tool \
  ./cmd/silkide ./internal/filetxn ./internal/silkgen

go test -race -short -count=1 -p=2 -json \
  ./async ./httpx ./jsonx ./gui ./ged ./decl ./paint \
  ./examples/download-monitor ./examples/json-inspector

go vet ./async ./httpx ./jsonx
go vet -composites=false ./gui ./ged ./decl ./paint ./examples/download-monitor

go run ./examples/download-monitor
```

## macOS 原生 UI 闭环

通过 CUA 操作本轮编译的原生示例，HOME 隔离在 QA 目录，只访问自己的临时 localhost 服务；构造界面不请求数据。

1. Known size → Download：运行中可见已接收字节／百分比；结束为绿色 succeeded，2,097,152 bytes。
2. Unknown size → Download：运行中显示 Unknown total 与持续变化的已收字节，而非伪百分比；完成显示 2.0 MiB、succeeded。
3. 两种传输摘要相同，并与独立计算的 demo 内容 SHA-256 一致：
   `1aa2812b6da836dc9e7de84492b43d4efadc5a47f07c801b3b951d93ad1ec767`。
4. Bad checksum → Download：完整接收后保持 failed，红色进度条，显示 SHA-256 mismatch；没有把字节 100% 当成校验成功。
5. HTTP error → Download：HTTP 503，BytesWritten = 0，失败状态。
6. 同一次自动化调用中点击 Download、再点击 Cancel：显示 cancelled；随后截图与缩放仍保留取消状态，没有晚到进度覆盖。
7. 实际拖动窗口缩小，发现 footer 裁切后修复并重新构建。最终在约 558 × 303 client 尺寸 footer 完整；约 400 × 320 时按钮两列、进度与状态保留、可选 footer 隐藏。新增自动回归检查同样的尺寸。
8. Cmd+Q 退出；shell 检查确认 QA 实例停止。CUA 查询已退出 app 可能自动重开，因此最后退出后用进程检查而非再次查询 app。

截图在会话内由 CUA 查看，未保存仓库 PNG。最后一次生产代码变动只收紧 ExpectedSHA256 字符串的长度预检；之后重新运行回归与构建，原生最后构建另做启动／下载复测。没有将本轮示例窗口验收冒充整个 IDE 设计器的原生拖放验收。

## 性能证据与边界

`bench.log` 为普通／race 回归完成后单独运行的三次微基准：

```sh
go test -run '^$' \
  -bench 'BenchmarkLatestPublishQueued|BenchmarkDownloadStreamingMemory' \
  -benchmem -benchtime=500ms -count=3 ./async ./httpx
```

- Latest 已有待处理值时 Publish：约 7.85–7.88 ns/op，0 B/op、0 allocs/op；测量单线程替换快照，不包含 UI 回调或锁争用。
- 合成内存 Transport → io.Discard，包含 SHA-256：1 MiB 和 16 MiB 两档均约 **34,800 B/op、25 allocs/op**。这是复制链路不随 body 大小累积缓冲的证据，不是真实网络吞吐量，也不含用户 Writer 的存储成本。
- 取消后本机 QA 实例的 5 秒被动观察，累计 CPU 时间由 1.68 秒增至 1.71 秒；见 `idle-process.json`。这只是单次 macOS 空闲观察，不是全应用 FPS／延迟基准，更不是 Windows CPU 问题已解决的证明。

## 限制与未验收项

- Writer 失败或取消后可能已写入部分／全部数据；校验在 EOF 后完成。SDK 不回滚、不关闭目标，也不发布文件。调用者负责临时文件、成功后的安全发布和错误清理。
- 取消是合作式；不可强杀阻塞 Writer／回调。异步调用应等 Future.Done 才能关闭或复用 Writer。
- Latest 仅合并可替换快照，不适合不能丢事件的日志／交易流；每个实例一条，不代表把全应用 gui.Post 队列改为了有界队列。
- 未知长度使用静态活动标记，不做心跳动画；这不是全应用绘制帧率调整。
- 没有实现断点续传、文件覆盖策略、SSE 或持久化下载任务队列。
- Windows/Linux 本轮未做原生 UI、剪贴板、输入法、混合 DPI 或线程调度实测。Windows pure-Go 构建不等于 Cairo 原生后端验收；没有宣称此前 GDI／输入延迟问题已经解决。
