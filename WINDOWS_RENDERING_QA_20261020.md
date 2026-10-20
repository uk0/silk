# Windows 原生绘制与 SDK 修复（2026-10-20，PDT）

延续 [上轮 Windows UI QA](WINDOWS_UI_QA.md)。本轮优先处理持续 GDI 增长，而不是用降低帧率掩盖资源泄漏；不宣称完整 Qt Creator 功能或性能对等。

## 根因与修复

`WM_PAINT → gui.Window.on_WM_PAINT → win32.EndPaint` 的最后一步实际调用了 **`procBeginPaint.Call`**。这不是 Cairo finalizer 延迟：SDK 把结束绘制错误地转发成再次开始绘制，覆盖原 `PAINTSTRUCT` 并遗漏显示 DC 的释放。

- 改为真正的 `procEndPaint.Call`，保留公开函数签名和既有 defer 顺序。
- Windows 原生回调中执行 105 次绘制，检查 `EndPaint` 不修改只读 `PAINTSTRUCT`，并测量预热后 100 次的 GDI 差值。
- 顺带审查相同风格的 SDK 转发器，发现 `TranslateAccelerator` 错调 `TranslateMessage`、`GetStockObject` 错调 `GetDeviceCaps`，均修正入口。
- 新增 AST 防回归检查，覆盖 192 个有同名 procedure 变量的转发器；另外以真实原生加速键表验证一次 `WM_COMMAND` 分派、未绑定键不被吞，以及 stock object / 显式键盘布局 API 的返回值。
- `MapVirtualKeyEx` 原来已绑定 `MapVirtualKeyExW`，本轮只是把内部变量改为同名以便审计，**不把重命名描述为已发现的键盘行为缺陷**。
- 窗口分别持有自己创建的大小图标：替换/销毁时释放，重复销毁幂等，关闭后设置图标不再分配对象；不盲目销毁 `WM_SETICON` 返回的共享/外部句柄。
- 实际英文输入发现 Ctrl+End 只移动到当前行尾。新增 Ctrl/Cmd+Home/End 文档首尾导航、Shift 扩展选择、视口跟随、Unicode rune 列、折叠 EOF 展开与次级光标合并；空文件安全，导航不修改文本或增加 Undo。GLFW 真实按键处理的 Ctrl/Cmd 回归先失败后通过，Windows 最终以 EOF 磁盘断言复验。

Microsoft 明确要求每次 BeginPaint 都有 EndPaint，后者释放显示 DC；自行创建的图标需要销毁，共享图标不能当作自己拥有的资源释放。见 [BeginPaint](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-beginpaint)、[EndPaint](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-endpaint)、[DestroyIcon](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-destroyicon)。

## 可复现证据

| 检查 | 修复前 | 修复后 |
| --- | --- | --- |
| 100 次原生绘制（预热后） | GDI 10 → 210；105 次结构被覆盖 | GDI 0 → 0；结构覆盖 0 次 |
| 创建/销毁窗口的生成图标 | 关闭后 `GetIconInfo` 仍成功，测试失败 | 旧句柄无效；50 个窗口 GDI 7 → 7，无强制 GC |
| SDK 转发器检查 | EndPaint / TranslateAccelerator / GetStockObject 三项失败 | 192 项转发检查通过，原生命令分派通过 |
| 真实 IDE 120 次文件菜单开闭（两边 debug=false） | GDI=156/270/374/486/584/684，USER=23 | 每 20 次采样均 GDI=18、USER=23 |

另保留首轮 debug=true 基线 160/278/398/520/638/756，但主要对照重新使用两边 debug=false 的原版/修复程序，不混合诊断构建。真正隔离入口变化的原生绘制测试直接失败→通过，桌面计数检验实际应用中的增长是否再出现；没有将其换算为 CPU/输入延迟提升倍数。

独立 QA 构建只在**自己的 Cairo DLL 导入表**计数，171 次 DC、171 次 DIB 分配均成功配对释放，但进程 GDI 仍增长到 192。这条证据排除了该组 Cairo 临时资源的失配，最终沿到错误的 EndPaint 封装。诊断不涉及其他进程；导入表探针不在生产源码/最终二进制中，不能用其耗时作为性能基准。

## 桌面与持久化

- Claude `win10testcomputer` skill，Windows 10 22H2 build 19045，Go 1.26.5、Cairo 1.18.4；Session 1 真实桌面，1440×1000 主窗。SSH Session 0 只编译/运行测试/回读。
- 隔离目录 `C:\silk\qa-20261020\current` 保存旧程序及诊断构建，`C:\silk\qa-20261020\after` 保存修复版本；不替换旧安装、不修改机器持久化 PATH。
- 实际搜索 Label 并拖入已有两 Button 的 Grid，屏幕保存文件中为同一个 Grid 的第三行。Undo/Redo 每一步均保存并回读控件数；最后恢复两 Button，非只发送鼠标事件。
- 保留三控件的屏幕保存快照，在真实 SDK 生成控件并 resize 为 300×160、420×240、220×100；三个子控件均属于 Grid，边界不越界，输出 `SCREEN_LABEL_RUNTIME_OK`。
- 原生 Open 打开 QA `main.go`；英文输入不再只做子串回读，最终严格要求 EOF 和实际构建结果，见下条。文件选择器和缺失工具的行为仍遵循上轮边界。
- 首次 F6 截图实际是 missing go.sum，不能当作通过：QA 工程没有复制旧 go.sum，且临时 runtime probe 被放进项目构建范围。已将 probe 源另存为非 Go 产物、移出项目，并仅在隔离 QA 工程执行 go mod tidy。之后最终导航构建实际执行 Ctrl+Home / Ctrl+Shift+End / Ctrl+End，输入 `// Windows editor QA 20261020_end`；断言该行是最后非空行，保存回读后 F6 截图 `19-navigation-build.png` 确为 `build ok`。这不是新增 IDE 自动安装依赖能力。
- 测试机另一个应用的置顶浮层曾覆盖截图右下角，这是桌面遮挡，不当作 SilkIDE 子控件悬空；该应用不被关闭。公开截图只抓本应用可见边界，必要时临时置顶本应用并恢复。

最终导航构建在原生 Open/编辑/构建之后另做 40 次菜单循环，GDI=82/82、USER=35/35。其绝对基线不同于未打开文件选择器时的 18/23；这里仅证明该状态下后续菜单没有继续增长，不冒充反复 Open/Cancel 已完成资源压力验收。

最终进程静置 20.229 秒，CPU 0.234375 秒，约 **1.16% 单核占用**（四逻辑 CPU）；20 次均 Responding，Private 74.70 MiB、handles 651。不是干净启动/长时间内存基线，也不是前后整体 CPU 或输入 p95 对照。

## 验证与产物

```powershell
$env:PATH="C:\msys64\ucrt64\bin;C:\go\bin;"+$env:PATH
$env:GOMAXPROCS="2"
# APPDATA / SILK_DATA_DIR 另外指向隔离 QA 目录。
go test -p 1 -count=1 -json ./core ./cairo ./paint ./gui ./graph ./prop ./ged ./decl ./cmd/silkide ./ide/taskrunner ./internal/processutil ./win32
go build -p 1 -ldflags "-H windowsgui" -o silkide-after.exe ./cmd/silkide
```

本地 macOS 另跑六包非 short 回归、targeted race、vet；本轮修改的原生 Windows 分支仍以 Windows 真机结果为准。GitHub Actions 已增加绘制配对、图标寿命与 SDK 转发验证，但本轮不声称 CI 已远程执行。

原生 API 修复阶段十二包 Windows JSONL 为 4,593 pass / 38 skip；加入导航后再次完整回归，最终十二包为 **4,595 pass / 38 skip / 0 fail**，记录在 `windows-navigation-all.jsonl`。macOS 最终六包为 **3,788 pass / 5 skip / 0 fail**。跳过项不是通过项，也不代表整个仓库全包覆盖。另有 targeted race/vet、native 构建；macOS native GL 每像素回读再次通过，full=76,800 bytes / partial=2,688 bytes。

纯 Go `win32` 测试程序另以 `GOOS=windows GOARCH=386/arm64 CGO_ENABLED=0 go test -c ./win32` 交叉编译通过；这只检查该绑定包的编译与已有静态 ABI 约束，**不是 386/ARM64 的 Cairo/整套 GUI 运行验收**。

最终 75 个新增/修改 Go 文件与 Windows `after` 源码逐文件 SHA-256 相同。初次 API 修复阶段的 72 文件清单也单独保留，不与最终导航构建混用。

```text
最终生产 Windows exe SHA-256
AC38703AAC20C9FF9D892B5D025C1AF3257286E6D0D99836A2105DE1F9D406F5
最终隔离 macOS binary SHA-256（未覆盖已安装 App）
205b2cd8f28ae2c7b9b44a1617b9f13bdb6bdf80cf352745f6c20f2d549dd679
EndPaint/图标修复后、额外 SDK 转发修复前的桌面压力构建 SHA-256
64ABBBAAE96360484F5C5B748C47A4054B0EE83EBE4CCC7E2F5DD3746BBF34A8
```

脚本、原始 JSONL、前后失败/通过日志、保存文件、独立原生诊断源位于忽略目录 `bin/qa-artifacts/iteration-20261020/`。重要日志为 `windows-navigation-all.jsonl` / `local-navigation-all.jsonl`、`endpaint-before.log` / `endpaint-after.log`、`icon-before.log` / `icons-after.log`、`forwarders-before.log` / `win32-after.log`、`silk_ui_baseline_release_menus.out` / `silk_ui_after_menus.out`、`silk_ui_navigation_save.out`、`source-navigation-final-manifest.json` / `source-navigation-final-remote.json`、`gdi-trace-native.log`、`saved-label-runtime.log`。两张公开截图为 `screenshots/silkide-designer-windows.png`、`screenshots/silkide-menu-windows.png`。

## 边界与后续

- 已定位并修复的是上述可复现 GDI 增长路径；120 次有界测试不是所有控件/设备/长期运行永不泄漏的证明。
- 上轮启动阻塞、中文 IME 候选窗定位仍需专项验证；本轮窗口 ready 时间不是完整首帧或输入 p95，未给出整体 CPU/FPS 提升倍数。
- 缺失的 Git/gopls/dlv、原生打印、完整甘特任务依赖拖动、多屏混合 DPI、高对比度/无障碍不由这些通过项替代。
- Linux 未新增 native desktop 验收，macOS 未新增交互验收；未自动 commit/push。
