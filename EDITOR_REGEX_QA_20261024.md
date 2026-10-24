# Editor regular expressions / lifecycle QA — 2026-10-24

基线：`341703773594d68c8f62e31aadbfd4044c36a245`。
范围是编辑器查找／替换及生命周期，不代表 Qt Creator 全功能对齐或发行包发布。

## 修复和功能

- 负例实测：`foo foo` → 无变化替换后，当前索引已到第二处，计数仍为 `1 / 2`。
  现在同步刷新计数，不产生无意义的 Undo。
- 负例实测：隐藏编辑器后外部修改正文，原实现重新启动后台扫描。
  现在在请求、结果投递和范围使用前检查可见祖先；隐藏、取消、关闭及无查找栏
  的正文变更会失效旧位置和计数。`FindSnapshot` 增加 `Ready`、`Revision`。
- `.*` 正则开关：Go 正则语法，跨行高亮、零宽标记、编号和命名捕获组替换。
  `$1`、`${name}`、`$$` 与标准库一致，包括重复命名组选择实际参与匹配的分支。
  `\n`、`(?m)`、`(?s)` 可用于查询；替换中的反斜线不作额外转义解释。
- 正则编译、查询、单次／全部替换提案都后台执行，沿用 debounce、可取消、
  latest-only、全进程最多两个计算许可。文档／查询／选项／替换文本过期时丢弃
  提案，不覆盖新编辑；提交为一次 Undo 和一次编辑通知。
- 正则扫描保留真实前一个 rune 的边界上下文，不把截断字符串误当文档开头。
  安装的 Go 1.26.10 无公开 start-offset 迭代器，因此使用可取消 RuneReader；
  取消造成的 EOF 在接纳结果前再次检查，不能误产生 `$` 匹配。
- 跨行范围按可见行／列二分定位；零宽匹配至少绘制 2 个逻辑像素。
  同行命中直接使用已缓存的结束列，不为每个高亮反复扫描整条压缩长行。
- 查找控件仍是编辑器子树，不创建悬浮原生窗口；增加开关后按真实 SizeHints
  换行，长错误可通过 tooltip 查看，关闭时撤销全部 tooltip 注册。

## 资源边界与非目标

- 正则查询最多 64 KiB、128 个捕获组、32,768 的保守展开成本；编译前校验。
- 单请求计算期限 5 秒；扫描、展开和输出均检查取消，不提交超时的部分结果。
- 替换模板最多 1 MiB，输出最多 128 MiB；捕获组向有界 writer 展开，不能先无界
  `ExpandString` 再检查长度。导航／高亮保留最多 100,000 项，但计数与全部替换完整。
- 不支持 Go 正则本身不提供的 look-around / pattern backreferences。
- 全量正文提交和 Undo 仍在 UI 线程；本轮采样不是任意文件、任意硬件的延迟保证。
- Linux 未做原生桌面测试；纯 Go 后端回归不能代替 Linux GUI 验收。
- QA app 是开发构建，不是签名／公证发行包；初次 `codesign --deep` 把许可证目录
  误判成子 bundle 而失败，未改变发布签名流程，也未宣称通过公证。

## 自动验证

```sh
GOMAXPROCS=2 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide \
  ./cmd/silktools ./internal/idetools ./internal/filetxn
```

- macOS 最终：**5,185 pass / 47 skip / 0 fail**（包含子测试）。
- macOS 同组 `go vet -composites=false` 通过。
- macOS QA app 随包 gopls 真实集成：**3 pass / 0 skip**，含未保存 Unicode
  补全／格式化／重命名／类型／实现／定义与返回导航。
- `CGO_ENABLED=0 -tags silk_pure_go` 的 cairo / purecairo / paint 三包通过。
- Windows 10 原生 cgo / race 最终：**5,073 pass / 62 skip / 0 fail**；
  随包 gopls 真实集成 **3 pass / 0 skip**，未依赖全局 gopls PATH。
- 两平台所有 10 包均通过；跳过项保留在 JSONL，不能当作覆盖通过。
- 测试覆盖数千组标准库 oracle（锚点、边界、分支、零宽、Unicode、无效 UTF-8、
  随机组合）、重复命名捕获、捕获展开语义、跨行坐标、截断保留但完整替换、
  超限拒绝、取消误 EOF、单次替换 stale fence、Undo/Redo 字节回放、隐藏祖先、
  无变化计数、200–1024 宽度无控件交叠、错误状态与 tooltip 清理。
- M2 Max、Go 1.26.10、GOMAXPROCS=2，3 次 benchmark：十万行正则查询约
  **96.4 ms/op / 40.1 MB/op**；2 MB 单行同行高亮列查询 **0 allocation**。
  这是独立计算 benchmark，不是键盘到显示的端到端时延。

## 原生 UI 回放

隔离工作区 `small.txt`：

```text
foo_12 foo_34
中foo_56
alpha
beta
```

1. Ctrl/Cmd+F，输入 `foo_(?P<id>[0-9]+)`，启用 `.*`，确认 3 个命中。
2. 打开替换，输入 `${id}_中`，Shift+Enter；Esc 后保存。
   逐字节断言 `12_中 34_中\n中56_中\nalpha\nbeta\n`；一次 Undo 后保存恢复原文。
3. 查询 `(`，确认错误和禁用替换，正文不变；查询 `alpha\nbeta`，确认两行高亮。
   首轮 macOS 另验证跨行单次替换为 `joined` 并保存／Undo 精确恢复。
4. `large.txt` 为 `foo line 00000000\n` 重复 100,000 次（1,800,000 字节）。
   查找 `(?P<word>foo) line [0-9]+`，确认 100,000 命中；替换 `${word}_中`，
   保存断言全部 100,000 行为 `foo_中\n`；一次 Undo 保存，原文逐字节恢复。
5. 保持 100,000 高亮，测空闲 CPU；缩窄编辑器观察控件换行，正常退出并检查
   本轮拥有的 IDE / gopls 进程全部退出。

macOS 最终构建以上中文捕获、非法查询、跨行绘制、大文件替换／单次撤销、
窄布局均复验。10.005 秒空闲窗口，CPU 增加 0.070 秒，约 **0.70% 单核**。
Windows 最终构建在 1280×900 桌面完成同一流程，并实际点击 Replace All / Replace
按钮；跨行替换和单次 Undo 逐字节验证。`(?m)^` 显示 5 个零宽标记，空替换后
计数从 `1 / 5` 正常前进到 `2 / 5`，正文与 Undo 均不增加无效修改。
10.194 秒空闲窗口，CPU 增加 0.203125 秒，约 **1.99% 单核**；20/20 次 Alive /
Responding 采样正常。上述 CPU 是本轮样本，不是跨硬件性能承诺。

两平台正常退出，拥有的 IDE / gopls 进程均为 0；Windows 删除本轮 6 个计划任务。

## 证据

本地隔离产物：`bin/qa-artifacts/editor-regex-20261024/`。

- `lifecycle-negative.log`：两个修复前失败；`focused-final.log`：专项通过。
- `host-race-final.jsonl`、`host-final-gopls.log`、`vet-final.log`、`purego.log`。
- `mac-*-final*.png`、`mac-*-final-*.txt`、`mac-final-idle.json`、`mac-cleanup.json`。
- 最终 macOS exe SHA256：`b319298d1cf9e31d3c81621cdd33355ac0d038bf20c5725ff244e4d2d2d7616e`。
- Windows 使用不可变 1,801 文件基线＋校验 overlay，测试前后校验输入；首次
  探索结果与最终结果分开，发布结论仅使用最终快照。

- 最终 Windows exe SHA256：`953c910145311f5bd094b0b845906e308fd85fd786bb29f982f6f69090921859`。
- Windows 归档：`windows-evidence.tar.gz`，SHA256
  `ebfcbc287d87d92cbfa3f1254484e1fb53e031e08c98bd145f6dd7a54cab0983`。
- 最终前后均确认基线 1,801 文件＋122 个 overlay；取回后严格解析 native JSONL，
  对照本地源文件哈希、全部保存／撤销的原始字节、进程 SHA 和退出记录。
- 首次 SSH detached build 未存活，未作为测试结果；后改成保持连接的执行。
  初轮测试后只调整了同行高亮列的重复计数开销及对应 benchmark，最终两平台
  重新全跑上述 10 包，并重建 QA 可执行文件后重新进行原生 UI 回归。
