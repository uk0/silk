# Editor selection scope / long-span drawing QA — 2026-10-24

基线：`d47082712209c0f66fd7df892b8f110817806c93`。
这是编辑器增量开发与 QA，不是新版本发行，也不代表 Qt Creator 全功能对齐。

## 功能与安全边界

- 新增 **Selection** 开关与 SDK `CodeEditor.SetFindInSelection(bool) error`。
  `ShowFind` 捕获打开前的选区，启用后冻结范围；搜索导航、查询和选项切换不会
  把它移动到当前命中处。选区背景保留轻量高亮。
- 选中片段是独立搜索文本：`^` / `$` / `\A` / `\z` 与 Whole word 的边界
  都相对于片段。这是明确的本地边界语义，不声称与其他 IDE 完全一致。
- 字符串及正则的单次／全部替换不修改选区外字节；查找产生的替换跟踪范围大小，
  一次 Undo/Redo 同时恢复正文及范围。普通编辑使范围失效，保持开关开启并禁用
  替换，绝不自动扩大到全文；关闭查找清除范围。
- 范围属于文档 revision、scope ID 和请求 generation。过期后台结果被丢弃，旧
  Undo 记录不能重定位新选区。无选区、取消、输出超限均不会提交部分替换。
- 无效 UTF-8 仍按字节保存。如果替换接缝把旧的无效字节拼成一个有效 rune，
  单次替换保守地失效旧坐标；全量替换重新计算片段坐标并验证外围边界。
  Undo 可准确恢复原字节与旧有效范围，不插入 U+FFFD。
- 输出 128 MiB 上限包含未改变的选区外内容。保留最多 100,000 条导航位置，
  但计数及 Replace All 不截断；继承可取消、latest-only 与两 worker 计算许可。
- 查找栏所有按钮及开关保持在编辑器子树，按照实际 SizeHints 自动换行。

## 长行绘制优化

旧代码为整行的选中区域、查找和 occurrence 背景测量完整前缀，再交给 clip。
现在只计算可视区及 guard band 所需的填充几何，不改变文字抗锯齿、字体或描边。
只有当前源码及字体都通过认证的固定宽度 ASCII 才使用 O(1) 列坐标；Tab、CJK、
组合字符、emoji、无效 UTF-8 保留原有测量路径。远距离横滚的 Unicode 仍可能测量
较长前缀，本轮不宣称解决所有文本塑形成本。

M2 Max / Go 1.26.10，1 MiB ASCII 单行、800×160、预热后的 noop-painter Draw，
同一 `BenchmarkEditorLongSpanPainting`，每项 5 次：

| 背景类型 | 修改前 ns/op | 修改后 ns/op |
|---|---:|---:|
| Find | 2,239,917 | 15,008 |
| Selection | 2,413,917 | 9,558 |
| Occurrence | 2,280,408 | 10,825 |

这是绘制微基准，不是键盘到像素的端到端延迟或任意机器帧率保证。
1 / 1.25 / 2 倍缩放、冷／热缓存、横滚、零宽／超长范围和混合字符均做逐像素对照。

## 自动验证

```sh
GOMAXPROCS=2 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide \
  ./cmd/silktools ./internal/idetools ./internal/filetxn
```

- macOS：**5,210 pass / 47 skip / 0 fail**，10 包通过（包含子测试）。
- macOS 同组 `go vet -composites=false` 通过。
- `CGO_ENABLED=0 -tags silk_pure_go` 的 cairo / purecairo / paint 通过。
- macOS 随 QA app 打包的真实 gopls：**3 pass / 0 skip**，覆盖未保存 Unicode
  补全、格式化、重命名、类型／实现／定义跳转及返回。
- Windows 10 原生 cgo / race：**5,098 pass / 62 skip / 0 fail**，同组 10 包通过。
- Windows 随包 gopls 真实集成：**3 pass / 0 skip**；不依赖全局 gopls PATH。
- 两平台 skip 单独保留在 JSONL，不能记为覆盖通过。首轮新增几何测试 fixture
  未设置尺寸，导致 Draw 未执行；补齐 800×160 尺寸后再冻结源码并重跑上述全组。
- 专项测试还覆盖反向／跨行选区、本地锚点／词边界、中文增长与跨行缩短、零宽
  插入／删除为空范围、无变化替换、缺少选区、普通编辑失效、异步过期、旧历史
  与新范围隔离、无效字节拼接、完整计数、取消和输出预算。

## 原生 UI 回放

隔离工作区 `small.txt`：

```text
outside foo_11
foo_22 foo_33
中foo_44
outside foo_55
```

1. 文档开头 Down / Home，再 Shift+Down 两次，选中第 2–3 行；打开 Find，查询
   `foo`。未启用 Selection 显示 5 项，启用后显示 3 项，外围两处不高亮。
2. 展开替换并实际点击 Replace all，替换为 `bar`。保存后逐字节验证只有中间三处
   变化。查找栏保持打开，正文 Undo 后保存，验证原文和 3 项范围恢复。
3. 改为 `foo_(?P<id>[0-9]+)`，启用 Regex，替换 `${id}_中`；保存逐字节验证
   `outside foo_11\n22_中 33_中\n中44_中\noutside foo_55\n`，一次 Undo 恢复。
4. 在普通正文插入 `!`：范围提示错误、替换按钮禁用、外围未自动纳入搜索。
   撤销并重新打开查找，再捕获选区，缩窄编辑器确认布局换行、按钮无重叠。
5. `large.txt` 是连续 1,048,576 个 `a`。查询 `a+` 并启用 Regex 显示 1 项；
   跳到行尾，输入 `x` 后保存，逐字节验证；一次 Undo 保存恢复原始 1 MiB。
   保持整行高亮采样空闲 CPU。

macOS 上述流程已完成。10.005 秒采样 CPU 增加 0.080 秒，约 **0.80% 单核**。
被测 PID 正常 Cmd+Q 退出；退出后的 CUA 状态观察意外重启了应用，该新进程无法
建立自动化连接，按专属路径 SIGTERM 清理。最终本轮 IDE/gopls 残留为零。

Windows 同样完成上述流程，并补充缺少选区提示、真实 Replace 按钮、
`${id}_中_extra` 变长单次替换后继续 Replace All，以及分两次 Undo 恢复；
14 个保存产物在本地再次逐字节断言，选区外内容始终不变。原生窄栏截图显示
Selection 自动换到独立行，无按钮重叠。10.160 秒空闲采样 CPU 增加 0.109375 秒，
约 **1.08% 单核**；20/20 次 Alive / Responding。Alt+F4 正常退出，
本轮 IDE/gopls 残留为零，6 个本轮交互任务已清理。

Linux 未做原生 GUI；纯 Go 测试不替代 Linux 桌面验收。QA 应用为开发构建，
本轮未新增签名、公证、安装器或 Release tag，也未保证全部硬件均无卡顿。

## 证据

本地原始结果：`bin/qa-artifacts/editor-selection-20261024/`。
源码、测试产物和被测二进制 hash 在提交前再次比较；测试 fixture 不进入提交。
主要证据包括 `host-race.jsonl`、`host-gopls.log`、`span-before.log`、`span-after.log`、
`mac-*-replaced.txt` / `mac-*-undo.txt`、`mac-idle.json`、`mac-cleanup.json` 和原生截图。

Windows 原生构建使用经过 SHA-256 核验的 1,801 个基线文件及 125 个 overlay 输入；
运行前／归档后均复核，取回后与当前本地源码再次比较。原生 JSONL 严格解析，
不靠单个 marker 或终端末尾的 PASS 判断结果。

```text
macOS app SHA-256
66bc9092e7c6f463e04f65a234bf3e82032451ad0b2f9264b18a6586eb267a80
Windows exe SHA-256
01c8e059b759f48b5268e067b3a0b732ff783fc8daa1739d1f483d0a323022c3
Windows evidence archive SHA-256
ce8b00b42fdfd63bb82854aa2401df90037401918ba958d73744d089d80891ab
```
