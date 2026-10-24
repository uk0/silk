package gui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() { core.RegisterFactory("gui.TransferProgress", core.TypeOf((*TransferProgress)(nil))) }

type TransferState string

const (
	TransferIdle      TransferState = "idle"
	TransferRunning   TransferState = "running"
	TransferSucceeded TransferState = "succeeded"
	TransferFailed    TransferState = "failed"
	TransferCancelled TransferState = "cancelled"
)

// TransferProgress is a passive, event-driven byte progress display. It creates
// no network requests, windows, timers or animations, even for unknown totals.
// All methods/signals are UI-thread-only. Feed worker snapshots via async.Latest
// with gui.Post. Runtime counts/state are not saved as designer properties.
type TransferProgress struct {
	Widget
	label, summary  string
	showBytes       bool
	received, total int64
	state           TransferState
	stateChanged    func(string)
}

func NewTransferProgress() *TransferProgress { v := new(TransferProgress); v.Init(v); return v }
func (v *TransferProgress) Init(self IWidget) {
	v.Widget.Init(self)
	v.label, v.total, v.state, v.showBytes = "Transfer", -1, TransferIdle, true
	v.refreshSummary()
}
func (v *TransferProgress) Label() string { return v.label }
func (v *TransferProgress) SetLabel(label string) {
	if v.label != label {
		v.label = label
		v.Update()
	}
}
func (v *TransferProgress) ShowBytes() bool { return v.showBytes }
func (v *TransferProgress) SetShowBytes(on bool) {
	if v.showBytes != on {
		v.showBytes = on
		v.refreshSummary()
		v.Update()
	}
}
func (v *TransferProgress) ReceivedBytes() int64            { return v.received }
func (v *TransferProgress) TotalBytes() int64               { return v.total }
func (v *TransferProgress) State() TransferState            { return v.state }
func (v *TransferProgress) Summary() string                 { return v.summary }
func (v *TransferProgress) SigStateChanged(fn func(string)) { v.stateChanged = fn }

// SetProgress retains integer precision. Negative received values become zero;
// any negative total means unknown. Counts exceeding total remain observable,
// while the drawn fraction is clamped. Progress alone NEVER declares success.
func (v *TransferProgress) SetProgress(received, total int64) {
	received, total = max(0, received), max(-1, total)
	if received == v.received && total == v.total {
		return
	}
	v.received, v.total = received, total
	v.refreshSummary()
	v.Update()
}
func (v *TransferProgress) SetState(state TransferState) error {
	switch state {
	case TransferIdle, TransferRunning, TransferSucceeded, TransferFailed, TransferCancelled:
	default:
		return fmt.Errorf("TransferProgress: invalid state %q", state)
	}
	if v.state == state {
		return nil
	}
	v.state = state
	v.refreshSummary()
	v.Update()
	if v.stateChanged != nil {
		v.stateChanged(string(state))
	}
	return nil
}
func (v *TransferProgress) Reset() {
	v.SetProgress(0, -1)
	_ = v.SetState(TransferIdle)
}

// Fraction reports a display ratio and whether total is known. An empty
// transfer reaches 100% only after the caller explicitly marks success.
func (v *TransferProgress) Fraction() (float64, bool) {
	if v.total < 0 {
		return 0, false
	}
	if v.total == 0 {
		if v.state == TransferSucceeded {
			return 1, true
		}
		return 0, true
	}
	return min(1, float64(v.received)/float64(v.total)), true
}
func (v *TransferProgress) refreshSummary() {
	ratio, known := v.Fraction()
	v.summary = "Unknown total"
	if known {
		percent := int(ratio * 100)
		// Float rounding can reach 1 for large, almost-complete int64 values.
		if v.received < v.total && percent >= 100 {
			percent = 99
		}
		v.summary = fmt.Sprintf("%d%%", percent)
	}
	if v.showBytes {
		if v.total >= 0 {
			v.summary += " · " + transferBytes(v.received) + " / " + transferBytes(v.total)
		} else {
			v.summary += " · " + transferBytes(v.received) + " received"
		}
	}
}
func transferBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	value, unit := float64(n), "B"
	for _, u := range []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"} {
		value /= 1024
		unit = u
		if value < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}
func transferTitle(label string) string {
	count := 0
	for i := range label {
		if count == 128 {
			label = label[:i] + "…"
			break
		}
		count++
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, label)
}
func (v *TransferProgress) EnumProperties(list core.IPropertyList) {
	list.AddProperty("Label", v.Label, v.SetLabel)
	list.AddProperty("Show Bytes", v.ShowBytes, v.SetShowBytes)
}
func (v *TransferProgress) SizeHints() SizeHints {
	return SizeHints{Width: 360, Height: 82, Policy: GrowHorizontal | GrowVertical}
}
func (v *TransferProgress) Draw(g paint.Painter) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	t := Theme()
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	g.Clip()
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()
	g.SetPen1(t.BorderColor, 1)
	g.Rectangle(.5, .5, max(0, w-1), max(0, h-1))
	g.Stroke()
	color := t.Accent
	palette := paint.CurrentIconPalette()
	switch v.state {
	case TransferIdle, TransferCancelled:
		color = palette.Disabled
	case TransferSucceeded:
		color = palette.Success
	case TransferFailed:
		color = palette.Danger
	}
	pad := min(12.0, w/4)
	trackWidth, trackHeight := max(0, w-2*pad), min(6.0, h/3)
	trackY := max(0, h-pad-trackHeight)
	g.SetBrush1(t.BorderColor)
	g.Rectangle(pad, trackY, trackWidth, trackHeight)
	g.Fill()
	ratio, known := v.Fraction()
	if !known {
		ratio = 0
		if v.state == TransferRunning {
			ratio = .15
		}
		if v.state == TransferSucceeded {
			ratio = 1
		}
	}
	g.SetBrush1(color)
	g.Rectangle(pad, trackY, trackWidth*ratio, trackHeight)
	g.Fill()
	fe := t.Font.FontExtents()
	if h < 2*pad+fe.Height+trackHeight+6 {
		return
	}
	g.SetFont(t.Font)
	state := elideToWidth(t.Font, string(v.state), trackWidth)
	sw := t.Font.TextExtents(state).Width
	g.SetBrush1(color)
	g.DrawText1(w-pad-sw, pad+fe.Ascent, state)
	labelBudget := max(0, trackWidth-sw-16)
	g.SetBrush1(t.TextColor)
	g.DrawText1(pad, pad+fe.Ascent, elideToWidth(t.Font, transferTitle(v.label), labelBudget))
	if h >= 2*pad+2*fe.Height+trackHeight+6 {
		g.SetBrush1(t.TextColor)
		g.DrawText1(pad, pad+fe.Height+fe.Ascent+4, elideToWidth(t.Font, v.summary, trackWidth))
	}
}
