package main

import (
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
)

// TestHistoryPanelFollowsTheActiveDocument covers the re-aim: one 历史 panel is
// shared by every open design, so switching tabs has to point it at the new
// document's undo stack. Left bound to the first document it lists commands
// that belong to a canvas the user is no longer looking at, and clicking a row
// undoes them there.
func TestHistoryPanelFollowsTheActiveDocument(t *testing.T) {
	historyPanel = ged.NewUndoPanel()
	defer func() { historyPanel = nil }()

	first := ged.NewGedView()
	second := ged.NewGedView()

	bindHistoryPanelTo(first)
	if got := historyPanel.Scene(); got != first.GedScene() {
		t.Errorf("panel bound to %p, want the first document's scene %p", got, first.GedScene())
	}

	bindHistoryPanelTo(second)
	if got := historyPanel.Scene(); got != second.GedScene() {
		t.Errorf("after the tab change the panel is still on %p, want %p", got, second.GedScene())
	}

	// A tab with no canvas behind it must leave the panel where it was rather
	// than blanking it or panicking.
	bindHistoryPanelTo(nil)
	if got := historyPanel.Scene(); got != second.GedScene() {
		t.Errorf("binding nil moved the panel to %p, want %p", got, second.GedScene())
	}
}

// TestHistoryPanelIsDockedAndRebound reads createPanels because it needs a live
// frame to run. The panel has to be created and docked, and bindHistoryPanelTo
// has to be called both at startup and from the tab-change callback — one call
// site alone leaves the panel either empty or stuck on the first document.
func TestHistoryPanelIsDockedAndRebound(t *testing.T) {
	body := funcBody(t, "design.go", "func createPanels(mainFrame *gui.Frame) {")
	for _, want := range []string{
		"historyPanel = ged.NewUndoPanel()",
		"rightDockI.AddView(historyPanel)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("createPanels no longer contains %q", want)
		}
	}
	if n := strings.Count(body, "bindHistoryPanelTo(gv)"); n < 2 {
		t.Errorf("createPanels binds the history panel %d time(s); it must bind at startup and again on every tab change", n)
	}
}
