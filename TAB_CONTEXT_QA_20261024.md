# 编辑器页签上下文菜单 — 2026-10-24

基线：`b97eb114c1850ee2613e6474e79917516aa73daf`。本轮在页签导航之上补齐
页签菜单，不声明全 IDE 或 Qt Creator 功能已完成。

## 缺口与实现

原来的 TabBar 没有右键菜单入口；新增 SDK `SetContextMenuCallback`、
同一标签上的右键按下／抬起识别，以及标签栏有焦点时的 Shift+F10。
非活动页的菜单不自动选择该页；禁用／隐藏／空白位置、松开时目标已删除、
重复按键和额外修饰键均不会误弹菜单。

Windows 真机还发现 F10 走 `WM_SYSKEYDOWN` 而不是普通按键分发。新增
`TabBar.OnSystemKeyDown` 仅接管 Shift+F10；保留宿主快捷键优先级及未处理系统键。
使用实际 HWND、线程键盘状态和 WM_SYSKEYDOWN 的新增测试，在首版二进制对应
overlay 上复现 `system F10 routed 0 times to -1; want 1/1`，再重新冻结并运行最终回归。

SilkIDE 菜单包含目标文件标题、保存、关闭、恢复已关闭编辑器、复制完整路径、
在文件列表定位。动作执行时重新核验编辑器对象、当前索引及跟踪路径，
而不是把打开菜单时的数字索引当作永久身份。未跟踪的缓冲区禁用保存／关闭等文件动作。
关闭复用 Save／Discard／Cancel、LSP 清理和关闭历史；定位复用异步项目文件扫描。

SDK 逻辑 popup 归标签栏所有，每个栏只复用一个；Windows 原生 HWND 仍随显示／隐藏创建与销毁。关闭时释放菜单项、子 popup、工具提示／
上下文注册以及动作闭包，避免隐藏菜单继续引用已关闭的大文档。这个策略仅用于新 API，
普通 `Menu.Clear` 契约未变。新增 `edit-copy` 程序化双页图标，按物理像素尺寸绘制，
复用主题颜色，不加载模糊放大的低分辨率位图。

先红后绿：最初 `TestTabBarContextMenuAvailable` 报
`TabBar has no owned context-menu/right-click API`。后续覆盖目标身份、重排／改名后的动作、
旧菜单不影响重开的同路径文档、关闭三种决定、空菜单／回调自移除、重复打开、
隐藏释放、菜单先隐藏再执行动作、修饰键／repeat、复制错误和项目范围。
定位单测记录调用路径；真实异步扫描的选择结果单独通过 Windows UI 验证，不能将 mock 当作 UI。

## 自动测试与性能

最终冻结 1,549 个源码／构建输入；Windows 对不可变基线 1,801 个文件和 192 个 overlay
逐项核对 SHA-256。所有回归使用 `-count=1`，不是缓存或 Windows 交叉编译：

```sh
GOMAXPROCS=2 GOTOOLCHAIN=go1.26.10 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide ./cmd/silktools \
  ./internal/idetools ./internal/fileops ./internal/filetxn ./filesearch ./internal/regexscan
GOMAXPROCS=2 GOTOOLCHAIN=go1.26.10 go test -race -short -json -count=1 -p 2 \
  ./graph ./prop ./settings
```

| 回归 | macOS arm64 | Windows 10 amd64 |
| --- | --- | --- |
| 主批次 race | 5,359 pass / 47 skip / 0 fail | 5,249 pass / 62 skip / 0 fail |
| graph / prop / settings race | 238 pass / 1 skip / 0 fail | 240 pass / 0 skip / 0 fail |
| 真实随包 gopls | 3 pass / 0 skip / 0 fail | 3 pass / 0 skip / 0 fail |

Windows 新增 `TestWindowsTabContextSystemF10AndRepeat` 在最终源码通过。
两端 IDE／SDK 示例均构建成功，工具目录完整性验证通过。

计数包括子测试终结事件；skip 不计作通过。主批次 12 个测试包，
`internal/regexscan` 无单独测试文件，额外三个包。
macOS `go vet -composites=false` 通过，不是严格 vet／全仓 `./...` 声明。
两端真实随包 gopls 均验证未保存 Unicode 补全／格式化、重命名与类型／实现跳转返回，
不以开发机 PATH 中的 gopls 代替随包工具。

菜单动作可用性不读取文档或扫描文件树，没有新增计时器或轮询。
图标复用既有资源加载／缓存，首次资源初始化不属于纯内存基准。微基准
`BenchmarkEditorTabContextMenu`：**2,621 ns/op、4,240 B/op、29 allocs/op**，
仅测内存中的菜单构建／清理，不包含原生窗口显示、文件读写或端到端输入延迟。
保存与关闭仍沿用既有同步单文件读写，慢网络文件仍可能阻塞；本轮不新增同步批量关闭循环。

## 原生 UI

首轮 UI 夹具把菜单正文和 Windows 阴影窗口一起当作“唯一弹窗”，因此报
`MISSING_DIALOG`；当时截图中菜单已正确显示。捕获 HWND／矩形后确认是正文与
右下各大 5 像素的阴影，夹具改按 `SILK_GUI_POPUP` 类精确选择正文，保留原始失败记录，
并先通过 UI 撤销／保存回读恢复干净文档再重跑，不为通过测试改动产品实现。

最终版的第二阶段夹具最初错误地假定关闭中间的活动页会选择右邻页；实际既有
关闭逻辑选择左邻页，文档正常丢弃但标题断言失败。保留失败脚本／截图／任务退出码，
通过真实 UI 恢复三页原顺序并逐字确认原文，再修正期望从干净状态重放；未改产品选择策略。

最终 Windows IDE（Session 1）验证：

- 无全局 gopls 的继承 PATH，实际子进程来自本轮 `apps\tools\gopls.exe`，状态栏 connected。
- Alpha 保持活动时右击 Beta，保存只落到 Beta；Alpha 未保存内容保留，Ctrl+Tab 往返后仍正确。
- Copy Full Path 从系统剪贴板回读为 Beta 完整路径；Reveal 在 Files 选中 Beta，再按 Enter 真实打开。
- Shift+F10 实际弹出菜单并关闭 Beta；快捷键恢复后文件内容与已保存字节一致。
- 原生 Save／Discard／Cancel 三分支、上下文菜单 Reopen 均通过；丢弃后恢复不复活已丢弃内容。
- 20 次右键菜单／Escape 循环后没有残留弹窗，光标后的输入仍写入正确 Unicode 行，
  保存逐字回读。最终三个文档全部恢复原文；22 份阶段字节文件重新独立验证。
- 空闲 10.167 秒，IDE CPU 时间增加 0.359375 秒，约 3.53% 单核；20/20 样本存活且 Responding。
  这不是 FPS、输入延迟或长时间零卡顿承诺。
- 正常 Alt+F4 关闭，IDE 与随包 gopls 退出；不终止其他应用。

阶段聚合 JSON 在跨脚本追加时发生 PowerShell 数组嵌套／默认序列化深度截断。
原日志保留，实际保存断言和各阶段 `.txt` 字节文件未受影响；使用独立 Python 脚本
验证 22 份字节并生成 `verified-save-stages.json`，不把损坏的聚合摘要作为依据。

最终 Windows SDK 示例验证：10 次菜单／Escape 循环，popup 内 Ctrl+Tab 不切换背景页；
关闭非活动 Beta 不发 CurrentChanged，Shift+F10 关闭 Alpha 后同一索引换为 Zulu
仅发一次事件；关闭末页进入 -1 空状态，Previous／Next 禁用。
四份快照的 Changes 为 `[1,1,2,3]`、Index 为 `[0,0,0,-1]`，字段文本均保留。
空闲 10.206 秒，CPU 时间 0.046875 秒，约 0.46% 单核，20/20 响应，正常关闭。
示例保留 SDK 默认资源主题，Close Page 的红色图标与 `icon/16x16/close.png` 一致，
不是 image-missing；IDE 已使用主题矢量优先模式。没有为截图统一主题而改动 SDK 全局默认值。
SDK 原始结果中的 PowerShell 字符串携带扩展属性，另从原始 stdout 独立校验事件序列，
生成不带扩展元数据的 `verified-sdk-results.json`；归档保留原件。

截图：[IDE 页签菜单](screenshots/windows-tab-context-20261024.png)、
[SDK 键盘菜单](screenshots/windows-sdk-tab-context-20261024.png)。

首版 macOS 构建曾启动，但 `cua.getApp` 截图再次返回
`ScreenCaptureKit.SCStreamErrorDomain Code=-3811`，未注入 UI 输入。
最终版 macOS 构建、单测和 gopls 集成通过不等于 macOS 原生 UI 通过。
首版隔离进程组核对身份后以 SIGTERM 清理，不计作正常 UI 退出；未重复启动最终 UI。
Linux GUI、多屏混合 DPI、中文 IME 未验收。

## 证据与后续

原始脚本、冻结清单、JSONL、输入／保存字节、截图与清理记录：
首版 `bin/qa-artifacts/tab-context-20261024/` 与最终版
`bin/qa-artifacts/tab-context-final-20261024/`；最终 Windows 运行于
`C:\silk\qa-tab-context-final-20261024-codex`，没有修改共享基线或全局工具链配置。

最终产物 SHA-256：

```text
Windows IDE  89e879db3909a06cbe9883cf91792c93793f9affd3e534ce4ecd77bd45b798d3
Windows SDK  09d8c9c1b8dd6d959b9b919a69f1ed55c466637328e499102d0c73bae13d682a
Windows LSP  5c409131c93b3d7136145492c5a39a08972fc483660beb4d2125a25447a79502
macOS IDE    82f6cd338ecc272db91701522052c9ab9992e4e939de38614989510fd07fe202
macOS SDK    1dfd50cbbc5ccd74f205d6e1d678ecac1418b2f942f6240711dd0afaf3aa8cc8
证据归档     9503a72a3cfa46b5761bb9ad79db3278324562e7fa9dcf4fefac61229d7fb10e
```

证据归档与远端哈希一致；Windows 运行进程的路径／哈希与构建产物一致。
7 个最终交互任务退出码均为 0 后移除，应用目录下进程归零；持久化回读仍为
3 个编辑器、Alpha 活动、edit 模式。原始首版失败归档单独保留。
最终归档脚本初次按 ANSI 读取含 Unicode 的 Go JSONL 时解析失败；显式改为 UTF-8 后
成功导出，未修改测试数据。最后再次确认 1,549 个源码／构建输入未变化，
`gofmt -l` 与 `git diff --check` 通过。

仍待推进：安全的批量关闭／关闭其他页签、关闭前异步磁盘预检、更多原生平台／DPI 验收。
这些未完成项未被本轮菜单功能替代，也未把长期目标标成完成。
