# Dock 标签溢出与差异视图可读性（2026-10-24）

上一轮 Windows 截图确认两个未解决问题。本轮先复现负例，再修改 SDK，而非只挪动 IDE 的某几个按钮。

## 已确认原因与实现

- Dock 原来按所有标签的自然宽度布置条带，不预留右侧最小化/最大化控件。400 px 面板中，标签条实际宽约 666 px、窗口按钮从 348 px 起，出现覆盖和不可达标签。现在使用真实可用宽度、受限的窗口按钮边界及 TabBar 视口。
- TabBar 增加 All tabs 入口，列出全部完整标题并标记当前项；按对象身份激活，菜单打开后标签删除/重排不会误开替代项。选择器复用一个有 parent 的菜单，不进入全局 tooltip 表。复查发现 `ShowAsPopup` 非阻塞，立即 defer 解除 parent 会丢失逻辑归属；增加失败反例后改为持有至 TabBar 生命周期。Windows 旧实现的原生 HWND owner 与 resize 自动关闭实测已经正确，不将逻辑风险夸大为已复现的原生悬空。当前标签自动滚入可见区；文字按实际宽度省略，绘制、点击、关闭与拖动插入使用同一滚动坐标。
- Windows 截图还确认：弹出菜单绘制忽略 `Action.IsChecked()`，当前标签虽有状态但没有视觉标识。增加真实 Draw 反例后，使 checked 项使用既有菜单选中色及文字色，保留图标；原来的 blue-500 配白字仅 3.68:1，改用 blue-600 配白字；明暗主题均验证至少 4.5:1。
- 新增鼠标滚轮、Left/Right、Home/End 与 Down 打开选择器；TabBar 可获取键盘焦点，Ctrl+Tab 保留切换能力。极窄尺寸仍有受限的选择入口，隐藏标签不响应视口外点击。
- Tab 尺寸按标题、图标、字体及主题几何缓存；前缀位置缓存与二分定位首个可见项，绘制只访问可见单元格，不按打开文档总量重绘所有标题。
- 差异视图原先在深色主题上叠加浅色半透明红/绿底；实际 Draw 记录的新增行正文对比度约 1.64:1，头部约 1.19:1。新增主题语义色（新增/删除/头部底色、行号和动作文字），默认明暗主题各类正文/行号/动作均以至少 4.5:1 为回归门槛。
- 按列裁剪差异正文，头部文本为动作区留出范围；行号缓存随数据更新，不再每次 Draw 遍历全部行并分配两份数组。行号栏按位数扩展。

## 验证

- macOS arm64：显式 7 包 `-race -short -count=1 -p 2`，**5,051 pass / 47 skip / 0 fail**；Windows amd64 真机同组 **4,946 pass / 62 skip / 0 fail**。包组为 `internal/idetools`、`cmd/silktools`、`cmd/silkide`、`core`、`gui`、`ged`、`paint`。条件 skip 与 pass 分开统计。
- macOS `make bundle-ide`、7 包 `go vet -composites=false`、`silk_pure_go` 的 Dock/Diff/Tab/checked-menu 定向 race 通过。
- 两个平台的真实随包 gopls 集成分别 **3 pass / 0 skip**，覆盖未保存 Unicode 补全/格式化、重命名、定义/类型/实现与返回；macOS 在实际 `.app/Contents/MacOS` 内执行，Windows 使用 race 编译的测试程序。
- Windows 构建逐文件验证 1,801 项不可变基线 + 88 项 overlay。最终 IDE SHA-256：`1967d1fd53ee0f9d4b794f55b639720b62802de7e26899bb54da34188192843d`。
- 无实际栅格化的 Painter 基准：1,000 / 100,000 行视口绘制约 9.76–9.92 / 10.07–10.11 µs，分配均 49 次；10,000 tabs 仅绘制可见项约 6.12–6.32 µs，9 次分配。这是遍历/分配基准，不是原生绘制耗时、FPS 或输入延迟。

### Windows 原生桌面闭环

- 新配置、新 QA 仓库、模块目录带空格；实际 gopls 子进程来自 `apps\tools\gopls.exe`，非 PATH 服务器。完整包、源码、进程记录和 UI 结果中的 IDE SHA 一致。
- 打开 12 个文档，在 1280×900 / 1000×800 两种尺寸检查活动标签、All tabs 与最小化/最大化按钮；菜单列出全部标题，当前项可见选中。实际鼠标拖动将 notes12 放到 notes11 前，菜单顺序及 Home/End/Left 后的窗口标题确认正确页面。
- 从 Dock 选择器选择 Git Changes，实际点击仓库根目录的中文带空格文件（位于工作目录之外）；3 次普通文件保存/撤销、1 次中文文件保存/撤销，共 **8 个阶段**逐字节读回并验证 SHA-256。F12 从 main.go 跳到 target.go，Alt+Left 返回。
- 实际打开同一个 Git 提交的差异窗口；切换 Appearance 中的主题后分别截图。两种主题正文、行号清晰，长行只绘制在自身列内。浅色截图中红/绿底像素另做实际颜色检查，不以“点了 Apply”代替已生效。
- 10 次菜单打开/Esc 关闭，原生 HWND owner 始终为主窗口；GDI **22→22**、USER **34→34**。再次打开后缩小主窗口，菜单确实关闭。这只证明本轮选择器场景，不等于既有全部 GDI 问题已解决。
- 10.351 秒无输入采样，IDE CPU 时间 0.266 秒（约 **2.57% 单核**），gopls 1.641 秒（约 **15.85% 单核**），已知主窗口句柄 **20/20** 次响应 WM_NULL。该样本不能证明语言服务已静默、FPS 达标或长期不卡顿；gopls 后台活动需要后续关联诊断。
- 正常 Alt+F4 关闭，全部本轮拥有的应用进程为 0；清除本轮 11 个交互任务，保留 QA 目录，不停止其他应用。

QA 适配器纠正：首次拖动后的标签视口改变，旧坐标点到了 notes10；菜单顺序证明拖动已成功，按最新截图修正坐标后继续验证。跨任务时前台窗口不一定是模态窗口，改为按当前 IDE PID + 标题定位原生对话框。初次点击了可编辑 ComboBox 的正文而非下拉箭头，仍为 Dark；实际展开列表、点击 Light 后重放并验证像素颜色。上述失败日志/截图保留，不当作产品失败或成功结果。

### 证据

- 原始日志、反例、重放脚本、before 快照、源码哈希、`increment.patch`、`summary.json`、`visual-review.json`：`bin/qa-artifacts/dock-diff-polish-20261024/`。
- Windows 导出包 `windows-evidence.tar.gz` 与远端逐字节 SHA 核对：`f0a3073488d8c2375e26f701a4dbdf73759fd30a701e220cb012d5e72bd11b36`。
- `verify.py` 独立验证 7 包计数、必须通过的用例、3+3 真实 gopls 集成、8 次精确保存内容、源码/二进制哈希、popup owner 与清理结果。
- 截图：[窄窗口标签](screenshots/silkide-dock-overflow-windows-20261024.png)、[深色 Diff](screenshots/silkide-diff-dark-windows-20261024.png)、[浅色 Diff](screenshots/silkide-diff-light-windows-20261024.png)。

## 范围

- 不声称完整的 Qt Creator 对等性、全部控件零缺陷或长期不卡顿。
- Diff 的完整 LCS 构建仍是二次复杂度，本轮优化的是重复绘制及其可见区开销；大型完整文件比较的算法/异步化与横向浏览仍需要后续迭代。
- 当前 Diff 测试验证默认明暗主题；任意自定义颜色组合不自动保证对比度。浅色整机截图仍显示 Git History 内容区和编辑器 sticky header 保留深色，后续应统一主题，不将本轮 Diff 修复冒充整个 IDE 已完成浅色适配。
- 拖动日志仍有 `apps/cursor` 缺失告警，实际拖动重排成功，但自定义拖动光标未验收；截图不包含鼠标指针，应独立审计资源/fallback 路径。
- macOS 本轮为原生构建、race 与协议集成，没有重新做桌面点击；Linux、Windows ARM64、真实中文 IME、高 DPI 不在本轮验证范围。
- 本机根目录 `silkide` 仍是旧二进制（观察到时间为 2026-09-11），未擅自覆盖。当前产物为 `bin/silkide` / `bin/SilkIDE.app`；修复源码不会自动升级旧安装目录。
- 本轮原始记录：`bin/qa-artifacts/dock-diff-polish-20261024/`。保留上一轮 195 项既有工作区变化，没有 commit、push 或发布。
