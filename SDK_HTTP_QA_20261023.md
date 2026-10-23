# SDK 同步／异步 HTTP 与可视请求控件验收 — 2026-10-23

前置：上一轮 52 文件改动提交为 `1489464`，已推送 `origin/main`。
本轮在该提交上新增 SDK 小工具组装能力；未发布版本或安装包。
接口清单、使用代码、转换契约见 [SDK_FEATURES](docs/SDK_FEATURES.md)。

## 本轮内容

- 独立 `async` 包：同步 Run、限并发 Executor、泛型 Future、取消／等待／关闭、显式完成 dispatcher。
  满载不创建等待 goroutine；work panic 和 Goexit 都会退休执行槽位。没有额外 watcher 或空闲轮询。
- 独立 `httpx` 包：Do/Get/PostJSON 的同步／异步入口，默认 30 秒／4 MiB，HTTP 状态错误与响应分开，
  严格 JSON 字段提取、int64 精度、有限 float64、bool，Header/Body 防御性复制、正常 TLS 验证。
- `gui.RunAsync` + `gui.HTTPButton`：后台请求和解析、主线程结果信号、忙碌／取消、过期投递防护、关闭／直接重挂载清理。
- 设计器 palette/drop、属性持久化、预览构造、6 个事件、强类型生成以及 `decl.HTTPButton` 快速组装入口。
- 双向 HTTP 箭头图标遵循现有亮／暗主题 Accent/Success 色，使用设备 DPI 光栅化；不引入低清位图。
- 可运行 `examples/http-tool`：只启动 loopback 示例服务，提供方法／类型／字段选取和请求结果展示。
  组件在 0×0、90×80、440×360、780×520、1200×800 自动边界测试中不超父容器。

## 自动验证

macOS arm64，Go 1.26.10。证据目录 `bin/qa-artifacts/http-sdk-20261023/` 被 Git 忽略。

| 验证 | 结果 |
| --- | --- |
| 14 包非缓存 short 测试 | 通过：packages.log / packages.exit=0 |
| async/httpx/gui/ged/paint/decl/HTTP 示例 7 包 race | 通过：focused-race.log / focused-race.exit=0 |
| 14 包 vet，`-composites=false` | 通过：vet.log / vet.exit=0 |
| async/httpx、HTTP 控件与 UI 派发各 10 次 race 重复回归 | 通过：repeat-sdk / repeat-ui 的 log 与 exit=0 |
| 非 short 设计器 → 全事件／属性代码生成 → consumer 编译 | 通过：generated.log / generated.exit=0 |
| macOS IDE 和 HTTP 示例构建 | 通过 |
| Windows amd64 IDE 和 HTTP 示例 pure-Go 交叉构建 | 通过；不是原生 GUI 验收或 Cairo 发布构建 |
| Linux amd64 async/httpx 测试二进制交叉编译 | 通过；不是原生执行，也不是 Linux GUI 构建 |

复现命令：

```sh
go test -short -count=1 ./async ./httpx ./core ./gui ./ged ./paint ./cmd/silkide \
  . ./graph ./decl ./prop ./settings ./internal/silkgen ./examples/http-tool
go test -race -short -count=1 ./async ./httpx ./gui ./ged ./paint ./decl ./examples/http-tool
go vet -composites=false ./async ./httpx ./core ./gui ./ged ./paint ./cmd/silkide \
  . ./graph ./decl ./prop ./settings ./internal/silkgen ./examples/http-tool
go test -count=1 -run '^TestHTTPWidgetPaletteDropPersistencePreviewAndGeneration$' -v ./ged
go build -o bin/qa-artifacts/http-sdk-20261023/http-tool ./examples/http-tool
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags silk_pure_go \
  -o bin/qa-artifacts/http-sdk-20261023/silkide-windows ./cmd/silkide
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags silk_pure_go \
  -o bin/qa-artifacts/http-sdk-20261023/http-tool-windows ./examples/http-tool
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c \
  -o bin/qa-artifacts/http-sdk-20261023/async-linux.test ./async
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c \
  -o bin/qa-artifacts/http-sdk-20261023/httpx-linux.test ./httpx
```

测试覆盖：真实 httptest HTTP/TLS 服务、POST 编码、运行时 header 覆盖、非 2xx 错误正文、
声明长度／chunked／gzip 解压后的响应大小上限、取消／超时、自签证书拒绝、定制可信 TLS client、
RFC 6901 转义／数组／缺失路径、拼接 JSON 拒绝、整数溢出／小数／类型拒绝、完整 int64、
异步请求快照、防止后台触碰 GUI、派发已排队结果后关闭、取消后新请求不被旧结果覆盖、
属性不持久化运行时凭据、全事件签名及导出编译、主题色与透明图标边角。

## macOS Computer Use 实测

使用隔离 HOME 的 `Silk HTTP Tool QA.app`，不接触用户外部 API 或凭据。
最终桌面实例的 loopback 端口为 64872；重启时端口会变化，以 URL 输入框为准。

1. 初次截图发现按钮有图标却没有文字：基类有图标时默认隐藏文本。
   HTTPButton 构造器显式 `SetTextVisible(true)`，添加回归断言，重建后确认 Send/Cancel 文本均可见。
2. GET `/json`，json + `/data`：显示选中对象，状态 HTTP 200。
3. int + `/data/count`：实际显示 `9007199254740993`，没有舍入成 `9007199254740992`。
4. float + `/data/ratio`：显示 `1.25`。
5. URL 改 `/error`：显示 `httpx: HTTP 422 Unprocessable Entity`，状态为失败，不发送成功值。
6. `/slow` 时出现 Cancel request／忙碌状态；取消后恢复 Send request，结果为 `context canceled`。
7. 通过方法菜单选 POST，str + `/data/method`：实际显示 `POST`。
8. bool + `/data/ready`：实际显示 `true`。
9. 缩窄窗口后重新截图：输入、方法／类型下拉、按钮、结果区仍在父容器内，无按钮相互覆盖。
10. Cmd+Q 退出，进程检查确认最终 QA 实例不再运行。

截图在 Computer Use 会话中检查，未保存额外 PNG。菜单选项与 HTTP 响应的可见结果构成原生 UI 证据；
不能把它说成 Windows 或 Linux 原生结果。纯 JSON 对象的首轮 UI 路径先于最后的按钮标签修复；
最终包的整数、浮点、POST 字符串、布尔、错误和取消均重新实际操作。

## 保留的边界

- IDE 默认 `.silk.go` 树生成与 `ged` 强类型事件导出仍是两条路线；本轮不宣称默认 IDE 预览执行内嵌 Go 回调。
  原生 HTTP 示例通过正常 SDK 构造与信号连接运行，不冒充设计器默认生成路线的端到端证明。
- 首次 JSON 结果截图暴露现有 macOS 代码编辑器默认字体的中文／emoji 缺字。
  网络、解析、持久化测试证明 Unicode 值保留，但字形显示问题尚未修复，不能将其标记为视觉通过。
- QA app 是最小开发包，icon 资源目录警告由程序化图标回退接管；实测 HTTP 图标和文本可见。
  日志仍有事件循环长间隔警告，未做 CPU/FPS 或输入延迟基准，不能声称性能问题全部解决。
- 通用 Future 回调可能在 Cancel 后已经排队；宿主必须管理生命周期，HTTPButton 内置 generation 防护。
  祖先容器销毁时应显式 Close/取消 Context；不使用 GC 代替取消。
- 不含文件下载、流式响应、SSE/WebSocket、OAuth 向导、代理配置 UI、自动重试、可视化信号连线或新的 LSP 功能。
- 初次开发中的测试控件命名 setter 和示例不存在的只读 API 编译错误已修正；只采用最终成功结果。
