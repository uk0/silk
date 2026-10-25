package main

import (
	"path/filepath"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// globalFrame is the topmost silkide window. Toast notifications render
// inside its bottom-right content area, above the status bar, rather
// than in independent floating native windows. Set in main()
// after gui.NewFrameWindow returns; nil before that, and silkideToast
// is built to no-op in the nil case so panics never reach a user.
var globalFrame *gui.Frame

// silkideToast surfaces a transient banner via gui.ShowToast. The
// helper is a function var (not a plain func) so tests can install a
// recording hook without touching the global toast manager — see
// TestSilkideToast* in feedback_test.go for the pattern.
//
// The session history retains messages after expiry; warnings/errors remain
// for 5 seconds. Default duration is 2.5 seconds — long enough to read a short
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
	if p := notificationsFor(globalFrame); p != nil {
		p.record(msg, level)
	} else {
		gui.ShowToastWithOptions(globalFrame, msg, gui.ToastOptions{DurationMs: 2500, Level: level, Position: gui.ToastBottomRight})
	}
}

// saveDesignDocument is shared by explicit saves and quit/open/new guards.
// It attempts companion Go generation after saving; failures appear in Output
// but do not invalidate the successfully saved design document.
func saveDesignDocument(canvas *ged.GedView) bool {
	if canvas == nil || canvas.GedScene() == nil || !canvas.GedScene().Save() {
		return false
	}
	regenerateGoForSilkui(canvas.GedScene().Filename())
	return true
}

// performSave adds feedback to the shared save flow. Dirty-work prompts use
// saveDesignDocument directly to avoid stacking a toast on a closing dialog.
// The scene API does not distinguish a cancelled chooser from a failed write.
func performSave(canvas *ged.GedView) bool {
	if !saveDesignDocument(canvas) {
		return false
	}
	scene := canvas.GedScene()
	name := filepath.Base(scene.Filename())
	if name == "" || name == "." {
		name = i18n.T("untitled")
	}
	silkideToast(i18n.Tf("Saved %s", name), gui.ToastSuccess)
	return true
}
