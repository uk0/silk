# 原生文件面板与命令状态回归 — 2026-10-25

接续设计器细节迭代。本轮不修改全局/画布的按键优先级，不将单测当作桌面验收。

## 根因及修复

- 上一轮已验收的基线二进制（SHA-256 `51c06c012b3c18262c2592523b6989664b6b572c5b2eaed53ff0b0ad9b1e61c6`）：Design 模式空画布、选中控件时 Cmd+Shift+P 均可用；原生 Save 成功或 Open→Cancel 后失效，点击画布后恢复。
- 最小探针记录：文件面板开始时 `key=owner / responder=GLFWContentView`；完成与释放时 `key=0 / responder=GLFWContentView / active=1`。失败按键没有进入 Go `onKey`，因此不是画布吞键或 IME 拦截。
- native session 结束时先恢复 owner 原启用状态，再恢复关键窗口；不激活后台 app，不覆盖其它 key window/sheet，不恢复已关闭/禁用/隐藏/最小化 owner。退出和强制取消/异常退出不抢焦点。没有新增 timer 或稳态轮询。
- 六个设计动作将工具栏的 `enabled` 同步到命令面板。不可用命令按 Enter 不关闭面板；模态退出后仍会重查状态，不修改隐藏画布。不可用提示放在说明前，避免被长说明挤出视口。

## 自动回归

- 新设计命令测试在修复前失败，保留 `focused-before-palette-fix.log`；修复后通过。覆盖空画布、双控件选区、应用/解除 GridLayout、Edit/Welcome、过期命令、不可用提交和提示位置。
- 原生 session 测试覆盖启用状态恢复顺序、成功/取消、owner 关闭、退出、异常传播及 owner 可恢复性。
- macOS 四包 `go test -race -short -json -count=1 -p 2 ./gui ./ged ./prop ./cmd/silkide`：**5,192 pass / 42 skip / 0 fail**，含子测试；`go vet -composites=false` 同四包和 RequireBundle 构建通过。
- 独立设计器根包 `go test -race -short -count=1 .` 与构建通过；未计入四包数量，根包未做本轮原生 UI 验收。
- 额外 Objective-C 策略探针直接包含生产 `.m`，12 个模拟 Cocoa 状态通过；它验证判断分支，不替代真实 AppKit UI 测试。

## macOS 真机 UI

使用独立 HOME/数据目录和未插桩构建，SHA-256 `eb358bcf18746dffb1b870e8b980f7b8ecaca4739229419d09d65577ba67718b`。

- 干净启动→Design→Open→Cancel→不点击画布，直接 Cmd+Shift+P：成功。
- 空画布搜索 grid：可见 Unavailable；Enter 后面板仍打开。
- 真实拖入两个 Button→Cmd+A→命令面板 grid→Enter→原生保存：文件读回一个 GridLayout、两个 Button。
- 保存后不点击画布，立即 Cmd+Shift+P→break：可执行；解除后保存读回无 GridLayout、两个 Button 均保留。
- Cmd+2→grid：不可用，Enter 不执行。原生打开 `qa_beta.go` 后立即打开命令面板返回 Welcome，最近文件可见并持久化。
- 打开文件测试一度停在文件选择阶段（Return 未提交），已通过 AX Open 按钮完成，再做快捷键验证；保留中间截图，不作为通过证据。
- 六个原有 fixture 字节不变；正常退出后 IDE 与随包 gopls 均消失。
- Welcome 闲置 5.006 秒，主进程 CPU +0.04 秒，约单核 **0.80%**，不含 gopls。只是短时存活/闲置观察，不代表所有场景性能或较前版本的提速幅度。

## Windows 10

- 同四包原生 `-race -short -count=1`：**5,061 pass / 65 skip / 0 fail**（含子测试）；RequireBundle 构建与工具 manifest 校验通过。
- 强制随包 gopls 的未保存重命名、Unicode 补全/格式化、导航三项集成测试全部通过、无跳过。
- 基线 1,801 文件 hash 与 351 overlay 输入校验通过；远端 tested-overlay 与本地当前源文件一致。该范围是代码/构建输入，不声称覆盖全部资源资产。
- Open→Cancel 后直接打开命令面板；空画布 grid 显示不可用，Enter 保持面板；实际拖入两个 Button→grid→原生保存，读回一个 GridLayout、两个 Button。
- 保存后直接打开 break→解除→保存，两个子控件保留；Edit 模式 grid 不可用且 Enter 不执行。截图已逐张检查，原有六个 fixture hash 不变。
- 设计模式闲置 5.012 秒 CPU +0.078125 秒，约单核 **1.56%**（不含 gopls），`Responding=true`；不作为跨场景提速结论。
- 两个交互任务均 `Ready / LastTaskResult=0`；IDE 与 bundled gopls 正常退出，QA apps 下无残留进程。
- Windows IDE SHA-256：`d5451bfcaabced6d0bbdf8ea5fdac9e0d4b525a20683c0fbbeb32354819957aa`。原始截图含窗口外边框/桌面，留在忽略的 QA 目录，不作发布截图。

## 证据与边界

`bin/qa-artifacts/shortcut-routing-20261025/` 保存原始基线、独立 trace、fixed app、构建与测试日志、输入 hashes、原生截图、保存前后 `.silkui`、fixture 比较、CPU 与退出读回。

- 探针与原始二进制分开；正式源码不包含 KEYTRACE。诊断时发现 Go overlay 对该 `.m` 未实际生效，先用二进制字符串确认插桩存在，才采信 `trace/native2.stderr`；早期未生效 trace 不用于 Cocoa 根因结论。
- 本轮当时仅保证禁用状态不再被长说明挤走；后续已增加文本测量／省略与完整说明区，见 [命令面板 QA](PALETTE_DETAILS_QA_20261025.md)。不宣称所有布局细节完成。
- 继续发现的细节：属性表仍有长浮点数直接显示（例如 `65.087499999…`）、混合值低对比度、部分中文属性标签；后续修复与精度回读见 [属性面板 QA](PROPERTY_POLISH_QA_20261025.md)。
- 本轮没有 Linux 原生桌面验收。没有新增公开 SDK API，没有发布、commit 或 push。
