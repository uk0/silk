package gui

import (
	"fmt"
	"time"

	"silk/core"
	"silk/paint"
)

func init() {
	core.RegisterFactory("gui.TimePicker", core.TypeOf((*TimePicker)(nil)))
}

// TimePicker is a compact hours:minutes (optionally :seconds) time selector
// with per-field up/down steppers, modelled on Qt's QTimeEdit. The value is
// three plain ints (hour 0-23, minute 0-59, second 0-59) rather than a full
// time.Time, since only the clock part matters here.
//
// Each field renders as a small box reading "HH" / "MM" (/ "SS") with a pair
// of stepper arrows on its right edge. The currently focused field gets an
// accent ring. Interaction mirrors QTimeEdit:
//
//   - Clicking a field focuses it; clicking that field's up/down arrow steps
//     it by one with wraparound.
//   - Up/Down step the focused field; Left/Right move focus between fields.
//   - The mouse wheel steps whichever field sits under the cursor.
//
// Wraparound rule: each field wraps within its own range and does NOT carry
// into the next field (minute 59→00 leaves the hour untouched), matching the
// simplest QTimeEdit-style per-section spin.
//
// Usage:
//
//	tp := gui.NewTimePicker()
//	tp.SigTimeChanged(func(h, m, s int) { label.SetText(fmt.Sprintf("%02d:%02d", h, m)) })
//
// TimePicker pairs with Calendar/DatePicker to cover the clock side of a
// date-time selection.
type TimePicker struct {
	Widget

	hour   int // 0-23
	minute int // 0-59
	second int // 0-59

	showSeconds bool

	focusField int // 0=hour, 1=minute, 2=second
	hoverField int // field under the cursor, -1 when none
	hoverUp    bool // the hovered stepper half (true=up arrow, false=down arrow)

	cbTimeChanged func(h, m, s int)
}

// NewTimePicker creates a TimePicker initialised to the current local
// wall-clock time (hours:minutes), with the seconds field hidden. The hour
// field starts focused.
func NewTimePicker() *TimePicker {
	p := new(TimePicker)
	p.Init(p)
	now := time.Now()
	p.hour = now.Hour()
	p.minute = now.Minute()
	p.second = now.Second()
	p.hoverField = -1
	return p
}

func (this *TimePicker) Init(self IWidget) {
	this.Widget.Init(self)
	this.hoverField = -1
}

func (this *TimePicker) EnumProperties(list core.IPropertyList) {
	list.AddProperty("时", this.Hour, func(v int) { this.SetTime(v, this.minute, this.second) })
	list.AddProperty("分", this.Minute, func(v int) { this.SetTime(this.hour, v, this.second) })
	list.AddProperty("秒", this.Second, func(v int) { this.SetTime(this.hour, this.minute, v) })
	list.AddProperty("显示秒", this.ShowSeconds, this.SetShowSeconds)
}

// Hour returns the selected hour (0-23).
func (this *TimePicker) Hour() int { return this.hour }

// Minute returns the selected minute (0-59).
func (this *TimePicker) Minute() int { return this.minute }

// Second returns the selected second (0-59).
func (this *TimePicker) Second() int { return this.second }

// ShowSeconds reports whether the seconds field is shown.
func (this *TimePicker) ShowSeconds() bool { return this.showSeconds }

// SetTime sets the time, normalising each field into its valid range by
// wrapping (euclidean modulo): SetTime(25, 70, 0) becomes 01:10:00. Negative
// values wrap too, so SetTime(-1, 0, 0) is 23:00:00. Fires SigTimeChanged
// only when the normalised value actually differs from the current one.
func (this *TimePicker) SetTime(h, m, s int) {
	h = wrapInto(h, 24)
	m = wrapInto(m, 60)
	s = wrapInto(s, 60)
	if h == this.hour && m == this.minute && s == this.second {
		return
	}
	this.hour = h
	this.minute = m
	this.second = s
	this.Self().Update()
	if this.cbTimeChanged != nil {
		this.cbTimeChanged(this.hour, this.minute, this.second)
	}
}

// SetShowSeconds toggles the seconds field. Hiding it keeps the stored second
// value; it simply stops rendering and stops being a focus target. When the
// seconds field is hidden while focused, focus falls back to the minute.
func (this *TimePicker) SetShowSeconds(show bool) {
	if show == this.showSeconds {
		return
	}
	this.showSeconds = show
	if !show && this.focusField == 2 {
		this.focusField = 1
	}
	this.Self().Update()
}

// SigTimeChanged registers the callback fired when the time changes through a
// real edit (stepper click, wheel, or key). Programmatic SetTime that lands on
// a different value fires it too; a no-op SetTime does not.
func (this *TimePicker) SigTimeChanged(fn func(h, m, s int)) {
	this.cbTimeChanged = fn
}

// --- Pure field helpers (unit-testable without GL) ---

// wrapField returns val+delta wrapped into the inclusive range [0, max], so a
// step off either end rolls around (e.g. wrapField(59, 1, 59) == 0 and
// wrapField(0, -1, 59) == 59). delta is expected to be small (±1), but any
// magnitude wraps correctly via modulo over the (max+1)-sized ring.
func wrapField(val, delta, max int) int {
	n := max + 1
	v := (val + delta) % n
	if v < 0 {
		v += n
	}
	return v
}

// wrapInto folds val into [0, mod) by euclidean modulo. Used by SetTime to
// normalise out-of-range hour/minute/second arguments.
func wrapInto(val, mod int) int {
	v := val % mod
	if v < 0 {
		v += mod
	}
	return v
}

// stepFocused steps the currently focused field by delta with wraparound and
// routes the result through SetTime so the change callback fires consistently.
func (this *TimePicker) stepFocused(delta int) {
	this.stepField(this.focusField, delta)
}

// stepField steps the given field (0=hour, 1=minute, 2=second) by delta with
// wraparound, leaving the other fields untouched.
func (this *TimePicker) stepField(field, delta int) {
	switch field {
	case 0:
		this.SetTime(wrapField(this.hour, delta, 23), this.minute, this.second)
	case 1:
		this.SetTime(this.hour, wrapField(this.minute, delta, 59), this.second)
	case 2:
		this.SetTime(this.hour, this.minute, wrapField(this.second, delta, 59))
	}
}

// fieldCount is 2 (HH:MM) or 3 (HH:MM:SS) depending on showSeconds.
func (this *TimePicker) fieldCount() int {
	if this.showSeconds {
		return 3
	}
	return 2
}

// --- Layout metrics ---

const (
	timeFieldW    = 30.0 // width of one HH/MM/SS box
	timeStepperW  = 12.0 // width of the stepper-arrow column inside a field box
	timeSepW      = 10.0 // width of the ":" separator between fields
	timePickerPad = 3.0  // outer padding
)

// fieldX returns the left x of field i (0-based) inside the widget.
func (this *TimePicker) fieldX(i int) float64 {
	return timePickerPad + float64(i)*(timeFieldW+timeSepW)
}

// --- Events ---

func (this *TimePicker) OnMouseLeave() {
	if this.hoverField != -1 {
		this.hoverField = -1
		this.Self().Update()
	}
}

func (this *TimePicker) OnMouseMove(x, y float64) {
	field, up := this.hitField(x, y)
	if field != this.hoverField || up != this.hoverUp {
		this.hoverField = field
		this.hoverUp = up
		this.Self().Update()
	}
}

func (this *TimePicker) OnLeftDown(x, y float64) {
	this.SetFocus()
	field, up := this.hitField(x, y)
	if field < 0 {
		return
	}
	this.focusField = field
	// A click landing on the stepper column also steps the field.
	if this.inStepper(x, field) {
		if up {
			this.stepField(field, 1)
		} else {
			this.stepField(field, -1)
		}
	}
	this.Self().Update()
}

func (this *TimePicker) OnMouseWheel(x, y, z float64) {
	field, _ := this.hitField(x, y)
	if field < 0 {
		field = this.focusField
	}
	if z > 0 {
		this.stepField(field, 1)
	} else if z < 0 {
		this.stepField(field, -1)
	}
}

// OnKeyDown gives QTimeEdit-style keyboard control while focused: Up/Down step
// the focused field with wraparound, Left/Right move focus between fields.
func (this *TimePicker) OnKeyDown(key int, repeat bool) {
	if !this.IsEnabled() {
		return
	}
	switch key {
	case KeyUp:
		this.stepFocused(1)
	case KeyDown:
		this.stepFocused(-1)
	case KeyLeft:
		if this.focusField > 0 {
			this.focusField--
			this.Self().Update()
		}
	case KeyRight:
		if this.focusField < this.fieldCount()-1 {
			this.focusField++
			this.Self().Update()
		}
	}
}

// hitField maps a point to a field index (0=hour, 1=minute, 2=second) and
// which stepper half (up vs down) the point falls in, or (-1, false) when the
// point is outside every field box. The up/down split is the box mid-line, so
// clicking anywhere in a field's top half counts as "up".
func (this *TimePicker) hitField(x, y float64) (int, bool) {
	_, h := this.Size()
	for i := 0; i < this.fieldCount(); i++ {
		fx := this.fieldX(i)
		if x >= fx && x < fx+timeFieldW {
			return i, y < h*0.5
		}
	}
	return -1, false
}

// inStepper reports whether x falls in field i's stepper-arrow column (the
// right slice of the box), where a click should step rather than just focus.
func (this *TimePicker) inStepper(x float64, i int) bool {
	fx := this.fieldX(i)
	return x >= fx+timeFieldW-timeStepperW && x < fx+timeFieldW
}

// --- Drawing ---

func (this *TimePicker) Draw(g paint.Painter) {
	t := Theme()
	_, h := this.Size()
	g.SetFont(t.Font)

	for i := 0; i < this.fieldCount(); i++ {
		this.drawField(g, t, i, h)
		if i < this.fieldCount()-1 {
			// ":" separator centred in the gap after this field.
			sx := this.fieldX(i) + timeFieldW
			this.drawCentered(g, t.Font, ":", t.TextColor, sx, timeSepW, 0, h)
		}
	}
}

// fieldValue returns the int currently held by field i.
func (this *TimePicker) fieldValue(i int) int {
	switch i {
	case 0:
		return this.hour
	case 1:
		return this.minute
	default:
		return this.second
	}
}

// drawField paints one HH/MM/SS box: a framed value plus a two-arrow stepper
// column. The focused field gets an accent ring; the hovered stepper half
// gets a subtle wash.
func (this *TimePicker) drawField(g paint.Painter, t *defaultTheme, i int, h float64) {
	fx := this.fieldX(i)
	focused := this.focusField == i && this.Self().HasFocus()

	// Box background + frame.
	g.Rectangle(fx, timePickerPad, timeFieldW, h-2*timePickerPad)
	g.SetBrush1(t.ViewBGColor)
	g.Fill()
	if focused {
		g.SetPen1(t.HighLightColor, 1.5)
	} else {
		g.SetPen1(t.BorderColor, 1)
	}
	g.Rectangle(fx, timePickerPad, timeFieldW, h-2*timePickerPad)
	g.Stroke()

	// Two-digit value, centred in the text area left of the stepper column.
	text := fmt.Sprintf("%02d", this.fieldValue(i))
	this.drawCentered(g, t.Font, text, t.TextColor, fx, timeFieldW-timeStepperW, 0, h)

	// Stepper column: hover wash on the active half, then the arrows.
	sx := fx + timeFieldW - timeStepperW
	mid := h * 0.5
	if this.hoverField == i {
		g.SetBrush1(paint.Color{228, 228, 228, 255})
		if this.hoverUp {
			g.Rectangle(sx, timePickerPad+1, timeStepperW-1, mid-timePickerPad-1)
		} else {
			g.Rectangle(sx, mid, timeStepperW-1, mid-timePickerPad-1)
		}
		g.Fill()
	}

	cx := sx + timeStepperW*0.5
	a := 2.5
	// Up arrow (top half).
	uy := mid * 0.5
	g.MoveTo(cx, uy-a)
	g.LineTo(cx-a*1.2, uy+a*0.5)
	g.LineTo(cx+a*1.2, uy+a*0.5)
	g.LineTo(cx, uy-a)
	g.SetBrush1(t.TextColor)
	g.Fill()
	// Down arrow (bottom half).
	dy := mid + mid*0.5
	g.MoveTo(cx, dy+a)
	g.LineTo(cx-a*1.2, dy-a*0.5)
	g.LineTo(cx+a*1.2, dy-a*0.5)
	g.LineTo(cx, dy+a)
	g.SetBrush1(t.TextColor)
	g.Fill()
}

// drawCentered renders text centred in the box (x, y, boxW, boxH). Mirrors the
// Calendar helper so the two date/time widgets share a look.
func (this *TimePicker) drawCentered(g paint.Painter, f paint.Font, text string, fg paint.Color, x, boxW, y, boxH float64) {
	ext := f.TextExtents(text)
	tx := x + (boxW-ext.Width)*0.5 - ext.XBearing
	ty := y + 0.5*(boxH+ext.YBearing) - ext.YBearing
	g.SetBrush1(fg)
	g.DrawText1(tx, ty, text)
}

// --- SizeHints ---

func (this *TimePicker) SizeHints() SizeHints {
	w := timePickerPad*2 + float64(this.fieldCount())*timeFieldW + float64(this.fieldCount()-1)*timeSepW
	return SizeHints{Width: w, Height: 32, Policy: GrowHorizontal | GrowVertical}
}
