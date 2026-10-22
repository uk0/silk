# 138cea8 Review 修复与 Go 漏洞检查（2026-10-22）

基线：`138cea8a6ad36f35e8492df41a42560e6732b6ff`。对应用户提供的
`CODE_REVIEW_138cea8_20261021.md`。本报告区分源码/包测试与原生桌面验收，
不将历史 Windows/macOS QA 当成本轮结果。原 review、不相关的 `gen_image/`、
根目录旧 `silkide` 二进制和用户数据均未修改。

## 逐项处理

| # | 调整 | 回归证据 |
| --- | --- | --- |
| 1 | 属性配置使用 `errors.Is(err, os.ErrNotExist)` 识别诊断包装后的缺失文件；检查相关读取调用方 | `TestPropertyConfigurationUserOverlay` 基线失败，修复后通过 |
| 2 | `TDocIni` 将初次读取错误返回给调用方；新增 `SyncError`，初始/重载失败禁止覆盖原文件，失败不推进同步时间；写入前重新验证磁盘内容 | `TestIniReadFailureNeverReplacesOriginal`、`TestIniReloadFailureProtectsExternalEdit` |
| 3 | 零字节设置视为空文档；解析损坏文件先创建并同步精确保留原始字节的 `.corrupt-*` 副本；备份失败/权限/特殊文件仍阻止写入；支持解析并固定符号链接目标；`Reload` 在修复后恢复，并保留失败读取期间的内存编辑；IDE 日志提示备份位置 | `TestSettingsCorruptOriginalIsBackedUpBeforeRecovery`、`TestSettingsEmptyAndSymlink`、`TestSettingsReloadAfterReadErrorPreservesRuntimeEdits`、gzip CRC/截断测试 |
| 4 | 两层异步保存器即使本次写入失败也继续执行已排队的最终请求；没有新请求时不无限重试，返回最新结果 | `TestSettingsQueuedSnapshotSurvivesWriteFailure`、`TestPreferencesShutdownRetriesQueuedFinalSaveAfterFailure`；原 `finishPreferenceSaves` 已记录退出错误，未重复添加日志 |
| 5 | 识别文件系统不支持原子防覆盖重命名；默认仍 fail closed，资源管理器提供明确确认后的兼容重命名；不把权限错误降级 | `TestCompatibleRenameRequiresUnsupportedAndProtectsExisting`、既有并发目标冲突测试。外部进程的 TOCTOU 风险见下文 |
| 6 | 两项相对路径测试先 `t.Chdir` 到临时目录，不再假设 checkout/TEMP 同盘符 | `TestProjectFilesCancellationAndRelativeRoot`、`TestSnapshotLongUnicodeAndRelativeNames` |
| 7 | AST 未找到声明时，以 Go scanner 恢复后续命名函数/方法，避免正则误认注释与字符串 | `TestDefinitionRecoveryAfterUnfinishedBody`：不完整 if、赋值、复合字面量、未闭合块，以及注释/原始字符串负例 |
| 8 | 区分 hover 与已展示的签名帮助；参数输入、Shift 不再清除签名，Esc、结束括号、导航/失焦仍清除；异步回复仍有文本/交互版本检查 | `TestSignatureHelpSurvivesArgumentInputAndModifiers`、既有 hint snapshot/lifecycle 测试 |
| 9 | Cocoa `hasMarkedText` 检查发生在 GLFW 按键提交/取消/快捷键分发前；主线程外不查询原生窗口 | `TestComposingEnterCannotSubmitInputBox` 验证确认候选→插入文本→再次 Enter 提交顺序；**未等同于真实日文/中文 IME 桌面实测** |
| 10 | 去掉 Quick Open 对所有 `local` 目录的全局屏蔽 | `TestQuickOpenIncludesUserLocalDirectories`：根目录及 `pkg/storage/local` 都可索引 |
| 11 | `ProblemsPanel.SetOutput` 只替换 BuildSource，保留 LSP、Macro、Load 等独立来源 | `TestBuildOutputPreservesOtherDiagnosticSources` |
| 12 | 未分配尺寸时延迟折行；每个逻辑行只计算一次 glyph，再切分/重定位显示行，消除重复处理剩余文本的二次复杂度 | `TestTextBlockLinearWrapPreservesUnicodeAndRows`、`TestMessageBoxDefersUnmeasuredWrap`，32 KiB 基准见下文 |
| 13 | 外部补全复制输入，文本修订/Undo/Redo 失效时关闭旧列表；光标变化后在合并与接受按键前清除过期项 | `TestExternalCompletionExpiresOnEditsAndUndo`、`TestExternalCompletionExpiresWhenCaretMoves` |
| 14 | 对链接/Windows irregular 入口仅查询目标类型以展示文件夹，不递归遍历，不向文件编辑器发送目录；提供链接目录说明 | `TestExplorerDirectoryLinksAreNotOpenableFiles`，保留原有不遍历外部目录及循环链接测试 |
| 15 | 树与 Git 使用独立取消/代次/投递通道；展开、过滤、隐藏项切换只更新树，旧徽标保留至新 Git 结果；输出 buffer 不再嵌入暴露 `ReadFrom` 绕过容量限制 | `TestTreeOnlyScanPreservesBadgesAndGitGeneration`、`TestExplorerOutputCapCannotBeBypassed` |

### 有意采用的安全边界

- **不将 GBK/损坏 UTF-8 静默转成 U+FFFD 再保存。** 严格解析仍保留，项目 INI 错误明确暴露且原文件不被空文档覆盖；偏好设置则先备份原始字节，再恢复默认值。
- **不静默把原子防覆盖降级成 `Lstat + Rename`。** 兼容模式仅在原子操作不受支持且用户确认后使用；同进程串行化、目标已存在/悬空链接保护仍在，但外部程序同时创建目标的竞态无法由该回退保证安全。默认 SDK `RenameNoReplace` 保持原保证。本轮未重挂载 exFAT/FAT 或验证 Linux NFS/CIFS。
- 设置 `Clear` 不会绕过读取/备份失败保护；磁盘错误必须修复，或由调用方处理，不以“成功保存”掩盖失败。

## 性能复测

同一台 macOS arm64 / Apple M2 Max / Go 1.26.10，`BenchmarkReviewMessageBox32K`：

| 路径 | 时间 | 累计分配/次 | 分配次数/次 |
| --- | ---: | ---: | ---: |
| 基线 `gui/edit.go` + `gui/textblock.go`（Go overlay，1 次） | 7.475 秒 | 13,769,751,840 B | 166,443 |
| 修复后（10 次均值） | 2.273 毫秒 | 1,963,939 B | 464 |

统计的是构造、设置 32 KiB 无空格文本、设置 620×280 尺寸；不是持续 FPS 或整机输入延迟保证。
基线文件由 `git show HEAD:<path>` 独立导出，不回滚工作树。每个逻辑行仅 shape 一次；窄到 1 像素也能保证前进与文本完整。

## Go CVE

扫描器 `govulncheck v1.9.0`，数据库 `https://vuln.go.dev`，本轮记录的数据库更新时间
`2026-10-21T00:27:29Z`。扫描发现基线 **36 个漏洞 ID 命中模块版本**；
在 macOS `./cmd/silkide ./gateway ./scada` 的源码调用分析中有 **11 个 ID 静态可达**。
这不是 36 个均可远程利用的声明，也不是已运行利用代码的证明。

升级：

| 组件 | 基线 | 本轮 |
| --- | --- | --- |
| Go 构建工具链 | 实际 1.26.5，go.mod 最低 1.26.0 | 最低/测试构建 1.26.10 |
| golang.org/x/image | v0.39.0 | v0.45.0 |
| golang.org/x/net | v0.52.0 | v0.60.0 |
| golang.org/x/sys | v0.42.0 | v0.48.0（同时满足上游依赖要求） |
| golang.org/x/sync | v0.20.0 | v0.23.0 |
| golang.org/x/text | v0.36.0 | v0.42.0 |

`mod/map` 同步更新依赖及校验和；CI/release/windows-runtime 使用 `go-version-file: go.mod`，
增加安全扫描工作流，避免旧版本工具链重新引入已修复标准库问题。
工具链最低版本声明不代表任意较新分支的早期版本都安全：如自行使用 Go 1.27，应至少采用 1.27.3，并扫描实际构建工具链。
Go 1.26.10 的安全修复见 [Go 官方发布记录](https://go.dev/doc/devel/release)，
最近的证书验证修复见 [GO-2026-6809 / CVE-2026-111943](https://pkg.go.dev/vuln/GO-2026-6809)，
图像解码内存消耗问题见 [GO-2026-6222](https://pkg.go.dev/vuln/GO-2026-6222)。

复扫结果：根模块版本扫描、上述应用入口源码扫描均 `No vulnerabilities found.`；
`mod/map` 两份 Go 源文件扫描无发现。该遗留模块名 `mod/map` 不能直接用于
`go/packages` 导入路径扫描，因此源码扫描明确传入 `map-scene.go map-view.go`。
其模块级扫描使用独立 manifest 快照，仅将主模块身份改为 `audit.silk.invalid/map` 并将本地 replace 改为绝对路径；
已比对其余 **33 个依赖的模块身份/选定版本完全一致**，此快照扫描同样无发现。原始模块名与运行时导入没有改名。
安全工作流中保留该规避及依赖图一致性断言，不将最初直接扫描失败计作通过。
macOS 与 Windows 原生 IDE 构建产物的 binary 模式扫描也无发现。JSON 扫描结果还检查了 `finding` 条目为 0；不能只看 JSON 模式的退出码。

范围限制：不覆盖原生 Cairo/GLFW/SQLite C 依赖、系统 DLL、外部安装的 gopls/dlv/打印驱动，
也不替代业务权限、协议和不可信项目执行边界的人工安全审查。旧安装包不会因为修改 go.mod 自动修复，必须重新构建并更新部署。

### 基线命中清单

| Go ID | CVE | Module | Fixed version | Reachable in baseline macOS entrypoints |
| --- | --- | --- | --- | --- |
| [GO-2026-4918](https://pkg.go.dev/vuln/GO-2026-4918) | CVE-2026-33814 | golang.org/x/net | v0.53.0 | not reported |
| [GO-2026-4961](https://pkg.go.dev/vuln/GO-2026-4961) | CVE-2026-33813 | golang.org/x/image | v0.42.0 | not reported |
| [GO-2026-5024](https://pkg.go.dev/vuln/GO-2026-5024) | CVE-2026-39824 | golang.org/x/sys | v0.44.0 | not reported |
| [GO-2026-5025](https://pkg.go.dev/vuln/GO-2026-5025) | CVE-2026-42506 | golang.org/x/net | v0.55.0 | not reported |
| [GO-2026-5026](https://pkg.go.dev/vuln/GO-2026-5026) | CVE-2026-39821 | golang.org/x/net | v0.55.0 | not reported |
| [GO-2026-5027](https://pkg.go.dev/vuln/GO-2026-5027) | CVE-2026-42502 | golang.org/x/net | v0.55.0 | not reported |
| [GO-2026-5028](https://pkg.go.dev/vuln/GO-2026-5028) | CVE-2026-25680 | golang.org/x/net | v0.55.0 | not reported |
| [GO-2026-5029](https://pkg.go.dev/vuln/GO-2026-5029) | CVE-2026-25681 | golang.org/x/net | v0.55.0 | not reported |
| [GO-2026-5030](https://pkg.go.dev/vuln/GO-2026-5030) | CVE-2026-27136 | golang.org/x/net | v0.55.0 | not reported |
| [GO-2026-5031](https://pkg.go.dev/vuln/GO-2026-5031) | CVE-2026-42500 | golang.org/x/image | v0.41.0 | not reported |
| [GO-2026-5032](https://pkg.go.dev/vuln/GO-2026-5032) | CVE-2026-46599 | golang.org/x/image | v0.41.0 | not reported |
| [GO-2026-5061](https://pkg.go.dev/vuln/GO-2026-5061) | CVE-2026-46601 | golang.org/x/image | v0.43.0 | not reported |
| [GO-2026-5062](https://pkg.go.dev/vuln/GO-2026-5062) | CVE-2026-46602 | golang.org/x/image | v0.43.0 | not reported |
| [GO-2026-5066](https://pkg.go.dev/vuln/GO-2026-5066) | CVE-2026-46604 | golang.org/x/image | v0.43.0 | not reported |
| [GO-2026-5942](https://pkg.go.dev/vuln/GO-2026-5942) | CVE-2026-46600 | golang.org/x/net | v0.56.0 | not reported |
| [GO-2026-5972](https://pkg.go.dev/vuln/GO-2026-5972) | CVE-2026-33818 | stdlib | v1.26.6 | yes |
| [GO-2026-6088](https://pkg.go.dev/vuln/GO-2026-6088) | CVE-2026-56859 | stdlib | v1.26.6 | not reported |
| [GO-2026-6089](https://pkg.go.dev/vuln/GO-2026-6089) | CVE-2026-56853 | stdlib | v1.26.6 | not reported |
| [GO-2026-6090](https://pkg.go.dev/vuln/GO-2026-6090) | CVE-2026-56862 | stdlib | v1.26.6 | yes |
| [GO-2026-6091](https://pkg.go.dev/vuln/GO-2026-6091) | CVE-2026-56858 | stdlib | v1.26.6 | not reported |
| [GO-2026-6218](https://pkg.go.dev/vuln/GO-2026-6218) | CVE-2026-56860 | stdlib | v1.26.6 | not reported |
| [GO-2026-6222](https://pkg.go.dev/vuln/GO-2026-6222) | CVE-2026-46603 | golang.org/x/image | v0.45.0 | not reported |
| [GO-2026-6599](https://pkg.go.dev/vuln/GO-2026-6599) | CVE-2026-94448 | stdlib | v1.26.9 | not reported |
| [GO-2026-6600](https://pkg.go.dev/vuln/GO-2026-6600) | CVE-2026-97030 | stdlib | v1.26.9 | not reported |
| [GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603) | CVE-2026-78659 | golang.org/x/net | v0.60.0 | yes |
| [GO-2026-6604](https://pkg.go.dev/vuln/GO-2026-6604) | CVE-2026-56857 | stdlib | v1.26.9 | yes |
| [GO-2026-6605](https://pkg.go.dev/vuln/GO-2026-6605) | CVE-2026-56866 | stdlib | v1.26.9 | not reported |
| [GO-2026-6607](https://pkg.go.dev/vuln/GO-2026-6607) | CVE-2026-97031 | stdlib | v1.26.9 | yes |
| [GO-2026-6608](https://pkg.go.dev/vuln/GO-2026-6608) | CVE-2026-94440 | stdlib | v1.26.9 | yes |
| [GO-2026-6609](https://pkg.go.dev/vuln/GO-2026-6609) | CVE-2026-78667 | stdlib | v1.26.9 | not reported |
| [GO-2026-6610](https://pkg.go.dev/vuln/GO-2026-6610) | CVE-2026-78660 | golang.org/x/net | v0.60.0 | not reported |
| [GO-2026-6611](https://pkg.go.dev/vuln/GO-2026-6611) | CVE-2026-78669 | golang.org/x/net | v0.60.0 | yes |
| [GO-2026-6612](https://pkg.go.dev/vuln/GO-2026-6612) | CVE-2026-78663 | golang.org/x/net | v0.60.0 | yes |
| [GO-2026-6613](https://pkg.go.dev/vuln/GO-2026-6613) | CVE-2026-94439 | stdlib | v1.26.9 | yes |
| [GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617) | CVE-2026-97032 | golang.org/x/net | v0.60.0 | yes |
| [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809) | CVE-2026-111943 | stdlib | v1.26.10 | yes |

## 验证记录

本轮本地证据：`bin/qa-artifacts/review-20261022/`（忽略目录，不提交二进制/临时用户目录）。

- macOS：42 个不同包的 CI 对应短测试命令通过（其中 2 个无测试）；涉及核心/GUI/设置/IDE 的最终重测通过。
- Race：13 个包的 `-race -short -count=1` 命令通过（1 个无测试）；最后增量再次验证 core/gui/ged，设置恢复增量单独 race 验证，最终候选再次覆盖 core/gui/ged/settings/cmd/silkide。
- CI 同配置 `go vet -composites=false` 通过；不带该配置的原始 `go vet` 仍报告既有未命名字段结构体字面量，不把它记录为通过。
- `CGO_ENABLED=0 -tags silk_pure_go` 的 cairo/purecairo/paint 验证通过。
- 原生 macOS GL 严格探针 `SILK_TEST_NATIVE_RENDER=1` 通过，实际 framebuffer 160×120，全帧 76800 B、局部更新 2688 B；不是模拟截图。
- `go mod verify` 通过；macOS IDE 使用 Go 1.26.10 构建成功。
- 安全工作流的完整 shell 步骤在本地执行通过，四次扫描均无发现，临时 manifest 与原始 map 依赖图一致性断言通过。GitHub 托管工作流本身尚未触发。
- Windows 10 22H2 / amd64 / Go 1.26.10：本轮七包原生测试通过，另以 `subst R:` 将 checkout 映射到 R:、TEMP 保留 C: 验证两项跨盘符相对路径测试通过。该映射验证盘符差异，不宣称是两个物理磁盘。最终 8 包原生测试（另含 win32）及 Windows GUI 子系统可执行文件构建成功；1697 份快照文件逐一 SHA-256 核对后才运行测试。此次没有新增 Windows 可视化点击/截图验收，也未运行 Windows Race；不把包测试当作完整桌面验收。

复现命令（源码目录不要递归包含私人/不可读的 `local/` 与 QA 产物）：

```sh
go test -short -count=1 ./prop ./core ./settings ./internal/fileops ./gui ./ged ./cmd/silkide
go test -race -short -count=1 ./settings ./core ./purecairo ./paint ./gui ./scada ./macro ./cmd/silkide ./ged ./prop ./internal/fileops ./ide/taskrunner ./internal/processutil
go test -run '^$' -bench BenchmarkReviewMessageBox32K -benchtime=10x -benchmem ./gui
SILK_TEST_NATIVE_RENDER=1 go test -count=1 -run TestGLFWNativeRenderTransfers -v ./gui
govulncheck -scan module
govulncheck ./cmd/silkide ./gateway ./scada
(cd mod/map && govulncheck map-scene.go map-view.go)
# Legacy mod/map module-only scan: see the isolated manifest and graph check
# in .github/workflows/security.yml.
```

由于当前工作区含旧 QA 的嵌套 Go 缓存与受保护数据目录，直接 `go mod tidy` 会遍历失败。
本轮在仅导出 tracked 文件与本轮新源码的独立快照中执行 tidy，再回写 go.mod/go.sum；未更改数据目录权限。

待补人工/环境验收：真实 macOS 日文/中文输入法候选确认、exFAT/FAT 兼容确认 UI、Linux 原生桌面以及多显示器输入路径。

### Windows 最终构建

- 隔离测试目录：`C:\silk\qa-20261022-review`，未覆盖用户正在使用的旧安装目录。
- 最终八包：`./core ./settings ./internal/fileops ./prop ./gui ./ged ./cmd/silkide ./win32`，`-short -count=1`，退出码 0。
- 原生构建：`go build -ldflags="-H=windowsgui" -o silkide-review.exe ./cmd/silkide`，退出码 0。
- Go 版本回读：`go1.26.10 windows/amd64`。
- 取回原生 Windows 可执行文件后的 `govulncheck -mode binary`：`No vulnerabilities found.`。
- 二进制 SHA-256：`bdcc4c690409412eddf43a8994d6926feac33ec5b9d1cfd6f68fda84c8801a65`。
- 源码快照清单：`bin/qa-artifacts/review-20261022/source-manifest.json`。测试后仅文档/安全扫描脚本调整不影响已测运行时代码。
- Windows 初次访问官方 Go proxy 超时，改用测试机既有可用的 Go proxy 下载；依赖保持 Go 模块校验机制。工具链/源码均未降级以绕过测试。
