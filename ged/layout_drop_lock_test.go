package ged

import (
	"testing"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

func TestPaletteGridAppendKeepsLockedGeometry(t *testing.T) {
	for _, variant := range []string{"direct-size", "nested-position", "unlocked"} {
		t.Run(variant, func(t *testing.T) {
			v, p, items := tabularFixture(t, LayoutGrid, cell(0, 0, 1, 1), cell(1, 0, 1, 1))
			s := v.GedScene()
			locked := items[1]
			column := 0
			switch variant {
			case "direct-size":
				locked.SetLockSize(true)
			case "nested-position":
				card := transferWidget(t, p, "gui.Card", "LockedChildCard", 15, 50, 80, 40)
				items[1].SetParent(nil)
				card.layoutCell = cell(1, 0, 1, 1)
				locked = transferWidget(t, card, "gui.Button", "LockedChild", 21, 56, 20, 8)
				locked.SetLockPos(true)
				peer := transferButton(t, p, "OtherColumn")
				peer.layoutCell = cell(1, 1, 1, 1)
				column = 1
				p.Layout()
			}
			originalChildren := len(p.Children())
			before := locked.MapRectToScene1(locked.Bounds1())
			bounds := p.MapRectToScene1(p.Bounds1())
			tracks := p.tabularTracks()
			last := tracks.Rows[len(tracks.Rows)-1]
			sx, sy := v.MapFromScene(bounds.X+gui.PixelToMm(tracks.Columns[column].Start+tracks.Columns[column].Size/2), bounds.Y+gui.PixelToMm(last.Start+last.Size-2))
			ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
			signalCount := 0
			s.SigDesignChanged(func() { signalCount++ })
			v.OnDragMove(sx, sy, ctx)
			if !v.paletteDrop.cell.appendRow {
				t.Fatalf("expected bottom-edge append: %+v", v.paletteDrop)
			}
			v.OnDrop(sx, sy, ctx)
			after := locked.MapRectToScene1(locked.Bounds1())
			if variant == "unlocked" {
				if ctx.Action() != gui.DndCopy || len(p.Children()) != originalChildren+1 || s.UndoStack().Count() != 1 || signalCount != 1 || after == before {
					t.Fatalf("expected ordinary reflow in one edit: action %v, rect %v -> %v, signals %d", ctx.Action(), before, after, signalCount)
				}
			} else if ctx.Action() != gui.DndIgnore || len(p.Children()) != originalChildren || s.UndoStack().CanUndo() || signalCount != 0 || after != before {
				t.Fatalf("append violated geometry lock: action %v, rect %v -> %v, signals %d", ctx.Action(), before, after, signalCount)
			}
		})
	}
}

func TestTabularMoveCannotResizeSizeLockedControl(t *testing.T) {
	v, p, items := tabularFixture(t, LayoutForm, cell(0, 0, 1, 1), cell(1, 1, 1, 1))
	s := v.GedScene()
	moved := items[0]
	moved.SetLockSize(true)
	before := moved.Bounds1()
	x, y := centerOfTabularCell(t, p, 0, 1)
	fromX, fromY := centerOfTabularCell(t, p, 0, 0)
	plan := v.tabularLayoutDrop(p, designDropArea(s.Bounds1(), p), x, y, moved)
	if !plan.valid || v.tabularReleaseFailure(p, moved, plan, false) != dropCellLockedGeometry {
		t.Fatalf("expected a geometrically incompatible field: %+v", plan)
	}
	dragOnCanvas(v, fromX, fromY, x, y)
	if moved.Parent() != graph.IItem(p) || moved.layoutCell != cell(0, 0, 1, 1) ||
		moved.Bounds1() != before || s.UndoStack().CanUndo() {
		t.Fatal("illegal resize committed")
	}
	moved.SetLockSize(false)
	dragOnCanvas(v, fromX, fromY, x, y)
	if moved.layoutCell != cell(0, 1, 1, 1) || moved.Width() <= before.Width || s.UndoStack().Count() != 1 {
		t.Fatal("same field should accept a normal move")
	}
}

func TestTabularDropAcceptsStableFractionalLockedDescendant(t *testing.T) {
	v, p, _ := tabularFixture(t, LayoutForm, cell(0, 1, 1, 1))
	card := transferWidget(t, p, "gui.Card", "FreeCard", 15, 15, 80, 30)
	card.layoutCell = cell(0, 0, 1, 1)
	leaf := transferWidget(t, card, "gui.Button", "LockedFractional", 15.413, 15.831, 3.149, 2.11)
	leaf.SetLockPos(true)
	leaf.SetLockSize(true)
	p.Widget().(*gui.FormLayout).SetRowHeight(40)
	p.Layout()
	before := leaf.MapRectToScene1(leaf.Bounds1())
	origin := p.MapRectToScene1(p.Bounds1())
	x, _ := centerOfTabularCell(t, p, 0, 1)
	sx, sy := v.MapFromScene(x, origin.Y+gui.PixelToMm(54))
	ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
	v.OnDragMove(sx, sy, ctx)
	if !v.paletteDrop.cell.valid || !v.paletteDrop.cell.appendRow {
		t.Fatalf("expected visible form append slot: %+v", v.paletteDrop)
	}
	v.OnDrop(sx, sy, ctx)
	if ctx.Action() != gui.DndCopy || leaf.MapRectToScene1(leaf.Bounds1()) != before {
		t.Fatalf("rounding in detached projection blocked a safe drop or moved the locked child: action=%v", ctx.Action())
	}
}
