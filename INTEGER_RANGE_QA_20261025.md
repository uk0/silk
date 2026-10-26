# 整数范围恢复与 UI 回归（2026-10-25）

## 修复内容

上一轮发现的负数初始值重载归零，不是输入框显示错误，而是共享属性恢复层只为
`SetRange(float64,float64)` 批量设置范围，遗漏 `SetRange(int,int)`。
SpinBox 枚举“值”在前，导致值先被工厂默认范围 `0..99` 裁剪；随后修改范围无法还原。
同一问题影响设计器保存重载、运行预览、声明式构建，不应逐个 UI 入口打补丁。

- 在 `decl/runtime_properties.go` 增加整数范围能力处理：完整范围先于值恢复。
- 整数解码不经过浮点数；保留机器整数端点及超过 `2^53` 的精度，拒绝小数、
  NaN/Inf、溢出和表达式文本。不新增任意反射 setter，也不改变设计器属性可写白名单。
- `decl/runtime.go` 的 `value` / `Value` 字面量增加 `SetValue(int)` 能力支持。
- 不增加窗口、线程、goroutine、轮询或绘制定时器；未修改渲染路径。

## 可重放自动验证

```sh
GOMAXPROCS=2 SILK_TEST_FONT_FALLBACK=1 go test -race ./decl ./ged \
  -run 'Test(PropertyInteger|SDKIntegerRange|IntegerRange)' -count=3 -v
GOMAXPROCS=2 SILK_TEST_FONT_FALLBACK=1 go test -race -short -count=1 \
  ./decl ./ged ./gui ./prop ./cmd/silkide ./examples/spinbox-edit
go vet -composites=false ./decl ./ged ./gui ./prop ./cmd/silkide
```

新增测试覆盖六种属性顺序、字面量/持久化/英文别名、三次序列化循环、正负范围、
单点范围、整数端点、部分属性、越界夹取、非法输入和只读属性。
独立 SDK 消费项目真正编译并运行设计器与声明式两种生成构造函数，不以源码字符串
匹配代替运行值检查。原浮点滑块范围测试一并回归。

修复前回归日志记录了 `-42 → 0`、`250 → 200`、`-250 → -200` 以及大整数预览裁剪。
首次新增精度测试误用了 `Ref.ID` 字段，编译失败已修正并保留失败日志；不计作通过。

## macOS 原生闭环

使用独立 QA app、HOME、工作区；二进制 SHA 和 PID 留档。

1. 真实设计器加载范围 `-100..100`、初始值 `-42`，画布和属性栏一致。
2. 属性栏输入 `-57`，工具栏保存；读取实际 `.silkui` 确认值为 `-57`。
3. 正常退出并重新启动，画布仍为 `-57`。
4. 打开运行预览仍为 `-57`；Up 按步长 7 变为 `-50`。
5. 预览操作没有改写设计文件。原生保存生成的 `.silk.go` 原样复制到独立模块，
   `go test -race` 验证值、范围、步长、后缀和内部逻辑子控件归属。
6. 两个 QA 进程正常关闭。5 秒静默样本 CPU 约 0.05 秒（单核约 1%）；
   这是短时观察，不是长期性能或帧率承诺。

macOS 六个包的 race/short 回归：**5612 pass、46 skip、0 fail**（含子测试）。
专项 race 重复三次、生成代码运行测试和 vet 均通过。

![macOS 负数保存与属性栏](docs/images/integer-range-macos.png)

## Windows

Windows 10 amd64 使用同一源码 overlay（468 个文件），基线 1801 个文件及二进制
哈希均在测试前后核验。新增专项 race 重复三次通过；`decl / ged / gui / prop / cmd/silkide`
五个包 race/short 回归 **5452 pass、69 skip、0 fail**（含子测试）。

- 原生属性栏显示 `-42`；输入 `-57` 后保存，实际文件只改变值。
- 正常退出、重开后，通过字段选中/复制读取验证仍为 `-57`。
- 预览初始值 `-57`，Up 后读取为 `-50`，预览没有修改 `.silkui`。
- 通过原生标题栏关闭预览，正常关闭主程序；无本轮 IDE 或 bundled gopls 遗留进程。
- 5.024 秒静默样本 CPU 0.125 秒（单核约 2.49%），`Responding=true`；不外推长期性能。
- Windows 原生保存生成的 `.silk.go` 与 macOS 独立 SDK 运行验证的文件逐字节相同。

![Windows 重开后的负数属性](docs/images/integer-range-windows.png)

### 未通过：属性编辑撤销粒度

**不能把本轮解释为所有 UI 测试通过或达到发布条件。** 额外点击设计器撤销按钮后，
`-57` 退到逐字符编辑的中间值 `-5`，而非编辑前 `-42`；实际保存文件确认了该结果。
失败任务 `silk_range_ide_reload` 保留 Result=1、原始脚本、错误日志与
`windows-after-failed-undo.silkui`，未改写成成功。

随后独立恢复任务用 Redo 回到 `-57`，再完成重开与预览检查。该恢复不算撤销用例通过。
下一轮应溯源属性编辑的变更合并/提交边界，再补一整个字段编辑的 Undo/Redo 回归。
脚本状态回读还暴露了 PowerShell 本地化错误输出的编码问题，原始字节与拉回的日志
一并归档，没有用工具退出码替代 UI 断言。

## 产物与边界

完整重放输入、源码 SHA、修复前后日志、截图、UI 保存文件、生成代码验证及独立
Windows overlay 位于 `bin/qa-artifacts/integer-range-20261025/`。
本轮只改动 2 个产品源码文件、增加 3 个回归测试文件；没有覆盖其他未提交工作。

此修复不代表所有 UI 问题已解决：预览输入焦点下 Escape 关闭语义、多窗口快速
操作后的编辑位置异常仍在后续清单；Linux、32 位与 Windows 高 DPI 没有本轮实机结论。
Windows 既有 vet 债务也不在本轮宣称清零的范围。macOS QA app 没有外置 `icon`
目录，启动日志保留该提示；界面使用现有图标资源回退，本轮不据此宣称发布包资源完整。
