package ged

import (
	"fmt"
	"strings"

	"github.com/uk0/silk/device"
	"github.com/uk0/silk/driver"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

// Point-table columns, in display order.
const (
	colTag = iota
	colAddress
	colType
	colOrder
	colAccess
	colCount
)

var pointColTitles = [colCount]string{"tag", "address", "type", "order", "RO/RW"}

// DevicePointsEditor is the designer-side editor for a device component's tag
// point list. It parses the component's CSV point text (via device.ParsePoints)
// into an editable row model — one driver.TagPoint per row — and, on an inline
// edit, emits the regenerated CSV through SigChanged so the property panel can
// push it back onto the component. Rendering is a read-style table; cell edits
// are applied through SetCell. It embeds gui.Widget so it can sit in the
// property panel; it holds no device connection, only the point config.
type DevicePointsEditor struct {
	gui.Widget
	rows      []driver.TagPoint
	cbChanged func(text string)
	rowHeight float64
	colWidth  float64
}

// NewDevicePointsEditor returns an empty editor ready to be given point text.
func NewDevicePointsEditor() *DevicePointsEditor {
	e := new(DevicePointsEditor)
	e.Init(e)
	return e
}

// Init sets up the embedded widget and table metrics.
func (e *DevicePointsEditor) Init(self gui.IWidget) {
	e.Widget.Init(self)
	e.rowHeight = 22
	e.colWidth = 96
}

// SetPointsText replaces the row model with the points parsed from text. It does
// not fire SigChanged (this is an external set, not a user edit). A malformed
// point list returns an error and leaves the current rows unchanged.
func (e *DevicePointsEditor) SetPointsText(text string) error {
	pts, err := device.ParsePoints(text)
	if err != nil {
		return err
	}
	e.rows = pts
	e.Self().Update()
	return nil
}

// PointsText returns the current rows as canonical CSV text.
func (e *DevicePointsEditor) PointsText() string {
	return device.FormatPoints(e.rows)
}

// SigChanged registers a callback fired with the updated CSV text after an edit.
func (e *DevicePointsEditor) SigChanged(fn func(text string)) {
	e.cbChanged = fn
}

// RowCount is the number of point rows currently displayed.
func (e *DevicePointsEditor) RowCount() int {
	return len(e.rows)
}

// Rows returns a copy of the row model.
func (e *DevicePointsEditor) Rows() []driver.TagPoint {
	out := make([]driver.TagPoint, len(e.rows))
	copy(out, e.rows)
	return out
}

// Cell returns the display string for one cell, or "" if row/col is out of range.
func (e *DevicePointsEditor) Cell(row, col int) string {
	if row < 0 || row >= len(e.rows) {
		return ""
	}
	cells := rowCells(e.rows[row])
	if col < 0 || col >= len(cells) {
		return ""
	}
	return cells[col]
}

// SetCell applies an inline edit: it replaces one field of a row from its string
// form, revalidates the whole row through the shared parser, updates the model
// and fires SigChanged with the new CSV. It errors (leaving the row unchanged)
// if the resulting line is malformed.
func (e *DevicePointsEditor) SetCell(row, col int, value string) error {
	if row < 0 || row >= len(e.rows) {
		return fmt.Errorf("ged: point row %d out of range", row)
	}
	if col < 0 || col >= colCount {
		return fmt.Errorf("ged: point column %d out of range", col)
	}
	cells := rowCells(e.rows[row])
	cells[col] = strings.TrimSpace(value)
	pts, err := device.ParsePoints(strings.Join(cells, ","))
	if err != nil {
		return err
	}
	if len(pts) != 1 {
		return fmt.Errorf("ged: edit did not yield exactly one point")
	}
	e.rows[row] = pts[0]
	if e.cbChanged != nil {
		e.cbChanged(e.PointsText())
	}
	e.Self().Update()
	return nil
}

// rowCells renders a point to its five display fields in column order.
func rowCells(p driver.TagPoint) []string {
	return []string{p.Tag, p.Address, p.Type.String(), p.Order.String(), p.Access.String()}
}

// SizeHints sizes the widget to the header plus the current rows.
func (e *DevicePointsEditor) SizeHints() gui.SizeHints {
	return gui.SizeHints{
		Width:  e.colWidth * colCount,
		Height: e.rowHeight * float64(len(e.rows)+1),
	}
}

// Draw renders the point table: a header row of column titles, then one row per
// point, with light column separators. It is display-only; edits go through
// SetCell.
func (e *DevicePointsEditor) Draw(g paint.Painter) {
	w, _ := e.Size()
	// header band
	g.SetBrush1(paint.Color{R: 226, G: 232, B: 240, A: 255})
	g.Rectangle(0, 0, w, e.rowHeight)
	g.Fill()
	g.SetBrush1(paint.Color{R: 40, G: 56, B: 72, A: 255})
	for c := 0; c < colCount; c++ {
		g.DrawText1(float64(c)*e.colWidth+6, e.rowHeight-7, pointColTitles[c])
	}
	// point rows
	for i, p := range e.rows {
		y := float64(i+1) * e.rowHeight
		for c, s := range rowCells(p) {
			g.DrawText1(float64(c)*e.colWidth+6, y+e.rowHeight-7, s)
		}
	}
	// column separators
	g.SetPen1(paint.Color{R: 200, G: 208, B: 216, A: 255}, 1)
	total := float64(len(e.rows)+1) * e.rowHeight
	for c := 1; c < colCount; c++ {
		x := float64(c) * e.colWidth
		g.MoveTo(x, 0)
		g.LineTo(x, total)
	}
	g.Stroke()
}
