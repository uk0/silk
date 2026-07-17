package gui

import (
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// EventRow is one plain, already-formatted 事件记录 (event-log) entry. The host
// owns all formatting: it turns a backend event (package eventlog) into these
// four display strings before handing a slice to the panel. The panel never
// parses Time or classifies Kind — it only groups by the Kind string, colours
// the cell, and renders the row. Keeping the row a bag of strings is what lets
// the panel stay decoupled from the eventlog backend and GL-free testable.
type EventRow struct {
	Time    string // host-formatted timestamp, e.g. "15:04:05"
	Kind    string // event class the filter groups on: "alarm"/"login"/"write"/"system"/…
	Source  string // originating tag / user / subsystem
	Message string // human-readable detail
}

// EventLogPanel is a read-only 事件记录 viewer for operator screens: a scrollable
// list of host-supplied EventRows with a top filter bar of kind tabs. It holds
// nothing but plain view-model data fed through SetEvents/SetKindFilter and
// emits the operator's filter intent through SigFilter; the host wires that back
// (typically by calling SetKindFilter with the emitted kind) and re-feeds rows.
// This mirrors AlarmPanel's pure-view posture: no backend import, no I/O, and a
// pure hit-test / scroll-clamp surface that unit tests exercise without a window.
type EventLogPanel struct {
	Widget

	events     []EventRow      // full list in host order (defensive copy)
	visible    []EventRow      // cached view: events whose Kind matches kindFilter
	kindFilter string          // "" = all kinds
	scrollY    float64         // vertical scroll offset over the visible rows
	rowHeight  float64         // per-row pixel height
	cbFilter   func(kind string) // fired when a kind tab is clicked
}

func init() {
	core.RegisterFactory("gui.EventLogPanel", core.TypeOf((*EventLogPanel)(nil)))
}

// NewEventLogPanel creates an empty event-log panel.
func NewEventLogPanel() *EventLogPanel {
	p := new(EventLogPanel)
	p.Init(p)
	return p
}

func (this *EventLogPanel) Init(self IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 22
}

// SetEvents replaces the event list with a defensive copy of in. EventRow is a
// value type (all strings), so the shallow copy fully isolates the panel from
// later mutation of the caller's slice. The current kind filter is re-applied to
// the new list and the scroll offset is clamped to the new content rather than
// reset, so a live refresh does not yank the operator's view back to the top.
func (this *EventLogPanel) SetEvents(in []EventRow) {
	cp := make([]EventRow, len(in))
	copy(cp, in)
	this.events = cp
	this.refilter()
	this.clampScroll()
	this.Self().Update()
}

// Events returns a defensive copy of the full event list in host order.
func (this *EventLogPanel) Events() []EventRow {
	out := make([]EventRow, len(this.events))
	copy(out, this.events)
	return out
}

// SetKindFilter narrows the visible list to rows whose Kind equals kind; the
// empty string shows every kind. It re-derives the cached visible set and
// clamps the scroll offset to the (usually shorter) filtered content.
func (this *EventLogPanel) SetKindFilter(kind string) {
	this.kindFilter = kind
	this.refilter()
	this.clampScroll()
	this.Self().Update()
}

// KindFilter returns the active kind filter ("" for all).
func (this *EventLogPanel) KindFilter() string { return this.kindFilter }

// SigFilter registers the callback fired when the operator clicks a kind tab.
// It receives the kind to filter on, already mapped so the "all" tab emits ""
// — the same argument SetKindFilter expects, so the host can wire the two
// directly.
func (this *EventLogPanel) SigFilter(fn func(kind string)) {
	this.cbFilter = fn
}

// refilter rebuilds the cached visible slice from events and the current
// kindFilter. It always allocates a fresh slice (even for the all-kinds case)
// so a caller mutating the returned view cannot reach the stored events.
func (this *EventLogPanel) refilter() {
	out := make([]EventRow, 0, len(this.events))
	for _, e := range this.events {
		if this.kindFilter == "" || e.Kind == this.kindFilter {
			out = append(out, e)
		}
	}
	this.visible = out
}

// visibleRows returns the rows currently shown after filtering (the cached
// view). Pure accessor: no copy, intended for the renderer and headless tests.
func (this *EventLogPanel) visibleRows() []EventRow { return this.visible }

// eventKindColor maps an event kind to its row accent colour: alarm burns red,
// login/write amber, and everything else (system, unknown) has no accent so the
// renderer falls back to the theme's muted text colour. The bool is false for
// that muted fallback, mirroring alarmSeverityColor's shape. Pure — no theme or
// GL dependency — so the mapping is unit-testable headless.
func eventKindColor(kind string) (paint.Color, bool) {
	switch kind {
	case "alarm":
		return paint.Color{R: 230, G: 80, B: 80, A: 255}, true // red
	case "login", "write":
		return paint.Color{R: 230, G: 180, B: 60, A: 255}, true // amber
	default:
		return paint.Color{}, false // system / unknown: theme-muted
	}
}

// --- Filter-bar geometry ---

// eventKindTabs are the clickable kind toggles, left to right. The "all" tab
// maps to the empty filter; the rest filter on their own label.
var eventKindTabs = []string{"all", "alarm", "login", "write", "system"}

const (
	eventHeaderH  = 22.0 // count-header band height
	eventFilterH  = 22.0 // kind-tab filter-bar height (below the header)
	eventToggleX0 = 8.0   // left inset of the first kind tab
	eventToggleW  = 58.0  // width of each kind-tab hit cell
)

// eventToggleAtX maps an x within the filter bar to a kind-tab index, or -1 when
// x is left of the first tab or right of the last. Pure geometry (only the
// layout constants), so the click routing is unit-testable headless.
func eventToggleAtX(x float64) int {
	if x < eventToggleX0 {
		return -1
	}
	i := int((x - eventToggleX0) / eventToggleW)
	if i < 0 || i >= len(eventKindTabs) {
		return -1
	}
	return i
}

// kindForTab maps a tab label to the filter kind it emits: "all" -> "".
func kindForTab(label string) string {
	if label == "all" {
		return ""
	}
	return label
}

// --- Drawing ---

// Draw renders a count header, a kind-filter bar, then one row per visible
// event. All backgrounds and text use Theme() semantic colours so the panel
// reads correctly in the dark IDE theme; only the per-kind accents are fixed.
func (this *EventLogPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	th := Theme()

	// View background.
	g.SetBrush1(th.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header band: "事件 Events   shown/total".
	g.SetBrush1(th.FormColor)
	g.Rectangle(0, 0, w, eventHeaderH)
	g.Fill()
	g.SetBrush1(th.TextColor)
	header := "事件 Events   " + strconv.Itoa(len(this.visible)) + "/" + strconv.Itoa(len(this.events))
	g.DrawText1(8, fe.Ascent+4, header)

	// Filter bar: one label per kind tab, active tab in the theme accent.
	g.SetBrush1(th.FormColor)
	g.Rectangle(0, eventHeaderH, w, eventFilterH)
	g.Fill()
	for i, label := range eventKindTabs {
		x := eventToggleX0 + float64(i)*eventToggleW
		active := kindForTab(label) == this.kindFilter
		if active {
			g.SetBrush1(th.Accent)
		} else {
			g.SetBrush1(th.MenuGrayTextColor)
		}
		g.DrawText1(x+4, eventHeaderH+fe.Ascent+4, label)
		if active {
			g.SetBrush1(th.Accent)
			g.Rectangle(x+4, eventHeaderH+eventFilterH-2, eventToggleW-10, 2)
			g.Fill()
		}
	}

	if len(this.visible) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := eventHeaderH + eventFilterH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.visible); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		e := this.visible[i]

		// Alternating row stripe from the muted text colour at low alpha, so it
		// reads in both themes without a hardcoded fill.
		if i%2 == 1 {
			stripe := th.TextColor
			stripe.A = 12
			g.SetBrush1(stripe)
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		baseline := y + fe.Ascent + 4

		// Time (muted), Kind (colour-coded), Source (text), Message (muted-text).
		g.SetBrush1(th.MenuGrayTextColor)
		g.DrawText1(8, baseline, e.Time)

		col, on := eventKindColor(e.Kind)
		if on {
			g.SetBrush1(col)
		} else {
			g.SetBrush1(th.MenuGrayTextColor)
		}
		g.DrawText1(90, baseline, e.Kind)

		g.SetBrush1(th.TextColor)
		g.DrawText1(150, baseline, e.Source)
		g.DrawText1(250, baseline, e.Message)
	}
}

// --- Events ---

// OnLeftDown routes a click in the filter bar to the kind tab under the cursor,
// firing SigFilter with that tab's mapped kind ("all" -> ""). Clicks on the
// header, on a row, or on empty filter-bar space are ignored.
func (this *EventLogPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	if y >= eventHeaderH && y < eventHeaderH+eventFilterH {
		i := eventToggleAtX(x)
		if i >= 0 && this.cbFilter != nil {
			this.cbFilter(kindForTab(eventKindTabs[i]))
		}
	}
}

// OnMouseWheel scrolls the visible row list vertically.
func (this *EventLogPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	this.clampScroll()
	this.Self().Update()
}

// rowAtY maps a y coordinate to a visible-row index, accounting for the header
// and filter bands and the current scroll offset; it returns -1 when y lands in
// either band. Pure geometry (only scrollY and rowHeight), so it is unit-testable
// headless. The index may exceed the row count for a click past the last row —
// callers bound-check against len(visibleRows()).
func (this *EventLogPanel) rowAtY(y float64) int {
	top := eventHeaderH + eventFilterH
	if y < top {
		return -1
	}
	return int((y - top + this.scrollY) / this.rowHeight)
}

// clampScroll pins scrollY within [0, maxScroll] for the current filtered
// content and viewport height.
func (this *EventLogPanel) clampScroll() {
	_, h := this.Size()
	maxScroll := float64(len(this.visible))*this.rowHeight - (h - eventHeaderH - eventFilterH)
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

func (this *EventLogPanel) SizeHints() SizeHints {
	return SizeHints{MinWidth: 280, MinHeight: 100}
}
