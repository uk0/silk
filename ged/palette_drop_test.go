package ged

import (
	"math"
	"reflect"
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

func TestPaletteDropRejectsOutsidePageWithoutHistory(t *testing.T) {
	for _, point := range [][2]float64{{-1, 30}, {30, -1}, {100, 30}, {30, 100}, {math.NaN(), 30}, {30, math.Inf(1)}} {
		view := NewGedView()
		ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
		x, y := view.MapFromScene(point[0], point[1])
		view.OnDragEnter(x, y, ctx)
		if ctx.Action() != gui.DndIgnore || view.paletteDrop.valid {
			t.Fatalf("outside point %v offered a valid drop", point)
		}
		view.OnDrop(x, y, ctx)
		if ctx.Action() != gui.DndIgnore || len(view.Scene().Children()) != 0 || view.Scene().UndoStack().CanUndo() {
			t.Fatalf("outside point %v changed design/history", point)
		}
		if view.paletteDrop.active || view.dropContainer != nil {
			t.Fatal("rejected release retained drag feedback")
		}
	}
}

func TestPaletteDropRejectsUnsupportedTextAndActions(t *testing.T) {
	for _, ctx := range []*dropCtx{
		{pa: gui.DndCopy, payload: "arbitrary text"},
		{pa: gui.DndCopy, payload: "ged.GedView"},
		{pa: gui.DndCopy, payload: "gui.ScrollArea"},
		{pa: gui.DndMove, payload: "gui.Button"},
		{pa: gui.DndCopy, payload: 42},
		{pa: gui.DndCopy, payload: "gui.Button", noFormat: true},
	} {
		view, scene, _, _, _ := newDropFixture(t)
		before := collectItems(scene, nil)
		x, y := view.MapFromScene(100, 40)
		ctx.SetAction(gui.DndCopy)
		view.OnDragMove(x, y, ctx)
		if ctx.Action() != gui.DndIgnore || view.dropContainer != nil || view.paletteDrop.valid || view.paletteDrop.hint == "" {
			t.Fatalf("invalid payload/action not visibly rejected: %#v", ctx)
		}
		view.OnDrop(x, y, ctx)
		if len(itemsAddedTo(scene, before)) != 0 || scene.UndoStack().CanUndo() {
			t.Fatalf("invalid payload/action added an item: %#v", ctx)
		}
	}
}

func TestPaletteDropPreviewMatchesFittedEdgePlacement(t *testing.T) {
	for _, zoom := range []float64{0.5, 1, 2.5} {
		view, scene, box := newEdgeDropFixture(t)
		view.SetZoomFactor(zoom)
		view.SetScrollX(9)
		view.SetScrollY(11)
		ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
		for _, point := range [][2]float64{{61.5, 40}, {140.5, 69.5}} {
			x, y := view.MapFromScene(point[0], point[1])
			view.OnDragMove(x, y, ctx)
			plan := view.paletteDrop
			if !plan.valid || plan.container != graph.IItem(box) {
				t.Fatalf("zoom %g: expected the box to accept at %v: %+v", zoom, point, plan)
			}
			if got := plan.bounds.IntersectCopy(box.Bounds1()); got != plan.bounds {
				t.Fatalf("preview is clipped: %v in %v", plan.bounds, box.Bounds1())
			}
			before := collectItems(scene, nil)
			view.OnDrop(x, y, ctx)
			added := itemsAddedTo(scene, before)
			if len(added) != 1 || added[0].MapRectToScene1(added[0].Bounds1()) != plan.bounds {
				t.Fatalf("zoom %g: released widget differs from preview %v", zoom, plan.bounds)
			}
			if view.paletteDrop.active || view.dropContainer != nil {
				t.Fatal("successful release retained preview")
			}
			view.Undo()
		}
	}
}

func TestPaletteDropFitsTheFormRightBottomEdge(t *testing.T) {
	view := NewGedView()
	button := placeFromPalette(t, view, "gui.Button", 99.5, 99.5)
	if got, want := button.Bounds1(), (geom.Rect{X: 75, Y: 93, Width: 25, Height: 7}); got != want {
		t.Fatalf("drop near form edge = %v, want full visible bounds %v", got, want)
	}
}

func TestPaletteDropRejectsContainerTooSmallWithoutChangingIt(t *testing.T) {
	view := NewGedView()
	box := sceneWidget(t, view.GedScene(), "gui.VBox", "tiny", 20, 20, 10, 5)
	ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
	x, y := view.MapFromScene(22, 22)
	view.OnDragMove(x, y, ctx)
	if ctx.Action() != gui.DndIgnore || view.paletteDrop.hint != i18n.T(dropContainerTooSmall) {
		t.Fatal("undersized target did not show an explicit refusal")
	}
	view.OnDrop(x, y, ctx)
	if box.HasChildren() || view.Scene().UndoStack().CanUndo() {
		t.Fatal("undersized target accepted a clipped widget")
	}
}

func TestPaletteDropRespectsAncestorClipAndLocalCoordinates(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	outer := sceneWidget(t, scene, "gui.VBox", "outer", 10, 10, 70, 70)
	outer.SetLocalCoord(true)
	inner := sceneWidget(t, scene, "gui.VBox", "inner", 20, 20, 80, 60)
	inner.SetParent(outer)
	button := placeFromPalette(t, view, "gui.Button", 78, 78)
	if button.Parent() != graph.IItem(inner) {
		t.Fatal("nested local-coordinate container was not the drop target")
	}
	actual := button.MapRectToScene1(button.Bounds1())
	if actual != (geom.Rect{X: 55, Y: 73, Width: 25, Height: 7}) {
		t.Fatalf("drop ignored inherited clipping or local coordinates: %v", actual)
	}
}

func TestPaletteDropSelectionFollowsOneStepUndoRedo(t *testing.T) {
	view, scene, box, nested, free := newDropFixture(t)
	view.Selection().AddMulti([]graph.IItem{box, free})
	previous := view.Selection().ItemList()
	var reported []graph.IItem
	view.AddSelectionCallback(func(items []graph.IItem) { reported = items })
	button := placeFromPalette(t, view, "gui.Button", 100, 40)
	view.OnIdle() // GraphView batches selection notifications until the UI heartbeat.
	if got := view.Selection().ItemList(); !reflect.DeepEqual(got, []graph.IItem{button}) || !reflect.DeepEqual(reported, got) {
		t.Fatalf("newly dropped item not selected/reported: %v, callbacks %v", got, reported)
	}
	if scene.UndoStack().Count() != 1 {
		t.Fatal("one palette drop is not one command")
	}
	view.Undo()
	if button.Parent() != nil || !reflect.DeepEqual(view.Selection().ItemList(), previous) {
		t.Fatal("undo did not restore the previous selection")
	}
	if nested.Parent() != graph.IItem(box) || free.Parent() != graph.IItem(scene) {
		t.Fatal("undo changed existing items")
	}
	view.Redo()
	if button.Parent() != graph.IItem(box) || !reflect.DeepEqual(view.Selection().ItemList(), []graph.IItem{button}) {
		t.Fatal("redo did not reselect the newly dropped item")
	}
}

func TestPaletteDropFeedbackPaintsAndClears(t *testing.T) {
	view := NewGedView()
	view.SetSize(600, 500)
	ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
	x, y := view.MapFromScene(50, 50)
	view.OnDragMove(x, y, ctx)
	pm := paint.NewPixmap(600, 500)
	view.Draw(pm.NewPainter())
	if !view.paletteDrop.active || !view.paletteDrop.valid {
		t.Fatal("valid hover has no preview")
	}
	view.OnDragLeave()
	if view.paletteDrop.active || view.dropContainer != nil {
		t.Fatal("leave retained preview")
	}
	ctx.payload = "unsupported"
	view.OnDragMove(x, y, ctx)
	view.Draw(pm.NewPainter())
	if !view.paletteDrop.active || view.paletteDrop.valid {
		t.Fatal("invalid hover has no rejection feedback")
	}
}

func TestPaletteDropUsesAvailableSparseLayoutCells(t *testing.T) {
	for _, kind := range []LayoutKind{LayoutGrid, LayoutForm} {
		view := NewGedView()
		scene := view.GedScene()
		box := sceneWidget(t, scene, string(kind), "layout", 10, 10, 70, 70)
		box.layoutManaged = true
		one := sceneWidget(t, scene, "gui.Button", "one", 15, 15, 25, 7)
		one.SetParent(box)
		one.layoutCell = layoutPlacement{Row: 0, Column: 0, RowSpan: 1, ColumnSpan: 1, Valid: true}
		two := sceneWidget(t, scene, "gui.Button", "two", 15, 40, 25, 7)
		two.SetParent(box)
		two.layoutCell = layoutPlacement{Row: 1, Column: 0, RowSpan: 1, ColumnSpan: 1, Valid: true}
		box.Layout()
		want := layoutPlacement{Row: 2, Column: 0, RowSpan: 1, ColumnSpan: 1, Valid: true}
		sx, sy := 40.0, 79.5 // explicit bottom insertion band
		if kind == LayoutForm {
			want.Row, want.Column = 1, 1
			sx, sy = centerOfTabularCell(t, box, want.Row, want.Column)
		}
		ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
		x, y := view.MapFromScene(sx, sy)
		view.OnDragMove(x, y, ctx)
		if p := view.paletteDrop; !p.valid || !p.autoLayout || p.cell.cell != want || p.cell.span.IsEmpty() {
			t.Fatalf("%s did not advertise the targeted cell: %+v", kind, p)
		}
		button := placeFromPalette(t, view, "gui.Button", sx, sy)
		if button.layoutCell != want || button.Parent() != graph.IItem(box) {
			t.Fatalf("%s drop collided with existing cell metadata", kind)
		}
		view.Undo()
		if len(box.Children()) != 2 {
			t.Fatal("undo did not remove the appended layout cell")
		}
		view.Redo()
		if len(box.Children()) != 3 || button.layoutCell != want {
			t.Fatal("redo did not restore the appended layout cell")
		}
	}
}
