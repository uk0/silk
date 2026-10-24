# 编辑器 LSP 快照与坐标读取回归（2026-10-24）

本轮是继续优化输入链，不是完整 IDE/SDK 验收。保留已有未提交工作，未 commit/push/发布。

## 测量与根因

上一轮将大文件符号解析移出绘制线程后，本轮单独测实际 `updateLSPDocument` 主线程快照路径：用无 worker 的受控文档记录版本/快照/合并唤醒，不运行 RPC 或绘制，也不修改文本。因此以下数据**不是每键延迟、整个 IDE 吞吐或 gopls 请求耗时**。

原路径仅为判断光标前一个字符就 `strings.Split(text, "\n")`，随后 `[]rune(currentLine)`；`CursorUTF16Col` 又复制同一整行。4 MB 单行即使光标只在前 6 个字符之后，也产生约 32 MB 分配。还复现 `utf16ColumnOf` 的负列越界 panic。

macOS M2 Max、各重复两次、每次 1 秒：

| 快照场景 | 修改前 | 修改后 | 每次分配（前 → 后） |
| --- | --- | --- | --- |
| 小文件 | 241.5–242.8 ns | 199.7–199.9 ns | 144 → 96 B |
| 100k 行，短末行 | 1.13–1.39 ms | 202.2–202.9 ns | 约 1.61 MB → 96 B |
| 4 MB 单行，光标在开头 | 8.14–8.21 ms | 201.0–201.3 ns | 约 32.01 MB → 96 B |
| 4 MB 单行，光标在末尾 | 10.45–10.56 ms | 5.34–5.35 ms | 约 32.01 MB → 约 98 B |

文档尾部仍需遍历当前行的前缀，不承诺任意位置 O(1)。剩余固定分配也没有被隐藏成“整个 LSP 零分配”。

## 实现

- 新增 SDK `LineText`、`TextPositionAt`、`CursorTextPosition` 与 `EditorTextPosition`：一次前缀遍历返回 rune/UTF-8 byte/UTF-16 三种列坐标和前一个码点，不复制全文/整行、不修改光标或发布修订。
- 有效空行与非法行由 bool 区分；负列和超长列安全夹取。中文、非 BMP、组合字符、NUL、无效 UTF-8 字节的读取语义有单测。
- 原有 `CursorUTF16Col` / `UTF16ColumnAt` 共用新转换逻辑，避免全行 rune 数组并修复负列 panic。
- LSP 自动补全触发读取合并坐标；通知文本与编辑器内容不一致时仍同步全文，但不混用不同快照的光标发补全请求。版本递增、后台 wire 顺序和 coalescing 不变。
- 无 `textEdit` 的补全 fallback 按 byte offset 反向检查标识符前缀，保留 Unicode/UTF-16 正确性，避免切分整个文件。过期光标或非法行不会 panic 或误替换；主编辑及附加编辑仍原子应用、可一次撤销。
- SDK 接口与复杂度说明：`gui/CODE_EDITOR_POSITIONS.md`。

## 单测与证据

- 原实现负例：`negative-before.log` 同时记录整行分配和负列 panic；修改后目标测试通过。
- macOS `go test -race -short -json -count=1 ./gui ./ged ./cmd/silkide`：**3,949 pass / 41 skip / 0 fail**（最终加入定向通知修复后重跑；此前为 3,942 pass）（包含子测试）。三包 `go vet -composites=false` 通过。
- `BenchmarkLSPInputSnapshot` 直接调用正式入口；不是另写一个更快的替代实现。通知快照内容、坐标、完成策略、版本和唤醒合并有断言。
- 本地：`bin/qa-artifacts/editor-lsp-position-20261024/`。
- Windows：`C:\silk\qa-editor-lsp-position-20261024-codex`；1,801 项冻结源文件及 最终 23 项 Go overlay 逐一验证 SHA-256，RequireBundle=1 构建。

## 新发现但尚未修复的问题

在扩展坐标测试时，发现既有 rune-based 编辑会把畸形 UTF-8 字节规范化为 U+FFFD，撤销不能恢复原始字节：`61 ff 62 → 输入空格并撤销 → 61 ef bf bd 62`。已经用隔离 overlay 在**本轮修改前后**分别复现，相同失败，确认不是新坐标 API 引入。

`invalid-utf8-final-before.log`、`invalid-utf8-final-after.log` 和最小 probe 保留在证据目录。正式 API 测试对无效字节验证只读坐标，对编辑/撤销使用有效 UTF-8，未把这个既有缺陷伪装成通过。下一步需要字节边界安全的输入/撤销路径，同时减少整行 rune 复制。同步 gofmt/保存、整篇文本重建、长行 popup 锚点仍有独立开销，当前不能宣称无卡顿或无数据保真问题。

## 真实截图发现的大纲刷新缺口

首轮 Windows UI 中，大文件 breadcrumb 已显示函数名，而右侧仍是 `Indexing symbols...`，直到下一次编辑/保存才恢复。这证明模型缓存已完成，但单独缓存绘制的大纲未被置脏；仅让 `SymbolsRevision` 递增再重绘 editor 不足以通知 sibling panel。

新增 `SubscribeSymbolsChanged` 多消费者订阅：文本/provider/语言变更及符号结果发布后，主线程向消费者发短通知；大纲对自身 `Update()`，不添加 timer、不强制全窗口刷新。切换编辑器/解绑自动取消旧订阅；回调中的解绑、再次订阅有安全快照语义。通知在可读状态发布后执行，消费者不在回调中同步重解析文本。

负例 `TestOutlineProviderInvalidatesItsOwnPaintRegion` 在旧代码得到 **0 次大纲失效**，修复后为 1；另测新旧编辑器切换、无重复订阅、自解绑、异步接受、同步抢先完成及过期结果。初次 probe 有一次测试类型编译错误，单独归档后修正，未当作产品负例。

首轮（无通知修复）源码、构建、单测、UI 和基准独立保留在 `before-notify-*`；最终 23 项 overlay 已重新跑单测、真实 UI，不沿用首轮二进制的通过结论。

## 最终 Windows 真机回归

- Go 1.26.10 / UCRT64，三包 `go test -race -short -json -count=1 -p 2`：**3,837 pass / 62 skip / 0 fail**（包含子测试），退出码 0。最终构建和测试全部使用已验证的 23 项 overlay。
- Session 1、全新应用设置和 fixture 工作区。实际进程确认为最终二进制；实际子进程为 `apps-final\tools\gopls.exe`，日志记录 v0.24.0 (bundled)、ready 和 workspace 完成加载。没有安装全局 gopls 或修改系统 PATH。
- 普通文件和约 4 MB / 100,006 行文件均完成：含中文/emoji 的行尾 Ctrl+Space → 选择跨文件变量 → 全文缓冲区精确比较 → Ctrl+S 全文磁盘回读 → 一次 Ctrl+Z 恢复原文并保存 → 一次 Ctrl+Y 恢复补全并保存 → F12 跨文件定义 → Alt+Left 返回。
- 最终截图实际显示 gopls footer、`远程变量完成值` 候选；不是只看到本地 fallback 就算成功。大文件补全弹层向上展开，未覆盖底部状态栏。
- `large-initial-idle-outline.png` 在打开大文件后等待 3 秒、未再导航/点击/编辑时截图，右侧已经显示 `large_unicodeProbe`，不再停留在 `Indexing symbols...`。补全、保存及定义跳转截图也显示正确大纲。
- 空闲采样 10.175 秒：IDE CPU 时间 0.21875 秒（约 **2.15% 单核**），gopls 0.265625 秒（约 **2.61% 单核**），20/20 次 Responding=true。这是一次短时间采样，不是输入延迟、FPS 或长期稳定性认证。
- 原生关闭按钮正常退出：IDE PID 9668、gopls PID 6408 均回收；按本轮路径枚举剩余进程为 0。仅移除本轮 4 个已结束的计划任务，保留其他应用和证据目录。

Windows 同机快照微基准（各 2 次、1 秒；不是键入/绘制/RPC 总延迟）：

| 场景 | 原路径 | 最终路径 | 每次分配（前 → 后） |
| --- | --- | --- | --- |
| 小文件 | 1.21–1.56 μs | 1.26–1.32 μs | 144 → 96 B |
| 100k 行、短末行 | 6.08–6.36 ms | 1.20–1.32 μs | 1,605,730 → 96 B |
| 4 MB 单行、开头 | 27.85–28.64 ms | 1.31–1.42 μs | 约 32.01 MB → 96 B |
| 4 MB 单行、末尾 | 36.90–40.07 ms | 21.35–39.22 ms | 约 32.01 MB → 102–110 B |

原路径和第一版坐标修复的对照日志单独保留于 `before-notify-final-evidence/`；加入大纲通知后，最终版本又重跑了相同基准。原/最终不是同时交替采样，长行末尾波动明显，不把单次最快值当成稳定倍数。长行末尾仍需扫描前缀，几十毫秒的同步工作仍值得继续处理。

## 交付证据与覆盖边界

- [最终 Windows 截图](screenshots/silkide-lsp-position-windows-20261024.png)。
- `bin/qa-artifacts/editor-lsp-position-20261024/summary.json` 汇总原始测试计数、跳过项、三阶段 Windows 基准、UI 结果和进程身份。
- 最终 Windows IDE SHA-256：`99c600f25872f742adb23e42ad529df78fecbc01aedf173428d18bcbb1312f1c`。
- `windows-final-evidence.tar.gz` 下载前后 SHA-256 相同：`9ab30d3270803fb6c3171245bf51105a75ec5c08605b6b8122a3bc203336d395`。含原/最终测试和 UI 证据，未用首轮旧二进制代替最终版本。
- 本地再次校验：23 项 overlay 与当前源码完全一致；冻结基线其余文件仅 README / 两份 SDK 文档有文字差异，不影响已测可执行输入。
- 跳过项不算通过；UI 日志的 Git 缺失警告仍保留，未把 Git 当作本轮已测功能。微基准宿主日志中临时目录 icon 缺失与 core.Close 警告不等于真实 UI 崩溃；真实应用正常关停有独立证据。
- 本轮没有重跑 Linux 桌面、macOS 原生 UI、真实输入法组合态、混合 DPI、所有控件/菜单。未 commit、push 或发布。上述畸形 UTF-8 编辑/撤销问题仍未解决。


## 后续：字节保真与多光标事务

上文结论是当时的验收快照。畸形 UTF-8 输入/撤销、最后一行撤销崩溃、snippet 字节锚点、多光标历史/多行操作，以及 Windows 系统键和异常字节绘制的后续修复，见 [字节编辑回归记录](EDITOR_BYTE_EDITS_QA_20261024.md)。新记录保留首次失败、修复后的源码清单与独立原生验证，不追改旧测试计数。
