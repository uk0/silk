# Windows 原生 SDK／设计器验收 — 2026-10-23

## 范围与来源

- 真实机器：Windows 10 22H2 / build 19045 / amd64；交互桌面 Session 1。
- Go 1.26.10，CGO_ENABLED=1，UCRT64 gcc，Cairo 1.18.4；不是 silk_pure_go 交叉编译。
- 基线 HEAD：`1dd5c05432244057ecf67b0abbe6c91eb6a4ed0b`，测试包含当前未提交的 SDK／设计器改动。
- 同步独立目录 `C:\silk\qa-transfer-20261023-codex`，未覆盖原工程；初始快照 1,766 个文件逐个 SHA-256 一致。后续 6 个修复文件另行验证，原件保留于 `source-before-fixes`。
- 本地证据：`bin/qa-artifacts/windows-transfer-20261023/`（Git 忽略）。远端原始日志、脚本和二进制保留，不作清理或发布。本轮没有 commit／push。

## 实测发现与修复

1. **按钮异步完成后仍显示 Cancel**：真实下载完成、SHA-256 已显示，但按钮未刷新；移动鼠标或其他输入后才更新。`gui/button.go` 的文字／图标／文字可见性 setter 现在直接触发局部 invalidation，不依赖下一次 OnIdle。相同文字和可见性不重复重绘。新增 `gui/button_invalidation_test.go`；使用修复前 button.go 的 overlay 可确定性复现失败。
2. **Windows 新生成文件回滚残留**：`internal/filetxn/filetxn.go` 原先拿不存在文件的默认 0644 与 Windows 实际报告的 0666 比较，错误判定回滚冲突。改为记录 staging 文件实际权限，保留内容、权限和并发修改保护。增加“新文件被外部修改时不得删除”回归；原失败用例现通过。
3. **取消测试依赖 1ns 时钟间隔**：初次 Windows 测试等待进度回调取消，最终等到 30 秒 deadline；改为 Writer 接收预期字节后确定性取消，不削弱对取消错误和写入字节的断言。Windows race 下重复 20 次通过；真实 UI 取消另行验证。未改变下载节流 API。
4. **APPDATA 测试误报**：隔离目录合法但不含字面量 `AppData`。测试改为核对实际环境路径，并覆盖显式重定向；没有修改平台配置路径实现。

## 自动化结果

计数包括子测试／fuzz seeds，不是独立函数数量，也不跨轮累加。

| 检查 | 结果 |
|---|---|
| 初始 17 包，非 short | 4,123 pass / 30 skip / 3 fail；失败如上，保留原始日志 |
| 修复后 17 包，short | **4,092 pass / 63 skip / 0 fail** |
| 修复影响的 httpx、gui、filetxn、settings，非 short | **2,035 pass / 14 skip / 0 fail** |
| 修复后 12 包，race + short | **3,693 pass / 50 skip / 0 fail**；无 race 报告 |
| Session 1，强制原生 modal + IME，TestWindows 子集 | **47 pass / 0 skip / 0 fail** |
| 取消用例，Windows race，count=20 | **20 次通过** |
| IDE、Download Monitor、JSON Inspector，Windows native | 三个构建退出码均为 0；19 个 UCRT64 DLL 闭包 |
| macOS 修复回归，4 包 race + short | 全部通过；另有取消用例 count=10 通过 |

测试包／参数完整记录在 `run-native.ps1`、`run-final.ps1`、`interactive-tests.ps1`。不遍历仓库受限 `local/`。关键日志：`packages-final-short.log`、`fixes-final-full.log`、`race-final.log`、`native-interactive.jsonl`、`test-summary.json`。

Session 0 的原生焦点／IME／模态跳过项由 Session 1 子集补测；short 的外部编译、fixture opt-in、缺失工具以及其他平台条件仍按 skip 报告，不把它们算通过。

## 真实 UI 操作

- 下载：2 MiB 已知长度／分块未知长度、SHA-256 不匹配、HTTP 503、运行中取消及等待后状态不回退；缩窄窗口到 400px 客户区，按钮两列且进度信息不重叠。示例写入 io.Discard，未下载文件到磁盘。
- JSON：中文与彩色 emoji、精确 `9007199254740993`、2000 项数组分组导航；定位 `/data/items/1999/id`，复制路径／JSON 后读回分别为该 Pointer 与 `1999`；无效 JSON 和取消保留旧数据。
- 设计器：通过面板搜索并鼠标拖入 JSONView、TransferProgress，再拖动尺寸手柄；通过原生另存为保存并读取磁盘文件，而非脚本直接构造设计文件。
- 保存结果：`workspace\windows-controls.silkui`；JSONView bounds `[10 5 85 55]`，TransferProgress bounds `[10 70 85 25]`，均在 100×100mm 表单内。设计数据不含下载字节数等运行时状态。
- 早期一次另存为脚本失去前台焦点导致输入无效；确认原生对话框属于本次 IDE 进程并重新聚焦后保存成功。这次脚本失败不被计作保存通过或产品缺陷。

## 最终构建复测与资源采样

- 最终 `apps-fixed` 的两个 SDK 示例重新走完上述 UI 场景；成功／校验失败后按钮均立即恢复 **Download**，取消后旧进度没有覆盖终态。对应 `transfer-fixed-*.png`、`json-fixed-*.png`。
- 最终 IDE 重启后自动恢复 `windows-controls.silkui`，控件位置、尺寸与磁盘内容一致；见 `ide-fixed-restored.png`。File／Help 菜单图标和文字在实窗可见；About 对话框正确显示 logo、版本、Go 1.26.10、windows/amd64、Cairo · Win32，Escape 可关闭并返回设计器。
- 当前桌面为 **96 DPI，4 个逻辑 CPU**。连续打开／关闭 File 菜单 **30 次**，第 0／10／20／30 次均为 **GDI 18 / USER 23**，进程保持 Responding。只证明本次有界循环未见增长，不是无限期泄漏认证。
- 停止编译和输入后，采样 **15.019 秒**：Download Monitor、JSON Inspector、IDE 的单核等效 CPU 分别约 **1.46% / 1.14% / 1.66%**（整机口径再除以 4）；各自 GDI／USER 均未增加，全部 Responding。没有测量实际绘制 FPS，不据此声称 0 CPU 或帧率保证。
- 最终安静阶段 IDE 日志未新增 watchdog 恢复告警；初始负载期提示仍保留在限制项中。原始数据：`menu-resources.json`、`idle-resources.json`。
- 复测结束已通过 WM_CLOSE 关闭本次三个 QA 程序；保留截图、原始 JSONL、构建哈希和保存文件。原始远端证据归档为本地 `evidence.tar.gz`，解包在同目录 `evidence/`；最终三个 GUI stderr 未见 panic、race 或 watchdog 告警。

## 边界与待补覆盖

- 测试机未提供可用 gopls／Git，IDE 可启动但有语言服务与 Git 提示；真实 LSP 跳转、补全、重命名不能称通过。
- 尚非 Windows 多版本／多 DPI／多显卡认证；未测试打印机硬件、外网下载、安装器和 Linux 桌面。
- Session 1 的 IMM 几何测试不是人工中文输入法候选窗端到端认证。
- 初始负载阶段 IDE 日志包含 5.703s／17.06s watchdog 恢复提示。未据此确定是应用阻塞、系统调度还是心跳检测边界；不能宣称所有性能问题已根治。
- 设计器中新控件初始通用尺寸偏紧；本轮验证了手柄缩放及保存结果，不把默认尺寸打磨视为已解决。

## 后续 gopls 交付修复

本报告中的 gopls 缺失及真实 LSP 跳过属于本轮当时的测试条件。后续已修复随包交付、启动查找和 CI 硬门禁，并完成 Windows 真机真实语言服务测试，见 [GOPLS_BUNDLE_QA_20261023.md](GOPLS_BUNDLE_QA_20261023.md)。原记录保留，不将旧 skip 改写为 pass；Git/Delve 仍为可选外部工具。
