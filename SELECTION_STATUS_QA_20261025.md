# 设计器选中状态与画布历史 QA — 2026-10-25

本轮是工作树增量，不是发布，也没有 commit / push。历史遗留修改未重置。

## 修复与根因

1. SDK 选择回调结束时覆盖宿主状态文字，且仅在换选区时更新；尺寸被格式化为整数。
   现在共用格式化入口并监听已提交设计变更，显示对象名、控件类型、坐标和小数尺寸。
2. `FakeWidget` 继承的 Title 暴露内部适配器名称；现在先识别设计控件的 factory 和 object name。
3. **实机发现**画布内 Undo/Redo 直接调用 UndoStack，文件及画布已恢复但属性栏／状态栏
   仍是旧值，代码生成也错过通知。现在保留 action 状态而将执行路由到派生视图的生命周期。
   回归测试直接触发实际 canvas action，而非仅调用 `GedView.Undo()`。
4. 状态监听仅在可见宿主及状态栏存在时建立；切场景和关闭时移除。保留了原有
   `TestCodePanelRebindRetiresSubscriptions` 的严格断言，没有放宽测试掩盖泄漏。

## 本地自动回归

Go `go1.26.10`；完整命令、JSON 事件与退出码位于
`bin/qa-artifacts/selection-status-20261025/`。

- macOS：`go test -race -short -json -count=1 -p 2`，覆盖 graph、prop、gui、ged、
  cmd/silkide、decl、examples/spinbox-edit；**5797 pass / 46 skip / 0 fail，7 个包通过**。
- 新状态／画布 action／CodePanel 生命周期测试 `-race -count=3` 通过。
- `go vet -composites=false` 六个核心包通过，IDE 原生构建通过。
- 保存后实际生成的 Go 构造代码另行 `go test -race -count=1` 通过：控件值、范围、
  step、page step、suffix、read-only 均核对，内部编辑器不泄漏为独立窗口。
  生成器将毫米转为整数像素，本轮不声称两者数值完全相同。
- Windows 10 22H2 真机：同源 overlay 原生构建与 `-race -short -count=1` 六个包通过，
  **5637 pass / 69 skip / 0 fail**；针对性测试 `-race -count=3` 通过。
- 所列数字为这些包的测试及子测试事件，不是全仓库／所有平台的质量认证。

## macOS 原生闭环

运行二进制 SHA-256：`ff7fbb3b3f4553fc1fcf1f780b8e153dd8afebd282be08dde7f4c91e00fef8f2`。
以原生按钮／字段进行：

- Width `85 → 60.5 → Undo 85 → Redo 60.5`，核对画布、属性值和底部提示三者一致。
- 保存 `(15.25,20.75) 60.5×12.125`；键盘右移及撤销重做；拖动吸附到 `(35,30)`；
  拉伸到 `70×20`，分别撤销重做并逐字节读取 `.silkui`。
- 回到小数几何；Edit 模式不残留设计器提示，Design 模式恢复选中信息。
- 正常关闭、重新启动并选中对象，仍显示 `counter [SpinBox] (15.25,20.75) 60.5×12.125 mm`。
- 原生窗口截图与文件断言分别留存，文件相同不冒充视觉断言。
- 重载 PID 12030 的 6 次短时 `ps` 样本为 0.4–0.8% CPU；不是 FPS 或长时性能基准。
  两个最终 macOS 进程均通过 Cmd+Q 正常关闭。

## Windows 原生闭环与最终验收

- 二进制 SHA-256：`c3e0b67e80b8e1665fbb22cdde61329ab8d7de0d7390f0ad13b30b8db79a5f5d`。
- 同样验证属性输入、画布 Undo/Redo、方向键、拖拽、右下手柄缩放、Edit/Design
  模式及正常关闭重启。撤销／重做后额外从实际 Width 字段复制读回 `85`／`60.5`，
  不仅检查保存文件。截图可见底部状态与画布／属性同步。
- **每个平台 14 个保存文件逐字节断言**，外加重启后的最终文件、属性与截图核对。
  两平台生成的实际 Go 文件 SHA 相同，并已用 SDK 构造测试回读。
- 9 个独立桌面计划任务均读回 `Ready / Result=0` 与各自成功 marker；输入前核对
  交互桌面、进程路径／SHA、前台 HWND 与点击像素所属进程。没有重叠运行 UI 任务。
- 5.019 秒静置区间 CPU 增量 0.015625 秒，约 **单核 0.31%**，窗口 Responding；
  这是本机短时样本，不是帧率、低配机器或长时间性能保证。
- 验收后 Windows QA 程序及其 packaged tools 进程为 0；1801 个远程基线文件
  与 479 个 overlay 文件再次校验未变，本地 1740 个源输入也与测试清单一致。
- 最终可重复验证：`python3 bin/qa-artifacts/selection-status-20261025/verify-final.py`。
  标记 `SELECTION_STATUS_SOURCE_TEST_NATIVE_VERIFIED`；源码／文档 diff、失败试验、
  脚本／测试／截图 SHA 和原始日志均保留在该目录。

### 重载后的实际窗口

![macOS 选中状态与小数几何](docs/images/selection-status-macos.png)

![Windows 选中状态与小数几何](docs/images/selection-status-windows.png)

## 失败证据与边界

- 保留原实现状态滞后红测试、过早建立监听导致的生命周期失败，以及旧二进制实际
  toolbar Undo 的属性滞后截图。三个 Mac 试验输入独立存放，不覆盖历史结论。
- 两次 Windows 旧输入构建在确认 Mac 缺陷后主动取消；不是测试通过，也不当作超时失败。
- 首次键盘微调断言预期 `16.25,20.75`，实际为 `16,21`。追到现有 MoveCommand
  对独立目标的 1 mm 取整，保存失败观察；本轮验证状态与已提交模型一致，**未修改此策略**。
- Linux、32 位、Windows 高 DPI、全控件／多窗口长时压力未测；既有 Preview Escape
  与快速多窗口焦点问题未在本轮处理。未宣称达到 Qt Creator 完整功能对等。
- 状态报告最终提交的几何，不承诺临时拖拽预览逐帧变化。SDK formatter 仍要求 UI 线程。
- 本轮没有重新设计所有素材，也没有新增后台刷新线程或独立子窗口。
