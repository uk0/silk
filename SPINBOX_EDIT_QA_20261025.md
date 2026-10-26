# 整数输入、单位绘制与设计器联动 QA — 2026-10-25

本轮是开发工作树上的一个增量，不是发布认证，也不代表全部 UI 已达到 Qt Creator 功能对等。
保留此前工作树改动，未 commit、push 或发布。

## 已修复和新增

- `SpinBox` 原先没有直接文本输入路径；现在拥有一个逻辑 `Edit` 子控件，支持带符号整数、
  选区、剪贴板、字段内 Undo/Redo、Enter／失焦提交与取消。`Value()` 始终表示已提交值。
- 上下箭头、滚轮和分页步进在机器整数边界饱和，不再从正上限绕回负数；
  自动分页步长不会因乘法溢出变为负数。重复设置相同规范值不重排文本。
- 子控件使用父控件坐标和裁剪范围，不创建独立 native window；自动 Tab 遍历只占一个位置。
  极小尺寸不产生负宽高，控件之外的点不会误命中箭头。
- 深浅主题使用统一的正常／悬停／按下／不可用颜色，箭头对比度加入测试。
- 原生截图发现带前导空格的单位 `" items"` 尾部被裁切。原因是绘制原点没有补偿
  `TextExtents.XBearing`；增加 ink bounds 回归，先复现失败再修复，并重新构建测试。
- 设计器补齐步长、分页步长、后缀、只读的保存／重载／Go 生成；自动分页模式保持为 0。
  现有打印份数和甘特图进度编辑器复用此控件，增加真实内部表单的提交和 Undo 单测。
- 新增 `examples/spinbox-edit`，用于整数边界、主题、禁用、模态输入和 JSON 状态回读。

涉及 `gui/spinbox.go`、`gui/spinbox_edit.go`、`gui/spinbox_edit_test.go`、
`ged/codegen.go`、`ged/spinbox_design_test.go` 和示例的两个 Go 文件。

## 修复前证据

候选目录：`bin/qa-artifacts/spinbox-edit-20261025/`。

1. 单测复现无文本输入路径、整数步进绕回以及控件外误命中。
2. macOS 旧版示例实际键入 42 后仍为 10、回调次数为 0；点击极值旁的上箭头后得到
   `-9223372036854775806`。正常退出后核对 PID 和二进制 SHA-256。
3. 首个候选版本的截图和 `before-suffix-regression.log` 证明单位末尾裁切；该候选的
   测试结果不冒充最终修复版本的验收结果。

## 最终源码与 macOS 验证

最终证据：`bin/qa-artifacts/spinbox-edit-final-20261025/`。

- 固定 1,728 个源码输入，macOS 1,715 个输入及 Windows 463 个 overlay 输入；
  相比 timer 阶段恰好 7 个源码文件变化。记录源码、可执行文件、测试、截图和文件回读。
- Go 1.26.10，12 个包 `-race -short -count=1`：**5,653 pass / 47 skip / 0 fail**。
  根包 race、12 包 vet、IDE 必须携带工具的构建、SDK 构建、5 项强制原生 cursor/GL 测试通过。
- 相关 GUI／设计器／示例的定向 race 测试连续三次通过；后缀边界回归覆盖两种主题和窄宽度。
- SDK 从干净进程验收，读取 16 份 JSON 状态：输入 42、一次局部撤销、Tab 提交 -17、
  999 夹取为 100、不完整负号／Escape／非法粘贴不改模型、禁用拦截、最大／最小整数饱和、
  模态 Enter 不关闭、Apply 返回 27、下一次 Escape 取消仍保留 27。
- 真实 IDE：属性 42 改为 56 保存；拖动坐标 `[15 15 85 12]` → `[25 25 85 12]`，
  Undo 恢复原坐标；内部编辑器未泄漏进设计对象列表。预览输入 60、Up 得到 67，验证步长 7。
- 将 UI 保存生成的 `geometry.silk.go` 原样复制到独立临时模块，race 测试实际 BuildWithIndex，
  回读值 56、步长 7、分页 28、后缀和只读状态，确认只有一个公开对象 ID。
- Edit → Select All、工具栏保存后继续输入、行列定位、About 返回均有文件内容回读。
  干净工作台得到 `SP + 原文` 和 `A + 原文`，Undo 恢复原始混合换行及 UTF-8 字节。
- 5 秒短样本：SDK CPU 约单核 0.8%，IDE 约 1.0%。这不是长期性能基准或帧率承诺；
  本轮没有增加定时器、线程或后台轮询。

## Windows

Windows 10 22H2、amd64、真实交互会话 1，Go 1.26.10 / UCRT64：

- 7 个包的定向 race 测试连续三次通过；GUI 全量 race-short 为
  **2,741 pass / 21 skip / 0 fail**。
- `go vet` 仍有 15 条既有 Win32 诊断，与前一 timer 阶段逐条比较无新增；不是 clean vet。
- SDK 16 份原生 JSON 状态与 macOS **逐字段完全一致**，包括准确的 int64 极值；
  Enter 留在模态输入框，Apply 和 Escape 返回状态正确。
- IDE 实际 About 返回、保存后继续输入、Ctrl+Tab、页签拖动、Edit → Select All、
  Go to Line、dock chooser 和原生边框缩放均通过；恢复文件 SHA-256 及混合换行逐字节一致。
- 7 个交互任务均为 Ready / Result 0，实际任务脚本哈希与归档一致；三个 GUI 进程正常关闭，
  无本轮 bundle 的 gopls 残留。固定基础源码 1,801 个文件及 overlay 463 个文件回读未变。
- 5 秒短样本 SDK CPU 0.109375 秒（约单核 2.18%），IDE 0.09375 秒（约 1.87%），
  均 Responding；短样本不等于持续性能或高 DPI 验收。
- 第一次跨任务接续模态检查被前台 owner 守卫阻止，尚未输入按键；显式验证并激活已记录的
  模态 HWND/PID 后复验通过。失败日志保留，没有绕过 owner 检查。

首轮代码生成测试失败：父 `go test -overlay` 的命令行选项不会自动传给测试启动的
`go vet` 子进程，子进程误用旧 SDK 并报告 `SetReadOnly undefined`。最终 QA 脚本通过
`GOFLAGS=-overlay=<绝对路径>` 让子进程使用相同源码；保留失败日志，不更改或跳过断言。

## 边界与后续

- 输入按 Edit 事件保存局部历史，逐字符键入 `12` 后一次 Undo 只撤销 `2`，不等于整组替换。
- macOS 自动化一次 `typeText("2:5")` 未投递冒号，真实 UI 显示无效位置且没有改正文；
  用原生粘贴精确投递后，定位和保存回读通过。不把自动化输错当成成功导航。
- 同时保留预览和主窗口时，一次快速连续操作出现保存后后续字符落到末尾；无预览的干净
  工作台分步复验通过。保留两种文件结果，尚未归因，也不宣称多窗口焦点所有时序已解决。
- 预览中的输入框会消费 Escape 来取消草稿，本次未把它计为预览关窗成功；后续需要统一
  预览容器和子输入框的快捷键优先级。通过系统 Window 菜单返回主工作台，最后正常退出进程。
- 示例模态将 SpinBox 直接作为内容，故意拉伸其边界以验证裁剪，不是业务表单排版模板。
- 额外负值探针发现旧的设计器属性加载顺序问题：范围 -100..100、值 -42 经保存重载后
  变为 0。用本轮修改前的 SpinBox／代码生成源码 overlay 也得到相同结果，确认是既有问题，
  不是本轮回归；尚未修复。正常正值 42／56 的回读成功不能覆盖这一边界。后续优先处理。
- Linux 原生桌面、Windows 高 DPI／多显示器没有在本轮认证。浮点数、区域化数字和
  全部 Qt 输入语义不在本次实现范围。

## 重放入口

```sh
GOTOOLCHAIN=go1.26.10 GOMAXPROCS=2 SILK_TEST_FONT_FALLBACK=1 \
  go test -race -short -count=3 ./gui ./ged ./examples/spinbox-edit \
  -run 'Test(Spin|BindSpinBox|PrintCopiesSupports|GanttProgressTyped)'
go run ./examples/spinbox-edit
```

SDK 操作顺序和精确整数结果见 `mac-sdk-verified-states.json`；完整冻结源码、构建命令、
Windows 脚本和未归因现象位于最终证据目录。不要使用旧候选版本的可执行文件替代最终版本。

## 画面证据

![整数输入与完整单位后缀](docs/images/spinbox-edit-macos-light.png)

![深色主题的可用／不可用箭头](docs/images/spinbox-edit-macos-dark.png)

![设计器中完整的数值属性](docs/images/spinbox-edit-designer-properties.png)

![Windows 深色整数控件](docs/images/spinbox-edit-windows-dark.png)

最终验收器 `verify-final.py` 已通过，标记为
`SPINBOX_SOURCE_TEST_NATIVE_FILES_VERIFIED`：254 份交付证据 SHA-256、冻结源码、
两平台状态与文件字节、任务脚本和进程退出状态一致。验收结果仍显式保留上述未解决项，
不将其解释为整个产品无缺陷或可以发布。
