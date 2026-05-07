package gui

import (
	"sync/atomic"
	"testing"
)

// TestRegisterShortcut: registering a callback keys it under
// (mods, key); a fresh registration overwrites; nil unregisters.
func TestRegisterShortcut(t *testing.T) {
	defer func() { shortcutHandlers = make(map[shortcutKey]func()) }()

	var first, second int32
	RegisterShortcut(ModAction, 'S', func() { atomic.AddInt32(&first, 1) })

	if h, ok := shortcutHandlers[shortcutKey{mods: ModAction, key: 'S'}]; !ok || h == nil {
		t.Fatal("first registration not stored")
	}

	RegisterShortcut(ModAction, 'S', func() { atomic.AddInt32(&second, 1) })
	shortcutHandlers[shortcutKey{mods: ModAction, key: 'S'}]()
	if atomic.LoadInt32(&first) != 0 {
		t.Errorf("first handler should not have fired: got %d", first)
	}
	if atomic.LoadInt32(&second) != 1 {
		t.Errorf("second handler should have fired once: got %d", second)
	}

	RegisterShortcut(ModAction, 'S', nil)
	if _, ok := shortcutHandlers[shortcutKey{mods: ModAction, key: 'S'}]; ok {
		t.Error("nil registration should remove the handler")
	}
}

// TestShortcutKeysDistinct: two shortcuts at the same key but
// different modifiers (Cmd+S vs Cmd+Shift+S) get separate slots.
func TestShortcutKeysDistinct(t *testing.T) {
	defer func() { shortcutHandlers = make(map[shortcutKey]func()) }()

	var save, saveAs int32
	RegisterShortcut(ModAction, 'S', func() { atomic.AddInt32(&save, 1) })
	RegisterShortcut(ModAction|ModShift, 'S', func() { atomic.AddInt32(&saveAs, 1) })

	shortcutHandlers[shortcutKey{mods: ModAction, key: 'S'}]()
	shortcutHandlers[shortcutKey{mods: ModAction | ModShift, key: 'S'}]()

	if atomic.LoadInt32(&save) != 1 {
		t.Errorf("save fired %d times, want 1", save)
	}
	if atomic.LoadInt32(&saveAs) != 1 {
		t.Errorf("saveAs fired %d times, want 1", saveAs)
	}
}
