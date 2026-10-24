# 原生菜单像素边界与关闭标签恢复最终回归 — 2026-10-24

基线：`20704574472d190722f4e018eabbd1d1b399347e`。
这是 [关闭标签恢复 QA](EDITOR_REOPEN_QA_20261024.md) 后续修复的最终验收，
包含同一工作树中的编辑器恢复功能；未创建 Release／tag。

## 根因与修复

File 菜单仅八个命令，本应一次完整显示，却出现滚动箭头并隐藏末尾 Print。
字体度量产生小数尺寸，GLFW／Win32 原生窗口接收整数尺寸；原来的截断少了不到
1 个坐标单位，被布局判成溢出，进一步预留两个 18 单位滚动带。

仅修改 `gui/menu_scroll.go` 的 `Menu.fitPopupWorkArea`：

- 自然宽高使用 `math.Ceil`，避免文字、末行及其点击区域被截断。
- 真正长菜单的工作区高度使用 `math.Floor`，不超过可用屏幕；保留原始行高。
- 原生 API 至少需要 1 单位尺寸，合成小于 1 的工作区有明确的最小值例外。
- 保留每次打开后的重新布局，避免 Windows 尺寸未变、不产生 `WM_SIZE` 时状态残留。
- 未改全局窗口缩放策略，未新增 timer、goroutine、轮询或动画。

原始真实字体 probe 不改断言重放：

```text
修复前：natural=229.79931640625 native=229 viewport=185 band=18 → FAIL
修复后：natural=229.79931640625 native=230 viewport=222 band=0  → PASS
```

新增常规测试先红后绿：六组小数／整数度量、三次重开、原生取整模拟、末行点击、
宽度不裁切、整数／小数／极小工作区上限，以及回到大工作区后取消滚动带。

## 自动测试

冻结 1,537 个编译输入，两端均 `-count=1` 非缓存运行：

```sh
GOMAXPROCS=2 GOTOOLCHAIN=go1.26.10 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide ./cmd/silktools \
  ./internal/idetools ./internal/fileops ./internal/filetxn ./filesearch ./internal/regexscan
GOMAXPROCS=2 GOTOOLCHAIN=go1.26.10 go test -race -short -json -count=1 -p 2 \
  ./graph ./prop ./settings
```

Windows 的 `build.ps1`／`sdk-extra.ps1` 另外指定哈希核验后的 `-overlay` 映射。

| 平台 | 主批次 pass / skip / fail | 额外三包 pass / skip / fail | 随包真实 gopls |
| --- | --- | --- | --- |
| macOS arm64 | 5,334 / 47 / 0 | 238 / 1 / 0 | 3 pass / 0 skip |
| Windows 10 amd64 原生 cgo/race | 5,225 / 62 / 0 | 240 / 0 / 0 | 3 pass / 0 skip |

计数为包括子测试在内的终结事件，不是独立测试函数数。主批次 12 个测试包通过，
`internal/regexscan` 无独立测试文件；额外三个包通过。skip 不算通过。
macOS `go vet -composites=false` 通过（非严格 vet／非全仓 `./...`）。
千项菜单滚动 helper：**10,615 ns/op、0 B/op、0 allocs/op**，不是帧耗时或全工程性能。

## 真实 UI 与持久化

- **短菜单**：macOS File 菜单八个命令含 Print 全部可见，无伪滚动箭头；滚轮和三次
  重开不再隐藏末项。Windows 三次打开，每次滚轮前后 popup 原始 PNG SHA-256 完全一致，
  并检查 popup 在工作区内。置灰 Print／Export 是当前文本编辑模式的预期状态。
- **恢复标签**：两端均用三个 100 行 Unicode 文档，菜单／快捷键／命令面板恢复后立即
  输入并保存，按 UTF-8 原始字节核验。第 41 行的插入位置仍正确，不只检查 toast。
  均十次关闭／重开后再输入、保存、撤销；Windows 另重复 Cancel 和关闭全部后从 Welcome
  按 LIFO 恢复。Save／Discard／Cancel 单测及首轮 macOS Discard 实测见前一份报告。
- **SDK 长菜单**：80 项主菜单、60 项子菜单，Home／End／PageDown、滚轮、末项鼠标点击、
  嵌套末项执行；对父菜单发滚轮后子菜单关闭。两端实际输出均为 Zulu 两次、Omega 一次、
  Alpha 十次，共 13 次，未重复触发。Windows 另验证滚动带点击和全部 popup 正文边界
  （系统阴影单独记录，不算正文越界）。
- **最终状态**：三份文档恢复原始字节／哈希；正常关闭后回读真正的偏好文件，三个编辑器、
  Alpha 活动文件、Edit 模式均保留。IDE、随包 gopls 和 SDK 均退出，无本轮所属进程残留；
  Windows 六个交互任务返回 0 后注销。

| 静置采样 | 墙钟时间 | CPU 增量 | 单核占用 |
| --- | --- | --- | --- |
| macOS IDE | 10.015 s | 0.090000 s | 0.90% |
| macOS SDK | 10.019 s | 0.090000 s | 0.90% |
| Windows IDE | 10.235 s | 0.234375 s | 2.29% |
| Windows SDK | 10.187 s | 0.140625 s | 1.38% |

Windows 两项均 20/20 Alive／Responding。上述为单次约 10 秒、单核百分比，
macOS IDE／SDK 和 Windows SDK 在菜单打开时采样；Windows IDE 在编辑器静置时采样。
这不是所有机器、工程、负载都不卡顿的保证。
macOS 日志存在系统 TSM CapsLock LED 诊断，不是应用 panic／race 报告。

截图：[macOS 短菜单](screenshots/silkide-menu-pixels-macos-20261024.png)、
[Windows 短菜单](screenshots/silkide-menu-pixels-windows-20261024.png)。
提交的截图与原始捕获逐字节一致。

## 产物与边界

原始日志、脚本、字节、哈希及再次校验脚本：`bin/qa-artifacts/menu-pixels-20261024/`。
Windows 隔离目录：`C:\silk\qa-menu-pixels-20261024-codex`，不可变基线 1,801 文件与
179 个 overlay 文件在测试前后均核验。GUI 进程哈希与实际构建产物一致；随包真实 gopls
测试两端各 3 项通过，涵盖未保存 Unicode 补全／格式化、重命名及类型跳转／返回。

| 产物 | SHA-256 |
| --- | --- |
| macOS IDE | `d51d4e6d745a2ade219b0b9a961f6136895c07481d73e9d8580b3f2feffaaa3e` |
| macOS SDK | `04b1fcdcc23db059ecfc2098b643b618d0d7a000c6da546df76fffeb3cec622d` |
| Windows IDE | `a71757ee11714b6f61369aa6cbf0eb014897eae4b4c4210c6ec42b626b494125` |
| Windows SDK | `a568e585a9c6a81b1c617922b1a7cadcd82beac9cb6eabd395010a191051a8e0` |
| Windows 证据归档 | `dd6feae00db41479d23966a118b0f0cc2f753ae7ab35c278161dede357413233` |

Linux 原生 GUI、多屏混合 DPI、中文 IME 和断连网络文件未验收。
关闭历史仍是进程内最多 32 项，不恢复被明确丢弃的文本；打开文件仍复用已有同步读取路径，
未声称解决超大文件或网络文件读取耗时。未扩展成全仓无缺陷或 Qt Creator 全功能对等声明。
