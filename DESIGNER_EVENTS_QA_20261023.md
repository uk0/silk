# 设计器事件、草稿安全与 SDK 协作迭代 — 2026-10-23

基线：`1427a4f`，在上一轮未提交的[导航与 SDK 改动](NAVIGATION_SDK_QA_20261022.md)上继续迭代。
本轮未 commit、push 或发布安装包；保留原工作树，不包含无关文件整理。

工作流对照 Qt Creator 的[设计器与代码编辑器协作／Go to Slot](https://doc.qt.io/qtcreator/creator-using-qt-designer.html)，
但本轮定位到的是设计文件内嵌事件编辑器，不声明已经实现外部 Go 源文件的双向导航或完整功能对等。

## 使用与新增行为

1. 在设计模式选中支持事件的控件，使用 **Design → Event Handlers（设计 → 事件处理函数）**，
   或画布右键“转到事件处理”子菜单。双击控件进入其默认事件；无可生成事件时保留原查看代码行为。
2. 菜单只列 SDK `widgetEvents` 已登记事件，模板参数来自同一份签名，避免手写模板与生成器漂移。
3. 没有处理函数时创建方法并绑定事件；已有函数时按 AST 精确定位。重复打开不增加代码、不制造新的脏状态。
4. 同一控件可以编辑多个事件。共享同名处理函数会定位到原所属控件，不复制函数正文；
   重复定义、签名冲突、非法名字或不可解析源码给出错误，而不是继续插入重复函数。
5. 修改内嵌代码后直接保存设计即可，不必先点代码面板的保存按钮。标题 `*` 和退出保护识别仅存在于面板中的草稿。

菜单使用已有 function 矢量图标；条目名称改用 ASCII `: ` 分隔。
macOS 桌面实测发现原箭头字符出现缺字方框，最终版本已复验没有该方框。
禁用状态同时设置到按钮与 action，宿主回调另有设计模式检查，避免旧菜单修改后台设计。

## 实现细节

### 草稿属于文档生命周期

- `ged/design_edits.go` 提供 `PendingDesignEdit`、可退订注册、`CommitPendingEdits()`、
  `SaveDesignWithPendingEdits()` 和 `DiscardPendingEdits()`。
- `GedScene.IsClean()`、`Title()` 和 `DirtyList()` 包含有效草稿；未编辑的建议模板不视为草稿。
  保存、独立设计器 Save As、Save All、恢复快照、模板保存、强类型代码导出都在边界提交草稿。
- 设计命令、撤销／重做及复制先提交草稿，使后续操作使用真实最新内容；自动热重载不覆盖只有草稿的设计。
- Save/Discard/Cancel：保存失败不允许离开，取消保留草稿，丢弃不会偷偷回写模型；已确认关闭时退订并解除面板绑定。
- 模型被其他编辑入口改变时，代码面板拒绝用旧 baseline 覆盖模型，保留当前草稿以便处理冲突。
- 修改回调具备重入防护，退订采用 copy-on-write；以编辑器 revision 快速识别未变内容。
  这些接口在 UI 线程使用，本轮没有增加后台轮询或逐帧代码字符串拼接。
- **兼容契约：`SaveDesign()` 保持无副作用，只取已提交模型。SDK 宿主需要包含草稿时必须调用
  `SaveDesignWithPendingEdits()` 并处理 error；不要把失败降级为成功保存。**

### 事件与代码协作

- `ged/event_navigation.go`：`SetEventBinding()`、`OpenEventHandler()`、`EventHandlerMenu()`，
  以及 `GedView.SetEventHandlerCallback()` / `RequestEventHandler()` 宿主接口。
- 新建 handler 与 binding 使用同一条设计命令，Undo/Redo 一起恢复；属性／右键绑定路径共享验证，
  拒绝 `_` 等无效名称。允许删除已不存在的旧插件事件绑定，但不允许新建未知事件。
- IDE 懒建并复用同一个 CodePanel，激活对应底部 dock；SDK 未安装宿主回调时可复用已绑定 panel。
- `HandlerEditor()` 供宿主访问嵌入编辑器。切到生成代码预览前提交真实草稿，避免预览滞后。
- `ged/handler_source.go` 通过 AST 分离多个事件方法、保留辅助声明与注释。
  单文件输出重写各方法 receiver；拆分输出不再仅识别首个方法或为后续方法生成重复空 stub。
- IDE 自动伴随文件生成成功时只记录 debug，不再冒充构建成功、不清已有 Problems、不切走代码面板；失败仍沿原错误反馈路径处理。

## 验证结果

证据目录：`bin/qa-artifacts/designer-events-20261023/`（Git ignored）。
主机为 macOS arm64，构建和测试工具链为 `go1.26.10`。

| 检查 | 结果与证据 |
| --- | --- |
| 11 包非缓存 short 回归 | 通过；`packages.log` / `packages.exit=0` |
| core、gui、ged、paint、cmd/silkide race | 通过；`race.log` / `race.exit=0` |
| 最终增量 ged、cmd/silkide race 复跑 | 通过；`final-affected-race.log` / `.exit=0` |
| 3 个非 short 生成器／真实 SDK consumer 测试 | 通过；`sdk-runtime.log` / `.exit=0` |
| 11 包 vet（显式关闭 composites） | 通过；`vet.log` / `.exit=0` |
| macOS 原生构建、Windows amd64 纯 Go 交叉构建 | 通过；`builds.exit=0` |
| macOS 原生桌面事件／退出／保存闭环 | 通过下述有限范围，非全平台 UI 验收 |
| `git diff --check` | 通过 |

核心复现命令（仓库根目录；不扫描 `local/`）：

```sh
go test -short -count=1 ./core ./gui ./ged ./paint ./cmd/silkide \
  . ./graph ./decl ./prop ./settings ./internal/silkgen
go test -race -short -count=1 ./core ./gui ./ged ./paint ./cmd/silkide
go test -race -short -count=1 ./ged ./cmd/silkide
go vet -composites=false ./core ./gui ./ged ./paint ./cmd/silkide \
  . ./graph ./decl ./prop ./settings ./internal/silkgen
go test -count=1 -v ./ged -run \
  '^(TestMultiEventGeneratedSDKRuntime|TestCodePanelTemplateIsAMethodThatExports|TestProjectRouteExportsPanelSeededHandler)$'
go build -o 'bin/qa-artifacts/designer-events-20261023/SilkIDE Events QA.app/Contents/MacOS/silkide' ./cmd/silkide
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags silk_pure_go \
  -o bin/qa-artifacts/designer-events-20261023/silkide-windows ./cmd/silkide
```

新增测试集中于 `ged/design_edits_test.go`、`ged/event_navigation_test.go`、
`cmd/silkide/designer_events_test.go`，覆盖草稿保存／恢复、撤销重做、复制、冲突阻止写盘、
关闭退订、Save All、热重载保护、退出各分支、事件菜单签名及单一面板路由。
保存焦点测试还验证保留既有构建诊断，而非只验证文件已写入。

早期宿主测试因 fixture 的 dock 未附加 frame 而失败，已改为实际 center 的 split dock，最终通过；
早期日志仍保留，不能混作最终结果。测试进程的 icon 路径／退出标记提示不等于发布包资源已验收。

### 真实生成代码运行

`TestMultiEventGeneratedSDKRuntime` 创建独立临时 consumer module，通过本地 replace 使用 SDK：

- SearchBox 同时绑定 `OnSearch` 与 `OnTextChanged`，正文修改保留在面板草稿，不调用单独 SaveCode。
- `GenerateCodeFile()` 提交草稿，将 receiver 从 `EventsUI` 变为 `ExportedUI`，输出 machine/user 文件对。
- 再次生成后 user 文件字节一致，不覆盖用户代码。
- 实例化 `NewExportedUI()`，调用 `Search.SetText("中文😀")`，断言 Label 为 `changed:中文😀`。
- 向 SearchBox 发送 Enter，断言 Label 变为 `search:中文😀`。

这证明强类型导出的两条真实回调能够执行，不只是生成文本匹配或 mock。
临时 consumer 内部的 `go test ./...` 仅作用于该小型测试目录，不是仓库全量扫描。

### macOS 桌面操作与持久化回读

使用隔离 HOME 的 `SilkIDE Events QA.app`，1100×760 窗口，英文 UI；
工作区位于证据目录 `workspace/`，设计文件为 `events.silkui`，内含 SearchBox `search` 和 Button `submit`。
未更改用户日常 IDE 设置。

1. 从未绑定事件的设计开始，双击 SearchBox，进入可编辑方法 `onSearchSearch(q string)`，标题出现 `*`。
2. 从 Design → Event Handlers 打开 `OnTextChanged`，生成并定位到第二个方法。
3. 编辑含中文／emoji 的正文，直接 Cmd+S；回读 `.silkui`，两个 binding 与完整正文均存在。
4. 根据初次实测修复成功生成抢 Output 焦点，以及菜单箭头缺字，再重建启动最终程序。
5. 重新打开已保存设计并双击控件，定位已有方法，不新增重复方法或脏状态；菜单正确列出两个事件与处理函数。
6. 改入 `// QA 中文😀 final readback` 后 Cmd+Q，实际出现 Unsaved changes 的 Save/Discard/Cancel 对话框。
7. Escape 取消，窗口、草稿与 `*` 均保留。
8. Cmd+S 后 `*` 消失、出现保存提示、代码面板仍显示，没有跳到 Output 或伪造 build ok。
9. Cmd+Q 正常退出；进程检查确认测试二进制不再运行。
10. 磁盘回读含上述 marker，且 `OnSearch = "onSearchSearch"`、
    `OnTextChanged = "onSearchTextChanged"` 保留。

Computer Use 的截图在会话中检查，没有额外保存 PNG，不将其描述为仓库截图产物。
Save/Discard/失败保存等更多分支由逻辑测试补充，不能一概标为逐个手动桌面点击。

## 明确未完成／下一步

1. **统一生成路线最优先。** IDE 保存自动生成的是 `internal/silkgen.GenerateSource()` 的 `decl.Node` 树；
   SDK 强类型事件输出走 `ged.GenerateCodeFile()`。本轮实测的自动 `events.silk.go` 中内嵌代码仍作为字符串属性、
   `events` 为空属性，没有生成上述两个方法及其可执行绑定。
   因此不能把强类型 consumer 通过说成 IDE 默认预览／运行已绑定事件。
   后续应统一生成契约、外部 user.go 导航与用户文件所有权，再验收 IDE Build/Run 整条路径。
2. 新导航操作设计文件内嵌代码，不会自动改写已经存在、由用户拥有的外部 `.go` 文件；
   未补齐完整可视化信号连线或任意布局／跨容器拖放。
3. Windows 只有 `CGO_ENABLED=0 -tags silk_pure_go` 交叉构建，不是原生桌面测试，也不是 Cairo 发布构建。
   本轮没有 Linux 原生验收、真实 IME 或混合 DPI 验收。
4. 本轮未重新进行 gopls 专项集成、CVE 扫描或 CPU/FPS 基准，不能宣称所有语言服务／渲染调度问题均已解决。
   桌面测试中的 gopls workspace 环境报告 Go 1.26.5，区别于本轮应用构建／包测试的 Go 1.26.10，未作工作区工具链升级验收。
5. QA app 是最小开发包，日志存在 `property/ged.cml` 缺失后的默认回退、系统 IMK 提示及事件循环长间隔警告。
   不能凭本轮功能闭环声称发布资源完整、真实 IME 正常或空闲／模态期间调度性能达标；发布前需完整包和专项性能复测。
