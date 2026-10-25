# Silk SDK：小工具组装功能清单

本页列出 HTTP／同步异步能力、常用组装入口与设计器生成约定；不是对所有 SDK 模块的完整质量或跨平台认证。
这些新增能力不增加第三方依赖。验收范围见 [HTTP SDK QA](../SDK_HTTP_QA_20261023.md) 、[字体／生成安全 QA](../FONT_GENERATION_QA_20261023.md) 和 [流式传输 QA](../TRANSFER_SDK_QA_20261023.md)。

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
| 编辑器文件类型 | `CodeEditor.Language()`、`SetFilePath()` | 自动识别 Go／SilkGo／普通文本；切换类型清理旧投影与后台候选，不修改正文 |

既有组装入口：`gui.Edit`、`ComboBox`、`Label`、`Button`、`CodeEditor`；
布局使用 `HBox`、`VBox`、`FormLayout`、`GridLayout`；设计文件使用 `ged`，声明式树使用 `decl`。
图表／甘特图、打印、数据绑定等既有模块见 [README](../README.md)，不把本轮 HTTP 验收扩展为它们的重新验收。

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

`Language()` 是 UI 线程 getter，返回 `go`、`silkgo` 或 `plaintext`，由
`SetFilePath()` 的扩展名自动决定（不区分大小写）。未命名编辑器保留原有 Go 默认；
其他已命名文件，包括 `.txt`、`.json`、README 与 go.mod，暂按普通文本处理。
这不是 JSON／Markdown 解析器，也不是手动语言选择器或多语言 LSP 管理器。

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
