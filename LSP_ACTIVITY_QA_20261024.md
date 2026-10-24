# 语言服务进度、消息隔离与后台 CPU 复核（2026-10-24）

## 证据与新增功能

- 上轮 Windows 短行编辑后的 10 秒样本包含 gopls CPU 增长 2.984 秒。同次 stderr 在 19:55:50 记录 `background imports cache refresh starting`，19:55:51 记录刷新耗时 1.6807885 秒。这解释了采样时间窗口内存在后台缓存任务，但不是精确 CPU profile，不把全部 CPU 都归给它，也不归咎于纯文本输入。
- 重新检查实际调用链：`openLSPDocumentAfter` / `updateLSPDocument` 都先检查 Go 文件。新增测试验证 `.txt`、`.json`、`.md` 不创建 LSP 文档 worker，不修改 LSP 版本。
- 补齐 SDK 的有界 work-done 进度 API；默认 SDK 能力不擅自改变，IDE 显式启用。独立进度通道合并通知，避免占用诊断队列；数值/字符串 token、begin/report/end、可选字段、不可变快照、超限和 EOF 都有测试。
- 原读循环在 1,000 条进度后丢失诊断的负例已复现；修复后只保留一个进度唤醒，诊断仍可读、RPC 响应仍可到达。该保障针对已启用的 work-done 通道，不声称解决所有类型的通知洪泛。
- IDE 状态栏新增可键盘访问的图标按钮：启动中、已连接、工作数量/百分比、未连接、显示受限；点击进入可复制的工具链报告。服务器标题/消息限制长度并清理控制字符，不直接撑大状态栏。
- Windows 首次原生截图发现带图标按钮默认隐藏文本：状态只剩齿轮，单看 `Text()` 单测发现不了。补充 `IsTextVisible()` 红阶段断言后复现失败，显式启用文本并重新进行两平台完整回归、构建与原生复测，不把首次截图算成功。
- 每个会话最多排队一个进度 UI 回调；回调检查客户端与会话代次，重启后的旧消息不能覆盖新状态。不添加轮询、动画或常驻刷新定时器；无上报任务只显示“已连接”，不宣称“CPU 已空闲”。

## 验证状态

最终源码验证（包含状态文字修复）：

| 平台 / 检查 | 结果 |
| --- | --- |
| macOS arm64，7 包 `-race -short -count=1 -p 2` | **5,027 pass / 47 skip / 0 fail** |
| Windows 10 amd64，同组原生 race 回归 | **4,899 pass / 85 skip / 0 fail** |
| macOS 实际 `.app/Contents/MacOS` 随包 gopls | **3 pass / 0 skip** |
| Windows 实际应用目录随包 gopls（race 编译测试） | **3 pass / 0 skip** |
| macOS `silk_pure_go` progress/activity race 子集 | 通过 |
| `make bundle-ide`、7 包 `go vet -composites=false`、`git diff --check` | 通过 |

7 包为 `internal/idetools`、`cmd/silktools`、`cmd/silkide`、`core`、`gui`、`ged`、`paint`；计数包含子测试，skip 不计为通过。真实 gopls 用例验证 `Setting up workspace` 的 completed work，且补全、格式化、重命名、定义/类型/实现及返回仍通过。SDK 的 1,000 条进度隔离、10,000 次 UI 合并、快照所有权、会话代次和并发读写均覆盖。

### Windows 原生闭环

- 最终 exe 在新的配置目录、新工作区启动；桌面继承 PATH 没有全局 gopls，实际子进程来自应用旁 `apps\tools\gopls.exe`。
- 点击**新状态栏按钮**，工具链报告复制回读验证 `[bundled]`、`connected: true`、进度未超限以及真实 `Last completed operation: Setting up workspace`。截图确认状态文字完整可见，图标/文字/缩放/版本未互相覆盖；可滚动报告与 Close 按钮分离。
- 原生 F12 跳到 `target.go`，Alt+Left 返回。纯文本 3 轮输入/保存/撤销/保存共 6 阶段；重启后 Go 注释保存/撤销再增加 2 阶段。**8 个阶段**均从磁盘复制，下载后用独立 Python 预期字节和 SHA-256 再次校验，不只相信 UI toast。
- 核对 PID、父进程、绝对路径后，仅故意终止本轮 gopls PID 5536。状态栏变为 `gopls: offline`，报告 `connected: false`，预期出现 `[SILK-LSP003]` 断线提示。随后实际点击 **Tools > Restart LSP**，新 PID 6084 启动，旧 PID 消失，进度报告与 F12/返回再次通过。
- 正常 Alt+F4 关闭 IDE，新服务器一并退出；本轮应用目录下进程为 0，清理本轮 4 个计划任务，未终止其他应用。原生状态按钮的 Tab 导航未单独测试；`TabFocus`/可见文本由单测验证，报告全选/复制/关闭由真实键盘输入验证。

### CPU 样本与边界

采样前实际日志已记录后台导入缓存刷新完成（20:38:22，耗时 1.6716121 秒）；以下时间为测试机本地 UTC+8：

| 样本 | 时长 | IDE CPU 时间 / 单核占比 | gopls CPU 时间 | 响应检测 |
| --- | --- | --- | --- | --- |
| 纯文本编辑前，20:38:47 起 | 10.188 秒 | 0.171875 秒 / **1.69%** | 0 秒 | 20/20 |
| 纯文本编辑后，20:39:09 起 | 10.165 秒 | 0.218750 秒 / **2.15%** | 0 秒 | 20/20 |

这证明本次受控窗口内没有持续的 gopls CPU 增长，不是零 CPU、单键输入延迟、帧率或长期不卡顿承诺。无采样器/轮询器被加入产品；进度显示也不等于后台缓存已完成。

### 产物一致性

- Windows 不可变基线 **1,801 文件 + 67 overlay 输入**逐文件校验；本轮 **13 个代码输入**及完整 overlay 在回归后再次核对，无漂移。
- 最终 IDE SHA-256：`482a0730d48a07df5d3ba14b349be3b3c9de3071d66828584b4d95bc6133a0cc`。
- 随包 gopls SHA-256：`5c409131c93b3d7136145492c5a39a08972fc483660beb4d2125a25447a79502`。
- 下载证据包 `final-evidence.tar.gz` 与远端 SHA-256 一致：`bc188da99145b29c73aca00e66a523e1f5034eea29555ae05c70354e93e9d02a`。
- `summary.json` / `summarize.py` 保存计数、逐字节验证与采样；`visual-review.json` 记录实际看过的截图；`increment.patch` 只比较本轮开始快照与本轮改动，不掩盖继承工作区修改。

[Windows 最终状态栏与编辑器截图](screenshots/silkide-lsp-activity-windows-20261024.png)

## 重放与范围

- `core/LSP_PROGRESS.md` 是公开 API/边界说明。
- 本轮原始证据：`bin/qa-artifacts/lsp-activity-20261024/`，保留修改前源码、负例、当前源码哈希、构建/回归和原生脚本。
- 真实集成继续覆盖未保存 Unicode 补全/格式化、重命名、定义/类型/实现及返回；其中补全用例额外断言真实 gopls 已发出 completed work，不以合成通知代替实际协议验证。
- 跳过项不当作已通过；尚不提供进度取消 UI，也不将进度静默解释为所有后台工作结束。
- 不替换或重建 Go SDK/Git/Delve；Windows 缺少可选 Git 的重复告警是独立待处理项，本轮不偷偷安装全局工具。
- 保留原工作区修改；未 commit、Git push 或发布。持续迭代目标未完成。

本轮未做 macOS/Linux 原生桌面、完整 IME、混合 DPI、多屏或长期负载验收。故意断线提示和可选 Git 缺失重复告警均保留在证据中，不声称日志零告警。旧安装目录未由本轮自动升级；当前改动尚未发布。
