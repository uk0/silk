package gui

import (
	"fmt"
	"math"
	"testing"

	"github.com/uk0/silk/geom"
)

func TestComboPopupConstrainsActualViewportToWorkArea(t *testing.T) {
	oldArea, oldFocus := popupWorkAreaAt, focusWidget
	t.Cleanup(func() { popupWorkAreaAt = oldArea; focusWidget = oldFocus })
	area := geom.Rect{X: -1200, Y: 24, Width: 1200, Height: 700}
	popupWorkAreaAt = func(float64, float64) geom.Rect { return area }
	c := NewComboBox()
	c.SetEditable(false)
	for i := 0; i < 100; i++ {
		c.Append(ListItem{Text: fmt.Sprintf("Item %03d", i)})
	}
	c.SetActiveIndex(0)
	// Same content-sized input as the original ShowSubPopup; placement must
	// constrain the list itself, not merely move an oversized native window.
	hints := c.sub.SizeHints()
	c.sub.SetSize(320, hints.Height)
	LayoutPopup(c.sub, -900, 100, 320, 32, true, 1)
	if c.sub.Height() > area.Height || c.sub.Y()+c.sub.Height() > area.Bottom() {
		t.Fatalf("popup extends beyond monitor: bounds=%v area=%v", c.sub.Bounds1(), area)
	}
	c.sub.Show()
	c.sub.OnKeyDown(KeyEnd, false)
	if c.ActiveIndex() != 0 || c.sub.ActiveIndex() != 99 || c.sub.ScrollY() <= 0 {
		t.Fatal("End must reveal the last pending row without committing")
	}
	_, vh := c.sub.ViewportSizePx()
	m := c.sub.padding
	y := m.T + (99-c.sub.ScrollY())*c.sub.RowHeight()
	if y < m.T || y+c.sub.RowHeight() > vh-m.B+.01 {
		t.Fatalf("last row outside actual viewport: y=%g h=%g", y, vh)
	}
	if row, _ := c.sub.HitTest(m.L+4, y+c.sub.RowHeight()/2); row != 99 {
		t.Fatalf("visible row hit test=%d", row)
	}
	c.HideSubPopup()
}

func TestComboPopupViewportEdgesAndFractionalMetrics(t *testing.T) {
	oldArea := popupWorkAreaAt
	t.Cleanup(func() { popupWorkAreaAt = oldArea })
	for _, tc := range []struct {
		name         string
		area, anchor geom.Rect
		count, limit int
		row          float64
	}{
		{"below", geom.Rect{Width: 1200, Height: 800}, geom.Rect{X: 50, Y: 100, Width: 320, Height: 32}, 100, 10, 24},
		{"above", geom.Rect{Width: 1200, Height: 800}, geom.Rect{X: 50, Y: 760, Width: 320, Height: 32}, 100, 10, 24},
		{"negative_monitor", geom.Rect{X: -1280, Y: -200, Width: 1280, Height: 800}, geom.Rect{X: -250, Y: 400, Width: 320, Height: 32}, 100, 10, 24},
		{"wide_field", geom.Rect{X: 16, Y: 24, Width: 240, Height: 500}, geom.Rect{X: 20, Y: 100, Width: 600, Height: 32}, 100, 10, 24},
		{"fractional_metrics", geom.Rect{X: -1200.4, Y: 24.2, Width: 1199.8, Height: 699.7}, geom.Rect{X: -300.3, Y: 600.1, Width: 320.2, Height: 31.2}, 3, 10, 23.3},
		{"tiny_area", geom.Rect{X: 20, Y: 30, Width: 80, Height: 20}, geom.Rect{X: 25, Y: 30, Width: 160, Height: 32}, 100, 10, 24},
		{"empty", geom.Rect{Width: 800, Height: 600}, geom.Rect{X: 10, Y: 10, Width: 200, Height: 32}, 0, 10, 24},
		{"one_row", geom.Rect{Width: 800, Height: 600}, geom.Rect{X: 10, Y: 10, Width: 200, Height: 32}, 100, 1, 24},
		{"large_limit", geom.Rect{Width: 800, Height: 600}, geom.Rect{X: 10, Y: 200, Width: 200, Height: 32}, 100, 100, 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			popupWorkAreaAt = func(float64, float64) geom.Rect { return tc.area }
			c := NewComboBox()
			c.SetMaxVisibleItems(tc.limit)
			for i := 0; i < tc.count; i++ {
				c.Append(ListItem{Text: fmt.Sprint(i)})
			}
			c.sub.SetRowHeight(tc.row)
			a := tc.anchor
			LayoutPopup(c.sub, a.X, a.Y, a.Width, a.Height, true, 1)
			b := c.sub.Bounds1()
			area := tc.area
			if b.X < area.X || b.Y < area.Y || b.X+b.Width > area.Right()+.001 || b.Y+b.Height > area.Bottom()+.001 || b.Width < 1 || b.Height < 1 {
				t.Fatalf("bounds=%v not within area=%v", b, area)
			}
			preferred := math.Ceil(float64(min(tc.count, tc.limit))*tc.row + c.sub.padding.T + c.sub.padding.B)
			if b.Height > preferred {
				t.Fatalf("height=%g exceeds preferred=%g", b.Height, preferred)
			}
			if tc.name == "fractional_metrics" && c.sub.VertScrollBar().IsVisible() {
				t.Fatal("rounding introduced a phantom scrollbar")
			}
		})
	}
}

func TestComboMaxVisibleItemsPreservesOpenPreview(t *testing.T) {
	oldArea, oldFocus := popupWorkAreaAt, focusWidget
	t.Cleanup(func() { popupWorkAreaAt = oldArea; focusWidget = oldFocus })
	popupWorkAreaAt = func(float64, float64) geom.Rect { return geom.Rect{Width: 1000, Height: 800} }
	c := NewComboBox()
	c.SetEditable(false)
	c.SetBounds(40, 40, 260, 32)
	for i := 0; i < 100; i++ {
		c.Append(ListItem{Text: fmt.Sprint(i)})
	}
	if c.MaxVisibleItems() != 10 {
		t.Fatal("default row cap changed")
	}
	c.SetActiveIndex(0)
	c.preparePopup()
	LayoutPopup(c.sub, 40, 40, 260, 32, true, 1)
	c.sub.Show()
	defer c.HideSubPopup()
	c.sub.OnKeyDown(KeyEnd, false)
	changes, submits := 0, 0
	c.SigSelectionChanged(func(interface{}, int) { changes++ })
	c.SigSubmit(func(interface{}) { submits++ })
	for _, n := range []int{4, 1, -5, 10, 100, 3} {
		c.SetMaxVisibleItems(n)
		if c.MaxVisibleItems() != max(1, n) || c.ActiveIndex() != 0 || c.sub.ActiveIndex() != 99 || c.Count() != 100 || changes != 0 || submits != 0 {
			t.Fatalf("resize %d changed preview, items or committed value", n)
		}
		_, height := c.sub.ViewportSizePx()
		row := c.sub.RowHeight()
		y := c.sub.padding.T + (99-c.sub.ScrollY())*row
		if y < c.sub.padding.T || y+row > height-c.sub.padding.B+.001 {
			t.Fatalf("resize %d hid pending row: y=%g h=%g", n, y, height)
		}
	}
	c.sub.OnKeyDown(KeyEsc, false)
	if c.ActiveIndex() != 0 || c.sub.ActiveIndex() != 0 || changes != 0 || submits != 0 {
		t.Fatal("resize committed canceled preview")
	}
}

func TestComboPopupRecomputesViewportAfterContentAndRowChanges(t *testing.T) {
	c := newTestCombo("one", "two", "three")
	c.SetMaxVisibleItems(20)
	area, anchor := geom.Rect{Width: 1000, Height: 800}, geom.Rect{X: 20, Y: 40, Width: 200, Height: 32}
	c.sub.fitPopupWorkArea(area, anchor)
	before := c.sub.Height()
	c.sub.SetRowHeight(40)
	c.sub.fitPopupWorkArea(area, anchor)
	if c.sub.Height() != 130 || c.sub.Height() <= before {
		t.Fatalf("row size not reflected: %g", c.sub.Height())
	}
	c.RemoveLast()
	c.sub.fitPopupWorkArea(area, anchor)
	if c.sub.Height() != 90 {
		t.Fatalf("removed row retained stale height: %g", c.sub.Height())
	}
	c.sub.fitPopupWorkArea(geom.Rect{}, anchor)
	if c.sub.Height() != 90 {
		t.Fatal("unavailable monitor lost preferred height")
	}
}

func TestComboMaxVisibleItemsProperty(t *testing.T) {
	c := NewComboBox()
	properties := new(recordingPropertyList)
	c.EnumProperties(properties)
	p, ok := properties.find("max_visible_items")
	if !ok {
		t.Fatal("designer is missing viewport row property")
	}
	get, getOK := p.get.(func() int)
	set, setOK := p.set.(func(int))
	if !getOK || !setOK {
		t.Fatal("row property must use integer getter and setter")
	}
	if get() != 10 {
		t.Fatal("wrong default property value")
	}
	set(6)
	if c.MaxVisibleItems() != 6 || get() != 6 {
		t.Fatal("row property did not round trip")
	}
	set(0)
	if get() != 1 {
		t.Fatal("invalid row property not clamped")
	}
}
