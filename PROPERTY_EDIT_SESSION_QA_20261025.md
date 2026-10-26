# 属性面板实时编辑与事务撤销（2026-10-25）

## 根因与修复

上一轮 Windows 实机发现：把整数属性 `-42` 逐字改成 `-57`，一次设计器 Undo
只返回中间输入 `-5`。旧属性层每次 `OnEdited` 都推入独立 `PropertyCommand`；
文本 Enter／失焦还可能重复提交。不是整数恢复层或控件绘制导致。

本轮只改变 5 个产品源码文件并新增 1 个测试文件，没有覆盖其他未提交工作：
`prop/property.go`、`prop/textedit.go`、`prop/intedit.go`、`prop/floatedit.go`、
`prop/edit_session.go`、`prop/edit_session_test.go`。

- 为字段建立本地编辑 session，接入既有 `UndoStack.MergeWidth` 协议。
  仍逐次更新模型，不用延迟提交伪装成正确 Undo；保留第一条命令的各对象初值、
  最后请求值与 setter 实际读回值，重复输入不保留整串中间历史命令。
- Enter、真实失焦、刷新、隐藏、禁用、只读切换、重绑定与历史重放结束当前段。
  字段自己的上下文菜单不切断连续编辑，Up/Down 连发在松键时结束。
- 沿用 UndoStack 保存点与分支边界；发现外部模型变化时不吞并该变化。
- 无变化的提交不写模型，多选保留各对象初值，已退役／禁用／隐藏／只读行不写回。
- 不增加原生窗口、goroutine、轮询或绘制定时器；UI 对象仍须在 UI 线程访问。

## 自动测试

```sh
GOTOOLCHAIN=go1.26.10 GOMAXPROCS=2 SILK_TEST_FONT_FALLBACK=1 \
  go test -race -short -count=3 ./prop
GOTOOLCHAIN=go1.26.10 GOMAXPROCS=2 SILK_TEST_FONT_FALLBACK=1 \
  go test -race -short -count=1 -p2 \
  ./prop ./gui ./ged ./cmd/silkide ./decl ./examples/spinbox-edit
go vet -composites=false ./prop ./gui ./ged ./cmd/silkide ./decl
```

首批回归先在修复前执行并失败，日志保留。新增测试涵盖整数／浮点／文本连续输入、
混合多选、Enter／失焦／刷新／隐藏／只读边界、保存点、历史分支、setter 归一化、
外部模型修改、无效旧行、禁用父节点、字段自有菜单、长按步进与不同属性行隔离。
200 次字符输入仍有 200 次实时回调，但只保留 1 条场景撤销记录。

macOS 六包 race/short：**5631 pass、46 skip、0 fail**（含子测试）。
`prop` race 重复 3 次、vet、IDE 构建均通过。

## 原生 UI 文件级验收

两端使用隔离工作区、QA 二进制、初始范围 `-100..100` 的 SpinBox。每一步保存后
比较整个 `.silkui` 文件，不用截图、按键发送成功或工具退出码代替结果断言。

| 操作 | 保存 | 一次撤销 | 重做 |
| --- | --- | --- | --- |
| 整数逐字输入 | `-42 → -57` | `-42` | `-57` |
| 后缀逐字输入 | `" items" → " units"` | `" items"` | `" units"` |
| 宽度逐字输入 | `85 → 65.5` | `85` | `65.5` |
| 同字段中途保存 | `60` 保存，再输入 `.5` 为 `60.5` | `60` | `60.5` |
| 离开字段再编辑 | `-50` 失焦，再改成 `-43` | `-50`，再撤销为 `-57` | 两次回到 `-43` |

macOS 上述 17 次完整文件断言均通过。正常退出再重启，属性栏和画布读到
`-43 / " units" / 60.5`。两个 QA 进程正常退出。5.004 秒静默样本仅耗
0.04 CPU 秒（单核约 0.80%）；这是短时观察，不是长期性能或帧率保证。
原生保存生成的 `.silk.go` 原样放入独立 SDK 消费模块，race 测试通过，检查了
值、范围、步长、后缀、只读与内部逻辑子控件没有独立窗口。

![macOS 重启后的完整属性](docs/images/property-session-macos.png)

## Windows 真机

Windows 10 amd64 使用同一源码 overlay 470 个文件，原始基线 1801 个文件；
构建前后及收尾再次核验源码、overlay、已测输入清单和二进制 SHA。

- 专项 race 连续 3 次通过。`decl / ged / gui / prop / cmd/silkide` 五包完整
  race/short 回归：**5471 pass、69 skip、0 fail**（含子测试）。
- 上表同样的 17 次完整文件断言全部通过；这次真正重跑了旧失败场景，
  `-57` 一次 Undo 恢复 `-42`，不是绕过撤销的恢复脚本。
- 正常退出再启动，字段全选／复制读到 `-43 / " units" / 60.5`。
  原生保存的 `.silk.go` 与 macOS 独立 SDK 运行验证的代码逐字节相同。
- 6 个独立交互式计划任务均 `Ready / Result=0`，脚本 SHA、阶段专属输出
  marker、桌面、PID、二进制和输入窗口归属均有记录；不能仅由 SSH 退出码认定通过。
- 5.018 秒静默 CPU 0.046875 秒（单核约 **0.93%**），`Responding=true`。
  主程序与本轮 bundled gopls 均正常退出，收尾未发现残留 QA 进程。

![Windows 重启读回三类属性](docs/images/property-session-windows.png)

### 新观察到的待修细节

修改宽度后，属性栏与控件尺寸已变为 `60.5`，但 Windows 底部仍显示选中时的
`85×12 mm`；重启重新选中后变为 `60×12 mm`，小数仍被取整。分别见阶段产物
`windows-boundaries-verified.png` 与 `ui-evidence/ide-reloaded.png`。
这是状态栏刷新／格式问题，不能因为本轮撤销用例通过而称其已经修复，后续继续处理。

## 证据与范围

完整输入、修复前后日志、源码 SHA、逐步保存文件、生成代码、原生截图与计划任务
脚本在 `bin/qa-artifacts/property-edit-session-20261025/`。
上一轮失败任务与 `-5` 的保存证据保留在旧阶段目录，未覆盖或改写为成功。

不宣称已达到发布条件：Linux、32 位、Windows 高 DPI 未在本轮实机测试；
预览输入焦点下的 Escape 语义、多窗口快速操作后的编辑位置问题仍待处理。
Windows 既有 vet 问题不在本轮清零范围。macOS QA app 的外置 icon 目录提示
仍保留，当前界面使用既有资源回退；这不等于发行包资源审计通过。
Windows QA 组装目录也保留了 `property/ged.cml` 缺失并回退 default 的日志；
不据此宣称发行包资源完整。当前 SDK 与设计器源码共 1733 文件 SHA 核验通过，
完整验证器结果为 `PROPERTY_SESSION_SOURCE_TEST_NATIVE_VERIFIED`。
本轮未 commit、push 或发布。
