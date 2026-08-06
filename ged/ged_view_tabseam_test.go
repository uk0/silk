package ged

import (
	"os"
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
)

// TestCanvasClaimsTabFromTheWindowLayer is the regression guard for a binding
// that could never fire. gui's window layer owns Tab for focus traversal and
// spends it before any widget's OnKeyDown runs, so the canvas's own Tab branch
// was unreachable: pressing Tab moved focus to the next IDE panel and took it
// off the canvas, and the next Tab walked further away.
//
// The tests the binding shipped with all called selectNextWidget or OnKeyDown
// directly — below the layer that ate the key — so none of them could see it.
func TestCanvasClaimsTabFromTheWindowLayer(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	// Nothing to walk: Tab must be left to the window so focus can leave.
	if view.WantsTab(false) {
		t.Error("an empty canvas claimed Tab; focus can never leave the panel")
	}

	a := sceneWidget(t, scene, "gui.Button", "a", 10, 10, 20, 8)
	sceneWidget(t, scene, "gui.Button", "b", 10, 30, 20, 8)

	if !view.WantsTab(false) {
		t.Fatal("a canvas with widgets did not claim Tab; the window spends it on focus traversal")
	}
	sel := view.Selection().ItemList()
	if len(sel) != 1 {
		t.Fatalf("WantsTab selected %d widgets, want 1", len(sel))
	}
	if sel[0] != a {
		t.Errorf("Tab selected %v, want the first widget in tab order", sel[0])
	}

	view.WantsTab(true) // Shift+Tab walks back
	if got := view.Selection().ItemList(); len(got) != 1 || got[0] == a {
		t.Error("Shift+Tab did not walk backwards through the canvas")
	}
}

// TestBothBackendsAskBeforeSpendingTab pins the seam itself. The claim is only
// honoured if BOTH window layers consult it — an earlier round found the
// shortcut registry inert on Windows because only the GLFW backend dispatched
// it, and this is the same shape of gap.
func TestBothBackendsAskBeforeSpendingTab(t *testing.T) {
	for _, f := range []string{"../gui/window_glfw.go", "../gui/window_windows.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("cannot read %s: %v", f, err)
		}
		// Assert the CALL, not the mere mention of the type: a comment naming
		// IWantsTab, or a renamed identifier that still contains the string,
		// would satisfy a substring check while the key is still spent.
		if !strings.Contains(string(src), "focusWidget.(IWantsTab)") ||
			!strings.Contains(string(src), ".WantsTab(") {
			t.Errorf("%s spends Tab without asking the focused widget; the canvas binding is dead on that backend", f)
		}
	}
	var _ gui.IWantsTab = (*GedView)(nil)
}
