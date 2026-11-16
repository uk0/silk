package gui

import (
	"math"
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

// Use ordinary SDK setters and the real posted-task pump, not a manual Layout
// after the edit: that extra Layout is exactly the workaround users should not
// need when a nested control's preferred size changes.
func TestLayoutInvalidationNestedLabelText(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root, inner := NewVBox(), NewVBox()
	root.SetSize(220, 600)
	label := NewLabel("Short")
	label.SetWrap(true)
	inner.AddWidget(label)
	root.AddWidget(inner)
	below := NewLabel("Below the description")
	root.AddWidget(below)
	root.Layout()
	before := below.Y()

	text := strings.Repeat("中文目录 / long description changes at runtime. ", 5)
	label.SetText(text)
	for i := 0; i < 4; i++ {
		drainUITasks()
	}
	need := MeasureForWidth(inner, inner.Width()).Height
	if inner.Height()+1e-7 < need || below.Y() <= before {
		t.Fatalf("nested text left stale allocation: inner=%.2f need=%.2f below=%.2f previous=%.2f", inner.Height(), need, below.Y(), before)
	}
	p := newLabelPaintRecorder()
	label.Draw(p)
	if strings.Join(p.texts, "") != text {
		t.Fatal("runtime text was retained but visual rows remain clipped")
	}
}

func drainLayoutTest(t *testing.T) {
	t.Helper()
	for i := 0; i < 16; i++ {
		uiTaskMu.Lock()
		n := len(uiTasks)
		uiTaskMu.Unlock()
		if n == 0 {
			return
		}
		drainUITasks()
	}
	t.Fatal("layout did not become quiescent in bounded UI turns")
}

type invalidationVBox struct {
	VBox
	calls, depth, maxDepth int
	onLayout               func()
}

func newInvalidationVBox() *invalidationVBox {
	p := new(invalidationVBox)
	p.Init(p)
	return p
}

func (p *invalidationVBox) Layout() {
	p.calls++
	p.depth++
	p.maxDepth = max(p.maxDepth, p.depth)
	defer func() { p.depth-- }()
	if p.onLayout != nil {
		p.onLayout()
	}
	p.VBox.Layout()
}

func TestLayoutInvalidationCoalescesAndStaysIdle(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root, inner := newInvalidationVBox(), newInvalidationVBox()
	root.SetSize(240, 500)
	label := NewLabel("start")
	inner.AddWidget(label)
	root.AddWidget(inner)
	drainLayoutTest(t)
	root.calls, inner.calls = 0, 0
	for i := 0; i < 1000; i++ {
		label.SetText(strings.Repeat("x", 1+i%20))
	}
	uiTaskMu.Lock()
	pending := len(uiTasks)
	uiTaskMu.Unlock()
	if pending != 1 || root.pendingLayout == nil || len(root.pendingLayout.entries) != 2 || root.calls != 0 || inner.calls != 0 {
		t.Fatalf("burst was not coalesced: tasks=%d rootCalls=%d innerCalls=%d batch=%+v", pending, root.calls, inner.calls, root.pendingLayout)
	}
	drainLayoutTest(t)
	if root.calls != 1 || inner.calls != 1 || root.pendingLayout != nil || root.queuedLayout != nil || inner.queuedLayout != nil {
		t.Fatalf("expected one pass per unchanged-size layout: root=%d inner=%d", root.calls, inner.calls)
	}
	for i := 0; i < 100; i++ {
		label.SetText(label.Text())
		drainUITasks()
	}
	if root.calls != 1 || inner.calls != 1 {
		t.Fatal("unchanged values or idle pumps keep laying out the tree")
	}
}

func TestLayoutInvalidationNestedPumpDoesNotReenter(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root := newInvalidationVBox()
	root.SetSize(240, 500)
	label := NewLabel("first")
	root.AddWidget(label)
	drainLayoutTest(t)
	root.calls, root.maxDepth = 0, 0
	root.onLayout = func() {
		root.onLayout = nil
		label.SetText("newer value inside layout callback")
		drainUITasks() // models a nested/modal event pump
	}
	label.SetText("pending value")
	drainLayoutTest(t)
	if root.maxDepth != 1 || root.calls != 2 || root.activeLayout != nil || label.Text() != "newer value inside layout callback" {
		t.Fatalf("reentry or newer request loss: depth=%d calls=%d", root.maxDepth, root.calls)
	}
}

func TestLayoutInvalidationReparentRevokesOldEntries(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	oldRoot, newRoot, inner := newInvalidationVBox(), newInvalidationVBox(), NewVBox()
	oldRoot.SetSize(200, 500)
	newRoot.SetSize(300, 500)
	label := NewLabel("short")
	label.SetWrap(true)
	inner.AddWidget(label)
	oldRoot.AddWidget(inner)
	drainLayoutTest(t)
	label.SetText(strings.Repeat("pending 中文 value ", 12))
	oldBatch := oldRoot.pendingLayout
	inner.SetParent(newRoot)
	for _, entry := range oldBatch.entries {
		if entry.widget == inner.NakedWidget() {
			t.Fatal("old queue still retains the moved layout")
		}
	}
	drainLayoutTest(t)
	if inner.Parent() != newRoot || inner.Width() != 300 || label.Height()+1e-7 < MeasureForWidth(label, label.Width()).Height {
		t.Fatal("new owner did not install the current wrapped geometry", inner.Bounds1(), label.Bounds1())
	}
	if oldBatch.root != nil || oldBatch.entries != nil {
		t.Fatal("delivered request retained its owner or descendants")
	}
}

func TestLayoutInvalidationHiddenRootResumesWithoutPolling(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root := newInvalidationVBox()
	root.SetSize(200, 500)
	label := NewLabel("short")
	label.SetWrap(true)
	root.AddWidget(label)
	drainLayoutTest(t)
	label.SetText("queued before hide")
	retired := root.pendingLayout
	root.Hide()
	if retired.root != nil || retired.entries != nil {
		t.Fatal("hidden root remains captured by an already-posted callback")
	}
	drainLayoutTest(t)
	root.calls = 0
	label.SetText(strings.Repeat("hidden changes 中文 ", 12))
	drainLayoutTest(t)
	if root.calls != 0 || root.pendingLayout != nil {
		t.Fatal("hidden changes created active layout work")
	}
	root.Show()
	drainLayoutTest(t)
	if root.calls != 1 || label.Height()+1e-7 < MeasureForWidth(label, label.Width()).Height {
		t.Fatal("show lost a deferred hidden change", root.calls, label.Bounds1())
	}
}

func TestLayoutInvalidationWrapFontVisibilityAndLegacySync(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root, inner := NewVBox(), newInvalidationVBox()
	root.SetSize(220, 600)
	label := NewLabel(strings.Repeat("runtime 中文 ", 8))
	inner.AddWidget(label)
	root.AddWidget(inner)
	below := NewLabel("following row")
	root.AddWidget(below)
	drainLayoutTest(t)
	before := below.Y()
	label.SetWrap(true)
	label.SetFont(paint.NewFont(Theme().Font.Family(), 24, false, false))
	drainLayoutTest(t)
	if below.Y() <= before || label.Height()+1e-7 < MeasureForWidth(label, label.Width()).Height {
		t.Fatal("wrap/font changes did not reach ancestors")
	}
	label.Hide()
	drainLayoutTest(t)
	if inner.Height() > 32+1e-7 {
		t.Fatal("hidden child still contributes its previous preferred height")
	}
	label.Show()
	drainLayoutTest(t)
	if label.Height()+1e-7 < MeasureForWidth(label, label.Width()).Height {
		t.Fatal("show did not restore current preferred geometry")
	}
	calls := inner.calls
	inner.InvalidateLayout()
	if inner.calls != calls+1 {
		t.Fatal("legacy own-layout invalidation is no longer immediate")
	}
	label.InvalidateParentLayout()
	if inner.calls != calls+2 {
		t.Fatal("legacy parent invalidation is no longer immediate")
	}
	drainLayoutTest(t)
}

type reparentOnResize struct {
	Widget
	once func()
}

func (p *reparentOnResize) OnResize() {
	if callback := p.once; callback != nil {
		p.once = nil
		callback()
	}
}

func TestLayoutInvalidationLayoutCallbackCannotMoveForeignChild(t *testing.T) {
	for _, factory := range []struct {
		name string
		new  func() IWidget
	}{{"VBox", func() IWidget { return NewVBox() }}, {"HBox", func() IWidget { return NewHBox() }}, {"FlexWrap", func() IWidget { return NewFlexWrap() }}} {
		t.Run(factory.name, func(t *testing.T) {
			resetUIQueue()
			t.Cleanup(resetUIQueue)
			root := factory.new()
			root.SetSize(220, 300)
			first := new(reparentOnResize)
			first.Init(first)
			first.SetParent(root)
			moved := NewLabel("moved by the preceding Resize callback")
			moved.SetParent(root)
			drainLayoutTest(t)
			first.SetSize(1, 1) // force a real Resize in the next installation
			newOwner := NewVBox()
			first.once = func() { moved.SetParent(newOwner); moved.SetBounds(7, 8, 9, 10) }
			root.(ILayout).Layout()
			b := moved.Bounds1()
			if moved.Parent() != newOwner || b.X != 7 || b.Y != 8 || b.Width != 9 || b.Height != 10 {
				t.Fatal("old snapshot moved a child now owned by another container", b)
			}
		})
	}
}

func TestLayoutInvalidationFormCallbackCannotMoveForeignField(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root := NewFormLayout()
	root.SetSize(220, 300)
	first := new(reparentOnResize)
	first.Init(first)
	moved := NewLabel("field")
	root.AddLayoutWidget(first, 0, 0, 1, 1)
	root.AddLayoutWidget(moved, 0, 1, 1, 1)
	drainLayoutTest(t)
	first.SetSize(1, 1)
	newOwner := NewVBox()
	first.once = func() { moved.SetParent(newOwner); moved.SetBounds(7, 8, 9, 10) }
	root.Layout()
	b := moved.Bounds1()
	if moved.Parent() != newOwner || b.X != 7 || b.Y != 8 || b.Width != 9 || b.Height != 10 {
		t.Fatal("label Resize moved field, but stale fieldActive overwrote its new owner", b)
	}
}

func TestLayoutInvalidationGridMutationDuringResize(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root := NewGridLayout()
	root.SetSize(220, 300)
	first := new(reparentOnResize)
	first.Init(first)
	moved := NewLabel("cell")
	root.AddWidget(first, 0, 0)
	root.AddWidget(moved, 0, 1)
	drainLayoutTest(t)
	first.SetSize(1, 1)
	first.once = func() { root.AddWidget(moved, 4, 6) }
	root.Layout() // a new cell coordinate must not index old track buffers
	drainLayoutTest(t)
	if moved.X() <= first.X() || moved.Y() <= first.Y() {
		t.Fatal("changed cell coordinates did not receive a new layout turn", moved.Bounds1())
	}
}

func TestLayoutInvalidationFormRegistrationMovesInsteadOfDuplicating(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	form := NewFormLayout()
	field := NewLabel("one field")
	form.AddLayoutWidget(field, 0, 1, 1, 1)
	form.AddLayoutWidget(field, 2, 1, 1, 1)
	if form.rows[0].widget != nil || form.rows[2].widget != field {
		t.Fatal("one widget remains registered in multiple form rows")
	}
}

func TestLayoutInvalidationFormReplacementDoesNotLeaveFloatingChild(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	form := NewFormLayout()
	old, current := NewLabel("old"), NewLabel("new")
	form.AddLayoutWidget(old, 0, 1, 1, 1)
	form.AddLayoutWidget(current, 0, 1, 1, 1)
	if old.Parent() != nil || current.Parent() != form || len(form.Children()) != 1 {
		t.Fatal("replaced form slot leaves an independently painted old child")
	}
}

func TestLayoutInvalidationBoundedTurnsAndPartialCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "finish", true: "hide-resume"}[cancel], func(t *testing.T) {
			resetUIQueue()
			t.Cleanup(resetUIQueue)
			root := newInvalidationVBox()
			root.SetSize(240, 4000)
			children := make([]*invalidationVBox, uiTaskBatchLimit+10)
			for i := range children {
				child := newInvalidationVBox()
				child.AddWidget(NewLabel("row"))
				root.AddWidget(child)
				children[i] = child
			}
			drainLayoutTest(t)
			root.calls = 0
			for _, child := range children {
				child.calls = 0
				child.RequestLayout()
			}
			drainUITasks()
			calls := root.calls
			for _, child := range children {
				calls += child.calls
			}
			if calls < 1 || calls > uiTaskBatchLimit || root.activeLayout == nil {
				t.Fatal("one callback exceeded its bounded layout work or failed to yield", calls)
			}
			partial := root.activeLayout
			if !cancel {
				// A new edit while a partial pass yields is held, not polled or
				// lost. Completion must explicitly wake that successor exactly once.
				last := children[len(children)-1].Children()[0].(*Label)
				last.SetText("newer data while the previous pass yielded")
				uiTaskMu.Lock()
				pending := len(uiTasks)
				uiTaskMu.Unlock()
				if root.pendingLayout == nil || pending != 1 {
					t.Fatal("blocked successor polled instead of waiting for active completion", pending)
				}
			}
			if cancel {
				root.Hide()
				if root.activeLayout != nil || partial.root != nil || partial.entries != nil {
					t.Fatal("hidden partial batch retains the tree or an active guard")
				}
				drainLayoutTest(t)
				root.Show()
			}
			drainLayoutTest(t)
			for _, child := range children {
				if child.calls != 1 || child.queuedLayout != nil {
					t.Fatal("partial pass lost or duplicated a descendant", child.calls)
				}
			}
			if root.activeLayout != nil || root.pendingLayout != nil {
				t.Fatal("completed pass keeps the hierarchy active")
			}
		})
	}
}

func TestLayoutInvalidationReentrantLegacyCallDefers(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root := newInvalidationVBox()
	root.SetSize(200, 200)
	drainLayoutTest(t)
	root.calls, root.maxDepth = 0, 0
	root.onLayout = func() { root.onLayout = nil; root.InvalidateLayout() }
	root.InvalidateLayout()
	drainLayoutTest(t)
	if root.calls != 2 || root.maxDepth != 1 {
		t.Fatal("legacy reentry recursively laid out the same widget", root.calls, root.maxDepth)
	}
}

func TestLayoutInvalidationCustomSDKHintsAreDeferred(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root := NewVBox()
	root.SetSize(240, 300)
	custom := &widthMeasureProbe{hint: SizeHints{Width: 100, Height: 20}, height: 20}
	custom.Init(custom)
	root.AddWidget(custom)
	drainLayoutTest(t)
	custom.naturalCalls, custom.widthCalls = 0, 0
	custom.height = 80
	custom.InvalidateSizeHints()
	if custom.naturalCalls != 0 || custom.widthCalls != 0 || custom.Height() != 20 {
		t.Fatal("setter invalidation synchronously ran custom getters/layout")
	}
	drainLayoutTest(t)
	if custom.Height() != 80 {
		t.Fatal("custom SDK hint update was lost", custom.Height())
	}
}

func TestLayoutInvalidationBlockedRootDoesNotPoll(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root := newInvalidationVBox()
	root.SetSize(200, 200)
	label := NewLabel("old")
	root.AddWidget(label)
	drainLayoutTest(t)
	wakes := 0
	SetUIWakeup(func() { wakes++ })
	root.onLayout = func() {
		root.onLayout = nil
		before := wakes
		label.SetText("new data inside modal layout callback")
		for i := 0; i < 8; i++ {
			drainUITasks()
		}
		uiTaskMu.Lock()
		pending := len(uiTasks)
		uiTaskMu.Unlock()
		if pending != 0 || wakes != before {
			t.Errorf("blocked root self-polls in nested event pump: queued=%d wakeups=%d", pending, wakes-before)
		}
	}
	root.RequestLayout()
	drainLayoutTest(t)
}

func TestLayoutInvalidationIndependentPopupDoesNotRelayoutOwner(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	owner, popup := newInvalidationVBox(), newInvalidationVBox()
	owner.SetSize(240, 300)
	label := NewLabel("popup")
	popup.AddWidget(label)
	owner.AddWidget(popup)
	drainLayoutTest(t)
	// Model a lazily attached popup's ownership boundary without creating an
	// OS window in a unit test; actual native acceptance is a separate gate.
	popup.wt = WtPopup
	t.Cleanup(func() { popup.wt = WtInherit })
	owner.calls, popup.calls = 0, 0
	label.SetText("popup changed")
	drainLayoutTest(t)
	popup.SetSpacing(4)
	drainLayoutTest(t)
	if owner.calls != 0 || popup.calls != 2 || owner.pendingLayout != nil {
		t.Fatal("popup native owner treated as its layout host", owner.calls, popup.calls)
	}
}

func TestLayoutInvalidationNestedSpacing(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	root, inner := NewVBox(), NewVBox()
	root.SetSize(220, 300)
	inner.AddWidget(NewLabel("First"))
	last := NewLabel("Last inside the box")
	inner.AddWidget(last)
	root.AddWidget(inner)
	below := NewLabel("Outside the box")
	root.AddWidget(below)
	root.Layout()

	inner.SetSpacing(30)
	for i := 0; i < 4; i++ {
		drainUITasks()
	}
	need := MeasureForWidth(inner, inner.Width()).Height
	if math.Abs(inner.Height()-need) > 1e-7 || below.Y()+1e-7 < inner.Y()+last.Y()+last.Height() {
		t.Fatalf("nested spacing overlaps following sibling: inner=%.2f need=%.2f siblingY=%.2f childBottom=%.2f", inner.Height(), need, below.Y(), inner.Y()+last.Y()+last.Height())
	}
}
