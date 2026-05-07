package main

import (
	"os"
	"path/filepath"
	"strings"

	"silk/core"
	"silk/ged"
	"silk/gui"
	"silk/i18n"
	"silk/settings"
)

// preferences keeps the user-visible IDE state that should survive
// across launches: window size, last opened directory, recent files.
// Backed by silk/settings (TDoc-on-disk under ~/Library/Application
// Support / %APPDATA% / ~/.config), so the persistence story is the
// same on every supported platform.
type preferences struct {
	store *settings.Settings
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

// SetWindowSize persists a (width, height) pair. Errors are best-
// effort: the IDE keeps running even if the settings file is read-
// only.
func (p *preferences) SetWindowSize(w, h int) {
	_ = p.store.SetInt("window/width", int64(w))
	_ = p.store.SetInt("window/height", int64(h))
	_ = p.store.Sync()
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
	_ = p.store.Sync()
}

// LastOpenedDir is the directory the next OpenFileDialog should start
// in. Empty means "cwd".
func (p *preferences) LastOpenedDir() string {
	return p.store.String("files/lastDir", "")
}

func (p *preferences) SetLastOpenedDir(dir string) {
	_ = p.store.SetString("files/lastDir", dir)
	_ = p.store.Sync()
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
		if _, err := os.Stat(path); err != nil {
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
	_ = p.store.Sync()
}

// filepathAbs delegates to filepath.Abs but isolates the import so
// prefs.go's import block stays minimal — the only filepath need is
// here in AddRecentFile.
func filepathAbs(path string) (string, error) {
	return filepath.Abs(path)
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
	if v := os.Getenv("SILKIDE_LOCALE"); v != "" {
		i18n.SetLocale(v)
	}
	registerSilkideTranslations()
}

// registerSilkideTranslations adds the silkide-internal Chinese
// translations to the default translator. Kept in code (not a JSON
// file) so the binary always has them — no separate asset to ship.
func registerSilkideTranslations() {
	i18n.Default.AddMany("zh-CN", map[string]string{
		"Debug":     "调试",
		"Run":       "运行",
		"Save":      "保存",
		"Open":      "打开",
		"Undo":      "撤销",
		"Redo":      "重做",
		"Refresh":   "刷新",
		"Export...": "导出...",
		"Settings":  "设置",
		"Menu":      "菜单",
		"New":       "新建",
		"main.go":   "main.go",
		"server.go": "server.go",
		"go.mod":    "go.mod",
		"untitled":  "未命名",
		// Status-bar message templates. i18n.Tf passes the format args
		// through unchanged, so the "%s" / "%d" placeholders translate
		// 1-for-1 from English to Chinese.
		"Selected: %s":       "已选中: %s",
		"Selected: %d items": "已选中: %d 项",
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
//	Cmd+S    → save active design canvas
//	Cmd+Z    → undo on the design canvas's UndoStack
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
	gui.RegisterShortcut(gui.ModAction, 'S', func() {
		if designCanvas == nil {
			return
		}
		scene := designCanvas.GedScene()
		if scene == nil {
			return
		}
		if scene.Save() {
			regenerateGoForSilkui(scene.Filename())
		}
	})
	gui.RegisterShortcut(gui.ModAction, 'Z', func() {
		if designCanvas == nil {
			return
		}
		if scene := designCanvas.GedScene(); scene != nil {
			if stack := scene.UndoStack(); stack != nil && stack.CanUndo() {
				stack.Undo()
				designCanvas.Update()
			}
		}
	})
	// Redo: both Cmd+Shift+Z (Mac convention) and Cmd+Y (Windows /
	// Linux convention) bind to the same handler.
	redo := func() {
		if designCanvas == nil {
			return
		}
		if scene := designCanvas.GedScene(); scene != nil {
			if stack := scene.UndoStack(); stack != nil && stack.CanRedo() {
				stack.Redo()
				designCanvas.Update()
			}
		}
	}
	gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'Z', redo)
	gui.RegisterShortcut(gui.ModAction, 'Y', redo)
	gui.RegisterShortcut(gui.ModAction, 'R', func() {
		if designCanvas != nil {
			designCanvas.Update()
		}
	})
	// Cmd+W: close active editor tab, Cmd+Q: quit. Standard mac UX.
	gui.RegisterShortcut(gui.ModAction, 'W', func() {
		if editorTabs == nil {
			return
		}
		if idx := editorTabs.CurrentIndex(); idx >= 0 {
			editorTabs.RemoveTab(idx)
		}
	})
	gui.RegisterShortcut(gui.ModAction, 'Q', func() {
		core.Quit()
	})
}
