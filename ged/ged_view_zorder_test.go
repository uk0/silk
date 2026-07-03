package ged

import (
	"github.com/uk0/silk/graph"
	"testing"
)

// sceneOrder returns the widget names of a scene's children in stacking
// (child iteration) order: back-to-front.
func sceneOrder(scene *GedScene) []string {
	var names []string
	for _, c := range scene.Children() {
		if fw, ok := c.(*FakeWidget); ok {
			names = append(names, fw.WidgetName())
		}
	}
	return names
}

// addFake drops a named FakeWidget onto the scene and returns it.
func addFake(t *testing.T, scene *GedScene, name string) *FakeWidget {
	t.Helper()
	fw, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	fw.SetWidgetName(name)
	fw.SetBounds(10, 10, 30, 8)
	fw.SetParent(scene)
	return fw
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestZorderReorderThenSaveLoadPreservesOrder reorders widgets on a GedScene
// via the graph Z-order methods, then round-trips through SaveDesign/
// LoadDesign and asserts the new stacking order survives serialization.
func TestZorderReorderThenSaveLoadPreservesOrder(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFake(t, scene, "a")
	b := addFake(t, scene, "b")
	_ = addFake(t, scene, "c")

	if got := sceneOrder(scene); !eqStrings(got, []string{"a", "b", "c"}) {
		t.Fatalf("initial order = %v, want [a b c]", got)
	}

	// Bring a (the backmost) to the front: a b c -> b c a
	a.BringToFront()
	if got := sceneOrder(scene); !eqStrings(got, []string{"b", "c", "a"}) {
		t.Fatalf("after BringToFront(a): %v, want [b c a]", got)
	}

	// Lower b one step: b is the head, no-op -> still b c a
	b.Lower()
	// Raise b one step: b c a -> c b a
	b.Raise()
	if got := sceneOrder(scene); !eqStrings(got, []string{"c", "b", "a"}) {
		t.Fatalf("after Raise(b): %v, want [c b a]", got)
	}

	want := sceneOrder(scene)

	// Round-trip through save/load and confirm the stacking order is intact.
	doc := scene.SaveDesign()
	if doc == nil {
		t.Fatal("SaveDesign returned nil")
	}
	scene2 := NewGedScene()
	if err := scene2.LoadDesign(doc); err != nil {
		t.Fatalf("LoadDesign: %v", err)
	}

	got := sceneOrder(scene2)
	if !eqStrings(got, want) {
		t.Errorf("order after save/load = %v, want %v", got, want)
	}
}

// TestZorderReorderSelectionViaView exercises the GedView context-menu glue
// (reorderSelection) on a multi-item selection and confirms the underlying
// scene order changes.
func TestZorderReorderSelectionViaView(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	addFake(t, scene, "a")
	b := addFake(t, scene, "b")
	addFake(t, scene, "c")

	// Select b and send it to the back: a b c -> b a c
	view.Selection().Clear()
	view.Selection().Add(b)
	view.reorderSelection(graph.IItem.SendToBack)

	if got := sceneOrder(scene); !eqStrings(got, []string{"b", "a", "c"}) {
		t.Errorf("after reorderSelection SendToBack(b): %v, want [b a c]", got)
	}
}
