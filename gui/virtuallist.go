package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.VirtualList", core.TypeOf((*VirtualList)(nil)))
}

// VirtualList is a virtualized scrolling list — only the rows currently inside
// the visible viewport are drawn, regardless of itemCount. This keeps draw cost
// O(viewport_height / itemHeight) instead of O(itemCount), making the widget
// usable for millions of rows where ListWidget would stall.
//
// Rows have a fixed itemHeight. Drawing of each row is delegated to a caller-
// provided drawItemFn, so the widget itself is data-source-agnostic.
type VirtualList struct {
	Widget
	itemCount   int
	itemHeight  float64
	scrollY     float64 // scroll offset in pixels
	selectedIdx int
	hoverIdx    int

	drawItemFn  func(g paint.Painter, index int, x, y, w, h float64)
	cbItemClick func(index int)

	// Cached visuals
	bgColor       paint.Color
	selectedColor paint.Color
	hoverColor    paint.Color
	hasBg         bool
}

// NewVirtualList returns a VirtualList with itemHeight=28 and no items.
// Call SetItemCount, SetItemHeight, and SetItemDrawer to configure it.
func NewVirtualList() *VirtualList {
	p := new(VirtualList)
	p.Init(p)
	p.itemHeight = 28
	p.selectedIdx = -1
	p.hoverIdx = -1
	return p
}

// ItemCount returns the configured item count.
func (this *VirtualList) ItemCount() int { return this.itemCount }

// SetItemCount sets the number of rows. Negative values are clamped to 0.
// The view is invalidated and scroll position is clamped.
func (this *VirtualList) SetItemCount(n int) {
	if n < 0 {
		n = 0
	}
	this.itemCount = n
	this.clampScroll()
	if this.selectedIdx >= n {
		this.selectedIdx = -1
	}
	this.Self().Update()
}

// ItemHeight returns the per-row height in pixels.
func (this *VirtualList) ItemHeight() float64 { return this.itemHeight }

// SetItemHeight sets the per-row height. Values <= 0 are ignored.
func (this *VirtualList) SetItemHeight(h float64) {
	if h <= 0 {
		return
	}
	this.itemHeight = h
	this.clampScroll()
	this.Self().Update()
}

// SetItemDrawer installs the function that paints a single row. The function
// receives a painter with origin at (0, 0) of the widget and the absolute row
// rectangle in widget-local coordinates.
func (this *VirtualList) SetItemDrawer(fn func(g paint.Painter, index int, x, y, w, h float64)) {
	this.drawItemFn = fn
	this.Self().Update()
}

// SigItemClick registers the click callback. The index is in [0, itemCount).
func (this *VirtualList) SigItemClick(fn func(int)) {
	this.cbItemClick = fn
}

// SelectedIndex returns the current selection, or -1 if none.
func (this *VirtualList) SelectedIndex() int { return this.selectedIdx }

// SetSelectedIndex selects a row programmatically. Out-of-range values clear
// the selection.
func (this *VirtualList) SetSelectedIndex(i int) {
	if i < -1 || i >= this.itemCount {
		i = -1
	}
	this.selectedIdx = i
	this.Self().Update()
}

// ScrollY returns the current scroll offset (pixels from top).
func (this *VirtualList) ScrollY() float64 { return this.scrollY }

// SetScrollY scrolls the view to the given pixel offset, clamped to the
// valid range.
func (this *VirtualList) SetScrollY(y float64) {
	this.scrollY = y
	this.clampScroll()
	this.Self().Update()
}

// SetBackgroundColor sets the optional background fill. Pass a zero-alpha
// color to disable.
func (this *VirtualList) SetBackgroundColor(c paint.Color) {
	this.bgColor = c
	this.hasBg = c.A > 0
	this.Self().Update()
}

// SetSelectedColor sets the highlight color drawn under the selected row.
func (this *VirtualList) SetSelectedColor(c paint.Color) {
	this.selectedColor = c
	this.Self().Update()
}

// SetHoverColor sets the highlight color drawn under the hover row.
func (this *VirtualList) SetHoverColor(c paint.Color) {
	this.hoverColor = c
	this.Self().Update()
}

// maxScroll returns the largest valid scrollY for the current viewport.
func (this *VirtualList) maxScroll() float64 {
	_, h := this.Self().Size()
	contentH := float64(this.itemCount) * this.itemHeight
	max := contentH - h
	if max < 0 {
		return 0
	}
	return max
}

func (this *VirtualList) clampScroll() {
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	if max := this.maxScroll(); this.scrollY > max {
		this.scrollY = max
	}
}

// hitTest converts a y-coordinate (widget-local) to a row index, or -1 if the
// point lies outside any valid row.
func (this *VirtualList) hitTest(y float64) int {
	if this.itemHeight <= 0 || this.itemCount == 0 {
		return -1
	}
	idx := int((y + this.scrollY) / this.itemHeight)
	if idx < 0 || idx >= this.itemCount {
		return -1
	}
	return idx
}

// --- Events ---

func (this *VirtualList) OnLeftDown(x, y float64) {
	idx := this.hitTest(y)
	if idx < 0 {
		return
	}
	this.SetFocus()
	if this.selectedIdx != idx {
		this.selectedIdx = idx
		this.Self().Update()
	}
	if this.cbItemClick != nil {
		this.cbItemClick(idx)
	}
}

func (this *VirtualList) OnMouseMove(x, y float64) {
	idx := this.hitTest(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

func (this *VirtualList) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the list. z is the wheel delta in notches (positive =
// up). Each notch moves defaultWheelScrollLines rows.
func (this *VirtualList) OnMouseWheel(x, y, z float64) {
	step := this.itemHeight * defaultWheelScrollLines
	this.SetScrollY(this.scrollY - z*step)
}

// --- Drawing ---

func (this *VirtualList) Draw(g paint.Painter) {
	w, h := this.Self().Size()

	if this.hasBg {
		g.Rectangle(0, 0, w, h)
		g.SetBrush1(this.bgColor)
		g.Fill()
	}

	if this.itemHeight <= 0 || this.itemCount <= 0 || this.drawItemFn == nil {
		return
	}

	// Compute the inclusive [r0, r1) range of row indices that intersect the
	// viewport. r0 is the topmost partially-visible row; r1 is the first row
	// fully below the viewport.
	r0 := int(this.scrollY / this.itemHeight)
	if r0 < 0 {
		r0 = 0
	}
	r1 := int((this.scrollY+h)/this.itemHeight) + 1
	if r1 > this.itemCount {
		r1 = this.itemCount
	}

	// Clip to widget rect so partial rows at the edges don't paint outside.
	g.Save()
	g.Rectangle(0, 0, w, h)
	g.Clip()

	// y for r0, in widget-local coordinates.
	yStart := float64(r0)*this.itemHeight - this.scrollY

	y := yStart
	for r := r0; r < r1; r++ {
		// Selection background.
		if r == this.selectedIdx && this.selectedColor.A > 0 {
			g.Rectangle(0, y, w, this.itemHeight)
			g.SetBrush1(this.selectedColor)
			g.Fill()
		} else if r == this.hoverIdx && this.hoverColor.A > 0 {
			g.Rectangle(0, y, w, this.itemHeight)
			g.SetBrush1(this.hoverColor)
			g.Fill()
		}
		this.drawItemFn(g, r, 0, y, w, this.itemHeight)
		y += this.itemHeight
	}

	g.Restore()
}

// SizeHints returns a virtual size: width is a default 200, height is the
// total content height (itemCount * itemHeight). This is an O(1) computation
// so it's safe to call extremely frequently.
func (this *VirtualList) SizeHints() SizeHints {
	totalH := float64(this.itemCount) * this.itemHeight
	return SizeHints{
		Width:  200,
		Height: totalH,
		Policy: GrowHorizontal | GrowVertical,
	}
}

func (this *VirtualList) EnumProperties(list core.IPropertyList) {
	list.AddProperty("行高", this.ItemHeight, this.SetItemHeight)
}
