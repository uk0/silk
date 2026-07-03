package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"strconv"
	"time"
)

func init() {
	core.RegisterFactory("ged.LogPanel", gui.TypeOf(LogPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.LogPanel",
		Name: "日志",
		Icon: "edit",
		Desc: "IDE 运行时日志（信息 / 警告 / 错误）",
	})
}

// LogLevel buckets a LogEntry by severity. The order matters — SetFilter
// shows entries whose level is at or above the configured floor, so the
// numeric ranking has to be debug < info < warn < error.
type LogLevel int

const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarn
	LogError
)

// LogEntry is one row in the runtime log. Time is stamped at Append
// time, not at Draw, so the pane shows when the event happened, not
// when the user last opened the dock.
type LogEntry struct {
	Time    time.Time
	Level   LogLevel
	Message string
}

// LogPanel is the IDE's runtime log pane: a scrollable, filterable list
// of LogEntry rows. It is deliberately distinct from BuildOutput (raw
// compiler output) and the terminal panel (interactive subprocess I/O):
// this pane is for IDE-internal events — warnings, diagnostics, the
// "something's not working" trail.
//
// A future commit will plumb core.Log / core.Warn / core.Error here by
// installing a log writer that fans out to both the existing log
// destinations and a registered LogPanel; see Append for the sink shape
// that wiring is going to call.
type LogPanel struct {
	gui.Widget

	entries    []LogEntry
	maxEntries int
	minLevel   LogLevel // floor for visibleEntries(); does not drop entries
	scrollY    float64
	hoverIdx   int
	rowHeight  float64
	cbClicked  func(LogEntry)
	nowFn      func() time.Time // injectable for tests; defaults to time.Now
}

// NewLogPanel creates an empty log pane with the default 1000-entry cap.
func NewLogPanel() *LogPanel {
	p := new(LogPanel)
	p.Init(p)
	return p
}

func (this *LogPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.maxEntries = 1000
	this.minLevel = LogDebug
}

// Append records a new log entry stamped with the current time, drops
// the oldest entry FIFO when the cap is hit, and follows the tail when
// the user was already scrolled to the bottom.
//
// This is the sink shape the future core-bridge commit will call once
// per emitted log line; the pane intentionally keeps its own copy of
// the entries rather than borrowing core's global state so a stale
// LogPanel can be torn down without disturbing the log subsystem.
func (this *LogPanel) Append(level LogLevel, message string) {
	now := time.Now
	if this.nowFn != nil {
		now = this.nowFn
	}
	entry := LogEntry{Time: now(), Level: level, Message: message}

	// Decide auto-follow *before* mutating entries — once we append, the
	// content height changes and the "were we at the bottom?" question
	// can no longer be answered.
	_, h := this.Size()
	follow := shouldAutoScroll(this.scrollY, this.contentHeight(), h)

	this.entries = append(this.entries, entry)
	if this.maxEntries > 0 && len(this.entries) > this.maxEntries {
		drop := len(this.entries) - this.maxEntries
		this.entries = this.entries[drop:]
		// Keep the visual position stable when we drop from the head:
		// the rows shifted up by drop*rowHeight, so the scroll offset
		// has to shift the same amount to keep the same content under
		// the viewport. If we were following the tail anyway the clamp
		// below pins us back to the new bottom.
		this.scrollY -= float64(drop) * this.rowHeight
		if this.scrollY < 0 {
			this.scrollY = 0
		}
	}

	if follow {
		this.scrollToBottom()
	}
	this.Self().Update()
}

// Clear empties the entries slice and resets the view. The filter level
// is preserved — clearing the log is not the same as resetting the UI.
func (this *LogPanel) Clear() {
	this.entries = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// Entries returns a defensive copy of the entries in arrival order.
// Returning the backing slice would let callers mutate (and inadvertently
// truncate) the log; a copy keeps the panel's invariants intact.
func (this *LogPanel) Entries() []LogEntry {
	out := make([]LogEntry, len(this.entries))
	copy(out, this.entries)
	return out
}

// SetMaxEntries adjusts the FIFO cap. A non-positive value disables the
// cap. When the new cap is smaller than the current entry count the
// oldest entries are trimmed immediately so the invariant "len(entries)
// <= maxEntries" holds at all times.
func (this *LogPanel) SetMaxEntries(n int) {
	this.maxEntries = n
	if n > 0 && len(this.entries) > n {
		drop := len(this.entries) - n
		this.entries = this.entries[drop:]
		this.scrollY -= float64(drop) * this.rowHeight
		if this.scrollY < 0 {
			this.scrollY = 0
		}
		this.Self().Update()
	}
}

// MaxEntries returns the configured FIFO cap.
func (this *LogPanel) MaxEntries() int {
	return this.maxEntries
}

// SigEntryClicked registers the callback invoked when the user clicks
// a row. The callback receives a copy of the entry so the host can hold
// onto it past a later Clear without aliasing the panel's slice.
func (this *LogPanel) SigEntryClicked(fn func(LogEntry)) {
	this.cbClicked = fn
}

// SetFilter sets the minimum severity to render. Entries below the
// floor stay in the backing slice (so flipping the filter back makes
// them visible again) — only the rendered subset is filtered.
func (this *LogPanel) SetFilter(level LogLevel) {
	this.minLevel = level
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// Filter returns the current minimum severity floor.
func (this *LogPanel) Filter() LogLevel {
	return this.minLevel
}

// visibleEntries returns the entries that pass the current filter, in
// arrival order. Used by Draw and exposed for unit tests so the filter
// semantics can be exercised without poking at the renderer.
func (this *LogPanel) visibleEntries() []LogEntry {
	if this.minLevel == LogDebug {
		// Fast path: no filtering, hand out a copy of the whole slice.
		out := make([]LogEntry, len(this.entries))
		copy(out, this.entries)
		return out
	}
	var out []LogEntry
	for _, e := range this.entries {
		if e.Level >= this.minLevel {
			out = append(out, e)
		}
	}
	return out
}

// countsByLevel tallies how many entries fall into each bucket. Kept as
// a free function so the header math is pure and trivially testable.
func countsByLevel(entries []LogEntry) (debug, info, warn, err int) {
	for _, e := range entries {
		switch e.Level {
		case LogDebug:
			debug++
		case LogInfo:
			info++
		case LogWarn:
			warn++
		case LogError:
			err++
		}
	}
	return
}

// shouldAutoScroll answers "was the panel scrolled to the bottom before
// the latest Append?" Returning true tells the caller to follow the
// tail; returning false tells it to leave the user's scroll position
// alone (they were reading older entries and don't want the view to
// jump out from under them).
//
// The "at the bottom" check has a tolerance of one rowHeight so that
// rounding from the scroll wheel doesn't accidentally drop the user out
// of follow mode. A panel that hasn't been laid out yet (viewHeight ==
// 0) is also treated as following — otherwise the very first Append on
// a freshly-created panel would never auto-scroll.
func shouldAutoScroll(scrollY, contentHeight, viewHeight float64) bool {
	if viewHeight <= 0 {
		return true
	}
	if contentHeight <= viewHeight {
		// All entries fit; there is nothing to follow but the next
		// Append should still be visible, so report following.
		return true
	}
	maxScroll := contentHeight - viewHeight
	return scrollY >= maxScroll-1.0
}

// contentHeight is the total pixel height of the rendered rows, header
// excluded. Used by the auto-follow decision and the scroll clamp.
func (this *LogPanel) contentHeight() float64 {
	return float64(len(this.visibleEntries())) * this.rowHeight
}

// scrollToBottom pins the view to the last entry. Called after an
// Append when shouldAutoScroll said the user was following the tail.
// A panel that has not been laid out yet (h == 0) has no meaningful
// "bottom" — leave the scroll at zero so OnLeftDown coordinate math
// in early tests stays aligned with the row geometry.
func (this *LogPanel) scrollToBottom() {
	_, h := this.Size()
	if h <= logPanelHeaderH {
		this.scrollY = 0
		return
	}
	max := this.contentHeight() - (h - logPanelHeaderH)
	if max < 0 {
		max = 0
	}
	this.scrollY = max
}

// --- Drawing ---

const logPanelHeaderH = 22.0

// Draw renders a count header followed by one row per visible entry.
func (this *LogPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling panes.
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Header band: "✓ N  ! M  ✕ K". Counts are over the *visible*
	// entries so the user can see, at a glance, what the filter is
	// currently letting through.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, logPanelHeaderH)
	g.Fill()

	visible := this.visibleEntries()
	_, info, warn, errCount := countsByLevel(visible)
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	header := "✓ " + strconv.Itoa(info) +
		"  ! " + strconv.Itoa(warn) +
		"  ✕ " + strconv.Itoa(errCount)
	g.DrawText1(8, fe.Ascent+4, header)

	if len(visible) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := logPanelHeaderH
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(visible); i++ {
		y := areaTop + float64(i)*rh - this.scrollY
		e := visible[i]

		// Hover wins over the alternating stripe.
		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// Timestamp in muted blue-grey.
		ts := e.Time.Format("15:04:05")
		g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
		g.DrawText1(8, y+fe.Ascent+2, ts)
		tsExt := font.TextExtents(ts)

		// Level glyph in the level's accent colour.
		glyph, col := levelGlyph(e.Level)
		g.SetBrush1(col)
		g.DrawText1(8+tsExt.Width+10, y+fe.Ascent+2, glyph)
		glyphExt := font.TextExtents(glyph)

		// Message in light grey.
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
		g.DrawText1(8+tsExt.Width+10+glyphExt.Width+8, y+fe.Ascent+2, e.Message)
	}
}

// levelGlyph maps a LogLevel to a single-character glyph and an accent
// colour for it. Kept as a free function so the colour palette can be
// adjusted without reaching into Draw.
func levelGlyph(lv LogLevel) (string, paint.Color) {
	switch lv {
	case LogDebug:
		return "·", paint.Color{R: 130, G: 145, B: 165, A: 255}
	case LogInfo:
		return "✓", paint.Color{R: 110, G: 200, B: 110, A: 255}
	case LogWarn:
		return "!", paint.Color{R: 230, G: 180, B: 60, A: 255}
	case LogError:
		return "✕", paint.Color{R: 230, G: 80, B: 80, A: 255}
	}
	return "·", paint.Color{R: 200, G: 200, B: 210, A: 255}
}

// --- Events ---

// OnLeftDown activates the clicked row via SigEntryClicked. Indexing is
// against the *visible* slice — a row the filter is hiding cannot be
// clicked.
func (this *LogPanel) OnLeftDown(x, y float64) {
	this.SetFocus()
	idx := this.rowAt(y)
	visible := this.visibleEntries()
	if idx < 0 || idx >= len(visible) {
		return
	}
	if this.cbClicked != nil {
		this.cbClicked(visible[idx])
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *LogPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.visibleEntries()) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *LogPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list vertically. A user scroll always
// breaks auto-follow until they scroll back to the bottom themselves —
// shouldAutoScroll re-checks the position on the next Append.
func (this *LogPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := this.contentHeight() - (h - logPanelHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate (below the header) to a visible-entry
// index, or -1 when y lands on the header band.
func (this *LogPanel) rowAt(y float64) int {
	if y < logPanelHeaderH {
		return -1
	}
	return int((y - logPanelHeaderH + this.scrollY) / this.rowHeight)
}

func (this *LogPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}
