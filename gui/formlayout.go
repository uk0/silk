package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.FormLayout", core.TypeOf((*FormLayout)(nil)))
}

// FormRow represents a label+widget pair in the form layout.
type FormRow struct {
	label  *Label
	widget IWidget
}

// FormLayout is a two-column layout with labels on the left and widgets on the right
// (similar to QFormLayout).
type FormLayout struct {
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
	p.labelWidth = 100
	return p
}

func (this *FormLayout) LabelWidth() float64 {
	return this.labelWidth
}

func (this *FormLayout) SetLabelWidth(w float64) {
	this.labelWidth = w
}

func (this *FormLayout) Spacing() float64 {
	return this.spacing
}

func (this *FormLayout) SetSpacing(s float64) {
	this.spacing = s
}

func (this *FormLayout) RowHeight() float64 {
	return this.rowHeight
}

func (this *FormLayout) SetRowHeight(h float64) {
	this.rowHeight = h
}

func (this *FormLayout) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标签宽度", this.LabelWidth, this.SetLabelWidth)
	list.AddProperty("间距", this.Spacing, this.SetSpacing)
	list.AddProperty("行高", this.RowHeight, this.SetRowHeight)
}

func (this *FormLayout) SetPadding(p Padding) {
	this.padding = p
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

func (this *FormLayout) Layout() {
	n := len(this.rows)
	if n == 0 {
		return
	}

	w, h := this.Self().Size()
	x0, y0, w, h := this.padding.Apply(0, 0, w, h)

	labelW := this.labelWidth
	widgetX := x0 + labelW + this.spacing
	widgetW := w - labelW - this.spacing
	if widgetW < 0 {
		widgetW = 0
	}

	// Determine row heights
	heights := make([]float64, n)
	var fixedTotal float64
	var flexCount int

	for i, row := range this.rows {
		if this.rowHeight > 0 {
			heights[i] = this.rowHeight
			fixedTotal += this.rowHeight
		} else {
			// Use the larger of label/widget size hints
			lh := row.label.SizeHints().Height
			wh := row.widget.SizeHints().Height
			rh := math.Max(lh, wh)
			if rh > 0 {
				heights[i] = rh
				fixedTotal += rh
			} else {
				flexCount++
			}
		}
	}

	spacingTotal := float64(n-1) * this.spacing
	remaining := h - fixedTotal - spacingTotal
	var flexH float64
	if flexCount > 0 && remaining > 0 {
		flexH = remaining / float64(flexCount)
	}

	yOff := y0
	for i, row := range this.rows {
		rh := heights[i]
		if rh == 0 {
			rh = flexH
		}

		// Label: right-aligned in the label column
		row.label.SetBounds(x0, yOff, labelW, rh)

		// Widget: fills the remaining width
		row.widget.SetBounds(widgetX, yOff, widgetW, rh)

		yOff += rh + this.spacing
	}
}

func (this *FormLayout) Draw(g paint.Painter) {
	// Children are drawn by the framework after this method returns.
}

func (this *FormLayout) SizeHints() SizeHints {
	n := len(this.rows)
	if n == 0 {
		return SizeHints{}
	}

	var totalH, maxWidgetW float64
	for _, row := range this.rows {
		wh := row.widget.SizeHints()
		lh := row.label.SizeHints()
		rh := math.Max(lh.Height, wh.Height)
		if this.rowHeight > 0 {
			rh = this.rowHeight
		}
		totalH += rh
		maxWidgetW = math.Max(maxWidgetW, wh.Width)
	}

	totalH += float64(n-1) * this.spacing
	totalH += this.padding.T + this.padding.B
	totalW := this.labelWidth + this.spacing + maxWidgetW + this.padding.L + this.padding.R

	return SizeHints{Width: totalW, Height: totalH, Policy: GrowHorizontal | GrowVertical}
}
