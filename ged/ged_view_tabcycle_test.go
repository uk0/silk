package ged

import (
	"testing"

	"github.com/uk0/silk/graph"
)

// TestReadingOrder pins the sort the tab walk is built on: top-to-bottom, and
// left-to-right within a row. It must not disturb the caller's slice — the
// walk sorts scene children and container children alike, and the scene's own
// child order is the Z-order the canvas draws from.
func TestReadingOrder(t *testing.T) {
	scene := NewGedScene()
	topRight := addFakeAt(t, scene, "topRight", 50, 10, 10, 5)
	topLeft := addFakeAt(t, scene, "topLeft", 10, 10, 10, 5)
	bottom := addFakeAt(t, scene, "bottom", 30, 40, 10, 5)

	in := []graph.IItem{topRight, topLeft, bottom}
	got := readingOrder(in)

	want := []*FakeWidget{topLeft, topRight, bottom}
	for i, w := range want {
		if got[i] != graph.IItem(w) {
			t.Errorf("position %d = %v, want %s", i, got[i], w.WidgetName())
		}
	}
	if in[0] != graph.IItem(topRight) {
		t.Error("readingOrder sorted the caller's slice in place")
	}
}

// tabWalk presses Tab n times from the current selection and returns the widget
// name selected after each press. Drives selectNextWidget rather than
// OnKeyDown(KeyTab) because the Shift probe reads live window state (see the
// note in ged_view_selectall_test.go) — plain Tab and Shift+Tab share one
// OnKeyDown case, so the helpers are what the case actually dispatches to.
func tabWalk(view *GedView, n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		view.selectNextWidget()
		items := view.Selection().ItemList()
		if len(items) != 1 {
			out = append(out, "")
			continue
		}
		if fw, ok := items[0].(*FakeWidget); ok {
			out = append(out, fw.WidgetName())
		} else {
			out = append(out, "")
		}
	}
	return out
}

// TestTabReachesWidgetNestedInContainer: a widget dropped into a container must
// still be selectable from the keyboard. Tab walks the scene, so a container's
// children have to be part of that walk — otherwise the only way to select them
// is with the mouse, which is exactly what keyboard reachability rules out.
// Ctrl+A (selectAll) already recurses; Tab must agree with it.
func TestTabReachesWidgetNestedInContainer(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	box, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	box.SetWidgetName("box1")
	box.SetBounds(10, 10, 80, 60)
	box.SetParent(scene)

	inner, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	inner.SetWidgetName("innerBtn")
	inner.SetBounds(12, 14, 30, 8)
	inner.SetParent(box)

	// Four presses is more than the two reachable widgets, so a correct walk
	// visits both and wraps; a walk that only sees scene children repeats "box1".
	seen := map[string]bool{}
	for _, name := range tabWalk(view, 4) {
		seen[name] = true
	}
	if !seen["box1"] {
		t.Errorf("Tab never selected the container box1; visited %v", seen)
	}
	if !seen["innerBtn"] {
		t.Errorf("Tab never reached innerBtn nested in box1; visited %v — "+
			"a widget inside a container is unreachable without the mouse", seen)
	}
}

// TestTabSkipsHiddenWidget: Tab must not park the selection on a widget the
// canvas does not draw. Selection handles are drawn from the item's bounds, so
// landing on a hidden widget leaves the user with an invisible selection and no
// way to tell what the arrow keys will move.
func TestTabSkipsHiddenWidget(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	visible := addFakeAt(t, scene, "visible", 10, 10, 30, 8)
	hidden := addFakeAt(t, scene, "hidden", 10, 30, 30, 8)
	hidden.SetVisible(false)

	for _, name := range tabWalk(view, 4) {
		if name == "hidden" {
			t.Fatalf("Tab selected the hidden widget %q", name)
		}
	}
	if !view.Selection().Contains(visible) {
		t.Errorf("after cycling, selection is not the only visible widget")
	}
}

// TestTabWrapsAroundNestedScene: the walk is a cycle. With three reachable
// widgets, four presses must return to the first one.
func TestTabWrapsAroundNestedScene(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	box, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	box.SetWidgetName("box1")
	box.SetBounds(10, 10, 80, 60)
	box.SetParent(scene)

	a, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	a.SetWidgetName("a")
	a.SetBounds(12, 14, 30, 8)
	a.SetParent(box)

	addFakeAt(t, scene, "tail", 10, 100, 30, 8)

	walk := tabWalk(view, 4)
	if len(walk) != 4 {
		t.Fatalf("walk = %v", walk)
	}
	if walk[0] != walk[3] {
		t.Errorf("Tab cycle did not wrap after 3 widgets: %v", walk)
	}
	for _, want := range []string{"box1", "a", "tail"} {
		found := false
		for _, got := range walk {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("Tab walk %v never visited %q", walk, want)
		}
	}
}
