package gui

import "sync"

// Shortcut modifier flags. Pack into a uint8 for the registry key so
// ModAction|ModShift addresses Ctrl+Shift+S on Linux/Windows and
// Cmd+Shift+S on macOS through one entry.
const (
	ModAction uint8 = 1 << iota // platform "action" modifier (Cmd / Ctrl)
	ModShift
	ModAlt
)

type shortcutKey struct {
	mods uint8
	key  int
}

var (
	shortcutMu       sync.RWMutex
	shortcutHandlers = make(map[shortcutKey]func())
)

// RegisterShortcut binds (mods, key) to fn. Subsequent registrations
// for the same (mods, key) overwrite — silkide-style apps only need
// the last binding to take effect when reconfiguring at runtime.
//
// Pass nil fn to unregister. The fn runs on the GLFW event-loop
// goroutine, same as widget OnKeyDown.
//
// Modifiers are abstracted across platforms: ModAction is Cmd on
// macOS, Ctrl elsewhere. Shortcuts that use raw KeyCtrl / KeyLWin
// directly should still go through widget-level OnKeyDown.
func RegisterShortcut(mods uint8, key int, fn func()) {
	shortcutMu.Lock()
	defer shortcutMu.Unlock()
	k := shortcutKey{mods: mods, key: key}
	if fn == nil {
		delete(shortcutHandlers, k)
		return
	}
	shortcutHandlers[k] = fn
}

// ShortcutHandler returns the fn bound to (mods, key), or nil when the
// combination is unbound. The registry is otherwise only reachable through a
// real key event — which needs a window and the modifier physically held — so
// this is how a caller confirms which action a key actually carries.
func ShortcutHandler(mods uint8, key int) func() {
	shortcutMu.RLock()
	defer shortcutMu.RUnlock()
	return shortcutHandlers[shortcutKey{mods: mods, key: key}]
}

// dispatchShortcut is called by the window-level key callback BEFORE
// focus routing. Returns true if a registered shortcut consumed the
// key, in which case onKey skips the normal IEventKeyDown dispatch
// to avoid double-handling (e.g. Cmd+S triggering both the IDE save
// and the focused CodeEditor's save).
func dispatchShortcut(key int) bool {
	mods := currentMods()
	shortcutMu.RLock()
	fn, ok := shortcutHandlers[shortcutKey{mods: mods, key: key}]
	shortcutMu.RUnlock()
	if !ok {
		return false
	}
	fn()
	return true
}

// currentMods reads the currently-held modifier keys and packs them
// into a ModAction/ModShift/ModAlt bitmap. Mirrors isActionModifier
// for the action bit so macOS Cmd and Linux/Windows Ctrl both
// resolve to ModAction.
func currentMods() uint8 {
	var m uint8
	if isActionModifier() {
		m |= ModAction
	}
	if IsKeyDown(KeyShift) {
		m |= ModShift
	}
	// silk uses Windows-style "KeyMenu" virtual code for the Alt key.
	if IsKeyDown(KeyMenu) {
		m |= ModAlt
	}
	return m
}
