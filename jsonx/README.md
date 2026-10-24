# jsonx：可复用的只读 JSON 文档

无第三方依赖、无 GUI 初始化。适合 HTTP 返回值、配置查看与字段提取。

```go
doc, err := jsonx.Parse([]byte(`{"data":{"count":9007199254740993,"name":"Silk"}}`))
if err != nil { return err }
count, err := doc.Int("/data/count") // int64，精确保留大于 2^53 的整数
name, err := doc.Str("/data/name")
node, err := doc.Lookup("/data")
children, err := node.Children(0, 64) // 对象键稳定排序；只取直接子节点
```

- `Document.Root/Lookup/NodeCount/Str/Int/Float/Bool`：解析一次，重复查询。
- `Node.Kind/Key/Pointer/Len/Child/ChildIndex/Children`：只读树与分页访问。
- `Node.Value/JSON` 返回独立 Go 值或 JSON 字节；修改它们不影响原文档。
- `Node.Preview(n)`：有界显示摘要，不保证是合法 JSON。容器只显示数量，长字符串先截短再转义。
- `SplitPointer/JoinPointer`：RFC 6901 字符串语法。根是 `""`，空字段名是 `/`；`~0` 表示 `~`，`~1` 表示 `/`。
  JoinPointer 的 base 必须已经有效；不是 JSONPath，也不接受 `#` URI-fragment、通配符、负索引或前导零数组索引。
- 数字保持 `json.Number`。`Int` 拒绝小数、指数写法、溢出；`Float` 可能受 float64 精度限制，但拒绝无穷大。
- null 是有效节点，`Kind()==jsonx.Null`；零值 Node 无效，不能只用 `Value()==nil` 判断缺失。
- 无语法执行、网络访问、自动解密或动态类型推断；采用 Go JSON decoder 语义，重复对象键最后一个值胜出。

## 限制与并发

`ParseWithOptions(data, jsonx.Options{MaxBytes: ..., MaxDepth: ..., MaxNodes: ...})`：
零值默认分别为 **4 MiB / 128 / 100,000**，负数拒绝。根深度为 0，节点数包含根。
只接受一个完整 JSON 值和有效 UTF-8 字节；不接受尾随第二个文档。
大小在解码前检查，深度／节点上限在构造只读树时检查（不是流式解析器）。

成功构造后 Document 和 Node 可被多个 goroutine 只读共享，不要覆盖 Document 本身。
返回的 Value/JSON 是副本，不共享可变的 map/slice。

```go
future, err := jsonx.ParseAsync(ctx, executor, data, jsonx.Options{})
if err != nil { return err }
_ = future.OnComplete(gui.Post, func(doc *jsonx.Document, err error) {
    if err != nil { status.SetText(err.Error()); return }
    if err := viewer.SetDocument(doc); err != nil { status.SetText(err.Error()) }
})
```

异步入口先复制有界输入，使用既有 bounded executor。取消检查在解析前后，不能在 JSON token 中间强杀解析。
调用者负责窗口销毁时取消、失效旧回调；不要在 UI 线程等待 `Await`。
`jsonx` 自身不导入 `gui`，上面仅展示 GUI 宿主如何选择 dispatcher。

## 与 HTTP / 设计器配合

`httpx.Response.Document()` 或 `DocumentWithOptions(options)` 可以构造文档。
它们是**同步解析、每次调用重新解析**，不会改写已有 JSON/Str/Int API 的行为；请保留返回的文档供多次查询。
大响应的请求和解析都应在 `gui.RunAsync` 的 work 中完成。

`gui.JSONView` 支持分组展开、字段定位和复制；`decl.JSONView` 可声明式组装。
属性 `JSON Text` 是设计样例，会持久化；运行时响应请用 `LoadJSON` / `SetDocument`，它们不会改设计样例。
更多 GUI 接口见 `docs/SDK_FEATURES.md`，验收与平台边界见 `JSON_INSPECTOR_QA_20261023.md`。
