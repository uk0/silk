package main

import (
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// newStatusBarLabels installs fresh permanent indicator labels, standing in for
// createStatusBar which needs a live frame.
func newStatusBarLabels() {
	statusModeLabel = gui.NewLabel("")
	statusWidgetCountLabel = gui.NewLabel("")
	statusZoomLabel = gui.NewLabel("")
	statusInfoLabel = gui.NewLabel("")
}

// dropWidget replays what GedView.OnDrop does once the hit-test has picked a
// parent: a fresh FakeWidget is attached through an AddCommand pushed onto the
// scene's undo stack. Going through the command is the point — it is the path
// drop, paste and their undo/redo all share.
func dropWidget(t *testing.T, gv *ged.GedView, factory string) {
	t.Helper()
	item, err := ged.NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatalf("create %s: %v", factory, err)
	}
	cmd := graph.NewAddCommand()
	cmd.AddItem(item, gv.Scene())
	gv.Scene().PushCommand(cmd)
}

// TestStatusBarWidgetCountFollowsDrops guards the state the designer shipped
// in: three widgets on the canvas and a status bar still reading "0 widgets",
// because nothing refreshed the indicators when the scene gained or lost an
// item — the count only ever showed what was there at startup. Undo and redo
// replay the same AddCommand, so they ride on the same wiring.
func TestStatusBarWidgetCountFollowsDrops(t *testing.T) {
	newStatusBarLabels()
	gv := ged.NewGedView()
	bindStatusBarTo(gv)

	dropWidget(t, gv, "gui.Button")
	dropWidget(t, gv, "gui.Label")
	dropWidget(t, gv, "gui.Edit")
	if got := statusWidgetCountLabel.Text(); got != "3 widgets" {
		t.Errorf("after three drops the count cell reads %q, want %q", got, "3 widgets")
	}

	gv.Scene().UndoStack().Undo()
	if got := statusWidgetCountLabel.Text(); got != "2 widgets" {
		t.Errorf("after undo the count cell reads %q, want %q", got, "2 widgets")
	}

	gv.Scene().UndoStack().Redo()
	if got := statusWidgetCountLabel.Text(); got != "3 widgets" {
		t.Errorf("after redo the count cell reads %q, want %q", got, "3 widgets")
	}
}

// TestStatusBarZoomFollowsTheView covers the other stale cell: the zoom
// percentage was written only where updateStatusBarInfo happened to be called,
// so Ctrl+wheel or a fit-to-view moved the canvas and left "100%" behind.
func TestStatusBarZoomFollowsTheView(t *testing.T) {
	newStatusBarLabels()
	gv := ged.NewGedView()
	bindStatusBarTo(gv)

	gv.SetZoomFactor(2)
	if got := statusZoomLabel.Text(); got != "200%" {
		t.Errorf("zoom cell = %q, want %q", got, "200%")
	}
}

// TestModeShortcutsAreRegisteredGlobally pins the fix for Ctrl+1 / Ctrl+2 doing
// nothing while the code editor had focus: GedView.OnKeyDown is reached only
// after focus routing, so a focused CodeEditor swallowed the keys before the
// canvas could see them. They have to go through the window-level registry,
// which runs first. There is no live window in a test process, so this reads
// the source — crude, but it fails the moment the registration is dropped.
func TestModeShortcutsAreRegisteredGlobally(t *testing.T) {
	body := funcBody(t, "design.go", "func main() {")
	for _, want := range []string{
		"gui.RegisterShortcut(gui.ModAction, '1'",
		"gui.RegisterShortcut(gui.ModAction, '2'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("main() no longer contains %q; the mode shortcut only fires when the canvas has focus", want)
		}
	}
}
