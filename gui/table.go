package gui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
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

// EditableTableModel is an optional capability for models that accept cell
// writes. Table's in-place editor commits through SetCellText, so a model
// that does not implement this interface stays read-only even when cell
// editing is enabled on the widget.
type EditableTableModel interface {
	SetCellText(row, col int, text string)
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

// SetCellText writes a single cell, ignoring out-of-range indices. It makes
// SimpleTableModel satisfy EditableTableModel so Table's inline editor can
// commit into it.
func (m *SimpleTableModel) SetCellText(row, col int, text string) {
	if row < 0 || row >= len(m.rows) {
		return
	}
	r := m.rows[row]
	if col < 0 || col >= len(r) {
		return
	}
	r[col] = text
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
	displayOrder       []int     // view-side sort permutation for a non-sortable model; nil = model's own order
	colWidths          []float64 // per-column override widths, lazily seeded from the model
	resizeCol          int       // column being resized via a header-boundary drag, -1 when idle
	resizeStartX       float64   // pointer x at drag start (widget space)
	resizeStartW       float64   // width of resizeCol at drag start
	hoverBoundary      int       // column boundary currently hovered, -1 when none
	cbSelectionChanged func(interface{}, int)
	cbSortChanged      func(col int, ascending bool)
	cbRowActivated     func(interface{}, int) // fired when the current row is activated (Enter/Space)

	// In-place cell editing. Off by default so existing read-only tables
	// are unaffected; enable with SetCellsEditable.
	cellsEditable bool
	editor        *tableCellEditor // lazily created inline editor, reused across cells
	editing       bool             // an edit is currently active
	editRow       int              // cell being edited while editing
	editCol       int              // cell being edited while editing
	cbCellEdited  func(row, col int, newText string)

	// Double-click detection for opening the editor (the table has no
	// framework-level double-click, so it is timed here like file-explorer.go).
	lastClickTime time.Time
	lastClickRow  int
	lastClickCol  int

	// Multi-row selection driven by Ctrl/Shift clicks. selectedRow stays the
	// current row / range anchor; selectedRows holds the extra rows, nil when
	// only a single row is selected (the default).
	selectedRows map[int]bool
	selectAnchor int
}

// columnResizeGrab is the half-width, in pixels, of the grab zone around a
// column boundary that starts a resize drag instead of a sort.
const columnResizeGrab = 4.0

// columnMinWidth is the smallest width a column may be dragged down to.
const columnMinWidth = 20.0

// tableDoubleClickInterval is the window within which two clicks on the same
// cell count as a double-click that opens the inline editor. Matches the
// 400ms used by the file explorer's double-click detection.
const tableDoubleClickInterval = 400 * time.Millisecond

// NewTable creates a new Table widget.
func NewTable() *Table {
	p := new(Table)
	p.Init(p)
	p.selectedRow = -1
	p.sortColumn = -1
	p.resizeCol = -1
	p.hoverBoundary = -1
	p.editRow = -1
	p.editCol = -1
	p.lastClickRow = -1
	p.lastClickCol = -1
	p.selectAnchor = -1
	p.scrollArea = NewScrollArea()
	p.scrollArea.SetParent(p)
	return p
}

// SetModel sets the TableModel that provides data for this table.
func (this *Table) SetModel(m TableModel) {
	this.cancelEdit()
	this.model = m
	this.sortColumn = -1
	this.displayOrder = nil // drop any view-side sort permutation from the old model
	this.colWidths = nil    // reseed widths from the new model on next use
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

// CurrentRow returns the index of the current row, or -1 if none. The table's
// current row and selected row are the same concept, so this mirrors
// SelectedRow; both keyboard navigation and clicks move it.
func (this *Table) CurrentRow() int {
	return this.selectedRow
}

// SetCurrentRow sets the current row, clamped to the valid range. It is an
// alias for SetSelectedRow so the navigation API reads consistently.
func (this *Table) SetCurrentRow(row int) {
	this.SetSelectedRow(row)
}

// SigRowActivated sets the callback invoked when the current row is activated
// (Enter or Space). The argument is the activated row index.
func (this *Table) SigRowActivated(fn func(interface{}, int)) {
	this.cbRowActivated = fn
}

// activateSelected fires the row-activated callback for the current row, if a
// valid row is selected and a callback is set. Shared by Enter and Space.
func (this *Table) activateSelected() {
	if this.cbRowActivated == nil || this.model == nil {
		return
	}
	if this.selectedRow < 0 || this.selectedRow >= this.model.RowCount() {
		return
	}
	this.cbRowActivated(this.Self(), this.selectedRow)
}

// visibleRowsPerPage returns the number of whole data rows that fit in the body
// (below the header), at least 1, used as the PageUp/PageDown step.
func (this *Table) visibleRowsPerPage() int {
	rh := this.RowHeight()
	if rh <= 0 {
		return 1
	}
	n := int((this.h - this.HeaderHeight()) / rh)
	if n < 1 {
		n = 1
	}
	return n
}

// scrollRowIntoView adjusts the vertical scroll (measured in rows, matching
// updateScroll) so row r is visible. If r is above the viewport it becomes the
// top row; if below, the view scrolls just far enough to reveal it.
func (this *Table) scrollRowIntoView(r int) {
	if this.model == nil || r < 0 || r >= this.model.RowCount() {
		return
	}
	top := int(this.scrollArea.ScrollY())
	if r < top {
		this.scrollArea.SetScrollY(float64(r))
		return
	}
	perPage := this.visibleRowsPerPage()
	if r >= top+perPage {
		this.scrollArea.SetScrollY(float64(r - perPage + 1))
	}
}

// OnKeyDown provides Qt QTableView-style row navigation (it coexists with the
// header click-to-sort and column-resize handling, which are mouse-only):
//
//	Up/Down       : move the current row by one, clamped to the ends
//	PageUp/PageDown: move by a viewport page of rows
//	Home/End      : jump to the first/last row
//	Enter/Space   : activate the current row (fires SigRowActivated)
func (this *Table) OnKeyDown(key int, repeat bool) {
	if this.model == nil {
		return
	}
	n := this.model.RowCount()
	if n == 0 {
		return
	}
	page := this.visibleRowsPerPage()

	switch key {
	case KeyDown:
		this.moveCurrentRow(tableRowStep(this.selectedRow, n, 1))
	case KeyUp:
		this.moveCurrentRow(tableRowStep(this.selectedRow, n, -1))
	case KeyPageDown:
		this.moveCurrentRow(tableRowStep(this.selectedRow, n, page))
	case KeyPageUp:
		this.moveCurrentRow(tableRowStep(this.selectedRow, n, -page))
	case KeyHome:
		this.moveCurrentRow(0)
	case KeyEnd:
		this.moveCurrentRow(n - 1)
	case KeyEnter, KeySpace:
		this.activateSelected()
	}
}

// moveCurrentRow selects row r and scrolls it into view; the entry point for
// every keyboard navigation key. Keyboard navigation collapses any multi-row
// selection back to a single row and re-anchors the range there.
func (this *Table) moveCurrentRow(r int) {
	this.selectedRows = nil
	this.selectAnchor = r
	this.SetSelectedRow(r)
	this.scrollRowIntoView(r)
}

// tableRowStep moves a current-row index by delta within a list of n rows,
// clamping to [0, n-1]. A delta from a "no current row" state (cur < 0) lands
// on the first row for a downward step and stays put otherwise. Pulled out as a
// pure function so the clamp + page-step math is unit-testable without a window.
func tableRowStep(cur, n, delta int) int {
	if n <= 0 {
		return -1
	}
	if cur < 0 {
		if delta > 0 {
			cur = 0
			delta--
		} else {
			return 0
		}
	}
	r := cur + delta
	if r < 0 {
		r = 0
	}
	if r >= n {
		r = n - 1
	}
	return r
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
// column clears back to the model's natural order. A model implementing
// SortableTableModel reorders itself; any other (read-only) model is left
// untouched and the view sorts a display-order permutation instead, so a header
// click still sorts on screen without mutating the model. This is the entry
// point used by the header-click handler.
func (this *Table) sortByColumn(col int) {
	if this.model == nil {
		return
	}
	if col < 0 || col >= this.model.ColumnCount() {
		return
	}

	// Advance the sort state: unsorted -> ascending, ascending -> descending,
	// descending -> cleared (natural order).
	switch {
	case this.sortColumn != col:
		this.sortColumn = col
		this.sortAscending = true
	case this.sortAscending:
		this.sortAscending = false
	default:
		this.sortColumn = -1
		this.sortAscending = false
	}

	// Apply the new state. A SortableTableModel reorders itself; a read-only
	// model keeps a view-side permutation so it never gets mutated.
	if sm, ok := this.model.(SortableTableModel); ok {
		this.displayOrder = nil
		if this.sortColumn < 0 {
			sm.RestoreOrder()
		} else {
			sm.SortByColumn(this.sortColumn, this.sortAscending)
		}
	} else if this.sortColumn < 0 {
		this.displayOrder = nil
	} else {
		this.buildDisplayOrder(this.sortColumn, this.sortAscending)
	}

	if this.cbSortChanged != nil {
		this.cbSortChanged(this.sortColumn, this.sortAscending)
	}
	this.Update()
}

// buildDisplayOrder computes the view-side display permutation used to sort a
// model that does not implement SortableTableModel. It stable-sorts the row
// indices [0, RowCount) by column col — numerically when every non-empty cell
// in the column parses as a number, otherwise case-insensitively — without ever
// reordering or mutating the model. dispRow maps a display position back to the
// model row through the result.
func (this *Table) buildDisplayOrder(col int, ascending bool) {
	n := this.model.RowCount()
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	numeric := this.columnIsNumericModel(col)
	sort.SliceStable(order, func(a, b int) bool {
		ca := this.model.CellText(order[a], col)
		cb := this.model.CellText(order[b], col)
		var c int
		if numeric {
			c = compareTableCellsNumeric(ca, cb)
		} else {
			c = compareTableCells(ca, cb)
		}
		if !ascending {
			c = -c
		}
		return c < 0
	})
	this.displayOrder = order
}

// columnIsNumericModel reports whether every non-empty cell in the model's
// column parses as a float64. It mirrors columnIsNumeric but reads through the
// TableModel interface (used for the view-side sort of a non-sortable model).
func (this *Table) columnIsNumericModel(col int) bool {
	seen := false
	for r := 0; r < this.model.RowCount(); r++ {
		s := strings.TrimSpace(this.model.CellText(r, col))
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

// dispRow maps a display-space row position to the underlying model row. It is
// the identity unless a view-side sort permutation is active (see
// buildDisplayOrder), which happens only for a model that cannot sort itself.
// Every model-row read or write that starts from a display position (drawing,
// inline editing) goes through here so a view-side sort reorders rows on screen
// without touching the model.
func (this *Table) dispRow(r int) int {
	if this.displayOrder != nil && r >= 0 && r < len(this.displayOrder) {
		return this.displayOrder[r]
	}
	return r
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
		if this.isRowSelected(r) {
			g.SetBrush1(t.HighLightColor)
			g.Rectangle(0, y, this.totalColumnWidth(), rh)
			g.Fill()
		}

		// Cell text
		xPos := 0.0
		for c := 0; c < colCount; c++ {
			cw := this.columnWidth(c)
			txt := this.model.CellText(this.dispRow(r), c)
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
	if row < 0 || row >= this.model.RowCount() {
		return
	}
	col := this.headerColumnAt(x)

	// Ctrl / Shift extend the selection to multiple rows; a plain click keeps
	// the original single-row behaviour and (on a second click) opens the
	// inline editor.
	switch {
	case IsKeyDown(KeyCtrl):
		this.toggleRowSelection(row)
		return
	case IsKeyDown(KeyShift):
		this.selectRowRange(this.selectAnchor, row)
		return
	}

	this.selectedRows = nil
	this.selectAnchor = row
	this.SetSelectedRow(row)

	// Double-click on the same cell opens the inline editor (a no-op unless
	// editing is enabled).
	now := time.Now()
	if row == this.lastClickRow && col == this.lastClickCol &&
		now.Sub(this.lastClickTime) < tableDoubleClickInterval {
		this.lastClickTime = time.Time{} // reset so a third click is a fresh single
		this.tryBeginEdit(row, col)
		return
	}
	this.lastClickTime = now
	this.lastClickRow = row
	this.lastClickCol = col
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
	this.commitEdit() // an open inline editor would otherwise float off its cell
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

// --- In-place cell editing -------------------------------------------------

// tableCellEditor is the inline cell editor: a gui.Edit that reports Enter,
// Esc and focus-loss back to its owning Table, which drives commit / cancel.
// Embedding Edit reuses the whole text-input widget rather than building a new
// one.
type tableCellEditor struct {
	Edit
	table *Table
}

// newTableCellEditor builds a borderless single-line editor bound to a table.
func newTableCellEditor(t *Table) *tableCellEditor {
	e := new(tableCellEditor)
	e.Init(e)
	e.table = t
	e.SetNoFrame(true) // inline overlay: no separate frame around the cell
	return e
}

// OnKeyDown commits on Enter and cancels on Esc; every other key falls through
// to the embedded Edit's normal handling.
func (this *tableCellEditor) OnKeyDown(key int, repeat bool) {
	switch key {
	case KeyEnter:
		this.table.commitEdit()
		return
	case KeyEsc:
		this.table.cancelEdit()
		return
	}
	this.Edit.OnKeyDown(key, repeat)
}

// OnFocusChanged commits when focus leaves the editor for another widget
// (click-away / blur), matching the Enter behaviour.
func (this *tableCellEditor) OnFocusChanged(newFocus, oldFocus IWidget) {
	this.Edit.OnFocusChanged(newFocus, oldFocus)
	if oldFocus == this.Self() && newFocus != this.Self() {
		this.table.commitEdit()
	}
}

// SetCellsEditable toggles in-place cell editing. It defaults to false, so a
// table stays read-only (and byte-for-byte unchanged) unless a host opts in.
// Turning editing off cancels any editor currently open.
func (this *Table) SetCellsEditable(b bool) {
	if this.cellsEditable == b {
		return
	}
	this.cellsEditable = b
	if !b {
		this.cancelEdit()
	}
}

// IsCellsEditable reports whether in-place cell editing is enabled.
func (this *Table) IsCellsEditable() bool {
	return this.cellsEditable
}

// SigCellEdited sets the callback fired after a cell edit commits, with the
// edited row, column and the new text. Fires even when the model does not
// implement EditableTableModel, so a host can persist the value itself.
func (this *Table) SigCellEdited(fn func(row, col int, newText string)) {
	this.cbCellEdited = fn
}

// cellAtXY maps a widget-space point to the data cell under it, or (-1, -1)
// for the header row or an empty area past the data. It mirrors the draw
// path's scroll handling and reuses headerColumnAt for the column hit-test.
func (this *Table) cellAtXY(x, y float64) (row, col int) {
	if this.model == nil {
		return -1, -1
	}
	hh := this.HeaderHeight()
	if y < hh {
		return -1, -1
	}
	rh := this.RowHeight()
	sy := this.scrollArea.ScrollY()
	row = int((y-hh)/rh + sy)
	if row < 0 || row >= this.model.RowCount() {
		return -1, -1
	}
	col = this.headerColumnAt(x)
	if col < 0 {
		return -1, -1
	}
	return row, col
}

// cellRect returns the widget-space rectangle of a data cell, matching the
// draw path: columns offset by the horizontal scroll, rows by the (fractional)
// vertical scroll. Used to position the inline editor over the cell.
func (this *Table) cellRect(row, col int) (x, y, w, h float64) {
	hh := this.HeaderHeight()
	rh := this.RowHeight()
	sx := this.scrollArea.ScrollX()
	sy := this.scrollArea.ScrollY()
	left := 0.0
	for c := 0; c < col; c++ {
		left += this.columnWidth(c)
	}
	x = left - sx
	y = hh + (float64(row)-sy)*rh
	w = this.columnWidth(col)
	h = rh
	return
}

// tryBeginEdit opens the editor on (row, col) when editing is enabled and the
// cell is a real data cell. It is the gated entry from the double-click
// handler; a read-only table (or an out-of-range cell) is a no-op, keeping the
// default behaviour unchanged.
func (this *Table) tryBeginEdit(row, col int) {
	if !this.cellsEditable || this.model == nil {
		return
	}
	if row < 0 || row >= this.model.RowCount() {
		return
	}
	if col < 0 || col >= this.model.ColumnCount() {
		return
	}
	this.beginEdit(row, col)
}

// beginEdit overlays the inline editor on a cell, seeds it with the cell text,
// selects all of it and gives it focus. Any editor already open is committed
// first, so only one editor is ever active.
func (this *Table) beginEdit(row, col int) {
	this.commitEdit()
	if this.editor == nil {
		this.editor = newTableCellEditor(this)
		this.editor.SetParent(this)
	}
	this.editRow = row
	this.editCol = col
	this.editing = true
	x, y, w, h := this.cellRect(row, col)
	this.editor.SetBounds(x, y, w, h)
	this.editor.SetText(this.model.CellText(this.dispRow(row), col))
	this.editor.SelectAll()
	this.editor.SetVisible(true)
	this.editor.SetFocus()
	this.Update()
}

// commitEdit writes the editor's text back through EditableTableModel (a no-op
// when the model is read-only), fires SigCellEdited, and hides the editor. It
// is safe to call when no edit is active. The editing flag is cleared first so
// the focus change from hiding the editor cannot re-enter this method.
func (this *Table) commitEdit() {
	if !this.editing || this.editor == nil {
		return
	}
	this.editing = false
	row, col := this.editRow, this.editCol
	text := this.editor.Text()
	hadFocus := this.editor.HasFocus()
	this.editor.SetVisible(false)
	mrow := this.dispRow(row) // display row -> model row under a view-side sort
	if em, ok := this.model.(EditableTableModel); ok {
		em.SetCellText(mrow, col, text)
	}
	if this.cbCellEdited != nil {
		this.cbCellEdited(mrow, col, text)
	}
	// Return focus to the table only when the commit was driven from the
	// editor itself (Enter / programmatic). On a blur to another widget the
	// editor has already lost focus, so we must not steal it back.
	if hadFocus {
		this.SetFocus()
	}
	this.Update()
}

// cancelEdit discards the active editor without writing to the model.
func (this *Table) cancelEdit() {
	if !this.editing || this.editor == nil {
		return
	}
	this.editing = false
	hadFocus := this.editor.HasFocus()
	this.editor.SetVisible(false)
	if hadFocus {
		this.SetFocus()
	}
	this.Update()
}

// --- Multi-row selection ---------------------------------------------------

// isRowSelected reports whether a row is part of the current selection. When a
// multi-selection is active the set is authoritative; otherwise the single
// current row is the selection.
func (this *Table) isRowSelected(row int) bool {
	if this.selectedRows != nil {
		return this.selectedRows[row]
	}
	return row == this.selectedRow
}

// SelectedRows returns every selected row index in ascending order. With no
// multi-selection active it returns just the current row (or an empty slice
// when nothing is selected), so single-select callers get a sensible result.
func (this *Table) SelectedRows() []int {
	if this.selectedRows != nil {
		ret := make([]int, 0, len(this.selectedRows))
		for r := range this.selectedRows {
			ret = append(ret, r)
		}
		sort.Ints(ret)
		return ret
	}
	if this.selectedRow >= 0 {
		return []int{this.selectedRow}
	}
	return []int{}
}

// ensureSelectionSet lazily creates the multi-selection set, seeding it with
// the current single selection so the first Ctrl-click extends rather than
// replaces it.
func (this *Table) ensureSelectionSet() {
	if this.selectedRows == nil {
		this.selectedRows = make(map[int]bool)
		if this.selectedRow >= 0 {
			this.selectedRows[this.selectedRow] = true
		}
	}
}

// toggleRowSelection flips row's membership in the multi-selection (Ctrl-click)
// and makes it the current row and range anchor.
func (this *Table) toggleRowSelection(row int) {
	if this.model == nil || row < 0 || row >= this.model.RowCount() {
		return
	}
	this.ensureSelectionSet()
	if this.selectedRows[row] {
		delete(this.selectedRows, row)
	} else {
		this.selectedRows[row] = true
	}
	this.selectedRow = row
	this.selectAnchor = row
	this.Update()
}

// selectRowRange selects the inclusive range of rows between anchor and row
// (Shift-click), replacing any prior multi-selection. A negative anchor falls
// back to a single-row selection.
func (this *Table) selectRowRange(anchor, row int) {
	if this.model == nil || row < 0 || row >= this.model.RowCount() {
		return
	}
	if anchor < 0 {
		anchor = row
	}
	lo, hi := anchor, row
	if lo > hi {
		lo, hi = hi, lo
	}
	this.selectedRows = make(map[int]bool)
	for r := lo; r <= hi; r++ {
		this.selectedRows[r] = true
	}
	this.selectedRow = row
	this.Update()
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
