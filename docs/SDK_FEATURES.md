# Silk SDK：小工具组装功能清单

本页列出 HTTP／同步异步能力、常用组装入口与设计器生成约定；不是对所有 SDK 模块的完整质量或跨平台认证。
这些新增能力不增加第三方依赖。验收范围见 [HTTP SDK QA](../SDK_HTTP_QA_20261023.md) 、[字体／生成安全 QA](../FONT_GENERATION_QA_20261023.md) 和 [流式传输 QA](../TRANSFER_SDK_QA_20261023.md)。

## 工业 3D 场景基础（开发中）

- `scene3d`：纯 Go 不可变网格、层级节点/快照、Box/Cylinder、受限 OBJ 加载器、
  深度缓冲/ID 拾取与按需缓存的 CPU 参考渲染器；不新增第三方依赖。
- `gui.SceneView`：同一控件树裁剪合成，HiDPI 物理像素渲染与拾取复用；拖拽环绕、
  有界滚轮缩放、点击选择。无新增持续动画/轮询线程或定时器。
- `SetScene` / `SetNodeTransform` / `SetNodeVisible` / `SetNodeColor` 在 UI 线程调用；
  直接编辑 `Scene()` 后需 `RefreshScene()`。`Scene.Snapshot()` 可移交独立后台 Renderer。
- `BindNodePosition` / `BindNodeVisibility` 复用 `BindTagUI` 合并最新 Good 样本；返回的
  取消函数可跨 goroutine 调用。变更父级/场景和 Close 注销订阅，宿主须显式 Close。
- `SetModelSource` 只保存配置；`LoadModel(ctx)` 用既有有界执行器加载，取消/版本检查
  阻止过期模型覆盖新状态，失败保留旧场景；成功替换需重新绑定新节点。
- 设计器 SCADA 面板、矢量图标、相机/源路径持久化、预览和 Go 代码/事件生成接入。
  当前没有完整 Scene/多点位绑定持久化、glTF/PBR 或全平台 UI 验收。
- 可选 `scene3d/glrender` 提供 OpenGL 离屏深度/颜色/ID 渲染和共享网格缓冲。
  `SetRenderBackend` 选择 auto/cpu/opengl；`ActiveRenderBackend` / `GPUInfo` /
  `GPUError` 报告实际能力和回退原因。颜色仍回读至正常控件合成，详见
  [GPU 契约与限制](../scene3d/glrender/README.md)。
- [API 与限制](../scene3d/README.md)、[原生示例](../examples/scene3d/README.md)、
  [CPU 验证](../SCENE3D_QA_20261028.md)、[GPU/原生验证](../SCENE3D_GPU_QA_20261028.md)、[后续路线](../SCENE3D_ROADMAP.md)。

## 有效补全与等待意图

- `CodeEditor.CompletionActive() bool` 区分用户仍在等待补全与候选框当前是否可见；
  `CompletionVisible()` 仍只报告绘制状态。均为 UI 线程访问；宿主必须先拒绝过期文档／光标响应，
  不能用此方法代替异步版本校验。
- 与已输入前缀逐字节相同的字面候选不占行、不吞 Enter／Tab；大小写修正和带 `Apply` 的
  不透明宿主动作保留。`RankCompletions` 的独立排序语义不变。
- 数字后继续补全；空候选等待期间的 Escape、导航、滚动、显式跳转与失焦也取消请求。
  大文档后台结果仍填满最多 50 项有效候选；没有新增轮询或 UI 定时器。
- 字面候选接受使用一次字节事务，保留混合行尾、非法 UTF-8 和未编辑内容；Undo／Redo 恢复
  对应事务状态：Undo 恢复原始光标、选区和附加光标，Redo 恢复接受后的单一插入点。
  历史只持有独立的替换片段，不通过候选子串保留整篇旧文件。
- SilkIDE 对没有附加编辑、且文本与光标都不改变的 LSP 结果进行保守筛除；自动导入、改变范围、
  函数调用文本仍保留。IDE 仍与 gopls 协商纯文本，不宣称新增 LSP snippet 支持。
- 详见 [补全意图 QA](../COMPLETION_INTENT_QA_20261026.md)。

## 查找结果选区与异步导航

- `SetFindQuery` 的定位、Enter／Shift+Enter、F3／Shift+F3 和查找栏前后按钮共用真实
  匹配选区。光标在选区末端；Esc 关闭查找栏后可直接输入替换。跨行 regex 使用实际
  span 端点，零宽匹配只定位光标并可继续前后导航，不制造虚假非空选区。
- 单个匹配导航清除附加光标，不修改文档内容或创建撤销记录；输入后的替换仍走编辑器
  原有可撤销编辑路径。只读文档可以查找／选择，但不允许输入改写。
- 用户手动改变光标后，下一项／上一项按当前位置在有序结果中定位，而不是沿用旧索引。
  排队期间的前后步数累计，新查询作废旧查询的待执行导航。
- 异步扫描交付时核对文档 revision、光标、选区、附加光标、焦点、父级和所属窗口。
  已改变交互状态时只更新仍有效的计数／高亮，不移动新光标或覆盖新选区。
- 自动选中的匹配与打开 Find 时冻结的原始选区分离；启用 Selection 不会悄悄缩成单个命中。
  零命中时只清除本次查找持有的旧选区，不清除用户另选的内容。
- 不新增线程、定时器、轮询或公共 API；复用既有有界搜索 worker。导航快照在 UI 事件
  中取得，有附加光标时复制其位置。详见 [查找导航 QA](../FIND_NAVIGATION_QA_20261026.md)。

## 弹窗输入归属与光标菜单锚点

- GLFW 后端统一同窗口／跨窗口按钮分发，按下控件持有该次抬起事件，不因指针移到
  其他菜单行就重新选择接收者；实时命中检测更新 hover，支持没有前置 MOVE 的点击，
  拖出后松开不误触发原按钮。显式叶控件 capture 不被其他窗口事件改派。
- `CodeEditor` 和 `Edit` 的 `Shift+F10` 复用 `InputMethodCursorRect()`，在可见光标下方
  打开菜单，锚点限制于自身视口；小于 gutter 的视口、1×1 和零尺寸也不越界。
  普通 `Edit` 需实际拥有焦点；重复按键、其他修饰键不重复打开／抢占菜单。
- 本次不新增公共 API、定时器、后台线程或轮询。Windows 原生鼠标分发保持原实现，
  两类文本控件的键盘锚点逻辑跨平台共享。详见 [输入路由 QA](../INPUT_ROUTING_QA_20261026.md)。

## 编辑器上下文菜单与可撤销选区删除

- `CodeEditor` 实现 `IContextMenuProvider.BuildContextMenu`；默认菜单包含 Undo/Redo、
  剪贴板、选区 Delete、全选、多光标及符号操作，每个命令均有已有图标系统的图标。
- 右键按下定位／保留选区，抬起才打开；`Shift+F10` 在光标位置打开，按键重复不重复弹出。
  每个编辑器复用一个自有 popup，关闭时清空命令行，隐藏编辑器时关闭并作废快照。
- 动作执行前、恢复焦点后都核对文档 revision、文件路径、父级／窗口、光标／选区、只读状态
  及菜单版本。已替换的文档或宿主回调不被旧菜单重新定位。自定义 builder 的回调由宿主自行校验。
- `CanDeleteSelectedText() bool` 是无分配、无剪贴板读取的状态查询；`DeleteSelectedText() bool`
  仅删除非空选区，一次 Undo 恢复原始 bytes、选区方向与光标；没有选区时不删除当前行。
  与旧的低级 `DeleteSelection` 不同，新入口自行记录历史。只读／禁用时返回 `false`。
- `CodeEditorShortcuts()` 增加历史与符号导航提示，普通 Edit 也复用 Undo/Redo 提示。
  `CodeEditorSymbolActions.RenameShortcut` 只描述宿主绑定，不安装快捷键；宿主提供 rename
  回调却未提供此字段时，不显示误导性的默认提示。SilkIDE 填入其实际 `F2` 绑定。
- 菜单创建时翻译，SDK 自带简体中文表；宿主仍可覆盖。不会在每帧翻译或增加计时器、线程、
  轮询。打开菜单只检查光标附近的标识符存在性，不复制整行或访问 LSP／剪贴板。
- 详见 [编辑器上下文菜单 QA](../EDITOR_CONTEXT_QA_20261026.md)。

## 组合字符排版与一致的命中几何

- `paint.LayoutText` 的 `TextLayout.Clusters` 提供不可拆分的视觉簇，`TextCluster.Begin/End`
  仍是源字符串 rune 偏移。`Boundary(pos, trailing)` 将簇内位置对齐边界；
  `WrapEnd(start, end)` 调整换行，超宽首簇独占一行，不重复绘制。
- `paint.NeedsClusterLayout(text)` 查询当前后端是否有适用的组合排版路径；不是完整
  Unicode、双向文本或输入法能力查询。macOS + cgo + Cairo Quartz 才启用此次实现；
  Windows、Linux、`silk_pure_go` 及无 Quartz 字体后端的构建保持原路径。
- 组合序列通过 Core Text 整形并持有实际 `CGFont` 对应的 Cairo 字体，不将其他字体的
  glyph ID 塞入按 family 名猜测的字体。保留实际小数字号；失败时兼容回退，不删源字节。
- 编辑器的鼠标定位、插入点锚点与高亮片段共用整行几何；TextBlock/查找字段换行不拆簇。
  文档偏移、LSP 偏移和撤销历史仍按现有逻辑处理，尚非完整字素簇键盘编辑。
- 与既有 paint 字体缓存一致，只在 UI/渲染线程调用。布局缓存上限 128 项、约 1 MiB
  文本/坐标/glyph 负载，原生簇缓存上限 256 项、约 256 KiB；这些不是进程总内存上限。
  对外返回独立布局副本；不新增线程、定时器或后台轮询。
- 验证范围及平台边界见 [组合文字 QA](../COMPLEX_TEXT_QA_20261026.md)。

## macOS 原生字符选择器

- `gui.CharacterPaletteAvailable()`：当前仅 macOS + cgo 返回 `true`；
  Windows／Linux 返回 `false`，不改动系统输入设置。
- `gui.ShowCharacterPalette(target)`：UI 线程调用，目标必须已经是当前窗口的
  可见、启用、挂接的文本输入控件。不抢焦点、不读剪贴板，不直接插入文字。
- 返回 `true` 只表示已向系统发出请求，不保证面板最终显示或用户选择了字符。真正提交
  仍经过控件的输入校验和历史记录；只读行为由控件正常输入路径决定。
- 标准快捷键为 `Control+Command+Space`，宿主显式绑定与正在组合的 IME
  优先；按住快捷键不会反复调用。后台／隐藏／最小化／错误 responder 不打开面板。
- 从上下文菜单调用时，先关闭菜单、恢复原文本目标的既有焦点，再请求；
  不为菜单另开字符浮层。实现不增加定时器、后台线程或轮询。

验收范围与自动化 Unicode 注入限制见 [原生输入 QA](../NATIVE_INPUT_QA_20261026.md)。

## 属性面板的实时编辑与事务撤销

`prop.TextEdit / IntEdit / FloatEdit` 保留逐次有效输入的实时 setter 与画布更新，
但把同一字段的一段连续编辑合并为一次设计器 `Undo/Redo`；不改变 `gui.Edit`
自身的局部文本历史语义，也不把所有程序化 `PropertyItem.SetValue` 合并。

- Enter、真正离开字段、刷新、隐藏／禁用／只读切换结束当前编辑；字段自己的
  右键菜单不算离开。长按 Up/Down 合并重复步进，松键后下一次独立记录。
- 保存点、撤销／重做后的历史分支不跨界合并。多选对象保留各自初值；setter
  归一化后的值用于后续比较，外部模型变化不会被吞进上一段编辑。
- 不变值、未触碰的多值占位与过期属性行不产生写入；禁用或隐藏的祖先也阻止写入。
- 所有操作仍属于 UI 线程；没有添加轮询、定时器、goroutine 或独立子窗口。
  完整行为及未覆盖平台见 [属性编辑事务 QA](../PROPERTY_EDIT_SESSION_QA_20261025.md)。

## 设计器选中状态与画布历史

- `ged.FormatItemBounds(item)`：格式化已提交的场景坐标与尺寸，保留小数，
  使用 10 位有效数字抑制浮点运算噪声；不修改模型。返回值不带 `mm` 后缀。
- `ged.SelectionItemName(item)`：显示 `对象名 [控件类型]`，未命名时显示类型；
  不将 `FakeWidget` 内部适配器名称泄漏给用户。
- `GedView.SetSelectionStatusFormatter(fn)`：宿主统一定制／本地化状态文字，
  `nil` 恢复 SDK 默认，返回空字符串可隐藏消息。回调在 UI 线程执行，不能改变选区。
- `GedView.RefreshSelectionStatus()`：命令、选择与拖拽落点自动刷新；宿主直接调用
  模型 setter 后可显式刷新。隐藏／关闭状态静默，相同文字不重复重绘；无后台轮询。
- `GraphView` 内置撤销／重做 action 保留 UndoStack 的禁用状态、图标、文字和版本，
  优先调用派生视图的 `Undo/Redo`，使设计器的待提交编辑、属性面板和代码生成同步。
  普通 GraphView 继续直接使用栈；场景替换后旧 action 不再修改旧文档。

状态栏显示已提交的几何，不保证拖拽过程中每个临时预览帧都更新。
状态通知与内层工具栏验证见 [设计器状态与画布历史 QA](../SELECTION_STATUS_QA_20261025.md)；
该阶段遗留的整数取整行为已由下面的精度修复替代。

## 精确移动与显式吸附

- `graph.MoveCommand.AddItem(item, x, y)` 的目标坐标原样提交与历史重放；
  不再隐式取整到 1 mm。SDK 调用方如果需要网格吸附，应先计算吸附后的目标。
- `Selection.GenerateMoveCommand`／`graph.AddMoveSubtree` 仍按原有父子坐标边界
  移动整棵子树；选中父子时不重复平移，锁定的后代仍随可移动容器一起移动。
- 设计器方向键每次平移 1 mm，保留起点小数及未移动轴；Shift 使用粗移步长。
  对齐、居中与等间距分布保留精确计算结果，不强制落到整数坐标。
- 鼠标拖拽继续遵循现有显式网格／参考线策略；关闭吸附后不再发生隐藏的整数化。
  小数网格（例如 2.5 mm）落点在 Undo／Redo 后保持不变。
- 这些 API 仍由 UI 线程调用；不新增计时器、后台任务或渲染循环。
  验证与未覆盖场景见 [精确几何 QA](../PRECISE_GEOMETRY_QA_20261025.md)。

## 场景命令与属性面板同步

- `GedScene.PushCommand` 成功执行非属性输入命令后，通过既有设计变更链刷新
  绑定面板；`NotifyDesignChanged()` 也适用于宿主直接改变模型后的显式通知。
  画布 Undo／Redo 共用这条链，不再重复刷新。
- `prop.PropertyCommand` 的实时输入不在 Push 中反向刷新字段，避免将 `2.`
  改写为 `2` 或打断连续输入的撤销合并。Undo／Redo 后仍刷新实际值。
- `PropertySheet.RefreshValuesForOwner(owner)` 是共享面板的可选能力：只有
  非 nil 且与当前绑定 owner 相同的通知才刷新；nil 不是通配符。`Clear` 在
  控件脱离前使 owner 失效，旧文档无法借重入焦点回调更新新绑定。
- 刷新复用原属性控件，保留筛选／分组状态；隐藏、关闭、场景替换及拖动中间态
  不发布过期值。隐藏视图再次显示时补齐；拖动结束仍刷新最终吸附后的结果。
- 自定义面板可实现新 owner 接口；仅实现原 `RefreshValues()` 的面板仍兼容，
  但应自行管理共享绑定的生命周期。所有入口均要求 UI 线程，不新增轮询或线程。

验证范围及限制见 [属性命令同步 QA](../PROPERTY_COMMAND_SYNC_QA_20261025.md)。

## 嵌套弹出菜单的原生焦点交还

GLFW 后端关闭整条菜单链时，将焦点交还安排在隐藏后代之前：鼠标选中的深层
子菜单也计入该关闭链，直接激活仍存活的 owner，而不是先激活马上消失的一级菜单。
逻辑焦点仍由已有 Hide／capture 清理链恢复，菜单 action 继续在弹层隐藏后执行。

- 仅沿原生 popup owner 链判断，不把逻辑上挂在菜单下的独立 Form 混入同一窗口。
- 保留 UI 线程、owner 可见／启用／未最小化、应用激活与焦点 generation 校验；
  后台窗口关闭、其他窗口／新弹窗取得焦点时不会强制抢回旧 owner。
- 不修改菜单对外 API，不改变 Windows 后端实现，不增加定时器、轮询或 goroutine。
  macOS 原生修复与 Windows 兼容回归范围见 [嵌套菜单焦点 QA](../MENU_KEYBOARD_QA_20261025.md)。

## 代码编辑器连续输入与历史边界

`CodeEditor.OnTextInput` 将直接单字符输入明确标记为可合并：相邻输入间隔小于
500 ms 时，选区替换的首字符与后续字符共同组成一个 Undo，不再留下首字符。
内部仍使用字节锚点，保留被替换的 Unicode、无效 UTF-8 字节及混合换行原文。

- 剪贴板粘贴、多字符提交、自动括号／包裹、补全和其他编辑命令不冒充直接打字；
  后续输入不会把这些原子操作吞进同一历史记录。
- `MarkSaved`、Undo／Redo、光标导航、鼠标点击、焦点切换结束已有连续输入。
  宿主仍应仅在成功加载／保存后调用 `MarkSaved`，不会新增历史状态通知或轮询。
- `OnTextInput("")` 是无操作，不删除选区、不修改 revision，也不清除重做历史。
- 多光标仍使用既有逐次原子事务；本轮不把多光标、跨行、长停顿输入统一合并。
  API 仍限 UI 线程，无新增依赖、定时器或 goroutine。

原生 UI、自动测试、性能采样和输入路径限制见 [连续输入 QA](../EDITOR_TYPING_QA_20261026.md)。

## 能力索引

| 场景 | 入口 | 说明 |
| --- | --- | --- |
| 同步执行可取消工作 | `async.Run[T]` | 不创建 goroutine；捕获 work panic 为 `PanicError` |
| 有界后台任务 | `async.NewExecutor`、`async.Submit[T]` | 限制同时运行任务；满载立即返回 `ErrBusy`，没有无界等待队列 |
| 等待／取消／关闭 | `Future.Await/Cancel/Done`、`Executor.Close/Wait` | 合作式取消，不能强杀忽略 Context 的用户函数 |
| 完成通知 | `Future.OnComplete` | 显式指定 dispatcher；没有每次订阅的 watcher goroutine |
| GUI 异步包装 | `gui.RunAsync[T]` | 后台 work + `gui.Post` 主线程完成通知 |
| 剪贴板异步交付 | `Clipboard.ReadTextAsync`、`Widget.PastePending/CancelPendingPaste` | UI 线程调用；macOS 后台 helper 有界读取，其他平台不保证读取非阻塞 |
| 可取消文本比较 | `DiffView.SetTextsAsync/CancelDiff/DiffPending/DiffError/DiffSummary` | UI 线程配置，后台精确比较；关闭宿主时取消，旧同步 SetTexts 保留 |
| 通用 HTTP | `httpx.Client.Do/DoAsync` | Method、URL、Header、Body；返回有大小限制的 Response |
| GET | `Client.Get/GetAsync` | 同步／异步对称入口 |
| JSON POST | `Client.PostJSON/PostJSONAsync`、`httpx.JSONRequest` | 自动编码请求体并设置 JSON Content-Type / Accept |
| 字符串／字节 | `Response.Text/Bytes` | 整个响应；Bytes 和 Header 返回副本 |
| JSON 字段 | `Response.JSON/Str/Int/Float/Bool` | JSON Pointer，严格类型，缺失／格式错误不冒充零值 |
| 通用结果转换 | `Response.Extract(kind, pointer)` | `str/json/int/float/bool`，适合配置驱动的小工具 |
| 可视请求按钮 | `gui.HTTPButton` | 显式点击才请求；忙碌时再点击取消；UI 线程结果信号 |
| 声明式组装 | `decl.HTTPButton` | 使用 `decl.P` 设置属性；Build 不联网 |
| 设计器组装 | 网络与工具 → `gui.HTTPButton` | 拖放、属性保存／重载、预览构造、强类型导出、6 个事件入口 |
| 复用 JSON 文档 | `jsonx.Parse/ParseWithOptions`、`Response.Document` | 一次解析，多次类型化查询；精确保留整数；有界输入 |
| 后台 JSON 解析 | `jsonx.ParseAsync` | 有界执行器、输入快照、显式完成派发 |
| 结构化 JSON 控件 | `gui.JSONView`、`decl.JSONView` | 类型列、字段定位、分组展开、复制路径／节点值；错误保留旧数据 |
| JSON 工具示例 | `examples/json-inspector` | 本地 HTTP → 后台解析 → UI 队列 → 树查看，支持取消 |
| 流式接收 | `Client.DownloadTo/DownloadToAsync` | 固定复制缓冲区、Writer、超时／取消、大小限制、SHA-256 校验 |
| 高频进度合并 | `async.NewLatest[T]` | 每个实例至多一条排队／正在派发的更新；关闭后抑制未开始回调 |
| 传输状态控件 | `gui.TransferProgress`、`decl.TransferProgress` | 字节数、未知总量、显式结果状态；无轮询、无动画心跳 |
| 下载示例 | `examples/download-monitor` | localhost 分块下载、取消、错误／校验失败；写入 io.Discard，不创建文件 |
| 可运行示例 | `examples/http-tool` | URL、方法、返回类型、JSON 字段、请求体、结果编辑器、错误／取消状态 |
| 编辑器后台补全索引 | `gui.CodeEditor`、`CompletionIndexLoading()` | 大于 32 KiB 的完整文档后台收集／排序；只保留最新请求，静置无索引定时器 |
| 编辑器补全生命周期 | `CodeEditor.CancelCompletion()` | UI 线程调用；关闭候选并取消后台工作、释放索引；下次触发可复用编辑器 |
| 编辑器文件类型 | `CodeEditor.Language()`、`SetFilePath()` | 自动识别 Go／SilkGo／JSON／普通文本；切换类型清理旧投影与后台候选，不修改正文 |
| 离线文本格式化 | `formatx.Format/FormatAsync` | Go 标准格式化、JSON 美化／压缩，不调用外部程序；输入／输出／深度限制 |
| 编辑器后台格式化 | `CodeEditor.FormatDocumentAsync/CancelFormat/Formatting` | 有界任务、版本校验、UI 派发；保留历史并合为一次 Undo |
| 缩进配置 | `CodeEditor.InsertSpaces/SetInsertSpaces`、`TabSize/SetTabSize` | 默认 Tab；空格模式 Tab 对齐下一逻辑制表位，宽度 1–16 |
| 行缩进事务 | `IndentLines/OutdentLines`、`IndentSelection/DedentSelection` | 保留原始字节、选区方向及历史；一次 Undo 恢复整组，重复光标所在行只修改一次 |

### 原生文本输入作用域

GLFW 与 Win32 的文本输入共用逻辑焦点筛选：焦点控件本身以及到所属原生窗口为止
的每一层控件均须可见且启用。脱离窗口、隐藏、禁用或属于无关窗口的旧焦点不接收
字符；没有有效焦点时也不会回退到窗口根编辑器。

- 非激活弹窗保留 owner 链路由，不要求为了输入强制激活新的系统窗口。
- 独立 `WtForm` 是边界：即使逻辑上有父窗口，也不继承另一个原生窗口的控件状态。
- `SetFocus()` 不会自动显示／启用控件；宿主应在显示并启用正确输入页面后设置焦点。
- 窗口级按键处理仍可回退到窗口根对象，和必须具有有效焦点的文本输入不同。
- 检查仅发生于输入派发，不新增计时器、轮询或线程。24 层祖先查找的分配回归为零。
- GLFW 丢弃控制字符及无效 Unicode 标量；Windows 保留现有 UTF-16 代理项与 IME 路径。

### 可观察的导航历史

`gui.NavigationStack` 零值可用，保存最多 256 个位置；所有访问均由 UI 线程负责，
首次使用后不要复制。`RecordJump`、`Navigate` 仍负责跨文件／实时光标的后退与前进，
新增以下宿主绑定接口：

- `Len()`：已保留的位置数（含当前位置），不探测磁盘文件是否可用。
- `Clear()`：释放位置记录，保留订阅；不触碰编辑器文本、光标、dirty 或 Undo。
- `SubscribeChanged(fn) func()`：返回幂等退订函数。回调在完整变更后按注册顺序调用；
  `RecordJump` 的两端和 `MapPaths` 的所有路径只发布一次通知。没有初始通知，
  绑定后应主动读取 `CanGoBack/CanGoForward/Len`。

失败导航、边界操作、完全相同的 `Push`、空 `Clear` 不通知。回调必须简短且不得
同步修改历史；回调内订阅／退订安全，退订立即生效，新订阅从下次变更开始接收。
宿主卸载时调用退订函数。位置更新不创建定时器、后台轮询或每次回调的切片副本。

```go
refresh := func() {
    back.SetEnabled(history.CanGoBack())
    forward.SetEnabled(history.CanGoForward())
}
stop := history.SubscribeChanged(refresh)
refresh()
// 窗口／工具栏销毁时调用 stop()。
```

可用状态表示记录中存在相邻位置，不承诺文件仍在磁盘上；`Navigate` 通过宿主的
`visit` 返回值跳过不可打开的目标。IDE 的菜单、工具栏和命令面板统一复用该历史，
从 Welcome／Design 也能返回目标代码页；工具栏重建及退出会退订并清理提示。

### 文件查询与行列定位

`locator.ParseFileQuery(text) (FileQuery, error)` 将文件搜索文本拆分为 `Path` 与
可选的 `Location`（从 1 开始的 `行号[:列号]`）。支持 Windows 盘符／UNC、相对路径、
Unicode 文件名和带空格的路径；含数字尾缀冒号的文件名需要加双引号，例如
`"report:2026"` 搜索字面文件名，`"report:2026":3:2` 才表示附带位置。

```go
query, err := locator.ParseFileQuery("gui/edit.go:120:4")
if err != nil {
    // errors.Is(err, locator.ErrFileQuerySyntax)；不要使用失败的目标。
    return
}
// 对 query.Path 做宿主自己的索引匹配和路径约束。
// 只有拿到实际目标编辑器以后，才对它的实时缓冲区定位：
if query.Location != "" {
    err = editor.NavigateToLocation(query.Location)
}
```

解析器不做 I/O、路径授权、环境变量展开或文档范围校验；空文本是合法的空搜索。
输入上限 `MaxFileQueryLength` 为 4096 字节，位置部分最多 64 字节；不完整引号、
空位置、零、负数和数字溢出均不可作为可执行目标。Windows `C:`／`C:123` 保留
盘符语义；普通非数字冒号可以是文件名的一部分。

IDE 快速打开复用该解析器及现有有界异步索引，接受已索引文件的绝对路径。
列号按 Unicode code point，而非字节／UTF-16／制表符视觉宽度；使用未保存文本，
只读文档也能定位。成功后加入工作台后退／前进历史，不自动保存或修改文本。
范围错误会打开或激活目标文件以展示警告，但不移动其光标，也不新增导航记录。

### 搜索输入框与结果列表联动

`Edit.SetNavigationList(list)` / `NavigationList()` 将单行输入框与同一控件树、
同一原生窗口内的 `ListWidget` 组合。未按修饰键的 Up／Down／PageUp／PageDown
移动高亮行并滚动可见，但焦点、查询文本、反向选区和输入框光标不变；接着打字
仍输入查询。重复按键可连续导航。Home／End、Tab、Enter 及带修饰键操作保留
原有输入框语义，多行输入框不会将行导航转发给结果。

```go
input, results := gui.NewEdit(), gui.NewListWidget()
box := gui.NewVBox()
box.AddWidget(input)
results.SetFillAvailable(true)
results.SetSelectionVisible(true)
box.AddWidget(results)
input.SetNavigationList(results)
input.SigSubmit(func(_ interface{}, _ string) {
    if index := results.ActiveIndex(); index >= 0 && index < results.Count() {
        // 使用 results.Item(index).Data 执行宿主动作，并重检结果是否仍有效。
    }
})
// 关闭搜索控制器或不再需要联动时解除引用。
input.SetNavigationList(nil)
```

不接管筛选、提交或选区回调；隐藏／禁用／分离／移到其他窗口的目标不导航。
原生对话框构成可见性边界，隐藏的逻辑宿主不会禁用已经显示的对话框输入。
SetNavigationList 及关联控件仅限 UI 线程；新增接口无轮询、定时器、goroutine
或静置扫描。IDE 的三个查找器共用此接口，异步查询刷新时仍立即清空过期结果。

### 窗口 backbuffer 生命周期

窗口的缓存画笔和表面由 UI 线程独占：尺寸变化时先释放画笔再释放表面，缺失表面
即使缓存尺寸相同也必须重建；同尺寸帧保留像素与对象。GLFW 保持精确物理尺寸，
Windows 保持既有分配粒度，不以缩放纹理代替正确布局和步长。

GLFW 同窗口递归 paint 延后为下一帧全量更新；Draw 回调中关闭 owner 的原生资源
直到当前帧结束才退役。无需应用新增 Close 调用，不修改 Painter／Surface 接口，
无新增轮询或后台任务。资源复用基准约 1.95 ns、零分配，不代表整帧绘制零分配。
[回归与平台边界](../BACKBUFFER_LIFETIME_QA_20261025.md)。

### 菜单原生表面与模态焦点

默认菜单按真实原生 framebuffer 能力绘制：支持 alpha 的 GLFW popup 保留圆角、
预乘 RGBA 与完整内缩边框；不支持时完整填充不透明矩形，避免黑角或残留像素。
当前 Windows GDI 路径使用方角回退，不等于新增 Windows 每像素透明窗口支持。
透明 hint 只属于 popup，每次创建普通窗口均显式关闭；图表叠加独立计算 alpha。

`ged.ShowAboutDialogWithInfo` 由底层 `ShowModal` 先保存所属窗口的当前焦点，再激活
Close；调用者传隐藏的设计器面板也不会抢走其兄弟编辑器的返回焦点。不覆盖回调
主动转移的新焦点。既有 Menu 键位不变：Left 返回父菜单，Escape 关闭整条弹出链。

无新增计时器／轮询／goroutine。macOS 原生像素及 Close／Enter／Escape 后直接输入、
保存、撤销读回已验收；Windows 在后续资源生命周期轮次恢复连接并补验原生菜单、
About 三种关闭后的输入与撤销，记录了跨任务焦点前置条件的未决问题；Linux 原生未测。
[初次验证](../POPUP_SURFACE_QA_20261025.md) · [Windows 补验与边界](../BACKBUFFER_LIFETIME_QA_20261025.md)。

### Edit 字段菜单与局部历史

默认 `Edit` 支持右键和 Shift+F10 菜单：撤销、重做、剪切、复制、粘贴、删除、
全选。菜单使用现有矢量图标和平台快捷键提示；只读字段保留复制，修改类动作禁用。
`CanCopy/CanCut/CanPaste/CanSelectAll/CanUndo/CanRedo` 只查询字段状态，不同步读取
系统剪贴板；`CanPaste` 表示允许粘贴，不保证剪贴板当前含有可用文本。

- `Undo/Redo/ClearUndoHistory` 仅限 UI 线程。历史只保留变化的 UTF-8 片段，最多
  256 条、1 MiB 文本载荷；超大操作清空历史，不留下可跨越的修改缺口。
- 一次已提交输入／粘贴为一条历史；不合并连续打字。撤销恢复原选区方向和光标，
  重做恢复结果光标，各发出一次文本变化及用户编辑通知。选区移动不是历史条目。
- 成功 `SetText`、约束变化和旧的 validator fixup 路径重置历史；被校验器
  拒绝的 `SetText` 不清除历史。只读／禁用时保留历史，但禁止执行撤销／重做。
- `CopySelection() error`、`Cut() error` 报告系统写入错误；既有 `Copy()` 保持
  void 签名。失败剪切不删除文本；原生回调中内容／选区已变时也不执行旧的删除。
  不先清空剪贴板再写入。复制／剪切仅针对选区，不采用 CodeEditor 的整行语义。
- `Paste()` 使用已有异步粘贴路径；输入和历史重放取消过期请求，迟到结果不得写回。
- `HandleShortcut` 只接管字段自身 Undo/Redo，空历史也不向宿主文档穿透。
  自定义派生控件若覆盖此方法，需按自身交互契约显式转发；不自动改变现有覆盖行为。
- 菜单在原控件上懒创建、复用，关闭释放临时动作；执行前校验文本 revision、选区、
  owner 和可见／启用状态。`AttachContextMenu` 自定义构建器优先，其次动态调用
  派生控件的 `IContextMenuProvider`。没有全局菜单扫描、轮询或新后台线程。

菜单打开期间仍绘制原字段的选区高亮，但不绘制失焦字段的光标；菜单关闭／迁移后
不保留旧高亮。这不是 CodeEditor 文档历史，也没有新增密码回显模式。成功／失败
剪切使用注入 writer 的回归测试，不声称直接验证了用户系统剪贴板写入。
[初始验证边界](../EDIT_MENU_QA_20261025.md) · [补全与派生控件跟进](../EDIT_COMPLETION_QA_20261025.md)。

### 保留字体归属的文本布局

`paint.LayoutText(scaledFont, text)` 返回 `TextLayout`：

- `Carets`：每个 rune 边界的浮点 X 坐标，包含末尾边界，不再截断为整数宽度。
- `Runs`：每段 `GlyphRun.Font`、`Glyphs` 与 rune 范围 `Begin/End`；字体为独立副本。
  字形索引只属于对应字体，不可合并后拿单一字体绘制；布局存活期间应将返回数据视为只读。
- 保留原有 `Font` / `ScaledFont` 接口，第三方实现无需新增方法。异常 glyph/rune 数量
  不相等时走线性的逐 rune 兼容路径，不再静默丢弃整个文本块。
- 和已有绘图 API 一样由 UI／渲染线程调用；未新增跨线程共享字体缓存的保证。

`gui.Edit` 的绘制、选区、点击命中、水平滚动和输入法光标矩形共用此布局。
换行只切分保留的字形段，重绘不重新查询字体覆盖或 shape 文本；光标导航不重新排版。
末尾 CR 不再与内部换行哨兵误组成 CRLF，保留最后一个空白行；文档原始内容不变。

这是简单 LTR／rune 布局，并非完整 bidi、grapheme 或 ZWJ 合字引擎；系统必须安装
覆盖所需码点的字体。当前新增路径已通过 macOS 原生验收，Windows 本轮因测试机
SSH 不通尚未验证，Linux 原生 UI 也未验证。详见 [字形与布局 QA](../TEXTBLOCK_FALLBACK_QA_20261025.md)。

### Unicode 字段补全事务

`Edit.SetCompletionPrefixStart(n)` 保持 **UTF-8 字节偏移**接口；光标／选区仍为
rune 偏移。内部转换使用活动光标，不把选区锚点当光标；落在多字节字符内的起点
向前对齐到该字符开头，负值／超出光标的起点表示光标处空前缀。

`CompletionPrefix()` 只转换所需前缀，不复制光标后的长正文。
`AcceptCompletion(index)` 现在遵守只读、禁用、最大 rune 长度及 validator，
拒绝无效 UTF-8 候选；失败不修改文本、选区、历史或发出编辑通知。成功补全保留
光标后的后缀，是一个局部历史事务，发出一次 changed／edited 通知对；Undo/Redo
同时恢复补全起点。成功 `SetText` 重置补全起点到 0；旧的直接 validator fixup
仍清除历史。候选显示由宿主负责，不自动新增补全 popup 或后台搜索。

运行 `go run ./examples/edit-completion`：包含完整候选按钮、局部历史、只读、
重置、菜单及 JSON 状态快照。[示例说明](../examples/edit-completion/README.md)。

### ComboBox 选择模式与弹窗契约

```go
choice := gui.NewComboBox() // 默认保持可编辑，兼容自由文本输入。
choice.SetEditable(false)
choice.SetMaxVisibleItems(10) // 首选可见行数；工作区较小时会进一步缩小。
choice.SetItems([]string{"Dark", "Light", "简体中文"})
choice.SetActiveIndex(0)
choice.SigSelectionChanged(func(_ interface{}, index int) {
    fmt.Println(index, choice.Text())
})
```

- `SetEditable(false)` 显示已提交项的文字／可选图标，不把旧编辑草稿当成选择；
  `SetEditable(true)` 创建默认编辑器并填入已提交文字。仅在 UI 线程调用。
- 非编辑模式的 `SetText` 只接受**完全匹配**的现有项；没有匹配项时不改变选择。
  `SetActiveIndex` 仍是按索引选择的入口，`editable` 也出现在属性枚举中。
- `EditWidget()` 的接口仍为 `IEdit`，默认实例现在是内部 `Edit` 派生类型，
  不应断言其具体类型为 `*Edit`。`SetEditWidget` 继续支持自定义编辑器；自定义编辑器
  不会自动获得默认编辑器新增的方向键转发，宿主需自行处理。
- F4／Alt+Down 打开；关闭的非编辑字段也可用 Space 打开。打开时方向键、
  Home/End/PageUp/PageDown 和鼠标 hover 修改可见的**待确认项**，不提前通知选择。
  Enter/Space 或有效行点击提交；Esc／Alt+Up／F4 关闭不提交。
  Tab／Shift+Tab 取消预览并离开组合字段，不进入弹出列表的滚动条。
- `MaxVisibleItems` 默认 10；`SetMaxVisibleItems(n)` 将小于 1 的值归一为 1，
  不截断数据。设计器属性为 `max_visible_items`。展开中调整保持待确认项可见，不提交。
- 弹出前约束实际列表视口及原生窗口，按当前显示器工作区决定向上／向下展开；
  长列表复用滚动条和可见行绘制，避免创建全部条目总高度的离屏 surface。
- 默认 AutoFocus 下，可编辑组合框只在真实输入框停靠一次 Tab，避免外框先吞掉文本。
  显式 `TabFocus`／`NoFocus` 策略仍优先；非编辑模式恢复组合框自身停靠。
- 关闭时方向导航直接选择；默认可编辑输入框的 Home/End 和文本选择仍控制光标。
  Unicode 前缀匹配通过 `OnTextInput` 消费平台已提交文字，字符不在 KeyDown 再匹配一次；
  重复首字母循环匹配项。这不是多次按键累积搜索／模糊搜索，也不改变 IME 实现。
- 先关闭旧 popup，再发选择／提交回调，避免回调新打开的 popup 被旧流程误关。
  空白区域、隐藏／禁用控件不提交。弹出框取得逻辑焦点但不强制激活新原生窗口。
- 标签单项测量缓存随文字、字体和宽度变化失效；只在事件中查找，不增加计时器、
  轮询或 goroutine。文字裁剪与箭头区域隔离；没有图标的列表不预留空图标列。

macOS popup 在隐藏前把原生焦点归还仍存活的直接 owner，避免模态设置窗口落到
主窗口后面；切到别的应用／窗口时不抢焦点，不新增计时器或后台线程。

**验证边界：** 视口改动见 [Popup Viewport QA](../POPUP_VIEWPORT_QA_20261025.md)，
鼠标焦点根因、macOS 保存读回及 Windows 分项结果见
[Native Mouse QA](../POPUP_MOUSE_QA_20261025.md)。内部索引导航通过不等于末项在
屏幕上可见；开发中能力不应当成已经完成跨平台验收的发布版本。

可运行 `go run ./examples/combobox`。示例涵盖 100 项列表、图标、中文／emoji、
自由文本与可读的状态快照。验证范围见 [ComboBox QA](../COMBOBOX_QA_20261025.md)。

### 双行列表与独立状态提示

`ListWidget.SetItemPresenter(func(index int, item ListItem) ListItemPresentation)`
可选择启用双行结果：`ListItem.Text` 保留主标题，`SecondaryText` 为第二行说明，
`TrailingText` 为右对齐状态；标题和状态分别测量／省略，不修改原始数据。
`Muted` **仅表示视觉弱化，不禁止激活**，宿主仍须检查动作是否可用。

```go
results.SetItemPresenter(func(index int, item gui.ListItem) gui.ListItemPresentation {
    return gui.ListItemPresentation{
        SecondaryText: "完整说明保存在业务数据中",
        TrailingText:  "只读",
        Muted:         false,
    }
})
// 外部业务状态变化后显式失效；不要从 presenter 里修改列表。
results.InvalidateItemPresentation()
// 恢复原有单行样式。
results.SetItemPresenter(nil)
```

回调只在 UI 线程为可见行缓存缺失时调用，必须保持无副作用。缓存最多 128 行；
列表修改、尺寸、字体／主题变化自动失效，外部状态变化使用显式失效接口。
行高容纳两行字体，绘制和鼠标命中共用 padding／滚动视口边界，文字不覆盖相邻控件。
不新增后台线程、定时器或全量逐帧测量。旧 `ListItem` 结构和默认单行用法保持兼容。

IDE 命令面板以只读 `Edit` 补足完整说明，可换行、滚动和选择复制；行内省略不是
丢弃业务文本。选择、筛选和提交事件更新状态及计数，不在静置时轮询可用性。
见 [命令面板回归与边界](../PALETTE_DETAILS_QA_20261025.md)。

### 可操作提示与有界通知面板

`ShowToastWithOptions(parent, message, ToastOptions{...}) *Toast` 仍在宿主窗口
内部绘制，不创建独立原生窗口、不获取焦点。`ToastBottomRight` 限定在内容下半区，
避开 Frame 工具栏／状态栏；过小空间隐藏而非挤到顶部。`ToastTopCenter` 是默认位置，
原 `ShowToast` 签名及顶部位置保留。每个宿主最多 8 条活跃提示，超额淘汰最旧项。

```go
banner := gui.ShowToastWithOptions(frame, "请求完成，点击查看结果", gui.ToastOptions{
    Level: gui.ToastSuccess, Position: gui.ToastBottomRight, DurationMs: 2500,
    OnActivate: func() { /* 在提示销毁之后打开详情 */ },
})
// nil、到期或已销毁均安全；不会重复触发回调。
banner.Dismiss()
```

Options 的零时长使用 2500 ms；原 ShowToast 的时长语义不变。关闭 × 只关闭，
不执行 OnActivate；提示正文点击会被消费，不透传给下方控件。

`NotificationPanel` 新增 `SetLimit/Limit`（0 为兼容的无限制，正数保留最新 N 条）、
`SelectedIndex/SelectedItem/Select`、`UnreadCount/MarkAllRead`，以及
`SigItemsChanged/SigSelectionChanged`。`Items()` 返回分离快照，不能通过改切片
悄悄修改内部模型。Select 标为已读、滚动可见，但不触发 SigClick；鼠标／Enter／
Space 激活，方向键／Home／End／PageUp／PageDown 导航。缩减容量和删除行维护选区，
回调中清空列表不会再触发旧行。所有调用及回调均限 UI 线程；后台结果先经 `gui.Post`。

显示只处理可见行，标题／时间／消息预览先限制到 256 个 rune，再按宽度省略；
控制字符变空格，完整消息仍可通过 SelectedItem 获取。SDK 的单条消息本身不截断。
IDE 组合此面板与只读详情框，额外施加 200 条／每条 64 KiB 上限，历史仅保留本次
会话，不写盘。工具按钮自动换行，窄面板上下分栏。没有常驻刷新定时器或后台轮询。
验证与未覆盖范围见 [通知 QA](../NOTIFICATIONS_QA_20261025.md)。

### 显式空白清理与文件末尾换行

`CodeEditor.TrimTrailingWhitespaceChecked() error` 一次性删除各行 LF／CRLF／EOF
前的 ASCII 空格与 Tab，保留其他字节、换行种类、反向选区、多光标与书签行号。
Undo／Redo 恢复精确字节及光标／选区；历史只保存变化的尾部，不保存整篇正文副本。
原 `TrimTrailingWhitespace()` 的无返回值签名保持兼容；需要显示错误的宿主请用
Checked 版本。无变化时不增加撤销记录、revision 或文本通知。

`HasFinalNewline()` 常数时间查询是否以 LF（含 CRLF）结束；空文档返回 false。
`EnsureFinalNewline() error` 仅在非空且缺少末尾换行时追加一个当前 Enter 偏好的
分隔符，不改变已有换行、不压缩额外空行、不跳到 EOF，也不把已有选区扩到新行。
可通过 `LineEndings().Preferred` 读取偏好，或用 `ConvertLineEndings` 显式修改。

只读返回 `ErrEditorReadOnly`。若清理会把正文中的独立 CR 变成 CRLF 的一部分，
返回 `ErrEditorCleanupCR` 并拒绝整个事务，不部分修改：例如 `"a\r \n"` 去空格，
或在 `"a\r"` 后追加 LF。后者可先显式选择 CRLF，再保留正文 CR 追加 CRLF。
非 ASCII 空白（NBSP 等）、独立 CR、BOM、NUL 与无效 UTF-8 不被顺便正规化。
这些都是 UI 线程 API；没有新增空闲轮询、文件扫描器或自动保存清理。

IDE：编辑 → 文本清理；命令面板可搜索英文或中文名称。命令仅作用于打开菜单／
面板时对应的编辑器，输入框焦点、只读或切换文档后不会误改正文。错误显示警告。
注意清理是文本操作，会去掉原始字符串或 Markdown 中有意义的行尾空格，需用户
显式执行；它不是语义保持格式化。

示例：`go run ./examples/editor-cleanup`，包含全选／多光标、清理／末尾换行、
Undo／Redo、只读、重置和 JSON 状态快照。
[验证记录](../EDITOR_CLEANUP_QA_20261025.md)。

### 事件驱动书签与跨文件面板

`CodeEditor.SetBookmark(line, marked)`、`HasBookmark`、`BookmarkCount` 使用零基行号。
`BookmarkLines()` 返回排序后的独立副本；`SetBookmarkLines([]int)` 原子替换并去重，
任何无效行拒绝整个请求；`ClearBookmarks()` 清空。所有调用在 UI 线程进行，只读
文档仍可标记／导航，书签操作不修改正文、dirty 状态或文本撤销栈。

插入／删除按字节编辑映射锚点，行移动／批量复制删除按行事务映射，删除的行移除
书签。Undo/Redo 在注释 epoch 未被用户改变时恢复删除前的标记；用户显式增删书签
之后，文本撤销只映射当前标记，不复活用户已经移除的书签。

`NextBookmark/PrevBookmark` 在当前缓冲区循环跳转，展开目标折叠、清理额外光标和
选区并聚焦。排序索引只在位置改变时更新。`LineCount()` 是常数时间的逻辑行数查询，
包含末尾空行，避免调用 `Lines()` 复制整个行切片。

`SubscribeBookmarksChanged(func())` 不覆盖 `SigChanged`；标记变更或有标记时的
完整文本发布触发通知，方便宿主同步行内容。无初始通知、无空闲轮询；回调须短小，
不得同步修改该编辑器。返回幂等退订函数，宿主拆卸时调用。

`ged.BookmarksPanel.SetBookmarks` 批量更新、去重并保持所选位置身份；面板使用一基
行号。↑↓／Home／End／PageUp／PageDown 选行，Enter 激活，Delete／Backspace
发出 `SigRemoveRequested`，由宿主同步删除。仅绘制可见行，正文裁剪在固定表头下。

SilkIDE 将缓冲区锚点投影到 `ide/bookmarks.Store`、面板和异步保存的 preferences，
关闭文件保留书签，跨文件跳转进入 Back/Forward 历史。独立 SDK 的 F2/Shift+F2
仍是文件内导航；IDE 覆盖为上述平台菜单快捷键，不能混用两者提示。

宿主 `SetText`／全量 provider 替换用公共前后缀和附近 200 行的文本匹配回锚；
IDE 重开文件用持久化上下文匹配，找不到时保留并夹紧到有效行。重复文本可能含糊，
不是语义定位。单行上下文最多 1024 字节，过长则不保存上下文；偏好书签记录最多
4 MiB，损坏／超限时保留原有磁盘记录并提示未保存，不默默覆盖。

示例：`go run ./examples/editor-bookmarks`。包含 Toggle、Previous/Next、Undo/Redo、
折叠加额外光标、书签列表与 JSON 状态快照。
[验证记录](../EDITOR_BOOKMARKS_QA_20261025.md)。

### 经过验证的行／列导航

UI 线程调用 `CodeEditor.ResolveGoToLocation("2:3")`，解析一基行号与 Unicode
码点列号，返回零基 `EditorTextPosition`（含 rune、byte、UTF-16 坐标）。
省略列默认为 1；行尾位置有效，CRLF 的 CR 不算正文。只接受正整数，允许首尾
空白，不接受内部空白、相对位置或多个冒号；输入最多 64 字节。

`NavigateToLocation` 校验成功才移动光标、展开目标折叠、清理选区／多光标并聚焦；
错误可用 `errors.Is(err, gui.ErrEditorLocation)` 判断，失败不改变文本／光标／历史。
只读文档也可导航。有效查询只扫描目标行前缀，不复制文档、不启动后台任务。

`ShowGoToLine/HideGoToLine/GoToLineVisible` 提供编辑器子控件输入条；支持粘贴、
选区、Enter／Escape 和实时校验，首次打开填入当前坐标，再次打开选择已有查询。
导航输入拥有焦点时消费源代码 Undo／格式化／重命名快捷键，不误改背后的文档；
导航条先转发 `Edit` 局部 Undo/Redo，再拦截不属于输入字段的文档命令；查询框和
导航条按钮使用同一份字段历史，空历史不继续撤销正文。点击正文后可正常编辑，隐藏或拆卸编辑器时取消
输入与待处理粘贴、移除提示。和 Find 同时显示时按实际高度堆叠，不用独立窗口。

`examples/editor-goto` 演示导航、只读切换、重载及 JSON 状态快照。
码点列不是字形簇、视觉 Tab 列或 LSP UTF-16 列；导航不写入正文撤销栈。
[验证记录和平台边界](../EDITOR_GOTO_QA_20261025.md)。

### 行尾检测、转换与事件订阅

`CodeEditor.LineEndings()` 返回 LF/CRLF 数量与 `Preferred`；混合行尾显示 `Mixed`。
宿主 `SetText` 检测多数行尾，平局取首次出现的分隔符；无分隔符默认 LF。
UI 线程调用 `CanConvertLineEndings(gui.LineEndingLF/LineEndingCRLF)` 查询可用性，
`ConvertLineEndings(style)` 原子转换并设置后续 Enter 的行尾。一轮 Undo/Redo 恢复
混合行尾、光标、选区方向和偏好；历史为每个分隔符一位，不复制整篇正文。

`SubscribeLineEndingsChanged(func())` 返回幂等退订函数；先查询一次，再接收数量或
偏好的变化通知，不覆盖 `SigChanged`，没有轮询定时器。回调须短小，不能同步编辑
正文；组件拆卸时退订。空／单行文档仅改变偏好也可撤销，但不增加文本 revision，
不标记 Modified，不发文本变更事件；尚未实际插入行尾时，偏好不会写入文件。

- 粘贴与宿主 `SetText` 不隐式规范化；SDK 格式化提供者的输出也保持原义。
- SilkIDE 的 Go 自动保存格式化会将 gofmt 输出转换为文档选定的 CRLF；显式格式化
  命令不承诺保留该偏好。文本末尾有无换行由显式转换保留，不补尾换行。
- 独立 CR 是正文；为避免歧义与反复转换丢字节，`CRCRLF → LF` 返回
  `gui.ErrEditorAmbiguousCR` 且不修改内容；未知类型返回 `gui.ErrEditorLineEnding`，
  只读返回 `gui.ErrEditorReadOnly`。不支持旧 Mac CR-only 分行。
- `examples/editor-line-endings` 提供转换、历史、只读切换、混合重载与 JSON 快照。
  [验证记录](../EDITOR_LINE_ENDINGS_QA_20261025.md)。

### 只读代码与文本查看器

```go
viewer := gui.NewCodeEditor()
viewer.SetText(source)       // 宿主加载；即使只读也允许
viewer.SetReadOnly(true)    // 禁止用户编辑，不清空撤销历史
```

`IsReadOnly()`、`CanCut/CanPaste/CanUndo/CanRedo` 可直接驱动命令状态；锁定后仍可
选择、复制、查找、跳转与滚动。解锁恢复原有历史，不修改 revision、dirty 或正文。
`FormatDocument/FormatDocumentAsync` 和带错误返回的重命名操作返回
`gui.ErrEditorReadOnly`，无错误返回的修改命令不执行。

锁定会撤销编辑器拥有的异步粘贴、格式化、补全与替换请求；即使随后解锁也不接纳
旧结果。外部提供者仍须检查自己的请求身份／版本。状态切换与全部编辑器 API 一样，
必须在 UI 线程调用；不是并发锁或文件系统访问控制。宿主 `SetText` 有意绕过只读并
重置历史；应用自己的写盘／跨文件事务仍由宿主负责。

设计器属性 **只读** 支持保存、重载、预览及 Go 导出。`examples/editor-readonly`
演示切换、复制、查找、宿主重载与状态快照，窄窗口按钮换行。
[本轮验证和限制](../EDITOR_READONLY_QA_20261025.md)。

### 撤销历史状态与订阅

`CodeEditor.CanUndo()` / `CanRedo()` 是 O(1) 查询，与 `Modified()` 独立：保存不清除
历史；撤销恢复到已保存文本也不代表没有 Redo。`SubscribeHistoryChanged(func())`
返回可重复调用的取消订阅函数，支持多个观察者，不覆盖 `SigChanged`。绑定后先查询
一次状态；之后只在两个可用性布尔值变化时回调，编辑完成、Undo/Redo、`SetText`
清空历史均有覆盖。连续输入、保存和光标移动若未改变可用性，不额外通知。

查询、订阅、取消与回调均在 UI 线程进行；回调应短小，不能同步修改正文。消费者
拆卸时必须取消订阅。`examples/editor-lines` 展示 Undo/Redo 按钮按事件启用，以及
`CanUndo` / `CanRedo` 快照。IDE 工具栏随编辑、切换文档和模式同步，空历史命令
不再发出伪设计变更。菜单持有文档身份；长期注册的工具栏/快捷键解析当前可见文档。

设计器 `GedScene.HasPendingEdits()` 仅查询草稿，不触发提交或写盘。有草稿时 Undo
仍可先提交后撤销，提交失败则保留草稿；Redo 不可用，避免无意清除已有重做分支。

### 编辑器剪贴板命令

UI 线程调用 `CodeEditor.Copy() error`、`Cut() error`、`Paste()`、`SelectAll()`，
对应 `CanCopy` / `CanCut` / `CanPaste` / `CanSelectAll` 可用性查询。禁用控件时命令
不执行；查询不分配选中文本，也不读取系统剪贴板。`CanPaste` 表示允许发起粘贴，
不是剪贴板内容探测；空剪贴板在执行时保持无操作。

有选区时复制/剪切精确选中字节；无选区时沿用原有快捷键语义，只处理主光标当前行，
不附加 LF，也不剪掉其它光标所在行。空行剪切只删除该行，不改变剪贴板。先成功写入
剪贴板才删除源文本；NUL、超过 16 MiB 或系统写入失败均返回错误并保留原文与历史。
复制不清空撤销栈；剪切与粘贴均支持 Undo/Redo。全选会清除次光标，不创建编辑历史。

粘贴前应恢复编辑器焦点；macOS 使用已有异步读取、大小限制和失效目标保护，其它
后端仍沿用同步原生读取，不能据此承诺所有平台的剪贴板读取都不会阻塞。
`examples/editor-clipboard` 展示按钮调用、错误反馈及 JSON 状态快照。

IDE 菜单和命令面板以打开时的编辑器身份绑定剪贴板目标；其它输入框拥有焦点时，
这些文档命令不可用，不会偷改后台文档。独立文本框继续使用自己的快捷键。

### 平台快捷键提示

`gui.CodeEditorShortcuts()` 返回独立的 `CodeEditorShortcutHints` 值，提供剪贴板、
多光标、行移动、复制行、删除行、缩进等内置键位文本，可用于工具按钮、菜单和帮助。
Windows 推荐 `Ctrl+Shift+Alt+Up/Down` 添加光标，macOS 为 `Cmd+Alt+Up/Down`，
Linux 为 `Ctrl+Alt+Up/Down`。Windows 旧组合仍可接受，但系统或桌面可能在事件到达
应用前吞掉它；不通过全局键盘钩子或抬键补造按键规避。

此 API 是提示元数据，不是键位注册/重映射功能；宿主覆盖键位后须展示自己的提示。
`Tab`/`Esc` 保留完成列表、片段、查找框等上下文的优先级，不能视为无条件行缩进/清除光标。
IDE 主菜单、命令面板和编辑器右键菜单共用这些提示，禁用命令也保留其快捷键信息。

### 行块复制与删除

在 UI 线程使用 `CodeEditor.DuplicateLines()`、`DeleteLines()`，以及纯可用性查询
`CanDuplicateLines()`、`CanDeleteLines()`。Edit 菜单、命令面板与 Cmd/Ctrl+Shift+D/K
共用同一操作。有选区时复制或删除覆盖的逻辑行块，下一行第 0 列端点不包含该行；
无选区时各光标的相邻行合为组，同一行不重复处理。复制后光标跟随副本；删除后的
重合光标合并到下一条存活行（EOF 处回落末行），第 0 列。Undo/Redo 精确恢复文本、
所有光标及选区方向。无选区剪切仍仅删除已放入剪贴板的主光标行，不误删其他光标行。

复制末行时新增分隔符跟随上一行的 LF/CRLF，单行文件默认 LF；原末行的字面 CR
保留。复制 EOF 空行会增加一个空白行。删除末行同时移除前置分隔符；删除所有行
留下一个空行，空文档再次删除不改变 revision、dirty、撤销栈或临时输入状态。

撤销历史只保留受影响行的独立字符串，不保存整篇正文，也不引用完整行索引的底层
切片。任意数量的行组一次构造新行索引；涉及的折叠展开，未涉及的折叠重新定位。
可用性查询不扫描正文；真实编辑仍在 UI 线程重新发布文本，不能据此保证任意大小
文档零延迟。示例 `examples/editor-lines` 增加 Duplicate/Delete 按钮和 CanDelete 快照。

### 行块移动

`CodeEditor.MoveLinesUp/MoveLinesDown` 与 `CanMoveLinesUp/CanMoveLinesDown` 为
UI 线程 API，菜单、命令面板与 Alt+↑/↓ 使用同一实现。有选区时移动覆盖的逻辑行块，
终点位于下一行第 0 列不包含该行；无选区时合并各光标的相邻行，按组移动。
到达边界的组保持不变，其余组仍可移动。文件末尾换行后的空行视为终止标记，
不会与内容行交换；中间空白行仍是正常内容行。把空白正文行移动到原本无末尾换行
的 EOF 时，结果可以自然以现有分隔符结尾；这不插入新分隔符，Undo 可精确恢复。

一次 Undo/Redo 恢复整组文本、光标及选区方向。行分隔符留在原槽位，支持混合
LF/CRLF、无末尾换行、非法 UTF-8 字节以及 EOF 处的字面 CR；历史只记录旋转范围和
CRLF 位图，不保留正文副本。触及的折叠区域展开，旧片段/重命名提示失效；冻结的
“在选区中查找”范围按其他无关编辑的规则失效，不会悄悄扩大到整篇文档。

可用性查询只检查选区/光标行索引，不扫描行正文。真实修改仍需重新发布文本、更新
布局和绘制，不能由无分配的行旋转基准推导“任意大小文件不卡顿”。示例：
`go run ./examples/editor-lines`。

### 缩进与原始内容

在 UI 线程调用 `editor.SetInsertSpaces(true)`、`editor.SetTabSize(2)`，即可配置
后续 Tab／Enter／行缩进。配置不修改正文或 dirty 状态，也不转换粘贴、片段和格式化器
返回的内容；Go 格式化仍遵循 gofmt 的标准风格。Tab 的逻辑列按 rune 计数，已有 Tab
按制表位展开，不等同于终端 wcwidth 或比例字体的像素宽度。

`IndentLines/OutdentLines` 使用选区覆盖的行；无选区则使用各光标所在行。选区终点在
下一行第 0 列时不包含该行。Outdent 删除一个前导 Tab 或最多 `TabSize()` 个空格；
无前导空白的行不改动，无变化不记录历史或发出 Changed。旧 Selection API 仍要求选区。
单行选区的普通 Tab 保留原先“替换选区”的语义，Shift+Tab 改为减少当前行缩进；
活动片段的 Tab／Shift+Tab 仍优先导航占位符。

每次缩进操作只为修改过的前缀保存历史，不存储两份完整文档；最终文本发布、布局和
重绘仍在 UI 线程，不宣称任意大小文档零延迟。示例：`go run ./examples/editor-indent`。
IDE 配置是全局持久化选项，不是自动检测或每文件 `.editorconfig` 支持。

### 离线格式化与取消

`FormatDocumentAsync(gui.EditorFormatOptions{Compact: false}, done)` 在 UI 线程调用，
后台仅处理不可变文本；`done` 也在 UI 线程执行。立即返回错误时不会调用 `done`。
正文修改、路径变化、隐藏或显式取消会使结果与回调失效，包括编辑后 Undo 回原文。
宿主销毁编辑器／停止事件循环前必须调用 `CancelFormat()`；取消不等待标准库解析器退出，
任务真正结束前仍占用该编辑器的任务槽，立即重试可能得到 `async.ErrBusy`。
`Formatting()` 表示尚未取消且等待 UI 交付的本地请求，不表示 gopls 的任务状态。

`FormatDocument` 是同步入口；兼容入口 `FormatCode` 仍只处理 Go，但不再调用外部 gofmt。
交互式入口使用异步版本：SDK 编辑器 Ctrl/Cmd+Shift+F、代码面板格式化按钮；SilkIDE 使用
Ctrl/Cmd+Shift+I（Ctrl/Cmd+Shift+F 留给工作区搜索）。IDE 的 Go 文档优先保留 gopls 路径，
JSON 与无 LSP 时的 Go 使用本地版本。`Cancel Local Formatting` 只取消本地任务。
示例 `go run ./examples/editor-format`；资源限制、取消边界见 [formatx](../formatx/README.md)。

既有组装入口：`gui.Edit`、`ComboBox`、`Label`、`Button`、`CodeEditor`；
布局使用 `HBox`、`VBox`、`FormLayout`、`GridLayout`；设计文件使用 `ged`，声明式树使用 `decl`。
图表／甘特图、打印、数据绑定等既有模块见 [README](../README.md)，不把本轮 HTTP 验收扩展为它们的重新验收。

### 对话框初始焦点

`Dialog.SetInitialFocus(widget)` 在 UI 线程显式指定初始输入目标；目标必须仍属于
该对话框且祖先链可见、可用。不指定时保留已聚焦的可用子控件，否则选择内容区
第一个可 Tab 控件，再回退到默认按钮／按钮区。纯提示确认框直接聚焦默认按钮，
首次 Tab 不会重复停在默认项。Dialog／MessageBox 根节点不再占用 Tab 序列，
禁用容器的子树不会参与遍历。默认按钮标记仍可在隐藏构造阶段查询，但隐藏、
禁用、脱离按钮区或属于别的对话框的按钮不能通过默认提交执行。

此逻辑只在激活时运行，不增加轮询；完整证据与限制见
[对话框焦点 QA](../DIALOG_FOCUS_QA_20261025.md)。

### 编辑器查找与替换

`CodeEditor.ShowFind(replace)` 打开编辑器内部的真实子控件查找栏；
`SetFindQuery(query)`、`SetFindOptions(EditorFindOptions{MatchCase, WholeWord, Regex})`
配置匹配。字面量查询保持单行；默认不区分大小写，整词边界识别 Unicode 字母、
数字、组合标记和下划线。`Regex: true` 支持 Go 正则、跨行及捕获组替换，始终后台计算；
限制与语义见 [正则 QA](../EDITOR_REGEX_QA_20261024.md)。

`FindSnapshot()` 返回 `Query`、`Options`、完整 `Count`、保留的 `Retained`、
从 1 开始的 `Current`（无结果为 0）、`Pending`、`Ready`、`Revision`、`Err`；不要把后台计算中的 0
当作已确认无匹配。以上 API 均在 UI 线程调用。

- 文档与查询等输入超过 32 KiB 后转为后台计算，75 ms 合并抖动；每编辑器只留
  当前任务和最新请求，全进程最多同时计算两个。结果必须通过文档版本、查询、
  选项及请求代次校验；替换还校验替换文本。
- 最多保留 100,000 个高亮／导航位置，但计数与全部替换不受此截断影响。
  查询最多 1 MiB；替换结果最多 128 MiB，超过上限报错且不写入部分结果。
- 关闭查找栏、隐藏编辑器会取消后台任务；宿主销毁编辑器或停止事件循环前应
  在 UI 线程调用 `CancelFind()`，与 `CancelCompletion()` / `CancelSymbolIndex()`
  一起释放任务和工具提示注册。重新打开查找可复用编辑器。
- 后台只生成不可变结果；最终应用文本及一次 Undo／LSP 通知仍在 UI 线程。
  这不是任意大小文件零延迟的承诺。

### 剪贴板任务与宿主生命周期

`Clipboard.ReadTextAsync(ctx, done)` 在 UI 线程调用，`done(text, err)` 通过
`gui.Post` 交付；需有存活的窗口。函数返回 admission error 时不会调用 `done`，
回调本身不会替宿主修改任何文本。macOS 使用标准系统 `/usr/bin/pbpaste`，
最多两个在途读取、10 秒超时、16 MiB 上限；输出过大返回 `ErrClipboardTooLarge`，
不静默截断。Windows/Linux 仍在 UI 线程调用原生剪贴板，不能据此承诺非阻塞。

内置 macOS 文本控件会防止过期结果写入其他选区/文档；隐藏、禁用、重挂父节点
和逻辑焦点转移会取消 pending paste。自定义宿主可在销毁时调用
`CancelPendingPaste()`；连续相同锚点的粘贴键合并为一次插入和 Undo，最多 64 次。
这不包括旧 `Clipboard.Data/Formats` 或复制/剪切写入的异步化。完整验证和边界见
[剪贴板 QA](../EDITOR_PASTE_QA_20261024.md)；比较视图生命周期见
[异步 Diff QA](../DIFF_ASYNC_QA_20261024.md)。

### 嵌入 CodeEditor 时的补全生命周期

`CompletionIndexLoading()` 和 `CancelCompletion()` 与编辑器其余接口一样只能在 UI
线程使用。`CancelCompletion()` 不等待 worker，适合在宿主移除编辑器或关闭窗口时调用；
它不修改文本，也不会永久禁用补全。SilkIDE 已在关闭页签与退出路径调用。
后台仅使用不可变文本快照，90 ms 静默期后扫描／排序，全局最多两份索引计算并行；
编辑、光标变化、Escape 与失焦会使旧投递失效。没有轮询或持续重绘心跳。
`CompletionIndexLoading()` 只表示本地索引，不等于 gopls 已完成工作区加载。
这是词法候选索引，不代替 gopls 的类型、作用域和跨文件语义服务。
验收与限制见 [后台索引 QA](../EDITOR_INDEX_QA_20261023.md)。

### CodeEditor 文件类型与显式提供者

`Language()` 是 UI 线程 getter，返回 `go`、`silkgo`、`json` 或 `plaintext`，由
`SetFilePath()` 的扩展名自动决定（不区分大小写）。未命名编辑器保留原有 Go 默认；
`.json` 使用独立词法高亮（键、字符串、数字、布尔值与 null）、状态栏与缩略图颜色，
内置 `true`／`false`／`null` 候选，并保留文档词补全。它不启用 Go 符号、折叠或片段。
词法扫描逐行进行，达到 4,000 字节的长行回退为普通文本；不增加轮询或整文档解析。
词法高亮不是 JSON 实时校验／Schema 服务；显式美化／压缩见上方格式化接口。
`.jsonc`、`.txt`、README 与 go.mod 仍按普通文本处理，
也未提供手动语言选择器或多语言 LSP 管理器。验收见 [JSON 编辑器 QA](../EDITOR_JSON_QA_20261024.md)。

普通文本不启用内置 Go 高亮、符号解析、折叠、关键词候选和默认 Go 片段；
仍可从整份文档补全标识符。`FormatCode()` 仅格式化 Go，不把普通文本或未展开的
SilkGo 交给 gofmt。SilkGo 保留 Go 风格的词法功能以及宿主提供的宏投影。

显式调用 `SetSnippets(...)`、`SetSyntaxHighlights(...)`、`SetAdditionalSymbols(...)`
仍可为普通文本提供自定义片段／高亮／符号。跨类型改名会取消旧补全、清除旧投影及折叠，
但不改变正文、dirty 状态或撤销记录；宿主应随后重新发布该类型的投影。
外部语言服务的连接与文档生命周期仍由宿主管理，`Language()` 不替代这些协议。
验收与边界见 [文件类型模式 QA](../EDITOR_DOCUMENT_MODE_QA_20261023.md)。

`CodeEditor` 同时实现 `IWantsTab`：原生窗口在焦点遍历之前交给编辑器处理普通
Tab／Shift+Tab，复用补全、查找／替换、片段和缩进路径，不重复插入 Tab。
Ctrl／Alt／平台操作修饰键配合 Tab 时不由编辑器插入制表符；窗口先保留显式宿主
快捷键的优先级，再让 Ctrl+Tab 导航最近的 TabWidget，没有页签容器时沿用焦点遍历。
自定义宿主应沿用原生窗口的接口分发，不要先在外层把 Tab 消费掉。
新式与旧式片段展开共用原子文本替换：保留缩进、Unicode 字符与展开前后光标，
Undo／Redo 不残留展开正文。Undo、Redo、SetText 或实际整段替换会终止旧片段会话；
Redo 恢复正文与光标，不重建已取消的占位符会话。此路径保存完整文本历史快照，
并非只存局部增量；大文件的撤销历史内存仍需后续优化。

### 页签导航与事件契约

`TabWidget.NavigateTab(previous bool) bool` 在 UI 线程按当前可见标签顺序循环切换，
成功接纳切换返回 true；空／单页或隐藏／禁用容器返回 false。目标页第一个可聚焦、
可用控件接收键盘；纯展示页退回标签栏。嵌套组只操作最近的一组，单页组不穿透外层。
不重新读文件、创建 LSP 文档或后台任务，不保留额外的控件焦点历史。

`SetCurrentChangedCallback` 在索引或页面身份改变时一次通知，初次添加为 0、
删除最后一页为 -1。通知时标签、页面、元数据已同步；同页重复选择不通知。
关闭当前页后新页恰好占据原索引，仍会通知；应用应允许 -1，不把事件当作每次
SetCurrentIndex 都触发的刷新信号。回调内移除／改选页面或显式转移焦点受到尊重。
回调的任意用户代码仍在 UI 线程，不保证它本身非阻塞。

Ctrl+Tab／Ctrl+Shift+Tab、Ctrl+PageDown／Ctrl+PageUp 通过原生窗口路由至最近的
TabWidget，保留显式宿主快捷键优先级，不穿越 popup 或其他原生窗口。
在 macOS 上也是 Control，不是 Command；普通 Tab、Shift+Tab 行为不变。
示例 `examples/tab-navigation` 展示事件计数、输入、重排和关闭；关闭只丢弃示例的
临时页，真实应用应像 SilkIDE 一样用 `TabBar.SetCloseCallback` 接入保存／取消守卫。
验收范围见 [页签 QA](../TAB_NAVIGATION_QA_20261024.md)。

### 指针操作后的内容焦点（可选）

`TabBar.SetPointerFocusCallback(func(*TabBar))` 供宿主在鼠标完成选择、关闭或原生
拖放后接管焦点；默认 nil。普通键盘／程序切页不触发。回调前先清空内部左键手势，
以页面身份而非旧数字索引验证左键释放／拖放。只有标签栏仍持有焦点，且全部祖先
可见、启用才回调；不夺回在原生等待期间被对话框或其他控件主动取得的焦点。
宿主仍应检查当前页面、所属窗口和模式，不能无条件聚焦已隐藏的内容。
中键关闭只在标签栏本来仍持有焦点时提供同样回调，不改变中键按下的原有语义。
拖放缩略图在原生调用结束（含异常退出）后显式释放。

`ToolBar.SetPreserveContentFocus(true)` 让直属按钮的鼠标按下不抢走当前内容焦点；
`PreserveContentFocus()` 读取配置。默认 false，属性表提供“保留内容焦点”。
不禁止按钮的键盘焦点，不拦截工具栏内 Edit 等输入，也不回滚动作主动切换的焦点。
按钮移出该工具栏后自动恢复普通行为。开启后，宿主不能依赖失焦提交属性草稿，
保存／设计动作应沿用显式 pending-edit 提交路径。

SilkIDE 编辑器页签、主工具栏和设计工具栏已接入。剪贴板与文本菜单仍检查真正的
编辑焦点，导航／搜索／属性输入不被活动文档替代。新路径不增加计时器、轮询或
后台任务；现有低频场景标题同步保留为兜底。验证见
[指针焦点 QA](../EDITOR_POINTER_FOCUS_QA_20261025.md)。

### 溢出页签的绘制与命中几何

页签滚动原点对齐完整单元格，避免显示前一个标题的尾巴；末端标题按实际可见宽度
重新省略。剩余区域不够放下保留槽与省略号时留空，不生成不可见点击目标。
关闭图标与命中共享完整尺寸检查，窄到容不下图标／关闭按钮时不会让二者重叠。
绘制、点击和 DND 插入点使用同一组可见边界；标题测量缓存在布局／宽度变化阶段，
Draw 不扫描全部隐藏页签。零高度的 `TabWidget` 也不会保留一个越界的 26px 子标签栏。

macOS 的 Control-Tab 由每个原生窗口的局部事件监视器补送到 GLFW 原有键盘回调，
仍走宿主快捷键、最近容器、popup 和焦点守卫，不另造一条切页逻辑。
只处理 Control-Tab／Control-Shift-Tab 的按下、重复和抬起；Command、Option、
输入法 marked text、其他窗口和非内容视图的 responder 不拦截。
监视器在窗口脱离／销毁前移除，不增加定时器、空闲轮询或后台 goroutine。
滚轮在 macOS 优先读取当前同窗口 ScrollWheel 事件坐标，其他平台沿用 GLFW 回退。

`go run ./examples/tab-overflow` 可独立复测；原 `examples/tab-navigation` 保留不变。
截图、原生事件诊断、字节回读、性能边界见
[标签几何 QA](../TABSTRIP_GEOMETRY_QA_20261025.md)。

### 页签上下文菜单

`TabBar.SetContextMenuCallback(func(*TabBar, *Menu, int))` 在 UI 线程按右键目标
构建菜单，不改变活动页；焦点位于标签栏时 Shift+F10 打开活动页菜单，按键重复不重开。
按下与抬起必须仍是同一标签，空白／越界／隐藏／禁用位置不弹菜单。
传 nil 清理回调及所属 popup；每个标签栏复用一个 popup，不使用全局控件注册表。

传入索引只在构建时有效。菜单动作应捕获页面身份并在执行时重新定位，不能在弹出后
沿用旧索引；`examples/tab-navigation` 展示这种写法。菜单关闭会释放各项、子菜单、
工具提示／上下文注册与动作闭包；这些是 popup 所有的临时控件，不应移交其他宿主复用。
该关闭时清理策略仅用于这个新 API，不改变普通 Menu.Clear 的既有语义。

SilkIDE 已接入单文件保存／安全关闭／历史恢复／复制完整路径／项目内定位。
定位复用异步文件扫描；动作可用性只检查内存身份和路径关系，不读取文档或扫描文件树。
图标仍复用既有资源加载／缓存，首次资源初始化不计入纯内存菜单基准。
单页保存与关闭仍复用已有同步读写，不承诺慢网络文件零阻塞。SilkIDE 的关闭其他／
右侧／全部页签现在使用后台预检与 `internal/filetxn` 冲突检测保存，逐事件轮次发布
保存状态和移除页面；支持取消，已在执行的磁盘提交可能完成。快照有 64 MiB 上限，
单个任务最多 4,096 页；超限保留文档并提示分批处理。该编排是 IDE 内部实现，不是
新增的公共 SDK 批量关闭 API。复制图标使用随 SDK 提供的 HiDPI 矢量绘制，不依赖外置位图。
批量工作流验收见 [批量关闭 QA](../EDITOR_BATCH_CLOSE_QA_20261024.md)。
验收与平台边界见 [上下文菜单 QA](../TAB_CONTEXT_QA_20261024.md)。

### Go 跨行高亮与绘制成本

Go／SilkGo 编辑器在文本发布时维护块注释／反引号字符串的行首、行末状态，
只重新扫描变化区域及需要传播的后缀；滚动、折叠后的正文和 minimap 共用源行状态。
稳定重绘不再从文档第一行重新分词。无新增定时器、后台线程或宿主回调要求。
`SetText`、普通编辑、原子替换、Undo／Redo 与类型切换均接入相同维护路径。

超长行仍降级为单个颜色片段，但会解析分隔符，避免闭合注释／字符串后继续污染下一行。
这是内置词法高亮，不替代 gopls 的语义信息；也没有实现真正的自动换行布局。
状态索引占 O(行数) 内存，文本发布仍需比较公共前后缀；首次载入或修改跨全文分隔符
可能扫描整份文档。不是所有编辑／保存操作均已变成 O(1)。
测试与测量边界见 [增量词法状态 QA](../EDITOR_LEXICAL_QA_20261023.md)。

## 同步 HTTP：最少代码获得一个字段

以下代码应在 CLI 或后台任务中运行，**不要阻塞 UI 事件处理函数**：

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

response, err := httpx.Default().Get(ctx, "http://127.0.0.1:8080/api")
if err != nil {
    return err
}
count, err := response.Int("/data/count") // int64，支持完整 64 位有符号整数
if err != nil {
    return err
}
fmt.Println(count)
```

按需求替换最后一步：

```go
text := response.Text()                     // 完整 body，string
name, err := response.Str("/data/name")      // 指定 JSON string
node, err := response.JSON("/data/items/0")  // any；数字保持 json.Number
count, err := response.Int("/data/count")    // int64
ratio, err := response.Float("/data/ratio")  // 有限 float64
ready, err := response.Bool("/data/ready")   // bool
value, err := response.Extract("json", "/data") // compact JSON string
```

字段路径使用 [RFC 6901 JSON Pointer](https://www.rfc-editor.org/rfc/rfc6901)，不是 JSONPath 或点路径：
`""` 表示根；`/items/0/name` 表示数组首项；字段名中的 `/` 写成 `~1`，`~` 写成 `~0`。
支持空字段名 `/`；不支持通配符、递归搜索、负下标、`#` URI-fragment 形式。

转换规则：

- `Extract("str", "")` 返回原始 body；非空路径要求目标为 JSON 字符串。
- `Extract("json", path)` 返回合法 JSON 文本，字符串会保留 JSON 引号，null 为 `null`。
- `Int` 仅接受 int64 范围内的整数字面量；拒绝字符串 `"12"`、小数 `1.5`、指数 `1e3` 和溢出，不截断、不先转 float64。
- `Float` 接受 JSON number，但遵循 float64 的精度边界；拒绝溢出为无穷大的值。
- 缺失字段为 `ErrMissingField`，错误路径为 `ErrJSONPointer`，类型不符为 `ErrType`；所有错误都应处理。
- JSON 必须恰好一个完整值，拒绝拼接多个 JSON 文档。数字精度策略使用标准库的 [Decoder.UseNumber](https://pkg.go.dev/encoding/json#Decoder.UseNumber)。

## 异步 HTTP：显式派发到 UI

```go
future, err := httpx.Default().GetAsync(ctx, url)
if err != nil { // 例如满载、已取消
    status.SetText(err.Error())
    return
}
_ = future.OnComplete(gui.Post, func(response *httpx.Response, err error) {
    if err != nil {
        status.SetText(err.Error())
        return
    }
    status.SetText(response.Text())
})
// 窗口/任务取消时：future.Cancel()
```

如果包含解析或其他耗时步骤，也应放在后台：

```go
future, err := gui.RunAsync(ctx, nil, func(ctx context.Context) (int64, error) {
    response, err := httpx.Default().Get(ctx, url)
    if err != nil { return 0, err }
    return response.Int("/data/count")
}, func(count int64, err error) { // 由 GUI 事件循环执行
    if err != nil { status.SetText(err.Error()); return }
    status.SetText(strconv.FormatInt(count, 10))
})
```

`nil` executor 使用共享的 8 并发上限。大量不同来源任务应使用各自 `async.NewExecutor(n)`；
HTTP client 可以通过 `httpx.Options.Executor` 指定池。
`Future.Await(ctx)` 超时只停止等待，不自动取消工作；要终止请求应调用 `Cancel()`。
`Executor.Close()` 拒绝新任务并取消已接纳任务；`Wait(ctx)` 等待其工作返回，不保证 GUI 队列已经执行。
`Future.Done()` 同样不代表 UI 回调已执行。

`OnComplete` 的 dispatcher 不可阻塞或 panic；回调执行上下文由 dispatcher 决定。
`gui.RunAsync` 的 work 不可读写 GUI。宿主要在关闭时取消，并防范已经排队的旧回调；
GUI 必须运行 `core.EventLoop()` 才能处理 `gui.Post`。

## 请求策略和错误语义

```go
client, err := httpx.NewClient(httpx.Options{
    Timeout: 10*time.Second,
    MaxResponseBytes: 2 << 20,
    // HTTPClient: customClient, // 自定义 Transport/Jar/代理/重定向策略
    // Executor: async.NewExecutor(4),
})
```

- 默认总超时 30 秒、响应上限 4 MiB，包含 body 读取；父 Context 更早到期时优先停止。
- 仅接受绝对 HTTP(S) URL；保留标准库 TLS 证书验证，不添加跳过验证开关。
- 非 2xx 返回 `*httpx.StatusError`，同时保留受限的 Response，允许调用方读取 API 错误正文。
- 超大响应（包括自动解压后的内容）返回 `ErrResponseTooLarge`，不伪装为完整成功正文；所有响应 body 均关闭。
- 异步调用返回前复制 Header 和 Body，之后修改原请求不改变已发任务；调用复制期间仍不能并发修改输入。
- 本包不实现自动重试；标准库传输／重定向行为仍以 [net/http.Client](https://pkg.go.dev/net/http#Client) 为准。
  未提供 SSE、WebSocket、流式下载、上传进度或重试策略；请勿把受限的 buffered response 用于大文件。
- Options 中的 HTTPClient 被浅拷贝；共享 Transport/Jar 应事先配置好并支持并发，使用期间不要修改它们。

## HTTPButton：组装成桌面小工具

```go
request := gui.NewHTTPButton()
request.SetURL("http://127.0.0.1:8080/api")
request.SetResultType("int")
request.SetJSONPointer("/data/count")
request.SetHeader("Authorization", "Bearer "+token) // 运行时注入，不进入设计文件
request.SigIntResult(func(value int64) {
    status.SetText(strconv.FormatInt(value, 10))
})
request.SigError(func(message string) { status.SetText(message) })
request.SetParent(form)
request.SetBounds(12, 12, 160, 32)
// 点击发送；再点击取消。宿主销毁时 request.Close()。
```

设计器属性与 Go setter：

| 属性 | Setter | 默认 |
| --- | --- | --- |
| URL | SetURL | 空，不主动请求 |
| HTTP Method | SetMethod | GET |
| Headers JSON | SetHeadersJSON | 空；JSON object 的每个值必须是 string |
| Request Body | SetBody | 空 |
| Result Type | SetResultType | str |
| JSON Pointer | SetJSONPointer | 空 |
| Timeout ms | SetTimeoutMS | 30000；范围 1–300000 |
| Max Response KB | SetMaxResponseKB | 4096；范围 1–65536 |

事件：`OnResult(string)`（所有成功值的文本表示）、`OnIntResult(int64)`、
`OnFloatResult(float64)`、`OnBoolResult(bool)`、`OnError(string)`、`OnBusyChanged(bool)`。
对应 `SigResult`、`SigIntResult`、`SigFloatResult`、`SigBoolResult`、`SigError`、`SigBusyChanged`。
JSON 结果通过 `OnResult` 返回合法 JSON 文本；没有额外重复的 OnJSON 事件。

所有 widget 方法／回调均在 UI 线程使用。`Start(ctx)` 支持程序触发；忙碌时返回 `async.ErrBusy`。
取消、关闭和直接 detach/reparent 会失效旧回调；整个祖先容器销毁时宿主仍应显式 `Close()` 或取消传入 Context，
不要依赖 Go GC 来取消网络任务。属性加载、保存、画布拖放和构造器均不发请求。
HTTPButton 的请求和 JSON 提取都在后台，忙碌指示为静态主题色，不增加动画／轮询定时器。

不要把凭据写入 Headers JSON、URL 或 Body 等设计属性：这些属性会保存／生成。
使用 `SetHeader` 注入的 header 不进入属性表、设计文件或生成代码；`RemoveHeader` 可删除运行时覆盖。

## 示例与当前边界

```sh
go run ./examples/http-tool
```

默认只连接示例自己启动的 `127.0.0.1` 临时端口，提供 `/json`、`/error`、`/slow`。
切换 `/data/count` + int、`/data/ratio` + float、`/data/ready` + bool 即可试验；
改 URL 并发送才会访问用户指定服务器。结果编辑器允许复制／编辑显示文本，编辑不改变响应对象。

强类型 `ged` 导出已经验证生成控件属性和上述事件。
IDE 保存现在会识别**已有的强类型拆分导出**，保留原来的 UI 类型／构造器，并复用 SDK 生成路径；
不会再用声明式树覆盖它。外部 `<name>.go` 的现有方法正文优先，后续修改设计内嵌代码不会覆盖外部实现。
新设计仍默认生成 `Build<Name>() *decl.Node`；**自动迁移和默认预览执行内嵌回调尚未完成**。
Windows／Linux 原生窗口未在本轮重新验收，交叉编译不等于桌面验收。
macOS 原生 Cairo 路径已增加系统字体回退，并通过中文／彩色 emoji 实窗测试；
罕见字依赖已安装字体，复杂文本 shaping／bidi 与其他渲染后端不在该修复承诺内。

## 安全的强类型拆分导出

```go
result, err := scene.GenerateCodeFile("ui/view.go", ged.CodeGenOptions{
    PackageName: "ui",
    TypeName: "ViewUI",
})
if err != nil { return err }
if len(result.MissingImports) != 0 {
    // 提示用户在 result.UserFile 中补上这些 import；不要忽略它们后宣称构建成功。
    log.Printf("Add imports to %s: %v", result.UserFile, result.MissingImports)
}
```

- `<name>.silk.go` 必须不存在，或已是 SDK 拆分导出的机器文件。手写文件、单文件导出、其他生成器输出会报错，**不覆写**。
  若要从已有声明式树迁移，请先备份并移走其机器文件，或显式选择新输出路径；本轮不自动迁移 API。
- 两份源码先做语法／package／控件映射预检，再准备临时文件。普通 I/O 失败会尝试回滚；
  冲突不覆盖外部编辑，无法安全回滚时错误给出保留的恢复副本路径。符号链接和目录目标被拒绝。
- 相同内容不重新写入；已有文件权限保留。CRLF 用户源不重排／格式化；新增方法仍只追加。
- 首次导出按标题派生默认类型；再次导出未指定 `TypeName` 时沿用已有类型。
  显式更换已有类型名会报错，请保留旧名称或导出至新路径，避免破坏用户方法接收器。
- `ged.SplitCodeType(source)` 可读取已生成拆分文件的 UI 类型；它是结构识别，不是完整类型检查或内容可信性证明。
- 这些写入不是跨文件崩溃原子事务，也不是跨进程锁；仍应串行保存，不保证抵御外部进程在检查与 rename 之间的修改。
- IDE 自动保存遇到用户文件缺失／语法错误会报告问题，不擅自重建用户代码。`.silkui` 保存与伴随代码生成是两个结果。

## JSON 文档与树查看控件

```go
// 放在 gui.RunAsync 的 work 中：
response, err := httpx.Default().Get(ctx, url)
if err != nil { return nil, err }
return response.Document()
// 在 UI 完成回调中检查 err 后：
// err = viewer.SetDocument(doc)
```

文档可安全只读共享，多次 `doc.Int/Str/Float/Bool/Lookup` 不再重复解析整份 body；
`Node.Value/JSON` 返回副本。完整 API／默认大小、深度与节点数限制见 [jsonx](../jsonx/README.md)。
`Response.Document` 每调用一次解析一次，没有偷偷缓存旧接口结果；请自行复用返回的 doc。

```go
viewer := gui.NewJSONView()
viewer.SetJSONText(`{"data":{"name":"Silk 中文😀","count":9007199254740993}}`) // 设计样例
viewer.SetPageSize(64)
viewer.SigSelected(func(pointer string) { pathEdit.SetText(pointer) })
viewer.SigError(func(message string) { status.SetText(message) })
if err := viewer.SelectPointer("/data/count"); err != nil { return err }
node, ok := viewer.SelectedNode()
if ok { count, err := node.Int(); _ = count; _ = err }
```

- 设计器「数据视图」中可拖入 `JSONView`。持久属性：`JSON Text`、`Page Size`、`Root Pointer`。
- `PageSize` 默认 128，范围 16–256；大对象／数组以稳定排序的范围分组按需展开，每层最多该数量的子行。
  合成分组不是 JSON 值，不能误复制为其整个底层容器。
- `RootPointer` 用于只显示一个子树；`SelectPointer` 始终接收绝对 JSON Pointer，越出子树会报错。
- `SigSelected(func(string))` 对应 `OnSelected(pointer string)`，`SigError(func(string))` 对应 `OnError(message string)`。
  鼠标、键盘与程序定位共用选中通知。构造和属性修改不联网。
- `SelectedJSON/CopySelectedJSON/CopySelectedPointer` 用于取值／复制；剪贴板失败返回 error，不先清空原剪贴板。
  Ctrl/Cmd+C 复制 JSON，Ctrl/Cmd+Shift+C 复制路径。
- `SetJSONText` 是持久化的设计样例，不要放凭据／真实接口敏感内容。`LoadJSON/SetDocument` 只更新运行时显示。
- 解析失败保留之前的文档、展开状态和选中项，`LastError` / `SigError` 可查看错误。
- 控件方法仅在 UI 线程调用。大型解析放后台；默认文档不是无限数据集，也不是流式／JSONPath／JSON Schema 编辑器。
  显式展开大量分支仍会增加可见行与布局成本，没有宣称完整虚拟树或常数内存。

运行 `go run ./examples/json-inspector`。示例默认只请求自己的临时 localhost 端口；
`/json` 返回 2,000 条对象，`/invalid` 返回坏 JSON，`/slow` 延迟返回以测试取消。
取消或失败保留旧数据，新的完成通知通过 UI 队列派发并防止旧请求覆盖新状态。


## 流式下载：不把整个响应留在内存

```go
// 在 CLI 或后台工作函数中调用，不要阻塞 UI 线程。
result, err := httpx.Default().DownloadTo(ctx,
    httpx.Request{URL: url}, destinationWriter,
    httpx.DownloadOptions{
        MaxBytes: 64 << 20,
        ExpectedSHA256: expectedHash, // 可留空；填写时必须为 64 位十六进制
        OnProgress: func(p httpx.DownloadProgress) {
            // 这里仍在下载工作 goroutine，不能直接改 GUI。
            fmt.Printf("%d / %d bytes\n", p.Received, p.Total)
        },
    })
if err != nil { return err } // 不以 Received == Total 代替成功判断
fmt.Println(result.BytesWritten(), result.SHA256())
```

- `DownloadToAsync` 参数相同，返回 `*async.Future[*httpx.DownloadResult]`；复用 Client 的有界 Executor，快照请求 Header／Body。
- 下载大小默认 **256 MiB**，与缓冲式 Do 的默认 4 MiB 上限分开；复制缓冲区 **32 KiB**。
  这是 SDK 复制层的内存策略，不保证用户 Writer、Transport 或整个进程只占用 32 KiB。
- Client 默认 **30 秒整体超时**，包括读取 body、校验和进度回调所在的工作阶段；大文件可显式建立超时更长的 Client。
  callback／Writer 内部不可抢占，必须及时返回；取消不等于强杀用户函数。
- 进度 `Received` 是 Writer 已接受的字节数，`Total == -1` 表示未知；不是 TCP 流量。
  标准 Transport 透明解压时通常没有可用的解压后长度，大小限制与散列针对实际写入字节。
- 默认中间进度间隔 **100 ms**，可通过 ProgressInterval 配置；首次与最后一次变化强制报告，不重复报告相同计数。
  未到下个时间点但已结束时，最后计数不会因限频丢失。
- 非 2xx 响应只返回状态元数据，不把错误页面写入目标。大小超限、截断、短写、Writer 错误、取消、超时和校验失败均返回错误。
- `DownloadResult.StatusCode/Header/BytesWritten/TotalBytes/SHA256` 提供元数据。Header 是副本；完整 EOF 后才提供 SHA-256，校验失败时仍可检查实际摘要。
- 不执行重试、断点续传、自动文件命名、解压归档、权限变更或原子文件发布。
  不信任 Content-Disposition 作为写入路径，也不关闭调用者的 Writer。
- **失败／取消／哈希错误可能已产生部分或完整写入，SDK 不回滚 Writer。** 文件场景应由调用者使用临时文件，校验成功且关闭文件后再明确发布。
  异步 Writer 仅由工作拥有，等 Future.Done 后才能读取、关闭或复用；不能在 Cancel 后立刻关闭它。
- `async.Run` 既有语义会把返回成功后检测到的取消转为取消结果。因此取消后的 Future 可能不给出最后元数据，已发生的写入也不会消失；不得仅凭取消错误推断目标未被修改。

参考 [Qt 下载进度语义](https://doc.qt.io/qt-6/qnetworkreply.html#downloadProgress)、[Go HTTP Response](https://pkg.go.dev/net/http#Response) 与 [Writer 短写约定](https://pkg.go.dev/io#Writer)。

## 高频通知：一次 UI 排队，只保留最新快照

```go
progress := gui.NewTransferProgress()
progress.SetLabel("资源下载")
_ = progress.SetState(gui.TransferRunning)

updates, err := async.NewLatest(gui.Post, func(p httpx.DownloadProgress) {
    progress.SetProgress(p.Received, p.Total)
})
if err != nil { return err }

future, err := client.DownloadToAsync(ctx, request, destinationWriter,
    httpx.DownloadOptions{OnProgress: func(p httpx.DownloadProgress) {
        updates.Publish(p)
    }})
if err != nil { updates.Close(); return err }
_ = future.OnComplete(gui.Post, func(result *httpx.DownloadResult, err error) {
    updates.Close() // 后续排队的进度不能把终态覆盖为运行中
    if result != nil { progress.SetProgress(result.BytesWritten(), result.TotalBytes()) }
    if err != nil {
        _ = progress.SetState(gui.TransferFailed) // 可另判 context.Canceled
        return
    }
    _ = progress.SetState(gui.TransferSucceeded)
})
```

- `Latest.Publish` 可多 goroutine 调用，返回是否接受；连续 100,000 次发布且尚未消费时只排队一条通知。
- consume 中的新 Publish 在下一次 dispatcher 调度执行，不在当前回调里无限循环清空，给输入和绘制留出机会。
- 不创建 goroutine 或 timer。dispatcher 必须排队、恰好调用一次且不 panic；推荐 gui.Post。
- payload 应不可变或转移所有权；Latest 不深拷贝任意 T。**不适合日志、交易、按键等不能丢弃的事件**。
- Close 幂等，清理待消费 payload 和回调引用；拒绝后续发布，抑制尚未开始的消费；不能撤销已经开始的回调。
  GUI 宿主须在 UI 线程关闭 Latest，并给 Future 完成回调加已销毁／generation 检查。
  上面为最小配方，完整生命周期示例见 `examples/download-monitor`。

## TransferProgress 设计器／运行时接口

- 放在「网络与工具」分类，含主题适配的矢量下载图标。可拖入、保存、重载、预览与生成类型化 Go 代码。
- 设计属性：`Label`、`Show Bytes`；声明式入口 `decl.TransferProgress(decl.P("Label", "Download"))`。
- 运行时：`SetProgress(received, total int64)`、`ReceivedBytes/TotalBytes`、`Fraction`、`Summary`、`Reset`。
  getter 保留精确 int64；字节摘要采用 IEC 单位，展示值可能四舍五入。
- `SetState` 只接受 `TransferIdle/Running/Succeeded/Failed/Cancelled`；无效状态报错。
  `SigStateChanged(func(string))` 对应设计器事件 `OnStateChanged(state string)`，同值不重复通知。
- 字节到达总量不自动宣布成功；空内容也必须显式成功才显示完成。负总量归为未知，负接收值归为零，超过总量的原始计数仍可读回。
- 未知总量的条段是**静态活动标记，不表示 15% 的实际进度**；不会为 busy 状态持续唤醒 UI。
- Runtime 计数／状态不落入设计文件。所有控件调用与事件均在 UI 线程，不允许在 OnProgress 工作回调中直接改控件。
- 本轮未验证 Windows/Linux 原生 UI；构建与测试的具体范围见 [Transfer SDK QA](../TRANSFER_SDK_QA_20261023.md)。


## 可嵌入的响应式 WelcomeScreen

- `ged.NewWelcomeScreen()`：真实 Button/ListWidget 子控件，Tab 和列表键盘导航，
  窄窗／短窗滚动，主题字体缩放、两行最近文件及完整路径提示。
- `SetActionLabels(newLabel, openLabel)`：宿主动作名称经 i18n 翻译；
  `SetShortcuts([]ged.WelcomeShortcut{{Keys: "Ctrl+N", Label: "New Form"}})`：
  仅显示宿主已注册的快捷键，不注册全局按键；传空列表隐藏提示。
- `SetRecentFiles` 和 `SetShortcuts` 复制输入切片，调用者后续修改不改变动作目标。
  最近文件刷新时取消尚未完成的按下，空白／拖出释放不激活上一次选中项。
- `SetNewProjectCallback` / `SetOpenFileCallback` / `SetOpenRecentCallback` 保持原接口；
  传 nil 禁用对应动作。所有控件 API 均需在 UI 线程调用。
- 保留 `SetBranding` 的宿主 logo／名称／版本隔离；不添加轮询或动画 timer。
  验证范围见 [Welcome UI QA](../WELCOME_UI_QA_20261025.md)。

## 设计器控件面板与属性表的显示边界

- 控件面板图标按统一 18 逻辑像素画布缩放，使用独立裁剪和绘图状态，避免覆盖文字。
- 内建分类按界面语言显示；`RegisterWidget` 的既有分类身份、factory、拖拽数据不变。
- `PropertySheet` 的过滤提示及分类标题支持中英文；属性 ID 和筛选输入不翻译，不改变绑定语义。
- 验证范围与已知缺口：[设计器细节 QA](../DESIGNER_DETAILS_QA_20261025.md)。

## 原生文件面板焦点恢复与设计命令可用性

- macOS `OpenFileDialog` / `SaveFileDialog` 在原生 sheet 结束后先恢复 owner 的原始启用状态，再恢复键盘焦点；无需点击画布来重新激活快捷键。
- 退出、失效/隐藏/禁用 owner、最小化、其它 app 或 key window、其它 sheet 不抢焦点；异步取消与引用计数释放路径保留。不增加轮询或绘制 timer。
- IDE 的布局与解除布局、适应视口命令复用设计工具栏的模式/选区保护，命令面板显示不可用状态并阻止提交；执行前再次检查，避免模态退出期间状态变化后操作隐藏画布。
- 验证：[快捷键与命令状态 QA](../SHORTCUT_ROUTING_QA_20261025.md)。

## 精度安全的属性编辑与窄列呈现

- `prop.FloatEdit` / `prop.NewFloatEdit()` / factory `prop.control.FloatEdit`：
  float32/float64（含命名类型）的默认属性编辑器；显式 `CfgWidgetType` 优先。
  非聚焦态仅绘制 7/10 位有效数字；编辑缓冲和 tooltip 保留完整 round-trip 文本，
  不因显示、聚焦或失焦修改模型精度。
- 有效输入沿现有属性命令实时提交，支持科学计数；拒绝不完整、尾随垃圾、溢出、NaN/Inf。
  Enter/失焦恢复实际值；Up/Down 步进 1，多选从首项起步，同一命令保存各项撤销值。
  逐字输入保留现有逐次提交语义，不保证整段输入合并成一条 Undo。
- 常用历史中文属性 ID 只翻译显示名，显式配置标签保持原样；绑定、序列化 ID 不变。
  文本／整数／浮点／颜色／布尔的混合值提示使用主题文字色并提供完整 tooltip。
- `gui.ElideText(font, text, width)`：按逻辑像素和实际字体测量，以 Unicode 省略号截断；
  nil 字体、非正宽度、NaN 宽度返回空串。调用者仍需裁剪分配区域以约束字形外伸。
- `gui.Edit.ScrollToCaret`：单行禁止垂直滚动，过矮多行框至少按一行计算，
  修复小数行高下点击后文字消失／下一键再出现的问题；不增加 timer 或轮询。
- 所有控件与属性调用仍限 UI 线程。自动化、原生桌面与已知缺口见
  [属性面板 QA](../PROPERTY_POLISH_QA_20261025.md)。


## 输入框横向可见性与连续属性编辑

- `gui.Edit` 使用已经排版的字形配置隐藏横向滚动范围；`ScrollToCaret`／`SetSelection`
  保持活动光标可见，支持单行与非换行多行，不给紧凑行新增滚动条高度。
- 短值回写夹紧选区，字体／尺寸变化重算范围；文字与选区在内容视口裁剪，避免覆盖边框。
  光标导航不重排文本，不新增 timer；输入校验与文本替换统一使用 rune 偏移。
- SilkIDE 的设计历史保留仍然可用的属性控件焦点，不恢复已移除、重绑、隐藏、禁用
  或不同根窗口的控件。此项是 IDE 命令路由改进，不改变 SDK UndoStack 的通用焦点语义。
- 属性面板 Clear/Bind 会先使旧属性行失去写入权限，阻止迟到的失焦／对话框回调
  修改旧模型或向新 owner 错发命令；外部 owner 的 Clear 不影响当前绑定。
- 验证及限制：[输入框与焦点 QA](../EDIT_FOCUS_QA_20261025.md)。

## 内建拖拽反馈与光标资源生命周期

- `gui.LoadCursorData(name)` 优先读取 `cursor/name[_hotX_hotY].png`，缺少可选
  资源时提供内建 `arrow-no`、`arrow`、`arrow-copy`、`arrow-link`；操作徽标不同，
  不再全部默默退化为同一箭头。损坏 PNG、超过 256×256、越界热点会返回错误。
- `gui.GenerateDropCursors(content)` 返回拒绝／移动／复制／链接四个光标；
  真正绘制内容缩略图，预览保持比例并限制在 80×64，仅在拖拽开始合成。
  `content` 为借用资源，调用方仍负责自己的生命周期。
- 新增 `gui.ReleaseDropCursors(cursors)`：恢复先前光标后在 UI 线程调用，
  确定性释放临时原生句柄，跳过共享 fallback 和重复项；`Window.DoDragDrop`
  已自动调用。`LoadCursorData` 返回的像素由调用方持有，构建光标后应 `paint.CloseSurface`。
- GLFW 拖拽不再每毫秒主动唤醒；等待原生输入／UI 唤醒，最迟约 1/60 秒处理空闲工作。
  Escape 请求取消，普通键盘字符不泄漏到背景编辑器，结束时清理已启用与待启用捕获。
- Windows OLE 拖拽现在优先响应 Escape 取消，取消与松开按钮同时到达也不误投放；
  拖拽源正确提供 `IDropSource` 而非错误的 `IDropTarget` COM 身份。
- `examples/drag-feedback` 演示四种操作、数据载荷和状态快照；包含小尺寸布局、文字可见性测试。
  真实桌面验证与剩余限制见 [拖拽反馈 QA](../CURSOR_FEEDBACK_QA_20261025.md)。

## Windows 原生模态循环中的 UI 调度

- `gui.Post` 使用隐藏的 message-only HWND 接收唤醒，工作线程不直接修改控件；
  OLE 接管消息循环时仍能分批执行 UI 任务，不再依赖被丢弃的线程消息。
- `gui.Timer` 通过原生 `TIMERPROC` 分发，拖拽期间 SDK 定时器和渲染截止时间继续推进；
  同一定时器回调打开嵌套循环时不会递归重入，其他定时器与任务仍能执行。
- 原有脏区域合并、绘制互斥和帧率策略不变；静止拖拽不需要持续重绘。
  UI 消息窗口退出时先隔离并等待正在发送的唤醒，再销毁 HWND，避免向复用句柄发消息。
- `Timer.Start/Stop` 仍只允许在 UI 线程调用；后台业务使用 `gui.Post`。
  这不是为耗时回调提供抢占，也不改变原生线程定时器可能复用 ID 的既有限制。
- 示例设置 `SILK_DRAG_PUMP_PROBE=1` 可记录拖拽期间 Timer/Post/绘制计数；默认关闭，
  不为普通用户引入采样定时器。实机证据见 [调度 QA](../OLE_UI_PUMP_QA_20261025.md)。


## 模态安全定时器与后添加对话框按钮

- GLFW `gui.Timer` 对每次启用记录回调运行状态；同一回调打开模态循环时不递归进入自身，
  其他定时器继续推进。返回或 panic 后解除运行状态；停止／重启不会错误清理新记录。
  Windows 既有同 ID 重入防护保持不变，原生 ID 复用边界仍未消除。
- `gui.Dialog.Layout` 显式布局按钮容器；`SetSize` 后再 `AddButton`，即使 footer 尺寸
  不变，新增按钮也得到有效区域。右侧 24 像素留白与按钮间距保持一致。
- `examples/timer-lifecycle` 展示模态回调、可重启两秒延迟、取消与 JSON 状态快照。
  示例仅在模态显示期间启用 100ms 心跳；没有为框架新增轮询或后台任务。
- `Start/Stop`、控件读写仍限 UI 线程；异步业务先通过 `gui.Post` 回到 UI 线程。
  该改进不保证耗时回调可被抢占，也不把低精度 UI 定时器变为实时调度器。
- 单元测试、修复前失败证据、真实桌面闭环及限制见
  [定时器与对话框 QA](../TIMER_LIFECYCLE_QA_20261025.md)。


### 可直接编辑的整数控件

`gui.SpinBox` 保持原有整数 API 和 `Widget` 嵌入，内部只有一个逻辑 `Edit` 子控件，
不创建独立原生窗口，不添加轮询、定时器或后台 goroutine。

- `Text()` 读取数字草稿（不含后缀）；`Value()` 只返回已提交值。`SelectAll()` 选择数字。
- Enter 或正常失焦提交；`Commit() bool` 可显式提交。超出控件范围时夹取边界，
  空白／只有正负号恢复原值。`CancelEdit()` 或根窗口内 Escape 放弃草稿。
  模态对话框的 Escape 优先关闭对话框，不承诺只取消字段。
- 复用 Edit 的选区、剪贴板、局部 Undo/Redo、IME 路由；每次提交清理字段历史。
  拒绝非法字符、机器整数溢出以及超过 64 字节的草稿，不截断成另一个数字。
- `SetValueChangedCallback` 只在已提交值真正变化时触发一次；方向键步进会把草稿和
  步进合成一次变更。反复设置未变的规范值不会重新整形文本。
- `SetReadOnly` 保留选中／复制能力；禁用、隐藏或失去有效父链后不接收编辑输入。
  `SetEnabled(false)`、`SetReadOnly`、显式 `SetValue` 会撤销未提交草稿。
- Up/Down、PageUp/PageDown、鼠标按钮与滚轮有溢出保护；保留原有无修饰 Home/End
  跳到范围边界的行为，Shift+Home/End 仍用于选区。
- `PageStepOverride()` 返回配置值：0 表示跟随 `Step()*10`，有效分页步长通过
  `PageStep()` 读取并在整数上限饱和。设计器保留自动模式而非写死计算值。
- 设计器新增“分页步长”“后缀”“只读”属性，保存重载和 Go 代码生成保持一致；
  内部输入框不进入 `.silkui` 对象列表。打印份数和甘特图任务进度自动获得输入能力。

示例：`go run ./examples/spinbox-edit`。验证范围和实机证据见
[整数控件 QA](../SPINBOX_EDIT_QA_20261025.md)；不代表浮点数、地区化数字或 Qt 的所有输入行为已实现。

后续已修复整数范围恢复顺序：先设置完整范围，再恢复初始值；原先 `-100..100` 中
`-42` 重载归零、`200..300` 中 `250` 被裁剪的问题均加入回归。设计器重载、复制到
运行预览、声明式构建共用底层处理，不额外创建窗口、轮询或 goroutine。

```go
node := decl.New("gui.SpinBox",
    decl.P("value", -42), // 属性顺序不影响结果
    decl.P("min", -100),
    decl.P("max", 100),
)
widget, err := node.Build()
_ = widget
_ = err
```

`value`/`Value` 字面量现在支持 `SetValue(int)`；整数最小/最大值不经过 `float64`，
因此超过 `2^53` 的整数也不丢精度。拒绝小数、非有限值、机器整数溢出及表达式文本。
设计器的 `ApplyPersistedProperties` 仍只恢复控件明确枚举且可写的项，不反射调用任意 setter。
具体范围、实机闭环及未覆盖项见 [整数范围 QA](../INTEGER_RANGE_QA_20261025.md)。


## 可校验输入弹窗与统一重命名

- `InputBox.SetValidator(Validator)`、`HasAcceptableInput()`、`ValidationError()`
  和 `Text()` 区分有效值与编辑草稿；`ShowValidatedInputBox` 一次配置并显示。
  `nil` 校验器保留既有 `ShowInputBox` 的自由输入兼容性。
- 非法草稿保留在输入框中，显示可换行的错误原因、禁用确认按钮；鼠标与 Enter
  提交都会重新校验，Escape／取消仍可退出。提示区和按钮区独立布局，不互相覆盖。
  `ValidatorFunc` 允许暂时无效的输入；校验器应快速、无副作用，只在 UI 线程运行。
- `GoIdentifierValidator()` 拒绝空白、关键字、空标识符 `_` 和非法 Go 名称，允许
  Unicode 字母；不负责语义作用域或名称冲突检查。错误文本经过 `i18n.T`，已提供
  内置中文默认译文，宿主可覆盖翻译。
- `CodeEditor.WordAtCursor()` 是不移动光标的零分配词法查询，不是符号解析；
  允许数字、关键字、注释内单词。只扫描当前行前缀和相邻标识符，越界列仅对查询夹取。
- SDK 键盘与上下文菜单重命名统一调用 `RequestRenameSymbol()`，优先宿主
  `CodeEditorSymbolActions.RenameSymbol`。本地 AST 回退现在支持文本 Undo/Redo，
  仍按名称处理单缓冲区，并非作用域感知重构；IDE 使用 gopls 语义重命名。
- IDE 先同步全部已打开缓冲区，再请求重命名；应用前验证受影响缓冲区的编辑版本、
  实例、路径、归属和只读状态。模态期间还核对光标，避免把结果应用到新目标。
  不增加常驻定时器或轮询。未打开文件的外部写入竞争、多文件磁盘写入原子性和
  工作区整体撤销仍未解决。详见 [重命名 QA](../RENAME_DIALOG_QA_20261026.md)。

```go
name, accepted := gui.ShowValidatedInputBox(
    parent, nil, "Rename Symbol", "New name:", "oldName",
    gui.GoIdentifierValidator(),
)
if accepted {
    // 在宿主内执行语义重命名；此处的 name 已通过词法校验。
    _ = name
}
```


## 统一的弹窗图标与标准按钮本地化

### 窗口身份与资源所有权

- 新建 `WtForm` 在首次显示之前，按“控件显式的非空图标 → 所属原生窗口 → 已配置／缓存的
  应用图标”选择图标。适用于普通 Form、Dialog、MessageBox、InputBox 及嵌套弹窗。
  无配置的独立 Form 保留系统默认图标；Frame 保留原有按可执行文件名称查找图标的行为。
- `InputBox.SetIcon()` 与继承的 `Icon()` 现在使用同一属性，不再出现写入／读取不一致。
- `Form.SetIcon` 对已附加的原生窗口立即生效；传 `nil` 或空图标重新继承当前 owner／app，
  都没有时恢复系统默认。嵌入父窗口中的纯逻辑 Form 不会修改父窗口的图标。
- `Window.Icon()` 返回窗口的逻辑图标来源，不是系统 class icon 或 `.app` 图标文件的回读。
  既有 `Window.SetIcon(nil)` 保持不操作；需要恢复继承时使用 `Form.SetIcon(nil)`。
- 继承是创建／显式重置时的快照，不订阅父图标变化。所有窗口与控件方法仍限 UI 线程，
  后台调用应通过 `gui.Post`。不增加轮询、计时器或每帧图标查找。
- Windows 分别生成小／大图标并自行持有、替换、销毁，不借用父窗口 HICON；关闭子窗口
  不会释放父图标。菜单 `WtPopup`、内嵌 `WtChild` 不自动生成标题栏／任务栏图标。
  macOS 不调用不受支持的 GLFW 窗口图标 API，原生应用图标继续由 `.app` bundle 管理。

```go
// UI 线程；实际应用可以换成自己的品牌素材。
gui.SetAppIcon(paint.LoadIcon("document"))
form := gui.NewForm()
form.SetVisible(false)
form.SetParent(owner)
form.AttachWindow(gui.WtForm)
form.SetIcon(paint.LoadIcon("folder")) // 立即更新本窗口
form.SetIcon(nil)                      // 重新继承 owner/app，而非修改 owner
selected := form.Window().Icon()
_ = selected
```

### 标准显示文字与语义 ID 分离

- `@ok`、`@cancel`、`@save`、`@discard`、`@yes`、`@no`、`@apply` 及批量保存／丢弃／关闭
  的显示文字按当前 `i18n` locale 翻译；`SigSubmit` 和 `ExtraData` 始终保留原始命令 ID。
- 普通自定义字符串不会自动翻译。例如字面值 `"Cancel"` 仍由宿主决定如何显示；这与
  标准命令 `"@cancel"` 不同。`ShowMessageDialog` / `ShowConfirmDialog` 的内建按钮
  也使用翻译，返回的 `DialogResult` 不变。
- 翻译在构建／重新设置标准按钮时解析，不实时重译已经打开的全部界面。宿主可在
  导入 SDK 后覆盖默认译文，未知语言仍回退原文。验证范围见
  [弹窗细节 QA](../DIALOG_POLISH_QA_20261026.md)。

## 同步 UI 调用、可取消投递与高频进度合并

新增可组合的事件驱动接口（不创建轮询线程）：

| API | 用途 |
| --- | --- |
| `gui.IsUIThread()` | 查询调用者是否是事件循环所属线程；不表示循环已经启动 |
| `gui.Invoke(ctx, func() error)` | 工作线程同步等待一个短 UI 操作；UI 线程调用时直接执行 |
| `gui.InvokeValue[T](ctx, func() (T, error))` | 返回类型安全的 UI 查询／操作结果，无需共享输出变量 |
| `gui.PostContext(ctx, func())` | 投递到下一轮；执行前 context 已取消则丢弃 |
| `gui.NewLatestUI[T](consume)` | 合并进度、预览等可替换快照，每实例最多一个待执行／执行中投递 |
| `Widget.TryAttachWindow(wt)` | 返回原生附加错误，错误线程／无父级 child 在修改控件前被拒绝 |

```go
// 在后台 goroutine 中；窗口创建与属性访问只发生在 UI callback 中。
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
text, err := gui.InvokeValue(ctx, func() (string, error) {
    return input.Text(), nil
})
_ = text
_ = err

// 在 UI 线程创建，并在接收控件销毁前 Close。
progress, err := gui.NewLatestUI(func(percent int) {
    label.SetText(fmt.Sprintf("%d%%", percent))
})
_ = err
// 工作线程可调用 progress.Publish(percent)。
// 关闭时：progress.Close()
```

- `Invoke`／`InvokeValue` 的队列等待可以取消；尚未开始的 callback 被退休并释放捕获引用。
  **已开始的 callback 不能抢占**，可能在调用者返回 `ctx.Err()` 后结束。应通过返回值传递结果，
  不要写工作线程的局部输出变量，不要在 UI 内等待正在等待 UI 的工作线程。
- `Invoke` 的 panic 以 `*gui.UIInvocationPanicError` 返回，`errors.As` 可识别，若 panic 值是 error，
  `errors.Is` 可追溯；禁止 callback 使用 `runtime.Goexit`。普通 `Post` 的日志行为保持不变。
- `Invoke`／`InvokeValue`／`PostContext` 的 nil context／callback 返回 `ErrUIInvalid`；
  `NewLatestUI` 的 nil consumer 沿用 `async.ErrInvalid`。无后端线程身份时 `Invoke` 返回 `ErrUIUnavailable`。停止的循环不会处理
  队列，因此工作线程必须设置 deadline。`PostContext` 不返回完成通知，捕获对象保留到队列执行该槽。
- `NewLatestUI` 复用 `async.Latest`，不会为每个进度创建 goroutine；中间值可丢弃，不替代逐条事件流。
- 原生 Win32 创建检查改用真实线程身份，而非 `GetCurrentThread()` 伪句柄。
  现有 `AttachWindow` 保持签名；需要错误反馈时使用 `TryAttachWindow`。
  **其他控件方法仍是 UI-thread-only**，这些 API 并非为整个控件树加锁。

运行示例：`go run ./examples/ui-dispatch`。
