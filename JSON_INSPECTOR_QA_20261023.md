# JSON Inspector / SDK 验收记录 — 2026-10-23

> 后续 Windows 10 原生补测及修复结果见 [WINDOWS_TRANSFER_QA_20261023.md](WINDOWS_TRANSFER_QA_20261023.md)；本文下方环境与结果仍是当轮 macOS 验收记录。

## 本轮范围

基线 HEAD：`1dd5c05432244057ecf67b0abbe6c91eb6a4ed0b`。工作树包含上一轮尚未提交的字体与安全生成修复，本记录不将它们重复计为本轮新增功能。没有进行 commit、push 或发布。

新增一条可组装的链路：`httpx.Response` → `jsonx.Document` → UI 队列 → `gui.JSONView`，并接通设计器与代码生成。

- `jsonx`：只读 JSON 文档、精确整数、字符串／整数／浮点／布尔提取、JSON Pointer、分页子节点、受限预览、异步解析。
- `JSONView`：键／类型／值三列、大容器范围分组、路径定位、选中通知、复制路径／节点 JSON；继承既有 TreeView，不创建独立浮窗或轮询计时器。
- `httpx.Response.Document/DocumentWithOptions`：不改变旧提取 API；返回文档由调用者保存复用，每调用一次仍会重新解析。
- 设计器：控件面板、拖入、属性保存／恢复、`OnSelected`／`OnError` 事件、强类型导出、`decl.JSONView`。
- 示例：`go run ./examples/json-inspector`。默认使用自己的临时 localhost 服务，只有点击 Fetch 才请求；可显式修改 URL。
- 原生测试发现列宽缓存停留在初始化尺寸，已修复 JSONView 的 resize 布局，保留展开状态和选中项，并添加回归测试。

设计参考：Qt 的[按需加载模型示例](https://doc.qt.io/qt-6/qtwidgets-itemviews-fetchmore-example.html)启发范围分组／按需物化；路径采用 [RFC 6901](https://www.rfc-editor.org/rfc/rfc6901) 字符串形式；数字采用 Go 的 [Decoder.UseNumber](https://pkg.go.dev/encoding/json#Decoder.UseNumber)。没有引入 Qt 依赖，也没有实现完整 Qt fetchMore 协议。

## 环境与自动化结果

macOS 27.2 / arm64 / Apple M2 Max；`go version go1.26.10 darwin/arm64`。
本地证据目录（被 Git 忽略）：`bin/qa-artifacts/json-inspector-20261023/`。

| 检查 | 结果 | 证据 |
|---|---|---|
| 明确列出的 12 个包，非 short | 4,128 pass / 10 skip / 0 fail | `packages-final.jsonl`、`packages-final.exit` |
| 8 个包，race + short | 3,697 pass / 37 skip / 0 fail，无 race 报告 | `race-final.jsonl`、`race-final.exit` |
| JSON Pointer fuzz，2 workers，3 秒目标 | 16,718 次执行，PASS | `fuzz.log`、`fuzz.exit` |
| 纯 SDK 默认 vet | PASS | `vet.log`、`vet-build.exit` |
| GUI 相关 vet，关闭已有 composites 告警 | PASS | 同上 |
| macOS native IDE / JSON 示例构建 | PASS，示例另有实窗验收 | `silkide`、`Silk JSON QA.app/Contents/MacOS/json-inspector` |
| Windows amd64 IDE，silk_pure_go | 交叉构建 PASS；没有执行 | `silkide-windows-purego.exe`、`cross.exit` |
| Windows / Linux amd64 jsonx 测试二进制 | 交叉编译 PASS；没有执行 | `jsonx-windows.test.exe`、`jsonx-linux.test` |

pass / skip 数包含子测试与 fuzz seed，不代表同数量的独立测试函数。非 short 测试包含设计器导出后通过独立 Go 编译器编译、运行生成 UI 并验证事件的集成测试。race 的 short 模式会跳过这类外部编译。

跳过项包括平台条件、非 widget 的 gui.Action、显式 opt-in 的 native GL／图集／甘特图 fixture、当前字体不覆盖 U+20000，以及 3 个 real-gopls 用例。本轮没有将它们记为通过。
既有 GUI 测试环境日志还含 core.Close 异常退出标记和临时测试目录缺少 icon 资源的警告；相关测试退出码为 0，但不宣称日志无警告。全仓默认 vet 的既有 unkeyed composites 告警不在本轮清理范围。

可复跑命令（仅明确包，不遍历受限 local）：

```sh
go test -count=1 -p=2 -json \
  ./jsonx ./httpx ./async ./gui ./ged ./decl ./paint \
  ./examples/json-inspector ./examples/http-tool ./cmd/silkide \
  ./internal/filetxn ./internal/silkgen

go test -race -short -count=1 -p=2 -json \
  ./jsonx ./httpx ./async ./gui ./ged ./decl ./paint ./examples/json-inspector

go test -run '^$' -fuzz '^FuzzPointerRoundTrip$' -fuzztime=3s -parallel=2 ./jsonx
go vet ./jsonx ./httpx ./async
go vet -composites=false ./gui ./ged ./decl ./paint ./examples/json-inspector

go build -o bin/qa-artifacts/json-inspector-20261023/silkide ./cmd/silkide
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags silk_pure_go \
  -o bin/qa-artifacts/json-inspector-20261023/silkide-windows-purego.exe ./cmd/silkide
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c \
  -o bin/qa-artifacts/json-inspector-20261023/jsonx-windows.test.exe ./jsonx
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c \
  -o bin/qa-artifacts/json-inspector-20261023/jsonx-linux.test ./jsonx
```

## macOS 真实窗口验收

用本轮构建的 `Silk JSON QA.app` 内可执行文件、隔离 HOME 和 CUA 操作，窗口为 840 × 600。

1. Fetch `/json`：显示 `Loaded 6007 nodes`，只展开需要的分支，未一次性生成所有行。
2. Locate `/data/name`：看到 `Silk 中文😀`；同层数字 `9007199254740993` 保持完整，无 float64 截断。
3. Locate `/data/items/1999/id`：展开目标范围，定位到 number 值 `1999`。
4. 点击 Copy path，再粘贴到路径输入框：实际回读 `/data/items/1999/id`。
5. 点击 Copy JSON，再粘贴到路径输入框：实际回读 `1999`。
6. Fetch `/invalid`：可见 JSON 解析错误，旧文档及原选中项保留。
7. Fetch `/slow` 后在同一次自动化调用中立即点击 Cancel：显示 `Cancelled; previous data retained`。
8. Cmd+Q 退出；进程检查确认该 QA 实例已停止。

截图经 CUA 在会话内检查，未另存为仓库 PNG。复制验证针对按钮与系统粘贴；没有另做树焦点下 Ctrl/Cmd+C 快捷键的原生验收。缩放保留选中状态由自动测试验证；未完成实际多屏／混合 DPI 拖动验收。设计器拖入／保存／导出链路是自动化集成验收，不冒充本轮另做了设计器窗口鼠标拖拽。

启动边界：最初两次经 app wrapper 自动启动超时。采样分别停在 dyld 打开文件和 Go syscall 打开路径附近；终止自有 QA 进程后，隔离 HOME 直接启动 bundle 内二进制，CUA 成功绑定并完成上述操作。采样见 `native-sample.txt`、`native-sample-bundle.txt`。根因未确定，未关闭系统安全设置；此结果不代表通用 macOS 启动延迟已经修复。

## 微基准（回归完成后单独运行 3 次）

```sh
go test -run '^$' \
  -bench 'BenchmarkRepeatedJSONField|BenchmarkPreviewLargeString|BenchmarkJSONViewLargeArrayReveal' \
  -benchmem -benchtime=500ms -count=3 ./httpx ./jsonx ./gui
```

证据：`bench-isolated.log`、`bench-isolated.exit`。下表是本机三次结果范围：

| 场景 | 时间 | 分配 |
|---|---|---|
| 旧 Response.Int：每次重新解析小 JSON | 1,030–1,047 ns/op | 1,760 B / 23 allocs |
| 新 Document.Int：复用已解析文档 | 201.1–204.8 ns/op | 64 B / 5 allocs |
| 大字符串受限预览 | 2,510–2,530 ns/op | 1,120 B / 4 allocs |
| 10,000 元素数组：构造控件、设置文档、定位末项 | 34,306–35,180 ns/op | 45,504 B / 308 allocs |

复用查询不含首次解析成本；数组基准也不含 JSON 解析，不含真实像素渲染与输入延迟。这不是 FPS、全应用加速比例或 Windows CPU 结论。

## 明确边界 / 后续工作

- 默认 JSON 上限：4 MiB、深度 128、100,000 节点；大小在解码前检查，深度与节点数在构造只读树时检查。底层仍完整解析有界 JSON，不是流式解析器。
- 重复对象键遵循 Go decoder 的最后值胜出；Int 拒绝小数、指数形式和溢出；Float 仍受 float64 精度限制。不是完整 schema／安全校验器。
- ParseAsync 取消检查在解析前后，不会在单个 JSON token 中间抢占；调用者负责窗口关闭后取消和失效旧回调。
- 范围分组限制每层新建行数量；全部主动展开仍会增加内存／布局成本，不宣称常数内存或完整虚拟树。
- JSONView 是只读查看控件，尚未实现 JSON 编辑、全文搜索、JSONPath、JSON Schema 或网络增量流。
- 设计样例通过 JSONText 持久化；真实响应使用 LoadJSON／SetDocument，不写回设计样例。敏感内容不要写入 JSONText。
- Windows 与 Linux 未进行本轮原生 UI／剪贴板／输入法／渲染实测。Windows pure-Go 构建不等于 Cairo 原生后端验收，也不等于此前 GDI／线程调度问题已解决。
- 下一批可围绕可取消下载与进度、SSE 增量事件查看、表格数据绑定补充完整组件链路；本轮未将这些列为已实现。
