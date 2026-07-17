package gui

import (
	"strconv"
	"unicode/utf8"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// ReportView is a read-only 报表 (report) table viewer for operator screens: a
// bold column-header row over a vertically scrollable body of data rows with
// alternating (zebra) backgrounds. It is deliberately backend-free — it holds a
// plain view-model (a []string header and a [][]string body) fed via SetTable
// and never imports the report/query packages that produce those tables. Column
// widths are derived from the content; wide cells are clipped (v1 has no
// horizontal scroll).
//
// The only user intent it emits is export: the toolbar carries "导出CSV" and
// "导出HTML" buttons that fire SigExport with "csv" / "html". The host owns the
// actual serialization and file writing; the panel stays a pure view.
type ReportView struct {
	Widget

	headers   []string
	rows      [][]string
	scrollY   float64
	rowHeight float64
	cbExport  func(format string)
}

func init() {
	core.RegisterFactory("gui.ReportView", core.TypeOf((*ReportView)(nil)))
}

// NewReportView creates an empty report view.
func NewReportView() *ReportView {
	p := new(ReportView)
	p.Init(p)
	return p
}

func (this *ReportView) Init(self IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
}

// SetTable replaces the displayed table with a defensive deep copy of headers
// and rows. The inner row slices are copied too, so later mutation of the
// caller's data cannot reach the panel. The scroll offset is clamped to the new
// content rather than reset, so a refresh does not yank the operator's view back
// to the top.
func (this *ReportView) SetTable(headers []string, rows [][]string) {
	h := make([]string, len(headers))
	copy(h, headers)
	this.headers = h
	this.rows = copyStringRows(rows)
	this.clampScroll()
	this.Self().Update()
}

// Headers returns a defensive copy of the column headers.
func (this *ReportView) Headers() []string {
	out := make([]string, len(this.headers))
	copy(out, this.headers)
	return out
}

// RowCount returns the number of data rows (headers excluded).
func (this *ReportView) RowCount() int {
	return len(this.rows)
}

// SigExport registers the callback fired when the operator clicks an export
// button. It receives the requested format: "csv" or "html".
func (this *ReportView) SigExport(fn func(format string)) {
	this.cbExport = fn
}

// copyStringRows deep-copies a [][]string so the panel is isolated from later
// mutation of the caller's rows (the inner slices are reference types).
func copyStringRows(in [][]string) [][]string {
	out := make([][]string, len(in))
	for i, r := range in {
		rc := make([]string, len(r))
		copy(rc, r)
		out[i] = rc
	}
	return out
}

// --- Layout geometry (pure) ---

const (
	reportToolbarH   = 28.0 // top toolbar band (title + export buttons)
	reportHeaderH    = 24.0 // bold column-header row, below the toolbar
	reportBtnW       = 76.0 // export button width
	reportBtnH       = 20.0 // export button height
	reportBtnPad     = 8.0  // gap from the right edge to the last button
	reportBtnSpacing = 6.0  // gap between the two export buttons

	reportColCharW = 8.0   // approximate per-rune width used to size columns
	reportColPad   = 16.0  // horizontal padding added to each column
	reportMinColW  = 56.0  // column width floor
	reportMaxColW  = 260.0 // column width ceiling (wider content is clipped)
)

// dataTop is the y where the scrollable data rows begin.
func reportDataTop() float64 { return reportToolbarH + reportHeaderH }

// exportButtonAt maps a toolbar click to an export format ("csv" / "html") or
// "" when the click misses both buttons. Pure geometry over the panel width w,
// so the click routing is unit-testable headless. The buttons are right-anchored
// with CSV to the left of HTML.
func exportButtonAt(x, y, w float64) string {
	top := (reportToolbarH - reportBtnH) / 2
	if y < top || y >= top+reportBtnH {
		return ""
	}
	htmlX1 := w - reportBtnPad - reportBtnW
	htmlX2 := w - reportBtnPad
	csvX2 := htmlX1 - reportBtnSpacing
	csvX1 := csvX2 - reportBtnW
	switch {
	case x >= csvX1 && x < csvX2:
		return "csv"
	case x >= htmlX1 && x < htmlX2:
		return "html"
	default:
		return ""
	}
}

// columnWidths derives a per-column pixel width from the longest cell (header or
// body) in each column, clamped to [reportMinColW, reportMaxColW]. Pure over the
// stored headers/rows (no painter), so it is safe to call headless.
func (this *ReportView) columnWidths() []float64 {
	n := len(this.headers)
	for _, r := range this.rows {
		if len(r) > n {
			n = len(r)
		}
	}
	if n == 0 {
		return nil
	}
	widths := make([]float64, n)
	for i := 0; i < n; i++ {
		runes := 0
		if i < len(this.headers) {
			runes = utf8.RuneCountInString(this.headers[i])
		}
		for _, r := range this.rows {
			if i < len(r) {
				if c := utf8.RuneCountInString(r[i]); c > runes {
					runes = c
				}
			}
		}
		w := float64(runes)*reportColCharW + reportColPad
		if w < reportMinColW {
			w = reportMinColW
		}
		if w > reportMaxColW {
			w = reportMaxColW
		}
		widths[i] = w
	}
	return widths
}

// --- Drawing ---

// Draw renders a toolbar (title + export buttons), a bold column-header row and
// the scrollable, zebra-striped data rows, all in the active Theme() colours so
// the panel reads correctly in the dark IDE theme.
func (this *ReportView) Draw(g paint.Painter) {
	w, h := this.Size()
	th := Theme()

	// Data-area background.
	g.SetBrush1(th.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont(defaultFontFamily(), 13, false, false)
	bold := paint.NewFont(defaultFontFamily(), 13, true, false)
	fe := font.FontExtents()

	// Toolbar band (chrome fill + title on the left).
	g.SetBrush1(th.FormColor)
	g.Rectangle(0, 0, w, reportToolbarH)
	g.Fill()
	g.SetFont(bold)
	g.SetBrush1(th.TextColor)
	title := "报表 · " + strconv.Itoa(len(this.rows)) + " 行"
	g.DrawText1(8, (reportToolbarH+fe.Ascent-fe.Descent)/2, title)

	// Export buttons: right-anchored bordered pills in the theme accent.
	btnY := (reportToolbarH - reportBtnH) / 2
	htmlX := w - reportBtnPad - reportBtnW
	csvX := htmlX - reportBtnSpacing - reportBtnW
	this.drawExportButton(g, csvX, btnY, "导出CSV")
	this.drawExportButton(g, htmlX, btnY, "导出HTML")

	// Column geometry.
	widths := this.columnWidths()

	// Column-header row (bold), separated from the body by a hairline border.
	g.SetBrush1(th.FormColor)
	g.Rectangle(0, reportToolbarH, w, reportHeaderH)
	g.Fill()
	g.SetFont(bold)
	g.SetBrush1(th.TextColor)
	headerBaseline := reportToolbarH + (reportHeaderH+fe.Ascent-fe.Descent)/2
	this.drawCells(g, this.headers, widths, headerBaseline, reportToolbarH, w)
	g.SetPen1(th.BorderColor, 1)
	g.Line(0, reportDataTop(), w, reportDataTop())
	g.Stroke()

	if len(this.rows) == 0 {
		return
	}

	// Zebra tint for odd rows: derived from TextColor at low alpha so it stays
	// visible in both light and dark themes without a hardcoded grey.
	zebra := th.TextColor
	zebra.A = 12

	rh := this.rowHeight
	areaTop := reportDataTop()
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	g.SetFont(font)
	for i := startIdx; i < startIdx+visibleCount && i < len(this.rows); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		if i%2 == 1 {
			g.SetBrush1(zebra)
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}
		g.SetBrush1(th.TextColor)
		baseline := y + (rh+fe.Ascent-fe.Descent)/2
		this.drawCells(g, this.rows[i], widths, baseline, y, w)
	}
}

// drawCells lays out one row of cells left-to-right at the given text baseline,
// clipping each cell to its column so wide content cannot spill into the next
// column. cellTop/rowW bound the clip rectangle vertically and to the panel.
func (this *ReportView) drawCells(g paint.Painter, cells []string, widths []float64, baseline, cellTop, rowW float64) {
	x := 0.0
	for i, cw := range widths {
		if x >= rowW {
			break
		}
		if i < len(cells) && cells[i] != "" {
			g.Save()
			g.Rectangle(x, cellTop, cw, this.rowHeight)
			g.Clip()
			g.DrawText1(x+reportColPad/2, baseline, cells[i])
			g.Restore()
		}
		x += cw
	}
}

// drawExportButton renders one bordered accent-coloured pill with a centred
// label at (x, y) sized reportBtnW x reportBtnH.
func (this *ReportView) drawExportButton(g paint.Painter, x, y float64, label string) {
	th := Theme()
	roundedRect(g, x, y, reportBtnW, reportBtnH, 4)
	g.SetPen1(th.Accent, 1)
	g.Stroke()
	g.SetBrush1(th.Accent)
	g.DrawText1(x+8, y+reportBtnH-6, label)
}

// --- Events ---

// OnLeftDown fires SigExport when a toolbar export button is clicked. Clicks in
// the data body (or that miss the buttons) are ignored beyond taking focus.
func (this *ReportView) OnLeftDown(x, y float64) {
	this.SetFocus()
	w, _ := this.Size()
	if format := exportButtonAt(x, y, w); format != "" {
		if this.cbExport != nil {
			this.cbExport(format)
		}
		return
	}
}

// OnMouseWheel scrolls the data rows vertically.
func (this *ReportView) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	this.clampScroll()
	this.Self().Update()
}

// rowAtY maps a y coordinate to a data-row index, accounting for the toolbar +
// header bands and the current scroll offset; it returns -1 when y lands above
// the data area. Pure geometry (only scrollY and rowHeight), so it is unit-
// testable headless. The index may exceed the row count for a click past the
// last row — callers bound-check against RowCount().
func (this *ReportView) rowAtY(y float64) int {
	top := reportDataTop()
	if y < top {
		return -1
	}
	return int((y - top + this.scrollY) / this.rowHeight)
}

// clampScroll pins scrollY within [0, maxScroll] for the current content and
// viewport height.
func (this *ReportView) clampScroll() {
	_, h := this.Size()
	maxScroll := float64(len(this.rows))*this.rowHeight - (h - reportDataTop())
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	if this.scrollY < 0 {
		this.scrollY = 0
	}
}

func (this *ReportView) SizeHints() SizeHints {
	return SizeHints{MinWidth: 240, MinHeight: 100}
}
