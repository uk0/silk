package gui

import (
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// StatRow is one plain view-model row for a per-tag statistics table: the tag
// name plus its running count and min/max/avg/last aggregates. It is a value
// type carrying no backend references, so the host can compute it from whatever
// stats source it likes (typically package stats) and hand the panel a flat
// snapshot. Copying a StatRow fully isolates it.
type StatRow struct {
	Tag   string
	Count int
	Min   float64
	Max   float64
	Avg   float64
	Last  float64
}

// StatsPanel is a live per-tag 统计 (statistics) table for SCADA / 组态 screens:
// a scrollable list showing, per tag, the sample Count and the Min / Max / Avg /
// Last aggregates, plus a per-row 清零 (clear) affordance. It is deliberately
// UI-only and holds nothing but plain StatRow view-model data fed via SetStats;
// it does not import or know about the backend stats engine. The host computes a
// snapshot, calls SetStats, and wires SigReset to clear a tag's aggregates.
//
// Resetting is not done here. A click on a row's 清零 cell fires SigReset(tag);
// the host clears that tag in its stats store and pushes a fresh snapshot back.
// This keeps the panel a pure, GL-free-testable view.
type StatsPanel struct {
	Widget

	rows      []StatRow
	scrollY   float64
	rowHeight float64
	cbReset   func(tag string)
}

func init() {
	core.RegisterFactory("gui.StatsPanel", core.TypeOf((*StatsPanel)(nil)))
}

// NewStatsPanel creates an empty statistics panel.
func NewStatsPanel() *StatsPanel {
	p := new(StatsPanel)
	p.Init(p)
	return p
}

func (this *StatsPanel) Init(self IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
}

// SetStats replaces the displayed rows with a defensive copy of in. StatRow is a
// value type, so the shallow copy fully isolates the panel from later mutation
// of the caller's slice. The panel renders the rows verbatim in the order given
// (the host decides the ordering). The scroll offset is clamped to the new
// content rather than reset, so a live refresh does not yank the operator's view
// back to the top.
func (this *StatsPanel) SetStats(in []StatRow) {
	cp := make([]StatRow, len(in))
	copy(cp, in)
	this.rows = cp
	this.clampScroll()
	this.Self().Update()
}

// Stats returns a defensive copy of the displayed rows in display order.
func (this *StatsPanel) Stats() []StatRow {
	out := make([]StatRow, len(this.rows))
	copy(out, this.rows)
	return out
}

// RowCount returns the number of rows currently displayed.
func (this *StatsPanel) RowCount() int { return len(this.rows) }

// SigReset registers the callback fired when the operator clicks a row's 清零
// affordance. It receives the row's tag; the host clears that tag's statistics
// and pushes a refreshed snapshot back via SetStats.
func (this *StatsPanel) SigReset(fn func(tag string)) {
	this.cbReset = fn
}

// fmtStat renders one statistic value with fixed two-decimal precision, e.g.
// fmtStat(1.239) == "1.24". Kept a package-level pure helper so the numeric
// formatting is unit-testable without a painter.
func fmtStat(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// --- Drawing ---

const (
	statsTitleH      = 22.0                        // top title band height
	statsColHeadH    = 20.0                        // column-header band height
	statsHeaderTotal = statsTitleH + statsColHeadH // rows begin below both bands
	statsResetColW   = 52.0                        // width of the right-anchored 清零 hit column

	// Fixed left column x offsets (cosmetic; hit-testing only cares about the
	// right-anchored reset column, which is width-relative).
	statsColTag   = 8.0
	statsColCount = 130.0
	statsColMin   = 180.0
	statsColMax   = 240.0
	statsColAvg   = 300.0
	statsColLast  = 360.0
)

// Draw renders a title band, a column-header band, then one scrollable row per
// tag. All colours come from Theme() semantic fields so the panel reads
// correctly in the dark IDE theme; zebra striping is a low-alpha TextColor tint
// that stays visible in both light and dark modes (ViewBGColor == FormColor in
// dark mode, so a FormColor stripe would vanish).
func (this *StatsPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	t := Theme()

	// View background.
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := t.Font
	g.SetFont(font)
	fe := font.FontExtents()

	// Title band: "统计 Statistics · N tags".
	g.SetBrush1(t.FormColor)
	g.Rectangle(0, 0, w, statsTitleH)
	g.Fill()
	g.SetBrush1(t.TextColor)
	g.DrawText1(statsColTag, fe.Ascent+4, "统计 Statistics · "+strconv.Itoa(len(this.rows))+" tags")

	// Column-header band with muted labels and a separator line beneath it.
	g.SetBrush1(t.FormColor)
	g.Rectangle(0, statsTitleH, w, statsColHeadH)
	g.Fill()
	headBaseline := statsTitleH + fe.Ascent + 3
	g.SetBrush1(t.MenuGrayTextColor)
	g.DrawText1(statsColTag, headBaseline, "Tag")
	g.DrawText1(statsColCount, headBaseline, "Count")
	g.DrawText1(statsColMin, headBaseline, "Min")
	g.DrawText1(statsColMax, headBaseline, "Max")
	g.DrawText1(statsColAvg, headBaseline, "Avg")
	g.DrawText1(statsColLast, headBaseline, "Last")
	g.Line(0, statsHeaderTotal, w, statsHeaderTotal)
	g.SetPen1(t.SeperatorColor, 1)
	g.Stroke()

	if len(this.rows) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := statsHeaderTotal
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	// Zebra stripe tint derived from TextColor at low alpha (both-modes trick).
	stripe := t.TextColor
	stripe.A = 16

	for i := startIdx; i < startIdx+visibleCount && i < len(this.rows); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		r := this.rows[i]

		if i%2 == 1 {
			g.SetBrush1(stripe)
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		baseline := y + fe.Ascent + 4

		// Tag in primary text; the numeric columns in muted text.
		g.SetBrush1(t.TextColor)
		g.DrawText1(statsColTag, baseline, r.Tag)
		g.SetBrush1(t.MenuGrayTextColor)
		g.DrawText1(statsColCount, baseline, strconv.Itoa(r.Count))
		g.DrawText1(statsColMin, baseline, fmtStat(r.Min))
		g.DrawText1(statsColMax, baseline, fmtStat(r.Max))
		g.DrawText1(statsColAvg, baseline, fmtStat(r.Avg))
		g.DrawText1(statsColLast, baseline, fmtStat(r.Last))

		// Right-anchored 清零 reset affordance: a bordered accent cell.
		g.SetPen1(t.Accent, 1)
		g.Rectangle(w-statsResetColW+4, y+4, statsResetColW-10, rh-8)
		g.Stroke()
		g.SetBrush1(t.Accent)
		g.DrawText1(w-statsResetColW+9, baseline, "清零")
	}
}

// --- Events ---

// OnLeftDown fires SigReset when the click lands in a row's right-anchored 清零
// column. Clicks elsewhere on a row, on either header band, or past the last row
// are ignored.
func (this *StatsPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAtY(y)
	if idx < 0 || idx >= len(this.rows) {
		return
	}
	w, _ := this.Size()
	if statsResetColumnHit(x, w) && this.cbReset != nil {
		this.cbReset(this.rows[idx].Tag)
	}
}

// OnMouseWheel scrolls the row list vertically.
func (this *StatsPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	this.clampScroll()
	this.Self().Update()
}

// rowAtY maps a y coordinate to a row index, accounting for the two header bands
// and the current scroll offset; it returns -1 when y lands on the header. Pure
// geometry (only scrollY and rowHeight), so it is unit-testable headless. The
// index may exceed the row count for a click past the last row — callers
// bound-check against len(rows).
func (this *StatsPanel) rowAtY(y float64) int {
	if y < statsHeaderTotal {
		return -1
	}
	return int((y - statsHeaderTotal + this.scrollY) / this.rowHeight)
}

// statsResetColumnHit reports whether x falls in the right-anchored 清零 column
// of a panel of width w. Kept pure so the click routing is unit-testable.
func statsResetColumnHit(x, w float64) bool {
	return x >= w-statsResetColW
}

// clampScroll pins scrollY within [0, maxScroll] for the current content and
// viewport height.
func (this *StatsPanel) clampScroll() {
	_, h := this.Size()
	maxScroll := float64(len(this.rows))*this.rowHeight - (h - statsHeaderTotal)
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

func (this *StatsPanel) SizeHints() SizeHints {
	return SizeHints{MinWidth: 320, MinHeight: 100}
}
