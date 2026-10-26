# 原生字符输入与 SDK 入口 QA（2026-10-26）

本阶段在上一轮编辑器连续输入撤销修复之上继续；不代表整体 Qt Creator 功能对齐、
完整发布包认证或所有 Unicode 排版已经完成。已有未提交工作保留，不进行 commit / push。

## 本轮变化

- macOS 的文本控件支持 `Control+Command+Space` 请求系统 Emoji & Symbols。
  新增 `gui.CharacterPaletteAvailable()` 与 `gui.ShowCharacterPalette(target)`，
  宿主可复用，无新依赖、后台线程、定时器或轮询。
- 焦点必须已经归属于可见、启用、挂接的文本控件；窗口必须处于前台且
  Cocoa responder 确实是该窗口的文本输入视图。隐藏、最小化、后台及过期目标拒绝请求。
- 宿主显式快捷键与正在组合的 IME 优先。按键 Repeat/Release 不重复请求面板。
  真正提交仍走原生字符回调、输入校验和控件历史；不通过剪贴板假装系统输入。
- Windows/Linux 能力查询返回 `false`。真实 Windows 编译发现初版公共入口误用了
  GLFW 私有 `enabled` 字段，已改为跨平台 `Window.IsEnabled()`，再重新构建和测试。
- 系统面板是系统管理的 UI，不是新建的 Silk 浮动子控件。
  使用 Apple 的 [orderFrontCharacterPalette](https://developer.apple.com/documentation/appkit/nsapplication/orderfrontcharacterpalette(_:))。

## Unicode 自动化丢字的定位

上一轮 CUA `typeText("新🙂é 42")` 在旧版与新版都只保存为 `e 42`。
这轮临时诊断同时记录 AppKit 原始 keyDown 和 SDK `onChar`：

- AppKit 仅收到 `e`、空格、`4`、`2` 对应事件（及选择/保存快捷键）；
- SDK 收到同样的 U+0065、U+0020、U+0034、U+0032；
- 没有证据显示编辑器吞掉了已经交给它的汉字、Emoji 或组合附加符。
  **该注入路径的损失发生在 AppKit/SDK 输入分发之前**，不等于真实中文 IME 已通过。

诊断代码只用于独立 QA 二进制；源文件已恢复，最终 UI 验证使用无插桩构建。
实际系统字符选择器成功提交 🙂 和 🙂‍↕️，UTF-8 完整保存并可撤销/重做。

## macOS 验证

- 六核心包 `go test -race -short -json -count=1 -p 2`：**5882 pass / 46 skip**，
  含 18 个新功能测试事件（父测试与子测试分别计数，不是 18 个独立功能）。
- 定向输入/焦点/IME/字符面板用例 `-race -count=3` 通过。
- 原生 Objective-C eligibility 测试直接编译生产实现，覆盖成功请求和 10 个拒绝条件；
  后台线程使用真正 `pthread_create/join`，不假设 `dispatch_sync` 一定切换线程。
- 新快捷键测试在接入之前为红：三个文本控件均报告请求数 0；接入后通过。
- `go vet -composites=false` 通过；默认 vet 仍有 **449 个既有 unkeyed fields** 警告，
  与上一轮集合一致，没有新增。不能把默认 vet 说成全绿。
- 实际 AppKit UI：全选替换为 🙂、保存、一次撤销恢复原始文件、重做；
  再追加完整 ZWJ 序列并独立撤销/重做。恢复文件保留原始 CRLF/LF、Tab 与尾随空格。
  共 8 项文件回读断言；内嵌查找栏通过系统面板输入 🙂 后显示 **1 / 1**，文档不变。
- 5.015 秒空闲采样消耗 0.040 CPU 秒，约 **0.80% 单核**。
  这只是短时空闲样本，不是输入延迟、帧率或长时间稳定性的保证。
- bundled gopls 校验通过，测试 UI 显示 connected。QA app 没有打入外部 icon 目录，
  使用现有内置图标回退；该装配不代替正式安装包资源验收。

## Windows 验证

- Windows 10 真实构建及六核心包 `-race -short -count=1`：
  **5696 pass / 69 skip**；明确执行不支持平台的能力查询测试，结果为 `false`。
- 已校验不可变基线 1801 个文件和 501 个 overlay 文件；子进程沿用绝对路径
  `GOFLAGS=-overlay=...`，避免测试的是旧 SDK。bundled gopls 校验通过。
- Session 1 桌面验证：上下文菜单全选、连续输入保存、单次撤销/重做、工具栏
  Undo/Redo、保存边界、光标移动边界、`Ctrl+Space` 不污染纯文本；12 项文件回读。
  操作前验证 PID、二进制 SHA、窗口所有者与前台归属，菜单坐标来自本轮截图。
- 桌面脚本初次把空格写成 WinForms SendKeys 不支持的 `{SPACE}`，被 harness
  显式拒绝；改为该库实际支持的空格键表示后，从已还原文档重新执行全套并通过。
  失败日志保留，不将测试脚本语法错误归因为 SDK。
- 最终采样时窗口持续 Responding；5.018 秒空闲采样消耗
  0.0156 CPU 秒，约 **0.31% 单核**。同样不构成长期性能保证。
- 四个交互任务均回读 `Ready / Result 0` 与各阶段专属 marker；QA 程序与 bundled
  gopls 已关闭，基线、overlay、实际运行二进制和脚本哈希复核通过。

## 已知缺口与下一轮优先项

1. **复杂 Emoji 的渲染仍有缺陷**：🙂‍↕️ 的字节与历史完整，但截图显示被拆成
   Emoji、箭头和占位框，尚未正确按字素簇整形；不能将本轮输入通过说成排版通过。
   下轮从 `gui/codeeditor.go:drawHighlightedLine` → `paint/painter.go:DrawText`
   → 字体回退／glyph 转换链路验证，不通过删掉 ZWJ/变体选择符来掩盖。
2. GLFW Cocoa 的候选框坐标实现仍未接入 SDK 已有插入点几何。本轮没有修改它；
   后续应按 Apple 的 [firstRectForCharacterRange](https://developer.apple.com/documentation/appkit/nstextinputclient/firstrect(forcharacterrange:actualrange:)?language=objc)
   屏幕坐标契约补齐，并做真实中文组合、候选窗口与多屏验证。
3. CUA 对系统 Character Viewer 的截图/焦点获取有间歇 timeout；先点回目标字段后可请求并提交。
   搜索汉字仅看到结果，没有得到稳定提交闭环，**不记为通过**。
4. 未测试 Linux 原生桌面、Windows 高 DPI、全部语言输入法及全部控件矩阵。
   macOS 原生面板能力不扩张为 Windows/Linux 功能支持承诺。

## 可复核证据

`bin/qa-artifacts/native-input-20261026/`：

- `source-hashes.json` / `phase-source-changes.json`：1755 个代码输入，10 个本轮改变文件；
- `trace-final-ide.stdout` / `trace-final-ide.stderr` / `trace-final-result.txt`：原始事件与保存结果；
- `palette-before-tests.log` / `palette-targeted-tests.log` / `mac-tests.jsonl`：红绿与 race 回归；
- `mac-emoji-*.txt/png` / `mac-zwj-*.txt/png` / `mac-final-restored.txt/png`：原生提交和文件历史；
- `mac-find-native-emoji.png`：查找栏匹配；`mac-find-document-unchanged.txt`：文档未修改；
- `ide-process.json` / `mac-close.json` / `mac-idle-cpu.json`：最终二进制、关闭和短时 CPU；
- `verify-local.py` / `local-verification.json`：macOS 一致性检查；
- `windows-evidence/`：真机单测、四阶段任务结果、菜单截图、12 项文件快照及关闭证明；
- `artifact-hashes.json` / `verify-final.py` / `final-verification.json`：源码—构建—测试—UI—文件回读总验证。

最终 macOS QA binary SHA-256：
`41817a3e990447707ea96f501f8ed9acadc0f8704e8aed79fd4df5641413ae82`

最终 Windows QA binary SHA-256：
`57fd5fa8096f3857b9c308678cd558acfea3a3af1bf73a857c728d861a5ee678`

复核命令（在仓库根目录）：
```sh
python3 bin/qa-artifacts/native-input-20261026/verify-final.py
```
