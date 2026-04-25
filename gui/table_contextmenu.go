package gui

import (
	"sync"
)

// tableContextCallbacks stores context menu callbacks for Table widgets.
// Since we cannot add fields to the Table struct from a separate file,
// we use a package-level map keyed by the Table pointer.
var (
	tableContextCallbacks   = make(map[*Table]func(*Table, int, int, *Menu))
	tableContextCallbacksMu sync.Mutex
)

// SetContextMenuCallback sets a callback that is invoked when the user
// right-clicks on the table. The callback receives the table, the row
// and column indices under the click, and a Menu to populate.
//
// Pass nil to remove the callback.
func (this *Table) SetContextMenuCallback(fn func(table *Table, row, col int, menu *Menu)) {
	tableContextCallbacksMu.Lock()
	defer tableContextCallbacksMu.Unlock()
	if fn == nil {
		delete(tableContextCallbacks, this)
	} else {
		tableContextCallbacks[this] = fn
	}
}

// OnRightDown handles right-click events for context menu support.
// It determines which row and column were clicked, selects that row,
// and fires the context menu callback.
func (this *Table) OnRightDown(x, y float64) {
	this.SetFocus()
	if this.model == nil {
		return
	}

	hh := this.HeaderHeight()
	rh := this.RowHeight()

	// Determine clicked row
	row := -1
	if y >= hh {
		sy := this.scrollArea.ScrollY()
		row = int((y-hh)/rh + sy)
		if row >= this.model.RowCount() {
			row = -1
		}
	}

	// Determine clicked column
	col := -1
	if this.model.ColumnCount() > 0 {
		sx := this.scrollArea.ScrollX()
		xOff := x + sx
		accum := 0.0
		for c := 0; c < this.model.ColumnCount(); c++ {
			cw := this.model.ColumnWidth(c)
			if xOff >= accum && xOff < accum+cw {
				col = c
				break
			}
			accum += cw
		}
	}

	// Select the row under the click
	if row >= 0 && row != this.selectedRow {
		this.SetSelectedRow(row)
	}

	// Look up the callback
	tableContextCallbacksMu.Lock()
	cb, ok := tableContextCallbacks[this]
	tableContextCallbacksMu.Unlock()

	if ok && cb != nil {
		ShowContextMenu(this.Self(), x, y, func(menu *Menu) {
			cb(this, row, col, menu)
		})
	}
}
