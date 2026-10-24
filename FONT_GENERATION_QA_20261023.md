# 2026-10-23：原生字体回退与设计器生成安全

基线：`1dd5c05`。本轮未 commit / push，未修改 Go 版本或增加依赖。
环境：macOS arm64 / Apple M2 Max / `go1.26.10`。

## 修复与边界

### 1. macOS 中文／emoji

原生 Cairo 分支原来只在 Windows 启用缺字回退；macOS 的 Menlo 直接走无回退分支。
现在共享已存在的 run 分段、覆盖检测与有界缓存，分别保留 Windows／macOS 的系统字体候选表。
中文使用实际有覆盖的字体，emoji 使用有覆盖的 Apple Color Emoji；并不替换 ASCII 或图标字体的私有区字符。
测量、光标、命中测试与绘制使用相同 run advances，保留字号、粗斜体、DPI 及绘制后的原字体状态。

这仍是简单 LTR 字体回退，不是完整 shaping／bidi 实现。Cairo 的 toy text 本身不承诺自动字体回退，
见 [Cairo text API](https://www.cairographics.org/manual/cairo-text.html)。
本机没有覆盖 U+20000 的可用候选字体，该专项测试明确跳过；不能以 LastResort 方框冒充成功。
Linux、pure-Go 等其他后端没有因此获得同等回退能力。

### 2. 拆分导出不再“失败前先写坏一半”

`TestGenerateCodeFileInvalidUserKeepsMachine` 在修复前失败：用户 `.go` 语法错误时，
旧流程已经覆盖了 `.silk.go`。现流程先校验控件映射、两份源码、package 和目标文件归属，之后才准备替换。

- 通过 `GenerateCodeIndexed.Err` 拒绝降级／缺失工厂的输出，而不是忽略生成错误。
- `internal/filetxn` 同目录暂存、内容冲突检查、普通写入失败回滚；回滚遇外部编辑时不覆盖它，并保留已有文件的恢复副本。
- 拒绝符号链接、目录与不属于本生成器的输出；单文件导出的内嵌回调也不能误当作拆分机器文件而删除。
- 保留用户正文和 import；缺少新 stub 所需 import 时继续通过 `MissingImports` 明确返回。
- 内容没变化不重写，已有权限保留；接受 LF／CRLF 头，不重排 CRLF 用户源。
- 类型识别使用 `ged.SplitCodeType`；未指定类型时沿用已有类型，显式不兼容类型变更报错，避免标题改动破坏方法接收器。

**不是跨文件崩溃原子事务，也不是跨进程锁。** 多文件之间仍有可观察的替换间隔，
某些平台／文件系统的 rename 不保证原子，外部进程在检查与 rename 间写入仍有竞态窗口。
调用方应保持串行保存；不把 race 测试通过解释成任意并发调用 GUI／Painter 安全。

### 3. IDE 保存保留已有生成路线

以前自动保存固定输出 `decl.Node`，可能覆盖 SDK 导出的同名 `.silk.go`。
现在识别既有拆分机器文件，沿用其 UI 类型并调用 SDK 导出；用户文件缺失、语法错误、未知工厂、归属不明会报告问题。
新增 stub 缺 import 的提示明确注明“文件已保存，需要补 import”，不伪装成编译成功。

端到端回归：设计文件保存 → IDE companion generation → 独立 Go 编译／执行 →
`NewStableUI()` → `Action().Trigger()` → 用户编辑后的 handler → Label 文本改变。
同时验证改标题、控件文字与 CRLF 机器文件，用户方法正文保持原样。

新设计仍使用既有 `Build<Name>() *decl.Node` API；没有自动迁移，也没有承诺默认预览执行内嵌事件。
强类型导出后，外部用户 `.go` 是已有方法正文的所有者；后续内嵌代码编辑不会覆盖它。
从声明式生成器迁移须先备份移走旧机器文件，或显式选择新路径。

## 验证

| 检查 | 结果 |
| --- | --- |
| 14 个明确列出的包，非 short 测试 | 5,065 pass / 14 skip / 0 fail（含子测试） |
| 8 个包 `-race -short` | 4,066 pass / 43 skip / 0 fail（含子测试）；无 race 报告 |
| macOS 严格字体门禁 | 常用中文／emoji、1 / 1.5 / 2 倍测量与像素比较、缓存边界、编辑器 caret／IME rect／命中测试通过 |
| 生成代码完整链路 | 保留用户回调并实际执行；默认 tree-builder 与已有 SDK 生成／编译回归通过 |
| 文件写入故障注入 | 预检失败不写、第二次 rename 失败回滚、并发编辑保留、恢复副本、缺失路径、重复路径、symlink、权限和 no-op 通过 |
| 原生 macOS 构建 | `silkide` 与 `examples/http-tool` 通过 |
| Windows pure-Go IDE + Windows/Linux filetxn 测试二进制 | 交叉构建通过，未在目标平台执行 |
| Windows native Cairo 交叉编译尝试 | `CGO_ENABLED=0` 缺 Cairo `Surface` 等定义，失败；未声称 Windows native 分支编译／运行通过 |
| 完整默认 `go vet` | 未通过：416 条 unkeyed composite literals 告警；逐条源行均存在于基线 `1dd5c05` |
| `go vet -composites=false`（同范围） | 通过；这不等于默认 vet 全绿 |
| 格式／差异 | 本轮 Go 文件 gofmt 干净，`git diff --check` 通过 |

14 个包：`internal/filetxn`、`paint`、`gui`、`ged`、`cmd/silkide`、`async`、`httpx`、`decl`、
`internal/silkgen`、`core`、`graph`、`prop`、`scada`、`examples/http-tool`。
Race 范围是前 8 个包。没有运行仓库根 `go test ./...`，没有扫描或测试 `local/`。

Skip 包含 opt-in GL transfer、真实 gopls／dlv、平台限定、fixture 输出及罕见字字体缺失；
short race 另外跳过部分外部编译测试，这些生成器编译测试在非 short 测试中单独执行。
本轮没有重新验收真实 LSP、输入法候选窗、Windows 或 Linux 原生桌面。

### 实窗操作

使用隔离 HOME 的 `Silk Font QA.app`（本轮构建的 `examples/http-tool`），通过 Computer Use：

1. GET 示例自己启动的 `http://127.0.0.1:60610/json`，类型 json，Pointer `/data`。
2. 观察 `HTTP 200 - json result`，JSON 中 `Silk 中文😀` 可读并显示彩色 emoji。
3. Pointer 改 `/data/name`，类型选 str，再发请求，观察 `HTTP 200 - str result` 与 `Silk 中文😀`。
4. Cmd+Q 退出；通过进程查询确认测试进程已结束。

端口由每次启动随机分配，复现时采用窗口展示的 URL。
截图已在会话中检查，未保存为仓库图片文件。没有触碰用户常用 IDE 偏好或向外部服务器发送请求。
自动化粘贴期间日志出现一次 `event loop resumed after 9.229s`；未定位归因，
因此本轮不声称已解决输入延迟或 Windows 高 CPU 问题。

### 微基准（不是 UI FPS／端到端输入延迟）

本机 3 次 `TextExtents`：暖缓存混合文字约 **2.47–2.54 μs/op，544 B/op，10 allocs/op**；
ASCII 约 **1.54–1.59 μs/op，48 B/op，1 alloc/op**。这些是当前实现的测量值，未做旧版本对照，
不声明百分比提速；ASCII 绘制仍保留原 glyph-buffer 快速路径。

## 复现与证据

```sh
go test -count=1 -p 2 ./internal/filetxn ./paint ./gui ./ged ./cmd/silkide \
  ./async ./httpx ./decl ./internal/silkgen ./core ./graph ./prop ./scada ./examples/http-tool
go test -race -short -count=1 -p 2 ./internal/filetxn ./paint ./gui ./ged ./cmd/silkide ./async ./httpx ./decl
SILK_TEST_FONT_FALLBACK=1 go test -count=1 -v ./paint ./gui \
  -run 'TestDarwin.*Fallback|TestDarwinSupplementary'
go test -run '^$' -bench BenchmarkDarwinTextFallback -benchmem -count=3 ./paint
go vet -composites=false ./internal/filetxn ./paint ./gui ./ged ./cmd/silkide ./async ./httpx ./decl ./internal/silkgen
```

本地证据目录（ignored）：`bin/qa-artifacts/font-generation-20261023/`。
重点文件：`export-before.log`（失败复现）、`packages-final.jsonl`、`race.jsonl`、`fonts.log`、
`font-bench.log`、`native-http.log`、`vet.log`、`vet-baseline.json`、`windows-native-compile.log`、
`cross.log`、相应 `.exit` 文件及 `manifest.json`。初期测试断言参数名写错的记录保留，最终统计以 final 日志为准。
