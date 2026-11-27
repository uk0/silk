package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

func TestLinearPaletteDropKeepsPreviewAndRuntimeOrder(t *testing.T) {
	for _, kind := range []LayoutKind{LayoutHorizontal, LayoutVertical} {
		t.Run(string(kind), func(t *testing.T) {
			view := NewGedView()
			scene := view.GedScene()
			scene.SetSize(180, 140)
			box := sceneWidget(t, scene, string(kind), "dropLayout", 20, 18, 130, 100)
			box.layoutManaged = true
			first := sceneWidget(t, scene, "gui.Button", "first", 23, 20, 26, 9)
			next := sceneWidget(t, scene, "gui.Button", "next", 55, 38, 29, 10)
			first.SetParent(box)
			next.SetParent(box)
			box.Layout()
			b := next.MapRectToScene1(next.Bounds1())
			f := first.MapRectToScene1(first.Bounds1())
			x, y := b.X+b.Width/2, b.Y+b.Height*.25
			if kind == LayoutHorizontal {
				x, y = b.X+b.Width*.25, b.Y+b.Height/2
			}
			ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
			view.Selection().Add(first)
			var recorded linearPaletteInsertion
			for _, zoom := range []float64{0.6, 1.0, 2.4} {
				view.SetZoomFactor(zoom)
				view.SetScrollX(15)
				view.SetScrollY(9)
				px, py := view.MapFromScene(x, y)
				view.OnDragMove(px, py, ctx)
				p := view.paletteDrop
				if !p.valid || p.parent != graph.IItem(box) || !p.insertion.valid || p.insertion.index != 1 || p.insertion.ordinal != 2 ||
					p.hint != i18n.Tf(dropInsertSlot, "Button", "dropLayout", 2) {
					t.Fatalf("wrong slot at zoom %g: %+v", zoom, p)
				}
				if recorded.valid && recorded != p.insertion {
					t.Fatal("zoom changed the slot")
				}
				recorded = p.insertion
			}
			if kind == LayoutHorizontal {
				if recorded.marker.Width != 0 || recorded.marker.Height <= 0 || recorded.marker.X < f.Right() || recorded.marker.X > b.X {
					t.Fatalf("vertical marker not in slot: %+v", recorded.marker)
				}
			} else if recorded.marker.Height != 0 || recorded.marker.Width <= 0 || recorded.marker.Y < f.Bottom() || recorded.marker.Y > b.Y {
				t.Fatalf("horizontal marker not in slot: %+v", recorded.marker)
			}
			// The canvas feedback must actually make ink, not just retain a slot.
			pm := paint.NewPixmap(64, 64)
			defer paint.CloseSurface(pm)
			g := pm.NewPainter()
			defer paint.ClosePainter(g)
			scale := gui.ScreenDpmm() * view.ZoomFactor()
			mx, my := recorded.marker.X+recorded.marker.Width/2, recorded.marker.Y+recorded.marker.Height/2
			g.Scale(scale, scale)
			g.Translate(32/scale-mx, 32/scale-my)
			view.drawPaletteDropBounds(g)
			pm.Flush()
			rgba, err := pm.Image()
			if err != nil {
				t.Fatal(err)
			}
			ix, iy := 32, 32
			mark := false
			for yy := iy - 3; yy <= iy+3; yy++ {
				for xx := ix - 3; xx <= ix+3; xx++ {
					_, _, _, a := rgba.At(xx, yy).RGBA()
					if a != 0 {
						mark = true
					}
				}
			}
			if !mark {
				t.Fatal("insertion line was never painted")
			}
			// A hover at the tail must not be allowed to change a middle-slot release.
			right := box.MapRectToScene1(box.Bounds1())
			px, py := view.MapFromScene(right.Right()-.5, right.Bottom()-.5)
			view.OnDragMove(px, py, ctx)
			px, py = view.MapFromScene(x, y)
			view.OnDrop(px, py, ctx)
			if ctx.Action() != gui.DndCopy || view.paletteDrop.active {
				t.Fatal("drop not claimed or stale indicator")
			}
			children := box.Children()
			if len(children) != 3 || children[0] != first || children[2] != next {
				t.Fatalf("not inserted between the chosen siblings: %v", children)
			}
			added := children[1].(*FakeWidget)
			added.SetWidgetName("inserted")
			if !reflect.DeepEqual(view.Selection().ItemList(), []graph.IItem{added}) {
				t.Fatal("new widget not selected")
			}
			view.Undo()
			if scene.UndoStack().CanUndo() || !reflect.DeepEqual(box.Children(), []graph.IItem{first, next}) || !reflect.DeepEqual(view.Selection().ItemList(), []graph.IItem{first}) {
				t.Fatal("not a single undo restoring order and selection")
			}
			view.Redo()
			if !reflect.DeepEqual(box.Children(), []graph.IItem{first, added, next}) || !reflect.DeepEqual(view.Selection().ItemList(), []graph.IItem{added}) {
				t.Fatal("redo used a different slot or selection")
			}
			doc := scene.SaveDesign()
			loaded := NewGedScene()
			if err := loaded.LoadDesign(doc); err != nil {
				t.Fatal(err)
			}
			got := loaded.Children()[0].(*FakeWidget).Children()
			if len(got) != 3 || got[0].(*FakeWidget).WidgetName() != "first" || got[1].(*FakeWidget).WidgetName() != "inserted" || got[2].(*FakeWidget).WidgetName() != "next" {
				t.Fatal("saved design lost sibling order")
			}
			for _, node := range []*decl.Node{buildSceneDeclNode(loaded), mustDeclFromDesign(t, doc)} {
				_, idx, err := node.BuildWithIndex()
				if err != nil {
					t.Fatal(err)
				}
				a, b, c := idx["first"].(gui.IWidget), idx["inserted"].(gui.IWidget), idx["next"].(gui.IWidget)
				if !reflect.DeepEqual(a.Parent().Children(), []gui.IWidget{a, b, c}) {
					t.Fatal("runtime layout order differs from drop")
				}
				if kind == LayoutHorizontal && (a.X()+a.Width() > b.X()+.01 || b.X()+b.Width() > c.X()+.01) {
					t.Fatal("horizontal layout overlaps")
				}
				if kind == LayoutVertical && (a.Y()+a.Height() > b.Y()+.01 || b.Y()+b.Height() > c.Y()+.01) {
					t.Fatal("vertical layout overlaps")
				}
			}
		})
	}
}

func TestLinearPaletteSlotEmptyAndClipped(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	box := sceneWidget(t, scene, string(LayoutHorizontal), "layout", 5, 10, 120, 40)
	slot := linearPaletteSlot(box, geom.Rect{X: 30, Y: 12, Width: 20, Height: 10}, 34, 16)
	if !slot.valid || slot.index != 0 || slot.ordinal != 1 || slot.marker.X <= 30 || slot.marker.X >= 50 || slot.marker.Y <= 12 || slot.marker.Bottom() >= 22 {
		t.Fatalf("empty clipped lane: %+v", slot)
	}
}
