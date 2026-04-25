package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.GridLayout", core.TypeOf((*GridLayout)(nil)))
}

// GridCell represents a widget placed in the grid at a specific row/column with optional span.
type GridCell struct {
	widget             IWidget
	row, col           int
	rowSpan, colSpan   int
}

// GridLayout is a layout container that arranges children in a grid (similar to QGridLayout).
type GridLayout struct {
	Widget
	cells      []GridCell
	colWidths  []float64 // fixed column widths (0 = flexible)
	rowHeights []float64 // fixed row heights (0 = flexible)
	spacing    float64
	padding    Padding
}

func NewGridLayout() *GridLayout {
	p := new(GridLayout)
	p.Init(p)
	return p
}

func (this *GridLayout) Spacing() float64 {
	return this.spacing
}

func (this *GridLayout) SetSpacing(s float64) {
	this.spacing = s
}

func (this *GridLayout) EnumProperties(list core.IPropertyList) {
	list.AddProperty("间距", this.Spacing, this.SetSpacing)
}

func (this *GridLayout) SetPadding(p Padding) {
	this.padding = p
}

// AddWidget places a widget at the given row and column with span of 1x1.
func (this *GridLayout) AddWidget(w IWidget, row, col int) {
	this.AddWidgetSpan(w, row, col, 1, 1)
}

// AddWidgetSpan places a widget at the given row and column with the specified row/col span.
func (this *GridLayout) AddWidgetSpan(w IWidget, row, col, rowSpan, colSpan int) {
	if rowSpan < 1 {
		rowSpan = 1
	}
	if colSpan < 1 {
		colSpan = 1
	}
	this.cells = append(this.cells, GridCell{
		widget:  w,
		row:     row,
		col:     col,
		rowSpan: rowSpan,
		colSpan: colSpan,
	})
	w.SetParent(this.Self())
}

// SetColumnWidth sets a fixed width for a column. Pass 0 for flexible sizing.
func (this *GridLayout) SetColumnWidth(col int, width float64) {
	for col >= len(this.colWidths) {
		this.colWidths = append(this.colWidths, 0)
	}
	this.colWidths[col] = width
}

// SetRowHeight sets a fixed height for a row. Pass 0 for flexible sizing.
func (this *GridLayout) SetRowHeight(row int, height float64) {
	for row >= len(this.rowHeights) {
		this.rowHeights = append(this.rowHeights, 0)
	}
	this.rowHeights[row] = height
}

// gridDimensions returns the number of rows and columns needed.
func (this *GridLayout) gridDimensions() (maxRows, maxCols int) {
	for _, c := range this.cells {
		endRow := c.row + c.rowSpan
		endCol := c.col + c.colSpan
		if endRow > maxRows {
			maxRows = endRow
		}
		if endCol > maxCols {
			maxCols = endCol
		}
	}
	return
}

func (this *GridLayout) Layout() {
	if len(this.cells) == 0 {
		return
	}

	w, h := this.Self().Size()
	x0, y0, w, h := this.padding.Apply(0, 0, w, h)

	maxRows, maxCols := this.gridDimensions()
	if maxRows == 0 || maxCols == 0 {
		return
	}

	// Resolve column widths
	colW := make([]float64, maxCols)
	var fixedW float64
	var flexCols int
	for i := 0; i < maxCols; i++ {
		if i < len(this.colWidths) && this.colWidths[i] > 0 {
			colW[i] = this.colWidths[i]
			fixedW += colW[i]
		} else {
			flexCols++
		}
	}
	spacingW := float64(maxCols-1) * this.spacing
	remainW := w - fixedW - spacingW
	if remainW < 0 {
		remainW = 0
	}
	var flexColW float64
	if flexCols > 0 && remainW > 0 {
		flexColW = remainW / float64(flexCols)
	}
	if flexColW < 0 {
		flexColW = 0
	}
	for i := 0; i < maxCols; i++ {
		if colW[i] == 0 {
			colW[i] = flexColW
		}
	}

	// Resolve row heights
	rowH := make([]float64, maxRows)
	var fixedH float64
	var flexRows int
	for i := 0; i < maxRows; i++ {
		if i < len(this.rowHeights) && this.rowHeights[i] > 0 {
			rowH[i] = this.rowHeights[i]
			fixedH += rowH[i]
		} else {
			flexRows++
		}
	}
	spacingH := float64(maxRows-1) * this.spacing
	remainH := h - fixedH - spacingH
	if remainH < 0 {
		remainH = 0
	}
	var flexRowH float64
	if flexRows > 0 && remainH > 0 {
		flexRowH = remainH / float64(flexRows)
	}
	if flexRowH < 0 {
		flexRowH = 0
	}
	for i := 0; i < maxRows; i++ {
		if rowH[i] == 0 {
			rowH[i] = flexRowH
		}
	}

	// Compute cumulative X positions for columns
	colX := make([]float64, maxCols)
	colX[0] = x0
	for i := 1; i < maxCols; i++ {
		colX[i] = colX[i-1] + colW[i-1] + this.spacing
	}

	// Compute cumulative Y positions for rows
	rowY := make([]float64, maxRows)
	rowY[0] = y0
	for i := 1; i < maxRows; i++ {
		rowY[i] = rowY[i-1] + rowH[i-1] + this.spacing
	}

	// Position each widget (skip hidden)
	for _, c := range this.cells {
		if !c.widget.IsVisible() {
			continue
		}
		cx := colX[c.col]
		cy := rowY[c.row]

		// Sum spanned column widths
		var cw float64
		for j := c.col; j < c.col+c.colSpan && j < maxCols; j++ {
			cw += colW[j]
			if j > c.col {
				cw += this.spacing
			}
		}

		// Sum spanned row heights
		var ch float64
		for j := c.row; j < c.row+c.rowSpan && j < maxRows; j++ {
			ch += rowH[j]
			if j > c.row {
				ch += this.spacing
			}
		}

		c.widget.SetBounds(cx, cy, cw, ch)
	}
}

func (this *GridLayout) Draw(g paint.Painter) {
	// Children are drawn by the framework after this method returns.
}

func (this *GridLayout) SizeHints() SizeHints {
	if len(this.cells) == 0 {
		return SizeHints{}
	}

	maxRows, maxCols := this.gridDimensions()

	// For each column, find max child width; for each row, find max child height
	colW := make([]float64, maxCols)
	rowH := make([]float64, maxRows)

	for _, c := range this.cells {
		hi := c.widget.SizeHints()
		cw := hi.Width / float64(c.colSpan)
		ch := hi.Height / float64(c.rowSpan)
		for j := c.col; j < c.col+c.colSpan && j < maxCols; j++ {
			colW[j] = math.Max(colW[j], cw)
		}
		for j := c.row; j < c.row+c.rowSpan && j < maxRows; j++ {
			rowH[j] = math.Max(rowH[j], ch)
		}
	}

	// Override with fixed sizes where specified
	for i := 0; i < maxCols; i++ {
		if i < len(this.colWidths) && this.colWidths[i] > 0 {
			colW[i] = this.colWidths[i]
		}
	}
	for i := 0; i < maxRows; i++ {
		if i < len(this.rowHeights) && this.rowHeights[i] > 0 {
			rowH[i] = this.rowHeights[i]
		}
	}

	var totalW, totalH float64
	for _, w := range colW {
		totalW += w
	}
	for _, h := range rowH {
		totalH += h
	}

	totalW += float64(maxCols-1)*this.spacing + this.padding.L + this.padding.R
	totalH += float64(maxRows-1)*this.spacing + this.padding.T + this.padding.B

	return SizeHints{Width: totalW, Height: totalH, Policy: GrowHorizontal | GrowVertical}
}
