package ged

import (
	"testing"

	"github.com/uk0/silk/gui"
)

// shortcutByCommand returns the registry entry for a command.
func shortcutByCommand(t *testing.T, command string) ShortcutDef {
	t.Helper()
	for _, sc := range designerShortcuts {
		if sc.Command == command {
			return sc
		}
	}
	t.Fatalf("designerShortcuts has no entry for %s", command)
	return ShortcutDef{}
}

// pressCommand presses the chord the registry documents for command. Driving
// the registry's own Mods and Key — rather than a chord repeated in the test —
// is what makes these checks fail when the panel starts advertising a key the
// canvas does not answer.
func pressCommand(t *testing.T, view *GedView, command string) {
	t.Helper()
	sc := shortcutByCommand(t, command)
	var held []int
	if sc.Mods&gui.ModAction != 0 {
		held = append(held, gui.KeyCtrl)
	}
	if sc.Mods&gui.ModShift != 0 {
		held = append(held, gui.KeyShift)
	}
	if sc.Mods&gui.ModAlt != 0 {
		held = append(held, gui.KeyMenu)
	}
	holdKeys(t, held...)
	view.OnKeyDown(sc.Key, false)
}

// canvasCommandOptOut names the "global" bindings GedView.OnKeyDown does not
// implement, with the reason. Anything not listed here must be exercised by
// TestDocumentedChordsFireOnTheCanvas below, so a binding cannot be documented
// in the 快捷键 panel without something proving the canvas answers it.
var canvasCommandOptOut = map[string]string{
	"file.new":          "menubar command, no canvas key case",
	"file.open":         "menubar command, no canvas key case",
	"file.saveAs":       "menubar command, no canvas key case",
	"file.save":         "canvas case exists but writes the design to disk",
	"view.zoomIn":       "frame-level zoom, no canvas key case",
	"view.zoomOut":      "frame-level zoom, no canvas key case",
	"debug.perfOverlay": "frame-level overlay, no canvas key case",
	"help.shortcuts":    "frame accelerator registered by design.go, no canvas key case",
	"design.nudge":      "arrow family, covered by ged_view_nudge_test.go",
	"design.resize":     "arrow family, covered by ged_view_resize_test.go",
}

// callbackCommands are the bindings OnKeyDown forwards to a package callback.
// The address of the callback var is taken so the test can install a spy and
// put the original back.
func callbackCommands() map[string]*func() {
	return map[string]*func(){
		"view.design":         &SwitchToDesignCallback,
		"view.code":           &SwitchToEditCallback,
		"run.preview":         &PreviewCallback,
		"run.compile":         &RunCallback,
		"file.quickOpen":      &QuickOpenCallback,
		"design.alignLeft":    &AlignLeftCallback,
		"design.alignRight":   &AlignRightCallback,
		"design.alignTop":     &AlignTopCallback,
		"design.alignBottom":  &AlignBottomCallback,
		"design.alignCenterH": &AlignCenterHCallback,
		"design.alignCenterV": &AlignCenterVCallback,
		"design.distributeH":  &DistributeHCallback,
		"design.distributeV":  &DistributeVCallback,
	}
}

// TestDocumentedChordsFireOnTheCanvas presses every canvas chord the 快捷键
// panel prints and checks the canvas actually did the thing. Before the
// modifier probe was indirected, none of these could be driven headlessly at
// all — the panel could advertise any chord it liked and no test would notice.
func TestDocumentedChordsFireOnTheCanvas(t *testing.T) {
	// Selection and structural commands, each with its own scene.
	structural := []struct {
		command string
		check   func(t *testing.T, view *GedView)
	}{
		{"design.nextWidget", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			first := addFakeAt(t, scene, "first", 10, 10, 30, 8)
			second := addFakeAt(t, scene, "second", 10, 30, 30, 8)
			view.Selection().Add(first)
			pressCommand(t, view, "design.nextWidget")
			if !view.Selection().Contains(second) {
				t.Error("selection did not advance")
			}
		}},
		{"design.prevWidget", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			first := addFakeAt(t, scene, "first", 10, 10, 30, 8)
			last := addFakeAt(t, scene, "last", 10, 30, 30, 8)
			view.Selection().Add(first)
			pressCommand(t, view, "design.prevWidget")
			if !view.Selection().Contains(last) {
				t.Error("selection did not step back")
			}
		}},
		{"edit.selectAll", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			addFakeAt(t, scene, "a", 10, 10, 30, 8)
			addFakeAt(t, scene, "b", 10, 30, 30, 8)
			pressCommand(t, view, "edit.selectAll")
			if got := view.Selection().Count(); got != 2 {
				t.Errorf("selection count = %d, want 2", got)
			}
		}},
		{"edit.deselect", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			view.Selection().Add(addFakeAt(t, scene, "a", 10, 10, 30, 8))
			pressCommand(t, view, "edit.deselect")
			if !view.Selection().IsEmpty() {
				t.Error("selection survived")
			}
		}},
		{"edit.delete", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			view.Selection().Add(addFakeAt(t, scene, "a", 10, 10, 30, 8))
			pressCommand(t, view, "edit.delete")
			if got := len(scene.Children()); got != 0 {
				t.Errorf("scene still has %d children", got)
			}
		}},
		{"edit.copy", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			view.Selection().Add(addFakeAt(t, scene, "a", 10, 10, 30, 8))
			clipboard = nil
			pressCommand(t, view, "edit.copy")
			if len(clipboard) != 1 {
				t.Errorf("clipboard holds %d roots, want 1", len(clipboard))
			}
		}},
		{"edit.paste", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			view.Selection().Add(addFakeAt(t, scene, "a", 10, 10, 30, 8))
			view.CopySelected()
			pressCommand(t, view, "edit.paste")
			if got := len(scene.Children()); got != 2 {
				t.Errorf("scene has %d children after paste, want 2", got)
			}
		}},
		{"edit.cut", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			view.Selection().Add(addFakeAt(t, scene, "a", 10, 10, 30, 8))
			clipboard = nil
			pressCommand(t, view, "edit.cut")
			if got := len(scene.Children()); got != 0 {
				t.Errorf("scene still has %d children after cut", got)
			}
			if len(clipboard) != 1 {
				t.Errorf("cut left %d roots on the clipboard, want 1", len(clipboard))
			}
		}},
		{"edit.duplicate", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			view.Selection().Add(addFakeAt(t, scene, "a", 10, 10, 30, 8))
			pressCommand(t, view, "edit.duplicate")
			if got := len(scene.Children()); got != 2 {
				t.Errorf("scene has %d children after duplicate, want 2", got)
			}
		}},
		{"edit.undo", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			view.Selection().Add(addFakeAt(t, scene, "a", 10, 10, 30, 8))
			view.DeleteSelectedItems()
			pressCommand(t, view, "edit.undo")
			if got := len(scene.Children()); got != 1 {
				t.Errorf("undo left %d children, want the deleted widget back", got)
			}
		}},
		{"edit.redo", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			view.Selection().Add(addFakeAt(t, scene, "a", 10, 10, 30, 8))
			view.DeleteSelectedItems()
			view.Scene().UndoStack().Undo()
			pressCommand(t, view, "edit.redo")
			if got := len(scene.Children()); got != 0 {
				t.Errorf("redo left %d children, want the delete re-applied", got)
			}
		}},
		{"design.rename", func(t *testing.T, view *GedView) {
			scene := view.GedScene()
			fw := addFakeAt(t, scene, "before", 10, 10, 30, 8)
			view.Selection().Add(fw)
			answerRename(t, "after", true)
			pressCommand(t, view, "design.rename")
			if fw.WidgetName() != "after" {
				t.Errorf("widget name = %q, want after", fw.WidgetName())
			}
		}},
	}

	covered := map[string]bool{}
	for _, tc := range structural {
		covered[tc.command] = true
		t.Run(tc.command, func(t *testing.T) {
			tc.check(t, NewGedView())
		})
	}

	for command, slot := range callbackCommands() {
		covered[command] = true
		t.Run(command, func(t *testing.T) {
			fired := false
			prev := *slot
			*slot = func() { fired = true }
			defer func() { *slot = prev }()

			pressCommand(t, NewGedView(), command)
			if !fired {
				t.Errorf("pressing %s did not reach the %s callback",
					shortcutByCommand(t, command).chord(), command)
			}
		})
	}

	// The other direction: a canvas binding may not be documented without
	// something here proving it works, or an explicit opt-out saying why not.
	for _, sc := range designerShortcuts {
		if sc.Context != "global" || covered[sc.Command] {
			continue
		}
		if _, ok := canvasCommandOptOut[sc.Command]; !ok {
			t.Errorf("%s (%s) is documented as a global binding but nothing here "+
				"presses it; add a check or a canvasCommandOptOut reason",
				sc.Command, sc.chord())
		}
	}
	for command := range canvasCommandOptOut {
		if covered[command] {
			t.Errorf("%s is both exercised and listed in canvasCommandOptOut", command)
		}
	}
}
