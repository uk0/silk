package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

func transferFixture() *GedView {
	v := NewGedView()
	v.SetSize(940, 670)
	v.GedScene().SetSize(240, 160)
	return v
}

func transferWidget(t *testing.T, parent graph.IItem, factory, name string, x, y, w, h float64) *FakeWidget {
	t.Helper()
	f, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatal(err)
	}
	f.SetWidgetName(name)
	f.SetBounds(x, y, w, h)
	f.SetParent(parent)
	return f
}

func transferBox(t *testing.T, parent graph.IItem, kind LayoutKind, name string, x, y, w, h float64) *FakeWidget {
	return transferWidget(t, parent, string(kind), name, x, y, w, h)
}

func transferButton(t *testing.T, p graph.IItem, name string) *FakeWidget {
	return transferWidget(t, p, "gui.Button", name, p.X()+1, p.Y()+1, 22, 8)
}

func enableFixtureLayout(box *FakeWidget) {
	box.layoutManaged = true
	box.Layout()
}

func centerForTransfer(item graph.IItem) (float64, float64) {
	b := item.MapRectToScene1(item.Bounds1())
	return b.X + b.Width/2, b.Y + b.Height/2
}

func transferDrag(v *GedView, from graph.IItem, hoverX, hoverY float64) {
	clearClickHistory()
	v.OnLeftDown(v.MapFromScene(centerForTransfer(from)))
	v.OnMouseMove(v.MapFromScene(hoverX, hoverY))
}

func TestLayoutTransferReleasesAtActualLinearSlotAndRestoresOneUndo(t *testing.T) {
	v := transferFixture()
	s := v.GedScene()
	src := transferBox(t, s, LayoutVertical, "Source", 12, 15, 55, 50)
	target := transferBox(t, s, LayoutHorizontal, "Target", 85, 15, 110, 50)
	moved := transferButton(t, src, "Moved")
	left, right := transferButton(t, target, "Left"), transferButton(t, target, "Right")
	enableFixtureLayout(src)
	enableFixtureLayout(target)
	before := [3]geom.Rect{moved.Bounds1(), left.Bounds1(), right.Bounds1()}
	changes := 0
	s.SigDesignChanged(func() { changes++ })
	hoverX, hoverY := left.X()+left.Width()*.2, left.Y()+left.Height()/2
	transferDrag(v, moved, hoverX, hoverY)
	if hint := v.managedHint; !hint.valid || hint.insertion.index != 0 {
		t.Fatalf("expected managed preview before first child: %+v", hint)
	}
	releaseX := right.X() + right.Width()*.2
	v.OnLeftUp(v.MapFromScene(releaseX, hoverY))
	if !reflect.DeepEqual(target.Children(), []graph.IItem{left, moved, right}) || src.HasChildren() ||
		changes != 1 || s.UndoStack().Count() != 1 || !v.Selection().Contains(moved) {
		t.Fatalf("transfer not committed as one indexed edit: %v, changes=%d", target.Children(), changes)
	}
	after := [3]geom.Rect{moved.Bounds1(), left.Bounds1(), right.Bounds1()}
	v.Undo()
	if moved.Parent() != graph.IItem(src) || moved.IndexInParent() != 0 ||
		[3]geom.Rect{moved.Bounds1(), left.Bounds1(), right.Bounds1()} != before || s.UndoStack().CanUndo() {
		t.Fatal("one undo did not restore original graph and geometry")
	}
	v.Redo()
	if moved.IndexInParent() != 1 || [3]geom.Rect{moved.Bounds1(), left.Bounds1(), right.Bounds1()} != after {
		t.Fatal("redo did not reproduce planned placement")
	}
	doc := s.SaveDesign()
	reloaded := NewGedScene()
	if err := reloaded.LoadDesign(doc); err != nil {
		t.Fatal(err)
	}
	for _, n := range []*decl.Node{buildSceneDeclNode(reloaded), mustDeclFromDesign(t, doc)} {
		_, index, err := n.BuildWithIndex()
		if err != nil {
			t.Fatal(err)
		}
		runtime := index["Target"].(gui.IWidget)
		if !reflect.DeepEqual(runtime.Children(), []gui.IWidget{index["Left"].(gui.IWidget),
			index["Moved"].(gui.IWidget), index["Right"].(gui.IWidget)}) {
			t.Fatal("runtime order did not survive transfer and save")
		}
	}
}

func TestLayoutTransferGridSpanPreservesVacancyAndImplicitSourceSlots(t *testing.T) {
	v := transferFixture()
	s := v.GedScene()
	src := transferBox(t, s, LayoutGrid, "SourceGrid", 12, 15, 70, 90)
	dst := transferBox(t, s, LayoutGrid, "TargetGrid", 100, 15, 112, 90)
	moved := transferButton(t, src, "Big")
	moved.layoutCell = cell(2, 0, 1, 2)
	unset := transferButton(t, src, "Legacy")
	spanLimit := transferButton(t, dst, "Limit")
	spanLimit.layoutCell = cell(1, 2, 1, 1)
	enableFixtureLayout(src)
	enableFixtureLayout(dst)
	was := placementForChild(unset)
	x, y := centerOfTabularCell(t, dst, 0, 0)
	transferDrag(v, moved, x, y)
	if !v.managedHint.valid || v.managedHint.cell.cell != cell(0, 0, 1, 2) {
		t.Fatalf("incoming footprint not planned: %+v", v.managedHint)
	}
	v.OnLeftUp(v.MapFromScene(x, y))
	if moved.Parent() != graph.IItem(dst) || moved.layoutCell != cell(0, 0, 1, 2) ||
		placementForChild(unset) != was || spanLimit.layoutCell != cell(1, 2, 1, 1) {
		t.Fatalf("transfer changed count, span, or legacy effective placement: %v", moved.Parent())
	}
	v.Undo()
	if moved.Parent() != graph.IItem(src) || moved.layoutCell != cell(2, 0, 1, 2) || unset.layoutCell.Valid {
		t.Fatal("undo did not restore the original implicit cell metadata")
	}
}

func TestLayoutTransferPreservesSourceOnCancelOccupiedUnsupportedOrLocked(t *testing.T) {
	for _, mode := range []string{"escape", "occupied", "unsupported-span", "lock-displaced", "outside-form"} {
		t.Run(mode, func(t *testing.T) {
			v := transferFixture()
			s := v.GedScene()
			srcKind, destKind := LayoutHorizontal, LayoutVertical
			if mode == "unsupported-span" {
				srcKind, destKind = LayoutGrid, LayoutForm
			}
			if mode == "occupied" {
				destKind = LayoutForm
			}
			src := transferBox(t, s, srcKind, "Source", 12, 15, 85, 70)
			dst := transferBox(t, s, destKind, "Destination", 110, 15, 95, 70)
			moved, sibling := transferButton(t, src, "Moved"), transferButton(t, src, "Sibling")
			if mode == "unsupported-span" {
				moved.layoutCell, sibling.layoutCell = cell(0, 0, 1, 2), cell(1, 0, 1, 1)
			}
			d := transferButton(t, dst, "Placed")
			d.layoutCell = cell(0, 0, 1, 1)
			enableFixtureLayout(src)
			enableFixtureLayout(dst)
			if mode == "lock-displaced" {
				sibling.SetLockPos(true)
			}
			before := [3]geom.Rect{moved.Bounds1(), sibling.Bounds1(), d.Bounds1()}
			x, y := centerForTransfer(d)
			if mode == "unsupported-span" {
				x, y = centerOfTabularCell(t, dst, 0, 1)
			}
			transferDrag(v, moved, x, y)
			if mode == "escape" {
				v.OnKeyDown(gui.KeyEsc, false)
			}
			if mode == "outside-form" {
				x = 251
			}
			v.OnLeftUp(v.MapFromScene(x, y))
			if !reflect.DeepEqual(src.Children(), []graph.IItem{moved, sibling}) || s.UndoStack().CanUndo() ||
				[3]geom.Rect{moved.Bounds1(), sibling.Bounds1(), d.Bounds1()} != before {
				t.Fatalf("invalid transfer mutated source or destination: %v", s.UndoStack().Count())
			}
		})
	}
}

func TestFreeCanvasTransfersIntoExplicitVacantFormCell(t *testing.T) {
	v := transferFixture()
	s := v.GedScene()
	f := transferBox(t, s, LayoutForm, "Destination", 90, 15, 100, 70)
	already := transferButton(t, f, "Label")
	already.layoutCell = cell(0, 0, 1, 1)
	enableFixtureLayout(f)
	w := sceneWidget(t, s, "gui.Button", "Free", 15, 30, 32, 10)
	before := w.Bounds1()
	x, y := centerOfTabularCell(t, f, 0, 1)
	fromX, fromY := centerForTransfer(w)
	dragOnCanvas(v, fromX, fromY, x, y)
	if w.Parent() != graph.IItem(f) || w.layoutCell != cell(0, 1, 1, 1) || s.UndoStack().Count() != 1 {
		t.Fatalf("free canvas did not enter an explicit cell: %v, %+v", w.Parent(), w.layoutCell)
	}
	v.Undo()
	if w.Parent() != graph.IItem(s) || w.Bounds1() != before || w.layoutCell.Valid || s.UndoStack().CanUndo() {
		t.Fatal("one undo did not restore free placement")
	}
}

func TestLayoutTransferToAncestorReflowsNestedManagedOrigin(t *testing.T) {
	v := transferFixture()
	s := v.GedScene()
	target := transferBox(t, s, LayoutVertical, "Outer", 20, 18, 110, 115)
	source := transferBox(t, target, LayoutHorizontal, "Inner", 21, 19, 106, 44)
	hold := transferButton(t, target, "Tail")
	move, sibling := transferButton(t, source, "Moved"), transferButton(t, source, "Sibling")
	enableFixtureLayout(source)
	enableFixtureLayout(target)
	deepBefore := sibling.Bounds1()
	slot := hold.Bounds1()
	x, y := slot.X+slot.Width/2, slot.Y+slot.Height*.8
	transferDrag(v, move, x, y)
	if !v.managedHint.valid {
		t.Fatal("nested-to-ancestor target was not planned")
	}
	v.OnLeftUp(v.MapFromScene(x, y))
	if move.Parent() != graph.IItem(target) || move.IndexInParent() != 2 || source.Children()[0] != sibling ||
		drawnRect(sibling).IsEmpty() || drawnRect(move).IsEmpty() || s.UndoStack().Count() != 1 {
		t.Fatal("nested transfer did not settle in one visible transaction")
	}
	v.Undo()
	if move.Parent() != graph.IItem(source) || move.IndexInParent() != 0 || sibling.Bounds1() != deepBefore {
		t.Fatal("nested transfer undo lost saved shared-coordinate geometry")
	}
}

func TestManagedPositionOnlyMoveReflowsSharedDescendants(t *testing.T) {
	v := transferFixture()
	s := v.GedScene()
	outer := transferBox(t, s, LayoutVertical, "Outer", 20, 16, 100, 88)
	inner := transferBox(t, outer, LayoutHorizontal, "Inner", 23, 19, 92, 48)
	child := transferButton(t, inner, "Child")
	enableFixtureLayout(inner)
	enableFixtureLayout(outer)
	x0, y0 := child.Pos()
	bw, bh := inner.Size()
	outer.setDesignX(outer.X() + 17)
	outer.setDesignY(outer.Y() + 13)
	if x, y := child.Pos(); x != x0+17 || y != y0+13 {
		t.Fatalf("descendant remained at the old layout origin (%v,%v)", x, y)
	}
	if w, h := inner.Size(); w != bw || h != bh {
		t.Fatal("fixture changed size rather than testing an origin-only change")
	}
}

func TestLayoutTransferKeepsTransientGeometryHintOffStatusBar(t *testing.T) {
	v := transferFixture()
	frame := gui.NewFrame()
	status := gui.NewStatusBar()
	frame.SetStatusBar(status)
	v.SetParent(frame)
	s := v.GedScene()
	from := transferBox(t, s, LayoutVertical, "From", 15, 20, 60, 64)
	to := transferBox(t, s, LayoutHorizontal, "To", 90, 20, 95, 52)
	moved := transferButton(t, from, "Moved")
	peer := transferButton(t, to, "Peer")
	enableFixtureLayout(from)
	enableFixtureLayout(to)

	status.ShowMessage("selection-status")
	x, y := from.X()+from.Width()*.7, from.Y()+from.Height()*.5
	transferDrag(v, moved, x, y)
	if !v.managedHint.active || v.managedHint.valid || status.Message() != "selection-status" {
		t.Fatalf("drag fallback overwrote unrelated status: %q", status.Message())
	}
	x, y = centerForTransfer(peer)
	v.OnMouseMove(v.MapFromScene(x, y))
	if !v.managedHint.valid || status.Message() != "selection-status" {
		t.Fatalf("valid cross-layout preview kept a blocked status: %q", status.Message())
	}
	v.OnPointerCancel()
}
