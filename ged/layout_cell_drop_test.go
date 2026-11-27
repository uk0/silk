package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func cell(r, c, h, w int) layoutPlacement {
	return layoutPlacement{Row: r, Column: c, RowSpan: h, ColumnSpan: w, Valid: true}
}

func centerOfTabularCell(t *testing.T, p *FakeWidget, row, col int) (float64, float64) {
	t.Helper()
	s := p.tabularTracks()
	r, ok := trackByIndex(s.Rows, row)
	c, colOK := trackByIndex(s.Columns, col)
	if !ok || !colOK || r.Size <= 0 || c.Size <= 0 {
		t.Fatalf("no visible track for row %d, col %d: %+v", row, col, s)
	}
	origin := p.MapRectToScene1(p.Bounds1())
	return origin.X + gui.PixelToMm(c.Start+c.Size/2), origin.Y + gui.PixelToMm(r.Start+r.Size/2)
}

func tabularFixture(t *testing.T, kind LayoutKind, placements ...layoutPlacement) (*GedView, *FakeWidget, []*FakeWidget) {
	t.Helper()
	v := NewGedView()
	v.SetSize(900, 650)
	v.GedScene().SetSize(185, 145)
	parent := sceneWidget(t, v.GedScene(), string(kind), "cells", 15, 15, 140, 112)
	items := make([]*FakeWidget, len(placements))
	for i, p := range placements {
		f := sceneWidget(t, v.GedScene(), "gui.Button", "testButton"+string(rune('A'+i)), 18, 19, 22, 7)
		f.SetParent(parent)
		f.layoutCell = p
		items[i] = f
	}
	parent.layoutManaged = true
	parent.Layout()
	return v, parent, items
}

func TestTabularPaletteTargetsVacantCellsRejectsSpansAndUsesRelease(t *testing.T) {
	v, p, items := tabularFixture(t, LayoutGrid, cell(0, 0, 1, 2), cell(2, 2, 1, 1))
	s := v.GedScene()
	ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
	x, y := centerOfTabularCell(t, p, 0, 1)
	a, b := v.MapFromScene(x, y)
	v.OnDragMove(a, b, ctx)
	if v.paletteDrop.valid || v.paletteDrop.hint != i18n.T(dropCellTaken) {
		t.Fatalf("a span's interior was treated as empty: %+v", v.paletteDrop)
	}
	x, y = centerOfTabularCell(t, p, 0, 2)
	a, b = v.MapFromScene(x, y)
	v.OnDragMove(a, b, ctx)
	if !v.paletteDrop.valid || v.paletteDrop.cell.cell != cell(0, 2, 1, 1) || v.paletteDrop.cell.span.IsEmpty() {
		t.Fatal("missing vacant slot preview")
	}
	x, y = centerOfTabularCell(t, p, 1, 1)
	a, b = v.MapFromScene(x, y)
	v.OnDrop(a, b, ctx)
	c := p.Children()
	if len(c) != 3 || ctx.Action() != gui.DndCopy || s.UndoStack().Count() != 1 {
		t.Fatalf("release did not apply one insertion (%v)", ctx.Action())
	}
	added := c[2].(*FakeWidget)
	if added.layoutCell != cell(1, 1, 1, 1) || items[0].layoutCell != cell(0, 0, 1, 2) {
		t.Fatal("release used hover or changed the original span")
	}
	v.Undo()
	if len(p.Children()) != 2 || s.UndoStack().CanUndo() {
		t.Fatal("undo should remove the insertion as one edit")
	}
	v.Redo()
	added.SetWidgetName("PlacedGridButton")
	doc := s.SaveDesign()
	reopened := NewGedScene()
	if err := reopened.LoadDesign(doc); err != nil {
		t.Fatal(err)
	}
	actual := reopened.Children()[0].(*FakeWidget).Children()[2].(*FakeWidget)
	if actual.layoutCell != added.layoutCell {
		t.Fatal("cell did not survive SaveDesign / LoadDesign")
	}
	for _, d := range []*decl.Node{buildSceneDeclNode(reopened), mustDeclFromDesign(t, doc)} {
		_, idx, err := d.BuildWithIndex()
		if err != nil {
			t.Fatal(err)
		}
		w, ok := idx[added.WidgetName()].(gui.IWidget)
		if !ok {
			t.Fatal("preview/runtime lost the added cell")
		}
		owner, ok := w.Parent().(*gui.GridLayout)
		if !ok {
			t.Fatal("added widget does not belong to its generated grid")
		}
		tracks := owner.CellTracks()
		row, rowOK := trackByIndex(tracks.Rows, 1)
		col, colOK := trackByIndex(tracks.Columns, 1)
		x, y, wmm, hmm := w.Bounds()
		if !rowOK || !colOK || x != col.Start || y != row.Start || wmm != col.Size || hmm != row.Size {
			t.Fatalf("generated layout placement: got %g,%g,%g,%g; row=%+v col=%+v", x, y, wmm, hmm, row, col)
		}
	}
}

func TestTabularSwapAndCancelPreserveOwnershipAndOneUndo(t *testing.T) {
	for _, kind := range []LayoutKind{LayoutGrid, LayoutForm} {
		t.Run(string(kind), func(t *testing.T) {
			a, b := cell(0, 0, 1, 1), cell(1, 1, 1, 1)
			v, p, items := tabularFixture(t, kind, a, b)
			s := v.GedScene()
			original := p.Children()
			signals := 0
			s.SigDesignChanged(func() { signals++ })
			arm := func() (float64, float64) {
				t.Helper()
				clearClickHistory()
				x, y := centerOfTabularCell(t, p, a.Row, a.Column)
				toX, toY := centerOfTabularCell(t, p, b.Row, b.Column)
				v.OnLeftDown(v.MapFromScene(x, y))
				px, py := v.MapFromScene(toX, toY)
				v.OnMouseMove(px, py)
				if !v.managedHint.cell.valid || v.managedHint.cell.swap != items[1] {
					t.Fatalf("no swap preview at %v: %+v", kind, v.managedHint)
				}
				return px, py
			}
			x, y := arm()
			v.OnPointerCancel()
			v.OnLeftUp(x, y)
			if items[0].layoutCell != a || s.UndoStack().CanUndo() || signals != 0 {
				t.Fatal("cancel wrote an active cell slot")
			}
			x, y = arm()
			v.OnLeftUp(x, y)
			if items[0].layoutCell != b || items[1].layoutCell != a || !reflect.DeepEqual(p.Children(), original) ||
				signals != 1 || !v.Selection().Contains(items[0]) {
				t.Fatal("swap changed owner/order, failed to swap, or was not one edit")
			}
			v.Undo()
			if items[0].layoutCell != a || items[1].layoutCell != b || s.UndoStack().CanUndo() {
				t.Fatal("one undo did not restore both slots")
			}
			v.Redo()
			if items[0].layoutCell != b || items[1].layoutCell != a {
				t.Fatal("redo lost the swap")
			}
		})
	}
}

func TestTabularMovePreservesSpanAndNeverDisplacesLockedCell(t *testing.T) {
	v, p, items := tabularFixture(t, LayoutGrid, cell(0, 0, 1, 2), cell(1, 3, 1, 1))
	page := v.Scene().Bounds1()
	clip := designDropArea(page, p)
	items[1].SetLockPos(true)
	x, y := centerOfTabularCell(t, p, 1, 2)
	plan := v.tabularLayoutDrop(p, clip, x, y, items[0])
	if plan.valid || plan.failure != dropCellTaken {
		t.Fatal("span allowed to cover its locked neighbor")
	}
	x, y = centerOfTabularCell(t, p, 1, 0)
	plan = v.tabularLayoutDrop(p, clip, x, y, items[0])
	if !plan.valid || plan.cell != cell(1, 0, 1, 2) {
		t.Fatal("empty span does not preserve geometry")
	}
	fromX, fromY := centerOfTabularCell(t, p, 0, 0)
	dragOnCanvas(v, fromX, fromY, x, y)
	if items[0].layoutCell != plan.cell || items[1].layoutCell != cell(1, 3, 1, 1) ||
		items[0].Parent() != graph.IItem(p) {
		t.Fatal("drag did not preserve span and owner")
	}
}

func TestTabularReleaseRefusesRowsThatWouldVanish(t *testing.T) {
	v, p, _ := tabularFixture(t, LayoutForm, cell(0, 0, 1, 1))
	p.Widget().(*gui.FormLayout).SetRowHeight(gui.MmToPixel(p.Height()) + 20)
	p.Layout()
	added, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	plan := tabularDropPlan{cell: cell(1, 1, 1, 1), valid: true}
	if v.tabularReleaseFits(p, added, plan, true) || len(p.Children()) != 1 || v.Scene().UndoStack().CanUndo() {
		t.Fatal("trial accepted a row fully clipped out of its container")
	}
}

func TestTabularCanvasTransferCannotBypassProtectedCells(t *testing.T) {
	v, target, items := tabularFixture(t, LayoutForm, cell(0, 0, 1, 1))
	s := v.GedScene()
	free := sceneWidget(t, s, "gui.Button", "free", 15, 132, 30, 7)
	before := free.Bounds1()
	x, y := centerOfTabularCell(t, target, 0, 0)
	dragOnCanvas(v, 24, 135, x, y)
	if free.Parent() != graph.IItem(s) || free.Bounds1() != before || len(target.Children()) != 1 ||
		items[0].layoutCell != cell(0, 0, 1, 1) || s.UndoStack().CanUndo() {
		t.Fatal("a free-canvas move bypassed cell occupancy and mutated the form")
	}
}
