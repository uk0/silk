package gui

import (
	"math"
	"silk/core"
	"silk/paint"
	"time"
)

func init() {
	core.RegisterFactory("gui.Splitter", core.TypeOf((*Splitter)(nil)))
}

// Splitter is a layout container that splits space between children with draggable handles
// (similar to QSplitter).
type Splitter struct {
	Widget
	vertical    bool      // true = vertical split (children stacked top/bottom)
	sizes       []float64 // proportional sizes for each child
	handleSize  float64   // handle width/height between panes
	dragging    int       // index of handle being dragged (-1 = none)
	dragStart   float64   // mouse position at drag start
	hoverHandle int       // index of handle under the mouse (-1 = none)

	lastClickHandle int             // handle index of the last click (-1 = none)
	lastClickTime   time.Time       // time of the last handle click (double-click detection)
	collapsedSizes  map[int]float64 // prior proportional size of a collapsed pane, keyed by pane index
}

func NewSplitter(vertical bool) *Splitter {
	p := new(Splitter)
	p.Init(p)
	p.vertical = vertical
	p.handleSize = 5
	p.dragging = -1
	p.hoverHandle = -1
	p.lastClickHandle = -1
	p.collapsedSizes = make(map[int]float64)
	return p
}

func (this *Splitter) Vertical() bool {
	return this.vertical
}

func (this *Splitter) SetVertical(b bool) {
	this.vertical = b
}

func (this *Splitter) HandleSize() float64 {
	return this.handleSize
}

func (this *Splitter) SetHandleSize(s float64) {
	this.handleSize = s
}

func (this *Splitter) EnumProperties(list core.IPropertyList) {
	list.AddProperty("垂直", this.Vertical, this.SetVertical)
	list.AddProperty("分隔条大小", this.HandleSize, this.SetHandleSize)
}

// AddWidget adds a pane (child) to the splitter.
func (this *Splitter) AddWidget(w IWidget) {
	w.SetParent(this.Self())
	// Default: equal proportional size for the new child
	this.sizes = append(this.sizes, 1.0)
}

// SetSizes sets the proportional sizes for each child pane.
func (this *Splitter) SetSizes(sizes []float64) {
	this.sizes = make([]float64, len(sizes))
	copy(this.sizes, sizes)
}

// Sizes returns the current proportional sizes.
func (this *Splitter) Sizes() []float64 {
	ret := make([]float64, len(this.sizes))
	copy(ret, this.sizes)
	return ret
}

// normalizeSizes ensures sizes slice matches child count and total is > 0.
func (this *Splitter) normalizeSizes(n int) []float64 {
	for len(this.sizes) < n {
		this.sizes = append(this.sizes, 1.0)
	}
	this.sizes = this.sizes[:n]

	var total float64
	for _, s := range this.sizes {
		total += s
	}
	if total <= 0 {
		for i := range this.sizes {
			this.sizes[i] = 1.0
		}
		total = float64(n)
	}
	return this.sizes
}

func (this *Splitter) Layout() {
	n := this.NakedWidget().childCount()
	if n == 0 {
		return
	}

	w, h := this.Self().Size()
	sizes := this.normalizeSizes(n)

	var total float64
	for _, s := range sizes {
		total += s
	}

	handlesTotal := float64(n-1) * this.handleSize

	if this.vertical {
		// Vertical: children stacked top to bottom
		available := h - handlesTotal
		if available < 0 {
			available = 0
		}
		yOff := 0.0
		this.NakedWidget().eachChild(func(i int, c IWidget) bool {
			ch := available * sizes[i] / total
			c.SetBounds(0, yOff, w, ch)
			yOff += ch + this.handleSize
			return true
		})
	} else {
		// Horizontal: children side by side
		available := w - handlesTotal
		if available < 0 {
			available = 0
		}
		xOff := 0.0
		this.NakedWidget().eachChild(func(i int, c IWidget) bool {
			cw := available * sizes[i] / total
			c.SetBounds(xOff, 0, cw, h)
			xOff += cw + this.handleSize
			return true
		})
	}
}

// handleRect returns the bounding rect of handle at index i (between child i and child i+1).
// Computes geometry from cached sizes without materializing the child list.
func (this *Splitter) handleRect(i int) (hx, hy, hw, hh float64) {
	n := this.NakedWidget().childCount()
	if i < 0 || i >= n-1 {
		return
	}

	w, h := this.Self().Size()
	sizes := this.normalizeSizes(n)

	var total float64
	for _, s := range sizes {
		total += s
	}

	handlesTotal := float64(n-1) * this.handleSize

	if this.vertical {
		available := h - handlesTotal
		if available < 0 {
			available = 0
		}
		yOff := 0.0
		for j := 0; j <= i; j++ {
			yOff += available * sizes[j] / total
			if j < i {
				yOff += this.handleSize
			}
		}
		return 0, yOff, w, this.handleSize
	}

	available := w - handlesTotal
	if available < 0 {
		available = 0
	}
	xOff := 0.0
	for j := 0; j <= i; j++ {
		xOff += available * sizes[j] / total
		if j < i {
			xOff += this.handleSize
		}
	}
	return xOff, 0, this.handleSize, h
}

// handleAtPos returns the handle index at point (x, y), or -1 if none.
// Walks handles inline rather than calling handleRect repeatedly so the
// child-count fetch and total computation happen once per call.
func (this *Splitter) handleAtPos(x, y float64) int {
	n := this.NakedWidget().childCount()
	if n < 2 {
		return -1
	}

	w, h := this.Self().Size()
	sizes := this.normalizeSizes(n)

	var total float64
	for _, s := range sizes {
		total += s
	}

	handlesTotal := float64(n-1) * this.handleSize

	if this.vertical {
		available := h - handlesTotal
		if available < 0 {
			available = 0
		}
		yOff := 0.0
		for i := 0; i < n-1; i++ {
			yOff += available * sizes[i] / total
			// handle is at [yOff, yOff+handleSize) in y
			if y >= yOff && y < yOff+this.handleSize && x >= 0 && x < w {
				return i
			}
			yOff += this.handleSize
		}
		return -1
	}

	available := w - handlesTotal
	if available < 0 {
		available = 0
	}
	xOff := 0.0
	for i := 0; i < n-1; i++ {
		xOff += available * sizes[i] / total
		if x >= xOff && x < xOff+this.handleSize && y >= 0 && y < h {
			return i
		}
		xOff += this.handleSize
	}
	return -1
}

// paneToCollapse picks which pane a double-click on handle h collapses: the
// smaller of the two adjacent panes, defaulting to the pane before the handle
// (index h) when they are equal. Pure decision logic; no layout side effects.
func (this *Splitter) paneToCollapse(h int) int {
	n := this.NakedWidget().childCount()
	if h < 0 || h >= n-1 {
		return -1
	}
	sizes := this.normalizeSizes(n)
	if sizes[h+1] < sizes[h] {
		return h + 1
	}
	return h
}

// collapsePane stores the pane's current proportional size and sets it to 0.
// The proportional layout redistributes the freed space across the other panes
// automatically. No-op for an out-of-range index or an already-collapsed pane.
func (this *Splitter) collapsePane(pane int) {
	n := this.NakedWidget().childCount()
	if pane < 0 || pane >= n {
		return
	}
	this.normalizeSizes(n)
	if this.collapsedSizes == nil {
		this.collapsedSizes = make(map[int]float64)
	}
	if _, ok := this.collapsedSizes[pane]; ok {
		return
	}
	this.collapsedSizes[pane] = this.sizes[pane]
	this.sizes[pane] = 0
}

// restorePane returns a previously collapsed pane to its stored size. The size
// is reclaimed from the other panes via the proportional layout. Reports
// whether a restore happened.
func (this *Splitter) restorePane(pane int) bool {
	if this.collapsedSizes == nil {
		return false
	}
	prior, ok := this.collapsedSizes[pane]
	if !ok {
		return false
	}
	n := this.NakedWidget().childCount()
	if pane >= 0 && pane < n {
		this.normalizeSizes(n)
		this.sizes[pane] = prior
	}
	delete(this.collapsedSizes, pane)
	return true
}

// toggleHandleCollapse implements Qt-style double-click-to-collapse for handle
// h: if either adjacent pane is already collapsed, restore it; otherwise
// collapse the smaller adjacent pane to zero. This is the method the
// double-click path drives, kept free of GL so it can be unit-tested.
func (this *Splitter) toggleHandleCollapse(h int) {
	n := this.NakedWidget().childCount()
	if h < 0 || h >= n-1 {
		return
	}
	if this.restorePane(h) || this.restorePane(h+1) {
		return
	}
	this.collapsePane(this.paneToCollapse(h))
}

func (this *Splitter) OnLeftDown(x, y float64) {
	idx := this.handleAtPos(x, y)
	if idx < 0 {
		return
	}

	now := time.Now()
	// Double-click on the same handle toggles collapse/restore of an adjacent
	// pane (Qt QSplitter behaviour) instead of starting a drag.
	if idx == this.lastClickHandle && now.Sub(this.lastClickTime) < 400*time.Millisecond {
		this.toggleHandleCollapse(idx)
		this.lastClickTime = time.Time{} // reset to avoid triple-click
		this.Layout()
		this.Self().Update()
		return
	}
	this.lastClickTime = now
	this.lastClickHandle = idx

	this.dragging = idx
	if this.vertical {
		this.dragStart = y
	} else {
		this.dragStart = x
	}
	this.PushCapture()
	if this.vertical {
		SetOverrideCursor(cursorSizeNS)
	} else {
		SetOverrideCursor(cursorSizeWE)
	}
}

func (this *Splitter) OnMouseMove(x, y float64) {
	if this.dragging >= 0 {
		n := this.NakedWidget().childCount()
		sizes := this.normalizeSizes(n)

		var total float64
		for _, s := range sizes {
			total += s
		}

		w, h := this.Self().Size()
		handlesTotal := float64(n-1) * this.handleSize

		var delta, available float64
		if this.vertical {
			delta = y - this.dragStart
			available = h - handlesTotal
			this.dragStart = y
		} else {
			delta = x - this.dragStart
			available = w - handlesTotal
			this.dragStart = x
		}

		if available <= 0 {
			return
		}

		// Convert pixel delta to proportion delta
		propDelta := delta * total / available

		i := this.dragging
		newLeft := sizes[i] + propDelta
		newRight := sizes[i+1] - propDelta

		minProp := 0.01 * total // minimum 1% each
		newLeft = math.Max(newLeft, minProp)
		newRight = math.Max(newRight, minProp)

		this.sizes[i] = newLeft
		this.sizes[i+1] = newRight

		this.Layout()
		this.Self().Update()
	} else {
		// Hover detection: track the handle under the cursor so Draw can
		// highlight it, and swap in a resize cursor as an affordance.
		idx := this.handleAtPos(x, y)
		if idx != this.hoverHandle {
			this.hoverHandle = idx
			this.Self().Update()
		}
		if idx >= 0 {
			if this.vertical {
				SetOverrideCursor(cursorSizeNS)
			} else {
				SetOverrideCursor(cursorSizeWE)
			}
		} else {
			SetOverrideCursor(nil)
		}
	}
}

func (this *Splitter) OnLeftUp(x, y float64) {
	if this.dragging >= 0 {
		this.dragging = -1
		this.PopCapture()
		SetOverrideCursor(nil)
	}
}

func (this *Splitter) OnMouseLeave() {
	if this.dragging < 0 {
		SetOverrideCursor(nil)
	}
	if this.hoverHandle != -1 {
		this.hoverHandle = -1
		this.Self().Update()
	}
}

func (this *Splitter) Cursor() *Cursor {
	return cursorArrow
}

func (this *Splitter) Draw(g paint.Painter) {
	n := this.NakedWidget().childCount()
	t := Theme()

	// Accent wash painted behind the active/hovered handle as a drag
	// affordance: stronger while dragging, fainter on plain hover.
	accent := t.HighLightColor

	// Draw handles between children
	for i := 0; i < n-1; i++ {
		hx, hy, hw, hh := this.handleRect(i)
		g.Save()
		g.Translate(hx, hy)
		if i == this.dragging || i == this.hoverHandle {
			wash := accent
			if i == this.dragging {
				wash.A = 90
			} else {
				wash.A = 45
			}
			g.Rectangle(0, 0, hw, hh)
			g.SetBrush1(wash)
			g.Fill()
		}
		// Subtle handle: a centered line
		g.SetPen1(t.SeperatorColor, 1)
		if this.vertical {
			mid := hh * 0.5
			g.Line(hw*0.25, mid, hw*0.75, mid)
		} else {
			mid := hw * 0.5
			g.Line(mid, hh*0.25, mid, hh*0.75)
		}
		g.Stroke()
		g.Translate(-hx, -hy)
		g.Restore()
	}
}

func (this *Splitter) SizeHints() SizeHints {
	n := this.NakedWidget().childCount()
	if n == 0 {
		return SizeHints{}
	}

	var totalW, totalH, maxW, maxH float64
	this.NakedWidget().eachChild(func(_ int, c IWidget) bool {
		hi := c.SizeHints()
		totalW += hi.Width
		totalH += hi.Height
		maxW = math.Max(maxW, hi.Width)
		maxH = math.Max(maxH, hi.Height)
		return true
	})

	handlesTotal := float64(n-1) * this.handleSize

	if this.vertical {
		return SizeHints{
			Width:  maxW,
			Height: totalH + handlesTotal,
			Policy: GrowHorizontal | GrowVertical,
		}
	}
	return SizeHints{
		Width:  totalW + handlesTotal,
		Height: maxH,
		Policy: GrowHorizontal | GrowVertical,
	}
}
