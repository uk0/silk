package gui

import (
	"math"

	"github.com/uk0/silk/geom"
)

const defaultComboVisibleItems = 10

// MaxVisibleItems is the preferred number of rows in the dropdown. The actual
// viewport may show fewer on a small monitor or near the work-area edge.
func (c *ComboBox) MaxVisibleItems() int {
	if c.maxVisibleItems < 1 {
		return defaultComboVisibleItems
	}
	return c.maxVisibleItems
}

// SetMaxVisibleItems changes the preferred viewport size, not the item count.
// Values below one become one. Call on the UI thread; an open preview is kept.
func (c *ComboBox) SetMaxVisibleItems(n int) {
	n = max(1, n)
	if n == c.MaxVisibleItems() {
		return
	}
	c.maxVisibleItems = n
	if c.IsSubPopupVisible() {
		x, y := c.MapToGlobal(0, 0)
		w, h := c.Size()
		LayoutPopup(c.sub, x, y, w, h, true, 1)
		c.sub.scrollRowIntoView(c.sub.activeIndex)
	}
}

// Window positions/sizes are integer coordinate units on the native backends.
// Round the work area inward, including on monitors with negative origins.
func comboPopupWorkArea(area geom.Rect) geom.Rect {
	if !validPopupArea(area) {
		return area
	}
	left, top := math.Ceil(area.X), math.Ceil(area.Y)
	return geom.Rect{X: left, Y: top, Width: math.Max(1, math.Floor(area.Right())-left), Height: math.Max(1, math.Floor(area.Bottom())-top)}
}

func (p *comboPopup) fitPopupWorkArea(area, anchor geom.Rect) {
	limit := defaultComboVisibleItems
	if p.combo != nil {
		limit = p.combo.MaxVisibleItems()
	}
	rows := min(p.Count(), limit)
	width := math.Max(1, math.Ceil(anchor.Width))
	height := math.Max(1, math.Ceil(float64(rows)*p.RowHeight()+p.padding.T+p.padding.B))
	if validPopupArea(area) {
		width = math.Min(width, area.Width)
		// Prefer staying next to the field, rather than covering the whole owner.
		below := math.Max(0, area.Bottom()-math.Max(area.Y, anchor.Bottom()-1))
		above := math.Max(0, math.Min(area.Bottom(), anchor.Y+1)-area.Y)
		available := math.Min(area.Height, math.Max(below, above))
		if available < 1 {
			available = area.Height
		} // field itself fills the work area
		height = math.Min(height, math.Max(1, math.Floor(available)))
	}
	p.SetSize(width, height)
	// Reopening on another monitor or changing font may retain native dimensions
	// after rounding. Always refresh scroll ranges before revealing the selection.
	p.Layout()
}
