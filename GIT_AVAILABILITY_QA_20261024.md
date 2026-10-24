# Git 可用性、后台刷新与路径保真（2026-10-24）

## 本轮依据和改动

上轮 Windows 原生日志在启动及 8 次保存时，共记录 10 条缺少可选 Git 的 status/log 告警。实际调用链为保存 → `refreshGitChanges` → 每次启动独立 goroutine → `core.GitStatusPorcelain`；错误只记日志，面板仍像一个空列表。旧请求完成时只检查全局面板非空，未校验仓库或请求代次。

- IDE 更改/历史刷新改成每面板最多一个执行中查询、一个最新待处理请求。重复保存合并；旧查询完成后才启动替代查询。工作区切换取消旧任务，旧结果、关闭后的回调不能覆盖当前 UI。
- 缺 Git / 非仓库不随每次保存重试；面板明确显示原因和可点击、可键盘操作的 Retry。手动刷新、配置恢复或工作区变化可以重新查询；进行中的手动重试不会被旧失败吞掉。查询、失败、真实空结果与正常数据分开，不将工具错误画成“工作区干净”。
- SDK 新增 context 版本 Git status/history/root 读取、可识别错误、输出预算和超时；失败不返回部分列表。禁用只读后台查询的可选索引锁，不修改机器 PATH/全局 Git 配置，也不自动安装 Git。
- 真实仓库负例复现：中文、引号/制表符/换行文件名被保留为转义字节，普通文件名中的 `->` 被误认为重命名。改成官方推荐的 porcelain v1 `-z`，保留路径原始字节；用真实仓库根路径处理从子目录打开的工作区。
- 两个 Git 面板增加共用加载状态 API，使用同一控件树内的 Label/Button，提供图标、可见文字及 Tab 焦点；禁止加载/失败时点击旧行或提交。反馈不注册全局 tooltip 引用，不创建独立窗口或常驻动画。
- 真实中文/特殊路径、非仓库/未有提交、上下文取消、超限输出、10,000 次请求合并、重试、旧工作区回调及窄尺寸布局都有针对性测试。
- 首轮 Windows 截图揭露 SDK `Label.SetWrap(true)` 原先没有参与绘制，显式换行也被画成方框。修复共享多行布局、按实际宽度软换行、逐行对齐与边界裁剪；短容器只绘制可见行，保留原单行光学居中。复用现有 `TextBlock` 线性分行，按文本、宽度、字体属性、主题及 wrap 状态缓存；重绘不重新测量文字。新增实际 Draw 记录测试，不再仅检查 Wrap 属性值。

## 验证状态

最终源码验证（包含 Label 修复）：

| 平台 / 检查 | 结果 |
| --- | --- |
| macOS arm64，7 包 `-race -short -count=1 -p 2` | **5,042 pass / 47 skip / 0 fail** |
| Windows 10 amd64，同组原生 race 回归 | **4,937 pass / 62 skip / 0 fail** |
| macOS 实际 `.app/Contents/MacOS` 随包 gopls | **3 pass / 0 skip** |
| Windows 实际应用目录随包 gopls（race 编译测试） | **3 pass / 0 skip** |
| macOS `silk_pure_go` Label / panel / controller race 子集 | 通过 |
| `make bundle-ide`、7 包 `go vet -composites=false`、`git diff --check` | 通过 |

7 包为 `internal/idetools`、`cmd/silktools`、`cmd/silkide`、`core`、`gui`、`ged`、`paint`。计数包含子测试；跳过项单独保存，不计为通过。Windows 两项真实 Git 仓库测试及三个 Label 实际绘制/缓存测试均明确通过，不依赖缺工具时的 skip。真实 gopls 集成仍验证未保存 Unicode 补全/格式化、重命名、定义/类型/实现/返回与 completed work。

### Windows 原生闭环

- 新配置、新工作区和核对过 SHA-256 的 exe 启动；gopls 子进程路径是应用旁 `apps\\tools\\gopls.exe`，没有借用全局 gopls。测试目录保留空格，Go 模块根位于 Git 仓库子目录。
- 在进程 PATH 中故意缺少 Git，Changes / History 显示实际错误；Retry 仍缺工具时保持失败状态。3 轮编辑/保存/撤销/保存后提示仍正确，不变成“工作区干净”。截图与运行时 A11y 文本同时检查。
- 缩窄至 1000 × 800，再恢复 1280 × 900：反馈标题/详情换行、Retry 图标/文本和按钮边界正确。单测另覆盖更小容器；不把单测等同于这些极小尺寸的原生 UI 验收。
- 仅将 QA 私有 Git 目录移入该进程已继承的 PATH 位置；**不重启 IDE**，点击 Retry 后出现真实更改行。点击仓库根下的中文文件（位于工作区外），实际打开正确文件；保存/撤销再增加 2 阶段。
- **8 个阶段**均下载磁盘副本，由独立 Python 逐字节和 SHA-256 校验。关闭中文文件后恢复基线、刷新，显示真实 clean 状态；历史第一行打开 QA baseline 的差异窗口并关闭。
- 原生 F12 跳至 `target.go` 的 `greeting`，Alt+Left 返回 `main.go`。随后正常 Alt+F4，IDE 与随包 gopls 退出；另开无 `.git` 的新目录，显示 No Git repository，确认没有自动初始化仓库，并正常关闭。
- 下载证据独立核对所有成功标记、同一 exe 哈希及最终进程为 0；清理本轮 7 个计划任务。Retry 的 Tab 焦点及键盘回调由单测验证，未单独声称完成原生 Tab 遍历。

### 性能样本与产物一致性

- Windows 最终原生窗口采样 10.246 秒：IDE CPU 时间 **0.140625 秒，约单核 1.37%**；gopls CPU 时间 **0 秒**。20/20 次已核对 HWND 的 `WM_NULL` 有界响应检查成功，同时进程 Responding 为 20/20。仅代表本次短样本，不是输入延迟、帧率、零 CPU 或长期不卡顿保证。
- macOS 缓存绘制微基准约 69 ns/op、0 B/op、0 allocs/op，使用记录型 Painter，仅衡量缓存/调度开销，**不代表原生 Cairo 绘制耗时**；约 64 KiB 文本重新分行约 9.4–9.6 ms。实际测量计数测试断言反复重绘不重测字体。
- Windows **1,801 个基线文件 + 78 个 overlay 输入**逐文件校验；本轮 **14 个代码输入**在测试后重新核对，无漂移。
- 最终 IDE SHA-256：`f1cc7c21dba43cbc5fb16a61e727a18194a0f2457ff562e91b9f6708329dd06a`。
- 随包 gopls SHA-256：`5c409131c93b3d7136145492c5a39a08972fc483660beb4d2125a25447a79502`。
- `windows-evidence.tar.gz` 下载后与远端校验一致：`ee9464efe9035ce4630429ffa720ed8b2acb3c8138afb2006d887193d6f8323b`。
- `verify-evidence.py` / `summary.json` 保存独立验证与计数；`visual-review.json` 记录实际看过的截图及未解决项；`increment.patch` 只比较本轮开始快照与改动，不混入此前工作区改动。

[Windows 最终 Git 状态与随包语言服务截图](screenshots/silkide-git-recovery-windows-20261024.png)

## 证据与范围

- 测试过程保留了两类失败：首轮真实截图确认 Label 不换行，修复 SDK 后重新构建与完整回归；随后 F12 检查失败，独立 `go list` 明确报 `malformed import path "gitqa/sub folder": invalid char ' '`，随包 gopls CLI 同样无定义目标。后者是 QA 样例将带空格目录放在模块内部造成，不冒充产品修复。样例改成该目录自身为模块根，仍保留空格路径和 Git 仓库子目录；增加 Go 包预检、按已核对 HWND 读取标题及有界等待，再从新工作区完整重放。
- `core/GIT_READ.md`：新增 SDK API 的资源预算、路径语义和宿主责任。
- 本轮证据：`bin/qa-artifacts/git-availability-20261024/`，修改前快照、真实路径负例、源输入哈希、测试与原生重放脚本均分别保存。
- Windows 之前没有 Git 可执行文件；本轮只在隔离 QA 目录解压官方 MinGit 2.56.0(3)，不安装到系统、不修改持久 PATH。ZIP 官方 SHA-256：`2e5619966e588fde5f8122c5557cad50c0fbafcaed736689f40ec1dfb1d78ea7`，下载后校验匹配。此工具仅是测试依赖，不加入 SilkIDE 产品包。
- [Git porcelain v1 文档](https://git-scm.com/docs/git-status#_porcelain_format_version_1)规定 `-z` 的目标/源顺序及无转义路径；[Git for Windows 官方发行页](https://github.com/git-for-windows/git/releases/tag/v2.56.0.windows.3)提供 MinGit 校验值。
- 本轮不声称完成所有 Git 写操作/pathspec 语义审计、全部 SCM 工作流或长期性能认证。后续仍应检查 staging/unstaging 对特殊 pathspec 的处理以及面板本地选择与实际 index 状态的一致性。
- 截图仍看到 Dock 标签在窄窗口下截断、部分底部标签被控制区挤占，以及差异视图新增行的文字/背景对比度待改善。这些不是本轮 Label 反馈布局的通过项，留待下一轮修复。
- 未做 macOS/Linux 原生桌面、混合 DPI、完整 IME、多屏或长期负载验收；没有审计所有历史元数据超长情况下的渲染成本。旧安装目录未自动升级，当前改动尚未发布。
- 保留此前工作区修改，未 commit / push / 发布；持续目标仍未完成。
