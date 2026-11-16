package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// Native GLFW modals may queue owner-focus restoration after ShowModal returns.
// That is part of our own prompt, not a new user's input intent. Keep the full
// ownership checks, but apply only after the queued restoration had its turn.
func TestManualDesignRecoveryWaitsForQueuedModalFocusReturn(t *testing.T) {
	for _, recoverSaved := range []bool{false, true} {
		t.Run(map[bool]string{false: "original", true: "autosave"}[recoverSaved], func(t *testing.T) {
			w, _ := recentOpenWorkbench(t)
			path := filepath.Join(t.TempDir(), "form.silkui")
			writeRecoveryDesign(t, path, "Original", "Saved")
			writeRecoveryDesign(t, path+".autosave", "Recovered", "Recovered")
			now := time.Now().Add(time.Hour)
			if err := os.Chtimes(path+".autosave", now, now); err != nil {
				t.Fatal(err)
			}
			focus := gui.NewEdit()
			focus.SetParent(w.frame)
			focus.SetFocus()
			transient := gui.NewEdit()
			transient.SetParent(w.frame)
			w.designOpen.settings.confirmRecovery = func(gui.IWidget, string, string) bool {
				previous := gui.CurrentFocusWidget()
				transient.SetFocus()
				w.designOpen.settings.post(func() { previous.SetFocus() })
				return recoverSaved
			}
			if !requestDesignOpen(path, w.tabs, w.canvas, true) {
				t.Fatal("not admitted")
			}
			// Explicitly opening a design intentionally activates its workspace.
			// Compare against admission, not the focus before that user action.
			admittedFocus := gui.CurrentFocusWidget()
			pumpTestDesignOpens(t, w)
			want := "Saved"
			if recoverSaved {
				want = "Recovered"
			}
			if w.canvas.GedScene().Filename() != path || recoveryButtonText(t, w.canvas.GedScene()) != want {
				t.Fatal("own modal focus transition discarded the user's explicit choice")
			}
			if gui.CurrentFocusWidget() != admittedFocus {
				t.Fatal("design install stole focus")
			}
		})
	}
}

func TestManualDesignModalFocusGapDoesNotStealInput(t *testing.T) {
	canvas := ged.NewGedView()
	defer canvas.Close()
	field := gui.NewEdit()
	for _, tc := range []struct {
		name           string
		saved, current gui.IWidget
		retired, want  bool
	}{
		{"same-canvas", canvas, canvas, false, true},
		{"no-modal-no-exception", canvas, nil, false, false},
		{"own-retired-modal-empty-focus", canvas, nil, true, true},
		{"new-field-wins", canvas, field, true, false},
		{"foreign-saved-field-no-exception", field, nil, true, false},
		{"same-field", field, field, true, true},
		{"canvas-cannot-replace-saved-field", field, canvas, true, false},
		{"original-empty-focus", nil, nil, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := gui.CurrentFocusWidget()
			if got := designOpenFocusOwned(tc.saved, tc.current, canvas, tc.retired); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if gui.CurrentFocusWidget() != before {
				t.Fatal("ownership predicate changed focus")
			}
		})
	}
}

func TestManualDesignRecoveryDeferredChoiceCannotOverrideNewIntent(t *testing.T) {
	for _, action := range []string{"cancel", "close", "edit", "draft", "focus", "newer-open"} {
		t.Run(action, func(t *testing.T) {
			w, prefs := recentOpenWorkbench(t)
			path := filepath.Join(t.TempDir(), "form.silkui")
			writeRecoveryDesign(t, path, "Original", "Saved")
			writeRecoveryDesign(t, path+".autosave", "Recovered", "Recovered")
			now := time.Now().Add(time.Hour)
			if err := os.Chtimes(path+".autosave", now, now); err != nil {
				t.Fatal(err)
			}
			w.designOpen.settings.confirmRecovery = func(gui.IWidget, string, string) bool { return true }
			var continuation func()
			w.designOpen.settings.afterModal = func(f func()) { continuation = f }
			requestDesignOpen(path, w.tabs, w.canvas, true)
			pumpTestDesignOpenUntil(t, w, func() bool { return continuation != nil })
			scene := w.canvas.GedScene()
			before := scene.SaveDesign().String()
			next := ""
			switch action {
			case "cancel":
				cancelOwnedDesignOpen(w.designOpen)
			case "close":
				w.designOpen.Close()
			case "edit":
				scene.PushCommand(recoveryTitleEdit{scene, scene.FormTitle(), "User edit"})
				before = scene.SaveDesign().String()
			case "draft":
				scene.AddPendingEditor(&restoreDraft{pending: true})
			case "focus":
				f := gui.NewEdit()
				f.SetParent(w.frame)
				f.SetFocus()
			case "newer-open":
				next = designOpenPath(t, "New intent")
				requestDesignOpen(next, w.tabs, w.canvas, true)
			}
			continuation()
			continuation()
			pumpTestDesignOpens(t, w)
			if next != "" {
				if scene.Filename() != next || len(prefs.RecentFiles()) != 1 {
					t.Fatal("newer open lost")
				}
				return
			}
			if scene.SaveDesign().String() != before || scene.Filename() == path || len(prefs.RecentFiles()) != 0 {
				t.Fatal("deferred modal choice overrode new user intent")
			}
		})
	}
}
