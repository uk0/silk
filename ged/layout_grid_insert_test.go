package ged

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func TestGridTrackInsertionRoundTripAndPaletteDrop(t *testing.T) {
	for _, c := range []struct {
		name   string
		axis   GridTrackAxis
		before []layoutPlacement
		after  []layoutPlacement
	}{
		{"row", GridRow,
			[]layoutPlacement{cell(0, 0, 1, 2), cell(1, 0, 1, 1), cell(2, 1, 1, 1)},
			[]layoutPlacement{cell(0, 0, 1, 2), cell(2, 0, 1, 1), cell(3, 1, 1, 1)}},
		{"column", GridColumn,
			[]layoutPlacement{cell(0, 0, 2, 1), cell(0, 1, 1, 1), cell(1, 2, 1, 1)},
			[]layoutPlacement{cell(0, 0, 2, 1), cell(0, 2, 1, 1), cell(1, 3, 1, 1)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			v, parent, widgets := tabularFixture(t, LayoutGrid, c.before...)
			s := v.GedScene()
			order := parent.Children()
			var before []geom.Rect
			for _, w := range widgets {
				before = append(before, w.Bounds1())
			}
			v.Selection().Add(widgets[1])
			if !v.CanInsertGridTrack(c.axis) || !v.InsertGridTrackBeforeSelection(c.axis) {
				t.Fatal("safe interior insertion rejected")
			}
			if s.UndoStack().Count() != 1 || !v.Selection().Contains(widgets[1]) || !reflect.DeepEqual(order, parent.Children()) {
				t.Fatal("insertion split history, selection or reading order")
			}
			check := func(expected []layoutPlacement) {
				t.Helper()
				for i, w := range widgets {
					if w.layoutCell != expected[i] {
						t.Fatalf("%s placement=%+v, want %+v", w.WidgetName(), w.layoutCell, expected[i])
					}
				}
			}
			check(c.after)
			v.Undo()
			check(c.before)
			for i, w := range widgets {
				if w.Bounds1() != before[i] {
					t.Fatalf("Undo changed bounds for %s: %v != %v", w.WidgetName(), w.Bounds1(), before[i])
				}
			}
			v.Redo()
			check(c.after)

			// Save and reopen while the inserted track is still completely
			// empty: no dummy widget is needed to preserve the gap.
			reopened := NewGedScene()
			if err := reopened.LoadDesign(s.SaveDesign()); err != nil {
				t.Fatal(err)
			}
			grid := reopened.Children()[0].(*FakeWidget)
			centerOfTabularCell(t, grid, 1, 1)

			// Drop into the new track through the actual palette pipeline.
			x, y := centerOfTabularCell(t, parent, 1, 1)
			ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
			px, py := v.MapFromScene(x, y)
			v.OnDragMove(px, py, ctx)
			if !v.paletteDrop.valid || v.paletteDrop.cell.cell != cell(1, 1, 1, 1) {
				t.Fatalf("no vacant track feedback %+v", v.paletteDrop)
			}
			v.OnDrop(px, py, ctx)
			if ctx.Action() != gui.DndCopy || len(parent.Children()) != 4 || s.UndoStack().Count() != 2 {
				t.Fatal("drop did not enter the newly inserted track")
			}
			check(c.after)
			added := parent.Children()[3].(*FakeWidget)
			added.SetWidgetName("InsertedButton")
			saved := s.SaveDesign()
			if err := reopened.LoadDesign(saved); err != nil {
				t.Fatal(err)
			}
			for _, node := range []*decl.Node{buildSceneDeclNode(reopened), mustDeclFromDesign(t, saved)} {
				_, indexed, err := node.BuildWithIndex()
				if err != nil {
					t.Fatal(err)
				}
				item := indexed["InsertedButton"].(gui.IWidget)
				tracks := item.Parent().(*gui.GridLayout).CellTracks()
				r, ok := trackByIndex(tracks.Rows, 1)
				k, colOK := trackByIndex(tracks.Columns, 1)
				if !ok || !colOK || item.Bounds1() != (geom.Rect{X: k.Start, Y: r.Start, Width: k.Size, Height: r.Size}) {
					t.Fatalf("preview/SDK gap shifted: %v, row=%v column=%v", item.Bounds1(), r, k)
				}
			}
		})
	}
}

func TestGridTrackInsertionRejectsUnsafeReflow(t *testing.T) {
	tests := []struct {
		name   string
		axis   GridTrackAxis
		first  layoutPlacement
		second layoutPlacement
		setup  func(*FakeWidget, *FakeWidget)
		reason string
	}{
		{"crossing span", GridRow, cell(0, 0, 2, 1), cell(1, 1, 1, 1), nil, gridInsertSpan},
		{"crossing column", GridColumn, cell(0, 0, 1, 2), cell(1, 1, 1, 1), nil, gridInsertSpan},
		{"track limit", GridColumn, cell(0, 0, 1, 1), cell(1, 4095, 1, 1), nil, gridInsertLimit},
		{"position lock", GridRow, cell(0, 0, 1, 1), cell(1, 1, 1, 1), func(p, w *FakeWidget) { p.SetLockPos(true) }, dropLayoutLocked},
		{"size lock", GridRow, cell(0, 0, 1, 1), cell(1, 1, 1, 1), func(p, w *FakeWidget) { w.SetLockSize(true) }, dropCellLockedGeometry},
		{"clipping", GridRow, cell(0, 0, 1, 1), cell(1, 1, 1, 1), func(p, w *FakeWidget) { p.SetPos(200, 157); p.Layout() }, gridInsertNoSpace},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			v, p, w := tabularFixture(t, LayoutGrid, c.first, c.second)
			if c.setup != nil {
				c.setup(p, w[0])
			}
			v.Selection().Add(w[1])
			r0, r1 := w[0].Bounds1(), w[1].Bounds1()
			plan, why := v.gridTrackInsertion(c.axis)
			if why == "" {
				why = v.gridInsertionReflowFailure(plan)
			}
			if why != c.reason {
				t.Fatalf("reason %q, want %q", why, c.reason)
			}
			if v.InsertGridTrackBeforeSelection(c.axis) || v.Scene().UndoStack().Count() != 0 ||
				w[0].layoutCell != c.first || w[1].layoutCell != c.second || w[0].Bounds1() != r0 || w[1].Bounds1() != r1 {
				t.Fatal("rejected grid change altered placement, geometry or history")
			}
		})
	}
}

type gridTrackPendingEdit struct{ change func() }

func (p *gridTrackPendingEdit) HasPendingChanges() bool { return p.change != nil }
func (p *gridTrackPendingEdit) CommitPendingChanges() error {
	run := p.change
	p.change = nil
	run()
	return nil
}

func TestGridTrackInsertionDoesNotFollowReplacementSelection(t *testing.T) {
	first, second := cell(0, 0, 1, 1), cell(1, 1, 1, 1)
	v, _, w := tabularFixture(t, LayoutGrid, first, second)
	v.Selection().Add(w[1])
	v.GedScene().AddPendingEditor(&gridTrackPendingEdit{change: func() {
		v.Selection().Clear()
		v.Selection().Add(w[0])
	}})
	if v.InsertGridTrackBeforeSelection(GridRow) || v.Scene().UndoStack().Count() != 0 ||
		w[0].layoutCell != first || w[1].layoutCell != second {
		t.Fatal("a reentrant draft redirected the insertion")
	}
	if v.Selection().ItemList()[0] != w[0] {
		t.Fatal("test did not replace selection")
	}
}

func TestGridTrackContextMenuKeepsAnchor(t *testing.T) {
	first, second := cell(0, 0, 1, 1), cell(1, 1, 1, 1)
	v, p, w := tabularFixture(t, LayoutGrid, first, second)
	at := func(row, col int) *gui.Menu {
		x, y := centerOfTabularCell(t, p, row, col)
		return v.buildContextMenu(v.MapFromScene(x, y))
	}
	menu := at(1, 1)
	target := contextButton(menu, i18n.T(gridInsertRowTitle))
	if target == nil || !target.IsEnabled() {
		t.Fatal("layout context menu did not expose grid insertion")
	}
	v.Selection().Clear()
	v.Selection().Add(w[0])
	target.Action().Trigger(target)
	if v.Scene().UndoStack().Count() != 0 {
		t.Fatal("stale context menu acted on a different grid widget")
	}
	fresh := contextButton(at(1, 1), i18n.T(gridInsertRowTitle))
	if fresh == nil {
		t.Fatal("missing fresh grid action")
	}
	fresh.Action().Trigger(fresh)
	if w[1].layoutCell.Row != 2 || v.Scene().UndoStack().Count() != 1 {
		t.Fatal("real menu action did not insert a single row")
	}
}

func TestGridTrackInsertionGeneratedGoKeepsEmptyTracks(t *testing.T) {
	if testing.Short() {
		t.Skip("generated Go integration")
	}
	v, _, w := tabularFixture(t, LayoutGrid, cell(0, 0, 1, 1), cell(1, 1, 1, 1))
	w[0].SetWidgetName("Start")
	w[1].SetWidgetName("End")
	v.Selection().Add(w[1])
	if !v.InsertGridTrackBeforeSelection(GridRow) || !v.InsertGridTrackBeforeSelection(GridColumn) {
		t.Fatal("grid refused two independent insertions")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "layout.go")
	tests := filepath.Join(dir, "layout_test.go")
	code := v.GedScene().GenerateCode(CodeGenOptions{PackageName: "ui", TypeName: "InsertedGridUI"})
	if err := os.WriteFile(src, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	const runtime = `package ui
import ("testing"; "github.com/uk0/silk/gui")
func TestTracks(t *testing.T) {
 u:=NewInsertedGridUI()
 g:=u.End.Parent().(*gui.GridLayout)
 check:=func() {
   k:=g.CellTracks()
   if len(k.Rows)!=3 || len(k.Columns)!=3 || k.Rows[1].Size<=0 || k.Columns[1].Size<=0 ||
      u.End.X()!=k.Columns[2].Start || u.End.Y()!=k.Rows[2].Start || len(g.Children())!=2 {
     t.Fatal("generated grid lost the unoccupied track", k, u.End.Bounds1())
   }
 }
 check()
 g.SetSize(g.Width()*1.6, g.Height()*1.6)
 check()
}`
	if err := os.WriteFile(tests, []byte(runtime), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", src, tests)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated program failed: %v\n%s", err, out)
	}
}
