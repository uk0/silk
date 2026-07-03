package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.VBox", core.TypeOf((*VBox)(nil)))
}

// VBox is a layout container that stacks children vertically.
// Supports stretch weights, alignment, minimum sizes, and hidden-widget skipping.
type VBox struct {
	Widget
	spacing float64
	padding Padding
	halign  TextAlign // 子控件水平对齐方式
}

func NewVBox() *VBox {
	p := new(VBox)
	p.Init(p)
	return p
}

func (this *VBox) Spacing() float64      { return this.spacing }
func (this *VBox) SetSpacing(s float64)  { this.spacing = s; this.relayout() }
func (this *VBox) SetPadding(p Padding)  { this.padding = p; this.relayout() }
func (this *VBox) HAlign() TextAlign     { return this.halign }
func (this *VBox) SetHAlign(a TextAlign) { this.halign = a; this.relayout() }

func (this *VBox) relayout() {
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

func (this *VBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("间距", this.Spacing, this.SetSpacing)
}

func (this *VBox) AddWidget(iw IWidget) {
	iw.SetParent(this.Self())
	this.relayout()
}

func (this *VBox) Layout() {
	if this.NakedWidget().child == nil {
		return
	}

	scratch := acquireLayoutScratch()

	// Phase 0: filter visible children + collect hints in a single linked-list
	// walk. Buffers come from sync.Pool so this is allocation-free on the hot
	// path once steady-state.
	this.NakedWidget().eachVisibleChild(func(c IWidget) bool {
		scratch.visible = append(scratch.visible, c)
		scratch.hints = append(scratch.hints, c.SizeHints())
		return true
	})

	visible := scratch.visible
	hints := scratch.hints
	n := len(visible)
	if n == 0 {
		releaseLayoutScratch(scratch)
		return
	}

	w, h := this.Self().Size()
	cx, cy, cw, ch := this.padding.Apply(0, 0, w, h)
	if cw <= 0 || ch <= 0 {
		releaseLayoutScratch(scratch)
		return
	}

	// Phase 1: classify children (hints already collected in phase 0).
	var fixedTotal float64
	var totalStretch int

	for i := 0; i < n; i++ {
		hi := hints[i]
		if hi.Stretch > 0 {
			totalStretch += hi.Stretch
		} else {
			fixedH := hi.Height
			if fixedH <= 0 {
				fixedH = hi.MinHeight
			}
			if fixedH <= 0 {
				fixedH = 32
			}
			fixedTotal += fixedH
		}
	}

	if totalStretch <= 0 {
		totalStretch = 1
	}

	spacingTotal := float64(n-1) * this.spacing
	remaining := ch - fixedTotal - spacingTotal
	if remaining < 0 {
		remaining = 0
	}

	// Phase 2: distribute space. Reuse scratch.sizes for the heights buffer.
	if cap(scratch.sizes) >= n {
		scratch.sizes = scratch.sizes[:n]
	} else {
		scratch.sizes = make([]float64, n)
	}
	heights := scratch.sizes
	for i := 0; i < n; i++ {
		hi := hints[i]
		var v float64
		if hi.Stretch > 0 {
			v = remaining * float64(hi.Stretch) / float64(totalStretch)
		} else {
			v = hi.Height
			if v <= 0 {
				v = hi.MinHeight
			}
			if v <= 0 {
				v = 32
			}
		}

		// Clamp to min/max
		if hi.MinHeight > 0 && v < hi.MinHeight {
			v = hi.MinHeight
		}
		if hi.MaxHeight > 0 && v > hi.MaxHeight {
			v = hi.MaxHeight
		}
		heights[i] = v
	}

	// Phase 3: position children
	yOff := cy
	for i := 0; i < n; i++ {
		c := visible[i]
		hi := hints[i]
		childH := heights[i]

		// Determine child width and horizontal position
		childW := cw
		childX := cx

		if hi.Width > 0 && hi.Width < cw {
			if (hi.Policy & (GrowHorizontal | ExpandHorizontal)) == 0 {
				childW = hi.Width
				switch this.halign {
				case AlignCenter:
					childX = cx + (cw-childW)/2
				case AlignRight:
					childX = cx + cw - childW
				default: // AlignLeft
					childX = cx
				}
			}
		}

		c.SetBounds(childX, yOff, childW, childH)
		yOff += childH + this.spacing
	}

	releaseLayoutScratch(scratch)
}

func (this *VBox) Draw(g paint.Painter) {
	// Children are drawn by the framework after this method returns.
}

func (this *VBox) SizeHints() SizeHints {
	// Walk the linked list directly; avoids the slice allocation that
	// Widget.Children() would perform.
	n := 0
	var totalH, maxW float64

	this.NakedWidget().eachVisibleChild(func(c IWidget) bool {
		hi := c.SizeHints()
		h := hi.Height
		if h <= 0 {
			h = hi.MinHeight
		}
		totalH += h
		maxW = math.Max(maxW, hi.Width)
		n++
		return true
	})

	if n > 1 {
		totalH += float64(n-1) * this.spacing
	}
	totalH += this.padding.T + this.padding.B
	maxW += this.padding.L + this.padding.R

	return SizeHints{Width: maxW, Height: totalH, Policy: GrowHorizontal | GrowVertical}
}
