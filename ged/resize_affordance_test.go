package ged

import (
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

type resizeAffordancePainter struct {
	paint.Painter
	handles int
}

func (p *resizeAffordancePainter) DrawIcon1(icon paint.Icon, x, y, size float64, disabled bool) {
	p.handles++
	p.Painter.DrawIcon1(icon, x, y, size, disabled)
}

func paintedResizeHandles(v *GedView) int {
	p := &resizeAffordancePainter{Painter: paint.NewPixmap(480, 320).NewPainter()}
	v.Selection().OnDraw(p)
	return p.handles
}

func lowerRightGrip(w graph.IItem) (float64, float64) {
	r := w.MapRectToScene1(w.Bounds1())
	return r.Right() - .25, r.Bottom() - .25
}

func TestResizeAffordanceTracksLiveOwnershipAndLocks(t *testing.T) {
	v, box, child := managedGeometryFixture(t)
	child.SetLockSize(true) // initially locked before selection
	v.Selection().Add(child)
	point := func() graph.IDecor {
		x, y := lowerRightGrip(child)
		d, _ := v.FindHandleAt(x, y)
		return d
	}
	if point() != nil || paintedResizeHandles(v) != 0 {
		t.Fatal("managed/locked child exposes a false resize target")
	}
	child.SetLockSize(false)
	if point() != nil || paintedResizeHandles(v) != 0 || !v.Selection().Contains(child) {
		t.Fatal("unlock revived managed handles or deselected the widget")
	}
	box.layoutManaged = false
	original := point()
	if original == nil || paintedResizeHandles(v) != 8 {
		t.Fatal("free child did not regain handles without re-selection")
	}
	child.SetLockPos(true)
	if point() != nil || paintedResizeHandles(v) != 0 {
		t.Fatal("live position lock kept active geometry targets")
	}
	child.SetLockPos(false)
	if point() != original || paintedResizeHandles(v) != 8 {
		t.Fatal("lock changed decor identity or lost free handles")
	}
}

func TestResizeAffordanceMixedSelectionCannotResizeManagedSibling(t *testing.T) {
	v, _, managed := managedGeometryFixture(t)
	free := sceneWidget(t, v.GedScene(), "gui.Button", "free", 20, 85, 25, 10)
	v.Selection().AddMulti([]graph.IItem{managed, free})
	x, y := lowerRightGrip(free)
	if decor, _ := v.FindHandleAt(x, y); decor != nil || paintedResizeHandles(v) != 0 {
		t.Fatal("mixed selection advertised an unsupported resize")
	}
	m0, f0 := managed.Bounds1(), free.Bounds1()
	dragOnCanvas(v, x, y, x+12, y+8)
	if v.Scene().UndoStack().CanUndo() || managed.Bounds1() != m0 || free.Bounds1() != f0 {
		t.Fatal("free widget handle carried a layout-owned sibling into a resize")
	}
}

func TestResizeAffordanceRetiresCapturedGestureWhenModelChanges(t *testing.T) {
	for _, name := range []string{"new-selection", "lock", "layout-now-owns", "peer-moved"} {
		t.Run(name, func(t *testing.T) {
			v, box, child := managedGeometryFixture(t)
			box.layoutManaged = false
			peer := sceneWidget(t, v.GedScene(), "gui.Button", "peer", 80, 28, 25, 15)
			v.Selection().Add(child)
			if name == "peer-moved" {
				v.Selection().Add(peer)
			}
			x, y := lowerRightGrip(child)
			d, _ := v.FindHandleAt(x, y)
			if d == nil {
				t.Fatal("positive control has no resize handle")
			}
			clearClickHistory()
			v.OnLeftDown(v.MapFromScene(x, y))
			px, py := v.MapFromScene(x+14, y+11)
			v.OnMouseMove(px, py)
			if !d.NakedDecor().IsMovingHandle() {
				t.Fatal("test never captured a real resize gesture")
			}
			switch name {
			case "new-selection":
				v.Selection().Add(peer)
			case "lock":
				child.SetLockSize(true)
			case "layout-now-owns":
				box.layoutManaged = true
			case "peer-moved":
				peer.SetPos(86, 32)
			}
			before := []geom.Rect{child.Bounds1(), peer.Bounds1()}
			v.OnLeftUp(px, py)
			if d.NakedDecor().IsMovingHandle() || v.Scene().UndoStack().CanUndo() ||
				child.Bounds1() != before[0] || peer.Bounds1() != before[1] {
				t.Fatal("release committed a resize after its targets changed")
			}
		})
	}
}

func TestResizeAffordanceSelectedOwnerRemainsEditable(t *testing.T) {
	v, box, child := managedGeometryFixture(t)
	box.Layout()
	v.Selection().AddMulti([]graph.IItem{box, child})
	// Child fills the owner; exactly eight painted handles necessarily
	// belong to the owner and share its scene hit even at a shared corner.
	x, y := lowerRightGrip(box)
	d, _ := v.FindHandleAt(x, y)
	if d == nil || d.Item() != box || paintedResizeHandles(v) != 8 {
		t.Fatal("child swallowed the owner's eligible resize handle")
	}
	w, h := box.Width(), box.Height()
	dragOnCanvas(v, x, y, x+11, y+9)
	if box.Width() <= w || box.Height() <= h || !v.Scene().UndoStack().CanUndo() {
		t.Fatal("selected layout owner lost the resize operation")
	}
	r := box.Generate().Children()[0].Bounds1()
	if child.Width() != gui.PixelToMm(r.Width) || child.Height() != gui.PixelToMm(r.Height) {
		t.Fatal("selected child did not follow the normal registered layout")
	}
}

func TestResizeAffordanceDoesNotSnapUntouchedSizeLockedPeer(t *testing.T) {
	v, s := newResizeSnapView(t, 5, true)
	free := addFakeAt(t, s, "free", snapBtnX, snapBtnY, snapBtnW, snapBtnH)
	peer := addFakeAt(t, s, "peer", 61.75, 16.5, 12.3, 6.7)
	peer.SetLockSize(true)
	before := peer.Bounds1()
	v.Selection().AddMulti([]graph.IItem{free, peer})
	dragBottomRightHandle(t, v, snapPressX, snapPressY, snapEndX, snapEndY)
	if free.Width() != 35 || !v.Scene().UndoStack().CanUndo() || peer.Bounds1() != before {
		t.Fatalf("actual resize did not preserve untouched peer geometry: %v -> %v", before, peer.Bounds1())
	}
}

func TestResizeAffordanceManagedCornerBehavesLikeBodyReorder(t *testing.T) {
	v, parent, items := reorderLayoutFixture(t, LayoutVertical)
	v.Selection().Add(items[0])
	before := parent.Children()
	sx, sy := lowerRightGrip(items[0])
	if d, _ := v.FindHandleAt(sx, sy); d != nil {
		t.Fatal("managed corner unexpectedly exposes a resize handle")
	}
	x, y := v.MapFromScene(sx, sy)
	gx, gy := layoutBodyPoint(v, items[2], LayoutVertical, .8)
	v.OnLeftDown(x, y)
	v.OnMouseMove(gx, gy)
	if !v.managedHint.insertion.valid {
		t.Fatal("a former handle still blocked the visible reorder target")
	}
	v.OnLeftUp(gx, gy)
	got := parent.Children()
	if got[len(got)-1] != items[0] || v.Scene().UndoStack().Count() != 1 {
		t.Fatal("managed corner did not take the ordinary one-step reorder path")
	}
	v.Undo()
	restored := parent.Children()
	for i := range before {
		if restored[i] != before[i] {
			t.Fatal("undo did not restore original order")
		}
	}
}

func TestResizeAffordanceDoesNotSnapSelectionReplacedByCommitCallback(t *testing.T) {
	v, s := newResizeSnapView(t, 5, true)
	free := addFakeAt(t, s, "free", snapBtnX, snapBtnY, snapBtnW, snapBtnH)
	peer := addFakeAt(t, s, "peer", 61.75, 16.5, 12.3, 6.7)
	before := peer.Bounds1()
	v.Selection().Add(free)
	s.SigDesignChanged(func() {
		v.Selection().Clear()
		v.Selection().Add(peer)
	})
	dragBottomRightHandle(t, v, snapPressX, snapPressY, snapEndX, snapEndY)
	if !v.Scene().UndoStack().CanUndo() || !v.Selection().Contains(peer) || peer.Bounds1() != before {
		t.Fatal("post-command snap mutated the selection installed by the observer")
	}
}
