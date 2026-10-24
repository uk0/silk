# 大文件差异比较、取消与原生桌面回归（2026-10-24）

本轮承接 `DOCK_DIFF_QA_20261024.md` 的明确缺口：完整文本比较仍在 UI 线程分配二次大小的 LCS 矩阵。先保留失败反例，再替换算法和 IDE 调用链；不将绘制优化当成计算优化，也不将本轮通过当成 Qt Creator 全面对等。

## 改动

- **精确线性空间 Diff**：以双向 Myers 中点分割代替 `(M+1)*(N+1)` 矩阵，复用两个前沿数组、使用显式栈；保留完整输入与最优匹配长度，不使用“大文件一律替换”的近似降级。裁剪公共前后缀；完全不相交的区段先证明无公共行，再直接生成结果。最坏时间仍依赖编辑距离，并非所有输入都线性。算法参考 [Myers 原论文第 4b 节](https://neil.fraser.name/writing/diff/myers.pdf)。
- **SDK 异步接口**：`DiffView.SetTextsAsync`、`CancelDiff`、`DiffPending`、`DiffError`、`DiffSummary`、`SetDiffStateChanged`。每个视图一项活动计算和一项可替换的最新请求，全进程最多两项计算；后台只处理不可变文本。行号、统计和行数据一并在后台构建，UI 以 O(1) 赋值接收；代次/视图身份校验和关闭围栏阻止迟到结果覆盖新内容。没有新增空闲 worker 定时器。
- **生命周期**：同步 `SetTexts` / patch 替换会废弃旧异步结果；取消不等待 UI 队列或后台退出。单独隐藏 DiffView 会取消；宿主仍须在祖先对话框销毁时显式 `CancelDiff`。复查两端原生销毁都会先隐藏再脱离窗口，因此模态返回后的解除 parent 不会重建可见窗口。
- **IDE 三入口**：Diff vs Saved、Diff vs HEAD、Git 提交比较复用同一个可取消对话框，先打开再异步读取/比较。显示 loading/computing/error/行数状态；新增上一处/下一处按钮，计算中或没有变化时禁用。DiffView 的布局 stretch 修正后能够占用剩余高度。
- **Git 读取**：新增带 context 的比较入口，并让既有接口复用它们；使用既有 10 秒超时、8 MiB stdout / 64 KiB stderr 限额，禁止外部 diff/textconv 和颜色输出，拒绝 option-like revision。超过限额明确失败，不静默截断。保存文件读取移出 UI 线程，但阻塞的内核文件读取本身无法强制中止。

## 自动测试与对照

显式测试 8 包：`async`、`internal/idetools`、`cmd/silktools`、`cmd/silkide`、`core`、`gui`、`ged`、`paint`。使用 `-race -short -count=1 -p 2`，不使用受限目录下的 `./...`，不把缓存结果或条件 skip 当通过。

| 平台 | pass（含子测试） | skip | fail | 真实随包 gopls |
| --- | ---: | ---: | ---: | --- |
| macOS arm64 | 5,078 | 47 | 0 | 3 pass / 0 skip |
| Windows 10 amd64 真机 | 4,973 | 62 | 0 | 3 pass / 0 skip |

- 本轮 16 个顶层定向用例在两端均明确通过：穷举最优性/无损恢复、Unicode/不等长随机输入、十万行稀疏修改与整块替换、遍历取消、迟到结果、同步/patch 覆盖、上下文取消/隐藏/重启、200 请求合并、全局并行上限、对话框关闭围栏/导航/布局、Git 禁用外部转换与输出上限。
- 所有长度 0–6 的二元序列两两比较，以独立 DP 检查最优匹配长度及输入重建；另有 1,500 组随机 Unicode 等样本。20 秒 fuzz 完成 **214,884 次执行**，未发现失败。
- 旧实现负例：2,000 行、单处修改分配 **33,177,248 B**，超过 8 MiB 回归门槛。新实现同一基准为 **81,992 B / 3 次分配**。旧单次计时 17.36 ms，新两轮约 18.25–18.60 μs；旧样本只有一次，不能当严谨速度倍数或原生输入延迟证据。
- macOS `make bundle-ide`、8 包 `go vet -composites=false`、`silk_pure_go` 定向 race 通过。两端 gopls 集成覆盖未保存 Unicode 补全/格式化、文件重命名、定义/类型/实现/返回；Windows 为原生 race 测试程序，macOS 从实际 `.app/Contents/MacOS` 中使用随包工具。

## 原生 UI 闭环

### Windows

- 新 QA 仓库、独立 APPDATA/数据目录、带空格模块目录；核验实际 IDE 二进制哈希，实际 gopls 子进程来自 `apps/tools/gopls.exe`。源码在测试前后均核对 **1,801 项基线 + 95 项 overlay**，不修改共用基线。
- 十万行文本，在第 1 / 95,001 行各插入 `!`。Diff 显示 **100001 rows / 2 changed**（含尾部空行）；实际 Next、Next、Previous 分别落到第 1、95,001、1 行。缩小对话框至 450×600，按钮和 Close 仍在边界内，列文字不跨列。
- 关闭后保存、撤销一次保存、再次撤销保存，三阶段均逐字节读回，内容依次为双处修改、仅首行修改、原文。
- 将 50,000 行文本通过原生粘贴替换为逆序，并全选复制读回确认输入。比较期间截图确认为 **Computing comparison...**，导航禁用，20/20 次已知模态 HWND 的 WM_NULL 成功，最大约 **39.61 ms**；可缩小窗口并 Esc 关闭。关闭脚本耗时约 434 ms，**包含 SendKeys 固定等待，不能当纯键盘延迟**。
- 撤销逆序粘贴并保存，第四阶段字节读回与原文一致；重新打开比较显示 **50001 rows / 0 changed**，没有迟到的逆序结果。
- 取消后 10.171 秒无输入采样：IDE **0.109375 秒 CPU（约 1.08% 单核）**，gopls **0 秒 CPU**，主窗口 20/20 次消息响应。只证明这一有限样本，不证明长期空闲、所有输入延迟或 FPS 上限。
- 实际 Git 提交比较显示 3 行 / 2 处修改，长行按列裁剪；`N` 键选中变化行。Diff vs HEAD 的无变化分支也实际打开；当前文案仍为 `Comparison failed: No diff to compare`，应后续改为中性空状态，而非把“没有修改”描述成失败。
- 实际 F12 从 main.go 跳至 target.go，Alt+Left 返回。5 次对话框开关，GDI **18→18**、USER **30→30**。Alt+F4 正常退出，本轮应用进程剩余 0，清除本轮 5 个交互任务，保留 QA 目录。
- 本次 Windows UI 日志没有 panic、race 或事件循环长停顿告警；这不是所有控件或所有机器的无卡顿保证。

### macOS

- 使用本轮 `bin/SilkIDE.app`，实际随包 gopls 子进程路径已记录；原生打开十万行文本，在第 95,001 行修改，显示 100001 行 / 1 处变化，实际点击 Next 跳至该行。
- 关闭比较后保存、撤销保存，两个阶段独立字节读回通过。原生 F12 与 Alt+Left 成功，正常退出后 IDE / gopls PID 均消失。
- **未解决告警**：CUA 粘贴期间观察到一次 `event loop resumed after 9.408s`，尚不能归因于剪贴板、编辑器或自动化等待；没有用此次 Diff 成功掩盖它。下一步应以 `SILK_DEBUG=true` 抓停顿堆栈，单独重放普通输入与原生粘贴。
- 本轮 macOS 仍恢复了部分既有标签，不能声称偏好完全隔离。自动化首次过早 Enter 在文件搜索结果就绪前未完成打开；等结果可见后重试成功。首行初次输入未落入编辑器，本轮 macOS 因而只验证一处远端修改，不冒充两处修改。

## 证据与重放

- 报告对应原始目录：`bin/qa-artifacts/diff-async-20261024/`。包含 before 快照、失败反例、基准、fuzz、完整 race JSONL、代码哈希、Windows 源码 overlay、PowerShell 重放脚本、进程身份、保存字节、截图、`verify.py`、`summary.json`、`visual-review.json` 与 `increment.patch`。
- 独立 `verify.py` 核对 8 包结果、全部 16 个新增顶层测试、3+3 个真实 gopls 测试、4+2 个保存阶段、源码/产物身份、WM_NULL、资源计数及退出。Windows PowerShell 5.1 把先前三个保存阶段包进了数组 `value`；保留原始 JSON 和首次断言失败，校验包装结构后再展开并逐文件核对，不篡改原始档案凑计数。
- 源码输入与两端构建一致；文档/截图更新不改变已测代码。保留本轮开始时的 **209 项既有工作区变化**，本轮未 commit、push 或发布。

```text
Windows IDE SHA-256
91a7f01143d6a5ae7c9a95c047ecf0a956ab3f32ff963940131d13d6a42e04c1
macOS IDE SHA-256
cba0b1715e753f194779643d8273b768e17cfdaa78c9d615df7897e67a3165e7
Windows 原始证据归档 SHA-256（远端和本地一致）
831c886383daf7e4529ae6f4ededae098f69d2a919e52e708ee86a38a9d61e74
```

截图：[Windows 十万行跳转](screenshots/silkide-diff-async-windows-20261024.png)、[Windows 计算中可取消](screenshots/silkide-diff-pending-windows-20261024.png)、[macOS 原生比较](screenshots/silkide-diff-async-macos-20261024.png)。

## 保留边界

- `SetTexts` 仍是同步兼容 API；交互宿主需显式使用异步接口并正确取消。超大编辑距离仍可能耗 CPU，本轮以后台执行、并行上限和可取消性处理，不声称消灭最坏时间复杂度。
- Git 对比仍展示首个文件的变化邻域，没有实现完整多文件导航、缺失上下文重建或横向浏览。无变化提示文案、macOS 粘贴停顿是下一轮明确目标。
- 先前已记录的浅色 Git History/sticky header、拖动光标资源、完整高 DPI / IME、Linux 桌面和 Windows ARM64 尚未在本轮解决或验收。
- 当前产物是 `bin/silkide` / `bin/SilkIDE.app`；根目录旧 `silkide` 不会因改源码自动更新。完整打包包含 gopls，单独 `go build ./cmd/silkide` 不等于交付完整工具包；本轮没有替换用户旧安装或发布新版本。
