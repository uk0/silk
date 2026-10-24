# Workbench navigation QA — 2026-10-24

基线：`a79edf6833447bf5293791d7ba8f6900ecb99cfc`。本轮为增量开发，不是完整版本发行。

## 问题与实现

先用失败测试复现两条输入链：

1. Search 命中 → `navigateEditor` → `openFileInEditor` → `activate(modeEdit)`，
   后者每次都强制切到 Files / Outline，导致搜索、书签等面板消失。
2. 欢迎页／手动隐藏左栏 → Find in Files 快捷键，只切换 dock 页签并设置焦点，
   没有离开欢迎页或显示 dock，查询被输入到不可见控件。

修复：模式切换时记录／恢复 Edit、Design 各自的左右工具页签，同模式不再重置；
欢迎页的可见性快照与用户隐藏状态仍保留。显式 Show/Find 菜单、命令和快捷键共用
`revealToolView`；后台刷新不主动弹出面板。搜索结果从 Design 跨到 Edit 时显式保留
Search，不被编辑模式之前的 Files 页签覆盖。该跨模式分支在检查中补充了单独测试，
随后重新冻结源码、重跑双平台门禁。

新增 SDK `GlobalSearchPanel.CanNavigateMatches()` / `NavigateMatch(previous)`：

- F4／Shift+F4、Edit 菜单、Command Palette 使用同一导航路径。
- 按文件／行／字节列识别结果，首尾循环，自动展开目标组并滚动到可视范围。
- 复用原有 Unicode 列转换、旧行验证与 Go Back 历史，不另起磁盘扫描。
- 空、等待、失效、已关闭、写事务和替换预览状态不导航；预览仍使用原有 Diff 动作。
- 只在 UI 线程调用；每次至多遍历已有上限 10,000 项的结果，不在 idle 中重建。

## 验证范围

原生构建与 race 回归：

```sh
GOMAXPROCS=2 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide \
  ./cmd/silktools ./internal/idetools ./internal/filetxn \
  ./filesearch ./internal/regexscan
```

- 严格 `go vet` 有基线既存 composite literal 警告。本轮通过 HEAD overlay 对比，
  警告集合完全相同（426 行，无新增），不把严格 vet 记作通过。
- 按仓库 CI 配置使用 `go vet -composites=false` 验证。
- 随包真实 gopls 集成测试覆盖未保存 Unicode 补全／格式化、重命名、
  类型／实现／定义跳转与导航返回；不以全局 PATH 上的 gopls 代替。
- 测试与 SDK benchmark 不等同于每帧／键盘到像素延迟保证。

## 最终结果

- macOS：**5,269 pass / 47 skip / 0 fail**。
- Windows 10 原生 cgo/race：**5,157 pass / 62 skip / 0 fail**。
- 两平台各 11 个含测试包通过；`internal/regexscan` 无独立测试文件，仍由 gui /
  filesearch 测试覆盖。不能把 skip 当作执行通过。
- 每个平台随包真实 gopls **3 pass / 0 skip**；原生 UI 的 gopls 子进程路径也确认
  位于本轮 QA 应用的 tools 目录。Windows 启动环境没有全局 gopls。
- 一万条结果连续 F4 导航 21 次后，光标为第 1 行第 41 列；搜索选中行随之滚动。
- macOS 静置 10.017 s，CPU 增加 0.090 s，约 **0.90% 单核**。
- Windows 静置 10.163 s，CPU 增加 0.125 s，约 **1.23% 单核**；20/20 次进程
  存活并 Responding。这是一次样本，不是通用性能上界。
- SDK 10,000 项导航 benchmark：约 **42.37 µs/op，0 B/op，0 allocs/op**（M2 Max）；
  不含原生绘制和磁盘开文件成本。
- 最终构建 macOS 5 份、Windows 12 份保存产物逐字节核对；所有工作区夹具恢复。
  Cmd+Q / Alt+F4 正常退出后本轮 IDE、gopls 残留为零，7 个 Windows 测试任务清理。
- 1,517 个本地编译输入、Windows 1,801 个不可变基线文件和 146 个 overlay 文件
  经前后 SHA-256 验证；最终提交前再次核对编译输入。

## 原生 UI 操作

隔离工作区：a.txt 为 `中🙂 foo_12 foo_34\n`，b.txt 为 `foo_56\n`，
ignored_test.txt 为 `foo_78\n`，dense.txt 为 12,000 个 `a `。

通过真实按钮、菜单和键盘：欢迎页搜索；点击第二个 Unicode 后命中；输入保存、
逐字节回读与 Undo 恢复；F4／Shift+F4、历史返回；模式切换与快速打开；
菜单的启用／禁用、替换预览隔离；密集结果连续跳转与静置 CPU 采样。

Linux 原生 GUI 未测。测试不覆盖本轮以外的全部菜单，不承诺任意规模文件或驱动的
延迟；工具页签记忆限本次运行，尚未做跨启动持久化。本轮不新增 Developer ID
签名、公证、安装器、发布标签或 Release。QA 使用 Go linker 的 ad-hoc 二进制；
签整个开发 app 的尝试因 tools/licenses 目录布局失败，未将此记录为发布签名通过。

## 证据位置

`bin/qa-artifacts/workbench-navigation-20261024/`：失败回归、冻结输入哈希、双平台
race JSONL、bundled gopls 日志、原生截图、保存回读、CPU、正常关闭记录。
`first-run/` 保留补充跨模式修复前的测试输入与证据，不与最终测试结果混计。

最终构建 SHA-256：

```text
macOS  76897d70345b922efd0a5f994522ae5f6b3c8bad93131ce28c379ffba9f30ddc
Windows 9bed07de579dd380b52ce32dbccb6b0b19013b7dc3aa8a97609fed8fa3de8c77
Windows evidence archive
445b8fbb77125aa0019b1be2f4667d596b203dabb5247537f075711a24b3448d
```
