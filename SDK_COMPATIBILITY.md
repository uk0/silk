# SDK 与桌面平台兼容性复核

本轮关注设计文件实际加载、平台窗口事件、安装后的文件系统行为和交付链。
编译通过、虚拟显示启动和人工桌面交互是不同等级的证据，不能互相替代。

## 2026-10-20 输入链路后续

新增可选 `gui.IInputMethodCursor`，不改变 `IWidget` 必需接口；CodeEditor、Edit、SearchBox 提供光标矩形。Windows 通过 IMM 在输入事件中定位组合窗/候选窗，不新增逐帧或空闲轮询；借用的 HIMC 成对释放。UTF-16 代理对按完整字符提交，并支持 `WM_UNICHAR`，逻辑焦点变化会清空未完成的代理对。

实际 Win10 微软拼音候选位置、选择后单次提交、取消不落盘，以及 `你好 🚀𠀀` 保存回读已验证；**代码字体 CJK/emoji 回退仍缺失，保存正确不代表字形显示正确**。跨任务切换焦点的提交策略还未专项定因。完整证据和测试边界见 [Windows 输入 QA](WINDOWS_INPUT_QA_20261020.md)。下面绘制阶段的数字和边界保留为历史记录。

## 2026-10-20 原生 Windows 更新

下文的 Windows 类型编译边界为历史记录。最新真机十二包回归为 4,595 个测试/子测试通过、38 跳过、0 失败；实际桌面拖放、Undo/Redo、保存回读、生成运行时 resize、Ctrl+End EOF 导航和 F6 构建均另有证据，详见 [Windows 绘制与 SDK 复验](WINDOWS_RENDERING_QA_20261020.md)。

本轮修正 `EndPaint → BeginPaint`、`TranslateAccelerator → TranslateMessage`、`GetStockObject → GetDeviceCaps` 三处 SDK 错误转发，新增 192 项静态入口检查和真实原生绘制/加速键/stock object 测试；自有窗口图标在替换和销毁时释放。`MapVirtualKeyEx` 原先指向的 native API 本来正确，仅规范内部命名并增加显式布局对照测试。

`win32` 纯 Go 测试程序的 Windows 386/ARM64 交叉编译通过，不等同于这些架构的 Cairo 链接或 GUI 运行通过。启动延迟、中文 IME、混合 DPI、多屏与长期全控件稳定性仍不能由这些结果替代。

## 安装目录与用户数据

此前 `core.LocalDataDir` / `WorkspaceDir` / `TempDir` 把 `os.ModeDir` 作为权限
传给 `os.Mkdir`。它是文件类型标志，不是 `0700` 权限；Unix 上因此会产生
`000` 目录。另一个问题是把日志、设置和工作区写进可执行文件旁，无法正常
支持只读安装、macOS 应用包或 Windows Program Files。

现在的路径契约：

| 内容 | 默认路径 | 启动前覆盖 |
| --- | --- | --- |
| 静态资源 | macOS `Contents/Resources`，否则可执行文件旁；开发回退 cwd/icon、各 GOPATH/bin/icon | `SILK_RESOURCE_DIR` |
| 用户数据、日志、配置 | `os.UserConfigDir()/silk/<可执行文件基本名>` | `SILK_DATA_DIR` |
| SDK 默认工作区 | 用户数据目录下 `workspace` | `SILK_WORKSPACE_DIR` |
| SDK 临时文件 | OS 临时目录下的进程独立 `silk-<程序名>-*` 目录 | 无 |

- `os.UserConfigDir` 对应 Windows `%AppData%`、macOS
  `~/Library/Application Support`、Linux `$XDG_CONFIG_HOME` 或 `~/.config`。
- 新建数据目录使用 `0700`；Windows 的实际访问控制由 ACL 决定。
- 无 home/config 配置时，明确警告并使用进程私有临时目录，不写当前目录。
- 显式目录创建失败会报告到 stderr，文件 API 的错误仍需调用方处理。
- 路径在首次访问时固定；`core` 初始化期间已经使用数据目录，所以环境覆盖
  必须在启动进程前设置，而不是在 `main` 里 `os.Setenv`。
- 旧安装旁 `local/ini.cml` 可读时作为迁移默认值加载；之后写入新的用户目录。
  不自动修改旧目录权限，不移动/删除原工作区。便携部署可显式指定旧目录。
- 属性表配置采用「用户覆盖 → 安装资源默认值」，保存只写用户目录。
  缺失默认文件只创建内存默认值；读取只读 ini 不再偷偷写入安装目录。
- 临时目录不承担持久化用途；`core.Close` 在退出回调完成后回收本进程创建的
  私有临时目录，不回收配置/工作区目录。异常终止仍可能留下临时产物。

回归：`core/paths_test.go` 从只读安装目录运行独立消费者进程，隔离 HOME、
AppData 和 XDG 配置，验证初始化、用户设置保存、目录权限和安装目录不变。
另有属性默认值/用户覆盖、非法路径名、保存错误与只读 ini 回归。

## SDK 读写与生成器

- `gui.LoadForm` 使用与生成代码相同的 `decl` 构建链，恢复嵌套控件、
  属性和布局单元格；未知深层控件与非法几何不再被悄悄丢弃。
- 文档存储毫米，SDK 使用固定 96 DPI 的逻辑像素编码；不能拿当前显示器
  DPI 再编码一次，否则跨屏幕保存/加载会缩放几何。
- `gui.SaveForm` 写明确的 `bounds_space="absolute"`。旧 SDK 曾写出未标记
  的局部坐标文件，与设计器绝对坐标在语法上无法区分；这类旧文件可添加
  `bounds_space="local"` 明确兼容，默认仍遵循设计器绝对坐标契约。
- ComboBox 新增 `Items` / `SetItems` / `SetActiveIndex`，选择变化与提交事件
  区分；删除选中项、清空和替换编辑器不会保留旧显示或重复子控件。
- DatePicker `SetFormat` 使用 Go time layout，空值恢复 ISO 格式；格式变化
  不产生日期值变化事件，禁用状态不再打开日历。
- 日期、范围/值、列表/选择必须按依赖顺序恢复；列表字符串按数据解析，
  不执行表达式。控件内部持有的 Edit/popup 不重复序列化。
- `silkgen` 的生成链不依赖 Cairo/cgo。自动启动日志写 stderr，正常/错误
  CLI 退出都清理 runlock；连续生成及遗留 runlock 不再污染 stdout Go 源码。
- 真实 IDE 保存暴露了 SDK 单独测试未覆盖的另一条链：设计器以前没有捕获
  `[]string`，导致 ComboBox 选项丢失；IDE 自己的生成器也没有清理文件名中的
  连字符。现在设计器复用 `decl.ApplyPersistedProperties` 恢复属性，IDE 与
  CLI 共用 `internal/silkgen`；`sdk-platforms.silkui` 生成 `BuildSdkPlatforms`。
  连字符、中文文件名、转义选项、闰日和嵌套几何都有回归。

## 平台与事件

- Win32：修正 `GetParent` 绑定、`WindowFromPoint` 的 POINT 参数 ABI、
  `Get/SetWindowLongPtr` 的 64 位入口、`WINDOWPLACEMENT.Length` 和 INPUT
  布局；`GetKeyState` 的按下位改为 `0x8000`。
- Win32 弹层显式绑定 owner，移动、缩放、隐藏、销毁及失焦清理弹层；
  焦点检查合并，捕获/hover 不再误清其他窗口，重复指针引用及时释放。
- GLFW 初始化失败不再导致无显示 SDK import 崩溃；构造/编解码可运行，
  真正请求原生窗口或事件循环仍会明确报错，而非伪装为成功。
- macOS 禁止 GLFW 自动 chdir 到 `Contents/Resources`，保留启动工作区。
- macOS 文件打开/保存改用绑定实际 owner 的 `NSOpenPanel` / `NSSavePanel`
  异步 sheet，不再等待无 owner 的外部 `osascript` 选择器。同步 Go API 等待
  期间继续驱动 UI；取消、关闭 owner、退出及异常路径恢复窗口状态。
  Linux 仍使用现有 `zenity` 路径，不能据此宣称 Linux 文件弹窗归属已验收。
- Linux CI 分开无 DISPLAY 的 SDK 测试与 Xvfb 桌面组，并有隔离 HOME 的
  原生窗口启动检查。发布包包含 IDE/生成器，Windows DLL 缺失会拒绝打包。

## 验收边界

- 当前工作树历史 `local/` 和旧应用包内 `local/` 已存在 `000` 目录。
  本轮没有批量 chmod 或删除它们，因此不能用本工作树的 `go test ./...`
  遍历结果代表源码测试结论；应使用明确包列表或干净导出目录。
  旧 QA 包完整保存在 `bin/qa-artifacts/SilkIDE.before-sdk-signing.app`；新包
  使用标准 Resources 目录并重新 ad-hoc 签名，不复用不可读的运行数据目录。
- Windows 的 `silk_pure_go` 仅用于隔离 Go 类型/API 检查，不能证明 Cairo
  链接、字体渲染或 Win32 消息循环的真实运行。
  已有 386 拖放代码仍有 64 位 ABI 假设；386 类型编译不代表完整拖放支持。
- Linux 虚拟显示的启动存活检查不能替代真实窗口管理器下的菜单、DPI、
  输入法、拖拽及多显示器交互验收。
- Unix 发布包如未打包 Cairo 及递归依赖，必须安装系统运行库，不应称为
  完全独立的便携包。Windows DLL 打包应在缺依赖时失败，而非静默发布。

## 本轮验证结果（2026-09-11）

| 环境 | 已验证 | 未据此承诺 |
| --- | --- | --- |
| macOS arm64，Go 1.26.5 | 11 个明确包测试、同组 vet、core/prop race；实际 IDE 保存、生成、预览、父控件拖拽/撤销、原生文件 sheet；签名校验 | 混合 DPI / 双屏、所有控件逐项人工验收 |
| Debian 12 arm64，Go 1.26.0 | 真实 Linux 构建；无 DISPLAY 9 包、Xvfb 5 包；非 root 只读安装；原生窗口在隔离 HOME 存活 12 秒 | 真实桌面菜单/IME/拖拽、Wayland 原生、Linux amd64 运行 |
| Windows | amd64/386 类型编译、MinGW 系统头文件 INPUT/POINT ABI 校验；纯 Go silkgen 跨编译 | Cairo 链接、Win32 消息循环和真实 GUI；386 完整拖放 |

设计器链的专项测试在 macOS 和 Linux 都实际执行
`LoadDesign → Save 两轮 → IDE regenerate → 外部 Go module 编译并执行 Build`，
不仅检查生成字符串。屏幕上 Cmd+S 生成的 `sdk-platforms.silk.go` 又由独立
`bin/qa-workspace` 模块运行 `TestScreenSavedSDKPlatforms`：三个选项、macOS
选中值、2024-02-29/Format、三个子控件归属及局部坐标均保留。预览选择
Windows 没有泄漏到已保存的 macOS 选中值。

原生文件 sheet 实测包含 Cancel 后重开、打开 QA `main.go`、保存中文名
`sdk-原生保存.svg`；结果是有效 SVG（29342 bytes），主窗口恢复可交互且无
`osascript` 子进程。导出内容可解析不等于 SVG 视觉逐像素验收。

稳定证据目录：`bin/qa-artifacts/sdk-compatibility/`；Linux 二进制及其动态
依赖清单在 `linux-arm64/`，属于验收产物，不是独立便携发布包。
核心复验命令：

```sh
go test -count=1 ./core ./geom ./paint ./gui ./graph ./prop ./ged ./decl ./internal/silkgen ./cmd/silkgen ./cmd/silkide
go vet -composites=false ./core ./geom ./paint ./gui ./graph ./prop ./ged ./decl ./internal/silkgen ./cmd/silkgen ./cmd/silkide
go test -race -count=1 ./core ./prop
# 独立消费者直接执行真实屏幕保存后的生成源码：
(cd bin/qa-workspace && go test -count=1 -run TestScreenSaved -v ./ui)
codesign --verify --deep --strict --verbose=2 bin/SilkIDE.app
git diff --check
```

`report` 测试还移除了“7 次 SQLite 写入必须在 100ms 内完成”的时序假设，
改为固定时间戳 fixture，并补充半开时间边界测试；没有改动生产聚合算法。
当前 QA 未安装 gopls；语言服务/调试器真实联调不是本轮通过项。

弹层归属及完整人工回放见 `POPUP_AUDIT.md`、`WORKBENCH_QA.md`；仍未宣称
全部 Qt Creator 功能或全部桌面环境兼容已完成。
