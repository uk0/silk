package main

import (
	"os"
	"strings"

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
		"main.go":   "main.go",
		"server.go": "server.go",
		"go.mod":    "go.mod",
		"untitled":  "未命名",
	})
}

// localeIsChinese is a tiny helper for places where the test "is the
// IDE running in a Chinese locale" decides between English and
// Chinese display strings. Equivalent to checking the prefix of
// i18n.Locale().
func localeIsChinese() bool {
	return strings.HasPrefix(i18n.Locale(), "zh")
}
