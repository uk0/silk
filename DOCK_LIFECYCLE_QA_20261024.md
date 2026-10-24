# Dock 生命周期与面板恢复 QA — 2026-10-24

基线：`a0b906c4cdd7309d90716d91230895d59e1e137e`。增量开发，不是安装包／Release 发布。

## 从失败测试到实现

本轮先得到四个失败回归：最小化后 `Frame.Layout()` 又显示控件；最大化只是覆盖像素
坐标，兄弟 dock 仍显示、下次布局失效；未知 `ShowToolView` 返回成功；关闭 Search
后快捷键无法恢复原实例。原始失败输出在 QA 目录的 `before.log`。

- 工具 dock 可选 `SetRetainClosedViews(true)`：用户关闭标签仅隐藏并保留实例、父节点、
  状态和回调；隐藏最后一个标签释放布局空间，但不丢弃宿主 dock。显式 `CloseView` /
  `CloseAllViews` 仍销毁，包括隐藏实例。文档 dock 不拦截保存／关闭。
- `HideView` / `RevealView` / `HasView` / `AvailableViews` 共用所有权判断；重复关闭不
  二次销毁，未知视图不被擅自挂载；Frame 工具菜单恢复已配置对象，而不是重新构造
  一个没有 IDE 回调的面板。无 Frame 的独立 dock 也能安全增删视图。
- 最小化隐藏面板并释放空间；通过菜单／快捷键显式显示会恢复。最大化由 Frame 的
  临时状态驱动原 split tree，未修改用户可见性或 split 比例；还原无需保存过期像素
  边界，也移除了全局 `dockSavedState`。隐藏分支不再保留不可见 splitter 命中区域。
- 关闭／移走最大化 dock、模式切换、隐藏最后一个标签都会正确退出临时最大化。
  保留工具关闭策略随拖入新 dock 传递；拒绝的 Frame drop 不再先移除源视图。
- SilkIDE 三个工具区启用保留策略；快捷键、菜单和显式构建／引用／测试输出共用恢复
  路径。恢复查询实际父 dock，不再只相信最初的左右栏指针。后台的模式记忆只选择
  仍开放的标签，不自动重开关闭的标签。
- 新增 **View → All Panels**，使用标签同源的标题／图标，包含关闭的面板；增加
  **Restore Panel Layout** 命令和中英文提示。独立 `restore` 矢量轮廓图标已纳入
  浅／深主题精确色彩与 16–64 px 缩放栅格测试。

所有新 Dock API 仅供 UI 线程调用。保留策略不停止宿主主动启动的后台任务；搜索的
现有取消／上限协议不变。隐藏状态／标签位置仅本次运行保留，尚未跨启动持久化。

## 自动化验证

```sh
GOMAXPROCS=2 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide \
  ./cmd/silktools ./internal/idetools ./internal/filetxn \
  ./filesearch ./internal/regexscan
```

- macOS：**5,287 pass / 47 skip / 0 fail**。
- Windows 10 原生 cgo/race：**5,175 pass / 62 skip / 0 fail**。
- 两平台各 11 个含测试包通过；`internal/regexscan` 没有独立测试文件，不算测试通过。
- macOS 按仓库 CI 的 `go vet -composites=false` 通过，不宣称全严格 vet 无既存警告。
- 随包真实 gopls 集成覆盖未保存 Unicode 补全／格式化、重命名、定义／类型／实现和
  返回导航；另核验原生 GUI 的实际子进程路径。最终结果与哈希见下。

## macOS 原生操作

隔离 workspace 中 a.txt 为 `中🙂 foo_12 foo_34\n`，b.txt 为 `foo_56\n`；
另有 Go 文件、忽略文件、12,000 个 `a ` 的 dense.txt。

1. 欢迎页 Cmd+Shift+F 搜索 foo，4 项结果；中键关闭 Search，快捷键恢复，结果不丢失。
   F4 跳转并输入 q，保存逐字节验证 `中🙂 qfoo_12 foo_34\n`；Undo 后保存恢复。
2. 点最小化，编辑区回收左栏空间；快捷键恢复，F4 到第二项，保存验证
   `中🙂 foo_12 mfoo_34\n`，随后恢复。
3. 最大化 Search 后只显示该 dock，其他 dock 不重叠；View → All Panels → 首项
   Bookmarks 命令恢复原布局并显示对应面板。CUA 截图在子菜单弹出时只捕获父弹窗，
   因此以实际菜单命令结果而非未捕获的子菜单截图确认该路径。
4. 最大化后依次关闭 Packages／Widgets／Files，最后点击 Search 的 ×。空 dock 不留
   空白占位；Cmd+Shift+F 恢复，F4 跨文件保存验证 `efoo_56\n`，随后恢复。
5. 打开 main.go，关闭并恢复 Outline，点击 main 跳至函数行；写入测试注释，保存结果
   逐字节核验，包含真实 Go 格式化增加的空行；随后恢复无注释、gofmt 规范化夹具。
6. 一万条结果下执行 5 次关闭／恢复 + 5 次最小化／恢复，连续 21 次 F4 后光标第 1 行
   第 41 列。静置 10.014 s，CPU +0.100 s，约 **1.00% 单核**。
7. TXT 夹具恢复为原始字节，Go 夹具恢复为格式化后的原逻辑；Cmd+Q 正常退出，
   QA IDE／gopls 残留进程为零。

## Windows 原生操作与最终核验

Windows 10 22H2 交互桌面 Session 1，原生 cgo 构建；隔离源码 overlay、workspace
与应用数据，非交叉编译或 SSH Session 0 GUI 验证。

1. Search 关闭／快捷键恢复后，F4 跳转与保存回调仍有效；最小化后文档区回收空间，
   恢复后第二项跳转仍正确。保存字节分别为 `中🙂 qfoo_12 foo_34\n` 和
   `中🙂 foo_12 mfoo_34\n`，各自 Undo／保存恢复。
2. 最大化 Search，窗口从 1280×900 缩为 1100×760 再恢复，兄弟面板没有覆盖回来。
   **View → All Panels** 显示 18 项默认工具面板；选 Bookmarks 恢复布局与正确实例。
3. 最大化后关闭左区全部标签，空 dock 释放空间；快捷键恢复 Search，F4 跨文件，
   保存验证 `efoo_56\n`，随后恢复。
4. 关闭／恢复 Outline，点击 main 跳转，写入注释并保存逐字节核验。最终恢复原始
   Go 夹具，包括其格式化空行。保留首次输入失败样本：Enter 接受了尚未关闭的补全
   候选，导致字节断言失败；从精确原始夹具重置后，先 Escape 拒绝候选再 Enter 换行，
   完整重跑关闭／恢复／跳转／保存成功。这是明确修正测试输入步骤，未把首次失败
   记作通过，也未未经基线对照将其归因于本轮产品回归。
5. 再执行 5 次关闭／恢复、5 次最小化／恢复；一万条搜索结果连续 21 次 F4，光标为
   第 1 行第 41 列。20 次 Alive／Responding 采样均通过；静置 10.284 s，CPU +0.1875 s，
   约 **1.82% 单核**。这是单次空闲样本，不是与旧版本对照的性能提升比例。
6. TXT／Go 夹具恢复原始字节，Alt+F4 正常退出，QA IDE／gopls 残留为零。
   清理本轮 10 个精确命名的计划任务；没有停止或删除其他测试会话。

跨任务保留的 Windows 弹出菜单曾因新任务夺焦而关闭；菜单测试改为在同一任务内
重新打开后操作。保留首次脚本／截图，以及重跑后的 18 项面板菜单截图。

### 源码、语言服务与产物回读

- 测试前后 1,520 个本地编译输入 SHA-256 一致；Windows 1,801 个不可变基线文件
  与 157 个 overlay 输入均核对，导出时再次校验。未遗留临时新增包目录。
- 两平台随包 gopls 各 **3 pass / 0 skip**：未保存 Unicode 补全／格式化、未保存
  定义重命名、定义／类型／实现／返回导航。原生 GUI 的实际子进程路径分别为
  `SilkDockLifecycleQA.app/Contents/MacOS/tools/gopls` 和
  `C:\silk\qa-dock-lifecycle-20261024-codex\apps\tools\gopls.exe`，不是仅检查 PATH。
- macOS 4 份、Windows 9 份关键保存／恢复文件再次逐字节核验；Windows 证据包下载
  后 SHA-256 验证，再解析 race JSONL、退出码、语言服务结果、清理记录和 CPU 样本。
- 原生截图已归档至 `screenshots/silkide-dock-lifecycle-macos-20261024.png` 和
  `screenshots/silkide-dock-lifecycle-windows-20261024.png`；更多过程截图在 QA 目录。

| 产物 | SHA-256 |
| --- | --- |
| macOS QA executable | `f0edd443412da9c89ad9c809590b6a7fb45015ec179656cc7da358be158c330b` |
| Windows QA executable | `b749d9df81e30dcc00e5563d2b1076e3309de2c95c0a9cb0120c0b937cb8efb4` |
| Windows evidence archive | `0a9787c6eaae55f4d93483d689ffe35264c71dd430ccbb6423f089f2fa5e8dbf` |

## 范围与证据

`bin/qa-artifacts/dock-lifecycle-20261024/` 保存失败复现、1,520 个本地编译输入哈希、
Windows 不可变基线与 157 个 overlay 输入、race JSONL、gopls、截图、保存字节、
CPU 和正常退出记录。测试冻结后未修改编译输入。

Linux 原生 GUI 未测；未覆盖全部面板业务、所有拖拽组合或长期内存稳定性。未新增
轮询／帧率定时器；空闲 CPU 是一次样本，不是任意机器、工程规模的延迟保证。
未创建发行标签、Release、安装器、Developer ID 签名或公证。
