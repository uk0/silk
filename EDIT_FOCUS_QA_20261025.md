# 输入框横向视口与属性历史焦点 QA — 2026-10-25

接续属性面板实测发现的问题；开发中改动，不是发布声明。

## 修复与原因

- `gui.Edit` 原先创建了隐藏横向滚动条，但范围始终为零，`ScrollToCaret` 又只处理纵向。因此窄属性框内的长数字／文本无法跟随光标显示尾部。
- Layout 现在从已经排版的字形计算横向范围，排除末尾用于选区绘制的巨大占位 advance；光标移动直接读取字形坐标，不重新排版。单行和非换行多行均可横向跟随；隐藏滚动条不占用紧凑行高度。
- 键盘、选择、输入／粘贴、删除、尺寸／字体变化均更新可见区域；短值回写时夹紧旧选区。绘制限定到输入内容区，避免选区／文字覆盖边框及相邻属性标签。
- `Edit` 的输入校验此前把 rune 选区当 UTF-8 字节偏移，中文／emoji 合法替换可能被拒绝；现在验证与实际替换使用同一个字符范围。
- 设计器 Undo/Redo 原先在刷新属性后无条件 `canvas.SetFocus()`，并非普通属性 Undo 必然重建控件。保留同一有效属性控件的焦点；移除、重绑、隐藏、禁用或不同根窗口的控件不被恢复。
- 属性清除时先让全部旧行失去写入权限，再移除控件；晚到的失焦／对话框回调不能通过旧行给原对象或新 owner 提交命令。历史命令持有的模型属性仍可正常撤销。
- 不增加 timer、goroutine、后台轮询或逐帧树遍历；焦点树仅在执行设计历史时检查。GUI API 仍要求 UI 线程调用。

## 自动测试

- 新横向光标与属性焦点测试先失败后通过；另以 HEAD 的 `gui/edit.go` overlay 单独复现严格 UTF-8 validator 拒绝合法选区替换。
- 覆盖 ASCII/CJK/emoji、Home/End、逐字符移动、Shift 选区、粘贴、只读导航、短值替换／删除、零尺寸、宽窄切换、字体放大、多行换行及鼠标命中。
- 暗／亮主题像素测试比较有内容与空输入框的 padding，确认滚动字形与选区没有侵入边框。
- 绑定真实属性命令的 Undo/Redo、连续编辑、隐藏／禁用／移除／重绑／异树焦点回退测试通过。
- 第一版虽通过常规 UI，追加模型断言仍发现重绑期间旧 FloatEdit 失焦会把撤销值 `65.08749999999999` 覆盖回 `66.08749999999999`。补充 binding retirement 后重跑全部 gate；旧控件、旧 PropertyItem 及新 owner 串写测试通过。修复前失败记录单独保留，不混入最终通过数量。
- macOS：四包 `go test -race -short -json -count=1 -p 2 ./gui ./ged ./prop ./cmd/silkide` **5,260 pass / 42 skip / 0 fail**（含子测试）；`go vet -composites=false`、根包 race-short 测试及 RequireBundle 构建通过。
- 微基准 `BenchmarkEditHorizontalCaret`：14,000 字符已有排版文本，Apple M2 Max，**52.85 ns/op，0 B/op，0 allocs/op**。仅测光标定位，不代表文字初次排版或渲染吞吐。
- Windows 10：同四包原生 race-short-count=1 **5,129 pass / 65 skip / 0 fail**；RequireBundle 构建、工具 manifest 校验及 **3 项强制 bundled gopls 真实集成测试**通过，无集成 skip。

## macOS 原生 UI

独立 HOME／数据目录，两个 Button 使用既有真实设计器保存 fixture。正式测试二进制 SHA-256：
`3b4e42e87be3eccbdb516a75c8d801883be7567053aeff8894c0f26c1898b560`。

1. 多选两个 Button，Width 精确值 `65.08749999999999`；End 显示尾部和光标，Home 返回开头。
2. Up→保存，回读两值均 `66.08749999999999`；Undo→保存，均为原值。
3. **不重新点击属性框**，全选并输入 `66.12345678901234`，Enter→保存，两值精确读回；End 可见 `78901234` 尾部。
4. Up→Undo→Redo→保存，两值 `67.12345678901234`；仍不点击，再输入短值 `42.5`，保存，两值正确且文字从框左侧显示。随后点击空白并重新多选触发属性清除／重绑，保存回读仍为 `42.5`。
5. 主进程闲置 5.005 秒，CPU +0.05 秒，约单核 1.00%；不是全场景性能认证，不含 gopls。
6. 正常 Quit；IDE PID 88407 与其 bundled gopls PID 88418 均退出。

## Windows 10 最终原生 UI

- 最终二进制 SHA-256：`1a632fec15e32f71673b9e3c4fd52a4c6feee169a570242f3d7c60ad28e89361`。
- 交互桌面计划任务执行，不是 SSH 会话 0 模拟。3 个任务均 Ready / LastTaskResult=0，且各自成功 marker 和文件断言齐全。
- End/Home、精确全文复制、Up、Undo 后不重新点击直接粘贴、Redo 后继续短值替换均通过。7 份保存产物本地二次回读：两个 Button 的 Width 正确，X/Y/Height 不变。
- 短值一次粘贴→Undo，恢复 `67.12345678901234`，再次复制确认焦点／完整文本；清空画布选区再多选，属性重绑后精度仍保持。
- 六个原代码／文本 fixture hash 不变。闲置 5.012 秒 CPU +0.0625 秒，约单核 **1.25%**，`Responding=true`；此值不含 gopls，也不是 FPS／全场景无卡顿认证。
- 正常关闭 IDE PID 1580 和其 gopls PID 11208；任务内逐个检查退出。
- 本地 **1,665 个输入**与 Windows 的 immutable baseline 1,801 项加 overlay 366 项逐一匹配；测试前后 hashes 一致。首次版本和最终版本产物分开保留。

## 证据与边界

- `bin/qa-artifacts/edit-focus-20261025/`：修复前后日志和第一版记录；**最终包含生命周期修复的二进制与验收位于 `final/`**，含源码 hashes、回归 JSONL、逐步保存的 `.silkui`、原生截图和闲置／退出检查。
- 逐字输入仍按原有实时属性命令提交，不宣称整段输入合成一个 Undo；单次粘贴则是一次替换。
- 本轮原生 UI 是英文暗色；中文／亮色来自自动化测试。Linux 原生桌面、Windows 混合 DPI、全部字体塑形／IME 组合未验收。
- 命令面板长说明缺口已在后续 [命令面板 QA](PALETTE_DETAILS_QA_20261025.md) 中处理；其它差距继续逐项迭代，不宣称 Qt Creator 功能全面对齐。
- 未发布、commit 或 push。
