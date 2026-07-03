package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.HBox", core.TypeOf((*HBox)(nil)))
}

// HBox is a layout container that stacks children horizontally.
// Supports stretch weights, alignment, minimum sizes, and hidden-widget skipping.
type HBox struct {
	Widget
	spacing float64
	padding Padding
	valign  VertAlign // 子控件垂直对齐方式
}

func NewHBox() *HBox {
	p := new(HBox)
	p.Init(p)
	return p
}

func (this *HBox) Spacing() float64      { return this.spacing }
func (this *HBox) SetSpacing(s float64)  { this.spacing = s; this.relayout() }
func (this *HBox) SetPadding(p Padding)  { this.padding = p; this.relayout() }
func (this *HBox) VAlign() VertAlign     { return this.valign }
func (this *HBox) SetVAlign(a VertAlign) { this.valign = a; this.relayout() }

func (this *HBox) relayout() {
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

func (this *HBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("间距", this.Spacing, this.SetSpacing)
}

func (this *HBox) AddWidget(iw IWidget) {
	iw.SetParent(this.Self())
	this.relayout()
}

func (this *HBox) Layout() {
	// No children → nothing to do, avoid pool acquisition entirely.
	if this.NakedWidget().child == nil {
		return
	}

	scratch := acquireLayoutScratch()

	// Phase 0: filter visible children using a non-allocating walk over the
	// circular linked child list. Captured into scratch.visible / scratch.hints
	// so phases 1-3 can reuse them by index.
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
			// Stretchable child — participates in proportional distribution
			totalStretch += hi.Stretch
		} else {
			// Fixed-width child
			fixedW := hi.Width
			if fixedW <= 0 {
				fixedW = hi.MinWidth
			}
			if fixedW <= 0 {
				fixedW = 32 // fallback minimum
			}
			fixedTotal += fixedW
		}
	}

	if totalStretch <= 0 {
		totalStretch = 1
	}

	spacingTotal := float64(n-1) * this.spacing
	remaining := cw - fixedTotal - spacingTotal
	if remaining < 0 {
		remaining = 0
	}

	// Phase 2: distribute space. Reuse scratch.sizes as the widths buffer.
	// Grow it to length n; existing capacity is reused without allocation
	// when n <= cap (typical case after the first call).
	if cap(scratch.sizes) >= n {
		scratch.sizes = scratch.sizes[:n]
	} else {
		scratch.sizes = make([]float64, n)
	}
	widths := scratch.sizes
	for i := 0; i < n; i++ {
		hi := hints[i]
		var v float64
		if hi.Stretch > 0 {
			// Proportional allocation based on stretch weight
			v = remaining * float64(hi.Stretch) / float64(totalStretch)
		} else {
			v = hi.Width
			if v <= 0 {
				v = hi.MinWidth
			}
			if v <= 0 {
				v = 32
			}
		}

		// Clamp to min/max
		if hi.MinWidth > 0 && v < hi.MinWidth {
			v = hi.MinWidth
		}
		if hi.MaxWidth > 0 && v > hi.MaxWidth {
			v = hi.MaxWidth
		}
		widths[i] = v
	}

	// Phase 3: position children
	xOff := cx
	for i := 0; i < n; i++ {
		c := visible[i]
		hi := hints[i]
		childW := widths[i]

		// Determine child height and vertical position
		childH := ch
		childY := cy

		if hi.Height > 0 && hi.Height < ch {
			if (hi.Policy & (GrowVertical | ExpandVertical)) == 0 {
				childH = hi.Height
				// Apply vertical alignment
				switch this.valign {
				case VA_CENTER:
					childY = cy + (ch-childH)/2
				case VA_BOTTOM:
					childY = cy + ch - childH
				default: // VA_TOP
					childY = cy
				}
			}
		}

		c.SetBounds(xOff, childY, childW, childH)
		xOff += childW + this.spacing
	}

	// Manual release: defer would add overhead on this hot path.
	releaseLayoutScratch(scratch)
}

func (this *HBox) Draw(g paint.Painter) {
	// Children are drawn by the framework after this method returns.
}

func (this *HBox) SizeHints() SizeHints {
	// Walk the linked list directly without materializing []IWidget. Each
	// child's SizeHints() is consulted exactly once and accumulated inline.
	n := 0
	var totalW, maxH float64

	this.NakedWidget().eachVisibleChild(func(c IWidget) bool {
		hi := c.SizeHints()
		w := hi.Width
		if w <= 0 {
			w = hi.MinWidth
		}
		totalW += w
		maxH = math.Max(maxH, hi.Height)
		n++
		return true
	})

	if n > 1 {
		totalW += float64(n-1) * this.spacing
	}
	totalW += this.padding.L + this.padding.R
	maxH += this.padding.T + this.padding.B

	return SizeHints{Width: totalW, Height: maxH, Policy: GrowHorizontal | GrowVertical}
}
