package gui

import (
	"fmt"
	"math"
	"testing"

	"github.com/uk0/silk/geom"
)

func TestLongPopupFitsAnchorWorkAreaWithoutShrinkingRows(t *testing.T) {
	previous := popupWorkAreaAt
	t.Cleanup(func() { popupWorkAreaAt = previous })
	area := geom.Rect{X: -640, Y: 24, Width: 640, Height: 300}
	popupWorkAreaAt = func(_, _ float64) geom.Rect { return area }
	m := NewPopupMenu()
	for i := 0; i < 80; i++ {
		m.AddButton1(fmt.Sprintf("Command %02d", i), nil)
	}
	hints := m.SizeHints()
	m.SetSize(hints.Width, hints.Height)
	LayoutPopup1(m, -200, 250)
	if m.Height() > area.Height || m.Y() < area.Y || m.Y()+m.Height() > area.Bottom() {
		t.Fatalf("long popup escapes work area: popup=%+v area=%+v", m.Bounds1(), area)
	}
	for _, item := range m.Items() {
		if item.Height() != item.SizeHints().Height {
			t.Fatal("row height was squeezed instead of scrolling")
		}
	}
}

func scrollTestMenu(t *testing.T) *Menu {
	t.Helper()
	oldHover, oldFocus, oldCapture := mouseHoverWidget, focusWidget, captureStack
	t.Cleanup(func() { mouseHoverWidget, focusWidget, captureStack = oldHover, oldFocus, oldCapture })
	captureStack = nil
	m := visibleActivationMenu()
	for i := 0; i < 100; i++ {
		m.AddButton1(fmt.Sprintf("Item %03d", i), nil)
	}
	m.items[99].(*Button).SetText("Zulu")
	m.items[97].Hide()
	m.items[98].SetEnabled(false)
	m.SetSize(m.SizeHints().Width, 240)
	return m
}

func assertMenuTargetVisible(t *testing.T, m *Menu) {
	t.Helper()
	b := m.highlightedButton()
	v := m.ChildViewport()
	if b == nil || b.Y() < v.Y-0.001 || b.Y()+b.Height() > v.Bottom()+0.001 {
		t.Fatalf("highlight %d outside viewport %+v", m.highlight, v)
	}
	if got := m.FindWidgetAt(b.X()+2, b.Y()+b.Height()/2); got != b {
		t.Fatal("visible highlighted row is not hittable")
	}
}

func TestLongMenuKeyboardAndTypeaheadKeepTargetVisible(t *testing.T) {
	m := scrollTestMenu(t)
	m.OnKeyDown(KeyEnd, false)
	if m.highlight != 99 || m.scroll.offset <= 0 {
		t.Fatal("End did not scroll to last enabled item")
	}
	assertMenuTargetVisible(t, m)
	m.OnKeyDown(KeyDown, false)
	if m.highlight != 0 || m.scroll.offset != 0 {
		t.Fatal("Down wrap did not reveal first item")
	}
	assertMenuTargetVisible(t, m)
	m.OnKeyDown(KeyPageDown, false)
	if m.highlight <= 1 || m.highlight >= 20 {
		t.Fatal("page did not move roughly one viewport", m.highlight)
	}
	assertMenuTargetVisible(t, m)
	m.OnKeyDown(KeyPageUp, false)
	if m.highlight != 0 {
		t.Fatal("page up did not clamp at first selectable item")
	}
	m.OnTextInput("z")
	if m.highlight != 99 {
		t.Fatal("typeahead missed the offscreen row")
	}
	assertMenuTargetVisible(t, m)
	m.OnKeyDown(KeyUp, false)
	if m.highlight != 96 {
		t.Fatal("hidden/disabled rows became navigation targets")
	}
	assertMenuTargetVisible(t, m)
	calls := 0
	m.items[96].(*Button).Action().BindFunc0(func() { calls++ })
	m.OnKeyDown(KeyEnter, false)
	if calls != 1 || m.IsVisible() {
		t.Fatal("scrolled activation did not execute once and close")
	}
}

func TestLongMenuWheelBandsClampAndDoNotActivateRows(t *testing.T) {
	m := scrollTestMenu(t)
	v := m.ChildViewport()
	m.OnKeyDown(KeyHome, false)
	m.OnMouseWheel(20, v.Y+30, -0.25)
	if m.scroll.offset <= 0 || m.highlight != -1 {
		t.Fatal("fractional wheel did not scroll/clear stale keyboard target")
	}
	if got := m.FindWidgetAt(20, v.Y-1); got != m {
		t.Fatal("offscreen row intercepted the top scroll band")
	}
	if got := m.FindWidgetAt(20, v.Bottom()+1); got != m {
		t.Fatal("offscreen row intercepted the bottom scroll band")
	}
	before := m.scroll.offset
	m.OnLeftDown(20, v.Bottom()+1)
	if m.scroll.offset <= before {
		t.Fatal("bottom scroll band did not scroll")
	}
	m.OnMouseWheel(20, v.Y+30, -100000)
	if m.scroll.offset != m.scroll.content-v.Height {
		t.Fatal("wheel did not clamp at end")
	}
	m.OnMouseWheel(20, v.Y+30, 100000)
	if m.scroll.offset != 0 {
		t.Fatal("wheel did not clamp at start")
	}
	for _, z := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0} {
		m.OnMouseWheel(20, v.Y+30, z)
	}
	if m.scroll.offset != 0 {
		t.Fatal("nonfinite or zero wheel changed position")
	}
	m.Hide()
	m.OnMouseWheel(20, 30, -1)
	if m.scroll.offset != 0 {
		t.Fatal("closed popup consumed a wheel")
	}
}

func TestLongMenuRelayoutRestoresNaturalSizeAndClampsOffset(t *testing.T) {
	m := scrollTestMenu(t)
	m.OnKeyDown(KeyEnd, false)
	// Live removal/visibility changes do not leave the viewport beyond the end.
	for i := 1; i < len(m.items); i++ {
		m.items[i].Hide()
	}
	m.Layout()
	if m.scroll.offset != 0 || m.scroll.band != 0 {
		t.Fatal("shortened menu retained stale scroll geometry")
	}
	m.fitPopupWorkArea(geom.Rect{Width: 1000, Height: 900})
	if m.Height() != math.Ceil(m.SizeHints().Height) {
		t.Fatal("short menu did not recover its natural size")
	}
	for _, c := range m.items {
		c.Show()
	}
	m.fitPopupWorkArea(geom.Rect{Width: 1000, Height: 180})
	if m.Height() != 180 || m.scroll.band == 0 {
		t.Fatal("second monitor constraint not applied")
	}
	m.OnKeyDown(KeyEnd, false)
	m.OnHide()
	m.fitPopupWorkArea(geom.Rect{Width: 1000, Height: 10000})
	if m.Height() != math.Ceil(m.SizeHints().Height) || m.scroll.offset != 0 || m.scroll.band != 0 {
		t.Fatal("reopen retained previous monitor constraint or offset")
	}
}

func TestLongMenuScrollingClosesAnchoredSubmenu(t *testing.T) {
	m := scrollTestMenu(t)
	owner := m.items[4].(*Button)
	sub := visibleActivationMenu()
	sub.AddButton1("Child", nil)
	owner.SetSubPopup(sub)
	sub.SetParent(owner)
	sub.setVisible(true)
	m.OnMouseWheel(20, 100, -1)
	if sub.IsVisible() {
		t.Fatal("submenu floated away from its scrolled owner")
	}
}

func BenchmarkMenuScrollThousandItems(b *testing.B) {
	m := visibleActivationMenu()
	for i := 0; i < 1000; i++ {
		m.AddButton1(fmt.Sprintf("Command %04d", i), nil)
	}
	m.SetSize(m.SizeHints().Width, 600)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.setScrollOffset(float64(i%100) * 25)
	}
}
