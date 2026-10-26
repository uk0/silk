# 下拉视口与 Windows 模态激活 QA — 2026-10-25

开发中改动，非发布声明。接续 [ComboBox 交互 QA](COMBOBOX_QA_20261025.md)。

后续鼠标焦点根因、修复和最新验证见 [原生鼠标 QA](POPUP_MOUSE_QA_20261025.md)。
以下 macOS 数字及二进制哈希是本阶段历史结果，不是后续修复产物。

## 根因与修复

- 100 项下拉框曾按自然高度创建 **2410 px** 的原生窗口。位置钳制不能缩小真实列表
  视口，End 只改变内部索引，末项仍在屏幕外。`before.log` 保存失败断言。
- ComboBox 在创建原生 surface **之前**限制实际宽高，默认首选 10 行；按工作区
  可用空间上下展开，复用原有滚动、命中和可见行绘制。工作区向内取整，避免小数
  字体尺寸引入虚假滚动条／超出边缘；跨显示器重新打开会重算范围。
- 新增 `MaxVisibleItems`／`SetMaxVisibleItems` 与设计器属性 `max_visible_items`。
  小于 1 的值归一为 1，不删减数据；展开中改变行数不会提交预览，也不会丢失待确认项。
- Windows 模态关闭原先先 `SW_HIDE`，前台可能已转给其他窗口，随后销毁无法恢复 owner。
  改为恢复已启用、可见、存活的直接 owner 的原生活动状态，再隐藏／销毁 modal。
  仅当前台仍是该 modal 或其 popup 时执行；已切到独立窗口则不抢回。
  使用 `SetActiveWindow`，不以 `SetForegroundWindow` 强制抢前台，也不新增 timer／轮询。
  API 语义参考 [Microsoft SetActiveWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setactivewindow)。

## 可复现证据

`bin/qa-artifacts/popup-workarea-20261025/`：

- `mac-source-hashes.json`：1,678 个源代码输入；Windows 不可变基线 1,801 个文件，
  382 个带 SHA-256 的 overlay 输入。前后校验，构建产物与 UI 实例再核对哈希。
- `before.log`、`viewport-tests.log`：失败→通过，包括负坐标显示器、上下边缘、过宽字段、
  极小工作区、空列表、分数行高、可见行数变化、预览不提交、属性 getter/setter。
- `red-overlay.json`：仅省略新增加的 pre-hide activation 调用，用于 Windows 原生对照；
  不改变现有 timeout、断言或其他生产代码。

## macOS 已验证

- 五包 `go test -race -short -json -count=1 -p 2`：**5,301 pass / 42 skip / 0 fail**，
  含子测试。根包 race-short、`go vet -composites=false`、IDE/SDK 构建通过。
- RequireBundle IDE、附带 gopls 3 项强制集成测试通过，**0 skip**：Unicode 未保存
  补全／格式化、重命名、类型／实现／返回导航。
- 原生 IDE：Light 输入 → Home 预览 → Esc 保留设置及 Light → Tab 离开预览 →
  Spaces → Apply → 配置读回 → 重开可见 Light／Spaces。7 个 fixture 未改变。
- 5.001 秒静置 CPU 时间增加 0.07 秒（约单核 1.40%）；只是短采样，不是 FPS 或
  长时无卡顿保证。标签 warm cache 10.46–10.59 ns/op，千项 Unicode 搜索
  6.140–6.192 μs/op，均 0 allocs/op；不等于完整绘制性能。
- IDE SHA-256：`65ec054a58e6458860d7dbbda68ee82ce3a9e7e9bc600c0c57cb75c6da05f892`。

## 尚待验证／处理

- Windows 首轮、串行重跑均未全绿。后续诊断构建成功、IDE 设置流程通过，
  不替代完整门禁；独立 SDK／原生模态用例的失败见后续 QA。
- Windows 首轮五包 race **未通过**：GUI 的 Git 子进程 flood 测试出现
  `command exceeded bounded wait`（7.34 秒）；`cmd/silkide` 达到包级 10 分钟超时，
  当时 `TestGutterToggleOnAnEditorOlderThanTheSessionReachesDlv` 正在编译 fake dlv
  fixture（该测试运行 24 秒）。其余 ged／prop／示例通过。保留完整首轮日志，
  后续 GUI 串行重跑不替代尚未完成的完整 IDE 回归；不放宽 Git 的单项断言。
- macOS SDK 独立窗口 CUA 再次返回 ScreenCaptureKit `-3811`，**未通过原生 UI**。
  仅对已核对路径的 QA 进程发送 SIGTERM 清理；IDE 则通过 Cmd+Q 正常退出。
- macOS 鼠标问题已从干净设置状态复现：点击语言字段 `(600,120)` 打开列表，
  点击截图上 English 行 `(650,205)` 后 Appearance 消失、焦点回主窗口。
  `mac-popup-mouse-owner-loss.png` 记录当时现象。后续原生事件日志证实 popup 收到
  正确坐标；Appearance **并未关闭**，而是落到主窗口后面。修复及鼠标验收见后续 QA。
- Linux 原生桌面、物理中文 IME／AltGr、多显示器混合 DPI 尚未验证。
