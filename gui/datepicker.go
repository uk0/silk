package gui

import (
	"fmt"
	"silk/core"
	"silk/paint"
	"math"
	"time"
)

func init() {
	core.RegisterFactory("gui.DatePicker", core.TypeOf((*DatePicker)(nil)))
}

// DatePicker is a date selection widget that shows the current date as text.
// Clicking it opens a dropdown calendar popup for selecting a day.
type DatePicker struct {
	Widget
	year          int
	month         int
	day           int
	format        string
	pushed        bool
	cbDateChanged func(year, month, day int)
	popup         *dateCalendarPopup
}

// NewDatePicker creates a new DatePicker initialized to today's date.
func NewDatePicker() *DatePicker {
	p := new(DatePicker)
	p.Init(p)
	now := time.Now()
	p.year = now.Year()
	p.month = int(now.Month())
	p.day = now.Day()
	p.format = "2006-01-02"
	return p
}

// Year returns the currently selected year.
func (this *DatePicker) Year() int { return this.year }

// Month returns the currently selected month (1-12).
func (this *DatePicker) Month() int { return this.month }

// Day returns the currently selected day (1-31).
func (this *DatePicker) Day() int { return this.day }

// SetDate sets the date value.
func (this *DatePicker) SetDate(year, month, day int) {
	if month < 1 {
		month = 1
	}
	if month > 12 {
		month = 12
	}
	maxDay := daysInMonth(year, month)
	if day < 1 {
		day = 1
	}
	if day > maxDay {
		day = maxDay
	}
	changed := this.year != year || this.month != month || this.day != day
	this.year = year
	this.month = month
	this.day = day
	if changed && this.cbDateChanged != nil {
		this.cbDateChanged(year, month, day)
	}
	this.Self().Update()
}

// Format returns the display format string.
func (this *DatePicker) Format() string { return this.format }

// SetFormat sets the display format string.
func (this *DatePicker) SetFormat(f string) {
	this.format = f
	this.Self().Update()
}

// SigDateChanged sets the callback for when the date changes.
func (this *DatePicker) SigDateChanged(fn func(year, month, day int)) {
	this.cbDateChanged = fn
}

func (this *DatePicker) displayText() string {
	return fmt.Sprintf("%04d-%02d-%02d", this.year, this.month, this.day)
}

// --- Drawing ---

func (this *DatePicker) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()

	// Background
	g.Rectangle(0, 0, w, h)
	if this.pushed {
		g.SetBrush1(paint.Color{220, 220, 220, 255})
	} else if this.IsHover() {
		g.SetBrush1(paint.Color{235, 235, 235, 255})
	} else {
		g.SetBrush1(paint.Color{245, 245, 245, 255})
	}
	g.Fill()

	// Border
	g.Rectangle(0, 0, w, h)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// Text
	text := this.displayText()
	g.SetFont(t.Font)
	g.SetBrush1(t.TextColor)
	fe := t.Font.FontExtents()
	ext := t.Font.TextExtents(text)
	tx := 6.0
	ty := 0.5*(h+ext.YBearing) - ext.YBearing
	g.Translate(tx-ext.XBearing, ty)
	g.DrawText(text)
	g.Translate(-(tx - ext.XBearing), -ty)

	// Dropdown arrow
	arrowX := w - 16
	arrowY := h * 0.5
	g.MoveTo(arrowX, arrowY-3)
	g.LineTo(arrowX+6, arrowY-3)
	g.LineTo(arrowX+3, arrowY+3)
	g.LineTo(arrowX, arrowY-3)
	g.SetBrush1(t.TextColor)
	g.Fill()

	_ = fe
}

// --- Events ---

func (this *DatePicker) OnMouseEnter() { this.Self().Update() }
func (this *DatePicker) OnMouseLeave() { this.Self().Update() }

func (this *DatePicker) OnLeftDown(x, y float64) {
	this.SetFocus()
	this.pushed = true
	this.Self().Update()
	this.showPopup()
}

func (this *DatePicker) OnLeftUp(x, y float64) {
	this.pushed = false
	this.Self().Update()
}

func (this *DatePicker) showPopup() {
	if this.popup != nil && this.popup.IsVisible() {
		this.popup.Hide()
		return
	}
	popup := newDateCalendarPopup(this)
	this.popup = popup
	gx, gy := this.MapToGlobal(0, this.h)
	popup.ShowAsPopup(gx, gy)
}

func (this *DatePicker) SizeHints() SizeHints {
	return SizeHints{Width: 150, Height: 26, Policy: GrowHorizontal | GrowVertical}
}

func (this *DatePicker) EnumProperties(list core.IPropertyList) {
	list.AddProperty("Year", this.Year, func(v int) { this.SetDate(v, this.month, this.day) })
	list.AddProperty("Month", this.Month, func(v int) { this.SetDate(this.year, v, this.day) })
	list.AddProperty("Day", this.Day, func(v int) { this.SetDate(this.year, this.month, v) })
}

// --- Calendar Popup ---

type dateCalendarPopup struct {
	Widget
	owner     *DatePicker
	viewYear  int
	viewMonth int
	hoverDay  int
}

func newDateCalendarPopup(owner *DatePicker) *dateCalendarPopup {
	p := new(dateCalendarPopup)
	p.Init(p)
	p.owner = owner
	p.viewYear = owner.year
	p.viewMonth = owner.month
	p.hoverDay = -1
	p.SetParent(owner)
	return p
}

const (
	calCellW   = 32.0
	calCellH   = 24.0
	calHeaderH = 30.0
	calDayRowH = 18.0
	calCols    = 7
	calRows    = 6
)

func (this *dateCalendarPopup) ShowAsPopup(xg, yg float64) {
	this.AttachWindow(WtPopup)
	if w := this.Window(); w != nil {
		w.SetCloseOnHide(true)
	}
	w := calCellW*calCols + 8
	h := calHeaderH + calDayRowH + calCellH*calRows + 8
	this.SetSize(0, 0)
	this.SetSize(w, h)
	LayoutPopup1(this.Self(), xg, yg)
	this.SetVisible(true)
	this.PushCapture()
}

func (this *dateCalendarPopup) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()

	// Background
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(paint.Color{255, 255, 255, 255})
	g.FillPreserve()
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	g.SetFont(t.Font)
	fe := t.Font.FontExtents()

	// Header: < month/year >
	headerText := fmt.Sprintf("%04d-%02d", this.viewYear, this.viewMonth)
	ext := t.Font.TextExtents(headerText)
	hx := (w - ext.Width) * 0.5
	hy := 0.5*(calHeaderH+ext.YBearing) - ext.YBearing
	g.SetBrush1(t.TextColor)
	g.Translate(hx-ext.XBearing, hy)
	g.DrawText(headerText)
	g.Translate(-(hx - ext.XBearing), -hy)

	// Left arrow
	g.MoveTo(14, calHeaderH*0.5)
	g.LineTo(20, calHeaderH*0.5-5)
	g.LineTo(20, calHeaderH*0.5+5)
	g.LineTo(14, calHeaderH*0.5)
	g.SetBrush1(t.TextColor)
	g.Fill()

	// Right arrow
	g.MoveTo(w-14, calHeaderH*0.5)
	g.LineTo(w-20, calHeaderH*0.5-5)
	g.LineTo(w-20, calHeaderH*0.5+5)
	g.LineTo(w-14, calHeaderH*0.5)
	g.SetBrush1(t.TextColor)
	g.Fill()

	// Day-of-week headers
	dayNames := []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	for i, name := range dayNames {
		dx := 4 + float64(i)*calCellW + calCellW*0.5
		dy := calHeaderH + 0.5*(calDayRowH+fe.Ascent-fe.Descent)
		dext := t.Font.TextExtents(name)
		g.SetBrush1(paint.Color{120, 120, 120, 255})
		tx := dx - dext.Width*0.5 - dext.XBearing
		ty := dy - fe.Ascent + fe.Descent
		ty = calHeaderH + 0.5*(calDayRowH+dext.YBearing) - dext.YBearing
		g.Translate(tx, ty)
		g.DrawText(name)
		g.Translate(-tx, -ty)
	}

	// Calendar grid
	startY := calHeaderH + calDayRowH
	firstWeekday := dayOfWeek(this.viewYear, this.viewMonth, 1)
	maxDay := daysInMonth(this.viewYear, this.viewMonth)

	selectedDay := -1
	if this.owner.year == this.viewYear && this.owner.month == this.viewMonth {
		selectedDay = this.owner.day
	}

	day := 1
	for row := 0; row < calRows; row++ {
		for col := 0; col < calCols; col++ {
			cellIdx := row*calCols + col
			if cellIdx < firstWeekday || day > maxDay {
				continue
			}

			cx := 4 + float64(col)*calCellW
			cy := startY + float64(row)*calCellH

			// Highlight selected day
			if day == selectedDay {
				g.Rectangle(cx, cy, calCellW, calCellH)
				g.SetBrush1(t.HighLightColor)
				g.Fill()
			} else if day == this.hoverDay {
				g.Rectangle(cx, cy, calCellW, calCellH)
				g.SetBrush1(paint.Color{230, 230, 245, 255})
				g.Fill()
			}

			// Day text
			dayText := fmt.Sprintf("%d", day)
			dext := t.Font.TextExtents(dayText)
			dx := cx + (calCellW-dext.Width)*0.5 - dext.XBearing
			dy := cy + 0.5*(calCellH+dext.YBearing) - dext.YBearing

			if day == selectedDay {
				g.SetBrush1(paint.Color{255, 255, 255, 255})
			} else {
				g.SetBrush1(t.TextColor)
			}
			g.Translate(dx, dy)
			g.DrawText(dayText)
			g.Translate(-dx, -dy)

			day++
		}
	}
}

func (this *dateCalendarPopup) hitTestDay(x, y float64) int {
	startY := calHeaderH + calDayRowH
	if y < startY {
		return -1
	}
	col := int((x - 4) / calCellW)
	row := int((y - startY) / calCellH)
	if col < 0 || col >= calCols || row < 0 || row >= calRows {
		return -1
	}
	firstWeekday := dayOfWeek(this.viewYear, this.viewMonth, 1)
	cellIdx := row*calCols + col
	day := cellIdx - firstWeekday + 1
	maxDay := daysInMonth(this.viewYear, this.viewMonth)
	if day < 1 || day > maxDay {
		return -1
	}
	return day
}

func (this *dateCalendarPopup) OnLeftDown(x, y float64) {
	w, _ := this.Size()

	// Check if outside popup
	if x < 0 || y < 0 || x >= w || y >= this.h {
		this.PopCapture()
		this.Hide()
		emulateMouseDown(true)
		return
	}

	// Check header navigation
	if y < calHeaderH {
		if x < 30 {
			this.prevMonth()
			return
		}
		if x > w-30 {
			this.nextMonth()
			return
		}
		return
	}

	// Check day click
	day := this.hitTestDay(x, y)
	if day > 0 {
		this.owner.SetDate(this.viewYear, this.viewMonth, day)
		this.PopCapture()
		this.Hide()
	}
}

func (this *dateCalendarPopup) OnMouseMove(x, y float64) {
	day := this.hitTestDay(x, y)
	if day != this.hoverDay {
		this.hoverDay = day
		this.Self().Update()
	}
}

func (this *dateCalendarPopup) prevMonth() {
	this.viewMonth--
	if this.viewMonth < 1 {
		this.viewMonth = 12
		this.viewYear--
	}
	this.Self().Update()
}

func (this *dateCalendarPopup) nextMonth() {
	this.viewMonth++
	if this.viewMonth > 12 {
		this.viewMonth = 1
		this.viewYear++
	}
	this.Self().Update()
}

// --- Date utilities ---

// daysInMonth returns the number of days in the given month.
func daysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeapYear(year) {
			return 29
		}
		return 28
	}
	return 30
}

// isLeapYear returns whether the given year is a leap year.
func isLeapYear(year int) bool {
	return (year%4 == 0 && year%100 != 0) || year%400 == 0
}

// dayOfWeek returns the day of week for the given date.
// Returns 0 for Monday, 6 for Sunday (ISO week day).
func dayOfWeek(year, month, day int) int {
	// Tomohiko Sakamoto's algorithm
	t := []int{0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4}
	y := year
	if month < 3 {
		y--
	}
	w := (y + y/4 - y/100 + y/400 + t[month-1] + day) % 7
	// Convert from Sunday=0 to Monday=0
	w = (w + 6) % 7
	return w
}

// Ensure math is used
var _ = math.Pi
