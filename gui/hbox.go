package gui

import (
	"silk/core"
	"silk/paint"
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

func (this *HBox) Spacing() float64    { return this.spacing }
func (this *HBox) SetSpacing(s float64) { this.spacing = s; this.relayout() }
func (this *HBox) SetPadding(p Padding) { this.padding = p; this.relayout() }
func (this *HBox) VAlign() VertAlign    { return this.valign }
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
	children := this.Self().Children()
	if len(children) == 0 {
		return
	}

	// Phase 0: filter visible children
	var visible []IWidget
	for _, c := range children {
		if c.IsVisible() {
			visible = append(visible, c)
		}
	}
	n := len(visible)
	if n == 0 {
		return
	}

	w, h := this.Self().Size()
	cx, cy, cw, ch := this.padding.Apply(0, 0, w, h)
	if cw <= 0 || ch <= 0 {
		return
	}

	// Phase 1: collect hints, classify children
	hints := make([]SizeHints, n)
	var fixedTotal float64
	var totalStretch int

	for i, c := range visible {
		hi := c.SizeHints()
		hints[i] = hi

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

	// Phase 2: distribute space
	widths := make([]float64, n)
	for i, hi := range hints {
		if hi.Stretch > 0 {
			// Proportional allocation based on stretch weight
			widths[i] = remaining * float64(hi.Stretch) / float64(totalStretch)
		} else {
			widths[i] = hi.Width
			if widths[i] <= 0 {
				widths[i] = hi.MinWidth
			}
			if widths[i] <= 0 {
				widths[i] = 32
			}
		}

		// Clamp to min/max
		if hi.MinWidth > 0 && widths[i] < hi.MinWidth {
			widths[i] = hi.MinWidth
		}
		if hi.MaxWidth > 0 && widths[i] > hi.MaxWidth {
			widths[i] = hi.MaxWidth
		}
	}

	// Phase 3: position children
	xOff := cx
	for i, c := range visible {
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
}

func (this *HBox) Draw(g paint.Painter) {
	// Children are drawn by the framework after this method returns.
}

func (this *HBox) SizeHints() SizeHints {
	children := this.Self().Children()
	n := 0
	var totalW, maxH float64

	for _, c := range children {
		if !c.IsVisible() {
			continue
		}
		hi := c.SizeHints()
		w := hi.Width
		if w <= 0 {
			w = hi.MinWidth
		}
		totalW += w
		maxH = math.Max(maxH, hi.Height)
		n++
	}

	if n > 1 {
		totalW += float64(n-1) * this.spacing
	}
	totalW += this.padding.L + this.padding.R
	maxH += this.padding.T + this.padding.B

	return SizeHints{Width: totalW, Height: maxH, Policy: GrowHorizontal | GrowVertical}
}
