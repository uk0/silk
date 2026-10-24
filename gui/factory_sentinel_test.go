package gui

import (
	"math"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// The widgets below all keep a "-1 means nothing is selected / nothing is
// hovered" index. core.RegisterFactory builds a widget by reflection and calls
// Init only, so an index seeded in the constructor is 0 on every instance the
// designer places or a .tdoc file loads — a phantom selection or hover pinned
// to item 0. These tests pin the sentinel to Init by checking both build paths.

// sentinelWidget names one member of the family: the factory id the designer
// places, the constructor a hand-written caller uses, and the index a fresh
// instance must report as -1 no matter which path built it.
type sentinelWidget struct {
	factory string
	build   func() IWidget
	field   string
	read    func(IWidget) int
	meaning string // what a 0 would mean on screen
}

var sentinelFamily = []sentinelWidget{
	{"gui.Accordion", func() IWidget { return NewAccordion() }, "hoverIdx",
		func(w IWidget) int { return w.(*Accordion).hoverIdx },
		"section 0 draws hovered"},
	{"gui.Accordion", func() IWidget { return NewAccordion() }, "curIdx",
		func(w IWidget) int { return w.(*Accordion).curIdx },
		"section 0 draws as the keyboard current item"},
	{"gui.Breadcrumb", func() IWidget { return NewBreadcrumb() }, "hoverIdx",
		func(w IWidget) int { return w.(*Breadcrumb).hoverIdx },
		"crumb 0 draws in the hovered link colour"},
	{"gui.ButtonBox", func() IWidget { return NewButtonBox() }, "highlight",
		func(w IWidget) int { return w.(*ButtonBox).highlight },
		"the first button counts as keyboard-highlighted"},
	{"gui.Calendar", func() IWidget { return NewCalendar() }, "hoverRow",
		func(w IWidget) int { return w.(*Calendar).hoverRow },
		"the top-left day cell draws the hover wash"},
	{"gui.Calendar", func() IWidget { return NewCalendar() }, "hoverCol",
		func(w IWidget) int { return w.(*Calendar).hoverCol },
		"the top-left day cell draws the hover wash"},
	{"gui.DiffView", func() IWidget { return NewDiffView() }, "ActiveChangeRow()",
		func(w IWidget) int { return w.(*DiffView).ActiveChangeRow() },
		"row 0 reports as the active change before anything is loaded"},
	{"gui.DropdownButton", func() IWidget { return NewDropdownButton() }, "Selected()",
		func(w IWidget) int { return w.(*DropdownButton).Selected() },
		"item 0 reports as chosen while the caption still shows the placeholder"},
	{"gui.Menu", func() IWidget { return NewPopupMenu() }, "highlight",
		func(w IWidget) int { return w.(*Menu).highlight },
		"the first item counts as keyboard-highlighted"},
	{"gui.NotificationPanel", func() IWidget { return NewNotificationPanel() }, "hoverIdx",
		func(w IWidget) int { return w.(*NotificationPanel).hoverIdx },
		"card 0 draws the hover background"},
	{"gui.Pagination", func() IWidget { return NewPagination() }, "hover",
		func(w IWidget) int { return w.(*Pagination).hover },
		"the prev-arrow cell draws the hover wash"},
	{"gui.Splitter", func() IWidget { return NewSplitter(false) }, "dragging",
		func(w IWidget) int { return w.(*Splitter).dragging },
		"handle 0 draws as if it were being dragged"},
	{"gui.Splitter", func() IWidget { return NewSplitter(false) }, "hoverHandle",
		func(w IWidget) int { return w.(*Splitter).hoverHandle },
		"handle 0 draws hovered"},
	{"gui.Splitter", func() IWidget { return NewSplitter(false) }, "lastClickHandle",
		func(w IWidget) int { return w.(*Splitter).lastClickHandle },
		"the first click on handle 0 counts as the second half of a double-click"},
	{"gui.VirtualList", func() IWidget { return NewVirtualList() }, "SelectedIndex()",
		func(w IWidget) int { return w.(*VirtualList).SelectedIndex() },
		"row 0 reports and paints as selected"},
	{"gui.VirtualList", func() IWidget { return NewVirtualList() }, "hoverIdx",
		func(w IWidget) int { return w.(*VirtualList).hoverIdx },
		"row 0 paints as hovered"},
}

// TestFactoryBuiltWidgetsReportNothingSelected is the designer/.tdoc path: the
// factory reflects on Init, so a sentinel that only the constructor writes never
// runs here.
func TestFactoryBuiltWidgetsReportNothingSelected(t *testing.T) {
	for _, sw := range sentinelFamily {
		obj := core.New(sw.factory)
		w, ok := obj.(IWidget)
		if !ok {
			t.Fatalf("factory %s built %T, want an IWidget", sw.factory, obj)
		}
		if got := sw.read(w); got != -1 {
			t.Errorf("%s built by core.New reports %s = %d, want -1 (nothing selected/hovered): %s",
				sw.factory, sw.field, got, sw.meaning)
		}
	}
}

// TestConstructedWidgetsReportNothingSelected is the same assertion on the
// hand-written path, so moving a sentinel into Init cannot quietly drop it from
// the constructor's result.
func TestConstructedWidgetsReportNothingSelected(t *testing.T) {
	for _, sw := range sentinelFamily {
		w := sw.build()
		if got := sw.read(w); got != -1 {
			t.Errorf("%s built by its constructor reports %s = %d, want -1 (nothing selected/hovered): %s",
				sw.factory, sw.field, got, sw.meaning)
		}
	}
}

// TestButtonBoxHasNoPhantomHighlight covers the reason the sentinel belongs in
// Menu.Init rather than a ButtonBox.Init: ButtonBox embeds Menu and NewButtonBox
// never routes through NewMenu, so before the fix highlight was 0 on BOTH build
// paths. highlightedButton is what Enter and Right consult, so a 0 means the box
// acts on its first button before any arrow key has been pressed.
func TestButtonBoxHasNoPhantomHighlight(t *testing.T) {
	factoryBox, ok := core.New("gui.ButtonBox").(*ButtonBox)
	if !ok {
		t.Fatalf("factory gui.ButtonBox did not build a *ButtonBox")
	}
	for _, tc := range []struct {
		path string
		box  *ButtonBox
	}{
		{"NewButtonBox", NewButtonBox()},
		{"core.New(\"gui.ButtonBox\")", factoryBox},
	} {
		tc.box.SetButtonsSpec("@ok,@cancel")
		if b := tc.box.highlightedButton(); b != nil {
			t.Errorf("%s: a fresh ButtonBox already highlights %q; Enter/Right would act on it before any arrow key",
				tc.path, b.Text())
		}
	}
}

// TestFactoryMenuFirstDownHighlightsFirstItem is the keyboard consequence for a
// factory-built menu: from the -1 sentinel the first Down lands on item 0. With
// highlight seeded at 0 the same key jumps straight to the second item.
func TestFactoryMenuFirstDownHighlightsFirstItem(t *testing.T) {
	m, ok := core.New("gui.Menu").(*Menu)
	if !ok {
		t.Fatalf("factory gui.Menu did not build a *Menu")
	}
	// Turn it into the popup form — the only one that takes keys — in the same
	// order NewMenu does, so no real window is created in a unit test.
	m.SetVisible(false)
	m.LazyAttachWindow(WtPopup)
	first := m.AddButton1("Item0", nil)
	m.AddButton1("Item1", nil)

	// Hidden popups must reject stale keyboard input, even when factory-built.
	m.OnKeyDown(KeyDown, false)
	if m.highlightedButton() != nil {
		t.Fatal("hidden factory menu accepted navigation")
	}
	m.setVisible(true) // Logical show, without creating a native window.
	m.OnKeyDown(KeyDown, false)
	if got := m.highlightedButton(); got != first {
		name := "nothing"
		if got != nil {
			name = got.Text()
		}
		t.Errorf("first Down on a factory-built menu highlighted %s, want the first item %q",
			name, first.Text())
	}
}

// TestFactoryAccordionFirstDownSelectsFirstSection is the same keyboard
// consequence for the accordion: Down from "no current section" opens at
// section 0, not section 1.
func TestFactoryAccordionFirstDownSelectsFirstSection(t *testing.T) {
	a, ok := core.New("gui.Accordion").(*Accordion)
	if !ok {
		t.Fatalf("factory gui.Accordion did not build an *Accordion")
	}
	a.AddSection("One", nil)
	a.AddSection("Two", nil)

	a.OnKeyDown(KeyDown, false)
	if a.curIdx != 0 {
		t.Errorf("first Down on a factory-built accordion moved to section %d, want 0", a.curIdx)
	}
}

// --- Draw assertions --------------------------------------------------
//
// Each case builds the widget the way the designer does (factory + property
// setters) and checks that nothing paints the highlight over index 0.

// TestFactoryBuiltWidgetsDrawNoPhantomHighlight drives Draw on fresh
// factory-built widgets and fails if the highlight colour reaches the painter.
func TestFactoryBuiltWidgetsDrawNoPhantomHighlight(t *testing.T) {
	t.Run("gui.Accordion", func(t *testing.T) {
		a := core.New("gui.Accordion").(*Accordion)
		a.SetSize(200, 200)
		a.AddSection("One", nil)
		a.AddSection("Two", nil)

		rec := newSentinelPainter()
		a.Draw(rec)
		// FormLightColor is the hovered/current header fill (FormColor is the
		// idle one); HighLightColor is the current item's left accent bar.
		if f, ok := rec.filled(Theme().FormLightColor); ok {
			t.Errorf("a fresh accordion painted the hover/current header fill at y=%.0f; no section is hovered yet", f.y)
		}
		if f, ok := rec.filled(Theme().HighLightColor); ok {
			t.Errorf("a fresh accordion painted the current-section accent bar at y=%.0f; no section is current yet", f.y)
		}
	})

	t.Run("gui.Breadcrumb", func(t *testing.T) {
		b := core.New("gui.Breadcrumb").(*Breadcrumb)
		b.SetSize(240, 24)
		b.AddItem("Home", nil)
		b.AddItem("Page", nil)

		rec := newSentinelPainter()
		b.Draw(rec)
		hoverLink := paint.Color{R: 66, G: 133, B: 244, A: 255}
		if rec.painted(hoverLink) {
			t.Errorf("a fresh breadcrumb drew a crumb in the hovered link colour %v; the mouse has not entered it", hoverLink)
		}
	})

	t.Run("gui.Calendar", func(t *testing.T) {
		c := core.New("gui.Calendar").(*Calendar)
		c.SetSize(240, 240)
		c.ShowMonth(2026, time.June)

		rec := newSentinelPainter()
		c.Draw(rec)
		hoverWash := paint.Color{R: 230, G: 230, B: 245, A: 255}
		if f, ok := rec.filled(hoverWash); ok {
			t.Errorf("a fresh calendar washed the day cell at (%.0f,%.0f) as hovered; the mouse has not entered the grid", f.x, f.y)
		}
	})

	t.Run("gui.NotificationPanel", func(t *testing.T) {
		// Asserted in dark mode on purpose: in the light theme the hovered card
		// fill (FormLightColor) and the idle one (ViewBGColor) are both #FFFFFF,
		// so the phantom hover is invisible there and provable here.
		orig := CurrentThemeMode()
		t.Cleanup(func() { SetThemeMode(orig) })
		SetThemeMode(ThemeDark)

		n := core.New("gui.NotificationPanel").(*NotificationPanel)
		n.SetSize(280, 200)
		n.AddNotification(NotificationItem{Title: "A"})
		n.AddNotification(NotificationItem{Title: "B"})

		rec := newSentinelPainter()
		n.Draw(rec)
		if f, ok := rec.filled(Theme().FormLightColor); ok {
			t.Errorf("a fresh notification panel painted the hover card background at y=%.0f; no card is hovered yet", f.y)
		}
	})

	t.Run("gui.Pagination", func(t *testing.T) {
		p := core.New("gui.Pagination").(*Pagination)
		p.SetSize(320, 32)
		p.SetTotalPages(5)
		p.SetCurrentPage(2) // keeps the prev arrow enabled, so a hover would show

		rec := newSentinelPainter()
		p.Draw(rec)
		// FormDarkColor is only ever *filled* for the hover wash; disabled
		// arrows use it as ink, which the recorder keeps separate.
		if f, ok := rec.filled(Theme().FormDarkColor); ok {
			t.Errorf("a fresh pager washed the cell at x=%.0f as hovered; the mouse has not entered it", f.x)
		}
	})

	t.Run("gui.Splitter", func(t *testing.T) {
		s := core.New("gui.Splitter").(*Splitter)
		s.SetHandleSize(10)
		s.SetSize(200, 50)
		s.AddWidget(NewLabel("left"))
		s.AddWidget(NewLabel("right"))

		rec := newSentinelPainter()
		s.Draw(rec)
		dragWash := Theme().HighLightColor
		dragWash.A = 90
		hoverWash := Theme().HighLightColor
		hoverWash.A = 45
		if f, ok := rec.filled(dragWash); ok {
			t.Errorf("a fresh splitter painted the drag wash on the handle at x=%.0f; no handle is being dragged", f.x)
		}
		if f, ok := rec.filled(hoverWash); ok {
			t.Errorf("a fresh splitter painted the hover wash on the handle at x=%.0f; the mouse has not touched it", f.x)
		}
	})

	t.Run("gui.VirtualList", func(t *testing.T) {
		v := core.New("gui.VirtualList").(*VirtualList)
		v.SetSize(200, 100)
		v.SetItemHeight(20)
		v.SetItemCount(5)
		v.SetItemDrawer(func(g paint.Painter, index int, x, y, w, h float64) {})
		selected := paint.Color{R: 1, G: 2, B: 3, A: 255}
		hover := paint.Color{R: 4, G: 5, B: 6, A: 255}
		v.SetSelectedColor(selected)
		v.SetHoverColor(hover)

		rec := newSentinelPainter()
		v.Draw(rec)
		if f, ok := rec.filled(selected); ok {
			t.Errorf("a fresh virtual list painted row %.0f with the selection colour; nothing has been clicked",
				math.Floor(f.y/20))
		}
		if f, ok := rec.filled(hover); ok {
			t.Errorf("a fresh virtual list painted row %.0f with the hover colour; the mouse has not entered it",
				math.Floor(f.y/20))
		}
	})
}

// --- recording painter ------------------------------------------------

// sentinelFill is one fill the recorder captured: the brush in force and the
// bounding box of the path it covered, in widget coordinates.
type sentinelFill struct {
	color      paint.Color
	x, y, w, h float64
}

// sentinelPainter records what a Draw paints instead of rendering it: every
// brush colour installed, and every fill together with the bounding box of the
// path built since the last fill/stroke. That is enough to answer the only
// question these tests ask — "did anything paint the highlight over index 0?".
// Translate/Save/Restore are tracked so the recorded boxes stay in widget
// coordinates. Methods the widgets under test never call fall through to the
// embedded nil Painter.
type sentinelPainter struct {
	paint.Painter
	brush   paint.Color
	tx, ty  float64
	stack   [][2]float64
	hasPath bool
	x0, y0  float64
	x1, y1  float64

	brushes []paint.Color
	fills   []sentinelFill
}

func newSentinelPainter() *sentinelPainter { return &sentinelPainter{} }

// painted reports whether c was ever installed as the brush — the test for ink
// (text) as well as fills.
func (p *sentinelPainter) painted(c paint.Color) bool {
	for _, b := range p.brushes {
		if b == c {
			return true
		}
	}
	return false
}

// filled reports the first fill painted in colour c, if any.
func (p *sentinelPainter) filled(c paint.Color) (sentinelFill, bool) {
	for _, f := range p.fills {
		if f.color == c {
			return f, true
		}
	}
	return sentinelFill{}, false
}

func (p *sentinelPainter) point(x, y float64) {
	x, y = x+p.tx, y+p.ty
	if !p.hasPath {
		p.x0, p.y0, p.x1, p.y1 = x, y, x, y
		p.hasPath = true
		return
	}
	p.x0, p.y0 = math.Min(p.x0, x), math.Min(p.y0, y)
	p.x1, p.y1 = math.Max(p.x1, x), math.Max(p.y1, y)
}

func (p *sentinelPainter) record() {
	p.fills = append(p.fills, sentinelFill{p.brush, p.x0, p.y0, p.x1 - p.x0, p.y1 - p.y0})
}

func (p *sentinelPainter) Save() int {
	p.stack = append(p.stack, [2]float64{p.tx, p.ty})
	return len(p.stack)
}

func (p *sentinelPainter) Restore() int {
	if n := len(p.stack); n > 0 {
		p.tx, p.ty = p.stack[n-1][0], p.stack[n-1][1]
		p.stack = p.stack[:n-1]
	}
	return len(p.stack)
}

func (p *sentinelPainter) Translate(tx, ty float64) { p.tx, p.ty = p.tx+tx, p.ty+ty }

func (p *sentinelPainter) Rectangle(x, y, w, h float64) { p.point(x, y); p.point(x+w, y+h) }
func (p *sentinelPainter) MoveTo(x, y float64)          { p.point(x, y) }
func (p *sentinelPainter) LineTo(x, y float64)          { p.point(x, y) }
func (p *sentinelPainter) Line(x1, y1, x2, y2 float64)  { p.point(x1, y1); p.point(x2, y2) }

func (p *sentinelPainter) Arc(xc, yc, radius, angle1, angle2 float64) {
	p.point(xc-radius, yc-radius)
	p.point(xc+radius, yc+radius)
}

func (p *sentinelPainter) Fill()           { p.record(); p.hasPath = false }
func (p *sentinelPainter) FillPreserve()   { p.record() }
func (p *sentinelPainter) Stroke()         { p.hasPath = false }
func (p *sentinelPainter) StrokePreserve() {}
func (p *sentinelPainter) Clip()           { p.hasPath = false }
func (p *sentinelPainter) ClipPreserve()   {}

func (p *sentinelPainter) SetBrush1(c paint.Color) {
	p.brush = c
	p.brushes = append(p.brushes, c)
}

func (p *sentinelPainter) SetPen1(c paint.Color, width float64) {}
func (p *sentinelPainter) SetFont(f paint.Font)                 {}
func (p *sentinelPainter) DrawText(text string)                 {}
func (p *sentinelPainter) DrawText1(x, y float64, text string)  {}
