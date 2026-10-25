# formatx：离线 Go / JSON 格式化

无 GUI 依赖，不启动外部程序、不写临时文件、不依赖 `gofmt` 的 PATH。

```go
text, err := formatx.Format(ctx, "json", input, formatx.Options{})
future, err := formatx.FormatAsync(ctx, executor, "json", input,
    formatx.Options{Compact: true})
// GUI 回调必须选择 UI 派发器；处理上面的 admission error 后再订阅。
if err == nil {
    err = future.OnComplete(gui.Post, func(text string, err error) {
        // 检查错误与宿主生命周期，再应用结果。
    })
}
```

- `go` 使用 `go/format`；不支持 Compact，不处理未展开的 SilkGo。
- `json` 美化为两空格缩进、末尾一个 LF；Compact 删除非字符串内的空白，无末尾 LF。
  不经过 `map` 或浮点数解码，所以保留键顺序、重复键、整数／指数写法和字符串转义。
- 输入必须是有效 UTF-8；语法错误不返回部分正文。不是 JSONC、Schema 校验器或自动修复器。
- 输入最多 8 MiB，结果最多 16 MiB，JSON 最大嵌套 128 层。JSON 在分配缩进结果前
  预估膨胀大小；Go 的输出大小在格式化后检查，这不是总峰值内存上限。
- `Format` 同步；`FormatAsync` 用传入的 `async.Executor`，nil 使用共享两任务执行器。
  无无界队列，满载返回 `async.ErrBusy`。两者支持 Context；取消是阶段间合作式检查，
  不能强制中断标准库解析器，不能保证任意大文件的响应延迟。
- 错误类别：`ErrLanguage`、`ErrTooLarge`、`ErrTooDeep`、`ErrUTF8`，以及标准库语法错误／Context 错误。

嵌入编辑器优先用 `CodeEditor.FormatDocumentAsync`：它负责版本校验、取消、UI 派发和一次 Undo。
独立示例：`go run ./examples/editor-format`。验收见 [格式化 QA](../EDITOR_FORMAT_QA_20261025.md)。
