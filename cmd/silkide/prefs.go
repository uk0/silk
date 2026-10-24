package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/settings"
)

// preferences keeps the user-visible IDE state that should survive
// across launches: window size, last opened directory, recent files.
// Backed by silk/settings (TDoc-on-disk under ~/Library/Application
// Support / %APPDATA% / ~/.config), so the persistence story is the
// same on every supported platform.
type preferences struct {
	store           *settings.Settings
	saveOnce        sync.Once
	writer          *preferenceWriter
	lastSaveMessage string // UI-owned error de-duplication
}

// newPreferences returns the shared silkide preferences instance. The
// underlying settings.Default("silk", "silkide") points at the
// canonical per-user file path; silkide itself doesn't have to know
// where that lives.
func newPreferences() *preferences {
	return &preferences{store: settings.Default("silk", "silkide")}
}

// WindowSize reads the saved window size, falling back to the JetBrains-
// style 1280x800 default that the unmodified silkide originally used.
func (p *preferences) WindowSize() (int, int) {
	w := int(p.store.Int("window/width", 1280))
	h := int(p.store.Int("window/height", 800))
	if w < 320 {
		w = 320
	}
	if h < 240 {
		h = 240
	}
	return w, h
}

// SetWindowSize updates runtime geometry and queues background persistence.
// Write errors are reported asynchronously; the runtime values remain usable.
func (p *preferences) SetWindowSize(w, h int) {
	_ = p.store.SetInt("window/width", int64(w))
	_ = p.store.SetInt("window/height", int64(h))
	_ = p.requestSave()
}

// WindowPos returns the saved window position. (-1, -1) means
// "nothing saved yet — the caller should centre the window".
// Negative coords are also reported as "no preference" so the IDE
// doesn't restore to off-screen positions left behind by an earlier
// monitor configuration.
func (p *preferences) WindowPos() (int, int) {
	x := int(p.store.Int("window/x", -1))
	y := int(p.store.Int("window/y", -1))
	if x < 0 || y < 0 {
		return -1, -1
	}
	return x, y
}

// SetWindowPos persists window screen position.
func (p *preferences) SetWindowPos(x, y int) {
	_ = p.store.SetInt("window/x", int64(x))
	_ = p.store.SetInt("window/y", int64(y))
	_ = p.requestSave()
}

// LastOpenedDir is the directory the next OpenFileDialog should start
// in. Empty means "cwd".
func (p *preferences) LastOpenedDir() string {
	return p.store.String("files/lastDir", "")
}

func (p *preferences) SetLastOpenedDir(dir string) {
	_ = p.store.SetString("files/lastDir", dir)
	_ = p.requestSave()
}

// RunArgs is the command-line argument string appended to `go run .`
// when silkide's Run action fires. Stored as a single raw string so the
// user can edit it as they'd type it on the terminal; splitRunArgs
// breaks it into argv when the runner spawns. Empty means "no args" —
// the previous `go run .` behaviour.
func (p *preferences) RunArgs() string {
	return p.store.String("run/args", "")
}

// SetRunArgs updates runtime arguments and queues a coalesced background save.
func (p *preferences) SetRunArgs(args string) {
	_ = p.store.SetString("run/args", args)
	_ = p.requestSave()
}

// RunWorkingDir is the project directory the runner should treat as
// "cwd" instead of the auto-detected projectDir. Empty means "use
// projectDir as-is" — backwards-compatible with pre-RunConfigPanel
// builds that only persisted run/args.
func (p *preferences) RunWorkingDir() string {
	return p.store.String("run/workingDir", "")
}

// SetRunWorkingDir persists the run working-directory override.
func (p *preferences) SetRunWorkingDir(dir string) {
	_ = p.store.SetString("run/workingDir", dir)
	_ = p.requestSave()
}

// RunEnv is the list of "KEY=value" environment variable lines forwarded
// to the spawned `go run`. Stored as a StringList mirroring RecentFiles /
// OpenSession. Empty means "no extra env" — matches the historical
// behaviour where the run worker inherited os.Environ() verbatim.
func (p *preferences) RunEnv() []string {
	return p.store.StringList("run/env", nil)
}

// SetRunEnv persists the run environment-variable list.
func (p *preferences) SetRunEnv(env []string) {
	_ = p.store.SetStringList("run/env", env)
	_ = p.requestSave()
}

// effectiveRunDir returns the directory the runner should use: the user-
// configured prefsDir when non-empty, falling back to the auto-detected
// autoDir otherwise. Pure helper so the override rule is unit-testable
// without spinning up a Frame or terminal panel.
func effectiveRunDir(prefsDir, autoDir string) string {
	if prefsDir != "" {
		return prefsDir
	}
	return autoDir
}

// recentFilesMax caps the rolling MRU list. JetBrains IDEs default to
// 50 but most users only ever scan the first 10, and the longer the
// list the slower the de-dup walk on every open.
const recentFilesMax = 10

// RecentFiles returns the most-recently-opened-first list of file
// paths. Capped at recentFilesMax entries. Stale entries (file no
// longer exists on disk) are filtered out before return — saves a
// round-trip stat on every menu draw and matches Qt Creator's
// "Recent Files" behaviour.
func (p *preferences) RecentFiles() []string {
	raw := p.store.StringList("files/recent", nil)
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, path := range raw {
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		out = append(out, path)
	}
	return out
}

// AddRecentFile pushes `path` to the front of the recent-files list,
// de-duping any prior occurrence. Caps to recentFilesMax. Empty
// paths are ignored.
func (p *preferences) AddRecentFile(path string) {
	if path == "" {
		return
	}
	abs := path
	if absPath, err := filepathAbs(path); err == nil {
		abs = absPath
	}
	cur := p.store.StringList("files/recent", nil)
	out := make([]string, 0, len(cur)+1)
	out = append(out, abs)
	for _, prev := range cur {
		if prev == abs {
			continue
		}
		out = append(out, prev)
		if len(out) >= recentFilesMax {
			break
		}
	}
	_ = p.store.SetStringList("files/recent", out)
	_ = p.requestSave()
}

// filepathAbs delegates to filepath.Abs but isolates the import so
// prefs.go's import block stays minimal — the only filepath need is
// here in AddRecentFile.
func filepathAbs(path string) (string, error) {
	return filepath.Abs(path)
}

// OpenSession returns the file paths that were open when silkide last
// closed: the active design canvas's .silkui (if any) plus every
// open editor-tab path. Order is preserved as written by
// SetOpenSession. Stale entries are NOT filtered here — restoreSession
// does that via existingPaths so callers that want the raw list (e.g.
// tests) still see it.
func (p *preferences) OpenSession() []string {
	return p.store.StringList("session/open", nil)
}

// SetOpenSession persists the set of currently-open file paths so the
// next launch can reopen them. Same string-list idiom as RecentFiles;
// background saving keeps the close callback responsive. The process joins
// the actual writer after the event loop returns.
func (p *preferences) SetOpenSession(paths []string) {
	_ = p.store.SetStringList("session/open", paths)
	_ = p.requestSave()
}

// existingPaths filters a session path list down to the entries that
// still exist on disk, preserving order and de-duplicating. Pure
// helper (no globals, no I/O beyond os.Stat) so session restore can be
// unit-tested without a frame. Empty / blank entries are dropped.
func existingPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

// installLocale picks the user's locale and registers the inline
// silkide translations for Chinese. Detection order:
//
//  1. SILKIDE_LOCALE env var (override for testing).
//  2. macOS AppleLocale via i18n's per-platform locale_*.go.
//  3. Fall back to English ("en") if neither resolves.
//
// The translations cover only the strings silkide itself emits —
// "Debug", status-bar cells, dialog titles. Library widgets that
// need their own strings translated should register in their own
// init().
func installLocale() {
	if v := appearanceLocale(globalPrefs); v != "" {
		switch strings.ToLower(strings.ReplaceAll(v, "_", "-")) {
		case "zh", "zh-cn", "zh-hans", "zh-sg":
			v = "zh-CN"
		}
		i18n.SetLocale(v)
	}
	registerSilkideTranslations()
	registerAppearanceTranslations()
}

// registerSilkideTranslations adds the silkide-internal Chinese
// translations to the default translator. Kept in code (not a JSON
// file) so the binary always has them — no separate asset to ship.
func registerSilkideTranslations() {
	i18n.Default.AddMany("zh-CN", map[string]string{
		"Expand Silk Macros":                       "展开 Silk 宏",
		"Open a saved .silkgo document first":      "请先打开已保存的 .silkgo 宏文件",
		"Save the macro source before expanding":   "请先保存宏源文件再展开",
		"Macro source changed; expand again":       "宏源文件已变化，请重新展开",
		"Macro expansion failed; output unchanged": "宏展开失败，输出文件未修改",
		"Macro expanded to standard Go":            "宏已展开为标准 Go 代码",
		"File":                                     "文件", "View": "视图", "Tools": "工具", "Help": "帮助",
		"Widgets": "控件", "Filter widgets...": "搜索控件...",
		"Search by widget or factory name": "按控件名称或工厂名搜索，拖入画布添加",
		"Object Inspector":                 "对象检查器", "Build Output": "构建输出",
		"Debug":                  "调试",
		"Run":                    "运行",
		"Build":                  "构建",
		"Welcome":                "欢迎",
		"Edit":                   "编辑",
		"Design":                 "设计",
		"Save All":               "全部保存",
		"Preview":                "预览",
		"Save":                   "保存",
		"Open":                   "打开",
		"Undo":                   "撤销",
		"Redo":                   "重做",
		"Refresh":                "刷新",
		"Retry":                  "重试",
		"Git unavailable":        "Git 不可用",
		"No Git repository":      "当前目录不是 Git 仓库",
		"Git query failed":       "Git 查询失败",
		"Git query timed out":    "Git 查询超时",
		"Git result too large":   "Git 查询结果超出限制",
		"Refreshing Git changes": "正在刷新 Git 更改",
		"Refreshing Git history": "正在刷新 Git 历史",
		"Editing remains available while Git runs.":                 "Git 在后台查询，不影响编辑。",
		"Working tree clean":                                        "工作区没有未提交的更改",
		"No commits yet":                                            "尚无提交记录",
		"Refresh to check for external changes.":                    "点击刷新以检查外部更改。",
		"Check Toolchain Status and repository access, then retry.": "请检查工具链状态和仓库访问权限，然后重试。",
		"Git is optional and is not bundled. Configure Git in PATH, then retry; restart SilkIDE after changing the system PATH.": "Git 是可选工具，不随包提供。配置 Git 的 PATH 后重试；修改系统 PATH 后请重启 SilkIDE。",
		"This folder is not a Git working tree. Open an existing repository, or initialize one yourself, then retry.":            "当前文件夹不是 Git 工作区。请打开已有仓库，或自行初始化仓库后重试。",
		"The result exceeded the display budget; no partial or clean-looking snapshot was accepted.":                             "查询结果超出显示预算，未接受可能误报为无更改的部分结果。",
		"Export...":       "导出...",
		"Settings":        "设置",
		"Menu":            "菜单",
		"New":             "新建",
		"Dump A11y Tree":  "显示无障碍树",
		"Unsaved changes": "有未保存的修改",
		"The current design has unsaved changes. Save before continuing?": "当前设计有未保存的修改。在继续前保存吗？",
		"Discard":          "丢弃",
		"Cancel":           "取消",
		"Project Settings": "项目设置",
		"Close":            "关闭",
		"Recover Autosave": "从自动保存恢复",
		"A more recent autosave was found for %s. Recover from it?": "检测到 %s 有更新的自动保存。是否恢复？",
		"About":                                     "关于",
		"A visual designer for Go interfaces.":      "面向 Go 界面的可视化设计器。",
		"Drag, arrange and generate Go.":            "拖拽控件、编排布局、生成 Go 代码。",
		"SDK version":                               "SDK 版本",
		"App version":                               "应用版本",
		"Development build":                         "开发构建",
		"Build information unavailable":             "构建信息不可用",
		"Working tree modified":                     "工作区含未提交修改",
		"Runtime":                                   "运行时",
		"Platform":                                  "平台",
		"Rendering":                                 "渲染后端",
		"Project settings could not be saved":       "项目设置保存失败",
		"Project settings saved":                    "项目设置已保存",
		"No design to export":                       "没有可导出的设计文档",
		"Accessibility tree unavailable":            "无障碍树暂不可用",
		"Accessibility tree shown in Output":        "无障碍树已显示在输出面板",
		"Open an editor document first":             "请先切换到已打开的代码文档",
		"Run stopped":                               "运行已停止",
		"All documents are saved":                   "所有文档均已保存",
		"Save All did not complete":                 "全部保存未完成",
		"No running process":                        "当前没有运行中的任务",
		"build ok":                                  "构建成功",
		"build: error":                              "构建失败",
		"build: %d errors":                          "构建错误: %d",
		"Command Palette":                           "命令面板",
		"Open Recent":                               "打开最近",
		"Open Recent...":                            "打开最近...",
		"(empty)":                                   "(空)",
		"Fit to View":                               "适应视口",
		"Find in Files":                             "在文件中查找",
		"Outline":                                   "大纲",
		"No symbols":                                "暂无符号",
		"Indexing symbols...":                       "正在索引符号…",
		"Show Outline":                              "显示大纲",
		"Show Files":                                "显示文件列表",
		"Filter Project Files":                      "筛选项目文件",
		"Refresh Editor Git Markers":                "刷新编辑器 Git 标记",
		"Git markers refreshed":                     "Git 标记已刷新",
		"Refresh Project Files":                     "刷新项目文件",
		"Toggle Hidden Files":                       "切换隐藏文件显示",
		"Reveal Active File in Explorer":            "在文件列表定位当前文件",
		"The active file is not inside the project": "当前文件不在项目目录内",
		"Filter Files":                              "筛选文件",
		"Name or path contains:":                    "名称或路径包含：",
		"No matching files":                         "没有匹配的文件",
		"Open a project folder":                     "请打开项目文件夹",
		"File scan":                                 "文件扫描",
		"Show Problems":                             "显示问题",
		"Show Bookmarks":                            "显示书签",
		"Show Packages":                             "显示包列表",
		"Refresh Packages":                          "刷新包列表",
		"Add Bookmark":                              "添加书签",
		"Rename Symbol":                             "重命名符号",
		"New name:":                                 "新名称：",
		"Renamed %s → %s (%d occurrences)":          "已重命名 %s → %s (%d 处)",
		"Rename failed: %v":                         "重命名失败: %v",
		"Rename Symbol not available":               "暂不支持重命名符号",
		"Configure Run...":                          "配置运行参数...",
		"Run Configuration":                         "运行配置",
		"OK":                                        "确定",
		"Run arguments:":                            "运行参数:",
		"Run args saved":                            "运行参数已保存",
		"Run config saved":                          "运行配置已保存",
		"silkide: %d env entries configured (not yet forwarded to go run)": "silkide: 已配置 %d 项环境变量（暂未转发给 go run）",
		"Open File":                               "打开文件",
		"Quick Open File":                         "快速打开文件",
		"Saved %s":                                "已保存 %s",
		"Build successful":                        "构建成功",
		"Build failed":                            "构建失败",
		"Running...":                              "运行中...",
		"Running tests...":                        "运行测试中...",
		"Tests passed":                            "测试通过",
		"Tests failed":                            "测试失败",
		"Run Tests":                               "运行测试",
		"Run with Coverage":                       "运行测试 (覆盖率)",
		"Show Coverage":                           "显示覆盖率",
		"Running with coverage...":                "运行测试 (覆盖率) 中...",
		"Coverage applied":                        "覆盖率已应用",
		"Coverage failed":                         "覆盖率运行失败",
		"Run go vet":                              "运行 go vet",
		"Running go vet...":                       "运行 go vet 中...",
		"go vet ok":                               "go vet 通过",
		"go vet failed":                           "go vet 失败",
		"Test Results":                            "测试结果",
		"Show Test Results":                       "显示测试结果",
		"Show Diff vs Saved":                      "对比已保存版本",
		"Loading comparison...":                   "正在加载比较内容…",
		"Computing comparison...":                 "正在计算差异…",
		"Comparison failed: %v":                   "比较失败：%v",
		"Comparison cancelled":                    "已取消比较",
		"No diff to compare":                      "没有可比较的差异",
		"No text changes to compare":              "没有可比较的文本差异",
		"Paste failed: %v":                        "粘贴失败：%v",
		"Previous change (P)":                     "上一处更改 (P)",
		"Next change (N)":                         "下一处更改 (N)",
		"%d rows - %d changed":                    "%d 行 · %d 行更改",
		"Diff vs Saved: %s":                       "对比已保存版本: %s",
		"Diff vs HEAD":                            "对比 HEAD 版本",
		"Diff vs HEAD: %s":                        "对比 HEAD 版本: %s",
		"No changes vs HEAD":                      "与 HEAD 无差异",
		"git diff failed (not a repo?)":           "git diff 失败（不是仓库？）",
		"Show Log":                                "显示日志",
		"Clear Log":                               "清空日志",
		"Continue":                                "继续",
		"Step Over":                               "单步跳过",
		"Step Into":                               "单步进入",
		"Step Out":                                "单步跳出",
		"Show Debug":                              "显示调试",
		"No debug session":                        "无调试会话",
		"LSP not running":                         "LSP 未运行",
		"Go to Definition":                        "跳转到定义",
		"Go to Type Definition":                   "跳转到类型定义",
		"Go to Implementation":                    "跳转到实现",
		"Event Handlers":                          "事件处理函数",
		"Go Back":                                 "后退",
		"Go Forward":                              "前进",
		"No navigation targets found":             "未找到可跳转的目标",
		"Format Document":                         "格式化文档",
		"Format failed: %v":                       "格式化失败: %v",
		"Rename: no changes":                      "重命名: 无改动",
		"Rename aborted: buffer changed":          "重命名已取消: 缓冲区已改动",
		"Renamed across %d file(s)":               "已重命名 %d 个文件",
		"Find References":                         "查找引用",
		"Go to Symbol in Workspace":               "跳转到工作区符号",
		"Indexing project files...":               "正在索引项目文件…",
		"Searching files...":                      "正在搜索文件…",
		"No files found":                          "未找到文件",
		"[%s] Incomplete file index (%d matches)": "[%s] 文件索引不完整（%d 个匹配）",
		"Showing %d of %d files; refine your search": "显示 %d / %d 个文件，请细化搜索",
		"%d files":                      "%d 个文件",
		"Language service disconnected": "语言服务连接已断开",
		"Add Cursor Above":              "向上添加光标",
		"Add Cursor Below":              "向下添加光标",
		"Keep Primary Cursor":           "仅保留主光标",
		"Toolchain Status":              "工具链状态",
		"gopls: offline":                "gopls：未连接",
		"gopls: starting":               "gopls：启动中",
		"gopls: connected":              "gopls：已连接",
		"gopls: working (%d)":           "gopls：处理中（%d）",
		"gopls: working %d%%":           "gopls：处理中 %d%%",
		"gopls: activity limited":       "gopls：进度显示受限",
		"Open toolchain status for paths and recovery actions.":          "打开工具链状态查看路径与恢复操作。",
		"Starting the language service; editor input remains available.": "正在启动语言服务，编辑器仍可输入。",
		"No reported work. Background caches may still be running.":      "没有上报中的任务，后台缓存可能仍在运行。",
		"Progress display limit reached; activity may be incomplete.":    "已达进度显示上限，显示可能不完整。",
		"Click for a language-service snapshot and toolchain status.":    "点击查看语言服务快照与工具链状态。",
		"Language service failed to start":                               "语言服务启动失败",
		"Searching symbols...":                                           "正在搜索符号…",
		"Symbol search failed":                                           "符号搜索失败",
		"Symbol search cancelled":                                        "符号搜索已取消",
		"No symbols found":                                               "未找到符号",
		"Showing %d of %d symbols; refine your search":                   "显示 %d / %d 个符号，请缩小搜索范围",
		"%d symbols":                                 "%d 个符号",
		"Show Git Changes":                           "显示 Git 更改",
		"Refresh Git Changes":                        "刷新 Git 更改",
		"Scan TODOs":                                 "扫描 TODO",
		"Restart Debug":                              "重启调试",
		"Trim Trailing Whitespace":                   "清除行尾空白",
		"Set variable failed: %v":                    "设置变量失败: %v",
		"Restart failed: %v":                         "重启失败: %v",
		"Unstage failed: %v":                         "撤出暂存失败: %v",
		"Show Git History":                           "显示 Git 历史",
		"No diff in commit":                          "该提交无差异",
		"Toggle Blame":                               "切换 Blame 标注",
		"Committed %s":                               "已提交 %s",
		"No references found":                        "未找到引用",
		"Code Actions":                               "代码操作",
		"No code actions":                            "无可用操作",
		"Code action has no edit":                    "该操作无可应用编辑",
		"Applied: %s":                                "已应用: %s",
		"Open Project":                               "打开项目",
		"Project: %s":                                "项目: %s",
		"Failed to read saved file":                  "读取已保存文件失败",
		"Running %s...":                              "运行 %s 中...",
		"gofmt failed; saved unformatted":            "gofmt 失败，已按原样保存",
		"Save failed: %v":                            "保存失败: %v",
		"Run skipped: no main package":               "未找到 main 包，已跳过运行",
		"Opened %s":                                  "已打开 %s",
		"Recovered from autosave":                    "已从自动保存恢复",
		"Exported to %s":                             "已导出到 %s",
		"Print Design...":                            "打印设计...",
		"Switch to Design mode to print":             "请切换到设计模式后打印",
		"Cannot print a design with missing widgets": "设计存在缺失控件，不能打印",
		"Print backend accepted the document; check the printer queue": "打印后端已接收文档，请检查打印队列",
		"Print job queued: %s":        "打印任务已入队：%s",
		"Printing could not complete": "打印未完成",
		"Export failed":               "导出失败",
		"main.go":                     "main.go",
		"server.go":                   "server.go",
		"go.mod":                      "go.mod",
		"untitled":                    "未命名",
		// Status-bar message templates. i18n.Tf passes the format args
		// through unchanged, so the "%s" / "%d" placeholders translate
		// 1-for-1 from English to Chinese.
		"Selected: %s":       "已选中: %s",
		"Selected: %d items": "已选中: %d 项",
		// Debugger + LSP wiring strings. Shift+F5 / palette "Debug"
		// share these via runProjectInDebugger; the LSP background
		// launch only surfaces "Restart LSP" through the palette.
		"Debugger started":            "调试器已启动",
		"Debugger starting...":        "调试器启动中...",
		"Debugger stopped":            "调试器已停止",
		"Debugger already running":    "调试器已在运行",
		"Debuggee exited":             "被调试程序已退出",
		"Go to definition failed: %v": "跳转到定义失败: %v",
		"No definition found":         "未找到定义",
		"Stop Debugger":               "停止调试器",
		"Restart LSP":                 "重启 LSP",
		"Restarting LSP...":           "正在重启 LSP...",
		"Debugger failed: %v":         "调试器启动失败: %v",
		"Debugger error: %v":          "调试器错误: %v",
		"Stopped at %s:%d":            "已停在 %s:%d",
		// Toolbar Stop button + status-bar cursor cell, plus the git and
		// LSP code-action error/success toasts. These strings were already
		// routed through i18n.T/i18n.Tf at their call sites but lacked a
		// zh-CN entry, so a Chinese locale fell back to English.
		"Stop":                     "停止",
		"Ln 1, Col 1":              "行 1, 列 1",
		"Stage failed: %v":         "暂存失败: %v",
		"Commit failed: %v":        "提交失败: %v",
		"Blame failed: %v":         "Blame 失败: %v",
		"git show failed: %v":      "git show 失败: %v",
		"Applied: %s (%d file(s))": "已应用: %s (%d 个文件)",
		"Code action failed: %v":   "代码操作失败: %v",
	})
}

// localeIsChinese is a tiny helper for places where the test "is the
// IDE running in a Chinese locale" decides between English and
// Chinese display strings. Equivalent to checking the prefix of
// i18n.Locale().
func localeIsChinese() bool {
	return strings.HasPrefix(i18n.Locale(), "zh")
}

// registerShortcuts wires the IDE's keyboard shortcuts through silk's
// frame-level shortcut registry. The same callback bodies as the
// toolbar buttons fire here, so Cmd+S and the save toolbar icon
// share their save logic. silk's gui.RegisterShortcut routes through
// the window-level key callback BEFORE focus routing — without that
// pre-emption a focused CodeEditor would consume Cmd+Z for its own
// undo before the IDE-level design-canvas undo could fire.
//
// Mapping (Cmd on macOS, Ctrl elsewhere via gui.ModAction):
//
//	Cmd+O    → OpenFileDialog → openFromTree
//	Cmd+S    → save the visible code/design document
//	Cmd+Z    → undo in the visible code/design document
//	Cmd+Shift+Z / Cmd+Y → redo
//	Cmd+R    → refresh the design canvas
func registerShortcuts(editorTabs *gui.TabWidget, designCanvas *ged.GedView) {
	gui.RegisterShortcut(gui.ModAction, 'N', func() {
		newDesignCanvas(designCanvas)
	})
	gui.RegisterShortcut(gui.ModAction, 'O', func() {
		path := gui.OpenFileDialog()
		if path == "" {
			return
		}
		openFromTree(path, editorTabs, designCanvas, nil)
	})
	gui.RegisterShortcut(gui.ModAction, 'S', func() { saveCurrentDocument(editorTabs, designCanvas) })
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'S', func() { performSaveAll(editorTabs, designCanvas) })
	gui.RegisterShortcut(gui.ModAction, 'Z', func() { undoCurrentDocument(editorTabs, designCanvas, false) })
	redo := func() { undoCurrentDocument(editorTabs, designCanvas, true) }
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'Z', redo)
	gui.RegisterShortcut(gui.ModAction, 'Y', redo)
	for i, mode := range []workspaceMode{modeWelcome, modeEdit, modeDesign} {
		gui.RegisterShortcut(gui.ModAction, '1'+i, func() {
			if globalWorkbench != nil {
				globalWorkbench.activate(mode)
			}
		})
	}
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'R', func() { previewDesign(designCanvas) })
	gui.RegisterShortcut(gui.ModAction, 'R', func() {
		if designCanvas != nil {
			designCanvas.Update()
		}
	})
	// Cmd+W: close active editor tab, Cmd+Q: quit. Standard mac UX.
	gui.RegisterShortcut(gui.ModAction, 'W', func() {
		if editorTabs == nil || documentMode(editorTabs, designCanvas) != modeEdit {
			return
		}
		if idx := editorTabs.CurrentIndex(); idx >= 0 {
			// Through closeEditorTab, not RemoveTab: the keyboard close is the
			// one people reach for most, so it needs the same unsaved-work
			// prompt, openEditors untrack and gopls didClose the tab's X button
			// gets. closeEditorTab drops the tab itself once the close is agreed
			// to, so there is nothing left to remove here.
			closeEditorTab(editorTabs, idx)
		}
	})
	gui.RegisterShortcut(gui.ModAction, 'Q', func() {
		// Same dirty-save guard as File→New for the design scene, plus
		// the editor buffers — Cmd+Q used to quit straight over those,
		// and session restore reopening the files from disk made the
		// lost edits look like a restore that worked. canQuit is a
		// quick no-op when nothing has drifted.
		if !canQuit(designCanvas) {
			return
		}
		// Out through Frame.Close, the same door the title bar's close button
		// uses. Ending the loop directly skips the frame's closed callback,
		// which is where the window geometry and the open-file set get
		// written — so a Cmd+Q quit used to leave the PREVIOUS launch's
		// session on disk and restore it next time. Frame.Close never
		// consults the can-close guard, so canQuit above is still asked once.
		if closeApp != nil {
			closeApp()
			return
		}
		quitApp()
	})

	// Cmd+Shift+A: dump the accessibility tree of the active frame.
	// Useful for verifying that custom widgets expose sane Roles to
	// screen readers, or for snapshotting the visual hierarchy in a
	// bug report. Same handler as the hamburger menu's "Dump A11y
	// Tree" entry — output goes to stderr.
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'A', dumpA11yTree)

	// Canvas zoom shortcuts. Cmd+= / Cmd+- step by 1.25x (typical
	// designer-tool muscle memory: each press makes the canvas 25%
	// bigger or 80% of current). Cmd+0 resets to 1.0 (= 100%).
	// VK_EQUAL = 0xBB, VK_MINUS = 0xBD, '0' = 0x30.
	gui.RegisterShortcut(gui.ModAction, 0xBB, func() { zoomCanvas(designCanvas, 1.25) })
	gui.RegisterShortcut(gui.ModAction, 0xBD, func() { zoomCanvas(designCanvas, 1.0/1.25) })
	gui.RegisterShortcut(gui.ModAction, '0', func() { zoomCanvasTo(designCanvas, 1.0) })

	// A plain F belongs to the editor. Fit to View uses a modified chord;
	// subsequent manual zoom switches back to PL_FREE_ZOOM.
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, '0', func() { fitCanvasToView(designCanvas) })

	// F5 → Run, F6 → Build, F7 → Test. Visual Studio / JetBrains
	// muscle memory. No modifier so they don't clash with Cmd+R
	// (canvas refresh) and Cmd+B (which we may bind to "build" with
	// a modifier later if the function-keyless laptop crowd asks for
	// it). Cmd+Shift+T is the second F7 binding for laptops without
	// function rows.
	gui.RegisterShortcut(0, gui.KeyF5, func() { runProjectInTerminal(designCanvas) })
	// Shift+F5 → launch dlv against the project. Pairs with the Debug
	// toolbar button and the "Debug" palette command -- all three hit
	// the same runProjectInDebugger entry point.
	gui.RegisterShortcut(gui.ModShift, gui.KeyF5, func() { runProjectInDebugger(designCanvas) })
	gui.RegisterShortcut(0, gui.KeyF6, func() { buildProject(designCanvas) })
	gui.RegisterShortcut(0, gui.KeyF7, func() { runProjectTests(designCanvas) })
	// Debugger stepping (Qt Creator key layout: F10 over, F11 into,
	// Shift+F11 out). Active only when a dlv session is live; debugStep
	// shows a toast and no-ops otherwise. Continue is Shift+F5.
	gui.RegisterShortcut(0, gui.KeyF10, func() { debugStep("over") })
	gui.RegisterShortcut(0, gui.KeyF11, func() { debugStep("into") })
	gui.RegisterShortcut(gui.ModShift, gui.KeyF11, func() { debugStep("out") })
	// All IDE entry points share semantic navigation and explicit service
	// feedback. Standalone SDK editors retain their local AST resolver.
	gui.RegisterShortcut(0, gui.KeyF12, func() { goToDefinitionViaLSP(editorTabs) })
	gui.RegisterShortcut(gui.ModAction, gui.KeyF12, func() { requestSymbolNavigation(editorTabs, "Go to Type Definition") })
	gui.RegisterShortcut(gui.ModAlt, gui.KeyF12, func() { requestSymbolNavigation(editorTabs, "Go to Implementation") })
	gui.RegisterShortcut(gui.ModAlt, gui.KeyLeft, func() { navigateEditorHistory(editorTabs, false) })
	gui.RegisterShortcut(gui.ModAlt, gui.KeyRight, func() { navigateEditorHistory(editorTabs, true) })
	// Format Document via gopls (Cmd+Shift+I). Complements gofmt-on-save
	// with an on-demand reformat; gopls also applies goimports.
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'I', func() { formatDocumentViaLSP(editorTabs) })
	// Find References via gopls (Shift+F12) — lists usages in the panel.
	gui.RegisterShortcut(gui.ModShift, gui.KeyF12, func() { findReferencesViaLSP(editorTabs) })
	// Code Actions / quick-fixes via gopls (Cmd+.).
	gui.RegisterShortcut(gui.ModAction, '.', func() { codeActionsViaLSP(editorTabs) })
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'T', func() { runProjectTests(designCanvas) })
	// Cmd+T — Go to Symbol in Workspace (gopls workspace/symbol search).
	gui.RegisterShortcut(gui.ModAction, 'T', func() { showSymbolFinder(designCanvas, editorTabs) })
	// Cmd+Shift+B — toggle git-blame annotations on the active editor.
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'B', func() { toggleBlame(editorTabs, designCanvas) })
	// Shift+F6 → go vet; Cmd+Shift+F7 → tests with coverage. Both slot
	// in next to the existing F6/F7 build/test pair so the muscle memory
	// generalises: "F6 family runs static analysis (build/vet), F7 family
	// runs tests (plain/coverage)".
	gui.RegisterShortcut(gui.ModShift, gui.KeyF6, func() { runProjectVet(designCanvas) })
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, gui.KeyF7, func() { runProjectWithCoverage(designCanvas) })

	// Cmd+Shift+P — open the Command Palette. JetBrains "Find Action"
	// / VSCode command palette muscle memory: every action in the IDE
	// is searchable from here, including ones that don't have an
	// explicit toolbar button or shortcut. The dialog-anchor parent
	// is the design canvas so the modal centres over the workspace.
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'P', func() {
		showCommandPalette(designCanvas)
	})

	// Cmd+P — quick file open. Walks projectDir, lists every non-
	// hidden file, filters by subsequence match. Pairs with Cmd+Shift+P
	// the way VSCode / JetBrains do — actions vs files split across
	// the modifier so power users build muscle memory for both.
	gui.RegisterShortcut(gui.ModAction, 'P', func() {
		showFileFinder(designCanvas, projectDir(designCanvas), editorTabs)
	})

	// Cmd+Shift+F — focus the global-search panel in the left dock.
	// VSCode / JetBrains muscle memory; bringing the dock tab to
	// the front isn't enough — the panel handles its own input via
	// OnTextInput, so we also need to give it keyboard focus so the
	// user can start typing immediately.
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'F', func() {
		if globalSearch != nil {
			dockSetActiveView(globalLeftDock, globalSearch)
			globalSearch.SetFocus()
		}
	})

	// Cmd+Shift+O — bring the code-outline tab to the front of the
	// right dock. VSCode "Go to Symbol" muscle memory; the outline
	// reacts to clicks rather than typed input, so flipping the tab
	// (and focusing it for wheel scrolling) is all the shortcut needs.
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'O', func() {
		if globalOutline != nil {
			dockSetActiveView(globalRightDock, globalOutline)
			globalOutline.SetFocus()
		}
	})

	// F2 — JetBrains "Rename Symbol" muscle memory: pop an input box
	// prefilled with the identifier under the cursor and rewrite every
	// occurrence in the active editor. The cross-file bookmarks slot
	// moves to Cmd+F2 below — same key, modifier disambiguates between
	// "I want to navigate" and "I want to rename".
	gui.RegisterShortcut(0, gui.KeyF2, func() {
		renameSymbolAtActiveEditor(editorTabs)
	})
	// Cmd+F2 — add a bookmark on the active editor's cursor line. Qt
	// Creator's plain-F2 convention was the original silkide binding;
	// it stepped on the more common Rename Symbol shortcut, so the
	// bookmark slot picks up a modifier here. The "Add Bookmark"
	// palette command shares this handler.
	gui.RegisterShortcut(gui.ModAction, gui.KeyF2, func() {
		addBookmarkAtCursor(editorTabs)
	})
	// "Diff vs HEAD" stays palette-only. dispatchShortcut runs BEFORE
	// focus routing and consumes the key, so a global Cmd+D here would
	// shadow the CodeEditor's multi-cursor "select next occurrence"
	// (also Cmd+D) whenever an editor has focus. The VCS diff is a
	// low-frequency action the palette command already covers.
}

// fitCanvasToView switches the design canvas to PL_FIT_VIEW so the
// whole form is visible at the largest zoom that fits. SetPageLayout
// emits SigZoomChanged when the new layout settles on a different
// zoom, so the status-bar zoom %% cell tracks this automatically.
func fitCanvasToView(canvas *ged.GedView) {
	if canvas == nil {
		return
	}
	canvas.SetPageLayout(graph.PL_FIT_VIEW)
	canvas.Update()
}

// zoomCanvas multiplies the design canvas's zoom factor by `factor`.
// 1.25 zooms in, 0.8 zooms out. limitZoomFactor inside graph caps
// the absolute range so back-to-back +/- presses don't run away.
func zoomCanvas(canvas *ged.GedView, factor float64) {
	if canvas == nil {
		return
	}
	zoomCanvasTo(canvas, canvas.ZoomFactor()*factor)
}

// zoomCanvasTo sets the design canvas's zoom factor absolutely and
// triggers a layout/redraw. Used by Cmd+0 (reset to 1.0).
func zoomCanvasTo(canvas *ged.GedView, z float64) {
	if canvas == nil {
		return
	}
	canvas.SetZoomFactor(z)
	canvas.Update()
	setZoomLabel(canvas.ZoomFactor())
}

func actionKeyHint() string {
	if runtime.GOOS == "darwin" {
		return "Cmd"
	}
	return "Ctrl"
}

func registerAppearanceTranslations() {
	i18n.AddMany("zh-CN", map[string]string{
		"Apply": "应用", "Properties": "属性", "Welcome": "欢迎",
		"Appearance & Editor": "外观与编辑器", "Language (restart required)": "界面语言（重启生效）", "Theme": "主题", "Dark": "深色", "Light": "浅色",
		"UI font": "界面字体", "UI font size": "界面字号", "Editor font": "编辑器字体", "Editor font size": "编辑器字号", "Tab width": "Tab 显示宽度",
		"Accent (#RRGGBB)": "强调色（#RRGGBB）", "Editor background": "编辑器背景色", "Editor text": "代码文字颜色", "Keywords": "关键字颜色", "Comments": "注释颜色", "Strings": "字符串颜色",
		"Empty color = theme default; colors use #RRGGBB": "颜色留空使用主题默认值；格式为 #RRGGBB", "Appearance applied; saving in background. Language changes apply after restart": "外观已应用，正在后台保存。语言设置将在重启后生效",
	})
}
