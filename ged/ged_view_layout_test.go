package ged

import (
	"testing"

	"silk/geom"
)

// TestBoundingBoxOf checks the pure enclosing-rect helper.
func TestBoundingBoxOf(t *testing.T) {
	if got := boundingBoxOf(nil); (got != geom.Rect{}) {
		t.Errorf("empty input: got %+v, want zero rect", got)
	}
	rects := []geom.Rect{
		{X: 10, Y: 20, Width: 30, Height: 8}, // right=40 bottom=28
		{X: 5, Y: 50, Width: 10, Height: 6},  // left=5  bottom=56
		{X: 60, Y: 15, Width: 20, Height: 5}, // right=80 top=15
	}
	got := boundingBoxOf(rects)
	want := geom.Rect{X: 5, Y: 15, Width: 75, Height: 41} // (5,15) .. (80,56)
	if got != want {
		t.Errorf("boundingBoxOf = %+v, want %+v", got, want)
	}
}

// TestLayOutSelectionWrapsInVBox builds a scene with three widgets,
// selects them, and lays them out vertically. The scene should then
// hold a single top-level VBox container whose children are the three
// originals (reparented out of the scene), ordered top-to-bottom by Y.
func TestLayOutSelectionWrapsInVBox(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	mk := func(name string, y float64) *FakeWidget {
		w, err := NewFakeWidgetFromFactory("gui.Button")
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		w.SetWidgetName(name)
		w.SetBounds(10, y, 30, 8)
		w.SetParent(scene)
		return w
	}
	// Create out of visual order to prove the layout sort reorders them.
	mid := mk("mid", 30)
	top := mk("top", 10)
	bot := mk("bot", 50)

	sel := view.Selection()
	sel.Clear()
	sel.Add(top)
	sel.Add(mid)
	sel.Add(bot)

	view.layOutSelection(false) // vertical → VBox

	// Scene now has exactly one top-level child: the VBox container.
	top0 := scene.Children()
	if len(top0) != 1 {
		t.Fatalf("scene top-level children = %d, want 1 (the container)", len(top0))
	}
	container, ok := top0[0].(*FakeWidget)
	if !ok || container.WidgetFactoryName() != "gui.VBox" {
		t.Fatalf("top child = %T/%v, want *FakeWidget gui.VBox", top0[0],
			func() string {
				if ok {
					return container.WidgetFactoryName()
				}
				return "?"
			}())
	}

	// The three originals are now the container's children.
	kids := container.Children()
	if len(kids) != 3 {
		t.Fatalf("container children = %d, want 3", len(kids))
	}
	// Reparented in top-to-bottom order: top(10), mid(30), bot(50).
	wantOrder := []string{"top", "mid", "bot"}
	for i, k := range kids {
		fw := k.(*FakeWidget)
		if fw.WidgetName() != wantOrder[i] {
			t.Errorf("child[%d] = %q, want %q (layout order by Y)", i, fw.WidgetName(), wantOrder[i])
		}
	}

	// The container is sized to enclose the selection (x10..40, y10..58).
	b := container.Bounds1()
	if b.X != 10 || b.Y != 10 {
		t.Errorf("container origin = (%g,%g), want (10,10)", b.X, b.Y)
	}
	if b.Width != 30 || b.Height != 48 {
		t.Errorf("container size = (%g,%g), want (30,48)", b.Width, b.Height)
	}

	// Selection is now the container.
	if il := view.Selection().ItemList(); len(il) != 1 || il[0] != container {
		t.Errorf("selection after layout should be the new container")
	}
}

// TestLayOutSelectionHBoxSortsByX confirms horizontal layout orders the
// children left-to-right by X.
func TestLayOutSelectionHBoxSortsByX(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	mk := func(name string, x float64) *FakeWidget {
		w, _ := NewFakeWidgetFromFactory("gui.Button")
		w.SetWidgetName(name)
		w.SetBounds(x, 20, 15, 8)
		w.SetParent(scene)
		return w
	}
	r := mk("right", 60)
	l := mk("left", 10)

	sel := view.Selection()
	sel.Clear()
	sel.Add(r)
	sel.Add(l)

	view.layOutSelection(true) // horizontal → HBox

	container := scene.Children()[0].(*FakeWidget)
	if container.WidgetFactoryName() != "gui.HBox" {
		t.Fatalf("container = %q, want gui.HBox", container.WidgetFactoryName())
	}
	kids := container.Children()
	if len(kids) != 2 {
		t.Fatalf("children = %d, want 2", len(kids))
	}
	if kids[0].(*FakeWidget).WidgetName() != "left" || kids[1].(*FakeWidget).WidgetName() != "right" {
		t.Errorf("HBox order = [%s,%s], want [left,right]",
			kids[0].(*FakeWidget).WidgetName(), kids[1].(*FakeWidget).WidgetName())
	}
}

// TestLayOutSelectionNoOpBelowTwo confirms a single-item (or empty)
// selection does nothing — no container is created.
func TestLayOutSelectionNoOpBelowTwo(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	w, _ := NewFakeWidgetFromFactory("gui.Button")
	w.SetWidgetName("solo")
	w.SetParent(scene)

	sel := view.Selection()
	sel.Clear()
	sel.Add(w)

	view.layOutSelection(false)

	// Still just the original widget at the scene top level; no container.
	if n := len(scene.Children()); n != 1 {
		t.Fatalf("scene children = %d, want 1 (unchanged)", n)
	}
	if scene.Children()[0].(*FakeWidget).WidgetName() != "solo" {
		t.Errorf("single-selection layout should not wrap the widget")
	}
}
