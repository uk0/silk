package gui

import (
	"sort"
	"strconv"
	"strings"

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

// SortableTableModel is an optional capability for models that support
// click-to-sort on column headers. SortByColumn reorders the rows by the
// given column; RestoreOrder reverts to the original insertion order.
type SortableTableModel interface {
	SortByColumn(col int, ascending bool)
	RestoreOrder()
}

// columnIsNumeric reports whether every non-empty cell in the given column
// parses as a float64. Empty cells are skipped; a column with no parseable
// non-empty cells is treated as non-numeric.
func columnIsNumeric(rows [][]string, col int) bool {
	seen := false
	for _, r := range rows {
		if col < 0 || col >= len(r) {
			continue
		}
		s := strings.TrimSpace(r[col])
		if s == "" {
			continue
		}
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			return false
		}
		seen = true
	}
	return seen
}

// compareTableCells compares two cell strings case-insensitively, returning
// -1, 0, or 1. Used as the string fallback when a column is not numeric.
func compareTableCells(a, b string) int {
	la := strings.ToLower(a)
	lb := strings.ToLower(b)
	if la < lb {
		return -1
	}
	if la > lb {
		return 1
	}
	return 0
}

// compareTableCellsNumeric compares two cell strings as float64, returning
// -1, 0, or 1. An unparseable or empty cell sorts before any number.
func compareTableCellsNumeric(a, b string) int {
	fa, ea := strconv.ParseFloat(strings.TrimSpace(a), 64)
	fb, eb := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if ea != nil || eb != nil {
		switch {
		case ea != nil && eb != nil:
			return 0
		case ea != nil:
			return -1
		default:
			return 1
		}
	}
	if fa < fb {
		return -1
	}
	if fa > fb {
		return 1
	}
	return 0
}

// sortRowsByColumn stably sorts rows in place by the given column. If every
// non-empty cell in the column parses as a number the comparison is numeric,
// otherwise it is a case-insensitive string compare. The sort is stable so
// rows that compare equal keep their relative order.
func sortRowsByColumn(rows [][]string, col int, ascending bool) {
	numeric := columnIsNumeric(rows, col)
	cell := func(r []string) string {
		if col < 0 || col >= len(r) {
			return ""
		}
		return r[col]
	}
	sort.SliceStable(rows, func(i, j int) bool {
		var c int
		if numeric {
			c = compareTableCellsNumeric(cell(rows[i]), cell(rows[j]))
		} else {
			c = compareTableCells(cell(rows[i]), cell(rows[j]))
		}
		if !ascending {
			c = -c
		}
		return c < 0
	})
}

// SimpleTableModel is a basic in-memory TableModel implementation.
type SimpleTableModel struct {
	headers  []string
	widths   []float64
	rows     [][]string
	original [][]string // insertion-order snapshot captured on first sort
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
	m.invalidateOrder()
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
	m.invalidateOrder()
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

// snapshotOrder captures the current row order so RestoreOrder can revert to
// it. It is taken only once, on the first sort after the rows last changed.
func (m *SimpleTableModel) snapshotOrder() {
	if m.original != nil {
		return
	}
	m.original = make([][]string, len(m.rows))
	copy(m.original, m.rows)
}

// invalidateOrder drops the insertion-order snapshot; called whenever the row
// set changes so the next sort re-snapshots from the new contents.
func (m *SimpleTableModel) invalidateOrder() {
	m.original = nil
}

// SortByColumn reorders the rows by the given column. Numeric columns are
// compared as numbers, others case-insensitively as strings. The first sort
// captures the insertion order so RestoreOrder can revert to it.
func (m *SimpleTableModel) SortByColumn(col int, ascending bool) {
	if col < 0 || col >= len(m.headers) {
		return
	}
	m.snapshotOrder()
	sortRowsByColumn(m.rows, col, ascending)
}

// RestoreOrder reverts the rows to the original insertion order captured
// before the first sort.
func (m *SimpleTableModel) RestoreOrder() {
	if m.original == nil {
		return
	}
	m.rows = m.original
	m.original = nil
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
	sortColumn         int       // currently sorted column, -1 when unsorted
	sortAscending      bool      // direction of the current sort
	colWidths          []float64 // per-column override widths, lazily seeded from the model
	resizeCol          int       // column being resized via a header-boundary drag, -1 when idle
	resizeStartX       float64   // pointer x at drag start (widget space)
	resizeStartW       float64   // width of resizeCol at drag start
	hoverBoundary      int       // column boundary currently hovered, -1 when none
	cbSelectionChanged func(interface{}, int)
	cbSortChanged      func(col int, ascending bool)
}

// columnResizeGrab is the half-width, in pixels, of the grab zone around a
// column boundary that starts a resize drag instead of a sort.
const columnResizeGrab = 4.0

// columnMinWidth is the smallest width a column may be dragged down to.
const columnMinWidth = 20.0

// NewTable creates a new Table widget.
func NewTable() *Table {
	p := new(Table)
	p.Init(p)
	p.selectedRow = -1
	p.sortColumn = -1
	p.resizeCol = -1
	p.hoverBoundary = -1
	p.scrollArea = NewScrollArea()
	p.scrollArea.SetParent(p)
	return p
}

// SetModel sets the TableModel that provides data for this table.
func (this *Table) SetModel(m TableModel) {
	this.model = m
	this.sortColumn = -1
	this.colWidths = nil // reseed widths from the new model on next use
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

// SigSortChanged sets the callback invoked when the sort column or direction
// changes. col is the sorted column, or -1 when the table returns to its
// original unsorted order.
func (this *Table) SigSortChanged(fn func(col int, ascending bool)) {
	this.cbSortChanged = fn
}

// SortColumn returns the currently sorted column index, or -1 when unsorted.
func (this *Table) SortColumn() int {
	return this.sortColumn
}

// SortAscending reports the direction of the current sort.
func (this *Table) SortAscending() bool {
	return this.sortAscending
}

// sortByColumn cycles the sort state for the given column: an unsorted column
// sorts ascending, the active column toggles to descending, and a descending
// column clears back to the original insertion order. It requires the model to
// implement SortableTableModel; otherwise it is a no-op. This is the entry
// point used by the header-click handler.
func (this *Table) sortByColumn(col int) {
	if this.model == nil {
		return
	}
	sm, ok := this.model.(SortableTableModel)
	if !ok {
		return
	}
	if col < 0 || col >= this.model.ColumnCount() {
		return
	}

	switch {
	case this.sortColumn != col:
		this.sortColumn = col
		this.sortAscending = true
		sm.SortByColumn(col, true)
	case this.sortAscending:
		this.sortAscending = false
		sm.SortByColumn(col, false)
	default:
		// third click on the same column: restore original order
		this.sortColumn = -1
		this.sortAscending = false
		sm.RestoreOrder()
	}

	if this.cbSortChanged != nil {
		this.cbSortChanged(this.sortColumn, this.sortAscending)
	}
	this.Update()
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

// ensureColWidths lazily seeds the per-column width overrides from the model
// the first time a width is needed (or after the column count changes), so a
// resize drag has somewhere to persist without mutating the model.
func (this *Table) ensureColWidths() {
	if this.model == nil {
		this.colWidths = nil
		return
	}
	n := this.model.ColumnCount()
	if len(this.colWidths) == n {
		return
	}
	this.colWidths = make([]float64, n)
	for c := 0; c < n; c++ {
		this.colWidths[c] = this.model.ColumnWidth(c)
	}
}

// columnWidth returns the effective width of a column: the resized override if
// one exists, otherwise the model's width. All drawing and hit-testing reads
// widths through here so a resize reflows the table.
func (this *Table) columnWidth(col int) float64 {
	this.ensureColWidths()
	if col < 0 || col >= len(this.colWidths) {
		if this.model != nil {
			return this.model.ColumnWidth(col)
		}
		return 0
	}
	return this.colWidths[col]
}

// setColumnWidth records a new width for a column, clamped to the minimum.
func (this *Table) setColumnWidth(col int, w float64) {
	this.ensureColWidths()
	if col < 0 || col >= len(this.colWidths) {
		return
	}
	if w < columnMinWidth {
		w = columnMinWidth
	}
	this.colWidths[col] = w
}

func (this *Table) totalColumnWidth() float64 {
	if this.model == nil {
		return 0
	}
	total := 0.0
	for c := 0; c < this.model.ColumnCount(); c++ {
		total += this.columnWidth(c)
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
		cw := this.columnWidth(c)
		// Header background
		t.ButtonPushedFace.Draw(g, cw, hh)
		// Header text
		g.SetBrush1(t.TextColor)
		txt := this.model.HeaderText(c)
		yt := fe.Ascent + (hh-fe.Height)*0.5
		g.DrawText1(xPos+4, yt, txt)
		// Sort indicator arrow on the active column
		if c == this.sortColumn {
			this.drawSortArrow(g, xPos+cw-12, hh*0.5, this.sortAscending)
		}
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
			cw := this.columnWidth(c)
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

// drawSortArrow draws a small triangle indicator centered at (cx, cy):
// pointing up for ascending order, down for descending.
func (this *Table) drawSortArrow(g paint.Painter, cx, cy float64, ascending bool) {
	t := Theme()
	s := 4.0
	g.Save()
	if ascending {
		// up arrow
		g.MoveTo(cx-s, cy+s/2)
		g.LineTo(cx, cy-s/2)
		g.LineTo(cx+s, cy+s/2)
	} else {
		// down arrow
		g.MoveTo(cx-s, cy-s/2)
		g.LineTo(cx, cy+s/2)
		g.LineTo(cx+s, cy-s/2)
	}
	g.SetPen1(t.FormDarkColor, 1.5)
	g.Stroke()
	g.Restore()
}

// headerColumnAt returns the column index under the given widget-space x
// coordinate, accounting for horizontal scroll, or -1 if past the last column.
// It mirrors the column layout used when drawing the header.
func (this *Table) headerColumnAt(x float64) int {
	if this.model == nil {
		return -1
	}
	mx := x + this.scrollArea.ScrollX()
	xPos := 0.0
	for c := 0; c < this.model.ColumnCount(); c++ {
		cw := this.columnWidth(c)
		if mx >= xPos && mx < xPos+cw {
			return c
		}
		xPos += cw
	}
	return -1
}

// columnBoundaryAt returns the index of the column whose right boundary sits
// within columnResizeGrab pixels of the given widget-space x, or -1 if x is in
// a column body. Horizontal scroll is accounted for so the grab zone tracks the
// visible boundary. This is the hit-test that distinguishes a resize from a
// header-click sort.
func (this *Table) columnBoundaryAt(x float64) int {
	if this.model == nil {
		return -1
	}
	mx := x + this.scrollArea.ScrollX()
	xPos := 0.0
	for c := 0; c < this.model.ColumnCount(); c++ {
		xPos += this.columnWidth(c)
		if mx >= xPos-columnResizeGrab && mx <= xPos+columnResizeGrab {
			return c
		}
	}
	return -1
}

// OnLeftDown handles mouse clicks for row selection.
func (this *Table) OnLeftDown(x, y float64) {
	this.SetFocus()
	if this.model == nil {
		return
	}

	hh := this.HeaderHeight()
	if y < hh {
		// A click on a column boundary starts a resize drag; anything else in
		// the header body falls through to the sort handler.
		if col := this.columnBoundaryAt(x); col >= 0 {
			this.resizeCol = col
			this.resizeStartX = x
			this.resizeStartW = this.columnWidth(col)
			this.PushCapture()
			SetOverrideCursor(cursorSizeWE)
			return
		}
		// Clicked on a header cell: sort by that column.
		col := this.headerColumnAt(x)
		if col >= 0 {
			this.sortByColumn(col)
		}
		return
	}

	rh := this.RowHeight()
	sy := this.scrollArea.ScrollY()
	row := int((y-hh)/rh + sy)
	if row >= 0 && row < this.model.RowCount() {
		this.SetSelectedRow(row)
	}
}

// OnMouseMove drives an in-progress column resize and, when idle, shows a
// horizontal-resize cursor while hovering a header boundary.
func (this *Table) OnMouseMove(x, y float64) {
	if this.resizeCol >= 0 {
		this.setColumnWidth(this.resizeCol, this.resizeStartW+(x-this.resizeStartX))
		this.Layout()
		this.Update()
		return
	}

	// Hover affordance: a resize cursor over a boundary inside the header.
	onBoundary := -1
	if this.model != nil && y < this.HeaderHeight() {
		onBoundary = this.columnBoundaryAt(x)
	}
	if onBoundary != this.hoverBoundary {
		this.hoverBoundary = onBoundary
		if onBoundary >= 0 {
			SetOverrideCursor(cursorSizeWE)
		} else {
			SetOverrideCursor(nil)
		}
	}
}

// OnLeftUp ends a column resize drag.
func (this *Table) OnLeftUp(x, y float64) {
	if this.resizeCol >= 0 {
		this.resizeCol = -1
		this.PopCapture()
		SetOverrideCursor(nil)
	}
}

// OnMouseLeave clears the resize cursor affordance when not actively dragging.
func (this *Table) OnMouseLeave() {
	if this.resizeCol < 0 && this.hoverBoundary != -1 {
		this.hoverBoundary = -1
		SetOverrideCursor(nil)
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
