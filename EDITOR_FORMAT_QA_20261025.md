# 编辑器后台格式化 QA（2026-10-25）

## 本轮改动

- 替换 SDK 编辑器 UI 线程等待外部 `gofmt` 的路径，取消临时文件与 PATH 依赖。
  旧路径还使用 `SetText` 清空 Undo；新路径保留历史、单次撤销，不变结果不产生历史。
- 新增无 GUI 依赖的 `formatx` 同步／异步接口、Go 本地格式化、JSON 美化／压缩。
  保留重复键、键顺序、大整数、指数写法与字符串转义，不做 JSON 解码再编码。
- SDK 快捷键／代码面板按钮走后台；IDE Edit 菜单、命令面板和 Ctrl/Cmd+Shift+I 接入。
  Go 有 LSP 时保留既有 gopls 路径；本地取消入口不冒充 gopls 取消。
- 每编辑器一个在途任务，共享两工作任务执行器；取消后直到任务实际返回前不释放槽。
  不新增轮询或持续绘制；UI 队列校验文本版本、文件路径、语言及取消状态，防止迟到结果
  覆盖后续输入，包括编辑→撤销回原文的 ABA 情况。
- 新增 `examples/editor-format`，按钮按 SizeHints 换行；零尺寸／窄窗口子控件不越界。

## 自动验收

本轮产物位于 `bin/qa-artifacts/editor-format-20261025/`（本地忽略目录，不提交二进制）。
使用 Go 1.26.10，回归均 `-count=1`，不是缓存命中。

- 基线反证 `red.log`：PATH 无 gofmt 时旧 `FormatCode` 失败；加入新实现后相同用例通过。
- macOS arm64：15 个目标包 `go test -race -short -json -p 2`，5464 个 test/subtest
  pass、47 skip，0 fail；其中 `internal/regexscan` 无测试文件，包级 skip 单独计。
- Windows amd64 真机同一 15 包 `-race -short`：5348 pass、68 skip，0 fail；
  原始基线 1801 个文件与 215 个覆盖输入均校验哈希，构建前后不变；IDE／SDK 本机构建通过，
  捆绑工具校验和三项真实 gopls 集成测试通过，无 skip。
- macOS `graph/prop/settings`：238 pass、1 skip；示例布局测试通过。
- `go vet -composites=false` 通过；IDE 与示例本机构建、工具包校验通过。
- 实际捆绑 gopls：未保存 Unicode 补全／格式化、跨文件重命名、类型／实现／后退
  三项集成测试全部通过，无 skip。测试二进制位于应用工具目录旁，不借用全局 gopls。
- 新增用例覆盖异步 UI 派发、无修改时幂等、Undo/Redo、语法错误不改正文、并发限额、
  取消时不提前释放任务槽、路径／语言／隐藏／Escape／编辑／ABA 使旧结果失效。
- JSON 资源保护覆盖无效 UTF-8、多值、JSONC、空输入、128 层深度、8 MiB 输入及
  16 MiB 缩进膨胀上限；过大结果拒绝，不静默截断。
- macOS M2 Max 基准（非 GUI 延迟指标）：约 40 KB JSON，541076 ns/op、73.86 MB/s、
  344090 B/op、5 allocs/op。只代表该输入和机器，不是任意文档零卡顿承诺。

## macOS 原生 UI

使用隔离 HOME／数据目录的新构建，通过真实键盘、菜单与按钮操作：

1. JSON 快捷键美化，保存后逐字节核对预期，包含 `中🙂`、重复 `n`、`9007199254740993`、
   `1e+009` 与 `\u0041`；一次 Undo 恢复原文，Redo 恢复美化结果。
2. 命令面板 `Compact JSON`，保存后与原始紧凑文本完全一致；Edit 菜单 Format Document
   再次美化并核对文件。菜单内图标、禁用状态和菜单边界已观察。
3. 非法 JSON 显示错误 Toast，dirty 不改变；保存后原文逐字节不变。
4. SDK 示例使用 PATH `/usr/bin:/bin`，通过 Pretty、Undo、Redo、Load Go／Pretty 按钮
   生成四份实际控件 Snapshot；正文与预期逐字节匹配。正常退出两个测试实例，PID 已消失。

## Windows 10 原生 UI

- 使用 `win10testcomputer` 交互式 Session 1 运行本轮独立构建；不在 SSH Session 0 假测点击。
  干净 PATH 不包含 Go／gofmt／全局 gopls，实际子进程指向应用内 `tools/gopls.exe`。
- 快捷键美化、Undo、Redo、命令面板压缩／美化、无效 JSON 错误提示、继续输入与恢复，
  再加 Edit 菜单压缩／美化，共 10 个保存回读阶段，逐字节核对通过。
  菜单图标、禁用的取消按钮及弹出区域边界已截图检查。
- 空闲采样 10.1406 秒，20 次 Responding 检查全通过，CPU 增量 0.1875 秒，约单核 1.85%。
  此数据不是帧率、输入延迟或所有机器的性能保证。
- SDK 示例 Pretty、Undo、Redo、Compact、Load Go／Pretty 五个阶段均通过真实按钮
  和实际控件 Snapshot 核对，PATH 无 gofmt，离线 Go 格式化正常。
- IDE／SDK 正常退出，专属目录下剩余进程为 0；五个本轮交互任务结果均为 0 后注销。
  完整证据为 `accepted-windows-evidence.tar.gz`，源码输入清单为
  `accepted-source-hashes.json`（1565 个构建／测试输入，提交前重新校验）。

## 边界

- Linux 本轮未做原生桌面测试；macOS／Windows 的有限样例不代表所有控件与 DPI 场景。
- Context 取消是合作式；标准库 Go/JSON 解析器不能被强杀。Go 输出上限在格式化后检查，
  不是峰值内存硬上限；JSON 缩进输出有分配前预估。
- 最终 ReplaceAllText、Undo 快照、词法缓存更新仍在 UI 线程；大文档最终应用并非零成本。
- `FormatCode/FormatDocument` 仍为同步 SDK API，交互调用应使用异步入口。
- 本轮不新增 JSON Schema、JSONC、持续诊断、导入组织或多语言 LSP；不改写既有 gopls
  格式化任务生命周期。宿主销毁／停止 UI 循环前仍须主动 `CancelFormat()`。
