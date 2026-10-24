# Windows 大文件保存时序调查（2026-10-23）

本记录补充 `EDITOR_LEXICAL_QA_20261023.md` 的未通过项，不是修复完成报告。

## 已复现的边界

- 原始未插桩 IDE SHA-256 `e47fb8d12e1f385daa39b3d062a69745a5e7fa29a5ec6e54390a6055ad30cb20`。
- 独立工作区、独立 APPDATA/SILK_DATA_DIR、标准化 4,000,030 字节 Go 文件。在词法编辑/撤销重做、Tab 片段流程之后，向文件底部发送 16 个 `WM_CHAR`，接着 Esc 和 `SendKeys Ctrl+S`。
- 干净的未插桩 A/B 重放：首次快捷键后 20 秒内内容比较仍不匹配，第二次 Ctrl+S 成功。记录中的 `KeyboardSaved=false` 不能被后续重试成功覆盖。
- 这是混合 Win32 消息/SendKeys 自动化证据，尚不足以证明纯物理键盘具有同样失败；也不能把失败直接归因于 gopls、焦点或消息公平调度。

## 本次定位方法

使用 `go build -overlay`，原始源代码、原始可执行程序和派生跟踪程序分开保存。生产输入路由、消息泵及保存实现未修改。

1. 磁盘跟踪记录原生键消息、修饰键、快捷键匹配、文本版本及保存进入/写入/返回；简化流程三轮和完整流程均成功。
2. 改为 UI 线程上的 4,096 项固定内存环，运行时不逐键写盘，事后专用消息导出；完整流程同样成功。
3. 内存记录的成功案例：Ctrl+S 识别为 `key=83, mods=1, bound=true`，进入保存时文本已为 revision 17 / 4,000,046 字节。同步格式化及写入约 947 ms。

这些日志会改变执行时序；**插桩成功不是修复证明**。同步 gofmt 的耗时是另一个响应性调查线索，但未证明它导致首次保存失败。

## 产物与清理

- 本地：`bin/qa-artifacts/editor-save-20261023/`；远端：`C:\silk\qa-editor-save-20261023-codex`。
- `diagnostic-evidence.tar.gz` 保存 trace、完整 trace、未插桩 baseline、内存 ring 四组输出及失败日志。
- `trace-overlay.json`、`ring-overlay.json`、`trace-base-hashes.json`、构建/桌面脚本可用于重放；基础源输入 1,801 项 SHA-256 已核对。
- 初次关闭脚本误用尚未采集的 gopls PID 导致清理失败；保留该错误，随后按实际父子进程及精确路径重新关闭。两组 trace、baseline、ring 的 IDE 和随包 gopls 均已正常退出，最终清理 JSON 已回读。

## 当时的下一步（下文已推进）

以未插桩程序加入纯原生输入对照，取得失败时的文本版本/保存分支证据，再决定是否调整事件路由。当前不改消息公平策略，不将该项算作 UI 全绿，不以重试或工具栏保存掩盖第一次失败。


## 后续调查：读取观察者也会阻止 Windows 保存

此前“首次 Ctrl+S 不稳定”只能视为测试现象，不能继续作为 IDE 丢键或消息泵缺陷的证据。本轮定位到 QA 脚本的一个确定干扰源：它每 50 ms 调用 .NET `File.ReadAllText` 回读 4 MB 文件，默认读取句柄使用 `FileShare.Read`，没有允许其他进程写入。在 Windows 上，读操作本身就可能让 `os.WriteFile` 得到共享冲突；反过来，写入进行中时该读取也可能失败。

### 对照实验

使用上一轮未插桩的同一可执行文件，SHA-256 为 `accb129b87538b9bf983dc3f9681823a705464f2a2944482f2a81c126668edc0`，独立工作区和应用数据，保留词法编辑、保存/撤销/重做和 Tab 片段前置流程。

1. 原读取方式：混合 `WM_CHAR` + `SendWait` 与单次 `SendWait(文本 + Esc + Ctrl+S)` 共 6 轮，首次只通过 2 轮；4 轮未通过时缓冲区逐字正确、磁盘未更新，第二次保存恢复。这里的 `native` 标签仍是 WinForms 合成输入，不是物理键盘。
2. 最小整数环形探针：不逐键写盘、不增加 `GetKeyState` 或时钟读取。前三轮保存成功，第四轮测试读取自身抛出 `IOException` 而中止；随后导出的分支记录显示该轮实际已走到成功写入（revision 49 / 4,000,078 bytes）。不能把这个中止说成 UI 通过或 IDE 保存失败。
3. **只改变读取句柄的共享标志**为 `FileShare.ReadWrite | FileShare.Delete`，保持同一未插桩程序、同样的六轮输入流程：6/6 首次保存且完整内容回读一致。
4. 再使用直接 Win32 `SendInput`，把文本按键、Esc、Ctrl+S 放入同一批次，不经过 WinForms `SendKeys`：6/6 首次保存且完整内容回读一致。这仍是系统注入，不是物理键盘或输入法认证。
5. 受控持有读取句柄：禁止写共享时首次保存不更新磁盘，缓冲区保留，释放句柄后保存恢复；允许读写/删除共享时，即使读取句柄仍然打开也能第一次保存成功。唯一变化是共享标志。

这组证据证明旧的读取观察者会制造保存失败，足以撤销“已经证明快捷键丢失”的推断；它并不证明任意机器、输入法、负载和外部文件占用下都不存在其他保存问题。不调整消息公平调度，也不通过自动重试掩盖保存错误。

### 本轮代码增量

- `cmd/silkide/main.go`：写入失败时除原有 toast，再保留系统错误日志 `editor save failed:`，不记录编辑器内容。保存行为和返回值不变。
- `cmd/silkide/editor_save_windows_test.go`：Windows 原生文件共享回归，断言确为 `ERROR_SHARING_VIOLATION`，失败时文本版本、未保存状态和磁盘原文保持不变，错误提示存在；释放句柄后恢复，合作式读者不阻止保存。测试使用 Unicode 文本，不依赖 gofmt 或外部 LSP。

### 重放与边界

本轮原始/派生/最终产物位于 `bin/qa-artifacts/editor-save-native-20261023/`，远端为 `C:\silk\qa-editor-save-native-20261023-codex`。`evidence` 保留旧读取方式失败，`probe-evidence` 保留探针及读取异常，`shared-evidence` 是同一原始程序的观察者对照。`final-overlay.json` 只替换正式日志增量及新增测试；原始冻结源码不变，构建前核对 1,801 项基础输入和 2 项增量 SHA-256。

大文件每字符编辑、同步 gofmt、六位行号和 minimap 布局仍是独立待优化项；本轮不能称为低延迟或所有 UI 全绿。没有修改系统 PATH，没有安装全局 gopls，没有 commit/push/发布。


### 最终正式增量验证

- macOS：`cmd/silkide`、`internal/idetools`、`cmd/silktools` 三包 `-race -short -count=1`：**411 pass / 6 skip / 0 fail**；选定编辑器词法/保存状态/修饰键回归 **22 pass / 0 skip / 0 fail**。
- Windows 真机：相同三包 **407 pass / 13 skip / 0 fail**；选定 Win32 键路由、消息公平性、编辑器状态回归 **26 pass / 1 skip / 0 fail**。Session 0 中需要可见 HWND 的 `TestWindowsKeyLayerNativeShortcutRepeatAndFocus` 跳过，没有将它计入通过；真实键输入由 Session 1 UI 流程另行验证。
- 新增 `TestEditorSaveWindowsReaderSharing` 两个子用例在 Windows race 下均通过，包括实际 `ERROR_SHARING_VIOLATION`，不是 mock。
- `go vet -composites=false` 三包和 `git diff --check` 通过。未声明全库或全部 vet 选项通过。
- 最终正式程序 SHA-256：`013b74ca27843a972e5be726209559fcd0ef59dee9eee117638f0d7454f51de8`。它没有诊断环形插桩，只有正式的失败日志增量。
- 此程序重新执行完整 Windows UI 前置流程：词法编辑保存/撤销/重做、Tab 片段保存/撤销/重做；合作式读取下原六轮输入 **6/6 首次保存成功**，Win32 `SendInput` 六轮 **6/6 首次保存成功**，均逐字回读磁盘。
- 最终程序再次执行持有读句柄 A/B：禁止写共享时报错、保留缓冲区、释放后保存恢复；允许写共享时正常保存。UI 截图中错误 toast 和未保存标记可见；日志明确记录 `The process cannot access the file because it is being used by another process.`，不再依靠 toast 消失后的猜测。
- 正常关闭最终 IDE PID 6860 及随包 gopls PID 6124，关闭回读通过。保留全部旧读取失败、探针读取异常及最终通过记录，不覆盖原始结果。

- 另外从最终代码执行真实随包 gopls 集成：macOS / Windows 各 **3 pass / 0 skip / 0 fail**，覆盖未保存 Unicode 补全/格式化、重命名后的定义、类型/定义/实现跳转与返回。short 回归中的 LSP skip 不替代这组真实测试。
- 最后按本轮精确可执行路径重新枚举，IDE、随包 gopls 及其同路径派生进程剩余 **0**。最终原始测试归档 `final-evidence.tar.gz`，四组 UI/探针归档 `ui-evidence.tar.gz`，汇总 `summary.json`，本轮代码差异 `increment.patch`。

后续行号、正文裁剪及 minimap 的修复与真实 UI 验证见 `EDITOR_GEOMETRY_QA_20261024.md`；每字符处理与同步 gofmt 仍待继续优化。
