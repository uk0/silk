# gopls 交付与语言服务回归（2026-10-23）

## 根因与修复范围

这不是单纯的 Windows 测试机漏装：旧发布包明确不包含 gopls，`make ide` 只编译主程序，IDE 通过 PATH 查找服务器，Windows runtime CI 使用允许失败的 `@latest` 安装，真实语言服务测试在缺工具时跳过。上一轮 Windows QA 因而不能代表 LSP 已可用。

本轮修复：

- `tools/gopls-module` 独立工具模块（后续已消除源码目录与运行时二进制重名），固定 gopls **v0.24.0**、构建编译器 **Go 1.27.3**，保留 SDK 根模块 Go 1.26.10；不把新语言服务器的编译器要求强加给 SDK。
- `cmd/silktools` 构建/离线校验/修复工具目录，CGO=0；交付 SHA-256 清单、目标平台/架构/编译器元数据、实际链接模块的许可证。临时目录验证后替换，失败保留旧包，拒绝覆盖源码工具模块等非托管目录。
- `make ide` / `make bundle-ide` 和发布流水线交付 `tools/gopls[.exe]`；macOS `.app/Contents/MacOS/tools` 独立携带完整工具，不依赖包外 staging 目录。
- 完整构建通过 `RequireBundle=1` 强制随包工具；缺失、错误版本/架构/哈希/许可证明确报错，不以 PATH 掩盖坏包。仅源码裸构建允许开发者 PATH 回退；`SILKIDE_GOPLS` 为显式绝对路径覆盖。
- Explorer/Finder 启动检查已知 Go 安装位置；保留显式 PATH 优先级、不修改系统 PATH、不递归扫描用户目录。gopls 已存在但 Go SDK 不存在时单独说明原因。
- 新增“工具链状态”菜单和命令面板项，展示 Go/gopls/Git/Delve 的路径、来源、版本与连接状态；后台有界探测，不阻塞绘制，重复点击合并。报告可滚动、选择复制、只读，长路径不覆盖关闭按钮；同组提供重启 LSP。
- PR、release、Windows runtime CI 使用真实随包 gopls 硬门禁，不允许缺工具跳过。安全扫描单独覆盖 gopls 二进制，不再仅扫描 SDK 根模块。

版本依据：[gopls v0.24.0](https://go.dev/gopls/release/v0.24.0)、[gopls 安装及工具链要求](https://go.dev/gopls/)、[Go 下载版本](https://go.dev/dl/)。首次构建可能下载固定工具链和依赖；IDE 启动不执行 `go install`，已验证的工具缓存可以离线复用。

## 已执行的本地验证

- macOS arm64：5 包 `-race -short -count=1`：**1,158 pass / 10 skip / 0 fail**，回归通过：`cmd/silkide`、`core`、`diagnostic`、`internal/idetools`、`cmd/silktools`；新增宿主文件权限用例单独重跑通过。
- 从真实 `.app/Contents/MacOS` 路径执行 3 项真实 gopls 集成：未保存 Unicode 内容补全/格式化、含空格和中文文件名的重命名后定义跳转、类型/定义/实现查找与返回，全部通过，未跳过。
- 构建并校验 Windows amd64、Linux amd64、macOS amd64/arm64 四种 gopls 产物。13 项真实产物校验覆盖重定位、空格/中文路径、二进制篡改、缺清单/许可证/二进制、错误架构、超长清单和恢复。
- 四种 gopls 二进制执行 `govulncheck -mode=binary`，当次数据库结果均为 `No vulnerabilities found`。这不是未来漏洞或整个 SDK 的无漏洞保证。
- 平台/branding 打包脚本、shell 语法、4 个 workflow 的 YAML 解析、工具包 vet 通过。

## Windows 真机

测试目录 `C:\silk\qa-gopls-20261023-codex`，源码逐文件 SHA-256 回读验证，禁用测试缓存，使用 Go 1.26.10、UCRT64/Cairo 和独立数据目录。不安装全局 gopls，不修改持久化 PATH。

首次回归发现 Windows 文件系统没有 POSIX 执行权限位，跨平台校验用例错误地拒绝 Unix 目标。现改为仅在 Unix 宿主/目标检查执行位；Windows 仍检查普通文件、构建元数据及 SHA-256，已补回归用例。首次失败日志保留，不计入最终通过结果。

- 最终 5 包普通回归：**1,132 pass / 35 skip / 0 fail**；同一组 race 回归：**1,132 pass / 35 skip / 0 fail**。标准 short/平台条件 skip 不当作通过。
- 另行执行 3 项真实随包 gopls 集成：**3 pass / 0 skip / 0 fail**，覆盖 Unicode 未保存内容、补全/格式化、文件重命名、定义/类型/实现跳转与历史返回。
- Session 1 桌面从不含 Go/gopls 的 Explorer 风格 PATH 启动；运行时找到 Go 1.26.10，gopls 子进程路径确认为 `apps-ui\tools\gopls.exe`，完成 initialize 和 workspace 加载。
- 命令面板打开工具链状态窗口，显示 connected=true / bundled / v0.24.0；窗口截图确认长路径换行与滚动条不覆盖关闭按钮。Ctrl+A/C 复制报告内容回读校验，Esc 关闭后正常回到编辑器。
- 实际打开 `main.go`，F12 从 `greeting()` 调用跳到 `target.go` 声明，Alt+Left 返回原位置；Tools 菜单中的状态/重启命令及图标可见。
- 点击 Tools > Restart LSP 后，新 gopls 进程建立，旧进程退出回收，ready 次数递增；已打开文档上的 F12 仍能跳转。
- 最后复查发现后台探测输出缓冲区若嵌入 `bytes.Buffer`，`io.Copy` 会通过提升的 `ReadFrom` 绕过 4 KiB 限额。新增反例先复现保留 20,000 字节，再改为组合封装；macOS/Windows 原生 race 下 4 项工具状态测试均通过，并重建最终 IDE，再次桌面启动、复制回读工具链报告、关闭程序验证通过。
- 真实 LSP 补全/格式化由上述协议集成测试断言。桌面另捕获到补全弹层，但单凭弹层不能区分本地 fallback 与 gopls 响应，**不将该截图额外算作 GUI 端 gopls 补全验收**。
- 实际点击原生关闭按钮退出，IDE 与其 gopls 子进程均退出；保留测试目录，不终止其他应用。

测试适配器第一次读取原生窗口标题时因 skill 的 `GetWindowTextW` P/Invoke 缺 Unicode 声明，只读到首字母。修复仅限本轮 QA 的 `gui.ps1` 副本，原 skill 未改；使用正确标题和进程 ID 后重新读取、截图、复制校验成功。这不是 IDE 标题截断。

## 重放

```sh
make bundle-ide
bash scripts/test-bundled-gopls.sh bin
# macOS app-relative resolution
bash scripts/test-bundled-gopls.sh bin/SilkIDE.app/Contents/MacOS
# Other target metadata can be verified without executing the foreign binary.
go run ./cmd/silktools -goos windows -goarch amd64 -out bin/windows-tools
go run ./cmd/silktools -verify -goos windows -goarch amd64 -out bin/windows-tools
go test -race -short -count=1 ./internal/idetools ./cmd/silktools ./cmd/silkide ./core ./diagnostic
```

原始日志、测试脚本、源码哈希清单、工具清单和截图位于本地 `bin/qa-artifacts/gopls-bundle-20261023/`，Windows 对应目录 `evidence/`。产物与下载缓存未纳入源码。

## 边界

- Go SDK 仍需安装；Git/Delve 为可选外部工具，未捆绑，不将其缺失描述为正常可用。
- Linux/macOS Intel 本轮完成工具交叉构建、完整性校验和漏洞扫描，**未在对应桌面运行 IDE/LSP**；CI 已添加硬门禁，但未执行远端 GitHub Actions。
- 完整性校验不是发行者签名/真实性验证；macOS 原生库打包、公证等原有发布限制未在本轮解决。
- 本轮未 commit、push 或发布。保留工作区上一轮未提交的 SDK/UI 修改。


## 后续检查：源码启动与状态提示（2026-10-23）

本次在既有打包修复上继续补齐三个遗漏，不把此前未提交的所有修改冒充本次新增：

1. 原工具源码目录 `tools/gopls` 与 Unix 运行时二进制同名。根目录裸构建时，真实 `idetools.Resolve` 报 `bundled gopls: not an executable file`，即使 PATH 已有可用服务器也不能继续。源码移至 `tools/gopls-module`，同步构建入口、固定版本校验和 README；新增三平台路径冲突回归。macOS 原生解析探针先失败，再在仅移动源码目录后得到正确的 PATH 结果。
2. 源码恢复提示不能只建议 `make ide-tools`：它仅修复 `bin/tools`，无法修复其他目录里的裸可执行文件。现在提示 `make ide` 并启动 `bin/silkide` / `bin/silkide.exe`，保持错误链可检查。
3. 工具链报告底部改为条件式恢复帮助，不再在 `connected=true` 时仍显示 `Missing gopls:`。新增状态报告回归。版本、SHA、平台、许可证、显式 override 和发布包强制随包校验未放宽。

### 最终验证

- macOS arm64：5 包 `-race -short -count=1`，**1,166 pass / 10 skip / 0 fail**。
- Windows amd64 真机：相同 5 包，**1,141 pass / 35 skip / 0 fail**。包组为 `cmd/silkide`、`core`、`diagnostic`、`internal/idetools`、`cmd/silktools`。
- 两个平台分别执行 3 项真实随包 gopls 集成，均 **3 pass / 0 skip / 0 fail**，包括未保存 Unicode 补全/格式化、重命名、定义/类型/实现跳转与返回。
- 从迁移后的工具模块向空目录重新构建 macOS arm64 / Windows amd64 gopls；二进制 SHA-256 与迁移前对应固定产物一致，不是仅命中现有输出目录的缓存校验。
- 原生解析探针验证：源码布局不再阻挡开发者 PATH；带 `RequireBundle=1` 的程序仍拒绝缺失工具目录，即使 PATH 有 gopls；补齐完整目录后返回 `Source=bundled`。
- `make bundle-ide`、branding 打包脚本、5 包 `go vet -composites=false` 通过。不是 strict-vet 全选项声明。
- 源输入 1,801 项在 Windows 构建前逐文件哈希验证，最终代码与测试快照一致；测试后只有 README 的说明文字变化。

### Windows 原生 UI 重放

不安装全局 gopls，不修改持久化 PATH。Session 1 从不含 Go/gopls 的 PATH 启动，实际 Go SDK 自动找到，实际语言服务器为测试应用旁的 `apps-final\tools\gopls.exe`。

最终干净重放完成：

- 打开 Go 文件，命令面板打开 Toolchain Status，截图检查报告布局、滚动区域与 Close 按钮；Ctrl+A/C 复制回读确认 connected=true、bundled、v0.24.0 和新版恢复说明。
- F12 跳至 `target.go`，Alt+Left 返回 `main.go`。
- 点击 Tools > Restart LSP：旧 gopls PID 8364 退出，新 PID 10000 建立，ready 次数 1 → 2；再次 F12 成功。
- 原生键盘插入注释、Ctrl+S 落盘逐字回读，撤销后再次保存恢复原文。
- 10.198 秒空闲观察，IDE 进程 CPU 时间 0.28125 秒，约 **2.76% 单核占用**，20/20 次 Responding=true。这不包含 gopls 子进程 CPU，也不是输入延迟/FPS 或长期性能认证。
- 正常点击关闭，IDE/gopls 均退出；再按测试可执行路径枚举，测试应用及语言服务器（包括派生进程）均无残留。

保留了两次 QA 适配失败，未将其计入通过：

- 旧输入法重置 helper 额外发 Ctrl+Space，触发 IDE 的补全列表；后续鼠标点击接受 `type` 候选并修改了测试文件，导致 F12 检查失败。仅修改本轮 `gui.ps1` 副本，保留 WM_INPUTLANGCHANGEREQUEST、去掉 Ctrl+Space 副作用；撤销并回读恢复测试文件后关闭。
- 第一次重放脚本未创建新的证据输出目录，日志未落盘，误报握手未就绪；实际进程已启动。先关闭该次拥有的 IDE/gopls/telemetry 进程，再创建证据目录并从新工作区与新数据目录完整重放。原始错误日志保留。

### 证据与边界

- 本地：`bin/qa-artifacts/gopls-followup-20261023/`；远端：`C:\silk\qa-gopls-followup-20261023-codex`。
- `source-final-manifest.json`、`increment.patch`、`summary.json`、`evidence-final.tar.gz`；最终原生测试位于 `windows-final/evidence-final`，成功 UI 重放位于 `windows-final/evidence-ui-replay2`。
- 最终 Windows IDE SHA-256：`accb129b87538b9bf983dc3f9681823a705464f2a2944482f2a81c126668edc0`；随包 gopls：`5c409131c93b3d7136145492c5a39a08972fc483660beb4d2125a25447a79502`。
- 当前随包语言服务已验证，不代表此前的大文件快速 Ctrl+S 不稳定项已修复；详见 `EDITOR_SAVE_EVENT_QA_20261023.md`。
- 本次未重跑 Linux 桌面、真实中文 IME、所有控件组合及全库测试；未 commit、Git push 或发布，旧安装目录不会自动升级。
