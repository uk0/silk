package gui

import (
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// AlarmPanel is a live operator alarm list for SCADA / 组态 screens: a
// scrollable list of a core.AlarmDB's active alarms, one row per tag, showing
// the severity (colour + label), tag, value, "active since" time and an ACK
// affordance. The host feeds an already-ordered snapshot via SetAlarms (or wires
// a live AlarmDB with BindAlarmDB); the panel does not re-sort, so the db's
// unacked-first / most-severe-first / oldest-first ordering shows through.
//
// It is deliberately UI-only: acknowledging is not done here. A click on a
// row's ACK affordance fires SigAckRequested(tag); the host calls db.Ack(tag),
// which raises a transition the bound panel then re-reads. This keeps the panel
// a pure view and leaves the ack lifecycle in the (thread-safe) db.
type AlarmPanel struct {
	Widget

	alarms    []core.AlarmState
	scrollY   float64
	rowHeight float64
	cbAck     func(tag string)
}

func init() {
	core.RegisterFactory("gui.AlarmPanel", core.TypeOf((*AlarmPanel)(nil)))
}

// NewAlarmPanel creates an empty alarm panel.
func NewAlarmPanel() *AlarmPanel {
	p := new(AlarmPanel)
	p.Init(p)
	return p
}

func (this *AlarmPanel) Init(self IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
}

// SetAlarms replaces the displayed alarm list with a defensive copy of in.
// AlarmState is a value type, so the shallow copy fully isolates the panel from
// later mutation of the caller's slice. The caller is expected to pass an
// already-ordered snapshot (typically AlarmDB.Active()); the panel renders it
// verbatim. The scroll offset is clamped to the new content rather than reset,
// so a live refresh does not yank the operator's view back to the top.
func (this *AlarmPanel) SetAlarms(in []core.AlarmState) {
	cp := make([]core.AlarmState, len(in))
	copy(cp, in)
	this.alarms = cp
	this.clampScroll()
	this.Self().Update()
}

// Alarms returns a defensive copy of the displayed alarms in display order.
func (this *AlarmPanel) Alarms() []core.AlarmState {
	out := make([]core.AlarmState, len(this.alarms))
	copy(out, this.alarms)
	return out
}

// BindAlarmDB wires the panel to db as a live view: it seeds the panel with
// db.Active() and subscribes for every future transition. AlarmDB subscribers
// fire on whatever goroutine drove the transition (a driver-poll goroutine, not
// the UI thread), so the callback marshals the refresh onto the event-loop
// thread via Post before touching the panel — mirroring the tag bindings. The
// returned func unsubscribes and is idempotent (it wraps the db's CancelFunc).
func (this *AlarmPanel) BindAlarmDB(db *core.AlarmDB) func() {
	this.SetAlarms(db.Active())
	return db.Subscribe(func(core.AlarmState) {
		Post(func() { this.SetAlarms(db.Active()) })
	})
}

// SigAckRequested registers the callback fired when the operator clicks a row's
// ACK affordance. It receives the alarm's tag; the host acknowledges by calling
// db.Ack(tag). Already-acked rows do not fire.
func (this *AlarmPanel) SigAckRequested(fn func(tag string)) {
	this.cbAck = fn
}

// alarmSeverityColor maps a SCADA alarm severity to its row accent colour: the
// LoLo/HiHi trip limits burn red, the Lo/Hi warnings amber, and None has no
// accent. The bool is false for None so the renderer can skip the glyph.
func alarmSeverityColor(sev core.AlarmSeverity) (paint.Color, bool) {
	switch sev {
	case core.LowLow, core.HighHigh:
		return paint.Color{R: 230, G: 80, B: 80, A: 255}, true // red trip
	case core.Low, core.High:
		return paint.Color{R: 230, G: 180, B: 60, A: 255}, true // amber warning
	default:
		return paint.Color{}, false // None: no accent
	}
}

// formatAlarmValue renders the alarm's tripping value compactly.
func formatAlarmValue(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// --- Drawing ---

const (
	alarmHeaderH = 22.0 // header band height
	alarmAckColW = 46.0 // width of the right-anchored ACK hit column
)

// Draw renders a count header followed by one row per active alarm.
func (this *AlarmPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling list panes.
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header: "N active · M unacked".
	unacked := 0
	for _, a := range this.alarms {
		if !a.Acked {
			unacked++
		}
	}
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, alarmHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	header := strconv.Itoa(len(this.alarms)) + " active · " + strconv.Itoa(unacked) + " unacked"
	g.DrawText1(8, fe.Ascent+4, header)

	if len(this.alarms) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := alarmHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.alarms); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		a := this.alarms[i]

		// Alternating row stripe.
		if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		col, on := alarmSeverityColor(a.Severity)
		baseline := y + fe.Ascent + 4

		// Severity accent bar + label in the severity colour.
		if on {
			g.SetBrush1(col)
			g.Rectangle(0, y, 4, rh)
			g.Fill()
			g.SetBrush1(col)
		} else {
			g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
		}
		g.DrawText1(12, baseline, a.Severity.String())

		// Tag in light grey.
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		g.DrawText1(56, baseline, a.Tag)

		// Value, then "active since" time, right-anchored ahead of the ACK column.
		g.SetBrush1(paint.Color{R: 175, G: 180, B: 190, A: 255})
		g.DrawText1(w-alarmAckColW-150, baseline, formatAlarmValue(a.Value))
		g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
		g.DrawText1(w-alarmAckColW-76, baseline, a.Since.Format("15:04:05"))

		// ACK affordance: a bordered "ACK" while unacked, a green check once acked.
		if a.Acked {
			g.SetBrush1(paint.Color{R: 110, G: 200, B: 110, A: 255})
			g.DrawText1(w-alarmAckColW+12, baseline, "✓")
		} else {
			amber := paint.Color{R: 230, G: 180, B: 60, A: 255}
			g.SetPen1(amber, 1)
			g.Rectangle(w-alarmAckColW+4, y+4, alarmAckColW-10, rh-8)
			g.Stroke()
			g.SetBrush1(amber)
			g.DrawText1(w-alarmAckColW+9, baseline, "ACK")
		}
	}
}

// --- Events ---

// OnLeftDown fires SigAckRequested when the click lands in an unacked row's ACK
// column. Clicks elsewhere on a row, on the header, or past the last row are
// ignored.
func (this *AlarmPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAtY(y)
	if idx < 0 || idx >= len(this.alarms) {
		return
	}
	a := this.alarms[idx]
	w, _ := this.Size()
	if ackColumnHit(x, w) && !a.Acked && this.cbAck != nil {
		this.cbAck(a.Tag)
	}
}

// OnMouseWheel scrolls the row list vertically.
func (this *AlarmPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	this.clampScroll()
	this.Self().Update()
}

// rowAtY maps a y coordinate to an alarm-row index, accounting for the header
// band and the current scroll offset; it returns -1 when y lands on the header.
// Pure geometry (only scrollY and rowHeight), so it is unit-testable headless.
// The index may exceed the row count for a click past the last row — callers
// bound-check against len(alarms).
func (this *AlarmPanel) rowAtY(y float64) int {
	if y < alarmHeaderH {
		return -1
	}
	return int((y - alarmHeaderH + this.scrollY) / this.rowHeight)
}

// ackColumnHit reports whether x falls in the right-anchored ACK column of a
// panel of width w. Kept pure so the click routing is unit-testable.
func ackColumnHit(x, w float64) bool {
	return x >= w-alarmAckColW
}

// clampScroll pins scrollY within [0, maxScroll] for the current content and
// viewport height.
func (this *AlarmPanel) clampScroll() {
	_, h := this.Size()
	maxScroll := float64(len(this.alarms))*this.rowHeight - (h - alarmHeaderH)
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

func (this *AlarmPanel) SizeHints() SizeHints {
	return SizeHints{MinWidth: 240, MinHeight: 80}
}
