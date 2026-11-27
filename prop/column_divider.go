package prop

import (
	"math"
	"time"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

const (
	defaultColumnSplit   = 0.5
	minColumnSplit       = 0.24
	maxColumnSplit       = 0.78
	columnDividerWidth   = 6.0
	columnDividerMinPane = 72.0
	columnDividerHint    = "Drag to resize columns. Double-click or press Home to reset."
)

func init() {
	i18n.AddMany("zh-CN", map[string]string{columnDividerHint: "拖动调整列宽，双击或按 Home 恢复默认宽度。"})
}

// ColumnSplit reports what fraction of the Inspector's content width
// belongs to property names. The remaining space belongs to editors.
func (s *PropertySheet) ColumnSplit() float64 { return s.vertSplit }

// SetColumnSplit changes only presentation. It neither rebinds the sheet nor
// reads or writes property values; active editor and Undo state are retained.
// Call on the UI thread, like the other PropertySheet setters.
func (s *PropertySheet) SetColumnSplit(fraction float64) {
	if math.IsNaN(fraction) || math.IsInf(fraction, 0) {
		return
	}
	fraction = min(maxColumnSplit, max(minColumnSplit, fraction))
	if s.vertSplit == fraction {
		return
	}
	s.vertSplit = fraction
	s.positionColumnDivider()
	s.positionEditors()
	s.Update()
}

func (s *PropertySheet) positionColumnDivider() {
	if s.columnDivider == nil {
		return
	}
	width, height := s.body.Size()
	visible := s.body.IsVisible() && width >= columnDividerMinPane && height > 0 && len(s.vlist) != 0
	d := s.columnDivider
	if visible {
		x := s.vertSplitPx() - columnDividerWidth
		d.SetBounds(x, 0, columnDividerWidth, height)
	}
	d.SetVisible(visible)
}

// propertyColumnDivider lives in the same clipped viewport as the editors.
// Its hit strip is reserved wholly on the name side of the boundary, so it
// cannot cover or intercept any part of a value editor.
type propertyColumnDivider struct {
	gui.Widget
	sheet                     *PropertySheet
	dragging, moved           bool
	startBodyX, startFraction float64
	lastClick                 time.Time
	lastClickX, lastClickY    float64
}

func (d *propertyColumnDivider) Cursor() *gui.Cursor { return gui.HorizontalResizeCursor() }

func (d *propertyColumnDivider) contains(x, y float64) bool {
	return x >= 0 && x < d.Width() && y >= 0 && y < d.Height()
}

func (d *propertyColumnDivider) OnLeftDown(x, y float64) {
	if d.dragging || !d.IsVisible() || !d.contains(x, y) {
		return
	}
	bodyX := d.X() + x
	if !d.lastClick.IsZero() && time.Since(d.lastClick) < 400*time.Millisecond &&
		math.Abs(bodyX-d.lastClickX) <= 5 && math.Abs(y-d.lastClickY) <= 5 {
		d.lastClick = time.Time{}
		d.sheet.SetColumnSplit(defaultColumnSplit)
		return
	}
	d.lastClick = time.Time{}
	d.startBodyX, d.startFraction = bodyX, d.sheet.ColumnSplit()
	d.dragging, d.moved = true, false
	d.PushCapture()
	d.Update()
}

func (d *propertyColumnDivider) OnMouseMove(x, _ float64) {
	if !d.dragging {
		return
	}
	if !d.HasCapture() {
		d.OnPointerCancel()
		return
	}
	width := d.sheet.body.Width()
	if width < columnDividerMinPane || math.IsNaN(x) || math.IsInf(x, 0) {
		return
	}
	delta := d.X() + x - d.startBodyX
	if !d.moved && math.Abs(delta) < 2 {
		return
	}
	d.moved = true
	d.sheet.SetColumnSplit(d.startFraction + delta/width)
}

func (d *propertyColumnDivider) OnLeftUp(x, y float64) {
	if !d.dragging {
		return
	}
	d.OnMouseMove(x, y)
	if d.dragging && !d.moved && d.contains(x, y) {
		d.lastClick = time.Now()
		d.lastClickX, d.lastClickY = d.X()+x, y
	}
	d.endDrag()
}

func (d *propertyColumnDivider) endDrag() {
	if !d.dragging {
		return
	}
	d.dragging = false
	d.PopCapture()
	d.Update()
}

func (d *propertyColumnDivider) OnPointerCancel() {
	d.lastClick = time.Time{}
	d.endDrag()
}

func (d *propertyColumnDivider) OnHide() { d.OnPointerCancel() }
func (d *propertyColumnDivider) OnMouseEnter() {
	gui.SetToolTip(d, i18n.T(columnDividerHint))
	d.Update()
}
func (d *propertyColumnDivider) OnMouseLeave() { d.Update() }

func (d *propertyColumnDivider) OnKeyDown(key int, _ bool) {
	if !d.IsVisible() || d.sheet.body.Width() < columnDividerMinPane {
		return
	}
	step := 8 / d.sheet.body.Width()
	switch key {
	case gui.KeyLeft:
		d.sheet.SetColumnSplit(d.sheet.ColumnSplit() - step)
	case gui.KeyRight:
		d.sheet.SetColumnSplit(d.sheet.ColumnSplit() + step)
	case gui.KeyHome:
		d.sheet.SetColumnSplit(defaultColumnSplit)
	}
}

func (d *propertyColumnDivider) Draw(g paint.Painter) {
	if !d.dragging && !d.IsHover() && !d.HasFocus() {
		return
	}
	w, h := d.Size()
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	accent := gui.Theme().HighLightColor
	accent.A = 65
	if d.dragging {
		accent.A = 120
	}
	g.SetBrush1(accent)
	g.Fill()
}
