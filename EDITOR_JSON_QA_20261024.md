# JSON 编辑器模式验收（2026-10-24）

## 本轮范围

- `.json`（大小写不敏感）独立识别为 `json`，状态栏显示 JSON；键、字符串值、数字与布尔/null 使用现有主题色，缩略图同步着色。
- 按行词法扫描，不解析整份 JSON。完整字符串后同一行的冒号用于识别键；不跨行猜测键，不校验 Schema，不支持 JSONC、JSON 格式化或结构折叠。
- 数字严格识别 JSON 数字语法但不转为浮点数；Unicode、转义、未闭合字符串及异常 UTF-8 保持原字节。达到 4,000 字节的行沿用普通文本回退。
- 缓存键包含语言，防止 JSON 与普通文本串色；同步/异步补全只添加 JSON 的 `true`、`false`、`null`，不再把所有非普通文本都当成 Go。Go/SilkGo 功能仍显式限定于对应模式。
- 类型切换保留正文、dirty 与 undo，清理旧投影；继续支持宿主显式高亮。没有新增定时器或空闲轮询。

## macOS 验证

- 首轮反例先失败（JSON 被识别为普通文本），修复过程中又检测并排除 Go 候选泄漏。
- 最终原工作区 13 个目标包的 `go test -race -short -json -count=1 -p 2`：**5,420 pass / 47 skip / 0 fail**，包含子测试；`internal/regexscan` 无测试文件。补充 `graph`、`prop`、`settings`：**238 pass / 1 skip / 0 fail**。跳过项不算功能通过。
- 同组 `go vet -composites=false`、IDE/SDK 示例构建、`examples/tab-navigation` race 测试通过；并非 strict-vet 全选项声明。
- 强制随包 gopls 的真实集成 **3 pass / 0 skip**：未保存 Unicode 补全与格式化、重命名、定义/类型/实现跳转及返回。工具目录清单校验通过。本轮未更换已固定的 gopls 版本。
- 两项独立模糊测试分别完成 **103,670** 次原字节保持与 **81,989** 次数字语法/`encoding/json` 对照，均通过。
- 光栅绘制测试覆盖浅/深主题及 1×/2×，检查文字颜色、背景像素、状态栏、缩略图；原生桌面另行测试，不把离屏 PNG 冒充桌面截图。
- 原生测试：打开 JSON → 切至相同内容的 `.txt`，确认普通文本无 JSON 高亮 → 返回 JSON，输入包含中文/emoji 的修改 → Cmd+S 后磁盘逐字节匹配 → Cmd+Z、Cmd+S 后原文精确恢复 → 切至 Go，Go 高亮/outline 与 `gopls: connected` 正常 → 正常 Cmd+Q 退出。三份 fixture 原文均回读一致，本轮应用及随包 gopls 剩余进程为 0。
- 10.119 秒原生空闲采样：IDE 增加 0.06 秒 CPU，约 **0.59% 单核**；两个随包 gopls 进程 CPU 计数未增长。这不是长期性能、FPS 或输入延迟承诺。

## 性能样本

Apple M2 Max / darwin arm64 / Go 1.26.10 / `GOMAXPROCS=2`：

| 基准 | 结果 | 分配 |
| --- | --- | --- |
| 已缓存 JSON 行 | 29.75 ns/op | 0 B / 0 alloc |
| 1,000 行滚动位置 Draw | 85.160 µs/op | 8,280 B / 179 alloc |
| 100,000 行滚动位置 Draw | 89.115 µs/op | 8,288 B / 179 alloc |

Draw 使用 no-op painter，衡量编辑器遍历/缓存开销，不等于实际 GPU 帧耗时或桌面 FPS。

## 排障记录与证据

- 首次完整回归失败：`TestTransferDownloadUsesCoalescedUIQueue` 曾观察全局 UI 队列为 2；同时 `TestNoWidgetMutationOffTheUIThread` 读取 `main.go` 停在系统 `open`，最终超时。没有将该轮计为通过。
- 使用基线 `10af0f2` 与冻结增量重建回放树，逐一核对 1,554 个输入哈希；仅添加测试诊断 overlay，完整 GUI race 连跑三轮和定向连跑三十轮均通过，未复现队列来源，不能声称已证明该异常的根因。
- 新 JSON 类型切换测试改用自身的 Git service，并在清理时等待 worker 退出，避免依赖实际 Git 或把后台结果投递到其他测试的全局队列；绘制测试显式关闭 painter。未放宽原队列断言，也未改动产品 Git 调度器来隐藏失败。
- 原工作区恢复可读后，重新核对 1,554 个输入并在原路径完成上述最终完整回归。原生程序使用同一产品源码构建；最终调整仅涉及测试隔离和文档，不改变所测产品代码。
- 原始证据在 `bin/qa-artifacts/editor-json-20261024/`：`final-inputs.json`、`final-race.jsonl`、`final-sdk-extra.jsonl`、`final-bundled-gopls.log`、`final-benchmark.log`、`native-save-readback.json`、`native-undo-readback.json`、`native-idle.json`、`native-cleanup.json`。失败日志和回放诊断单独保留。

## Windows 验证

- Windows 10 22H2 / amd64：1,801 个基线文件及 199 个增量输入在构建前逐文件校验，构建后回读增量哈希与本地一致。主 race 测试组 **5,310 pass / 62 skip / 0 fail**；`graph`、`prop`、`settings` 补充回归 **240 pass / 0 skip / 0 fail**。计数包含子测试。
- IDE 与 SDK 示例构建通过；随包工具校验及强制真实 gopls 集成 **3 pass / 0 skip**。Session 1 使用未在 PATH 暴露 gopls 的环境启动，子进程实际来自该应用的 `apps\tools\gopls.exe`。
- 原生 UI 从独立配置和重新写入的 fixture 开始：JSON 高亮/状态/缩略图 → 同文纯文本无高亮 → 在含中文/emoji 的字符串末尾键入 ASCII ` QA`，Ctrl+S 后精确回读 → Ctrl+Z、Ctrl+S 后原字节恢复 → Ctrl+Tab 切换至纯文本、Go，再返回 JSON。Go outline 与 `gopls: connected` 正常，三份 fixture 最终均逐字节恢复。
- 10.185 秒空闲采样，IDE CPU 增加 0.25 秒，约 **2.45% 单核**，20/20 次 `Responding=true`。正常 Alt+F4 后，本轮应用目录内进程为 0；本轮交互任务已清除。这不是长期负载或帧率保证。
- 初次 UI 脚本的保存/撤销已通过，但随后点击的 Go 页签被仍可见的保存 toast 覆盖，导致标题检查失败。保留截图和失败记录，正常退出后使用全新配置重放；最终用已有 Ctrl+Tab 导航，不把点击被遮挡区域当作功能通过，也未因此修改产品代码。
- 最终截图：`screenshots/windows-editor-json-20261024.png`。本地证据在上述目录的 `windows-final/`，包括测试日志、源哈希、`ui-evidence/results.json`、`cleanup.json`、纯文本/Go/JSON 截图和 `ui-attempt1/` 原始失败记录。

```text
Windows IDE SHA-256:
600b037362d36c7ce1ba41adb2ea0e38874b59aa854d939c1114b1a301c13154
Windows evidence archive SHA-256 (remote/local matched):
e6ff90dd52ec7dfb86f51a7ee5fda634d3a71c9fce4ff52c3fd3451330d099ce
```

## 平台边界与前序遗留项

- Linux 桌面、Windows ARM64 与真实中文 IME 未覆盖。本轮不是全平台发布门禁，也未重新进行 CVE 扫描。
- Windows 恢复连接后，读回前序批量关闭连续脚本结果为 `DISK_MISMATCH discard-alpha`，尚未独立复现/归因，不能补记为通过。其旧 QA 进程已限定目录清理（强制退出，不计正常退出验收）；记录为 `windows-previous-cleanup.json`。这与本轮 JSON 新配置下的正常关闭验证分别记录，后续仍需处理该前序场景。
