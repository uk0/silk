# SilkIDE 竞争、渲染与线程调度 QA — 2026-10-20

本轮针对 race 数据竞争、后台任务抢占 UI、窗口退出与原生资源销毁；包含上一轮未提交的 Windows 字体回退和原生对话框 watchdog 修复。SDK 显示版本 2.5.1，IDE/仓库发布标签使用 v0.1.4。保留 `github.com/uk0/silk` 的现有导入路径，不发布需要 `/v2` 后缀的 SDK 模块标签；[Go 模块大版本规则](https://go.dev/ref/mod#major-version-suffixes)要求二者匹配。新增 tag / IDE / module 校验阻止不兼容的版本标签。

## 已复现与修复

- **输出回调竞争**：新增双管道子进程测试在旧实现触发 `WARNING: DATA RACE`，并观察到并发 progress callback；心跳仍在回调时命令已返回。现在序列化快照及其交付，退出前 join 心跳，防止旧进度覆盖最终结果。用例为跨平台 Go 子进程，不依赖 Unix shell。
- **UI 队列独占**：旧实现单次执行 1,000 个回调；一个慢回调后仍继续清空整个快照。现在每轮最多 64 个或 4 ms 软预算，保留未执行部分、FIFO 顺序、异常隔离、嵌套模态重入；执行过的槽位清空，尽早释放捕获的对象。单个回调不可抢占，阻塞工作必须移到后台。
- **Win32 消息饥饿**：有积压时轮转保留 input / paint / timer / 普通消息机会，两种主/模态泵共用选择器；空闲仍阻塞，不轮询刷帧。`WM_QUIT` 和取消息错误不被吞掉。其原因是原生默认消息类别优先级；参见 [Microsoft PeekMessage 文档](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-peekmessagew)。
- **任务取消后的孤儿管道**：Windows 只杀父进程，子进程继承输出管道后，旧实现取消 10 秒仍无法返回；`wg.Wait()` 在 `Cmd.Wait()` 之前，单靠 `WaitDelay` 不会关闭这些手动读取的管道。现对活跃父进程执行带 3 秒超时的 PID 范围进程树终止，取消后的读取另有 2 秒 grace，并在返回前 join 关闭监视器；父进程已经退出时也不能挂死取消或误报成功。
- **GLFW 退出边界**：仅 `SetUIWakeup(nil)` 不能撤销生产者已经复制的 hook。新增生命周期锁，注销 hook 后等待原生唤醒完成，再终止 GLFW；保留的旧 hook 随后为空操作。受控并发测试验证顺序，未把 Go race 检测器当作 C 库竞态的完整检测手段。参考 [GLFW 线程约束](https://www.glfw.org/docs/latest/intro#thread_safety)。
- **独立画布的字体竞争**：八个独立 purecairo context 并发绘制，在旧实现触发 OpenType/sfnt scratch-buffer race、字形错误和崩溃。现在共享不可变字体表，不共享可变 `font.Face`；临时 glyph 数组使用有容量上限的池，每次调用独占，嵌套调用不覆盖外层。
- **纯 Go 绘制**：修复颜色转换截断及非预乘/预乘混淆、路径恢复后 current point 错误、空文本未清空复用 glyph buffer、虚线未实现、高 DPI 仅变换中心线不变换描边宽度，以及禁用图标所依赖的嵌套 group/alpha mask。前一轮记录的 26 个失败记录（含父/子项）所在绘制专项已全绿。

## 本轮追加：锯齿、模糊与边缘颜色

- **位图图标选错分辨率**：原先 `DrawIcon` 只按逻辑 16px 请求 PNG / 自定义 `Icon.Pixmap`，125% / 150% / 200% / 300% 变换仍放大 16px。新增棋盘像素用例在旧实现四档均失败；现按 CTM 的最大伸缩倍率选择真实设备分辨率，并保留小数逻辑尺寸、原有 painter 状态。自定义位图分配边长有 4096 上限，已有低清资源不会凭空变成高清。
- **Windows 字体彩边**：Win10 原生 Cairo 默认 / subpixel 探针得到相同的彩色字边；固定灰度抗锯齿后白色字形的 RGB 三通道逐像素相等，保留部分透明覆盖。100% / 125% / 150% / 200% CTM 的 ASCII + 中文回退专项全部通过，不依赖修改系统 ClearType / RDP 设置。只改变 Windows 离屏文字策略，不修改 macOS / Linux 默认字体策略。
- **GL 合成再次乘 alpha**：旧最终画面读回得到半透明像素蓝通道 40，按 Cairo 预乘格式应为 55；此前仅纹理上传读回无法发现。改用 `ONE, ONE_MINUS_SRC_ALPHA` 呈现 Cairo 帧，再恢复 GPU 图表的直通 alpha 约定。真实 macOS GL 对 PBO / 直接上传、全量 / 局部帧的最终 RGB 和单像素条纹逐像素验收通过；透明合成允许最多 1/255 的舍入差异，验证没有污染后续 overlay 混合状态。
- **明确未覆盖**：这些缩放用例是画布 CTM 验证，不是修改 Windows 系统缩放后的桌面验收。当前 Win32 后端尚未完整实现 per-monitor DPI awareness；系统对 DPI-unaware 窗口做位图拉伸仍可能模糊。没有只加 DPI manifest 开关而让控件大小、鼠标与 IME 坐标失配。用户所见新现象的原尺寸截图 / 平台 / 系统缩放尚待补充，不能把本次三处实证修复泛化成所有模糊均已消失。

## 验证

### macOS arm64

- `go test -race -count=1 -json ./core ./cairo ./purecairo ./paint ./gui ./graph ./prop ./ged ./decl ./cmd/silkide ./ide/... ./internal/processutil`：**18 个通过包，4,885 pass / 9 skip / 0 fail**；跳过项未计入通过。
- `CGO_ENABLED=0 go test -count=1 -tags silk_pure_go -json ./cairo ./purecairo ./paint`：**192 pass / 0 fail**（cairo alias 包无测试，不计成一个已测包）。
- `go test -race -tags silk_pure_go -count=1 ./purecairo ./paint` 通过；队列/唤醒及输出并发专项多次重复通过。
- `SILK_TEST_NATIVE_RENDER=1 go test -count=1 -run '^TestGLFWNativeRenderTransfers$' -v ./gui`：真实 GL 读回通过，160×120 全量 76,800 字节、局部 2,688 字节。不是仅构建/离屏像素验证。
- `go vet -composites=false`、平台打包脚本测试、工作流 YAML 解析、`git diff --check` 及隔离 IDE 构建通过。没有替换用户安装的 App。

### Windows 与发布门禁

- Win10 22H2 / Go 1.26.5：逐一回读校验 1,180 个 Go 文件哈希，然后 `-race -p 1 -count=1` 执行 core / cairo / purecairo / paint / gui / graph / prop / ged / decl / cmd/silkide / ide/taskrunner / internal/processutil / win32，**13 个包通过，4,638 pass / 39 skip / 0 fail**。Windows 原生消息公平性另外连续 5 轮通过。
- 跳过项包含未安装的 Git / gopls / dlv、需要交互桌面的独立 IME 测试，以及旧 Unix-shell 专项等；不计作通过。新增双管道并发进度用例是跨平台 Go 子进程，在本轮 Windows race 运行中实际执行。
- Win10 普通构建同一套 13 包回归：**4,638 pass / 39 skip / 0 fail**；`CGO_ENABLED=0 -tags silk_pure_go` 绘制回归 **192 pass / 0 fail**，两个被测包通过，cairo alias 无测试。
- Session 1 真实桌面，隔离配置 / 工作区、干净 PATH 下的 race 插桩版：拼音提交恰好一次、补充平面字符保存、preedit 取消和 Save All 回读通过；原生 Open 保持 8 秒未产生假 stall。F7 实际执行含 4,000 行 stdout / stderr 的项目测试，同时发送菜单快捷键并反复缩放窗口，持久化完成标记正确。关闭后用原生进程句柄核实退出码 **0**，完整 stderr 无 race / checkptr / UI-thread / fatal 标记。
- 第一次 UI 验收的 PowerShell Process 对象没有保留退出码；保留其失败记录，修正为提前持有原生查询句柄后，用全新工作区完整复跑通过，不把空 ExitCode 当 0。
- 普通 GUI 构建同一交互流程通过，原生退出码 0。启动稳定后欢迎页空闲 12 秒消耗 **0.234375 CPU 秒**（约单核 1.95%，不是整机 CPU 百分比）；仅为该机器单次观察，不是启动/输入延迟认证或提速倍数。
- 后续 Windows CI 发现 checkout / TempDir 跨盘的相对路径测试假设，以及 Git Bash 的 Unix PID / signal(0) 并不等于 Windows 进程句柄。修正跨盘 fixture，并新增不依赖 shell 的原生子进程测试；旧实现复现取消后仍卡在继承管道。任务树测试验证后代退出、同名无关 sibling 不受影响，另以握手文件 + 原生父句柄证明父进程先退出的孤儿管道分支。
- race / task-runner 修复提交 `5823cd1` 的[四平台 CI](https://github.com/uk0/silk/actions/runs/39426374069)全部成功；这是追加画质改动之前的基线，不冒充最终提交的 CI。
- 画质追加：本地 paint / gui / cmd/silkide 三包 `-race` **2,425 pass / 4 skip / 0 fail**，其后新增的变换边界用例在 paint `-race` 再次通过；纯 Go 两包通过；Win10 paint `-race -count=3`、字体四档 CTM 像素检查通过。
- 画质版 Win10 Session 1 完整重跑：实际鼠标打开文件菜单并截图（不是仅发快捷键）、拼音及 Unicode 保存回读、取消、Save All、8 秒原生对话框、F7 4,000 行输出与 12 轮窗口缩放均通过，原生退出码 0。欢迎页 / 编辑器各 12 秒空闲分别消耗 0.28125 / 0.109375 CPU 秒，均保持响应；单次观察不当作性能保证。截图见 [菜单与文字](screenshots/silkide-windows-visual.png)。
- 画质修复提交 `d6d18e3` 的[四平台 CI](https://github.com/uk0/silk/actions/runs/39428561489)全部成功（Linux / Windows / macOS arm64 / macOS Intel）。最终 Win10 源码回读匹配 **1,185 个 Go 文件**，paint / gui / cmd/silkide 全部 `-race -count=1` 通过，退出标记 `FINAL_VISUAL_RACE_OK`。
- **发布状态：已 commit / push，未打 v0.1.4 标签、未正式发布。** Windows 整窗系统 DPI 虚拟化问题仍未完成适配，尚不能证明用户报告的所有模糊已消失；最终归档本体的校验与 GUI 验收也还未执行，不用源码构建的 UI 测试冒充发行包验收。

四平台 CI 首轮补充发现两个环境相关测试缺陷：Windows CRLF checkout 导致三个源码提取 helper 错判函数终止位置，现改用 Go AST 并添加 CRLF / LF / 原始字符串 / 无末尾换行测试，同类 IDE / 旧设计器线程检查也同步修复；macOS hosted VM 缺少 NSGL 像素格式，关闭回调测试现使用不需要 GL 的真实窗口。渲染探针独立创建 GL context，CI macOS 的 `auto` 模式仅允许已知的 GL 能力错误显式 skip，像素差异和其他错误仍失败；Linux CI 及本机 macOS 继续 strict 模式。本机改动后完整 gui race 和 strict GL 读回均再次通过，不将 hosted macOS 的 GL skip 写成渲染通过。

Windows `-race` 另暴露了一处原生 accelerator **测试夹具**的 ABI 假设：六字节 ACCEL 的 Go struct 仅保证两字节对齐。实机逐偏移探针证实只有 DWORD 对齐地址成功，其余返回 `Invalid access to memory location`。已使用对齐的单记录存储，保留六字节原生布局，Win32 全包 `-race -count=25` 通过；不是把失败 skip 掉，也未将该夹具问题误称成 TranslateAccelerator SDK 转发错误。

## 验收边界

- 只对执行过的路径报告 race 结果；不能证明整个工程不存在任何数据竞争，也不能替代 C 库线程亲和性检查。
- 同一个 widget tree、`paint` 缓存、purecairo context/surface/scaled font 仍属于单一所有者；后台修改 UI 必须 `gui.Post`。修复共享缓存不意味着所有 SDK 对象成为并发 API。
- Windows task-runner 使用有界的 PID 进程树终止，不是 Job Object 隔离；已经脱离或父进程提前退出的后代不保证被终止，但其继承管道不能挂死取消。所有测试 fixture 均按保留句柄清理自建子进程。
- 4 ms 是回调间让出的软预算，不能中断某个耗时 callback；没有以任意 sleep 或最低帧数驱动静态界面。
- purecairo 仍有 AABB 裁剪、近似描边/曲线、有限合成算子和文字 shaping 等限制；极小虚线周期有 65,536 段展开上限。桌面发布包使用原生 Cairo，不宣称可选后端完整等价。
- 中文/emoji/Ext-B 的回退范围、字体依赖和复杂脚本限制见 [Windows 字体 QA](WINDOWS_TEXT_QA_20261020.md)。本轮不发布整体提速倍数，也不将 race 插桩进程的 CPU 值当作正常发行版性能。

## 证据

`bin/qa-artifacts/iteration-20261020-race/` 保存旧实现失败、源码 manifest、上传日志、原始 JSONL、Windows 脚本、截图及 stdout/stderr。关键基线为 `progress-before.log`、`queue-before.log`、`pure-font-race-before.log`；最终本地记录为 `macos-race-release.jsonl`、`pure-go-verified.jsonl`、`macos-native-render.log`。

Windows 首次类别 mask 方案未能让实际到期定时器越过 posted wake；保留失败后改用显式 WM_PAINT/WM_TIMER 范围，原生专项连续 5 轮通过。panic 日志打印超过 4 ms 时正确让出，旧断言已改为下一轮仍须执行剩余任务，未放宽丢任务检查。中间两条源码 wiring guard 因消息选择器重构失败，已更新为验证新的共用选择器并额外约束其 blocking GetMessage fallback；旧失败日志保留，不删除或改写成成功。无关 `gen_image/` 和根目录 `silkide` 不纳入本次提交。

画质负向证据：`visual-before.log`（高 DPI 资源选错）、`visual-gl-before.log`（预乘 alpha 暗边）、`visual-probe/{default,gray}.png`（原生字体彩边）；正向证据：`visual-gl-final.log`、`visual-windows-after.log`、`visual-regression.jsonl`、`visual-ui-run.log`。正式发布仍需最终 CI、归档校验与制品本体 GUI 验收。
