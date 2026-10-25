# 编辑器后台批量关闭 — 2026-10-24

基线 `13e3d0b9dc51766ae9e25a544cd722b475628e6d`。本轮推进编辑器工作流完整性，
不代表 Qt Creator 对标目标或全部平台验收完成。

## 功能与安全边界

- 页签菜单：关闭其他／关闭右侧／关闭全部编辑器，按点击时的页对象和实际顺序选取目标。
  File 菜单和 Command Palette 也可全部关闭／取消；未跟踪缓冲区不自动关闭。
- 全组磁盘检查完成后才统一询问 Save All／Discard All／Cancel。取消确认不移除任何页。
  右键非活动页不会切换活动页；目标集合固定，新打开的文档不加入原批次。
- 磁盘读、gofmt、带冲突检测的保存都在单个工作线程上执行；worker 不访问控件。
  保存复用 `internal/filetxn`，先验证全组快照，普通失败回滚；不是跨文件 crash-atomic
  事务，也不能阻止第三方在最终检查／rename 之间写入。冲突／失败保留编辑器及恢复提示。
- 编辑器身份、路径、TextRevision 在预检后、对话框返回后、保存完成和实际关闭时复核。
  用户在后台工作期间继续输入时，不覆盖新文本、不把新文本误标成已保存。
- 保存状态逐页发布，关闭也逐页 Post，不在一个 UI callback 中循环关闭所有文档。
  已完成的页可以进入关闭历史，后续取消或内容变化会保留剩余页，不承诺撤回已完成关闭。
- 与重命名／工作区替换／宏生成及单页保存关闭的路径占用互斥；执行期间阻止退出，
  防止进程在写入过程中结束。停止关闭不会回滚已经提交的保存。
- 单个批次最多 4,096 页，内存文本快照总量上限 64 MiB，磁盘快照也按预算读取；
  磁盘检查有错误的非空文档仍可显式 Discard，Save 则报错保留。拒绝不安全的符号链接保存。
- Cancel 会取消后续工作／gofmt 并停止移除；已经进入的 OS 文件读取或文件提交不能强制中断，
  工作线程返回前仍保留任务占用，不以重复创建线程掩盖慢盘。单页保存、关闭和退出的既有
  同步读写尚未整体迁移，所以不是“所有 I/O 零阻塞”或 FPS 保证。
- 确认框最多显示 8 个路径，其余计数；长路径按 Unicode 字符截断，hover 保留完整路径，
  退出后清理 tooltip 注册，不把超长路径或上千标签直接变成超高对话框。

## 额外修复：单页关闭的模态竞态

原关闭确认框运行嵌套事件循环；若期间该页被移除并以同路径重开，旧缓冲区仍可能写回，
然后旧数字索引指向其他页。新增反例 `TestIndividualCloseModalReplacementDoesNotSaveOldBuffer`
先复现 `obsolete close wrote over replacement`，再修复为返回后先核验对象身份才允许保存，
移除时再次按页面身份定位。与批量关闭共享 cleanup、LSP 清理、Git/查找取消和关闭历史。

## 单元／集成验证

最初菜单反例：`missing batch-close command tab.closeOthers`。
后续覆盖三种范围、未跟踪／新打开页面、取消前不移除、三种决定、逐轮移除、磁盘冲突、
保存失败、不可读文件、工作中输入、保存后迟到输入、不同阶段取消、模态内修改、owner 替换、
File／Palette 命令、路径占用和历史恢复。真实 goroutine 被读取 barrier 阻塞时，测试线程
仍可修改缓冲区并取消，且不会启动第二个工作线程；它是合成慢盘验证，不冒充真实网络盘测量。

冻结 1,551 个源码／构建输入；Windows 使用不可变基线 1,801 文件和 194 个 overlay，
构建前后逐文件核对 SHA-256。主批次为 async/core/gui/ged/paint/purecairo/cmd/silkide/
cmd/silktools/internal/idetools/internal/fileops/internal/filetxn/filesearch/internal/regexscan，
额外 graph/prop/settings；均 `-race -short -count=1 -p 2`。

| 回归 | macOS arm64 | Windows 10 amd64 |
| --- | --- | --- |
| 主批次 race | 5,390 pass / 47 skip / 0 fail | 5,280 pass / 62 skip / 0 fail |
| graph / prop / settings race | 238 pass / 1 skip / 0 fail | 240 pass / 0 skip / 0 fail |
| 真实随包 gopls | 3 pass / 0 skip / 0 fail | 3 pass / 0 skip / 0 fail |

新增批量关闭／模态身份相关测试在两端各 31 个 pass 事件（含子测试）。

计数含子测试终结事件，skip 不计作通过；regexscan 没有单独测试文件。
macOS `go vet -composites=false` 通过，不是 strict-vet 或全仓 `./...` 声明。
实际随包 gopls 独立验证未保存 Unicode 补全／格式化、重命名、类型／实现／定义与返回。

本机微基准（Apple M2 Max）：1,000 页身份检查 28,526 ns/op、36,944 B/op、5 allocs/op；
含新增命令的页签菜单 4,348 ns/op、6,736 B/op、44 allocs/op。
只计内存检查／菜单构建，不含磁盘、原生弹窗、端到端输入延迟。

## 原生 UI

首轮 Windows 夹具错误地向菜单输入带空格的完整名称。菜单的 Space 是激活键，
因此 `close other` 在输入到空格时触发已有 Close Editor，后续 Enter 保存并关闭了 Beta，
没有执行批量命令。截图／磁盘回读和 `Menu.OnKeyDown` 证实这是输入夹具错误。
保留失败脚本／日志／任务退出码，真实 UI 撤销和恢复原文及页顺序后，从干净状态改用
Home＋方向键选择批量命令，不为通过夹具而修改 Space 的标准菜单行为。

Windows 10 Session 1，最终构建运行在隔离目录／配置中，未依赖用户 PATH 中的 gopls。
本轮已确认的原生 UI 行为：

- 三个含中文／emoji 的文档分别有未保存修改。右键非活动 Beta 选择关闭其他，
  确认框只列出 Alpha／Zulu；按钮、文字、路径没有越界或覆盖。
- Cancel 后三个缓冲区仍保留修改，逐个保存、精确字节回读，再撤销保存恢复原文。
- Alpha 保持活动，关闭右侧 Beta／Zulu，Save All 后两个修改准确写盘，Alpha 未改，
  两个目标页关闭。以上保存产物在本机再次核对 SHA-256 和 Unicode 原文。
- 原生菜单复用现有图标，截图见
  [菜单](screenshots/windows-editor-batch-close-20261024.png)／
  [确认框](screenshots/windows-editor-batch-close-confirm-20261024.png)。截图仅证明可见状态，
  不代替对应磁盘／页面身份断言。

焦点检查有未关闭的验证项：跨计划任务继续操作时，一次 Esc 未发给确认框，另一次
Save All 已完成但后续恢复页签快捷键发给了其他前台窗口。两次失败均未计入通过。
追加同一任务内的 Cancel 对照：确认框原生 owner 为主窗口，打开时主窗口禁用，
关闭后主窗口重新启用且成为前台窗口，未修改 SDK 为无条件抢焦点。
这只能证实该次 Cancel 正常，不能据此排除 Save All／跨任务场景的焦点问题。

后续连续 Save All／Discard All／菜单与 Palette 全部关闭脚本已投递，但测试机随后
SSH `Connection timed out during banner exchange`，两个已配置地址均无法回收结果。
最后复查时间为 `2026-10-25T01:43:00Z`。仅终止了本机核验身份后的失联 SSH 客户端，
没有重启远程测试任务或强杀未知进程。**连续脚本退出码、Discard All 原生流程、
菜单／Palette 全部关闭原生流程、64 页压力测试、空闲 CPU 和 Windows 正常退出清理
均未完成验收**；不能报告无卡顿、焦点问题已修复或远端残留为零。
恢复连接后应先读取原任务结果和当前窗口状态，再决定恢复夹具，不能盲目重放修改操作。

macOS 本轮实际启动已校验哈希的独立 bundle，但 `cua.getApp` 仍返回
`ScreenCaptureKit.SCStreamErrorDomain Code=-3811`。没有注入 UI 输入，不能声明 macOS
原生 UI 通过；核验 PID／进程组后 SIGTERM 清理，剩余为 0，不算正常 UI 退出。
Linux GUI、混合 DPI、多屏、中文 IME 和真实慢网络盘未验收。

## 证据

`bin/qa-artifacts/editor-batch-close-20261024/` 保存红／绿日志、冻结清单、JSONL、
真实 LSP 日志、脚本、截图及保存字节。Windows 根目录
`C:\silk\qa-editor-batch-close-20261024-codex`；未改全局工具链／PATH 或共享基线。

已测试二进制 SHA-256：

```text
Windows IDE  62d4059b34742bd94a8bf7ac498cee393e23eceb2697c7eb2fdb59cf5f9491fe
Windows SDK  09d8c9c1b8dd6d959b9b919a69f1ed55c466637328e499102d0c73bae13d682a
Windows gopls 5c409131c93b3d7136145492c5a39a08972fc483660beb4d2125a25447a79502
macOS IDE    32f1e49ff695734f66ebc5018db37adb76ae2f03c5b1914669c4a0a61a7a391d
```

提交前重新核对全部 1,551 个冻结输入无变化，并重新运行批量关闭／单页模态身份
相关的 `go test -race -short -count=1` 定向回归，通过；`git diff --check` 通过。

本轮为开发增量，不发布安装包，也不宣称跨平台原生 UI 全部通过。
提交排除无关的 review、gen_image 和本地 silkide 文件，以及忽略目录中的 QA 产物。
