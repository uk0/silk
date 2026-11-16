package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.FlexWrap", core.TypeOf((*FlexWrap)(nil)))
}

// FlexWrap is a CSS-Flexbox-like wrap container. Children flow left-to-right
// using their SizeHints widths/heights and wrap to the next line when the
// available width is exceeded. Useful for tag clouds, chip groups, and any
// pile-of-pills UI.
type FlexWrap struct {
	Widget
	spacing float64 // horizontal gap between children in the same row
	rowGap  float64 // vertical gap between rows
	padding Padding
}

// NewFlexWrap returns a FlexWrap with sensible defaults: 4px spacing in both
// directions and zero padding.
func NewFlexWrap() *FlexWrap {
	p := new(FlexWrap)
	p.Init(p)
	p.spacing = 4
	p.rowGap = 4
	return p
}

func (this *FlexWrap) Spacing() float64     { return this.spacing }
func (this *FlexWrap) SetSpacing(s float64) { this.spacing = layoutDimension(s); this.relayout() }

func (this *FlexWrap) RowGap() float64     { return this.rowGap }
func (this *FlexWrap) SetRowGap(g float64) { this.rowGap = layoutDimension(g); this.relayout() }

func (this *FlexWrap) SetPadding(p Padding) { this.padding = p; this.relayout() }

func (this *FlexWrap) EnumProperties(list core.IPropertyList) {
	list.AddProperty("水平间距", this.Spacing, this.SetSpacing)
	list.AddProperty("行间距", this.RowGap, this.SetRowGap)
}

func (this *FlexWrap) relayout() {
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

// AddWidget appends a child. Layout is recomputed lazily on the next Layout()
// call (or eagerly via relayout()). The child's parent is set to this FlexWrap.
func (this *FlexWrap) AddWidget(iw IWidget) {
	iw.SetParent(this.Self())
	this.relayout()
}

// Layout flows visible children left-to-right, wrapping to a new line when a
// child would overflow the available width. Each child is sized to its
// SizeHints.Width/Height (clamped to MinWidth/MaxWidth where set). Reuses the
// shared layoutScratch pool to avoid allocations on the hot path.
func (this *FlexWrap) Layout() {
	if this.NakedWidget().child == nil {
		return
	}

	scratch := acquireLayoutScratch()

	this.NakedWidget().eachVisibleChild(func(c IWidget) bool {
		scratch.visible = append(scratch.visible, c)
		scratch.hints = append(scratch.hints, MeasureForWidth(c, 0))
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
	cx, cy, cw, _ := applyLayoutPadding(this.padding, w, h)
	if cw <= 0 {
		releaseLayoutScratch(scratch)
		return
	}

	// Greedy row-by-row packing.
	xOff := cx
	yOff := cy
	rowH := 0.0

	for i := 0; i < n; i++ {
		childW, childH := flexChildSize(visible[i], hints[i], cw)

		// Wrap if this child won't fit in the current row.
		// Always allow the first child of a row to be placed even if it's
		// wider than cw — otherwise nothing can ever be drawn.
		if xOff > cx && xOff+childW > cx+cw {
			xOff = cx
			yOff += rowH + this.rowGap
			rowH = 0
		}

		visible[i].SetBounds(xOff, yOff, childW, childH)

		xOff += childW + this.spacing
		if childH > rowH {
			rowH = childH
		}
	}

	releaseLayoutScratch(scratch)
}

// Draw — no FlexWrap-specific painting. Children are painted by the framework
// after this method returns.
func (this *FlexWrap) Draw(g paint.Painter) {
}

// SizeHints reports the natural size of the FlexWrap given its current width.
// Width: longest packed row, capped at the configured width if any.
// Height: total stacked row heights + row gaps + vertical padding.
// If no width is set yet, returns the sum of intrinsic widths as a single row.
func (this *FlexWrap) SizeHints() SizeHints {
	return this.measureFlowWidth(this.Width())
}

func (this *FlexWrap) HeightForWidth(width float64) float64 {
	return this.SizeHintsForWidth(width).Height
}

func (this *FlexWrap) SizeHintsForWidth(width float64) SizeHints {
	if hint, ok := inheritedNaturalHints(this, width); ok {
		return hint
	}
	return preserveInheritedHints(this, this.measureFlowWidth(width))
}

func (this *FlexWrap) measureFlowWidth(width float64) SizeHints {
	if this.NakedWidget().child == nil {
		return SizeHints{}
	}

	_, _, cw, _ := applyLayoutPadding(this.padding, layoutDimension(width), 0)

	var maxRowW, totalH, rowW, rowH float64
	rows := 0
	first := true

	this.NakedWidget().eachVisibleChild(func(c IWidget) bool {
		childW, childH := flexChildSize(c, MeasureForWidth(c, 0), cw)

		if first {
			rowW = childW
			rowH = childH
			rows = 1
			first = false
		} else if cw > 0 && rowW+this.spacing+childW > cw {
			if rowW > maxRowW {
				maxRowW = rowW
			}
			totalH += rowH
			rows++
			rowW = childW
			rowH = childH
		} else {
			rowW += this.spacing + childW
			if childH > rowH {
				rowH = childH
			}
		}
		return true
	})

	// Final row commit
	if !first {
		if rowW > maxRowW {
			maxRowW = rowW
		}
		totalH += rowH
	}

	if rows > 1 {
		totalH += float64(rows-1) * this.rowGap
	}

	totalW := maxRowW + this.padding.L + this.padding.R
	totalH += this.padding.T + this.padding.B
	return SizeHints{Width: totalW, Height: totalH, Policy: GrowHorizontal | GrowVertical | ShrinkHorizontal}
}

func flexChildSize(child IWidget, hint SizeHints, available float64) (float64, float64) {
	width := hint.Width
	if width <= 0 {
		width = hint.MinWidth
	}
	if width <= 0 {
		width = 32
	}
	if available > 0 && hint.Policy&ShrinkHorizontal != 0 {
		width = min(width, available)
	}
	if hint.MinWidth > 0 {
		width = max(width, hint.MinWidth)
	}
	if hint.MaxWidth > 0 {
		width = min(width, hint.MaxWidth)
	}
	return width, layoutHintHeight(MeasureForWidth(child, width), 24)
}
