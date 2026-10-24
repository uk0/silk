# Workspace search / safe replacement QA — 2026-10-24

基线：`fcca4e93a4fa79169369a3aaec9fcc30ce5552c4`。
这是增量开发，不是新版本发行或 Qt Creator 全功能对齐声明。

## 修复与新增

- 工作区搜索改为一个 active + 一个 latest 请求；全进程最多两个计算 worker，
  75 ms debounce、10 秒计算期限，支持取消、隐藏与关闭生命周期。
  generation 同时隔离查询、选项、过滤器和根目录；清空输入立即撤销旧结果。
- 搜索、替换、Types 和 Exclude 使用面板内真正的 Edit 子控件，支持选择、粘贴、
  删除；增加 Aa / Word / Regex、Search / Cancel / Preview / Apply。
  按实际 SizeHints 换行，结果绘制只遍历可视行，命中片段按 UTF-8 截取。
- 点击结果先比较当前缓冲区行内容，再把字节列转换为 rune 列；旧结果不会盲目跳转。
- Preview 重新扫描磁盘、只生成提案；点击文件使用现有异步 DiffView 对话框。
  Apply 在后台检查 Before 与磁盘一致再执行 filetxn，不在 UI 线程同步写文件。
- 原有 ApplyReplacements 会覆盖预览后的外部修改，新增回归测试先复现失败后修复。
  文件事务提供乐观冲突检查、普通失败回滚及失败恢复副本；不是多文件崩溃原子操作，
  也不能排除第三方恰好在检查与 rename 间写入。错误不触发成功回调。
- IDE 拒绝未保存／内容不符的缓冲区、并发重命名／宏／调试，以及活动设计文档目录。
  写入期间保留编辑器并阻止保存、关闭、打开相关路径与退出；成功后同步正文和 LSP、
  保留一个 Undo 步骤；期间出现的更新缓冲区不会被旧结果覆盖。

## SDK 与边界

新增 `filesearch.SearchContext(ctx, root, pattern, ContextOptions)`、
`ReplaceTextContext`、`SearchLimits`、`Occurrence.Length`、显式完整性与资源错误。
保留既有 Search / Replace / Options 和同步面板便捷方法的兼容性。
旧同步方法不承诺非阻塞；原生 IDE 使用新异步路径。

- 扫描磁盘，不扫描未保存缓冲区；行内正则、正宽度命中（忽略零宽命中）。
- 编辑器与工作区复用 `internal/regexscan` 的可取消 RE2、真实边界上下文与捕获展开。
  工作区查询最多 64 KiB，仍受 32,768 表达式展开成本与 128 捕获限制。
- 默认最多 10,000 个命中／文件、100,000 目录项、64 层目录、8 MiB 每文件、
  256 KiB 每行、128 MiB 总读取、16 MiB 结果预算；二进制探测也计入读取预算。
  包含／排除过滤在读取前应用，跳过子路径符号链接并拒绝符号链接根。
- 超限／跳过长行或大文件／读取失败均明确显示 partial，禁止批量替换。
  替换提案 Before + After 总预算 32 MiB，模板最多 1 MiB；保留 LF、CRLF 与末尾换行。
- OS 层阻塞读取不能被 context 强行中断；并发 worker 上限限制其数量，不作任意磁盘
  延迟保证。多文件事务不是与外部进程协同加锁的数据库事务。

## 自动验证

```sh
GOMAXPROCS=2 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide \
  ./cmd/silktools ./internal/idetools ./internal/filetxn \
  ./filesearch ./internal/regexscan
```

- macOS：5,256 pass / 47 skip / 0 fail，11 个有测试包通过；
  internal/regexscan 无独立测试文件，但由 gui 原有正则差分测试及新 filesearch 测试执行。
- 同组 vet 通过；纯 Go filesearch / filetxn 与 silk_pure_go cairo / purecairo / paint 通过。
- 随 QA app 打包的真实 gopls：3 pass / 0 skip，覆盖未保存 Unicode 补全、格式化、
  重命名、类型／实现／定义跳转与返回；不是 PATH 上另一个 gopls 的替代验证。
- Windows 10 原生 cgo / race：5,144 pass / 62 skip / 0 fail；同样 11 个有测试包
  通过、regexscan 无独立测试文件。随包 gopls 3 pass / 0 skip，不依赖全局 PATH。
- 新测试覆盖取消、latest-only、过期目录／选项、无效正则、UTF-8 列、排除过滤、
  各类预算、CRLF、捕获替换、只读预览、外部冲突、脏缓冲区、事务保留与 Undo。
- 初轮专项测试有三个测试夹具问题：误用 Widget.Rect、中文字节列期望差一、
  密集结果预算测试失败时打印完整重复行导致日志分配过大；修正测试后冻结源码重跑。
  这不被计入通过；失败日志保留在本地 QA 目录。

## macOS 原生 UI

隔离工作区：`a.txt = 中🙂 foo_12 foo_34\n`、`b.txt = foo_56\n`、
`ignored_test.txt = foo_78\n`，以及 12,000 个 `a ` 的 dense.txt。

- 搜索 foo 显示 4 项／3 文件；Types=.txt、Exclude=*test.txt 后为 3 项／2 文件。
- 第二处结果跳转后输入 x 并保存，逐字节确认 `中🙂 foo_12 xfoo_34\n`；Undo 保存恢复。
- 使用 `foo_(?P<id>[0-9]+)` 与 `${id}_中`，真实预览 diff 显示中文和 emoji 正确。
  Preview 后两文件未修改；Apply 后分别为 `中🙂 12_中 34_中\n` 与 `56_中\n`，
  排除文件不变；打开的 a.txt 同步更新，Undo / 保存恢复原文。
- 预览后外部修改 b.txt：Apply 明确报冲突，a.txt 不受影响，b.txt 新内容保留。
  未保存 a.txt 时 Apply 明确拒绝，磁盘未改变。
- 非法 `[` 显示解析错误且禁用替换；密集查询达到 10,000 项时显示 partial 并禁用替换。
  测试结果滚动、取消及窄栏自动换行。
- 密集结果静置 10.005 秒 CPU 增加 0.080 秒，约 0.80% 单核。
  此为一次测试样本，不是通用键盘到像素延迟／帧率承诺。
- 正常 Cmd+Q 退出，本轮 IDE / gopls 残留为零。

## Windows 10 原生 UI

在独立目录、交互桌面会话中执行实际按钮／快捷键流程，不用 mock 代替原生 UI：

- 搜索 4 项／3 文件，过滤后 3 项／2 文件；第二处中文／emoji 后的命中准确定位。
- 命名捕获预览使用真实 DiffView；Preview 不写磁盘，Apply 同步已打开的编辑器。
  a.txt／b.txt 替换、排除文件不变、Undo 恢复、外部冲突与脏缓冲区保护均回读验证。
- 非法正则显示错误；10,000 命中显示 partial 且 Preview / Apply 禁用，点击无写入。
  取消和窄栏换行经过截图观察。12 份保存／磁盘产物取回后再次逐字节断言。
- 密集结果静置 10.190 秒 CPU 增加 0.15625 秒，约 1.53% 单核；20/20 次进程存活且
  Responding。正常 Alt+F4 退出，IDE / gopls 残留为零，5 个本轮交互任务已清理。

Linux 原生桌面未测试。
Windows 首轮 11 个业务测试包通过，但独立 regexscan 的 vet 因 Go overlay 的
虚拟包目录不存在而失败，整轮未计为通过。测试脚本现在临时创建缺失的空目录，
不改基线文件，在 finally 中清理，再重跑完整门禁。
开发 QA 构建不新增签名、公证、安装器或 Release tag。

## 证据

本地：`bin/qa-artifacts/workspace-search-20261024/`，包含原始 JSONL、专项测试、
二进制与源码 hash、磁盘逐字节回读、UI 截图与正常退出检查。

源码冻结后，Windows 验证 1,801 个基线文件与 142 个 overlay 输入；运行前／归档前
核对 SHA-256，取回归档后再次和本地及暂存源码比较。首轮失败不覆盖最终验收日志。

```text
macOS app SHA-256
b20ef4ac026490e545515323d59c0a1ab8299ee7ecd4451d5c88e52fdfe4cb8f
Windows exe SHA-256
56e509745b4274389325df6cf866532535b5b0b4125bf9d2358c21f6aee6074d
Windows evidence archive SHA-256
b93db3b8a920916d8f69e0bcc81b27e8488455d7913f7a98687a4f40f5614023
```
