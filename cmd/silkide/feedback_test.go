package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// TestSilkideToastNilFrameNoOp: silkideToast must not panic when
// globalFrame is unset. Early-bootstrap call sites (palette wiring,
// translation init) can fire before main() finishes setting up
// globalFrame, and a panic there would crash the IDE before the
// user ever sees a window.
func TestSilkideToastNilFrameNoOp(t *testing.T) {
	saved := globalFrame
	globalFrame = nil
	defer func() { globalFrame = saved }()
	silkideToast("hi", gui.ToastInfo) // must not panic
}

// TestSilkideToastFunctionVarOverride: silkideToast is a function
// var (not a plain func) so tests can swap in a recording hook.
// This is the primitive every other feedback-flow test depends on —
// without it we'd be asserting against the global toast manager
// (which has flake-prone teardown across the test binary's lifetime).
func TestSilkideToastFunctionVarOverride(t *testing.T) {
	saved := silkideToast
	defer func() { silkideToast = saved }()

	type record struct {
		msg   string
		level gui.ToastLevel
	}
	var calls []record
	silkideToast = func(msg string, level gui.ToastLevel) {
		calls = append(calls, record{msg: msg, level: level})
	}

	silkideToast("test-msg", gui.ToastError)
	if len(calls) != 1 {
		t.Fatalf("expected 1 recorded call, got %d", len(calls))
	}
	if calls[0].msg != "test-msg" || calls[0].level != gui.ToastError {
		t.Errorf("recorded %+v, want {msg: test-msg, level: Error}", calls[0])
	}
}

// TestPerformSaveNilCanvasReturnsFalse: nil canvas returns false
// without ever touching the toast hook. Cmd+S fires the same code
// path with no canvas yet bound, and a toast there would be
// nonsensical ("Saved <empty>") — silence is the right behaviour.
func TestPerformSaveNilCanvasReturnsFalse(t *testing.T) {
	saved := silkideToast
	defer func() { silkideToast = saved }()
	called := false
	silkideToast = func(string, gui.ToastLevel) { called = true }

	if performSave(nil) {
		t.Errorf("performSave(nil) returned true, want false")
	}
	if called {
		t.Errorf("performSave(nil) should not toast")
	}
}

// TestPerformSaveSuccessFiresToast: a fresh GedView with a temp
// .silkui filename should:
//  1. scene.Save returns true (writes the TDoc).
//  2. performSave returns true.
//  3. silkideToast records a Success-level call mentioning the
//     basename in the message.
//
// This is the single end-to-end happy path that protects every
// explicit Save action from regressing the toast wiring at once.
func TestPerformSaveSuccessFiresToast(t *testing.T) {
	saved := silkideToast
	defer func() { silkideToast = saved }()

	type record struct {
		msg   string
		level gui.ToastLevel
	}
	var calls []record
	silkideToast = func(msg string, level gui.ToastLevel) {
		calls = append(calls, record{msg: msg, level: level})
	}

	view := ged.NewGedView()
	target := filepath.Join(t.TempDir(), "scene.silkui")
	view.GedScene().SetFilename(target)

	if !performSave(view) {
		t.Fatalf("performSave returned false on a saveable scene")
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 toast, got %d", len(calls))
	}
	if calls[0].level != gui.ToastSuccess {
		t.Errorf("toast level = %v, want Success", calls[0].level)
	}
	if !strings.Contains(calls[0].msg, "scene.silkui") {
		t.Errorf("toast %q should mention scene.silkui basename", calls[0].msg)
	}
}

// (Filename-less Save would pop a SaveFileDialog which segfaults in a
// headless test on macOS — AppKit's NSOpenPanel requires the main
// thread + a real GLFW window. The "user cancelled the save dialog,
// stay silent" contract is documented in performSave's doc comment;
// leaving it untested here is the conscious trade-off rather than
// crash-prone integration testing.)
