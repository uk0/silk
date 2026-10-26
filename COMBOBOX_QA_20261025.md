# ComboBox 与设置字段交互 QA — 2026-10-25

开发中改动，不是发布声明；沿用上一轮命令面板和属性编辑修复。

后续视口与 Windows 激活修复见 [Popup Viewport QA](POPUP_VIEWPORT_QA_20261025.md)；
本文保留 `tab-focus/` 候选当时的证据与未通过项，不将历史失败覆盖为成功。

## 修复及兼容契约

- Appearance 的语言、主题、缩进是枚举值，不再允许显示任意草稿但保存旧索引。
  SDK 保留默认可编辑模式，新增 `SetEditable(false)`，绘制已提交项及可选图标。
- 下拉框的键盘／鼠标待确认项使用同一可见高亮；Home/End/Page 导航带动滚动。
  Enter/Space／有效行点击提交；Esc 只取消下拉预览，不取消所属设置对话框。
  Tab／Shift+Tab 取消预览并离开组合字段。
- 弹出框在 native show 之后取得**逻辑焦点**，保留原有原生 owner／恢复焦点链。
  先隐藏旧 popup 再发回调；回调中新建／重新打开的 popup 不被旧流程关闭。
- 已提交文本不读取外层模态快捷键遗留的 modifiers；实际快捷键仍保留修饰键检查。
  Unicode simple-fold 前缀匹配不构造整份候选文字副本；不是跨按键累积搜索。
- 可编辑字段在 AutoFocus 下只在真实编辑器停靠一次 Tab；显式 FocusPolicy 仍优先。
  `tab-stop-before.log` 保存原先错误聚焦外框的失败断言；双向 Tab 和模式切换已回归。
- 标签缓存、实际图标列检测、裁剪和非负几何边界；无新增轮询、timer 或 goroutine。
  `IEdit` 接口不变，但默认编辑器改为内部派生类型，自定义编辑器不会自动转发新增导航键。

## 失败证据与迭代

`bin/qa-artifacts/combobox-interaction-20261025/` 保留：

1. `before/`：真实设置窗口输入 Light 并 Apply，文件仍是 dark；End 移动待确认项却没有可见高亮。
2. `before-tests.log`：已选项不绘制、待确认项不可见、枚举字段仍可自由编辑的失败断言。
3. `modal-text-before.log`：Cmd+Shift+P 模态事件循环的外层修饰键错误地丢弃后续已提交字符。
4. `popup-focus-before.log`：popup 未取得逻辑焦点，原生 Esc 路由误取消 owner 对话框。
5. `final/`、`accepted/` 是中间候选，**不是本轮最终验收产物**；`verified/` 补足 popup 焦点后，又发现外框额外 Tab 停靠点；
   最新候选及源代码清单在 `tab-focus/`。

## 自动化结果

`tab-focus/` 源代码输入清单：1,675 个文件；Windows 保留 1,801 文件的不可变基线，
使用 377 个带 SHA-256 的 overlay 输入。测试前后校验，不以 transport 返回码代替测试结果。

macOS：

- `go test -race -short -json -count=1 -p 2 ./gui ./ged ./prop ./cmd/silkide ./examples/combobox`：
  **5,287 pass / 42 skip / 0 fail**（含子测试），五包通过。
- 根包 race-short、`go vet -composites=false`、RequireBundle IDE 构建通过。
- 在 bundle 内编译测试程序，强制使用附带 gopls：未保存 Unicode 补全／格式化、重命名、
  类型／实现／后退导航 **3 项通过，0 skip**。
- 回归包括绘制像素边界、缓存失效、空值／禁用状态、有效鼠标按下／释放、回调重入、
  可编辑输入框光标语义、registered `onChar`、真实 `dispatchEscape` 和 Tab 焦点路径。
- Apple M2 Max，三次 warm label cache **10.93–10.97 ns/op，0 allocs**；
  1,000 项 Unicode 查找 **6.145–6.177 μs/op，0 allocs**。只代表这些微基准，不代表整体 FPS。

## Windows 自动化与设置回归

Win10 22H2，原生 Go 1.26.10／UCRT64，同一组五包 race-short：
**5,153 pass / 65 skip / 0 fail**（含子测试）。RequireBundle IDE 与 SDK 示例构建通过，
附带 gopls 校验及 3 项强制 bundled gopls 集成测试通过，无 skip。

IDE SHA-256：`fd1a4e6a2ba74730659dc2775fcea346ac1450851eb16055e129e5867f716ff0`。
SDK SHA-256：`d7c60f86d564c87d98ed1e3c2b0c5aef03fa42d61b7dc696ee9d386ac7d51f16`。

- 交互桌面计划任务中，Light 输入、Home 预览、Esc 仅关闭 popup、Tab 取消预览、
  Spaces 输入和 Apply 后配置读回通过；重新打开显示 Light／Spaces。
- **尚待修复：Apply 关闭设置后，原生前台窗口落到了 Explorer 的 `client` 窗口，
  不是 IDE。** `after-apply-foreground.json` 已保留；重新打开验证前显式重新聚焦 IDE，
  不能把这一步算作框架自动恢复焦点通过。后续应单独追踪 Windows 模态结束／owner 激活。
- 第一轮自动化没有验证前台归属，曾把键盘发送到其他窗口；该轮 UI 脚本结果作废。
  安全取消误打开的 Open With 面板，未选择程序／修改文件关联；后续输入增加窗口 PID／
  命中归属断言，不再向未确认的窗口发送操作。
- 5.007 秒静置 CPU 时间增加 0.09375 秒，`Responding=true`（约单核 1.87%）；
  只是短采样，不等于长时压力、帧率或所有编辑器功能无卡顿。

## macOS 原生 UI

最终 IDE 二进制 SHA-256：
`a2985772193052572a80c0df864e3151b679a13c63a67a3ea846fc981e0c01c9`。
独立 HOME／数据目录，不改用户设置。

- 命令面板打开 Appearance，Theme 输入 Light 确实选择 Light。
- F4 → Home 的 Dark 只作预览；Esc 保留 Appearance 和 Light，焦点回到 Theme。
- 再次打开预览后 Tab 离开，按原表单顺序到缩进字段，选择 Spaces。
- Apply 后读取配置 `theme = "light"`、`insertSpaces = true`；重新打开显示一致。
- 7 个工作区 fixture 文件 SHA-256 未改变；5.016 秒静置 CPU 时间增加 0.05 秒
  （约单核 1.00%，单次短采样，不是长时间性能或零卡顿保证）。
- `mac-popup-preview-light.png` 显示亮色主题的可见高亮和独立提交值。
- 单独 SDK 示例的 macOS CUA 连接反复返回 ScreenCaptureKit `-3811`，没有取得可操作窗口；
  进程采样显示事件等待而非崩溃。此项**不能记为原生 UI 通过**。
- macOS popup 鼠标坐标点击尝试没有取得预期 owner 保留结果，需要后续隔离输入映射／
  原生鼠标路由；本轮 macOS 的原生通过范围仅为上述键盘与设置保存链路。

## SDK 原生回归仍未整体通过

Windows 最新候选在已校验输入归属的脚本中，以下步骤有实际 stdout 状态断言：

- 初值 Apple，End 只预览，Esc 不提交；End + Enter 提交 Item 099，选择与提交各一次。
- 两次原生 `a` 从 99 → Apple → Apricot，没有 KeyDown／字符输入重复跳转。
- popup Tab 取消预览并进入可编辑输入框，`free-text` 原样保留；Down 正确选择 Rust。

但实际截图 `tab-focus/windows-sdk-end-preview.png` 暴露了**尚未修复的屏幕边界问题**：
100 项 popup 按完整列表高度展开，超出屏幕；End 的内部索引虽到 99，视口中仍看不到末项。
因此不能把索引断言或单元测试里的小视口滚动当作原生可见性通过。

鼠标用例原先按“下拉框在字段下面”估算位置，实际 popup 被移动到屏幕顶端，
`(100,140)` 对应 Item 005，不是 Apple；截屏和状态均一致，提交回调一次，但整套脚本
仍为 `LastTaskResult=1`，没有修改结果冒充成功。

下一步优先：`gui/utils.go` 的 `LayoutPopup` 当前只对 Menu 调用工作区适配，
ComboBox 需约束真正列表视口（而不只是移动原生窗口），再从干净 baseline 重跑
Home/End/Page、滚轮、鼠标命中、owner 保留与边缘位置。Windows Apply 后 owner 激活
另作独立分支修复；macOS SDK 截屏连接和 popup 鼠标闭环也继续保留待验状态。

## 边界

Linux 原生桌面、物理中文 IME／AltGr、多显示器混合 DPI 尚未验证。
`-short` 跳过项单列；强制 gopls 只补足上述 3 项，不替代所有被跳过的集成场景。
