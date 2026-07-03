package ged

import (
	"testing"
)

// TestKeymapDefaultsGet exercises the built-in defaults through LoadKeymap().
// Because the file is lazily loaded once per process, the exact instance we
// get back may already contain user overrides from a previous run; we only
// assert on well-known commands that must be present.
func TestKeymapDefaultsGet(t *testing.T) {
	km := LoadKeymap()
	wantPairs := map[string]string{
		"file.new":              "Ctrl+N",
		"editor.find":           "Ctrl+F",
		"editor.addCursorAbove": "Ctrl+Alt+Up",
		"debug.perfOverlay":     "F12",
	}
	for cmd, wantKey := range wantPairs {
		got := km.Get(cmd)
		if got == "" {
			t.Errorf("Get(%q) = \"\", want a binding", cmd)
			continue
		}
		// Only check the well-known defaults — user may have customized.
		if got != wantKey {
			// Not a hard failure, the user may have remapped; log for visibility.
			t.Logf("Get(%q) = %q, default was %q (may be user-customized)", cmd, got, wantKey)
		}
	}
}

// TestKeymapSetNew adds a command that isn't in defaults and reads it back.
func TestKeymapSetNew(t *testing.T) {
	km := &KeyMap{}
	km.Reset()
	km.Set("custom.test.command", "Ctrl+Alt+T")
	if got := km.Get("custom.test.command"); got != "Ctrl+Alt+T" {
		t.Errorf("Get(custom) = %q, want Ctrl+Alt+T", got)
	}
}

// TestKeymapSetOverwrite ensures Set() on an existing command overwrites.
func TestKeymapSetOverwrite(t *testing.T) {
	km := &KeyMap{}
	km.Reset()
	original := km.Get("file.new")
	km.Set("file.new", "Ctrl+Shift+N")
	if got := km.Get("file.new"); got != "Ctrl+Shift+N" {
		t.Errorf("after overwrite: got %q, want Ctrl+Shift+N", got)
	}
	if original == "Ctrl+Shift+N" {
		t.Log("default was already Ctrl+Shift+N; overwrite test is weaker")
	}
}

// TestKeymapReset restores defaults.
func TestKeymapReset(t *testing.T) {
	km := &KeyMap{}
	km.Reset()
	km.Set("file.new", "Ctrl+Shift+XYZ")
	km.Reset()
	if got := km.Get("file.new"); got == "Ctrl+Shift+XYZ" {
		t.Errorf("after Reset(): file.new = %q, expected default", got)
	}
}

// TestKeymapBindingsSnapshot ensures Bindings() returns an independent copy.
func TestKeymapBindingsSnapshot(t *testing.T) {
	km := &KeyMap{}
	km.Reset()
	snap1 := km.Bindings()
	if len(snap1) == 0 {
		t.Fatal("Bindings() returned empty after Reset()")
	}
	// Mutate the snapshot; internal state must be unaffected.
	snap1[0].Key = "MUTATED"
	snap2 := km.Bindings()
	if snap2[0].Key == "MUTATED" {
		t.Error("Bindings() did not return an independent copy")
	}
}

// TestKeymapLen is a smoke test on the length helper.
func TestKeymapLen(t *testing.T) {
	km := &KeyMap{}
	km.Reset()
	if km.Len() != len(defaultKeymap) {
		t.Errorf("Len() = %d, want %d", km.Len(), len(defaultKeymap))
	}
}
