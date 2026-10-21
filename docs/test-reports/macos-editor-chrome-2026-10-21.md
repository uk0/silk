# SilkIDE 设计器 / 编辑器细节打磨与 macOS 原生回归

日期：2026-10-21。本文记录 Stage16 macOS 验收；后续 Windows 状态见
[Stage17 原生回归](../../EDITOR_WINDOWS_QA_20261021.md)。没有发行包发布。

## 本轮修改

- 主菜单、主工具栏、设计布局工具栏和画布操作条采用显式开启的紧凑布局；不缩小用户设计的普通表单控件。
- 图标按钮采用方形点击区域，最小 28 个逻辑单位；文字按钮按字体实际度量计算宽高，统一图标间距和文字基线。常态平面显示，保留悬停、选中、禁用和焦点反馈。
- 工具栏图标按工具栏自身的 `IconSize` 绘制，与布局测量使用相同尺寸。溢出按钮改为可缩放双箭头绘制，避免字体缺字。
- 溢出菜单右缘与工具按钮右缘对齐。旧版从按钮左侧向右展开，窄窗口截图中只能看到图标；这不是菜单文字数据丢失。
- 编辑器状态栏按可用宽度分配独立区域：优先保留行列和模式信息，再显示字数；极窄时缩写行列，不再互相覆盖。
- 修复异步语言提供器在文本未改变时发布宏符号，大纲却不刷新的问题。
- 修复编辑、点击、离开、失焦、导航后旧 Hover / Signature 响应重新显示的问题。响应验证编辑器身份、路径、文本版本和交互版本；LSP 列位置使用 UTF-16。

主要实现：`gui/toolbar.go`、`gui/menu.go`、`gui/button.go`、`gui/theme.go`、`gui/dock.go`、`paint/icon_fallbacks.go`、`gui/codeeditor_status.go`、`gui/codeeditor_language.go`、`ged/code-outline.go`、`cmd/silkide/editor_hint.go`。

## 原生测试环境与来源

最终验收版本是 **Polished**，不是目录名含 `final` 的前一个中间版本。

- 构建：`go build -o bin/qa-artifacts/editor-sdk-20261021/stage16/silkide-macos ./cmd/silkide`
- SHA-256：`6a9d9a74ecadc561c6c226a0e576519586e2d665c4a289921e573518d92bd269`
- 应用：`bin/qa-artifacts/editor-sdk-20261021/stage16/mac-native-polished/SilkIDE QA16 Polished.app`
- 清单：`bin/qa-artifacts/editor-sdk-20261021/stage16/source-manifest-polished.json`，1685 个输入；测试后回读无漂移。
- 通过 Computer Use 操作真实 macOS 窗口，不使用模拟截图代替原生结果。
- 独立测试项目和 HOME；启动器设置 HOME、数据目录、工作区、资源目录、语言、PATH 和 Git 配置隔离。实际进程 PID 75803 的 8 项环境及 cwd 全部回读匹配。
- 仅靠 `Info.plist` 的 `LSEnvironment` 不能保证 HOME/PATH 生效，因此测试包使用 C 启动器 `execv` 原 Go 二进制；应用逻辑无测试专用替换。

## 最终版本原生验证

| 操作 | 实际结果 |
| --- | --- |
| `Cmd+P` → `sample.silkgo` → Enter | 初次打开即显示 `M Declare`，无需输入触发刷新 |
| 点击宏大纲 `Declare` | 光标移到第 3 行 |
| 2560×1664 缩到 1084×1126 截图尺寸 | 主工具栏显示清晰双箭头；编辑器底部字段不叠字 |
| 点击主工具栏溢出按钮 | 完整显示 Stop / Preview / Export / Properties |
| 切换 Design → 布局工具栏溢出 → 适应视口 | 菜单关闭，画布由裁切状态变为完整 100×100 mm 画布，缩放为 25% |
| 恢复大窗口 | 主工具栏、设计布局工具栏及画布操作条保持对齐 |
| 打开 `main.go`，调用处按 F12 | 实际 gopls 将第 7 行调用定位到第 5 行第 6 列声明 |
| 输入测试注释，点击保存图标 | 磁盘回读存在 `// polished-native`；保存提示正常，无旧类型提示残留 |
| 点击撤销 / 重做图标 | 注释消失 / 恢复；再次撤销并保存，磁盘文件恢复基线 |
| `Cmd+Q` | 正常退出，测试进程消失 |

截图位于 `bin/qa-artifacts/editor-sdk-20261021/stage16/mac-native-polished/`：

- `01-macro-outline.jpg`
- `02-narrow-editor-status.jpg`
- `03-overflow-menu.jpg`
- `04-overflow-fit-viewport.jpg`
- `05-design-toolbar.jpg`
- `06-editor-save.jpg`
- `07-editor-final.jpg`

操作期间自动化偶发 `noWindowsAvailable` / ScreenCaptureKit `-3812`；恢复应用焦点并重新观察后完成操作，不将失败的调用计作通过。

## 自动化回归

最终源码通过：

```sh
go test -race -p 1 -count=1 \
  ./diagnostic ./settings ./macro ./internal/fileops ./core \
  ./paint ./gui ./ged ./cmd/silkgen ./cmd/silkide
go vet -composites=false ./paint ./gui ./ged ./cmd/silkide
git diff --check
```

10 个包全部通过，`POLISHED_GATE_EXIT=0`。日志：

- `stage16/macos-race-polished.log`
- `stage16/macos-vet-polished.log`
- `stage16/narrow-window-regression.log`：紧凑布局、溢出菜单、状态栏专项 race 测试重复 3 次通过。

新增测试覆盖字体 12/16/24、图标 16/24/32、工具栏宽度 900/180/28/0；状态栏覆盖 0–960 逻辑宽度、短/长行列信息；溢出图标覆盖 16–64 像素。包含紧凑菜单可逆性、缓存/新建画布操作条一致性、提示生命周期和异步大纲更新回归。

## 未关闭的验收项

- **Windows 旧失败已在 Stage17 关闭。** Stage15 的 `TestWindowsMessageFairnessNativeTimerProgressDuringPostBurst` 失败日志保留；已用完整包 / 独立用例对照定位为测试线程消息队列污染。隔离用例且加强真实消息洪峰断言后，20 轮专项、10 包 race、vet、构建和记录的 Windows 原生 UI 场景通过。没有据此声称修改过生产调度器。详情见上面的 Stage17 报告。
- Windows 已验证新工具栏和记录的 UI 场景；Linux 原生桌面仍未验收。
- Dock 标签区的选中标签可见性、面板最小宽度、底部全局状态项间距，以及极窄窗口侧栏截字仍需下一轮处理。
- 本轮只验证上表所列交互，不代表所有菜单、所有控件和 Qt Creator 功能对齐已完成；Stage17 有单次有界 Windows CPU / 资源采样，没有 FPS 或长期压力结论。
