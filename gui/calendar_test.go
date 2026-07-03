package gui

import (
	"testing"
	"time"

	"github.com/uk0/silk/paint"
)

// date is a tiny local helper so the table tests read as plain calendar
// days without repeating the time.Date(... 0,0,0,0, Local) boilerplate.
func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// TestFirstWeekdayOffset locks the Monday-first column mapping. June 2026
// starts on a Monday (offset 0); a month whose 1st is a Sunday must push
// to the last column (offset 6).
func TestCalendarFirstWeekdayOffset(t *testing.T) {
	// 2026-06-01 is a Monday.
	if got := firstWeekdayOffset(2026, time.June); got != 0 {
		t.Errorf("firstWeekdayOffset(2026, Jun) = %d, want 0 (Mon)", got)
	}
	// 2026-02-01 is a Sunday → offset 6 in a Monday-first grid.
	if got := firstWeekdayOffset(2026, time.February); got != 6 {
		t.Errorf("firstWeekdayOffset(2026, Feb) = %d, want 6 (Sun)", got)
	}
	// 2026-03-01 is a Sunday too; 2026-11-01 is a Sunday — spot a Saturday
	// start: 2026-08-01 is a Saturday → offset 5.
	if got := firstWeekdayOffset(2026, time.August); got != 5 {
		t.Errorf("firstWeekdayOffset(2026, Aug) = %d, want 5 (Sat)", got)
	}
}

// TestMonthGridJune2026 checks a month that starts cleanly on a Monday:
// the first cell is the 1st itself (no leading spill), 30 days span five
// full weeks, and the last cell is the 5th of the next month filling out
// the final row.
func TestCalendarMonthGridJune2026(t *testing.T) {
	g := monthGrid(2026, time.June)

	if len(g) != 5 {
		t.Fatalf("June 2026 grid rows = %d, want 5", len(g))
	}
	for r, week := range g {
		if len(week) != 7 {
			t.Fatalf("row %d width = %d, want 7", r, len(week))
		}
	}
	// First cell: Monday 2026-06-01 (offset 0, no leading days).
	if first := g[0][0]; !first.Equal(date(2026, time.June, 1)) {
		t.Errorf("first cell = %v, want 2026-06-01", first.Format("2006-01-02"))
	}
	// Last cell of a 5-week grid: 35 cells total, 30 in month → 5 trailing
	// days from July, so the bottom-right is 2026-07-05.
	if last := g[4][6]; !last.Equal(date(2026, time.July, 5)) {
		t.Errorf("last cell = %v, want 2026-07-05", last.Format("2006-01-02"))
	}
	// The 30th must appear in the month and be the last in-month day.
	if d := g[4][1]; !d.Equal(date(2026, time.June, 30)) {
		t.Errorf("cell [4][1] = %v, want 2026-06-30", d.Format("2006-01-02"))
	}
}

// TestMonthGridLeapFebruary verifies leap-year February (Feb 2024 = 29
// days) carries the 29th and no spurious March 1st duplication. 2024-02-01
// is a Thursday → offset 3.
func TestCalendarMonthGridLeapFebruary(t *testing.T) {
	g := monthGrid(2024, time.February)

	if off := firstWeekdayOffset(2024, time.February); off != 3 {
		t.Fatalf("Feb 2024 offset = %d, want 3 (Thu)", off)
	}
	// First cell sits 3 days before Feb 1 → 2024-01-29.
	if first := g[0][0]; !first.Equal(date(2024, time.January, 29)) {
		t.Errorf("first cell = %v, want 2024-01-29", first.Format("2006-01-02"))
	}
	// 3 leading + 29 days = 32 cells → 5 weeks (35 cells).
	if len(g) != 5 {
		t.Fatalf("Feb 2024 grid rows = %d, want 5", len(g))
	}
	// The 29th must be present exactly once and be the last February day.
	feb29Count := 0
	var feb29 time.Time
	for _, week := range g {
		for _, d := range week {
			if d.Month() == time.February && d.Day() == 29 {
				feb29Count++
				feb29 = d
			}
		}
	}
	if feb29Count != 1 {
		t.Errorf("Feb 29 appears %d times, want 1", feb29Count)
	}
	if feb29Count == 1 && !feb29.Equal(date(2024, time.February, 29)) {
		t.Errorf("Feb 29 cell = %v, want 2024-02-29", feb29.Format("2006-01-02"))
	}
	// Non-leap 2023 February must NOT have a 29th.
	for _, week := range monthGrid(2023, time.February) {
		for _, d := range week {
			if d.Month() == time.February && d.Day() == 29 {
				t.Errorf("Feb 2023 should have no 29th")
			}
		}
	}
}

// TestMonthGridMondayStartAlignsFlush confirms a month whose 1st is a
// Monday puts that 1st in column 0 with zero leading spill — the cleanest
// alignment case.
func TestCalendarMonthGridMondayStartAlignsFlush(t *testing.T) {
	// 2026-06-01 is a Monday.
	g := monthGrid(2026, time.June)
	if g[0][0].Month() != time.June || g[0][0].Day() != 1 {
		t.Errorf("Monday-start month: cell [0][0] = %v, want the 1st in col 0",
			g[0][0].Format("2006-01-02"))
	}
}

// TestMonthGridSundayStartFillsFullSixWeeks checks the worst-case layout:
// a 31-day month whose 1st lands on a Sunday needs 6 weeks (offset 6 + 31
// = 37 cells → 42). 2025-06-01 is a Sunday.
func TestCalendarMonthGridSundayStartSixWeeks(t *testing.T) {
	if off := firstWeekdayOffset(2025, time.June); off != 6 {
		t.Fatalf("Jun 2025 offset = %d, want 6 (Sun)", off)
	}
	g := monthGrid(2025, time.March) // 2025-03-01 is a Saturday (offset 5), 31 days → 6 weeks
	if off := firstWeekdayOffset(2025, time.March); off != 5 {
		t.Fatalf("Mar 2025 offset = %d, want 5 (Sat)", off)
	}
	if len(g) != 6 {
		t.Errorf("Mar 2025 grid rows = %d, want 6", len(g))
	}
}

// TestNextPrevMonthRollover exercises the year-boundary arithmetic in both
// directions: Dec → Jan bumps the year, Jan → Dec drops it.
func TestCalendarNextPrevMonthRollover(t *testing.T) {
	c := NewCalendar()

	// December 2026 → January 2027.
	c.ShowMonth(2026, time.December)
	c.NextMonth()
	if got := c.DisplayedMonth(); got.Year() != 2027 || got.Month() != time.January {
		t.Errorf("Dec 2026 +1 = %v, want 2027-01", got.Format("2006-01"))
	}

	// January 2027 → December 2026 (back across the boundary).
	c.ShowMonth(2027, time.January)
	c.PrevMonth()
	if got := c.DisplayedMonth(); got.Year() != 2026 || got.Month() != time.December {
		t.Errorf("Jan 2027 -1 = %v, want 2026-12", got.Format("2006-01"))
	}
}

// TestSetSelectedDateFiresNoCallback locks the contract that programmatic
// selection is silent — only user clicks fire SigDateSelected. It also
// confirms the getter round-trips the value (day-truncated).
func TestCalendarSetSelectedDateFiresNoCallback(t *testing.T) {
	c := NewCalendar()
	fired := false
	c.SigDateSelected(func(time.Time) { fired = true })

	want := date(2026, time.June, 15)
	c.SetSelectedDate(want)
	if fired {
		t.Errorf("SetSelectedDate must not fire SigDateSelected")
	}
	if got := c.SelectedDate(); !got.Equal(want) {
		t.Errorf("SelectedDate() = %v, want %v",
			got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
	// The displayed month should follow the selection.
	if m := c.DisplayedMonth(); m.Year() != 2026 || m.Month() != time.June {
		t.Errorf("DisplayedMonth after select = %v, want 2026-06", m.Format("2006-01"))
	}
}

// TestSetSelectedDateTruncatesClock verifies a date carrying a clock part
// round-trips as its midnight day, so equality checks in Draw line up.
func TestCalendarSetSelectedDateTruncatesClock(t *testing.T) {
	c := NewCalendar()
	withClock := time.Date(2026, time.June, 15, 13, 45, 30, 0, time.Local)
	c.SetSelectedDate(withClock)
	if got := c.SelectedDate(); !got.Equal(date(2026, time.June, 15)) {
		t.Errorf("SelectedDate() = %v, want 2026-06-15 (clock stripped)",
			got.Format("2006-01-02 15:04:05"))
	}
}

// TestSelectDateFiresCallback covers the user-click path: the internal
// selectDate (what OnLeftDown calls on a day cell) must fire the callback
// with the clicked date and update both the selection and the view.
func TestCalendarSelectDateFiresCallback(t *testing.T) {
	c := NewCalendar()
	var got time.Time
	count := 0
	c.SigDateSelected(func(d time.Time) {
		got = d
		count++
	})

	c.selectDate(date(2026, time.July, 4))
	if count != 1 {
		t.Fatalf("selectDate fired callback %d times, want 1", count)
	}
	if !got.Equal(date(2026, time.July, 4)) {
		t.Errorf("callback date = %v, want 2026-07-04", got.Format("2006-01-02"))
	}
	if !c.SelectedDate().Equal(date(2026, time.July, 4)) {
		t.Errorf("selection not updated after click")
	}
}

// TestCalendarSizeHints documents the default footprint: seven columns
// wide and six week-rows tall (plus header + weekday row + padding).
func TestCalendarSizeHints(t *testing.T) {
	c := NewCalendar()
	h := c.SizeHints()
	wantW := calendarPad*2 + calendarCellW*7
	wantH := calendarGridTop() + calendarCellH*6 + calendarPad
	if h.Width != wantW || h.Height != wantH {
		t.Errorf("SizeHints = %v×%v, want %v×%v", h.Width, h.Height, wantW, wantH)
	}
}

// TestCalendarDrawNoPanic is a smoke test: drawing across a couple of
// months (including ones with leading/trailing spill) must not panic. We
// drive a nil-safe painter shim since there's no GL surface in a unit
// test.
func TestCalendarDrawNoPanic(t *testing.T) {
	c := NewCalendar()
	c.SetSize(240, 240)
	c.SetSelectedDate(date(2026, time.June, 15))

	rec := calendarNopPainter{}
	months := []struct {
		y int
		m time.Month
	}{
		{2026, time.June},     // Monday start, 5 weeks
		{2025, time.March},    // Saturday start, 6 weeks
		{2024, time.February}, // leap February
	}
	for _, mm := range months {
		c.ShowMonth(mm.y, mm.m)
		c.Draw(rec) // must not panic
	}
}

// calendarNopPainter satisfies paint.Painter with no-op stubs (embeds a
// nil Painter) so Draw can run without a render target. Only the methods
// Calendar.Draw actually calls need behaviour; the embedded nil supplies
// the rest, which Draw never reaches. Models the spinner_test nopPainter
// pattern.
type calendarNopPainter struct{ paint.Painter }

func (calendarNopPainter) Arc(xc, yc, radius, angle1, angle2 float64) {}
func (calendarNopPainter) MoveTo(x, y float64)                        {}
func (calendarNopPainter) LineTo(x, y float64)                        {}
func (calendarNopPainter) Fill()                                      {}
func (calendarNopPainter) Stroke()                                    {}
func (calendarNopPainter) SetBrush1(c paint.Color)                    {}
func (calendarNopPainter) SetPen1(c paint.Color, width float64)       {}
func (calendarNopPainter) SetFont(f paint.Font)                       {}
func (calendarNopPainter) DrawText1(x, y float64, text string)        {}
