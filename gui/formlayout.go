package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.FormLayout", core.TypeOf((*FormLayout)(nil)))
}

// FormRow represents a label+widget pair in the form layout.
type FormRow struct {
	label  IWidget
	widget IWidget
}

// FormLayout is a two-column layout with labels on the left and widgets on the right
// (similar to QFormLayout).
type FormLayout struct {
	layoutDisabled bool
	Widget
	rows       []FormRow
	labelWidth float64 // width reserved for labels
	spacing    float64 // vertical spacing between rows
	rowHeight  float64 // default row height (0 = auto from SizeHints)
	padding    Padding
}

func NewFormLayout() *FormLayout {
	p := new(FormLayout)
	p.Init(p)
	return p
}

// Init carries the label column width, not NewFormLayout: the designer and
// the .tdoc loader build widgets through the core factory, which reflects on
// Init and never sees the constructor. A zero labelWidth starts the field
// column at x=0, so every field paints on top of its own label.
func (this *FormLayout) Init(self IWidget) {
	this.Widget.Init(self)
	this.labelWidth = 100
}

func (this *FormLayout) LabelWidth() float64 {
	return this.labelWidth
}

func (this *FormLayout) SetLabelWidth(w float64) {
	this.labelWidth = layoutDimension(w)
	this.InvalidateLayout()
}

func (this *FormLayout) Spacing() float64 {
	return this.spacing
}

func (this *FormLayout) SetSpacing(s float64) {
	this.spacing = layoutDimension(s)
	this.InvalidateLayout()
}

func (this *FormLayout) RowHeight() float64 {
	return this.rowHeight
}

func (this *FormLayout) SetRowHeight(h float64) {
	this.rowHeight = layoutDimension(h)
	this.InvalidateLayout()
}

func (this *FormLayout) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标签宽度", this.LabelWidth, this.SetLabelWidth)
	list.AddProperty("间距", this.Spacing, this.SetSpacing)
	enumLayoutPadding(list, this.Padding, this.SetPadding)
	list.AddProperty("行高", this.RowHeight, this.SetRowHeight)
}

func (this *FormLayout) Padding() Padding { return this.padding }

func (this *FormLayout) SetPadding(p Padding) {
	this.padding = p
	this.InvalidateLayout()
}

// AddRow adds a label+widget pair to the form.
// A Label is created internally for the label text, with right alignment.
func (this *FormLayout) AddRow(labelText string, w IWidget) {
	lbl := NewLabel(labelText)
	lbl.SetAlign(AlignRight)
	lbl.SetParent(this.Self())
	w.SetParent(this.Self())
	this.rows = append(this.rows, FormRow{
		label:  lbl,
		widget: w,
	})
}

// AddLayoutWidget registers existing designer objects, without manufacturing
// duplicate labels. Column 0 is the label and column 1 is its field.
func (this *FormLayout) AddLayoutWidget(w IWidget, row, col, rowSpan, colSpan int) {
	if w == nil || row < 0 || row > 4095 || col < 0 || col > 1 {
		return
	}
	for len(this.rows) <= row {
		this.rows = append(this.rows, FormRow{})
	}
	if col == 0 {
		this.rows[row].label = w
	} else {
		this.rows[row].widget = w
	}
	w.SetParent(this.Self())
}

// Intrinsic measurement is also used by the Gantt/scene dialog builders to
// choose their explicitly authored label columns and fixed row heights.
func formRowHints(row FormRow) (SizeHints, SizeHints) {
	var label, field SizeHints
	if row.label != nil {
		label = row.label.SizeHints()
	}
	if row.widget != nil {
		field = row.widget.SizeHints()
	}
	return label, field
}

func (this *FormLayout) rowHints(row FormRow, labelWidth, fieldWidth float64) (SizeHints, SizeHints, bool) {
	var label, field SizeHints
	active := false
	if row.label != nil && row.label.Parent() == this.Self() && row.label.IsVisible() {
		label = MeasureForWidth(row.label, labelWidth)
		active = true
	}
	if row.widget != nil && row.widget.Parent() == this.Self() && row.widget.IsVisible() {
		field = MeasureForWidth(row.widget, fieldWidth)
		active = true
	}
	return label, field, active
}

func (this *FormLayout) Layout() {
	if this.layoutDisabled {
		return
	}
	n := len(this.rows)
	if n == 0 {
		return
	}

	w, h := this.Self().Size()
	x0, y0, w, h := applyLayoutPadding(this.padding, w, h)

	labelW := math.Min(this.labelWidth, w)
	widgetX := x0 + labelW + this.spacing
	widgetW := w - labelW - this.spacing
	if widgetW < 0 {
		widgetW = 0
	}

	// Pooled scratch for row heights — reuse the generic layoutScratch sizes
	// buffer (it's an []float64 already). This eliminates the per-call
	// `make([]float64, n)`.
	scratch := acquireLayoutScratch()
	if cap(scratch.sizes) >= n {
		scratch.sizes = scratch.sizes[:n]
		for i := range scratch.sizes {
			scratch.sizes[i] = 0
		}
	} else {
		scratch.sizes = make([]float64, n)
	}
	heights := scratch.sizes

	var fixedTotal float64
	var flexCount int
	activeRows := 0

	for i, row := range this.rows {
		lh, wh, active := this.rowHints(row, labelW, widgetW)
		if !active {
			continue
		}
		activeRows++
		if this.rowHeight > 0 {
			heights[i] = this.rowHeight
			fixedTotal += this.rowHeight
		} else {
			// Use the larger of label/widget size hints
			rh := math.Max(layoutHintHeight(lh, 0), layoutHintHeight(wh, 0))
			if rh > 0 {
				heights[i] = rh
				fixedTotal += rh
			} else {
				flexCount++
			}
		}
	}

	spacingTotal := float64(max(0, activeRows-1)) * this.spacing
	remaining := h - fixedTotal - spacingTotal
	var flexH float64
	if flexCount > 0 && remaining > 0 {
		flexH = remaining / float64(flexCount)
	}

	yOff := y0
	for i, row := range this.rows {
		labelActive := row.label != nil && row.label.Parent() == this.Self() && row.label.IsVisible()
		fieldActive := row.widget != nil && row.widget.Parent() == this.Self() && row.widget.IsVisible()
		if !labelActive && !fieldActive {
			continue
		}
		rh := heights[i]
		if rh == 0 {
			rh = flexH
		}

		// Label: right-aligned in the label column
		if labelActive {
			row.label.SetBounds(x0, yOff, labelW, rh)
		}

		// Widget: fills the remaining width
		if fieldActive {
			row.widget.SetBounds(widgetX, yOff, widgetW, rh)
		}

		yOff += rh + this.spacing
	}

	releaseLayoutScratch(scratch)
}

func (this *FormLayout) Draw(g paint.Painter) {
	// Children are drawn by the framework after this method returns.
}

func (this *FormLayout) SizeHints() SizeHints {
	return this.measureFormWidth(0)
}

func (this *FormLayout) HeightForWidth(width float64) float64 {
	return this.SizeHintsForWidth(width).Height
}

func (this *FormLayout) SizeHintsForWidth(width float64) SizeHints {
	if hint, ok := inheritedNaturalHints(this, width); ok {
		return hint
	}
	return preserveInheritedHints(this, this.measureFormWidth(width))
}

func (this *FormLayout) measureFormWidth(width float64) SizeHints {
	if this.layoutDisabled {
		return SizeHints{Width: this.Width(), Height: this.Height()}
	}
	n := len(this.rows)
	if n == 0 {
		return SizeHints{}
	}

	var totalH, maxWidgetW float64
	labelWidth, fieldWidth := this.labelWidth, 0.0
	if width > 0 {
		_, _, innerWidth, _ := applyLayoutPadding(this.padding, layoutDimension(width), 0)
		labelWidth = min(labelWidth, innerWidth)
		fieldWidth = max(0, innerWidth-labelWidth-this.spacing)
	}
	activeRows := 0
	policy := GrowHorizontal | GrowVertical
	for _, row := range this.rows {
		lh, wh, active := this.rowHints(row, labelWidth, fieldWidth)
		if !active {
			continue
		}
		activeRows++
		rh := math.Max(layoutHintHeight(lh, 0), layoutHintHeight(wh, 0))
		if this.rowHeight > 0 {
			rh = this.rowHeight
		}
		totalH += rh
		maxWidgetW = math.Max(maxWidgetW, wh.Width)
		policy |= (lh.Policy | wh.Policy) & ShrinkHorizontal
	}

	totalH += float64(max(0, activeRows-1)) * this.spacing
	totalH += this.padding.T + this.padding.B
	totalW := this.labelWidth + this.spacing + maxWidgetW + this.padding.L + this.padding.R

	return SizeHints{Width: totalW, Height: totalH, Policy: policy}
}

// LayoutEnabled defaults to true for ordinary runtime widgets. Designer files
// can explicitly preserve a legacy free-positioned container without having
// a later resize silently rearrange its contents.
func (this *FormLayout) LayoutEnabled() bool { return !this.layoutDisabled }
func (this *FormLayout) SetLayoutEnabled(enabled bool) {
	this.layoutDisabled = !enabled
	this.InvalidateLayout()
}
