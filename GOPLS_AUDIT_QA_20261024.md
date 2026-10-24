# gopls 交付复查与兼容性加固（2026-10-24）

## 为什么会缺少

此前发布链只编译主程序、依赖 PATH 查找 gopls，缺工具时真实集成测试允许跳过。当前工作区已经加入固定工具模块、随包 `tools/gopls[.exe]`、清单/哈希/许可证校验、完整包强制解析、工具链状态与重启入口；具体历史修复见 `GOPLS_BUNDLE_QA_20261023.md`。这些修改尚未发布，不能据此宣称旧安装目录已经升级。

## 本次新增修复

- **强制测试不能跳过**：`SILK_TEST_REQUIRE_BUNDLED_GOPLS=1` 现在也隐含“必须存在服务器”。此前单独设置该开关时，缺工具或 `-short` 仍可返回 SKIP/PASS。新增子进程反例覆盖两个开关与 normal/short 四种组合，验证缺工具必须 FAIL，且不是测试自身启动失败。
- **CPU 基线可移植**：工具构建显式固定 `GOAMD64=v1` / `GOARM64=v8.0`，排除开发机高阶 CPU 参数，并在二进制构建信息验证中拒绝非基线产物。原校验只看 GOARCH，可能放过无法在老 CPU 上运行的工具。依据 [Go 官方最低硬件要求](https://go.dev/wiki/MinimumRequirements)，不盲目放宽校验或把兼容问题隐藏成 PATH 回退。
- **默认验收入口**：`make test-ide` 依赖真实随包 LSP 集成；新增 `make test-ide-lsp` 独立入口。`make bundle-ide` 除 staging 目录外，还校验 macOS `.app` 内最终工具目录。
- 不修改全局 PATH、不自动安装外部软件，不将 Go SDK/Git/Delve 冒充为已捆绑组件；不随意升级已经固定并验证的 gopls 版本。

## 本地验证

- 新增反例先失败：非基线 CPU 产物被接受、构建参数泄漏、bundled-only 测试跳过；修复后对应 race 测试通过。
- macOS arm64：7 包 `go test -race -short -count=1 -p 2`，**4,994 pass / 47 skip / 0 fail**（包含子测试），7 包均通过。包组：`internal/idetools`、`cmd/silktools`、`cmd/silkide`、`core`、`gui`、`ged`、`paint`。skip 不当作功能通过。
- `make bundle-ide` 通过；实际 `.app/Contents/MacOS` 目录执行真实 gopls 集成 **3 pass / 0 skip**。`make test-ide-lsp` 再从普通 bin 目录执行 **3 pass / 0 skip**，覆盖未保存 Unicode 补全/格式化、重命名、定义/类型/实现与返回。
- 注入 `GOAMD64=v4 GOARM64=v9.0`，向空目录构建 Windows amd64 和 Linux amd64 工具；产物均记录 `GOAMD64=v1`。Windows gopls SHA-256 与既有固定基线产物相同：`5c409131c93b3d7136145492c5a39a08972fc483660beb4d2125a25447a79502`。
- branding 打包脚本、7 包 `go vet -composites=false`、`git diff --check` 通过。不是 strict-vet 全选项声明。

## Windows 验证

- Windows 10 22H2 / amd64：同组 7 包 race 回归，**4,866 pass / 85 skip / 0 fail**（包含子测试）。
- 只设置 `SILK_TEST_REQUIRE_BUNDLED_GOPLS=1`，运行 race 编译的真实随包集成：**3 pass / 0 skip / 0 fail**。
- 源基线 1,801 文件 + 43 overlay 输入均逐文件验证。最终 IDE SHA-256：`3d819c7e2d5df749860ede2c483489ea6a26be4f1ce03c9796c3b2f68a7a3b33`。
- Session 1 原生桌面，以无 Go/gopls 的继承 PATH、新配置目录、新项目和故意缺少 `tools` 的独立应用目录启动。状态报告复制回读确认 `connected: false`，明确显示 `tools directory required`；无 gopls 子进程，编辑器仍可打开文件。
- 补入已验证的工具目录，**实际点击 Tools > Restart LSP**：状态报告回读确认 `connected: true`、`[bundled]`、`v0.24.0`，子进程路径是该应用旁的 `tools\gopls.exe`。截图检查菜单图标、报告滚动区域和 Close 按钮无覆盖。
- 在编辑器内定位 `greeting`，F12 跳至 `target.go`，Alt+Left 返回；再次点击菜单重启，旧 gopls PID 1940 回收、新 PID 8404 就绪，F12 仍成功。
- 原生输入注释、Ctrl+S 保存逐字读回；Ctrl+Z 撤销后再保存，原文逐字恢复。截图中的注释补全显示 `gopls: no matches; local suggestions`，不把它算作原生 gopls 补全验收；真实 gopls 补全/格式化由上述协议集成断言。
- 10.185 秒空闲样本：IDE CPU 时间 0.109375 秒，约 **1.07% 单核**；gopls CPU 计数器未增长，20/20 次 Responding=true。这不是 FPS、输入延迟或长期性能承诺。
- 正常 Alt+F4 关闭；全部本轮应用目录下进程为 0，清除本轮 5 个交互任务。未终止其他应用。

QA 脚本纠正记录（不算 IDE 修复或通过）：PowerShell JSON 数组取值最初误判哈希；初版错误地在 stderr 等待实际写入 UI Log/Toast 的启动错误；一次从 Tools 返回后未聚焦编辑器，Ctrl+G 未移动光标，F12 仍针对第 1 行。保留原始日志/截图，正常关闭对应进程；最终从新的缺工具目录与新的配置/项目目录重放，读取真实状态报告，并明确点击编辑器内的目标标识符。

## 证据与范围

- 本地原始证据：`bin/qa-artifacts/gopls-audit-20261024/`，包含 before 快照、本轮 `increment.patch`、negative/positive 日志、race JSON、打包与集成日志、43 个 Windows overlay 输入哈希。
- 最终 Windows 证据包 `final-evidence.tar.gz` 已与远端核对 SHA-256：`a7bd2150a38b5bc52bd848e8e5bc839d93f01fd06167dfe4aebcf648b3c0ac81`；汇总 `summary.json`，原生截图在 `windows-final/ui-evidence/`。
- Windows 基线为不可变的 1,801 文件源码快照，叠加当前 43 项增量；构建前逐文件校验，不使用遗漏工作区变更的 `git archive HEAD`。
- 上一轮尚未实现的多光标命令红阶段测试草稿，由本代理保留于 `bin/qa-artifacts/editor-commands-20261024/deferred/`，不混入本次 gopls 交付，也不将该未完成功能宣称完成。
- macOS 本轮是构建、单元和协议集成，未做桌面交互。Linux 本轮只有工具构建和静态元数据校验，未测试 Linux 桌面；Windows ARM64、真实中文 IME、所有控件组合未覆盖。本轮未重新扫描 CVE，不能将昨日扫描当成今日结果。
- 未 commit、Git push 或发布；保留此前所有无关工作区修改。
