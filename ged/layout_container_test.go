package ged

import (
	"testing"

	"github.com/uk0/silk/geom"
)

func containerLayoutFixture(t *testing.T, kind LayoutKind, count int) (*GedView, *FakeWidget, []*FakeWidget) {
	t.Helper()
	v, children := layoutFixture(t, kind)
	box, err := NewFakeWidgetFromFactory(string(kind))
	if err != nil {
		t.Fatal(err)
	}
	box.SetWidgetName("LayoutRoot")
	box.SetBounds(5, 5, 120, 90)
	box.SetParent(v.GedScene())
	for i, c := range children {
		if i < count {
			c.SetParent(box)
		} else {
			c.SetParent(nil)
		}
	}
	v.Selection().Clear()
	v.Selection().Add(box)
	return v, box, children[:count]
}

func TestSingleContainerLayoutRoundTrip(t *testing.T) {
	for _, kind := range []LayoutKind{LayoutHorizontal, LayoutVertical, LayoutGrid, LayoutForm} {
		t.Run(string(kind), func(t *testing.T) {
			v, box, children := containerLayoutFixture(t, kind, 4)
			oldBounds := box.Bounds1()
			before := make([]geom.Rect, len(children))
			for i, c := range children {
				before[i] = c.Bounds1()
			}
			if !v.CanLayoutSelection(kind) || !v.LayoutSelection(kind) {
				t.Fatal("single container layout unavailable")
			}
			if len(v.GedScene().Children()) != 1 || v.GedScene().Children()[0] != box || box.Bounds1() != oldBounds {
				t.Fatal("container identity or bounds changed")
			}
			runtime := box.Generate()
			checkRuntimeLayout(t, runtime, runtime.Children(), kind)
			if v.CanLayoutSelection(kind) || v.LayoutSelection(kind) {
				t.Fatal("already managed container should be a no-op")
			}
			v.Scene().UndoStack().Undo()
			if box.layoutManaged {
				t.Fatal("undo did not restore free layout")
			}
			for i, c := range children {
				if c.Bounds1() != before[i] || c.Parent() != box {
					t.Fatalf("undo failed: %s", c.WidgetName())
				}
			}
			if got := v.Selection().ItemList(); len(got) != 1 || got[0] != box {
				t.Fatal("undo selection lost")
			}
			v.Scene().UndoStack().Redo()
			loaded := NewGedScene()
			if err := loaded.LoadDesign(v.GedScene().SaveDesign()); err != nil {
				t.Fatal(err)
			}
			r := loaded.Children()[0].(*FakeWidget)
			if !r.layoutManaged {
				t.Fatal("layout mode lost on reload")
			}
			runtime = r.Generate()
			checkRuntimeLayout(t, runtime, runtime.Children(), kind)
			_, idx, err := mustDeclFromDesign(t, v.GedScene().SaveDesign()).BuildWithIndex()
			if err != nil {
				t.Fatal(err)
			}
			if idx["LayoutRoot"] == nil {
				t.Fatal("decl lost container")
			}
		})
	}
}

func TestSingleContainerLayoutOneChildAndNestedUndo(t *testing.T) {
	v, box, children := containerLayoutFixture(t, LayoutVertical, 1)
	leaf, err := NewFakeWidgetFromFactory("gui.Label")
	if err != nil {
		t.Fatal(err)
	}
	leaf.SetBounds(12, 15, 5, 5)
	leaf.SetParent(children[0])
	before := leaf.Bounds1()
	if !v.LayoutSelection(LayoutVertical) {
		t.Fatal("one child rejected")
	}
	v.Scene().UndoStack().Undo()
	if leaf.Bounds1() != before {
		t.Fatalf("nested descendant left floating after undo: %+v", leaf.Bounds1())
	}
	v.Scene().UndoStack().Redo()
	if !box.layoutManaged {
		t.Fatal("redo failed")
	}
}

func TestSingleContainerLayoutRefusesUnrelatedKindAndLockedChildren(t *testing.T) {
	v, _, children := containerLayoutFixture(t, LayoutVertical, 1)
	if v.CanLayoutSelection(LayoutGrid) {
		t.Fatal("unrelated kind enabled")
	}
	children[0].SetLockPos(true)
	if v.CanLayoutSelection(LayoutVertical) {
		t.Fatal("locked child would be repositioned")
	}
}
