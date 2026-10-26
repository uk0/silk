# 嵌套菜单关闭与键盘焦点 QA — 2026-10-25（10-26 收尾）

本轮保留既有工作树，未 commit、push 或发布。源码增量为 **2 个实现文件和 1 个测试文件**。

## 真实复现与根因

上一轮曾观察到菜单执行后 Save 快捷键失效。本轮先用未修改的已验收二进制复现：

1. 选中三个 SpinBox，打开右键菜单，鼠标展开“对齐”，鼠标点击“左对齐”。
2. 画布／属性已对齐，直接 Cmd+S，窗口仍带 dirty，完整保存文件仍是未对齐基线。
3. 同一基线下，纯键盘 Home／Down／Right／Enter 或鼠标打开后键盘 Enter 执行，
   Cmd+S 正常。这将问题缩小到“深层原生弹窗取得系统焦点后的关闭顺序”，不是
   笼统的菜单键盘导航全部失效。

`before-ide.stderr` 的现有 debug 日志显示：当前 key window 是二级菜单；一级菜单
准备焦点交还时只检查自己和 Form，错误地判为 `native-family-unfocused`。关闭后代时，
二级菜单先激活一级菜单，接着一级菜单立即隐藏，主 Form 留在没有原生键盘焦点的状态。
`before-mouse-leaf-save.silkui` 与 `before-mouse-leaf.png` 保留未保存文件／dirty 窗口证据。
旧二进制 SHA-256：`c66242c62253feaf040ca0cafbb394f60fc04339034dd1996942a1c6501f7319`。

## 修复

- `transientSourceFocused` 将当前 popup 的原生后代计入焦点来源，仅沿 transient owner
  链查找，不穿过独立 Form 边界。
- 在 `Window.setVisible(false)` 隐藏后代之前，直接把原生焦点交给仍存活的 owner。
  不再走“子菜单 → 马上要隐藏的父菜单 → 无 key window”的路径。
- 保留应用激活、owner 生命周期、generation 和新焦点意图保护；逻辑焦点／capture
  仍由既有清理路径恢复，不把 action 提前到菜单隐藏之前。
- 仅改变 GLFW 后端；没有修改 Windows 实现或公共 API，也没有添加后台线程、
  轮询、计时器、无条件激活窗口或全局键盘钩子。

## 自动测试

证据目录：`bin/qa-artifacts/menu-keyboard-20261025/`。

- 旧实现上的新增测试编译成功，1／2／4 层嵌套焦点交还断言失败；另五个新焦点意图
  场景也因根 popup 无法识别焦点来源而无法建立待验证返回。日志在 `before-tests.log`。
- 新测试覆盖多层链、先交还再关闭、逻辑焦点恢复、独立 Form／其子菜单隔离、
  其他应用、其他窗口、新弹窗、新 generation 和 owner 禁用。
- 针对性 popup／menu／modal／focus／shortcut 回归 `-race -count=3` 通过。
- macOS 七包 `go test -race -short -json -count=1 -p 2`：
  **5833 pass / 46 skip / 0 fail**；六核心包 vet、实际 bundled tool 校验及原生构建通过。
- 统计为选定包的测试／子测试事件，不是全仓库、所有平台或发布包认证。

## macOS 原生闭环

修复版 SHA-256：`5421fbd39e01fabcf7cc31c03578d2de5a89445985e64f2a656ac8375a4fef30`。
无 debug 绘制、无新增 instrumentation，以同一 fixture 和原生菜单重新验证：

- 三轮完整鼠标子菜单左对齐后直接 Cmd+S，完整文件均正确；不点 Save 按钮、
  不额外点击画布。第一轮还直接 Cmd+Z／Cmd+Shift+Z／Cmd+S 验证历史与保存。
- Home／Down／Right／Left／Right／Enter 保持子菜单导航；执行后直接保存与撤销。
- 鼠标展开子菜单后 Escape 取消，立即 Right 微移三个选中控件并保存；Undo 恢复基线。
- 正常退出并以新 PID 重载，验证微移文件和属性；恢复基线后再次完整鼠标菜单
  操作、直接保存及撤销，证明不是依赖前一进程的偶然焦点状态。
- **17 个设计器文件断言**和对应截图；另有编辑器右键“全选”后直接输入／保存
  的文件验证。编辑器撤销分组细节见限制，不把一次 Undo 的失败包装成通过。
- 5.018 秒空闲样本 CPU 累计增量约 0.05 秒，约单核 **1.0%**。仅是短采样，
  不用于宣称指定 FPS、输入延迟或长时性能保证。两个修复版 PID 均正常退出。

## Windows 兼容回归

Windows 10 22H2 真机同源 overlay 原生构建、实际 bundled tool 校验、针对性
`-race -count=3` 与六核心包完整回归通过：**5662 pass / 69 skip / 0 fail**。
二进制 SHA-256：`262622fdddb0b5826d0170ca1f909793d0c0112e1dd132a78babe30311f1f058`。
与上一轮 Windows 二进制相同，符合本轮只修改非 Windows 实现的预期；这里做的是
重新构建、重新测试及新桌面会话的兼容验证，不宣称修复了 Windows 上不存在的同一根因。

- 三轮鼠标“右键 → 对齐 → 左对齐”后直接 Ctrl+S；Ctrl+Z／Ctrl+Y／Ctrl+S
  验证撤销重做，不用 Save 按钮或额外画布点击。
- Home／Down／Right／Left／Right／Enter 导航并执行，直接保存；Escape 关闭
  子菜单后 Right 微移三项，保存并撤销。**13 个完整设计文件断言**和对应截图。
- 最终从实际字段复制核验 `15.25 / 20.75 / 60.5 / 12.125`，文件恢复基线。
  5.008 秒静置 CPU 增量 `0.015625` 秒，约单核 **0.31%**，`Responding=true`；
  仍只是单次短样本，不等于 FPS 或长期性能保证。
- 4 个交互任务均验证 `Ready / Result=0`、阶段 marker、脚本 SHA、二进制／PID、
  前台及点击所属进程，没有重叠操作。正常关闭后 QA app／packaged tools 进程为 0。
- 收尾重新校验远程 **1801** 个基线输入和 **490** 个 overlay 输入未变；本地
  **1745** 个源输入与构建／测试一致。未复用上一轮的通过 marker。

## 截图与复核

![macOS 新进程鼠标菜单操作后直接保存](docs/images/menu-keyboard-macos.png)

![Windows 鼠标菜单操作后直接保存](docs/images/menu-keyboard-windows.png)

复核命令：`python3 bin/qa-artifacts/menu-keyboard-20261025/verify-final.py`。
阶段标记为 `MENU_KEYBOARD_SOURCE_TEST_NATIVE_VERIFIED`；源码／文档 diff、失败记录、
测试事件、实际保存文件、脚本与截图哈希均在阶段目录。

## 已知边界与后续工作

- 原生菜单截图可能从主窗口切换到单独 popup 坐标系。修复版首次自动化在尚未确认
  子菜单展开时点击了画布，文件断言失败且未产生对齐操作；随后改为逐级观测菜单
  展开再操作。保留记录，不将未命中操作算作通过，也不据此改变产品行为。
- 编辑器“菜单全选 → typeText('Menu focus 2.75') → 保存”正常，但一次 Undo 仅
  回到 `M`，第二次才逐字节恢复原 Unicode／混合换行文本。该现象需要下一轮
  单独检查选择替换与连续输入的历史合并；本轮不声称修复了编辑器全部 Undo 分组。
- 本轮对原先泛化的“子菜单键盘问题”做了收窄：正确 Home／Down／Right 导航本来
  就可用；已修复的是鼠标取得深层原生焦点后整链关闭丢失 owner 焦点的路径。
- Linux、32 位、Windows 高 DPI、多窗口长时压力和全部控件未覆盖；已有 Preview
  Escape／快速多窗口编辑器焦点问题不能由这条菜单修复推断为已解决。
- QA 专用 app 未装配所有可选翻译／素材，不替代完整发布包验收。
