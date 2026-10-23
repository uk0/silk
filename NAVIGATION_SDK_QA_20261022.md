# 编辑器导航与 SDK 协作迭代 — 2026-10-22

基线：`1427a4f`。本轮未提交、未推送、未发布安装包；不将上一轮 Windows 原生测试当成本轮结果。

对照的是 Qt Creator 的[符号／类型导航](https://doc.qt.io/qtcreator/creator-how-to-move-to-symbols.html)
和[位置历史](https://doc.qt.io/qtcreator/creator-coding-navigating.html)工作流，不声明已经达到完整 Qt Creator 功能对等。

## 本轮实现

### 编辑器与语言服务

- 统一菜单、命令面板、F12 和 Cmd/Ctrl+Click 的宿主语义导航。修复 SDK 按键／鼠标路径绕过 IDE LSP 钩子、落到不精确本地文本查找的问题。
- 新增类型定义和实现跳转；单目标直接打开，多目标在 References 面板选择，不再固定取第一个定义。新查询清空旧结果与过滤器，支持取消。
- 位置历史跨文件共享：定义、类型、实现、诊断、引用、大纲、宏跳转使用同一入口。保存源／目标行列与垂直视口，后退保留离开时的实时光标；新分支丢弃前进历史，重复位置去重，最多 256 个位置。
- 文件打开失败不污染历史；已关闭文件可重开；不可用目标可跳过；成功文件／目录重命名后迁移历史路径。不会保存或覆盖未保存缓冲区。
- 显式跳转展开折叠祖先、清除旧选区／多光标／补全提示并聚焦目标；LSP UTF-16 坐标在实际目标缓冲区内转换。
- 导航异步队列最多一个 worker、一个待处理请求、一个待投递结果；新请求取消旧请求，结果验证当前文档、编辑器身份、文本修订、光标、LSP 会话及文件重命名状态。后台不读取 GUI 控件。
- Edit 菜单补齐导航入口、快捷键、启用状态及独立前进／后退矢量图标；保持项目现有 F12 约定，不强行改为 Qt Creator 的 F2。

| 功能 | 快捷键 |
| --- | --- |
| 定义 | F12 |
| 类型定义 | Cmd/Ctrl+F12 |
| 实现 | Alt+F12 |
| 引用 | Shift+F12 |
| 后退／前进 | Alt+Left / Alt+Right；编辑器内也支持 Cmd/Ctrl+[ / ] |

### SDK 与设计器控件协作

- `core.LSPClient` 新增 `TypeDefinition`，以及 Definition／TypeDefinition／Implementation／References 的 Context API。统一 Location／Location[]／LocationLink[] 解码，优先使用 `targetSelectionRange`；保留 null 为空结果的兼容语义，拒绝无 URI 的伪目标。协议依据 [LSP 3.17](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/)。
- `gui.CodeEditor` 新增精确位置回调、宿主历史回调、位置快照／恢复、UTF-16 显式导航 API。旧的行号回调仍可用；独立 SDK 编辑器仍有本地 AST 导航。
- `ged.EditorTabs` 使用共享历史及精确列定位。修复打开不存在文件后，错误地滚动当前无关编辑器的问题。
- 设计器事件代码面板重复收到同一控件选中事件时，不再覆盖未保存代码；保存进入场景 Undo/Redo，撤销重做同步干净的代码视图，不覆盖当前草稿。
- 新增可移除的画布选择订阅；代码面板切换设计时释放旧设计与选择订阅、停止旧定时器。测试往返切换 10 次后仍只有一个有效订阅，旧场景不再驱动新面板。
- 窄代码面板的操作按钮换行；零尺寸、90/180/320/700 px 宽度都约束在父边界内，移除底部无效留白；标题区使用主题颜色。
- 桌面截图发现底部状态栏仍显示硬编码的 `main`、`Ln 1, Col 1`、`Go 1.25`。移除假状态，真实光标／编码仍由编辑器自身状态栏显示，运行时改为 `Runtime ` + `runtime.Version()`（不混淆工作区工具链）。SDK 状态单元保留最小间距、忽略空单元、窄尺寸约束到父边界并省略长文本；过满时不再让临时消息覆盖状态单元；构建／缩放文本变化立即重排，空构建单元首次收到结果时正常展开。

## 验证

证据保存在忽略目录 `bin/qa-artifacts/navigation-20261022/`，不提交测试二进制、隔离 HOME 或 Go 工具缓存。

- macOS / arm64 / Go 1.26.10：`core gui ged paint cmd/silkide` 非缓存短测试通过；根包及 `graph decl prop settings` 集成测试通过，共 10 个包。
- 同上前五包 `-race -short -count=1` 通过；最后的 IDE 宿主增量 `-race` 也通过。
- `go vet -composites=false` 对前五包通过。没有将关闭 composites 的结果写成无选项的完整 vet。
- 真实 `gopls v0.22.0`（工具位于忽略目录，不改变项目依赖）集成与 Race 通过：磁盘 caller 是旧文本，先发送未保存且含 emoji 的版本，再验证类型定义、局部定义、两个接口实现，以及后退后的原缓冲区和精确列。
- macOS 原生 GL 严格探针通过：160×120 framebuffer，全帧 76800 字节，局部上传 2688 字节。
- macOS Computer Use 实际启动隔离 HOME／两文件工作区：`source.go` 第 4 行调用点 → F12 → `target.go` 第 3 行符号列；Alt+Left 返回调用点，Alt+Right 返回定义。再从 Edit 菜单以键盘选中 Go Back，实际回到 source.go。截图检查导航图标、菜单快捷键列与 Forward 禁用状态；最终重建后再次验证 F12，并从 Build 菜单执行隔离工作区构建，Output 显示 `build ok`，底部构建状态单元正常展开，运行时显示 `Runtime go1.26.10`。以上是本轮桌面实测范围。
- Windows amd64：`CGO_ENABLED=0 -tags silk_pure_go` 的 IDE **交叉构建通过**，不是 Windows 原生运行／桌面验收，也不是发布用 Cairo 构建。
- Linux amd64 的相同无 CGO 尝试受已有 GLFW/OpenGL CGO 依赖阻塞（`go-gl/gl` 无符合条件文件）；未计为通过，未为规避该环境限制改变后端。
- `git diff --check` 通过。

核心复现命令：

```sh
go test -short -count=1 ./core ./gui ./ged ./paint ./cmd/silkide
go test -short -count=1 . ./graph ./decl ./prop ./settings
go test -race -short -count=1 ./core ./gui ./ged ./paint ./cmd/silkide
go vet -composites=false ./core ./gui ./ged ./paint ./cmd/silkide
PATH="$PWD/bin/qa-tools:$PATH" go test -race -count=1 \
  -run '^TestRealGoplsNavigationUnsavedTypeImplementationAndBack$' -v ./cmd/silkide
SILK_TEST_NATIVE_RENDER=1 go test -count=1 -run '^TestGLFWNativeRenderTransfers$' -v ./gui
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags silk_pure_go \
  -o bin/qa-artifacts/navigation-20261022/cross/silkide-windows ./cmd/silkide
```

初次验证暴露了 null 返回值兼容性与新测试初始化问题，已修正并保留早期失败日志。
部分早期 shell 包装脚本使用 zsh 只读变量 `status`，导致包装器错误；最终验证改用 `rc` 并单独记录 `.exit`，不混淆包装器错误与测试结果。

## 尚未宣称完成

- 本轮没有 Windows／Linux 原生桌面验收、混合 DPI／IME 测试；纯 Go 交叉编译不覆盖它们。
- 画布控件跨容器拖放、任意布局插槽重排、可视化信号连线和完整的 Go to Slot 生成向导仍不是本轮完成项。
- 事件代码面板的保存／切换会提交其代码；没有把它改成每键自动保存，也未穷尽所有设计器退出／导出时的草稿处理场景。
- 导航历史为会话内历史，不持久化；大规模引用结果的后台预览读取、分页／虚拟化仍需专门优化。
- 本轮没有新的全机 FPS／CPU 性能承诺，也没有声称修复所有渲染或线程问题。
