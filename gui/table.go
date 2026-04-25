package gui

import (
	"silk/core"
	"silk/paint"
)

func init() {
	core.RegisterFactory("gui.Table", core.TypeOf((*Table)(nil)))
}

// TableModel is the data interface for Table widgets.
type TableModel interface {
	RowCount() int
	ColumnCount() int
	CellText(row, col int) string
	HeaderText(col int) string
	ColumnWidth(col int) float64
}

// SimpleTableModel is a basic in-memory TableModel implementation.
type SimpleTableModel struct {
	headers []string
	widths  []float64
	rows    [][]string
}

// NewSimpleTableModel creates a new SimpleTableModel with the given column headers.
func NewSimpleTableModel(headers []string) *SimpleTableModel {
	m := &SimpleTableModel{
		headers: make([]string, len(headers)),
		widths:  make([]float64, len(headers)),
	}
	copy(m.headers, headers)
	for i := range m.widths {
		m.widths[i] = 120
	}
	return m
}

func (m *SimpleTableModel) RowCount() int {
	return len(m.rows)
}

func (m *SimpleTableModel) ColumnCount() int {
	return len(m.headers)
}

func (m *SimpleTableModel) CellText(row, col int) string {
	if row < 0 || row >= len(m.rows) {
		return ""
	}
	r := m.rows[row]
	if col < 0 || col >= len(r) {
		return ""
	}
	return r[col]
}

func (m *SimpleTableModel) HeaderText(col int) string {
	if col < 0 || col >= len(m.headers) {
		return ""
	}
	return m.headers[col]
}

func (m *SimpleTableModel) ColumnWidth(col int) float64 {
	if col < 0 || col >= len(m.widths) {
		return 120
	}
	return m.widths[col]
}

// SetColumnWidth sets the width of a specific column.
func (m *SimpleTableModel) SetColumnWidth(col int, width float64) {
	if col >= 0 && col < len(m.widths) {
		m.widths[col] = width
	}
}

// AddRow appends a new row of cell values.
func (m *SimpleTableModel) AddRow(cells ...string) {
	row := make([]string, m.ColumnCount())
	for i := 0; i < len(cells) && i < len(row); i++ {
		row[i] = cells[i]
	}
	m.rows = append(m.rows, row)
}

// SetRow replaces the cell values at the given row index.
func (m *SimpleTableModel) SetRow(row int, cells ...string) {
	if row < 0 || row >= len(m.rows) {
		return
	}
	r := m.rows[row]
	for i := 0; i < len(cells) && i < len(r); i++ {
		r[i] = cells[i]
	}
}

// RemoveRow removes the row at the given index.
func (m *SimpleTableModel) RemoveRow(row int) {
	if row < 0 || row >= len(m.rows) {
		return
	}
	m.rows[row] = nil
	m.rows = append(m.rows[:row], m.rows[row+1:]...)
}

// RowData returns a copy of the cells at the given row.
func (m *SimpleTableModel) RowData(row int) []string {
	if row < 0 || row >= len(m.rows) {
		return nil
	}
	ret := make([]string, len(m.rows[row]))
	copy(ret, m.rows[row])
	return ret
}

// Table is a tabular data display widget with a fixed header row,
// scrollable body, row selection, and alternating row backgrounds.
type Table struct {
	Widget
	model              TableModel
	scrollArea         *ScrollArea
	selectedRow        int
	rowHeight          float64
	headerHeight       float64
	cbSelectionChanged func(interface{}, int)
}

// NewTable creates a new Table widget.
func NewTable() *Table {
	p := new(Table)
	p.Init(p)
	p.selectedRow = -1
	p.scrollArea = NewScrollArea()
	p.scrollArea.SetParent(p)
	return p
}

// SetModel sets the TableModel that provides data for this table.
func (this *Table) SetModel(m TableModel) {
	this.model = m
	this.updateScroll()
	this.Layout()
}

// Model returns the current TableModel.
func (this *Table) Model() TableModel {
	return this.model
}

// SelectedRow returns the index of the currently selected row, or -1 if none.
func (this *Table) SelectedRow() int {
	return this.selectedRow
}

// SetSelectedRow sets the selected row index.
func (this *Table) SetSelectedRow(row int) {
	if this.model == nil {
		this.selectedRow = -1
		return
	}
	if row < -1 {
		row = -1
	}
	if row >= this.model.RowCount() {
		row = this.model.RowCount() - 1
	}
	old := this.selectedRow
	this.selectedRow = row
	if old != row && this.cbSelectionChanged != nil {
		this.cbSelectionChanged(this.Self(), row)
	}
	this.Update()
}

// SetSelectionChangedCallback sets a callback invoked when the selected row changes.
func (this *Table) SetSelectionChangedCallback(cb func(interface{}, int)) {
	this.cbSelectionChanged = cb
}

// RowHeight returns the height of each data row.
func (this *Table) RowHeight() float64 {
	if this.rowHeight <= 0 {
		return Theme().ItemHeight
	}
	return this.rowHeight
}

// SetRowHeight sets the row height for data rows.
func (this *Table) SetRowHeight(rh float64) {
	this.rowHeight = rh
	this.Layout()
}

// HeaderHeight returns the height of the header row.
func (this *Table) HeaderHeight() float64 {
	if this.headerHeight <= 0 {
		return Theme().ItemHeight + 2
	}
	return this.headerHeight
}

// SetHeaderHeight sets the height of the header row.
func (this *Table) SetHeaderHeight(h float64) {
	this.headerHeight = h
	this.Layout()
}

func (this *Table) totalColumnWidth() float64 {
	if this.model == nil {
		return 0
	}
	total := 0.0
	for c := 0; c < this.model.ColumnCount(); c++ {
		total += this.model.ColumnWidth(c)
	}
	return total
}

func (this *Table) updateScroll() {
	if this.model == nil {
		return
	}
	rh := this.RowHeight()
	hh := this.HeaderHeight()
	bodyH := this.h - hh
	totalRows := float64(this.model.RowCount())
	visibleRows := bodyH / rh

	vs := this.scrollArea.VertScrollBar()
	if totalRows > visibleRows {
		vs.SetRange(0, totalRows-visibleRows)
		vs.SetDelta(1, visibleRows)
		vs.SetVisible(true)
	} else {
		vs.SetRange(0, 0)
		vs.SetVisible(false)
	}

	totalW := this.totalColumnWidth()
	sw := Theme().ScrollWidth
	viewW := this.w
	if vs.IsVisible() {
		viewW -= sw
	}
	hs := this.scrollArea.HorzScrollBar()
	if totalW > viewW {
		hs.SetRange(0, totalW-viewW)
		hs.SetDelta(20, viewW)
		hs.SetVisible(true)
	} else {
		hs.SetRange(0, 0)
		hs.SetVisible(false)
	}
}

// Layout arranges the header, scroll area, and scroll bars.
func (this *Table) Layout() {
	this.updateScroll()
	this.scrollArea.SetBounds(0, 0, this.w, this.h)
	this.scrollArea.Layout()
	this.Update()
}

// Draw renders the table: header row, data rows with alternating backgrounds,
// and selection highlight.
func (this *Table) Draw(g paint.Painter) {
	t := Theme()
	rh := this.RowHeight()
	hh := this.HeaderHeight()

	// Background
	g.Rectangle(0, 0, this.w, this.h)
	g.SetBrush1(t.ViewBGColor)
	g.Fill()

	if this.model == nil {
		t.DrawViewFrame(g, 0, 0, this.w, this.h)
		return
	}

	sw := t.ScrollWidth
	vs := this.scrollArea.VertScrollBar()
	hs := this.scrollArea.HorzScrollBar()

	viewW := this.w
	if vs != nil && vs.IsVisible() {
		viewW -= sw
	}
	viewH := this.h
	if hs != nil && hs.IsVisible() {
		viewH -= sw
	}

	sx := this.scrollArea.ScrollX()
	sy := this.scrollArea.ScrollY()

	colCount := this.model.ColumnCount()
	rowCount := this.model.RowCount()

	font := t.Font
	g.SetFont(font)
	fe := font.FontExtents()

	// Clip to viewport
	g.Save()
	g.Rectangle(0, 0, viewW, viewH)
	g.Clip()

	// Draw header
	g.Save()
	g.Translate(-sx, 0)
	xPos := 0.0
	for c := 0; c < colCount; c++ {
		cw := this.model.ColumnWidth(c)
		// Header background
		t.ButtonPushedFace.Draw(g, cw, hh)
		// Header text
		g.SetBrush1(t.TextColor)
		txt := this.model.HeaderText(c)
		yt := fe.Ascent + (hh-fe.Height)*0.5
		g.DrawText1(xPos+4, yt, txt)
		xPos += cw
	}
	g.Restore()

	// Draw data rows
	g.Save()
	g.Rectangle(0, hh, viewW, viewH-hh)
	g.Clip()
	g.Translate(-sx, hh)

	scrollRow := int(sy)
	visibleCount := int((viewH-hh)/rh) + 2
	endRow := scrollRow + visibleCount
	if endRow > rowCount {
		endRow = rowCount
	}

	fractionalOffset := (sy - float64(scrollRow)) * rh

	for r := scrollRow; r < endRow; r++ {
		y := float64(r-scrollRow)*rh - fractionalOffset

		// Alternating row background
		if r%2 == 1 {
			altColor := paint.Color{245, 245, 250, 255}
			g.SetBrush1(altColor)
			g.Rectangle(0, y, this.totalColumnWidth(), rh)
			g.Fill()
		}

		// Selection highlight
		if r == this.selectedRow {
			g.SetBrush1(t.HighLightColor)
			g.Rectangle(0, y, this.totalColumnWidth(), rh)
			g.Fill()
		}

		// Cell text
		xPos := 0.0
		for c := 0; c < colCount; c++ {
			cw := this.model.ColumnWidth(c)
			txt := this.model.CellText(r, c)
			if txt != "" {
				g.SetBrush1(t.TextColor)
				yt := y + fe.Ascent + (rh-fe.Height)*0.5
				g.DrawText1(xPos+4, yt, txt)
			}
			xPos += cw
		}
	}
	g.Restore()

	// Restore from viewport clip
	g.Restore()

	// Draw scroll bars on top
	if vs != nil && vs.IsVisible() {
		g.Save()
		vx, vy, vw, vh := vs.Bounds()
		g.Translate(vx, vy)
		vs.Draw(g)
		_ = vw
		_ = vh
		g.Translate(-vx, -vy)
		g.Restore()
	}
	if hs != nil && hs.IsVisible() {
		g.Save()
		hx, hy, hw, hh2 := hs.Bounds()
		g.Translate(hx, hy)
		hs.Draw(g)
		_ = hw
		_ = hh2
		g.Translate(-hx, -hy)
		g.Restore()
	}

	// Frame border
	t.DrawViewFrame(g, 0, 0, this.w, this.h)
}

// OnLeftDown handles mouse clicks for row selection.
func (this *Table) OnLeftDown(x, y float64) {
	this.SetFocus()
	if this.model == nil {
		return
	}

	hh := this.HeaderHeight()
	if y < hh {
		// Clicked on header, ignore for now
		return
	}

	rh := this.RowHeight()
	sy := this.scrollArea.ScrollY()
	row := int((y-hh)/rh + sy)
	if row >= 0 && row < this.model.RowCount() {
		this.SetSelectedRow(row)
	}
}

// OnMouseWheel handles mouse wheel scrolling.
func (this *Table) OnMouseWheel(x, y, z float64) {
	vs := this.scrollArea.VertScrollBar()
	if vs != nil && vs.IsVisible() {
		if z > 0 {
			vs.SmallBakward()
		} else if z < 0 {
			vs.SmallForward()
		}
		this.Update()
	}
}

func (this *Table) EnumProperties(list core.IPropertyList) {
	list.AddProperty("行高", this.RowHeight, this.SetRowHeight)
	list.AddProperty("选中行", this.SelectedRow, this.SetSelectedRow)
}

// SizeHints returns the preferred size for the table.
func (this *Table) SizeHints() SizeHints {
	w := 200.0
	h := 150.0
	if this.model != nil {
		tw := this.totalColumnWidth()
		if tw > w {
			w = tw
		}
		th := this.HeaderHeight() + this.RowHeight()*float64(this.model.RowCount())
		if th > h {
			h = th
		}
	}
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}
