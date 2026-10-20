# Windows 10 真机 UI 迭代（2026-10-11，PDT）

> 2026-10-20 更新：下文保留上轮原始结论。GDI 增长已定位到 SDK 的 EndPaint 错调 BeginPaint，并有修复前后原生/桌面复验；最新状态见 [Windows 绘制与 SDK 复验](WINDOWS_RENDERING_QA_20261020.md)。启动阻塞与中文 IME 不因该修复自动算作解决。

按 Claude `win10testcomputer` skill 在真实交互桌面进行，不是交叉编译或模拟鼠标测试。目标是推进 Qt Creator 风格工作流，**不是完整功能/性能对等认证**。

## 环境和隔离

- Windows 10 22H2 / build 19045，amd64；Go 1.26.5、UCRT64 Cairo 1.18.4。
- 桌面 Session 1，1600 × 1200；应用窗口 1440 × 1000，并复查 1000 × 800。
- 源码、测试程序和工程位于独立 `C:\silk\qa-20261011\current`。
- GUI 通过 Interactive scheduled task 启动；SSH Session 0 只用于编译、测试、取回证据。
- GUI 启动继承不含 Go/UCRT64 的原始 PATH；没有修改机器持久化 PATH。
- 最终回归使用 `SILK_DATA_DIR` 与 `APPDATA` 都指向隔离 QA 目录。注意：`SILK_DATA_DIR` 只隔离 SDK 数据，**不单独隔离 IDE settings**。初次对照启动读取了既有 IDE 设置，不能当作干净会话基线。
- 每轮上传核对新符号/文件哈希；测试加 `-count=1`。原有工程、安装包及其他应用未删除或替换。

## 已实现

- 文件 / 编辑 / 视图 / 设计 / 构建 / 工具 / 帮助分组菜单；语义图标、快捷键列、菜单 Alt 加速键、方向键切换。
- 菜单按打开时的文档/选择更新可用状态；替换旧菜单时释放整个原生 popup 树，不在 idle 中重建菜单。
- 微软雅黑下菜单两列共用字体基线，避免文字和快捷键的字形 bearing 导致错位。
- 控件库增加搜索，过滤后保留原 factory 和拖放路径；清除搜索恢复分类。
- SDK SearchBox 增加全选、选择替换、复制/剪切/粘贴、Shift 扩展选择；按 Unicode rune 操作，长文字滚动/裁剪不覆盖图标。
- 对象树标题、hover、选择、连接线和拖放反馈使用当前主题；缓存行字体，不在每次 Draw 创建三份字体。窄面板搜索框尺寸保持非负并留在父控件内。
- 原生 Open/Save 归属于本应用的活动 form（popup 解析到 owner），不借用其他进程窗口。`OFN_NOCHANGEDIR` 避免选择器改变工程 cwd；扩展 UTF-16 缓冲区，取消不报错误，API 失败才记录 extended error。
- GUI 进程补充已存在的标准 Go/GOPATH 工具目录，显式 PATH 优先；不扫描磁盘、不改注册表、不自动安装工具。
- 原生绘制资源增加确定性释放：临时 blit painter / HDC surface 在 `EndPaint` 前释放，窗口销毁/缓冲区改尺寸时释放原缓冲区。兼容原 Painter/Surface 接口，自定义实现不必新增方法。
- 字体缓存排除 CTM 平移，修复 Windows `cairo_glyph_t` 对齐 padding，避免同字形数组比较偶发失败；缩放/旋转/字号仍分别缓存。
- 补齐未注册的 `prop.control.IntEdit`：精确保留 int64/uint64，非法中间文字不写属性，提交时还原；Up/Down 有界步进，混合选区和撤销正确，无变化不生成 undo 命令。
- 保存 companion 不再硬编码 `package ui`；从相邻非测试 Go 文件推断，跳过旧 `.silk.go`，包冲突/解析失败明确报告。没有相邻代码仍默认 ui。**尚未按 build tags/跨平台文件条件选择 package**。
- 共用 Windows 后台子进程策略，用 `CREATE_NO_WINDOW` 防止 Go/gofmt/Git/LSP/调试器及内置终端的独立控制台遮挡 IDE，不更改已有进程组/取消属性；显式 external-console/detached 请求优先。[Microsoft process flags](https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags) 说明此标志仅针对 console 程序，GUI 子程序仍可显示自己的窗口。
- Windows 默认编辑器改用本机等宽 Consolas（macOS Menlo，其他平台 monospace），恢复实际字体度量确认的 ASCII 快速路径，不强制将任意回退字体标记为等宽。
- 修复 Windows 回归夹具：使用本机绝对临时路径，settings 测试设置 APPDATA，避免测试读取/写入真实用户偏好。

## 实际桌面流程

1. 无 Go PATH 的 GUI 启动后按 F6，隔离的最小 Go 工程显示 `build ok`。
2. `Alt+F` 打开文件菜单，Right 到编辑菜单，Escape 关闭；Help/About、Tools/Project Settings 可打开并用 Escape 关闭。
3. 搜索 Button，全选替换为 GanttChart，再替换为 Button；结果仍能拖放。
4. 拖入两枚 Button，Ctrl-click 多选，应用 Grid，Undo/Redo；对象树显示 `Form → GridLayout → Button × 2`。
5. `Ctrl+S` 打开原生 Save，现场检查 `Owner == main HWND` 且主窗口 disabled；通过可见 Save 按钮保存 `windows-layout.silkui`，标题脏标记清除，生成 companion `.silk.go`。
6. 读取**这份屏幕保存的文件**，在 Windows 构造真实 SDK GridLayout，检查两子控件父对象及 300×160、420×240、220×100 三次 resize 后的边界，输出 `SCREEN_SAVED_RUNTIME_OK`。这不是另外合成的布局 fixture，也不是仅检查语法。
7. Open 可取消；About 的 logo、IDE/SDK 版本、平台/后端及 Close 按钮可读；Settings 显示正确隔离工程 root，不提交 Kit 修改。
8. 重复菜单开闭和窄窗口布局另外保存资源计数/截图，见本地证据。
9. 现场选中 Button，属性 `ColumnSpan` 用 Up 从 1 改 2、Down 改回 1，Grid 正确重新布局，日志不再出现 `prop.control.IntEdit` 未注册警告。其他 property CML 资源回退提示不等于本轮全部解决。
10. 保存后 companion 为 `package main`；配置了指向本次 SDK 的 QA module 后 F6 显示 `build ok`。这不是只构建一个无 SDK 的 Hello World。首次发现的 Go 黑色控制台已消失，构建期间主 UI 保持可见。
11. 最终字体构建通过原生 Open 打开 `main.go`，实际 en-US 输入 `// Windows editor QA 0123456789`，Ctrl+S 后 SSH 磁盘回读匹配 `EDITOR_SAVE_READBACK_OK`；再次 F6 `build ok`。这是输入/保存闭环，不是输入延迟 p95 测试。

## 有界性能及仍未解决的 GDI

同一 Windows 10/Xeon E5-2686 v4 2.30 GHz VM、Go 1.26.5/Cairo 1.18.4，GOMAXPROCS=2，固定 48 字符 ASCII 测宽，三次运行：

| 请求字体 | 实际等宽检查 | 耗时 | 分配 |
| --- | --- | --- | --- |
| 原 Menlo 回退 | false | 4,961–5,549 ns/op | 160 B / 2 allocs |
| Consolas | true | 77.37–96.54 ns/op | 0 B / 0 allocs |

原来的三个等宽测宽测试从 skip 变为 pass，并新增不跳过的 Windows 默认字体断言。只有此热路径改善，**不能换算为整机提速倍数或打字延迟降幅**；非 ASCII/制表符/比例字体仍走真实原生测宽。

最终生产字体构建静置采样 20.441 秒：CPU 0.28125 秒，约 **1.38% 单核占用**（机器四逻辑 CPU），20 次采样均 Responding，Private 74.71 MiB、handles 667。计数来自编辑/构建后的进程，不是干净启动内存基线；没有长期 CPU/FPS、输入延迟或前后整机 A/B 结论。

**GDI 尚未修复**。前一生产构建（同样的资源释放逻辑，字体更改前）10/20/30 次菜单循环后 GDI=72/116/164，USER 恒为 23。独立 QA 诊断构建在 20/40/60 次后，强制 GC 前后 GDI=126/220/304 不下降；Go wrapper 为 painters=5、surfaces=71、scaledFonts=8、faces=7、fontKeys=8，始终不增长。因此仅凭“等待 GC”或“字体 cache 数下降”不足以解释/修复，原生后端仍需继续定位。诊断代码与临时 Go probe 已撤出测试源码，生产没有强制 GC。

临时 HDC/pixmap 和独立小 popup 的局部资源压力测试稳定，不能覆盖真实 IDE 上述增长。菜单 window/USER 有界也不能证明 GDI 不漏。

## 验证命令与证据

```powershell
$env:PATH="C:\msys64\ucrt64\bin;C:\go\bin;"+$env:PATH
$env:GOMAXPROCS="2"
go test -p 1 -count=1 -json ./core ./cairo ./paint ./gui ./graph ./prop ./ged ./decl ./cmd/silkide ./ide/taskrunner ./internal/processutil
go test -p 1 -count=3 -run '^Test(WindowsDefaultEditorFontIsReallyMonospace|MeasureText)' -bench '^BenchmarkMeasureText' -benchmem ./gui
go build -p 1 -ldflags "-H windowsgui" -o silkide-after.exe ./cmd/silkide
```

上述仓库内 benchmark 可复验当前快速路径和 native shaping；表格的旧 Menlo/新 Consolas 同机对照来自保留的 `qa_editor_bench_windows_test.go` 与对应日志，不把不同机器或不同语料混作 A/B。

最终结果按 JSONL 的 test/subtest 事件统计：Windows 十一包 **4,572 pass / 38 skip / 0 fail**；macOS 十个有测试包 **4,707 pass / 9 skip / 0 fail**，`internal/processutil` 在非 Windows 没有测试。不是 4,572 个独立顶层测试，也不是整个仓库所有应用包的覆盖率。
Windows skips 包括缺 Git/gopls/dlv、POSIX shell/取消测试夹具、Mac modifier 与 opt-in fixture；不计作验证通过。Windows 等宽路径测试现在实际通过，不再跳过。macOS 另跑 targeted race、vet、native build 和真实 GL texture readback（PBO/direct、partial/full、Draw 内失效保留）：full 76,800 / partial 2,688 bytes。

最终 67 个新增/修改 Go 文件与 Windows 源码逐文件 SHA-256 回读一致；三个临时诊断/benchmark 文件不在最终源码中。源码基线 `de8070d620759d06b30cb4a165e678f7cbbbc76c` 加当前工作区 diff，未在本轮自动提交/推送。

```text
Windows silkide-after.exe SHA-256
AA1E595C6DB11760AF1120B1366307B1B3E4AF0CFAB7CAA928C347CA3134E44B
macOS isolated silkide-macos SHA-256
66df058f10e5b118383b97e0b39106ad450c15521fd8079fdaaf9f63e660d0e7
```

原始脚本、JSONL、截图、屏幕保存文件、profile 和 runtime probe 位于忽略目录 `bin/qa-artifacts/windows-ui-20261011/`；不把全屏其他应用画面收入 README。
主要证据：`windows-font-final-all.jsonl`、`local-font-final-all.jsonl`、`source-font-final-manifest.json` / `source-font-final-remote.json`、`windows-editor-measure-benchmark.log`、`windows-final-saved-runtime.log`、`silk_ui_editor_save_final.out`、`silk_ui_idle_font_final.out`、`gdi-diag-ui-stderr.log`。README 截图为最终生产构建的 DWM 可见主窗口边界抓取，不是生成图或色彩重绘。

## 不能升级为已完成的部分

- GDI 增长已在资源释放改进后的生产构建复现，见上方具体计数，不仅是第一轮的待复查项。
- 启动期并发编译负载下曾出现约 13 秒 UI watchdog 告警，最终字体构建日志也记录 8.416 秒 event-loop resumed。最终 HWND/Responding ready 为 5.81 秒，不等于全屏绘制完成。30 秒 Go profile 包含消息等待、系统调用、字体/Cairo 和启动工作，**不能直接将其全部归因为 Cairo CPU**；尚未获得 ETW/WPA 原生归因或输入 p95 延迟。
- 中文 IME 的 preedit/candidate 曾出现在窗口左上角；英文测试实际加载 en-US HKL。中文候选窗定位、复杂文字、混合 DPI、多屏、高对比度与完整无障碍仍待专项验收。
- `gopls` / `dlv` 未安装/未真实端到端验收；SDK 依赖的 cgo/MSYS 工具链与 Git 不是 Go PATH 补充的自动安装范围。
- 没有派发真实打印作业，也没有把搜索到 GanttChart 升级为所有甘特任务/依赖拖动的 Windows UI 验收。
- 当前仍缺 Qt Creator 完整等价的 signal/slot 编辑、完整 Edit 命令菜单、工具链自动检测/诊断、全面布局约束与大型文档/大型控件树性能验证。
- Linux 本轮未重新执行 native desktop 回归；Windows 真机结果不是所有 Windows 版本/设备的兼容保证。
