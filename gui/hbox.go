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
	layoutDisabled bool
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

func (this *HBox) Spacing() float64     { return this.spacing }
func (this *HBox) SetSpacing(s float64) { this.spacing = layoutDimension(s); this.relayout() }
func (this *HBox) Padding() Padding     { return this.padding }

func (this *HBox) SetPadding(p Padding)  { this.padding = p; this.relayout() }
func (this *HBox) VAlign() VertAlign     { return this.valign }
func (this *HBox) SetVAlign(a VertAlign) { this.valign = a; this.relayout() }

func (this *HBox) relayout() {
	this.InvalidateLayout()
}

func (this *HBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("间距", this.Spacing, this.SetSpacing)
	enumLayoutPadding(list, this.Padding, this.SetPadding)
}

func (this *HBox) AddWidget(iw IWidget) {
	iw.SetParent(this.Self())
	this.relayout()
}

func (this *HBox) Layout() {
	if this.layoutDisabled {
		return
	}
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
	cx, cy, cw, ch := applyLayoutPadding(this.padding, w, h)
	if cw <= 0 || ch <= 0 {
		releaseLayoutScratch(scratch)
		return
	}

	scratch.sizes = resolveHBoxWidths(hints, cw, this.spacing, scratch.sizes)
	widths := scratch.sizes

	// Phase 3: position children
	xOff := cx
	for i := 0; i < n; i++ {
		c := visible[i]
		if c.Parent() != this.Self() || !c.IsVisible() {
			continue
		}
		childW := widths[i]
		hi := MeasureForWidth(c, childW)

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
	if this.layoutDisabled {
		return SizeHints{Width: this.Width(), Height: this.Height()}
	}
	// Walk the linked list directly without materializing []IWidget. Each
	// child's SizeHints() is consulted exactly once and accumulated inline.
	n := 0
	var totalW, maxH float64
	policy := GrowHorizontal | GrowVertical

	this.NakedWidget().eachVisibleChild(func(c IWidget) bool {
		hi := c.SizeHints()
		w := hi.Width
		if w <= 0 {
			w = hi.MinWidth
		}
		totalW += w
		maxH = math.Max(maxH, hi.Height)
		policy |= hi.Policy & ShrinkHorizontal
		n++
		return true
	})

	if n > 1 {
		totalW += float64(n-1) * this.spacing
	}
	totalW += this.padding.L + this.padding.R
	maxH += this.padding.T + this.padding.B

	return SizeHints{Width: totalW, Height: maxH, Policy: policy}
}

func (this *HBox) HeightForWidth(width float64) float64 {
	return this.SizeHintsForWidth(width).Height
}

func (this *HBox) SizeHintsForWidth(width float64) SizeHints {
	if hint, ok := inheritedNaturalHints(this, width); ok {
		return hint
	}
	return preserveInheritedHints(this, this.measureHBoxWidth(width))
}

func (this *HBox) measureHBoxWidth(width float64) SizeHints {
	if this.layoutDisabled {
		return this.SizeHints()
	}
	scratch := acquireLayoutScratch()
	defer releaseLayoutScratch(scratch)
	this.NakedWidget().eachVisibleChild(func(child IWidget) bool {
		scratch.visible = append(scratch.visible, child)
		scratch.hints = append(scratch.hints, MeasureForWidth(child, 0))
		return true
	})
	_, _, available, _ := applyLayoutPadding(this.padding, layoutDimension(width), 0)
	if width > 0 {
		scratch.sizes = resolveHBoxWidths(scratch.hints, available, this.spacing, scratch.sizes)
	}
	var height, preferredWidth float64
	policy := GrowHorizontal | GrowVertical
	for i, child := range scratch.visible {
		hint := scratch.hints[i]
		if width > 0 {
			hint = MeasureForWidth(child, scratch.sizes[i])
		}
		height = max(height, layoutHintHeight(hint, 0))
		preferredWidth += scratch.hints[i].Width
		policy |= hint.Policy & ShrinkHorizontal
	}
	if len(scratch.visible) > 1 {
		preferredWidth += float64(len(scratch.visible)-1) * this.spacing
	}
	return SizeHints{Width: preferredWidth + this.padding.L + this.padding.R,
		Height: height + this.padding.T + this.padding.B, Policy: policy}
}

// LayoutEnabled defaults to true for ordinary runtime widgets. Designer files
// can explicitly preserve a legacy free-positioned container without having
// a later resize silently rearrange its contents.
func (this *HBox) LayoutEnabled() bool { return !this.layoutDisabled }
func (this *HBox) SetLayoutEnabled(enabled bool) {
	this.layoutDisabled = !enabled
	this.InvalidateLayout()
}
