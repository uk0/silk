package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.GridLayout", core.TypeOf((*GridLayout)(nil)))
}

// GridCell represents a widget placed in the grid at a specific row/column with optional span.
type GridCell struct {
	widget           IWidget
	row, col         int
	rowSpan, colSpan int
}

// GridLayout is a layout container that arranges children in a grid (similar to QGridLayout).
type GridLayout struct {
	layoutDisabled bool
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
	this.spacing = layoutDimension(s)
	this.InvalidateLayout()
}

func (this *GridLayout) EnumProperties(list core.IPropertyList) {
	list.AddProperty("间距", this.Spacing, this.SetSpacing)
	enumLayoutPadding(list, this.Padding, this.SetPadding)
}

func (this *GridLayout) Padding() Padding { return this.padding }

func (this *GridLayout) SetPadding(p Padding) {
	this.padding = p
	this.InvalidateLayout()
}

// AddWidget places a widget at the given row and column with span of 1x1.
func (this *GridLayout) AddWidget(w IWidget, row, col int) {
	this.AddWidgetSpan(w, row, col, 1, 1)
}

// AddWidgetSpan places a widget at the given row and column with the specified row/col span.
func (this *GridLayout) AddWidgetSpan(w IWidget, row, col, rowSpan, colSpan int) {
	if w == nil || row < 0 || col < 0 || row > 4095 || col > 4095 {
		return
	}
	if rowSpan < 1 {
		rowSpan = 1
	}
	if colSpan < 1 {
		colSpan = 1
	}
	rowSpan, colSpan = min(rowSpan, 4096-row), min(colSpan, 4096-col)
	for i := range this.cells {
		if this.cells[i].widget == w {
			this.cells[i] = GridCell{widget: w, row: row, col: col, rowSpan: rowSpan, colSpan: colSpan}
			w.SetParent(this.Self())
			this.RequestLayout() // same-parent metadata changes still need layout
			return
		}
	}
	this.cells = append(this.cells, GridCell{
		widget:  w,
		row:     row,
		col:     col,
		rowSpan: rowSpan,
		colSpan: colSpan,
	})
	w.SetParent(this.Self())
	this.RequestLayout()
}

// AddLayoutWidget is the designer/runtime bridge preserving row and spans.
func (this *GridLayout) AddLayoutWidget(w IWidget, row, col, rowSpan, colSpan int) {
	if w == nil || row < 0 || col < 0 || row > 4095 || col > 4095 {
		return
	}
	if rowSpan > 4096-row {
		rowSpan = 4096 - row
	}
	if colSpan > 4096-col {
		colSpan = 4096 - col
	}
	this.AddWidgetSpan(w, row, col, rowSpan, colSpan)
}

// SetColumnWidth sets a fixed width for a column. Pass 0 for flexible sizing.
func (this *GridLayout) SetColumnWidth(col int, width float64) {
	if col < 0 || col > 4095 {
		return
	}
	for col >= len(this.colWidths) {
		this.colWidths = append(this.colWidths, 0)
	}
	this.colWidths[col] = layoutDimension(width)
	this.InvalidateLayout()
}

// SetRowHeight sets a fixed height for a row. Pass 0 for flexible sizing.
func (this *GridLayout) SetRowHeight(row int, height float64) {
	if row < 0 || row > 4095 {
		return
	}
	for row >= len(this.rowHeights) {
		this.rowHeights = append(this.rowHeights, 0)
	}
	this.rowHeights[row] = layoutDimension(height)
	this.InvalidateLayout()
}

// gridDimensions returns the number of rows and columns needed.
func (this *GridLayout) gridDimensions() (maxRows, maxCols int) {
	for _, c := range this.cells {
		if !this.ownsCell(c) {
			continue
		}
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
	if this.layoutDisabled {
		return
	}
	if len(this.cells) == 0 {
		return
	}

	w, h := this.Self().Size()
	x0, y0, w, h := applyLayoutPadding(this.padding, w, h)

	maxRows, maxCols := this.gridDimensions()
	if maxRows == 0 || maxCols == 0 {
		return
	}

	// Use a pooled scratch buffer for column widths, row heights, and the
	// cumulative offset arrays. This avoids 4 allocations per Layout() call.
	scratch := acquireGridScratch()

	colW := this.measureColumns(w, true, maxCols, scratch.colW)
	scratch.colW = colW
	rowH, _ := this.measureRows(colW, maxRows, scratch.rowH)
	scratch.rowH = rowH
	// Explicit row heights stay fixed. Flexible rows receive their measured
	// minimum first, then share extra space; spanning cells contribute only to
	// flexible tracks, never overwriting authored fixed heights.
	used, flexRows := float64(maxRows-1)*this.spacing, 0
	for i, height := range rowH {
		used += height
		if i >= len(this.rowHeights) || this.rowHeights[i] <= 0 {
			flexRows++
		}
	}
	if extra := h - used; extra > 0 && flexRows > 0 {
		for i := range rowH {
			if i >= len(this.rowHeights) || this.rowHeights[i] <= 0 {
				rowH[i] += extra / float64(flexRows)
			}
		}
	}

	// Compute cumulative X positions for columns (pooled).
	colX := growF64(scratch.colX, maxCols)
	scratch.colX = colX
	colX[0] = x0
	for i := 1; i < maxCols; i++ {
		colX[i] = colX[i-1] + colW[i-1] + this.spacing
	}

	// Compute cumulative Y positions for rows (pooled).
	rowY := growF64(scratch.rowY, maxRows)
	scratch.rowY = rowY
	rowY[0] = y0
	for i := 1; i < maxRows; i++ {
		rowY[i] = rowY[i-1] + rowH[i-1] + this.spacing
	}

	// Position each widget (skip hidden)
	for _, c := range this.cells {
		if !this.activeCell(c) {
			continue
		}
		if c.col >= len(colX) || c.row >= len(rowY) {
			// Earlier Resize callbacks can relocate a later registered cell.
			// Its new coordinates belong to the next requested track solution.
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

	releaseGridScratch(scratch)
}

func (this *GridLayout) Draw(g paint.Painter) {
	// Children are drawn by the framework after this method returns.
}

func (this *GridLayout) SizeHints() SizeHints {
	return this.measureGridWidth(0)
}

// LayoutEnabled defaults to true for ordinary runtime widgets. Designer files
// can explicitly preserve a legacy free-positioned container without having
// a later resize silently rearrange its contents.
func (this *GridLayout) LayoutEnabled() bool { return !this.layoutDisabled }
func (this *GridLayout) SetLayoutEnabled(enabled bool) {
	this.layoutDisabled = !enabled
	this.InvalidateLayout()
}
