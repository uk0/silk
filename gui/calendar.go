package gui

import (
	"fmt"
	"math"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.Calendar", core.TypeOf((*Calendar)(nil)))
}

// Calendar is a month-grid date picker — an always-visible calendar like
// Qt's QCalendarWidget. A header row shows the displayed month/year with
// prev/next-month arrows; below it a weekday header row, then up to six
// week-rows of day cells (weeks as rows, weekdays as the seven columns).
// Today gets an accent ring, the selected day a filled accent background
// with contrasting text, and days spilling in from the adjacent months
// (to square off the grid) render dimmed. Clicking a day selects it and
// fires SigDateSelected.
//
// Usage:
//
//	cal := gui.NewCalendar()
//	cal.SigDateSelected(func(d time.Time) { label.SetText(d.Format("2006-01-02")) })
//
// Calendar is distinct from DatePicker: DatePicker is a compact text
// field that opens a transient popup calendar, whereas Calendar is the
// full grid laid out inline as a first-class widget.
//
// The week starts on Monday (columns: Mon..Sun) to match DatePicker's
// popup and the project's ISO dayOfWeek helper. The header reads in the
// zh-CN form "2026年6月" to match the designer's Chinese UI.
type Calendar struct {
	Widget

	month    time.Time // the displayed month, normalised to its 1st at midnight
	selected time.Time // the currently selected date (midnight, local)

	hoverRow int // hovered grid row, -1 when none
	hoverCol int // hovered grid column, -1 when none

	cbDateSelected func(time.Time)
}

// NewCalendar creates a Calendar showing the current month with today
// selected.
func NewCalendar() *Calendar {
	c := new(Calendar)
	c.Init(c)
	now := time.Now()
	c.selected = dayStart(now)
	c.month = monthStart(now)
	c.hoverRow = -1
	c.hoverCol = -1
	return c
}

// SelectedDate returns the currently selected date (at midnight, local).
func (this *Calendar) SelectedDate() time.Time { return this.selected }

// SetSelectedDate selects d (truncated to its day) and scrolls the grid
// to the month containing it. It does not fire SigDateSelected — the
// callback is reserved for user clicks, so programmatic selection stays
// quiet. Selecting the already-selected day is a cheap no-op.
func (this *Calendar) SetSelectedDate(d time.Time) {
	d = dayStart(d)
	if d.Equal(this.selected) {
		return
	}
	this.selected = d
	this.month = monthStart(d)
	this.Self().Update()
}

// ShowMonth scrolls the grid to the given year/month without changing the
// selection.
func (this *Calendar) ShowMonth(year int, month time.Month) {
	m := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	if m.Equal(this.month) {
		return
	}
	this.month = m
	this.Self().Update()
}

// NextMonth advances the displayed month by one, rolling Dec→Jan and
// bumping the year.
func (this *Calendar) NextMonth() {
	this.month = this.month.AddDate(0, 1, 0)
	this.Self().Update()
}

// PrevMonth steps the displayed month back by one, rolling Jan→Dec and
// dropping the year.
func (this *Calendar) PrevMonth() {
	this.month = this.month.AddDate(0, -1, 0)
	this.Self().Update()
}

// SigDateSelected registers the callback fired when the user clicks a day
// cell. Receives the selected date at midnight, local.
func (this *Calendar) SigDateSelected(fn func(time.Time)) {
	this.cbDateSelected = fn
}

// DisplayedMonth returns the first day of the month the grid is showing.
func (this *Calendar) DisplayedMonth() time.Time { return this.month }

// --- Pure date helpers (unit-testable without GL) ---

// dayStart truncates a time to midnight in its own location, discarding
// the clock part so two dates on the same calendar day compare equal.
func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// monthStart returns the 1st of t's month at midnight, in t's location.
func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// firstWeekdayOffset returns how many leading cells (0..6) the grid must
// pad before the 1st of the given month, treating Monday as column 0 and
// Sunday as column 6. Go's time.Weekday has Sunday=0..Saturday=6, so we
// rotate it onto the Monday-first layout.
func firstWeekdayOffset(year int, month time.Month) int {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	return (int(first.Weekday()) + 6) % 7
}

// monthGrid builds the calendar grid for the given month as rows of seven
// dates (Monday..Sunday). The grid is padded at the front with trailing
// days from the previous month and at the back with leading days from the
// next month so every row is a full week. The row count is whatever the
// month needs (5 for most, 6 when the month spills past five weeks, and 4
// only for a 28-day February that starts on a Monday).
//
// Pulled out as a free function so the date math is testable without a GL
// context or a live widget.
func monthGrid(year int, month time.Month) [][]time.Time {
	offset := firstWeekdayOffset(year, month)
	// Grid starts `offset` days before the 1st of the month.
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.Local).AddDate(0, 0, -offset)

	days := daysInMonth(year, int(month))
	// Total cells rounded up to a whole number of weeks.
	cells := offset + days
	rows := (cells + 6) / 7

	grid := make([][]time.Time, rows)
	d := start
	for r := 0; r < rows; r++ {
		week := make([]time.Time, 7)
		for c := 0; c < 7; c++ {
			week[c] = d
			d = d.AddDate(0, 0, 1)
		}
		grid[r] = week
	}
	return grid
}

// --- Layout metrics ---

const (
	calendarHeaderH  = 28.0 // month/year + arrows row
	calendarWeekRowH = 20.0 // weekday-label row
	calendarCellW    = 32.0 // day cell width
	calendarCellH    = 30.0 // day cell height
	calendarPad      = 4.0  // outer padding
)

// calendarGridTop is the y of the first day-cell row (below the header and
// the weekday labels).
func calendarGridTop() float64 {
	return calendarPad + calendarHeaderH + calendarWeekRowH
}

// --- Events ---

func (this *Calendar) OnMouseLeave() {
	if this.hoverRow != -1 || this.hoverCol != -1 {
		this.hoverRow = -1
		this.hoverCol = -1
		this.Self().Update()
	}
}

func (this *Calendar) OnMouseMove(x, y float64) {
	row, col := this.hitCell(x, y)
	if row != this.hoverRow || col != this.hoverCol {
		this.hoverRow = row
		this.hoverCol = col
		this.Self().Update()
	}
}

func (this *Calendar) OnLeftDown(x, y float64) {
	this.SetFocus()
	w, _ := this.Size()

	// Header arrows: a fixed hit box at each end of the header row.
	if y >= calendarPad && y < calendarPad+calendarHeaderH {
		if x < calendarPad+calendarHeaderH {
			this.PrevMonth()
			return
		}
		if x > w-calendarPad-calendarHeaderH {
			this.NextMonth()
			return
		}
		return
	}

	// Day cells.
	row, col := this.hitCell(x, y)
	if row < 0 || col < 0 {
		return
	}
	grid := monthGrid(this.month.Year(), this.month.Month())
	if row >= len(grid) {
		return
	}
	this.selectDate(grid[row][col])
}

// selectDate applies a user-driven selection: it updates the selected day
// and fires SigDateSelected. Picking a day from an adjacent month also
// scrolls the grid to that month.
func (this *Calendar) selectDate(d time.Time) {
	d = dayStart(d)
	this.selected = d
	this.month = monthStart(d)
	if this.cbDateSelected != nil {
		this.cbDateSelected(d)
	}
	this.Self().Update()
}

// hitCell maps a point to a (row, col) in the day grid, or (-1, -1) when
// it falls outside the cells.
func (this *Calendar) hitCell(x, y float64) (int, int) {
	top := calendarGridTop()
	if y < top {
		return -1, -1
	}
	col := int((x - calendarPad) / calendarCellW)
	row := int((y - top) / calendarCellH)
	if col < 0 || col >= 7 || row < 0 {
		return -1, -1
	}
	grid := monthGrid(this.month.Year(), this.month.Month())
	if row >= len(grid) {
		return -1, -1
	}
	return row, col
}

// --- Drawing ---

// calendarWeekdayLabels are the Monday-first column headers in the zh-CN
// single-character form.
var calendarWeekdayLabels = []string{"一", "二", "三", "四", "五", "六", "日"}

func (this *Calendar) Draw(g paint.Painter) {
	t := Theme()
	w, _ := this.Size()
	g.SetFont(t.Font)

	this.drawHeader(g, t, w)
	this.drawWeekdays(g, t)
	this.drawGrid(g, t)
}

// drawHeader paints the "<  2026年6月  >" row.
func (this *Calendar) drawHeader(g paint.Painter, t *defaultTheme, w float64) {
	title := fmt.Sprintf("%d年%d月", this.month.Year(), int(this.month.Month()))
	cy := calendarPad + calendarHeaderH*0.5
	this.drawCentered(g, t.Font, title, t.TextColor, 0, w, calendarPad, calendarHeaderH)

	// Prev arrow (chevron pointing left).
	lx := calendarPad + calendarHeaderH*0.5
	this.drawChevron(g, t.TextColor, lx, cy, true)

	// Next arrow (chevron pointing right).
	rx := w - calendarPad - calendarHeaderH*0.5
	this.drawChevron(g, t.TextColor, rx, cy, false)
}

// drawWeekdays paints the Mon..Sun column header labels.
func (this *Calendar) drawWeekdays(g paint.Painter, t *defaultTheme) {
	y := calendarPad + calendarHeaderH
	label := paint.Color{120, 120, 120, 255}
	for col, name := range calendarWeekdayLabels {
		x := calendarPad + float64(col)*calendarCellW
		this.drawCentered(g, t.Font, name, label, x, calendarCellW, y, calendarWeekRowH)
	}
}

// drawGrid paints the day cells: dimmed for adjacent-month days, an accent
// fill for the selected day, an accent ring for today, and a hover wash.
func (this *Calendar) drawGrid(g paint.Painter, t *defaultTheme) {
	grid := monthGrid(this.month.Year(), this.month.Month())
	top := calendarGridTop()
	today := dayStart(time.Now())
	curMonth := this.month.Month()

	for row := range grid {
		for col := 0; col < 7; col++ {
			d := grid[row][col]
			x := calendarPad + float64(col)*calendarCellW
			y := top + float64(row)*calendarCellH

			inMonth := d.Month() == curMonth
			selected := d.Equal(this.selected)
			isToday := d.Equal(today)
			hovered := row == this.hoverRow && col == this.hoverCol

			// Cell background: accent fill for the selection, a subtle
			// wash on hover otherwise.
			if selected {
				calendarRoundRect(g, x+2, y+2, calendarCellW-4, calendarCellH-4, 4)
				g.SetBrush1(t.HighLightColor)
				g.Fill()
			} else if hovered {
				calendarRoundRect(g, x+2, y+2, calendarCellW-4, calendarCellH-4, 4)
				g.SetBrush1(paint.Color{230, 230, 245, 255})
				g.Fill()
			}

			// Today gets an accent ring (skipped when it's already the
			// filled selection — the fill is signal enough).
			if isToday && !selected {
				calendarRoundRect(g, x+2, y+2, calendarCellW-4, calendarCellH-4, 4)
				g.SetPen1(t.HighLightColor, 1.5)
				g.Stroke()
			}

			// Foreground colour: white on the accent fill, dimmed for
			// adjacent-month spill days, normal text otherwise.
			fg := t.TextColor
			if selected {
				fg = paint.Color{255, 255, 255, 255}
			} else if !inMonth {
				fg = paint.Color{180, 180, 180, 255}
			}

			text := fmt.Sprintf("%d", d.Day())
			this.drawCentered(g, t.Font, text, fg, x, calendarCellW, y, calendarCellH)
		}
	}
}

// drawCentered renders text centred in the box (x, y, boxW, boxH).
func (this *Calendar) drawCentered(g paint.Painter, f paint.Font, text string, fg paint.Color, x, boxW, y, boxH float64) {
	ext := f.TextExtents(text)
	tx := x + (boxW-ext.Width)*0.5 - ext.XBearing
	ty := y + 0.5*(boxH+ext.YBearing) - ext.YBearing
	g.SetBrush1(fg)
	g.DrawText1(tx, ty, text)
}

// drawChevron paints a small navigation chevron centred at (cx, cy).
// left=true points "‹" (previous month), false points "›" (next month).
func (this *Calendar) drawChevron(g paint.Painter, fg paint.Color, cx, cy float64, left bool) {
	d := 4.0
	if left {
		g.MoveTo(cx+d*0.5, cy-d)
		g.LineTo(cx-d*0.5, cy)
		g.LineTo(cx+d*0.5, cy+d)
	} else {
		g.MoveTo(cx-d*0.5, cy-d)
		g.LineTo(cx+d*0.5, cy)
		g.LineTo(cx-d*0.5, cy+d)
	}
	g.SetPen1(fg, 1.5)
	g.Stroke()
}

// calendarRoundRect emits a rounded-rect path (local shape helper, kept
// self-contained to match the other widgets in this package).
func calendarRoundRect(g paint.Painter, x, y, w, h, r float64) {
	g.MoveTo(x+r, y)
	g.LineTo(x+w-r, y)
	g.Arc(x+w-r, y+r, r, -math.Pi/2, 0)
	g.LineTo(x+w, y+h-r)
	g.Arc(x+w-r, y+h-r, r, 0, math.Pi/2)
	g.LineTo(x+r, y+h)
	g.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi)
	g.LineTo(x, y+r)
	g.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2)
	g.LineTo(x+r, y)
}

// --- SizeHints ---

func (this *Calendar) SizeHints() SizeHints {
	w := calendarPad*2 + calendarCellW*7
	// Reserve six week-rows so the footprint stays stable as the month
	// changes — a five-week month just leaves the last row blank.
	h := calendarGridTop() + calendarCellH*6 + calendarPad
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}

func (this *Calendar) EnumProperties(list core.IPropertyList) {
	list.AddProperty("年", func() int { return this.month.Year() },
		func(v int) { this.ShowMonth(v, this.month.Month()) })
	list.AddProperty("月", func() int { return int(this.month.Month()) },
		func(v int) { this.ShowMonth(this.month.Year(), time.Month(v)) })
}
