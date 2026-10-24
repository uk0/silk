# 页签导航与一致事件回归 — 2026-10-24

基线：`c9cd2110ce0b6e1088ff1aa9a27a75809d021cdc`。本轮只推进页签交互，
不声明 Qt Creator 全功能对等，也不创建 Release／tag。

## 问题与修复

- `SetCurrentIndex` 重复触发通知，同页选择也通知；删除页时回调见到旧元数据，
  标签、页面与 refs 数量不一致；回调内改选又被外层覆盖。
- 原来的拖拽用 Remove+Insert，发布中间激活状态，再重排页面；改为原地重排，
  页签、堆栈、元数据同步后再通知，尊重回调内删除／改选。
- 原生窗口先执行普通 Tab 焦点遍历，导致编辑器获得焦点时 Ctrl+Tab 不能切页。
  两个后端共用最近 TabWidget 的导航路由，保留宿主快捷键、popup、模态／窗口边界。
- 新增 View → Next Editor／Previous Editor、命令面板入口和 SDK
  `TabWidget.NavigateTab(previous)`；已有缓冲区、光标、修改标记与撤销历史保持不变。
- SDK 示例 `examples/tab-navigation` 展示临时输入、事件计数、重排和中键关闭。
  删除非活动页不会触发 selection 事件，因此关闭回调单独刷新按钮可用状态。

最初针对性测试先失败，再修复，原始输出保存在 `red.log`：

```text
changes [1 1 1 2], want [1 2]
incoherent callback: 0 bar=0 stack=0 counts=3/2/2
reentrant ... events=[1 2 2 1 2]
Ctrl+Tab failed: index=0 focus=*gui.TabWidget
```

事件兼容性：按索引或页面身份变化通知一次，首个页面为 0，删空为 -1；
同页选择不再触发刷新事件。SDK 使用方必须接受 -1。回调仍运行在 UI 线程。
导航不创建 timer、goroutine，不重新读文件或打开 LSP 文档；不保证用户回调本身非阻塞。

## 自动测试

```sh
GOMAXPROCS=2 GOTOOLCHAIN=go1.26.10 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide ./cmd/silktools \
  ./internal/idetools ./internal/fileops ./internal/filetxn ./filesearch ./internal/regexscan
GOMAXPROCS=2 GOTOOLCHAIN=go1.26.10 go test -race -short -json -count=1 -p 2 \
  ./graph ./prop ./settings
```

Windows 为真机原生 cgo/race，并使用哈希核验的 `-overlay`；不是交叉编译替代。

| 平台 | 主批次 pass / skip / fail | 额外三包 pass / skip / fail | 随包真实 gopls |
| --- | --- | --- | --- |
| macOS arm64 | 5,346 / 47 / 0 | 238 / 1 / 0 | 3 pass / 0 skip |
| Windows 10 amd64 | 5,236 / 62 / 0 | 240 / 0 / 0 | 3 pass / 0 skip |

计数包括子测试的终结事件，不是独立函数数；skip 不算通过。主批次 12 个测试包通过，
`internal/regexscan` 无独立测试文件，额外三个包通过。macOS
`go vet -composites=false` 通过，不是严格 vet 或全仓 `./...` 验收。
真实随包 gopls 两端各 3 项通过、0 skip：未保存 Unicode 补全／格式化、
重命名后未保存 URI、类型／实现跳转及返回，不能用 PATH 工具代替随包工具。

导航 helper 的 macOS 微基准：**343.1 ns/op、480 B/op、3 allocs/op**。
这不是帧耗时、输入延迟或零分配承诺；新页焦点查找遇到首个可用控件即停止，
不保存额外焦点历史。

## Windows 原生 UI

- 在交互式桌面运行哈希核验后的程序。三个 100 行 Unicode 文档使用
  Ctrl+Tab／Ctrl+Shift+Tab、Ctrl+PageUp／PageDown、菜单、命令面板切换。
- 切换后实际输入并保存，逐字节回读 UTF-8；回到 Alpha 后第 41 行、Unicode 后
  的插入位置仍正确。逐次撤销并保存，最终三个文档恢复原始哈希。
- 从 Welcome 经命令面板返回 Edit；十轮前进／后退后再输入、保存、撤销。
  关闭全部后用 Reopen Closed Editor 重新打开，正常退出并回读真正的偏好文件，
  三个文档、Alpha 活动页、Edit 模式均保留。
- IDE 静置 **10.417 s**，CPU 增量 **0.218750 s**，约 **2.10% 单核**；
  20/20 Alive／Responding。仅是该负载的一次短采样，不是所有场景无卡顿的保证。

SDK 真机界面验证：

- 三个输入框分别编辑，快捷键／Previous／Next 切换后内容保留。
- 把活动 Beta 从中间拖到末尾，只发布 `Changed 7: 2 Beta`，不闪过其他页面；
  Ctrl+Tab 按新顺序回到 Alpha，继续输入仍落在原值末尾。
- 十轮前进／后退后，中键关闭两个非活动页，事件计数保持 28；仅剩一页时
  Previous／Next 都置灰，点击不改变页面或事件计数。删空只通知一次 -1。
- 精确核对 29 条事件、8 个结构化输入快照，最后用 Close 按钮正常退出。
  静置 **10.189 s**，CPU 增量 **0.187500 s**，
  约 **1.84% 单核**，20/20 Alive／Responding。
- SDK 日志有隔离夹具缺少 `apps/cursor` 的警告；未把日志描述为零警告。
  本次没有验证自定义鼠标光标素材，未发现 panic／race 报告。

截图：[Windows 菜单](screenshots/silkide-tab-navigation-windows-20261024.png)、
[SDK 重排](screenshots/silk-tab-navigation-windows-20261024.png)。
图片与原始截图逐字节一致，没有用重绘图代替程序输出。

## 输入一致性与限制

冻结 1,544 个编译输入。Windows 原始整批测试使用不可变基线 1,801 文件与
187 个 overlay 文件；macOS 整批测试运行两次。最后只修改了独立示例
`examples/tab-navigation/main.go` 的关闭／按钮同步：与两次冻结逐项比较，
**其余 1,543 个输入均相同**。最终示例两端单独重新 `go test -race -count=1`
和构建（示例包没有测试文件，表示编译成功，不计作新增测试用例）。
SDK UI 使用最终示例产物；IDE、SDK 库、测试包输入未被该示例修订改变。

原始脚本、JSONL、截图、持久化文件及哈希：
`bin/qa-artifacts/tab-navigation-20261024/`；Windows 隔离目录
`C:\silk\qa-tab-navigation-20261024-codex`。

本轮 Windows 六个交互任务均返回 0，归档后注销；IDE、随包 gopls、SDK 正常退出，
无该隔离目录所属进程残留。`verify-final.py` 再次核验工作树输入、两端测试结果、
进程／构建哈希、文档恢复与事件序列。

| 产物 | SHA-256 |
| --- | --- |
| macOS IDE | `24ad7b0a9d5fa8e9130d47d8deaaa426b8dd056445f2465bbc65f102f6ea976a` |
| macOS SDK | `88f742715fabc404b26947555af2d8a76a16b095499f1b405b958ff95d839804` |
| Windows IDE | `b5be4a53236cf4128cdc469b0b9b48ac754f6bfcf758aad9d7064bf0b5c49347` |
| Windows SDK | `12fd3cf0e0bdca4a37c9e11b026474479c19517d36387c418e170389fb35b8c3` |
| Windows 证据归档 | `194f5a05c9da44e861c6515072584035897b69d55f844c0dca7b3b2a319d7c7e` |

**macOS 真实 UI 未完成**：多次原生截图均返回
`ScreenCaptureKit.SCStreamErrorDomain Code=-3811`，没有向该窗口注入测试输入。
macOS 单测／构建／真实随包 gopls 成功不能代替这项 UI 验收。
该隔离测试进程组已核对路径／哈希后用 SIGTERM 清理，不计作正常 UI 退出。
Linux 原生 GUI、多屏混合 DPI、中文 IME 未验收；嵌套／模态／popup 边界属于自动测试覆盖，
未将其全部表述为真机手工操作。CPU 数据与 helper benchmark 均非全工程性能保证。
