# Editor Find / Replace QA — 2026-10-24

本轮基线：`9698cc20b4af0089214d07a21dce551c43beedd2`。
本轮只完善编辑器查找／替换，不代表 Qt Creator 全功能对齐或新发行包发布。

## 实际发现与修复

- **过期坐标误替换**：`foo safe` 搜索 `foo` 后，外部把正文改成 `bar safe`，
  旧实现继续 Replace 得到 `X safe`。新增负例实际失败后，加入文档版本、
  请求代次、查询／选项及替换文本校验；文本变更重新计算匹配。
- **假输入框**：原查找栏由编辑器手绘并吞掉键盘事件，不支持完整选择、
  复制／粘贴和 Delete。替换为编辑器子树内的 Edit / Button / CheckBox / Label；
  不创建独立原生窗口，点击正文可在保持查找栏打开时编辑文本。
- **同步大扫描**：32 KiB 以上任务后台运行，75 ms debounce，每编辑器一个当前
  任务＋一个最新请求，全进程两个计算许可。KMP 按 UTF-8 字节流扫描，不构造
  全文 rune／小写副本；可取消，闲置无搜索计时器或工作 goroutine。
- **过度绘制**：匹配按行／列排序，绘制二分定位到可见行、可见列，不再每个
  可见行都遍历全部匹配。计数完整，导航／高亮最多保留 100,000 个位置。
- **新选项**：大小写敏感、Unicode 整词；不区分大小写支持 SimpleFold，含
  `Σ/σ/ς`、`S/ſ/s`、`K/K/k`；损坏 UTF-8 字节不与真正的 U+FFFD 混淆。
- **替换事务**：后台生成完整提案，最终在 UI 线程提交单次 Undo／编辑通知；
  全部替换处理全部命中，不受导航截断影响。1 MiB 查询／128 MiB 替换结果上限，
  超限不写入部分结果。
- **原生 UI 复验发现**：初版复选框文字／关闭图标裁切，空字体族导致中文方框。
  已改为实际主题 SizeHints 参与换行，以及平台 CJK 字体。查找高度不再拉伸
  独立的 Go to Line 栏。
- 宿主关闭标签、退出事件循环时调用 `CancelFind()`；隐藏编辑器取消任务，
  再次显示重新查询；关闭时释放 tooltip 注册，防止全局表持有已移除控件。

## 自动验证

命令（macOS 与 Windows 原生 cgo 都执行，Windows 使用校验过的源码 overlay）：

```sh
GOMAXPROCS=2 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide \
  ./cmd/silktools ./internal/idetools ./internal/filetxn
```

- macOS 最终 10 包：**5,160 pass / 47 skip / 0 fail**（含子测试）。
- `go vet -composites=false` 同一 10 包通过。
- macOS 实际 QA `.app/Contents/MacOS/tools/gopls`：**3 pass / 0 skip**；
  未保存 Unicode 补全／格式化／重命名／类型与定义／实现／返回导航。
- `CGO_ENABLED=0 -tags silk_pure_go` 的 `cairo / purecairo / paint` 三包通过；
  这不是 Linux 原生桌面验证。
- 新增／扩展测试涵盖：过期替换、编辑／Undo／查询／选项／关闭后的旧结果投递、
  单步 Undo/Redo、取消与最新请求合并、全进程并发上限、Unicode 随机 oracle、
  无效字节、命中截断但完整替换、超限拒绝、200–1024 宽度控件不相交、
  焦点与正文输入路由、tooltip 退订、查找与 Go to Line 高度独立。

## macOS 原生 UI 闭环

独立 `org.silkide.qa.find20261024` 应用、隔离工作区／数据目录；未操作用户正常实例。

1. 打开 `small.txt`，粘贴查询 `foo`：5 个子串命中；开启整词后 3 个，
   再开启大小写后 2 个。最终构建实际显示 `QA_中`，无方框。
2. 当前替换后全部替换，保存读回精确为：
   `Foo food QA_中\nΣ σ ς\nfoo_safe QA_中!\n`。
3. 把中间编辑器压窄，输入框、导航、选项、替换行自动换行且正文下移。
4. `large.txt` 为 `foo line 00000000\n` × 100,000，1,800,000 字节。
   确认 `1 / 100000`；F3 到 `2 / 100000`，Shift+F3 返回。
5. 替换为 `bar` 后保存，逐字节等于预期 1,800,000 字节；一次 Undo 后保存，
   逐字节恢复基线。最终构建重复了这条链路。
6. 十万命中高亮、查找框焦点保持时静置 10.004 秒：IDE 消耗 0.08 CPU 秒，
   约单核 **0.80%**。这是一段采样，不是帧率／长时无卡顿保证。
7. 正常退出后 QA 应用及其工具目录下进程数为 **0**。

## Windows 原生验证

- Windows 10 原生 cgo 最终 10 包：**5,048 pass / 62 skip / 0 fail**；
  单独实际 bundled gopls 集成：**3 pass / 0 skip**。
- 最终代码重新完整跑过上述命令；保存原生 stdout JSONL，逐行严格解析，未使用
  PowerShell 文本重编码输出，也未把打磨前的探索性测试计入最终结论。
- 校验基线 **1,801** 个文件＋**118** 个 overlay 文件；测试前后哈希一致，
  下载后再次逐项对照本地当前代码。测试产物 SHA-256 与 UI 进程身份一致。
- Session 1 实际点击整词／大小写选项，结果由 5 → 3 → 2；实际粘贴 `QA_中`，
  字形正常。全部替换保存逐字节符合预期，一次 Undo 保存恢复小文件基线。
- 100,000 处查找、F3／Shift+F3 导航、后台全部替换、保存、一次 Undo 保存；
  1,800,000 字节文件在两次保存后均逐字节匹配预期。再次缩窄面板，控件换行
  与正文下移正确，未出现文字／图标裁切或控件脱离父容器。
- 十万命中高亮、查询框焦点保持时：10.203 秒内 IDE 消耗 0.15625 CPU 秒，
  约单核 **1.53%**；20/20 采样存活且 Responding。仅为短时静置采样。
- Alt+F4 正常退出后自身目录内进程 **0**；本轮 5 个交互式计划任务已注销。
  未删除既有测试机工作区或更改全局 PATH／执行策略。

最终产物与证据摘要：

```text
macOS QA executable:
  c71d9ad4bfa77a60cd94d9088fe162dc3c0d5f207fa5dc6452513e7131e01fb9
Windows QA executable:
  588998262b6125ec5aac16a7518f9af2430273bb92b5a9ec9ea15519a7a815bb
Windows evidence archive:
  d5ca91fa06b5bd3002f9a5a65724cb6d0d7e191b784d75cea2e770add091e1e8
```

截图已加入 `screenshots/silkide-find-{macos,windows}-20261024.png`，
README 与 `docs/SDK_FEATURES.md` 同步更新。

## 证据与边界

本地原始证据：`bin/qa-artifacts/editor-find-20261024/`（未提交大体积日志／二进制）。
包含负例、测试原始 JSONL、前后 benchmark、输入源码哈希、产物哈希、截图、
保存后的文本副本、CPU 采样与进程清理记录。

1.8 MB / 100,000 命中字面量 helper 单机微基准：旧 11.58 ms / 15.68 MB alloc，
新 10.80 ms / 14.08 MB alloc（Apple M2 Max，短样本）；主要收益是扫描移出 UI
线程与绘制仅遍历可见命中，不以该短样本推断所有文档的性能。

未覆盖 Linux 原生桌面、真实中文 IME、Windows 每显示器 DPI、长时运行泄漏。
仍不支持正则／跨行查询。后台提案的最终应用仍需 UI 线程拆行、更新文本及通知，
不宣称任意大小文件零延迟。没有新增依赖或改变 gopls 固定版本／打包策略。
