# 长菜单、子视口与编辑器焦点 QA — 2026-10-24

基线：`a1510d7c17ffe0ec7f563d46ec84c9643393dcb5`。本轮是开发分支增量，不创建 Release/tag。

## 复现与修复

- 原 `LayoutPopup` 只限制弹窗位置，不限制高度。80 项菜单在 300 px 工作区仍高约
  2,216 px（修复前红测保留）。现在先按锚点显示器工作区限制高度，再定位和分配
  原生绘制表面；保留正常行高，不靠挤扁按钮隐藏溢出。
- 长菜单增加滚轮、上下箭头点击、Page Up/Down；Home/End、方向键和文字前缀导航
  自动保证目标在可视区域。关闭后重置滚动位置，每次重新打开重新测量字体、内容
  与工作区。父菜单滚动会关闭已打开的子菜单，防止子菜单脱离已移动的锚点。
- SDK `gui.IChildViewport` 为内联子控件提供统一裁剪范围。Win32/GLFW 实际绘制与
  递归命中测试共用该范围；无效、空或非有限范围不绘制、不命中子控件。
  容器自身及 overlay 不受子视口裁剪，独立原生 popup 仍由自己的窗口管理。
- 两平台滚轮分发以当前指针／capture 为准，并穿过不处理滚轮的按钮向父容器冒泡，
  不再依赖可能过期的 hover。嵌套菜单捕获时，滚动父菜单能正确关闭子菜单。
- 真实编辑器复现 All Tabs 选中文档后焦点停留在展开按钮，立即输入 q 不生效。
  现在激活后重新验证 tab 身份并将焦点交给可见且启用的页面；同一已激活 tab 也修复，
  激活回调移除页面时不会对已移除页面设置焦点。
- 新增 `examples/menu-scroll`：80 项命令、隐藏／禁用项、分隔线与 60 项嵌套子菜单。
  滚动不新增 timer、goroutine、悬停轮询或空闲动画。

- Windows 真机进一步发现：小数菜单宽度被系统取整后，重复 `SetSize` 可能没有
  `WM_SIZE`，关闭时清零的 offset 与旧行坐标失配，反复打开逐次偏移。现在每次适配
  工作区后显式 Layout，不依赖 resize 回调。新增零 HWND 回归模型，模拟未收到
  resize 的原生路径；旧实现红测，新实现必须恢复首行并正确响应 End/Home。

- 同一台 Windows 桌面还复现 n → Right 后子菜单先出现、约 300 ms 后自行关闭。
  弹窗／capture 改变产生了指针未移动的 `WM_MOUSEMOVE`，旧悬停停止事件抢走键盘导航。
  现在 Win32 与 GLFW 记录屏幕坐标：键盘选中菜单时取消旧悬停事件，忽略坐标不变的
  合成移动；真实移动、滚轮或退出 popup 恢复鼠标处理。不引入新计时器。
  Windows 原生 handler 红测与共享 guard 回归覆盖静止事件、真实移动及 capture 释放。

## 自动测试

冻结输入后的最终相关包门禁（每次 `-count=1`，不是缓存结果）：

| 平台 | 主门禁测试事件 | graph / prop / settings | 随包真实 gopls |
| --- | --- | --- | --- |
| macOS arm64 | 5,306 pass / 47 skip / 0 fail | 238 pass / 1 skip / 0 fail | 3 pass / 0 skip |
| Windows 10 amd64 原生 cgo/race | 5,197 pass / 62 skip / 0 fail | 240 pass / 0 skip / 0 fail | 3 pass / 0 skip |

主门禁 11 个含测试包通过，`internal/regexscan` 无测试文件，不能计作测试通过。
Mac 的 `go vet -composites=false` 通过；不宣称严格 vet 没有既存警告。

```sh
GOMAXPROCS=2 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide \
  ./cmd/silktools ./internal/idetools ./internal/filetxn \
  ./filesearch ./internal/regexscan
GOMAXPROCS=2 go test -race -short -json -count=1 -p 2 ./graph ./prop ./settings
```

Windows 使用同一包列表与经过哈希核验的 `-overlay`，不是交叉编译。
尝试仓库根 `go list ./...` 被不可读的 `bin/local` 阻断，未修改该目录权限；因此本报告
只声明上述显式包范围，不称全仓 `./...` 通过。

新增回归包括：负坐标工作区、自然行高、键盘目标可见、隐藏／禁用项、分数滚轮、
非有限输入、滚动箭头命中、重新布局、子菜单关闭、捕获与坐标映射；子视口测试
同时核对 **Cairo Pixmap 实际像素和命中结果**。Windows 另直接覆盖 `WM_MOUSEWHEEL`
有符号坐标／delta 分发。标签焦点测试包括已激活页面及激活回调移除页面。

随包 gopls 集成验证：未保存重命名、Unicode 补全／格式化、定义／类型／实现／返回。
两平台实际语言服务可执行文件均在本轮应用的 `tools` 目录内。

## macOS 原生 UI

- 独立 SDK 示例验证 End → Enter、PageDown、滚轮、箭头点击、最后一项鼠标点击、
  嵌套菜单 End → Enter，以及父菜单滚动关闭子菜单。前三次真实 stdout 为 Zulu、Zulu、Omega；滚动本身没有执行命令。
- 10 次打开 → End → Home → Enter 循环，stdout 另逐次核验 10 个 Alpha 动作，
  合计恰好 13 次，确认重复打开没有坐标漂移。菜单展开静置 10.021 s，CPU +0.080 s，
  约 **0.80% 单核**；Close 按钮正常退出。
- 真正 SilkIDE 从隔离偏好恢复 80 个 TXT 文档。All Tabs → End → Zulu → 立即输入 q
  → 保存，原始文件字节为 `q中🙂 menu scroll 79\n`；Undo／保存恢复原字节。
  Home → Alpha → 立即输入 a／保存同样成功，再 Undo／保存恢复。
- 5 次菜单开合导航循环，菜单内最后一项可见。展开静置 10.017 s，CPU +0.090 s，
  约 **0.90% 单核**。80 份文档均逐字节验证恢复；Cmd+Q 后 IDE、gopls 及示例均退出。

## Windows 10 原生 SDK UI

- 交互桌面 Session 1，80 项父菜单与 60 项子菜单；原生 `SILK_GUI_POPUP` 窗口
  的内容边界均在 1600 × 1160 工作区内。系统 `SysShadow` 的 5 px 装饰阴影另行
  记录，不误计为第二个产品菜单，也不当成可点击内容。
- End／Home／PageDown、实际滚轮、滚动箭头和 Zulu 鼠标点击均验证。n → Right 后
  明确等待超过 1 秒再 End，子菜单仍存在且 Omega 可见；向父菜单滚动关闭子菜单，
  再次键入／打开／Enter 正确执行 Omega。
- 十次重开 → End → Home → Enter，stdout 恰好为 Zulu、Zulu、Omega 和十次 Alpha，
  共 13 次。20/20 次 Alive／Responding；展开菜单静置 10.193 s，CPU +0.109375 s，
  约 **1.07% 单核**。使用 Close 正常退出，未强杀最终成功轮次。
- 本轮先跑新增原生 handler 回归并构建示例，完成这条 UI 路径；随后重跑完整相关包
  门禁。最终校验示例 UI 所运行的二进制与完整门禁生成的二进制 SHA-256 一致，
  不混用两次源码或旧构建的截图。

## Windows 10 编辑器 UI

- 隔离偏好恢复 80 个真实 TXT 文档，实际 gopls 子进程指向本轮 `apps/tools/gopls.exe`，
  启动环境没有全局 gopls。All Tabs 的 PageDown／滚轮／End 及原生弹窗边界均验证。
- End → Zulu → 立即输入 q／保存，真实字节为 `q中🙂 menu scroll 79\n`；
  Undo／保存恢复原内容。Home → Alpha → a／保存及撤销恢复同样验证。
- 连续五次 End → Home → Enter 重新选择已经激活的 Alpha，再立即输入 b／保存，
  得到 `b中🙂 menu scroll 0\n`，撤销保存恢复；覆盖“选择同一标签”仍须恢复焦点。
- 80 份夹具全部逐字节恢复。菜单展开的 20/20 次 Alive／Responding 采样通过：
  10.188 s 内 CPU +0.1875 s，约 **1.84% 单核**。Alt+F4 正常关闭 IDE 和 gopls；
  退出后回读实际保存的会话，仍是 80 个编辑器、Alpha 活动文件和 Edit 模式。
- Windows 首次夹具错误沿用 macOS 的 `.silkui` 扩展名，导致进入 Welcome 而非
  恢复会话。已按 `settings.DefaultPath` 使用 `.ini`，正常退出、重置隔离夹具后
  完整重跑；最终启动脚本也增加 Alpha 标题断言。这是测试夹具修正，不是产品修复。

截图：

- [macOS 80 文档菜单](screenshots/silkide-menu-scroll-macos-20261024.png)
- [Windows 选择标签后的实际输入保存](screenshots/silkide-menu-scroll-windows-20261024.png)
- [Windows 完整原生菜单窗口](screenshots/silkide-menu-scroll-popup-windows-20261024.png)

## 性能与边界

1,000 项菜单滚动偏移更新基准：从首次实现的 23,264 ns/op、41,119 B/op、7 allocs/op，
优化至 **10,366 ns/op、0 B/op、0 allocs/op**。优化是仅在存在已打开子菜单时才建立
回调安全的关闭快照；仍保留真正子菜单的关闭逻辑。这只是 helper benchmark，
不是完整绘制帧率／输入延迟，也不是相对发布版的性能保证。

Linux 原生 GUI、实际多屏切换、各 DPI 缩放、中文 IME 组合窗口未覆盖。宽于屏幕的
菜单尚未增加水平滚动；箭头只支持点击，不支持悬停自动重复。没有声明所有控件的
独立 popup 问题均已解决，也没有由一次 CPU 样本推导所有机器绝不卡顿。

## 证据与中间失败

`bin/qa-artifacts/menu-scroll-20261024/` 保存原始红测、最终输入哈希、race JSONL、
随包 gopls 输出、真实截图、保存字节、CPU 与正常退出记录（本地产物，不提交大日志）。
1,534 个本地编译输入、Windows 1,801 个不可变基线文件及 176 个 overlay 输入均核验。

最终离线校验器 `verify-final.py` 输出
`FINAL_SOURCE_RACE_GOPLS_NATIVE_UI_SAVE_RESTORE_HASHES_CLEANUP_VERIFIED`。
Windows 80 份恢复文档另以预期 UTF-8 字节的 SHA-256 逐一核对，保存于
`windows-restored-document-hashes.json`；不只检查编辑器画面或字符串长度。
本轮 10 个交互任务最终退出码均为 0，已注销；两平台本轮拥有的进程均为 0。
提交截图与最终原生捕获逐字节一致。

| 最终产物 | SHA-256 |
| --- | --- |
| macOS SilkIDE | `51d857dff19841020cf389205f061893861e2607ff60d65cd988f4da603fa182` |
| macOS menu-scroll | `9169d076b202386db61321907a83209cd976e2a2ec950d05ecda8e5b98e97d9d` |
| Windows SilkIDE | `f2e783035a0a28b4ad055074fdb95ba0b2899f4d7fedea9ad522c7ba4dc748e9` |
| Windows menu-scroll | `8522c33bd1b921c20053e340046cba0a738cb722928b77ac29e7c227e5d32a93` |
| Windows 随包 gopls | `5c409131c93b3d7136145492c5a39a08972fc483660beb4d2125a25447a79502` |
| Windows 原始证据归档 | `0be3a10cba04f0ae6959a1be6e60fb5bdc2203afc65784c86bef2d657307d1bb` |

中途 Windows 首次编译发现 Win32 `Window` 没有 GLFW 的 `enabled` 字段，已改为
Win32 的实际字段检查并完整重跑上述门禁。测试自身关于 Hide 后 capture 的预期也修正：
Hide 正确释放 capture，应交给当前根窗口，而不是丢弃滚轮；保留失败记录，不删除覆盖。
首次 SDK 桌面脚本还发现 PowerShell 5.1 JSON 数组管道导致哈希筛选为空，脚本修正为
先赋值数组再筛选；产物哈希实际相符，没有因此修改产品或跳过哈希检查。

原生窗口计数脚本最初把 `SysShadow` 计入菜单数量，已按实际窗口类修正，保留原始
计数失败与完整窗口列表。随后发现的重开位移和静止指针关闭问题均是产品缺陷，
不是用脚本规避：分别保留旧实现的原生回归红测、UI 现象及修复后的重跑证据。
