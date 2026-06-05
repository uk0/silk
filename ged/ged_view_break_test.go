package ged

import (
	"testing"
)

// TestBreakLayoutDissolvesContainer: selecting a container and breaking
// it lifts its children back up to the scene and removes the empty
// container; the freed children become the selection.
func TestBreakLayoutDissolvesContainer(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	vbox := sceneWidget(t, scene, "gui.VBox", "box", 10, 10, 40, 40)
	a, _ := NewFakeWidgetFromFactory("gui.Button")
	a.SetWidgetName("a")
	a.SetBounds(12, 14, 30, 8)
	a.SetParent(vbox)
	b, _ := NewFakeWidgetFromFactory("gui.Label")
	b.SetWidgetName("b")
	b.SetBounds(12, 26, 30, 6)
	b.SetParent(vbox)

	sel := view.Selection()
	sel.Clear()
	sel.Add(vbox)
	view.breakLayoutSelection()

	// Scene now holds the two freed widgets directly; the container is gone.
	top := scene.Children()
	if len(top) != 2 {
		t.Fatalf("scene top-level = %d, want 2 (children lifted, container removed)", len(top))
	}
	names := map[string]bool{}
	for _, c := range top {
		fw := c.(*FakeWidget)
		if fw.WidgetFactoryName() == "gui.VBox" {
			t.Error("container still present after break")
		}
		names[fw.WidgetName()] = true
	}
	if !names["a"] || !names["b"] {
		t.Errorf("freed children missing from scene: got %v", names)
	}

	// Freed children are the new selection.
	if il := view.Selection().ItemList(); len(il) != 2 {
		t.Errorf("selection after break = %d items, want 2 (the freed children)", len(il))
	}
}

// TestBreakLayoutRoundTrip: Lay Out then Break Layout restores the
// original flat scene — group and ungroup are inverses.
func TestBreakLayoutRoundTrip(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	a := sceneWidget(t, scene, "gui.Button", "a", 10, 10, 30, 8)
	b := sceneWidget(t, scene, "gui.Button", "b", 10, 30, 30, 8)

	sel := view.Selection()
	sel.Clear()
	sel.Add(a)
	sel.Add(b)
	view.layOutSelection(false) // group into VBox
	if len(scene.Children()) != 1 {
		t.Fatalf("after layout, scene top-level = %d, want 1 container", len(scene.Children()))
	}

	// The container is now selected; break it.
	view.breakLayoutSelection()
	top := scene.Children()
	if len(top) != 2 {
		t.Fatalf("after break, scene top-level = %d, want 2 (flat again)", len(top))
	}
	for _, c := range top {
		if c.(*FakeWidget).WidgetFactoryName() == "gui.VBox" {
			t.Error("container survived the round-trip")
		}
	}
}

// TestBreakLayoutNoOpWithoutContainer: breaking a selection that holds
// no container (just leaf widgets) changes nothing.
func TestBreakLayoutNoOpWithoutContainer(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	a := sceneWidget(t, scene, "gui.Button", "a", 10, 10, 30, 8)
	b := sceneWidget(t, scene, "gui.Button", "b", 10, 30, 30, 8)

	sel := view.Selection()
	sel.Clear()
	sel.Add(a)
	sel.Add(b)
	view.breakLayoutSelection()

	if len(scene.Children()) != 2 {
		t.Errorf("break with no container should be a no-op; scene children = %d, want 2", len(scene.Children()))
	}
}

// TestBreakLayoutNested: breaking an outer container lifts its children
// (including an inner container) up one level, preserving the inner
// container's own nesting.
func TestBreakLayoutNested(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	outer := sceneWidget(t, scene, "gui.VBox", "outer", 10, 10, 60, 50)
	inner, _ := NewFakeWidgetFromFactory("gui.HBox")
	inner.SetWidgetName("inner")
	inner.SetBounds(12, 14, 50, 20)
	inner.SetParent(outer)
	leaf, _ := NewFakeWidgetFromFactory("gui.Button")
	leaf.SetWidgetName("leaf")
	leaf.SetBounds(14, 16, 20, 8)
	leaf.SetParent(inner)

	sel := view.Selection()
	sel.Clear()
	sel.Add(outer)
	view.breakLayoutSelection()

	// outer dissolved → inner now sits at scene level, still holding leaf.
	top := scene.Children()
	if len(top) != 1 {
		t.Fatalf("scene top-level = %d, want 1 (inner container lifted)", len(top))
	}
	innerR := top[0].(*FakeWidget)
	if innerR.WidgetName() != "inner" {
		t.Fatalf("lifted item = %q, want inner", innerR.WidgetName())
	}
	if len(innerR.Children()) != 1 || innerR.Children()[0].(*FakeWidget).WidgetName() != "leaf" {
		t.Errorf("inner container lost its own child during the outer break")
	}
}
