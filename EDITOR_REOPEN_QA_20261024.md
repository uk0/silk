# 关闭标签恢复与后续菜单发现 — 2026-10-24

基线：`20704574472d190722f4e018eabbd1d1b399347e`。
此文保留恢复功能首次验收的证据；随后发现的菜单像素取整缺陷已修复，
最终合并代码的再次验收见 [菜单像素尺寸 QA](MENU_PIXEL_QA_20261024.md)。
不将这次检查描述为整个 IDE 无缺陷或全平台验收完成。

## 实现

- File → Reopen Closed Editor、命令面板和 Ctrl/Cmd+Shift+T 共用关闭恢复路径。
  原 Shift+T 运行测试的别名移交给恢复标签；F7、Build 菜单和 Run Tests 命令保留。
- 按最近关闭顺序恢复文件、Unicode rune 光标列及垂直视口，揭示 Edit 模式并交还输入焦点。
  即使全部文档已关闭、处于 Welcome，也能重新打开。新标签追加到标签栏末尾。
- 仅在成功关闭后记录；取消、保存失败、忙碌中的文件操作不消费／新增错误历史。
  丢弃意味着丢弃，重新打开读取当前磁盘，不存储旧缓冲区或恢复已丢弃文本。
- 每个 workbench 最多 32 个不同文件位置，只存轻量位置数据，不保留控件／LSP 引用；
  手动重新打开去除旧项，IDE 重命名同步映射历史路径。历史只在当前进程内保留。
- 一个无法打开的条目会给出提示并移出历史，下次操作可访问更早条目；忙碌条目保留供重试。
  不在菜单构造、绘制或空闲时扫描文件；不新增 timer、goroutine 或动画。

## 自动测试

冻结 1,536 个编译输入后执行（`-count=1`，非缓存）：

```sh
GOMAXPROCS=2 GOTOOLCHAIN=go1.26.10 go test -race -short -json -count=1 -p 2 \
  ./cmd/silkide ./gui ./ged ./core ./internal/fileops ./internal/filetxn
```

| 平台 | 测试事件 | 包 | 随包真实 gopls 集成 |
| --- | --- | --- | --- |
| macOS arm64 | 5,035 pass / 45 skip / 0 fail | 6 pass | 3 pass / 0 skip |
| Windows 10 amd64 原生 cgo/race | 4,925 pass / 61 skip / 0 fail | 6 pass | 3 pass / 0 skip |

Mac `go vet -composites=false ./cmd/silkide` 通过，不代表严格 vet 或全仓 `./...`。
新增回归覆盖三入口、焦点与视口、保存／丢弃／取消、保存失败、忙碌、LIFO、手动打开、
外部磁盘更新／截短、缺失文件、重命名、归属隔离、去重和容量限制。
历史更新基准：**88.55 ns/op，0 B/op，0 allocs/op**（32 项稳态 helper，非打开文件耗时）。

## 原生 UI 与读回

- macOS 与 Windows 均使用隔离偏好和真实的 Alpha／Beta／Zulu 三份 Unicode 文档。
  在第 41 行第 4 列关闭 Alpha 后，快捷键重新打开，立即输入 q 并保存；
  原始字节确认为该行 `中🙂 qAlpha.txt line 40`，不是只看 toast。
- File 菜单恢复后立即输入 m／保存，同样验证对应位置；随后撤销并保存恢复。
  命令面板恢复入口、关闭全部后从 Welcome 按 Beta → Zulu → Alpha 逆序恢复均实测。
- 两平台均执行十次关闭／重开，之后立即输入 c／保存再撤销，验证焦点与最终文件字节。
  Windows 每轮等待真实窗口标题，防止脚本只发送按键而未执行。
- macOS 实际点击 Cancel 保留脏缓冲区且恢复项仍置灰；再次点击 Discard 后经命令面板
  重新打开并保存，原始磁盘内容未被丢弃的 x 污染。Windows 实测 Cancel 分支。
  Save／Discard／Cancel 三分支另外都有单测覆盖。
- 静置约 10 秒：macOS CPU +0.110 s（约 1.10% 单核）；Windows CPU +0.21875 s
  （约 2.15% 单核），20/20 Alive／Responding。该采样不能保证所有机器／工程不卡顿。
- 三份文档的最终 UTF-8 字节／SHA-256 均恢复；正常退出后回读实际保存偏好，
  仍有三个编辑器、Alpha 活动文件和 Edit 模式。IDE／随包 gopls 均退出，
  Windows 三个交互任务最终返回 0 后已注销。

截图：[macOS 命令面板](screenshots/silkide-reopen-palette-macos-20261024.png)、
[Windows 恢复后立即输入保存](screenshots/silkide-reopen-windows-20261024.png)。
截图与原始捕获逐字节一致。

## 首轮发现（后续已修复）：短菜单误出现滚动箭头

本轮 macOS File 菜单只有八个命令，却显示滚动箭头并将末尾 Print 挤出初始视口。
现象保留在 `mac-reopen-menu.png`、`mac-cancel-menu.png`，不能算作正常完整布局。

进一步的独立 overlay 红测模拟实际 `Window.SetSize` 的整数像素截断，结果：

```text
natural=229.79931640625 native=229 content=221.79931640625 viewport=185 band=18
FAIL: short menu gains scroll bands from fractional native height rounding
```

GLFW `SetSize(int(w), int(h))` 和 Win32 `SetWindowPos(... int(h+margin))` 会截断尺寸；
当时 `Menu.fitPopupWorkArea` 未向上取整自然高度，`layoutPopupItems` 又将不足 1 px 的差额
判成真正溢出，额外预留两个 18 px 滚动带，扩大了问题。
后续按原生整数边界修正自然尺寸，保留长菜单的工作区上限并补常规回归；
同一探针修复后为 `native=230 viewport=222 band=0`，详见上述最终 QA 报告。

原始探针重放（修复前失败，修复后通过；与上方首次包测试分开记录）：

```sh
go test -overlay bin/qa-artifacts/editor-reopen-20261024/rounding-overlay.json \
  -count=1 -v -run '^TestMenuNaturalHeightNativePixelRoundingProbe$' ./gui
```

Linux 原生 GUI、多屏／多 DPI、真实中文 IME、断连网络文件和跨进程关闭历史恢复未验收。

## 产物定位

本地原始日志／脚本／字节／哈希：`bin/qa-artifacts/editor-reopen-20261024/`。
Windows 使用不可变基线 1,801 文件 + 178 个哈希核验 overlay，测试后再次核验；
最终 UI 的二进制哈希与本轮构建一致。

| 产物 | SHA-256 |
| --- | --- |
| macOS SilkIDE | `d68d9b5859a4ac828df55c824ea02d3eb8f4124bc3af40580066b9115fe3e30f` |
| Windows SilkIDE | `875b517c07cf60dabcd67b51577d301c59eff36cf27e4267b938fd4c6caa25a6` |
| Windows 证据归档 | `d98cc1a93ae8d3ab62004612eadcb04ad6de8f6b7dc2c688da7e771ebdba8cce` |
