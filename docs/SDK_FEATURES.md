# Silk SDK：小工具组装功能清单

本页列出本轮新增的 HTTP／同步异步能力，以及常用组装入口；不是对所有 SDK 模块的完整质量或跨平台认证。
新增能力不增加第三方依赖。验收范围见 [HTTP SDK QA](../SDK_HTTP_QA_20261023.md)。

## 能力索引

| 场景 | 入口 | 说明 |
| --- | --- | --- |
| 同步执行可取消工作 | `async.Run[T]` | 不创建 goroutine；捕获 work panic 为 `PanicError` |
| 有界后台任务 | `async.NewExecutor`、`async.Submit[T]` | 限制同时运行任务；满载立即返回 `ErrBusy`，没有无界等待队列 |
| 等待／取消／关闭 | `Future.Await/Cancel/Done`、`Executor.Close/Wait` | 合作式取消，不能强杀忽略 Context 的用户函数 |
| 完成通知 | `Future.OnComplete` | 显式指定 dispatcher；没有每次订阅的 watcher goroutine |
| GUI 异步包装 | `gui.RunAsync[T]` | 后台 work + `gui.Post` 主线程完成通知 |
| 通用 HTTP | `httpx.Client.Do/DoAsync` | Method、URL、Header、Body；返回有大小限制的 Response |
| GET | `Client.Get/GetAsync` | 同步／异步对称入口 |
| JSON POST | `Client.PostJSON/PostJSONAsync`、`httpx.JSONRequest` | 自动编码请求体并设置 JSON Content-Type / Accept |
| 字符串／字节 | `Response.Text/Bytes` | 整个响应；Bytes 和 Header 返回副本 |
| JSON 字段 | `Response.JSON/Str/Int/Float/Bool` | JSON Pointer，严格类型，缺失／格式错误不冒充零值 |
| 通用结果转换 | `Response.Extract(kind, pointer)` | `str/json/int/float/bool`，适合配置驱动的小工具 |
| 可视请求按钮 | `gui.HTTPButton` | 显式点击才请求；忙碌时再点击取消；UI 线程结果信号 |
| 声明式组装 | `decl.HTTPButton` | 使用 `decl.P` 设置属性；Build 不联网 |
| 设计器组装 | 网络与工具 → `gui.HTTPButton` | 拖放、属性保存／重载、预览构造、强类型导出、6 个事件入口 |
| 可运行示例 | `examples/http-tool` | URL、方法、返回类型、JSON 字段、请求体、结果编辑器、错误／取消状态 |

既有组装入口：`gui.Edit`、`ComboBox`、`Label`、`Button`、`CodeEditor`；
布局使用 `HBox`、`VBox`、`FormLayout`、`GridLayout`；设计文件使用 `ged`，声明式树使用 `decl`。
图表／甘特图、打印、数据绑定等既有模块见 [README](../README.md)，不把本轮 HTTP 验收扩展为它们的重新验收。

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
**IDE 自动 `.silk.go` 树生成与内嵌事件导出尚未统一**，不能据此宣称内嵌回调在默认预览中已执行。
本轮 Windows GUI 仅交叉构建；Linux 只验证两个新无 GUI 包的测试二进制交叉编译。
macOS 默认代码字体仍有中文／emoji 字形缺失，网络与解析测试中的 Unicode 数据未丢失；字体回退另行跟进。
