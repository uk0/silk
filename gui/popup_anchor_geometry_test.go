package gui

import "testing"

// This popup retains the last placed screen coordinates independently of the
// widget tree, like an actual Win32/GLFW surface. Logical parent changes must
// not make it appear to move before a fresh placement call.
type anchoredGeometryProbe struct {
	Widget
	screenX, screenY float64
	placements       int
	surface          *Window
}

func (p *anchoredGeometryProbe) SetPos(x, y float64) {
	p.placements++
	p.screenX, p.screenY = x, y
	if parent := p.Parent(); parent != nil {
		p.screenX, p.screenY = parent.MapToGlobal(x, y)
	}
	p.Widget.SetPos(x, y)
	if p.surface != nil {
		// Mirror the separately reported mapped position of a native popup.
		p.surface.queuePopupSurfacePlacement()
	}
}
func (p *anchoredGeometryProbe) Window() *Window { return p.surface }

func (p *anchoredGeometryProbe) MapToGlobal(x, y float64) (float64, float64) {
	return p.screenX + x, p.screenY + y
}

func isolateAnchoredGeometry(t *testing.T) {
	t.Helper()
	oldCapture, oldPending := captureStack, pendingButtonPopups
	captureStack, pendingButtonPopups = nil, nil
	uiTaskMu.Lock()
	oldTasks, oldWake := uiTasks, uiWakeupFn
	uiTasks, uiWakeupFn = nil, nil
	uiTaskMu.Unlock()
	t.Cleanup(func() {
		captureStack, pendingButtonPopups = oldCapture, oldPending
		uiTaskMu.Lock()
		uiTasks, uiWakeupFn = oldTasks, oldWake
		uiTaskMu.Unlock()
	})
}

func geometryTestPopup(anchor interface {
	popupAnchorButton
	SetSubPopup(IWidget)
}) *anchoredGeometryProbe {
	p := &anchoredGeometryProbe{}
	p.Init(p)
	anchor.SetSubPopup(p)
	p.SetParent(anchor)
	p.SetSize(112, 35)
	p.wt = WtPopup
	p.setVisible(true)
	x, y := anchor.MapToGlobal(0, 0)
	w, h := anchor.Size()
	LayoutPopup(p, x, y, w, h, true, 1)
	captureStack = append(captureStack, p)
	return p
}

func TestAnchoredPopupFollowsPanelLayoutWithoutReopen(t *testing.T) {
	isolateAnchoredGeometry(t)
	form, host := NewForm(), NewFrame()
	host.SetParent(form)
	host.SetBounds(35, 30, 250, 120)
	anchor := NewButton()
	anchor.SetParent(host)
	anchor.SetBounds(15, 17, 110, 28)
	p := geometryTestPopup(anchor)
	x, y := p.MapToGlobal(0, 0)
	placements := p.placements
	host.SetPos(67, 43)
	host.SetPos(105, 96)
	drainUITasks()
	nx, ny := p.MapToGlobal(0, 0)
	if nx-x != 70 || ny-y != 66 || !p.IsVisible() || p.Parent() != anchor {
		t.Fatalf("popup remained at %v,%v after anchor moved from %v,%v by 70,66 (visible=%v)", nx, ny, x, y, p.IsVisible())
	}
	if p.placements != placements+1 {
		t.Fatalf("burst moved mapped popup %d times, want one coalesced placement", p.placements-placements)
	}
}

func TestAnchoredPopupIgnoresUnrelatedAndNetZeroLayout(t *testing.T) {
	isolateAnchoredGeometry(t)
	form, host, other := NewForm(), NewFrame(), NewFrame()
	host.SetParent(form)
	other.SetParent(form)
	host.SetBounds(35, 30, 250, 120)
	anchor := NewButton()
	anchor.SetParent(host)
	anchor.SetBounds(15, 17, 110, 28)
	p := geometryTestPopup(anchor)
	initial := p.placements

	other.SetBounds(75, 94, 120, 35)
	if len(uiTasks) != 0 {
		t.Fatal("unrelated layout queued a placement")
	}
	host.SetPos(84, 90)
	host.SetPos(35, 30)
	drainUITasks()
	if p.placements != initial {
		t.Fatal("net-zero placement performed a native move")
	}
}

func TestAnchoredPopupRecomputesAfterSameFormReparent(t *testing.T) {
	isolateAnchoredGeometry(t)
	form, left, right, moved := NewForm(), NewFrame(), NewFrame(), NewFrame()
	left.SetParent(form)
	right.SetParent(form)
	left.SetPos(50, 50)
	right.SetPos(130, 100)
	moved.SetParent(left)
	anchor := NewButton()
	anchor.SetParent(moved)
	anchor.SetBounds(10, 15, 100, 30)
	p := geometryTestPopup(anchor)
	x, y := p.MapToGlobal(0, 0)

	moved.SetParent(right)
	drainUITasks()
	newX, newY := p.MapToGlobal(0, 0)
	if !p.IsVisible() || newX-x != 80 || newY-y != 50 || p.Parent() != anchor {
		t.Fatalf("same-form reparent: position %v,%v -> %v,%v, visible = %t", x, y, newX, newY, p.IsVisible())
	}
}

func TestAnchoredPopupSkipsAClosedPendingPlacement(t *testing.T) {
	isolateAnchoredGeometry(t)
	form, host := NewForm(), NewFrame()
	host.SetParent(form)
	host.SetPos(50, 60)
	anchor := NewButton()
	anchor.SetParent(host)
	anchor.SetBounds(10, 15, 100, 30)
	p := geometryTestPopup(anchor)
	initial := p.placements
	host.SetPos(75, 90)
	p.setVisible(false)
	drainUITasks()
	if p.IsVisible() || p.placements != initial {
		t.Fatal("pending layout repositioned a closed menu")
	}
}

func TestIdleWidgetGeometryDoesNotEnqueueOrAllocateForPopups(t *testing.T) {
	isolateAnchoredGeometry(t)
	p := &Widget{}
	p.Init(p)
	var x float64
	n := testing.AllocsPerRun(100, func() {
		x++
		p.SetPos(x, x)
	})
	if n != 0 || len(uiTasks) != 0 {
		t.Fatalf("geometry without popup: %v allocations, %d UI tasks", n, len(uiTasks))
	}
}

type childSurfaceAnchor struct {
	Button
	host *anchoredGeometryProbe
}

func (w *childSurfaceAnchor) MapToGlobal(x, y float64) (float64, float64) {
	return w.host.MapToGlobal(w.X()+x, w.Y()+y)
}
func (w *childSurfaceAnchor) MapFromGlobal(x, y float64) (float64, float64) {
	ax, ay := w.MapToGlobal(0, 0)
	return x - ax, y - ay
}

func TestAnchoredPopupChainTracksTheMappedParentBeforeRelocatingChild(t *testing.T) {
	isolateAnchoredGeometry(t)
	form, host := NewForm(), NewFrame()
	host.SetParent(form)
	host.SetPos(65, 55)
	a := NewButton()
	a.SetParent(host)
	a.SetBounds(20, 12, 110, 28)
	parent := geometryTestPopup(a)
	parent.surface = &Window{widget: parent, wt: WtPopup}
	childAnchor := &childSurfaceAnchor{host: parent}
	childAnchor.Init(childAnchor)
	childAnchor.SetParent(parent)
	childAnchor.SetBounds(12, 9, 84, 20)
	child := geometryTestPopup(childAnchor)
	ax, ay := parent.MapToGlobal(0, 0)
	bx, by := child.MapToGlobal(0, 0)
	parentInitial, childInitial := parent.placements, child.placements

	host.SetPos(122, 88)
	drainUITasks()
	// The child is in a different native coordinate space. It observes the
	// parent popup's mapped position callback, not the layout frame.
	if u, v := child.MapToGlobal(0, 0); u != bx || v != by {
		t.Fatal("child reanchored before mapped parent position was available")
	}
	drainUITasks()
	u, v := parent.MapToGlobal(0, 0)
	x, y := child.MapToGlobal(0, 0)
	if u-ax != 57 || v-ay != 33 || x-bx != 57 || y-by != 33 || !parent.IsVisible() || !child.IsVisible() {
		t.Fatalf("parent/child move deltas: %.0f,%.0f and %.0f,%.0f", u-ax, v-ay, x-bx, y-by)
	}
	if parent.placements != parentInitial+1 || child.placements != childInitial+1 || len(uiTasks) != 0 {
		t.Fatal("popup-chain move was duplicated or left a task behind")
	}
}
