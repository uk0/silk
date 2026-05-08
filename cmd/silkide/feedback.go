package main

import (
	"path/filepath"

	"silk/ged"
	"silk/gui"
	"silk/i18n"
)

// globalFrame is the topmost silkide window. Toast notifications anchor
// to it so they pop above the IDE shell rather than the active editor
// pane (which would clip them to the pane's bounds). Set in main()
// after gui.NewFrameWindow returns; nil before that, and silkideToast
// is built to no-op in the nil case so panics never reach a user.
var globalFrame *gui.Frame

// silkideToast surfaces a transient banner via gui.ShowToast. The
// helper is a function var (not a plain func) so tests can install a
// recording hook without touching the global toast manager — see
// TestSilkideToast* in feedback_test.go for the pattern.
//
// Default duration is 2.5 seconds — long enough to read a short
// message, short enough not to crowd the workspace if several toasts
// fire in succession (e.g. Save + Build + Run all happen on F5).
//
// Calls before globalFrame is set silently drop the toast rather than
// panicking. This keeps startup-time wiring (e.g. registerPaletteCommands
// firing during early bootstrap) from crashing the IDE before the frame
// exists.
var silkideToast = func(msg string, level gui.ToastLevel) {
	if globalFrame == nil {
		return
	}
	gui.ShowToast(globalFrame, msg, 2500, level)
}

// performSave saves the active design scene if any, regenerates the
// sibling .silk.go on success, and surfaces a "Saved <name>" toast.
// Returns true on save success.
//
// Used by every explicit Save action — Cmd+S, the toolbar save icon,
// the hamburger menu, and the Command Palette — so the side effects
// (file write + .silk.go regen + toast) stay consistent regardless of
// entry point. The dirty-discard prompt path keeps its own scene.Save
// call: it has its own confirmation flow and a stacked toast there
// would race the dialog dismissal animation.
//
// Save returning false typically means the user cancelled a
// SaveFileDialog (filenameless scene); we stay silent in that case.
// A real disk error is hard to distinguish from cancel without a
// richer return type from scene.Save, so the message bias is toward
// not toasting "Save failed" when the user actually clicked Cancel.
func performSave(canvas *ged.GedView) bool {
	if canvas == nil {
		return false
	}
	scene := canvas.GedScene()
	if scene == nil {
		return false
	}
	if !scene.Save() {
		return false
	}
	regenerateGoForSilkui(scene.Filename())
	name := filepath.Base(scene.Filename())
	if name == "" || name == "." {
		name = i18n.T("untitled")
	}
	silkideToast(i18n.Tf("Saved %s", name), gui.ToastSuccess)
	return true
}
