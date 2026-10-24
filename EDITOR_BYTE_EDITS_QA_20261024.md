# 编辑器字节保真与多光标事务回归（2026-10-24）

本轮继续编辑器/SDK 的输入与撤销链优化，不是整个 IDE 的最终验收。保留已有未提交修改，未 commit、push 或发布。

## 根因与修复

上一轮最小复现 `a ff b → 输入空格 → Undo` 会得到 `a ef bf bd b`。本轮扩展反例确认，普通输入、删除、换行、选区、粘贴和替换存在多条独立的 `[]rune(line) → string` 写回路径，既规范化无效 UTF-8，也复制整行；撤销只记 rune 数量，还会在畸形字节跨编辑边界组成合法字符时删除错误范围。

- 新增统一 `replaceByteRange` 存储原语：按原始字节切片拼接，保留未改部分；跨行、跨 LF、原始无效字节都不经过 rune 字符串写回。存储修改与发布修订分离，消费者不会看到选区“先删空、再插入”的中间态。
- 单光标 undo/redo 记录修改前的 byte column 和精确的插入/删除字节。文字列仍以 rune 表示；拼接形成新合法码点时，光标向该码点末尾归一化。小段删除内容复制成独立字符串，避免一字节历史记录长期引用整篇大文件。
- 普通输入、自动配对、选区包围、多行输入/粘贴、退格/Delete/Enter 和查找替换共用字节原语。单次 Replace 现在也有原子撤销，忽略大小写的 Replace All 保留未匹配字节，无匹配时原样返回。
- 修复删除最后一行后的撤销越界崩溃，以及复制最后一行后撤销残留空行：历史记录正确包含前面的 LF，而非不存在的末尾 LF。
- 修复撤销分支后新输入错误并入更早一次输入；不降低已有的连续输入合并策略。
- 发现并修复 snippet 的旧 `byteOffsetForCursor`：`a ff b` 的 rune 列 2 曾得到 byte offset 4，而非 2。改用原始前缀字节坐标，同时把超出行范围夹取到真实 EOF；默认 `iferr` 展开及一次撤销/重做有包含无效前缀的用例。

## 新增多光标完整事务

旧实现输入只记录主光标的撤销，Backspace/Delete 没有记录历史，多行输入只改主光标。

现在：

- 单行/多行输入、粘贴、退格、Delete、Enter 对所有光标执行；Enter 分别计算每个光标的缩进。
- 先在原始模型中计算全部 byte anchor，再从后向前修改；撤销反向重放，重做原向重放。行合并和畸形字节拼接不会让后续操作误用已变化的 rune 列。
- 每次操作是一个 undo 单元，恢复主光标、附加光标的位置；重复/越界光标先归一化并去重，纯 no-op 不写入历史。
- 记录的是编辑范围列表，不为每个按键保存两份完整文档。行偏移表、光标列表、实际行拼接仍有分配，不宣称多光标编辑零分配。
- 本轮仍是多 **光标**，不是每个光标各有独立 selection 的完整多选区模型。

## 本地测试与性能

- 原实现行为负例保留于 `regressions-before.log`；覆盖字节规范化、选区读取、替换、撤销和最后一行崩溃。初次测试用了两个不存在的方法名，编译错误单独保存为 `probe-setup-error*.log`，修正后才记录产品反例。
- `multicursor-before.log` 记录多光标字节损失、缺失事务及多行输入遗漏；修改后定向测试通过。
- `snippet-offset-known-failure.log` 的 4 vs 2 偏移反例已修复，最终 probe 单独重跑。
- 字节原语的穷举参考测试遍历多个有效/无效文本的所有 byte span，对照直接字节切片，并逆向重放。覆盖 LF、空行、NUL、中文/emoji、无效前导/续字节以及合法化拼接。
- Go fuzz 在最终版本实际执行 **227,899** 次，约 20 秒、2 workers，通过（此前另一次 244,252 次也通过，原始日志独立保留）。范围是字节拼接和逆操作，不代表完整 IDE fuzz。
- 后续加入绘制/系统按键修复后，最终 macOS 四包 `go test -race -short -json -count=1 -p 2 ./paint ./gui ./ged ./cmd/silkide`：**4,216 pass / 43 skip / 0 fail**；四包 `go vet -composites=false` 退出 0。此前三包 4,000 / 41 的日志单独归档，未与新结果相加。最终 Windows 同四包 race：**4,106 pass / 63 skip / 0 fail**，随后原生构建成功。统计含顶层用例/子用例；未与失败阶段或额外集成测试相加。真实 UI 的 34 个字节阶段及随包 gopls 流程已通过，输入路线限制见后文。

macOS M2 Max，同一真实 `insertTextAtCursor` 原语，各重复两次、每次 1 秒；每次重置原始行，不包含绘制、LSP、整篇 rebuild 或磁盘保存：

| 场景 | 原路径 | 字节路径 | 分配（前 → 后） |
| --- | --- | --- | --- |
| 短行开头 | 483–492 ns | 55.5–55.7 ns | 656 → 80 B |
| 短行末尾 | 481–485 ns | 190–191 ns | 656 → 80 B |
| 4 MB 长行开头 | 21.83–21.93 ms | 145–146 μs | 约 37.77 → 4.20 MB |
| 4 MB 长行末尾 | 21.81–21.85 ms | 8.66–8.67 ms | 约 37.77 → 4.20 MB |

原语的分配次数由 3 降至 1，仍须分配结果字符串。长行末尾的前缀扫描并未消失，不能把上表当作完整每键延迟。

Windows Xeon E5-2686 v4、GOMAXPROCS=2，最终冻结版本同样各重复两次：

| 场景 | 原路径 | 最终路径 | 分配（前 → 后） |
| --- | --- | --- | --- |
| 短行开头 | 2,393–2,449 ns | 355–365 ns | 656 → 80 B |
| 短行末尾 | 1,683–4,848 ns | 1,201–1,218 ns | 656 → 80 B |
| 4 MB 长行开头 | 98.76–98.84 ms | 1.13–1.58 ms | 37,773,312 → 4,202,496 B |
| 4 MB 长行末尾 | 100.81–100.88 ms | 45.86–54.24 ms | 37,773,312 → 4,202,496 B |

分配次数同样 3 → 1。前后非交替采样，短行末尾波动明显；长行末尾仍有几十毫秒成本，不能宣称编辑器性能已经优化到底。

## Windows 首次失败与处理

首次最终构建门禁被 race 测试挡住，未继续启动 UI。唯一失败是既有 `TestRuneColumnFromUTF16` 的全进程分配计数（71 次），该纯转换函数本轮没有修改，真机单独重复 5 次全部通过。`testing.AllocsPerRun` 使用进程级分配计数，不能隔离 GUI 套件内的异步 worker。

没有放宽零分配要求，也没有跳过该测试：把分配断言放到同一份测试二进制的新进程，仅执行该测量；父进程使用 `-race` 时子进程仍是同一个 race 二进制。数值语义子测试保持不变。随后重新冻结源码并完整重跑三包；首次失败日志和 26 项旧 overlay 单独保留。

## 随包 gopls 独立回归

本轮继续使用固定版本的随包工具，不安装全局 gopls、不修改持久化 PATH。`scripts/test-bundled-gopls.sh bin/SilkIDE.app/Contents/MacOS` 用当前源代码构建测试二进制，并在真实 app-relative 工具路径执行：macOS 的未保存 Unicode 补全/格式化、重命名后定义查找、类型/实现/定义及返回 **3 pass / 0 skip**。另外工具解析/打包两包 race **14 pass / 0 skip**。这是协议/路径集成，不是本轮 macOS 桌面 UI 测试。

Windows 集成启动脚本还修正了两个测试适配错误：PowerShell 5.1 的点号参数必须作为完整字符串传入；native stderr 警告不能被 `$ErrorActionPreference=Stop` 误判为失败。改为 `Start-Process` 独立捕获 stdout/stderr，并断言进程 exit code、恰好 3 个 PASS、没有 SKIP，保留两次 setup error。Windows 当前源码编译的 race 测试二进制，强制随包 gopls **3 pass / 0 skip / 0 fail**；工具解析/打包两包 race **14 pass / 0 skip / 0 fail**。最终桌面进程的语言服务器路径为 `apps-final\tools\gopls.exe`，版本 v0.24.0；标准 `-short` 套件中的相应 3 个 skip 仍保留，不将跳过改写为通过。

## 原生 UI 暴露的第二组问题

三包 Windows race 一度通过（3,888 pass / 62 skip），但不能据此宣布 UI 通过。实机首轮前 15 个单光标字节保存阶段正确，随后 `Ctrl+Alt+Down` 创建多光标失败，实际文件只修改第一行；截图还显示含畸形 UTF-8 的两行完全空白。该轮二进制、源码、失败输入、截图、正常退出记录独立归档，未覆盖失败证据。

- Windows 的 `WM_SYSKEYDOWN` 只尝试应用快捷键，然后直接交给 `DefWindowProc`，没有控件入口。增加可选择消费的 `IEventSystemKeyDown`；编辑器仅接管既有 Ctrl+Alt+Up/Down。保持未处理键原生行为，不用全量转发 Alt 键掩盖问题。原生 SendMessage 路由测试同时覆盖初次按下与 repeat；只回退窗口路由的负对照必须先失败。第一版 spy 仅替换窗口 root、没有替换逻辑 focus，导致新旧路由都绕过 spy：该测试适配错误及 4,105 pass / 63 skip / 1 fail 原始日志已单独归档。随后同时设置/断言 spy 的接收焦点，再跑负对照、定向正例和完整套件；不把该错误记为新的产品缺陷。
- 原始无效 UTF-8 传给 Cairo 后，字形转换会失败，部分测量/回退调用甚至留下错误字体/绘制上下文；NUL 则在 C 字符串边界截断余下文本。统一在 SDK 的绘制、测量和字形转换入口生成临时 display string，每个无效字节对应一个 U+FFFD，NUL 对应 U+2400；不修改文档。没有使用会折叠连续坏字节的 `strings.ToValidUTF8`。
- 增加原始/显示文本的 glyph、extents、逐像素结果等价测试，且在异常行后继续绘制 ASCII，检查两行都有实际像素、Cairo context 仍成功。负例日志 `display-before.log` 保留空行、NUL 截断及 `input string not valid UTF-8`。最初误用不存在的测试画刷构造函数已单独归档为 setup error，不算产品缺陷。
- 清理复用字形缓冲时，空文本不再保留调用方传入的旧 glyph 长度。有效字符串直接返回，新增显示转换零分配；原生字形/测量本身不是零分配。
- 模型字节测试不再使用会异步刷新 Git 的 `SetFilePath`，避免无关 worker 干扰套件；进程隔离的 UTF-16 零分配断言仍保留。

新增显示/空字形缓冲/系统键谓词在 macOS `silk_pure_go` 后端定向回归也通过；这不替代 Linux 桌面验证。

### 组合键的运行时归因修正

窗口系统键接口的独立负例已修复，但在最终生产二进制上，原 Ctrl+Alt+Down 仍未创建多光标，不能据此认定原 UI 问题解决。新增临时诊断 **overlay**（只在 `bin/qa-artifacts`，不修改生产源码）记录消息、修饰键、焦点及操作前后光标：

- Ctrl+Alt+Down 仅收到 Ctrl/Alt 的 `WM_KEYDOWN`，**没有收到 Down**，普通字符和其他方向键均正常。
- 只增加 Shift 后，收到 `WM_KEYDOWN key=0x28 ctrl=true alt=true shift=true`，`extras=[] → [{1 4}]`，输入 `!` 后两行落盘均正确。
- 因此这是当前机器/输入链的应用前投递限制；未定位具体系统热键、驱动或注入器，不把缺失消息归因为编辑器内部处理。原 Ctrl+Alt+Down 在本机仍不算通过，也没有改写为已解决。
- 正常关闭诊断版本及 gopls 后，使用同一份已通过 race 的 **无诊断补丁生产测试二进制**、新设置目录、重置后的原始 fixture，选择本就支持的 Ctrl+Alt+Shift+Down 完整重放；输入路线写入 `cursor-input-route.json`。诊断截图不充当最终验收截图。

macOS 原生测量微基准两次对照：ASCII 1,513 ns → 1,524–1,534 ns，中文热缓存 2,303 ns → 2,335–2,344 ns；分配分别仍为 48 B / 1 次和 544 B / 10 次。这里显示检查有微小扫描开销，不宣称性能无成本，也不是完整 UI 延迟。

## 最终 Windows 原生 UI

最终可执行输入无诊断补丁；新设置目录、重置原始 fixture 后完整重放，非接着失败缓冲区继续：

- **34/34** 阶段逐次 Ctrl+S 并 SHA-256 对照原始文件字节。覆盖输入/Undo/Redo、删除坏字节、跨编辑缝形成合法 UTF-8 及逆操作、选区多行粘贴，多光标输入/退格/多行粘贴/Enter/Delete 及各自 Undo/Redo。多光标创建使用 Ctrl+Alt+Shift+Down，不将原标准组合算作通过。
- 截图实际检查两行含无效字节的文本均可见、两个插入点有效、换行后四行内容和光标正确；SDK paint 字形/测量/像素回归同时通过。保存后的原始无效字节保持不变。
- 打开 Unicode Go fixture，补全弹层实际显示 `gopls` provider footer；Tab 接受跨文件变量后，缓冲区/磁盘内容精确回读，一次 Undo/Redo 正确，F12 跳到 `unicode_target.go`，Alt+Left 返回。此次是小文件，未把上一轮 100k 行结果冒充新二进制的大文件 UI 回归。
- 10.186 秒空闲采样：IDE CPU 时间 **0.109375 s**，约 **1.07% 单核**；gopls CPU **0.046875 s**，约 **0.46% 单核**；20/20 Responding，IDE 当时工作集约 59.0 MiB。不是 FPS、峰值内存、快速连续输入延迟或长期性能认证；脚本的固定等待也不是响应延迟测量。
- Alt+F4 正常退出：IDE PID 7696、gopls PID 8144 均退出。按本轮 `apps-final` 可执行路径回查（含诊断/测试工具）残留进程 **0**；只注销本轮创建的 7 个计划任务，测试目录/失败证据保留。
- [最终多光标及坏字节显示截图](screenshots/silkide-byte-edits-windows-20261024.png)；[最终随包 gopls 补全截图](screenshots/silkide-bundled-gopls-windows-20261024.png)。

## 证据与边界

- `summary.json` 汇总最终计数、跳过项、34 阶段、UI/子进程身份和性能采样；`REPLAY.txt` 说明重放条件。
- 本地 `bin/qa-artifacts/editor-byte-edits-20261024/`；远端 `C:\silk\qa-editor-byte-edits-20261024-codex`。
- Windows 使用 1,801 项冻结基线 + 最终 36 项 overlay；构建前逐项 SHA-256 验证。原路径微基准也校验其独立 before 输入哈希。
- 最终 Windows IDE SHA-256：`d5cf947e68023e8ba31246a5d903f52a6901115ef3ce90676f8b2aecb18a570a`。随包 gopls：`5c409131c93b3d7136145492c5a39a08972fc483660beb4d2125a25447a79502`。
- `windows-final-evidence.tar.gz` 下载前后 SHA-256 相同：`dedd832ef15be6f407bac91e35b4f1c8cae37de79af8129524f08ff9eeb1d9c2`；含最终及失败阶段的独立日志、截图、源码 overlay 和输入清单。当前源码 36 项与最终快照重新逐项核验一致；生产输入/最终 UI 日志均不含临时 `QAKEY` 诊断代码/输出。
- 畸形 UTF-8 在屏幕上显示替换字形是解码显示行为；验收以保存后的原始字节/hash 为准，而不是复制屏幕文字。Unicode 剪贴板无法承诺保存任意无效字节，UI 验证用有效 Unicode 粘贴并检查文件中未改的原始字节。
- 仍有全文 rebuild、同步 gofmt/保存、长行绘制/命中测试、所有 SDK 转换/外部 provider 的独立成本或覆盖缺口；没有宣称所有文本命令、控件及平台都已验收。

本轮未重跑 Linux 桌面、macOS 原生桌面 UI、真实 IME 组合态、混合 DPI、全部菜单与控件组合；标准 `-short` / 平台 / 缺可选工具的 skip 不算通过。Git 缺失仍明确记为外部能力缺口。下一步应补多光标的可发现菜单/命令面板入口，减少对可能被桌面拦截的组合键依赖，并继续处理超长单行的前缀扫描、全文 rebuild 和快速输入延迟。
