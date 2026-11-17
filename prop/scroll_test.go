package prop

import (
	"fmt"
	"math"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func longInspector(t *testing.T) *PropertySheet {
	t.Helper()
	s := newTestSheet()
	s.SetSize(320, 160)
	for i := 0; i < 30; i++ {
		v := i
		addTestProp(s, fmt.Sprintf("Field%02d", i), func() int { return v }, func(n int) { v = n })
	}
	s.Layout()
	return s
}

func TestInspectorScrollFocusFilterResizeAndClear(t *testing.T) {
	s := longInspector(t)
	last := s.vlist[len(s.vlist)-1]
	last.Control().SetFocus()
	_, limit := s.VertScrollBar().Range()
	if s.ScrollY() != limit || last.Control().Y()+last.Control().Height() != s.body.Height() {
		t.Fatal("keyboard focus did not reveal the last editor", s.ScrollY(), limit)
	}
	s.vlist[0].Control().SetFocus()
	if s.ScrollY() != categoryHeaderHeight {
		t.Fatal("backward focus did not minimally reveal first editor", s.ScrollY())
	}
	s.SetScrollY(limit)
	s.SetFilter("Field29")
	if s.ScrollY() != 0 || s.VertScrollBar().IsVisible() || len(s.vlist) != 1 {
		t.Fatal("filter retained a stale offset or scrollbar")
	}
	s.SetFilter("")
	s.SetScrollY(limit)
	s.SetSize(320, 2000)
	if s.ScrollY() != 0 || s.VertScrollBar().IsVisible() {
		t.Fatal("resize did not clamp range")
	}
	s.SetSize(320, 10)
	if s.body.IsVisible() || s.FindWidgetAt(200, 8) == last.Control() {
		t.Fatal("zero content viewport exposed an editor")
	}
	s.SetSize(320, 160)
	s.SetScrollY(limit)
	s.Clear(nil)
	if s.ScrollY() != 0 || s.VertScrollBar().IsVisible() || len(s.body.Children()) != 0 {
		t.Fatal("clear retained old editors or scroll geometry")
	}
}

func TestInspectorWheelFiniteAndTranslatedHeader(t *testing.T) {
	s := longInspector(t)
	s.OnMouseWheel(10, 100, -0.5)
	if s.ScrollY() != 39 {
		t.Fatal("fractional wheel lost", s.ScrollY())
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		s.SetScrollY(bad)
		s.OnMouseWheel(10, 100, bad)
		if s.ScrollY() != 39 {
			t.Fatal("non-finite scroll admitted", s.ScrollY())
		}
	}
	// Put a later category at the very top of the clipped content.
	addTestProp(s, "OnToggled", func() string { return "handler" }, nil)
	s.Layout()
	y := s.categories["events"].ypos
	s.SetScrollY(y - s.contentTop()) // Clamp: last header is near the end.
	sy := s.ScrollY()
	clientY := y - sy + categoryHeaderHeight/2
	barX := s.body.Width() + 1
	s.OnLeftDown(barX, clientY)
	if !s.categories["events"].expanded {
		t.Fatal("scrollbar click toggled category")
	}
	s.body.OnLeftDown(8, clientY-s.contentTop())
	if s.categories["events"].expanded {
		t.Fatal("scrolled category hit used stale coordinates")
	}
}

func TestInspectorHiddenEditorStaysLazyAndHidden(t *testing.T) {
	s := newTestSheet()
	p := addTestProp(s, "Field", func() int { return 1 }, nil)
	s.SetFilter("nonexistent")
	if p.control != nil {
		t.Fatal("filter eagerly materialized an unused editor")
	}
	if p.Control().IsVisible() {
		t.Fatal("late access revived a filtered editor")
	}
	last := addTestProp(s, "Last", func() int { return 2 }, nil)
	s.SetFilter("Last")
	s.SetFilter("")
	order := s.body.ChildFocusOrder()
	if len(order) != 2 || order[0] != p.control || order[1] != last.control {
		t.Fatal("lazy materialization changed visual Tab order")
	}
}

func TestInspectorScrollDoesNotRebindReadOrSplitTypingHistory(t *testing.T) {
	value, reads := 0, 0
	stack := gui.NewUndoStack("scroll-session")
	s := longInspector(t)
	s.objOwner = &sessionOwner{stack}
	p := addTestProp(s, "Editable", func() int { reads++; return value }, func(n int) { value = n })
	s.Layout()
	e := p.Control().(*IntEdit)
	e.SetFocus()
	e.SelectAll()
	e.OnTextInput("1")
	session := e.editSession
	beforeReads, beforeControl := reads, p.control
	s.SetScrollY(100)
	s.SetScrollY(200)
	if reads != beforeReads || p.control != beforeControl || session == nil || session.closed {
		t.Fatal("scroll read/recreated controls or closed a live typing session")
	}
	e.OnTextInput("2")
	e.OnSubmit(e.Text())
	if value != 12 || stack.Count() != 1 {
		t.Fatal("scroll split the typing transaction", value, stack.Count())
	}
	stack.Undo()
	if value != 0 {
		t.Fatal("undo did not preserve pre-scroll baseline")
	}
	stack.Redo()
	if value != 12 {
		t.Fatal("redo did not preserve final value")
	}
}

func TestInspectorScrollPaintAndHitClipping(t *testing.T) {
	s := longInspector(t)
	render := func() paint.Pixmap {
		pix := paint.NewPixmap(320, 160)
		g := pix.NewPainter()
		gui.DrawWidgetAll(s, g, 0, 0, 0, 0, 320, 160)
		paint.ClosePainter(g)
		pix.Flush()
		return pix
	}
	before := render()
	defer paint.CloseSurface(before)
	s.SetScrollY(70) // first editor now crosses above the body's top edge
	after := render()
	defer paint.CloseSurface(after)
	a, err := before.Image()
	if err != nil {
		t.Fatal(err)
	}
	b, err := after.Image()
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < int(s.contentTop()); y++ {
		for x := 0; x < 320; x++ {
			if a.At(x, y) != b.At(x, y) {
				t.Fatalf("scrolled editor painted over fixed search at %d,%d", x, y)
			}
		}
	}
	first := s.vlist[0].Control()
	if s.FindWidgetAt(200, 10) == first || s.FindWidgetAt(319, 60) != s.VertScrollBar() {
		t.Fatal("offscreen editor intercepted search/scrollbar input")
	}
	start, end := s.visibleEntries()
	if start == 0 || end-start >= len(s.categoryLayout) || end-start > 7 {
		t.Fatal("paint traversal was not bounded to the visible rows", start, end)
	}
}

type viewportPopupProbe struct {
	gui.Widget
	dismissals int
}

func (p *viewportPopupProbe) HideSubPopup() { p.dismissals++ }

func TestInspectorScrollDismissesMovedInlinePopupsOnly(t *testing.T) {
	s := longInspector(t)
	control := s.vlist[0].control
	p := new(viewportPopupProbe)
	p.Init(p)
	p.SetParent(control)
	s.SetScrollY(40)
	if p.dismissals != 1 {
		t.Fatal("moved custom descendant kept its popup", p.dismissals)
	}
	s.SetScrollY(40)
	if p.dismissals != 1 {
		t.Fatal("unchanged offset disturbed transient state")
	}
}

func TestInspectorScrollRangeAndBoundedEditors(t *testing.T) {
	s := longInspector(t)
	bar := s.VertScrollBar()
	_, max := bar.Range()
	if max <= 0 || !bar.IsVisible() {
		t.Fatalf("long property sheet has no scroll range: %v", max)
	}
	first := s.vlist[0].Control()
	if first.Parent() == s {
		t.Fatal("editor is unbounded in the sheet instead of a clipped body")
	}
	y := first.Y()
	s.SetScrollY(52)
	if first.Y() != y-52 {
		t.Fatal("scrollbar does not move the actual editor", first.Y(), y)
	}
	if s.searchBox.Y() != filterBoxMargin || bar.Y() != s.contentTop() {
		t.Fatal("scroll displaced the fixed search or put scrollbar above content")
	}
}

func TestInspectorWheelOverSingleLineEditor(t *testing.T) {
	s := longInspector(t)
	e := s.vlist[0].Control().(*IntEdit)
	before := e.Text()
	e.OnMouseWheel(4, 4, -1)
	if s.ScrollY() != 78 || e.Text() != before {
		t.Fatal("single-line editor swallowed the wheel or edited its value", s.ScrollY(), e.Text())
	}
	s.SetScrollY(0)
	e.SetMultiLine(true)
	e.OnMouseWheel(4, 4, -1)
	if s.ScrollY() != 0 {
		t.Fatal("multiline editor lost its own scrolling")
	}
	e.SetMultiLine(false)
	e.SetParent(nil)
	e.OnMouseWheel(4, 4, -1)
	if s.ScrollY() != 0 {
		t.Fatal("detached editor scrolled an unrelated sheet")
	}
}
