# 菜单键入导航与面板动作一致性 QA — 2026-10-24

基线：`878f78f3d053bdb5a353fa67dc1d1097e2a1fe57`。这是下一轮工作区增量，未创建 Release。

## 已复现问题与修改

- `Properties.Icon()` 默认为 nil，TabBar 过去只检查是否实现 `IIcon`，直接返回 nil，
  忽略已注册的 `propsheet` 图标。现在先尊重非空自定义图标，再使用注册图标，
  dock 标签与全部面板菜单共用同一结果。
- 从 Welcome 的 All Panels 选择 Properties 原先进入 Edit，而 View/Properties 进入
  Design；现在属性／对象／控件面板共用模式判断，不切换已处于 Edit/Design 的模式。
- SDK 弹出菜单增加文字前缀导航：大小写折叠、Unicode 已提交文本、重复单字母循环、
  Backspace、下一次输入检查 1 秒超时；Enter/Space 才确认，不自动执行命令。
  隐藏／禁用项目跳过，读取实时标题和 enabled 状态，不缓存过期菜单。
- 查询与每次输入最多处理 64 个 rune；没有 timer、goroutine、idle 轮询或定时重绘。
  菜单关闭、清空、移除项目及方向导航会清除前缀。Space 保持既有确认语义，
  本轮不增加多词查询输入框或菜单过滤列表。
- 原生 UI 又复现所有已打开面板都画成亮蓝色，无法辨认键盘目标。现在 checked 项
  用主题混色浅底与独立勾号，只有当前 hover／键盘目标使用实色选中底。勾号复用
  菜单尾部预留空间，不覆盖原图标、文字或快捷键；有子菜单时保持箭头。
  深浅主题文字对比度与勾号／快捷键几何均有回归测试。
- 补上菜单自身的可见性检查，避免已关闭菜单接受残留输入。既存工厂哨兵测试原先
  给隐藏 popup 发 Down；现同时断言隐藏时不响应、逻辑显示后首个 Down 选第一项，
  保留原哨兵回归而非删除该断言。

原始红测三项在 `before-valid.log`。首次扩展测试暴露自身隐藏检查遗漏，首次完整
回归暴露工厂测试的隐藏前提；相应失败日志保留。最终状态以重跑后的冻结输入和
门禁为准，不把中间失败改称通过。

## 验证结果

### macOS

- 冻结输入下 11 个含测试包的 `-race -short -count=1`：**5,297 pass / 47 skip / 0 fail**；
  `internal/regexscan` 无独立测试文件，不计作测试通过。按 CI 的
  `go vet -composites=false` 通过，不宣称严格 vet 无既存警告。
- 随包真实 gopls 的未保存 Unicode 补全／格式化、重命名、定义／类型／实现／返回
  导航三项集成测试均 pass，无 skip；原生 GUI 子进程路径也指向本轮 app 的 tools/gopls。
- 从 Welcome：Alt+V → `all` → Enter → `ppp` 定位 Properties，Enter 进入 Design；
  Widgets 的 `wid` 入口也进入 Design，面板图标可见。`sea` 恢复 Search 并进入 Edit。
- Search 查询 foo，F4 跳转、输入 q／保存，实际文件为 `中🙂 qfoo_12 foo_34\n`；
  Undo／保存恢复 `中🙂 foo_12 foo_34\n`，两份均保存原始字节。
- 在 main.go 中输入 `outp` → Backspace → `l`，选择 Outline。18 项面板即使均为
  open/checked 状态，也仅当前键盘目标显示亮蓝底；其余用浅底／勾号，保留原图标。
  原生截图进一步发现勾号与最长标题太近，已增加间距并加入最少 4 px 的几何断言，
  重新完整回归与重新启动隔离 GUI，而非沿用之前构建结果。
- 10 次菜单键入恢复，随后一万条结果下连续 21 次 F4，光标第 1 行第 41 列。
  静置 10.010 s，CPU +0.130 s，约 **1.30% 单核**；无卡死观察，不推导所有场景无卡顿。
- 1,000 项菜单查找最后一项基准：**30,312 ns/op，0 B/op，0 allocs/op**，
  只代表查询 helper，不当作完整菜单绘制／事件延迟。
- 全部夹具原始字节恢复，Cmd+Q 正常退出，IDE PID 742／gopls PID 748 均已消失。
  两条模式入口均保存完整窗口截图；另重跑 Widgets 路径确认 Design 状态。

### Windows 10

- Windows 10 22H2 原生 cgo/race，11 个含测试包：**5,185 pass / 62 skip / 0 fail**。
  测试不是交叉编译；GUI 在交互桌面 Session 1 运行，非 SSH Session 0。
- 与 macOS 相同的三项随包真实 gopls 集成测试各 pass，无 skip。GUI 的实际子进程为
  `C:\silk\qa-menu-navigation-20261024-codex\apps\tools\gopls.exe`；启动环境不存在全局 gopls。
- Alt+V → `all` → Enter → 连续三次 p：Properties 图标与目标高亮可见，Enter 进入
  Design；从 Welcome 的 `wid` 同样进入 Design。`sea` 恢复 Search 进入 Edit。
- 查询 foo，F4／输入 q／保存和 Undo／保存均核验真实文件字节。菜单 `outp` →
  Backspace → `l` 定位 Outline，18 项 checked 状态与唯一当前目标互不混淆。
- 10 次菜单恢复后，一万条结果连续 21 次 F4，光标第 1 行第 41 列。20/20 次
  Alive／Responding 采样通过；静置 10.280 s，CPU +0.28125 s，约 **2.74% 单核**。
  没有用这个单次样本宣称相对旧版降幅或平台绝对性能上限。
- TXT／Go／dense 夹具恢复原始字节，Alt+F4 正常退出，IDE PID 9104／gopls PID 3924
  及本轮所有拥有进程均退出；4 个精确命名的测试计划任务已清理。

首个 Windows 桌面脚本在第二次保存记录时遇到 PowerShell Hashtable `+=` 重复键错误，
计划任务实际结果为 1，**未当作通过**。保留 `ui-menu-first.ps1` 与错误日志；显式
初始化记录数组，从 Welcome／原始文件字节重置，清空查询后完整重跑成功。没有因此
修改产品代码。脚本包装命令的退出码不能替代计划任务、标记文件、截图与字节断言。

### 自动测试命令与产物核验

```sh
GOMAXPROCS=2 go test -race -short -json -count=1 -p 2 \
  ./async ./core ./gui ./ged ./paint ./purecairo ./cmd/silkide \
  ./cmd/silktools ./internal/idetools ./internal/filetxn \
  ./filesearch ./internal/regexscan
```

Windows 对同一组包增加经过哈希核验的 `-overlay`，其他选项相同。
最终 1,524 个本地编译输入哈希不变；Windows 1,801 个不可变基线文件与 163 个 overlay
输入在构建前后／导出时校验。最后的勾号间距调整也重新跑了两平台完整相关包门禁，
没有用调整前的通过结果替代。两个平台各 2 份保存／恢复文件再次逐字节比对。

| 产物 | SHA-256 |
| --- | --- |
| macOS QA executable | `0488124eee7733e58f7f1c75b934174cf238c86d78272a8dd319478330454b92` |
| Windows QA executable | `1fcee420940061e4e2650b8fa4548dd374cdf3bb58850005a3a30dda0fbebd4f` |
| Windows evidence archive | `db8bebae63f014350fd4c3c53a69c168886862890951ea3f8b0d86731cc7093e` |

截图：`screenshots/silkide-menu-navigation-macos-20261024.png`、
`screenshots/silkide-menu-navigation-windows-20261024.png`。
`verify-final.py` 重新解析最终 race JSONL、gopls 结果、退出码、源码／产物哈希、实际
保存字节、响应样本与清理记录；窗口截图另已人工检查。

## 范围

`bin/qa-artifacts/menu-navigation-20261024/` 保存源码哈希、原始红测、race JSONL、
随包语言服务测试、原生截图、保存字节、CPU 与正常退出记录。

Unicode/IME 的已提交文本分发由单元测试覆盖；本轮真实桌面使用英文键盘，不据此
宣称覆盖各平台中文 IME 组合输入窗口。Linux 原生 GUI、超长菜单滚动、多显示器和
所有控件业务未覆盖。空闲 CPU 为一次样本，不构成所有机器上的卡顿保证。
