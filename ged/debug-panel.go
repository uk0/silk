package ged

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.DebugPanel", gui.TypeOf(DebugPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.DebugPanel",
		Name: "调试 / Debug",
		Icon: "edit",
		Desc: "断点处的调用栈与局部变量",
	})
}

// DebugPanel is the bottom-dock pane a debugger uses to show the call
// stack, the local variables, and watched expressions at a breakpoint. It
// is a pure display/interaction widget: it never talks to dlv itself. The
// host (silkide) drives a core.DebugSession — Stacktrace() / ListLocals() /
// Eval() — and pushes the results in via SetCallStack / SetVariables /
// SetWatches. The panel renders them and emits signals back:
//
//	SigFrameSelected  — a stack-frame row was clicked. The host re-fetches
//	                    locals for that frame (ListLocals at the frame's
//	                    depth) and calls SetVariables to refresh the lower
//	                    section.
//	SigFrameActivated — a stack-frame row was double-clicked / Enter'd. The
//	                    host opens frame.File:frame.Line in the editor.
//	SigWatchAdded     — the user submitted a new expression in the watch
//	                    input. The host evaluates it (Eval) and pushes the
//	                    refreshed list back via SetWatches.
//	SigWatchRemoved   — the user dropped a watch row. The host stops
//	                    tracking that expression.
//	SigGoroutineActivated — a goroutine row was clicked. The host opens the
//	                    goroutine's File:Line in the editor (and may later
//	                    switch the inspected goroutine).
//	SigVariableEdited — the user submitted a new value for a local. The host
//	                    calls dlv SetVariable(name, newValue), re-fetches the
//	                    locals for the current frame, and pushes them back via
//	                    SetVariables.
//
// The four bands split the widget vertically: the call stack on top, the
// locals, the goroutines, then the watch section at the bottom. Each band
// has its own header (with a count) and its own independent vertical scroll.
//
// Goroutines (core.DebugSession.ListGoroutines) are pushed in the same
// data-push way via SetGoroutines. Locals are editable in place: a
// double-click (or Enter) on a row opens an inline value editor over the
// value column, and submitting emits SigVariableEdited — the panel never
// calls dlv SetVariable itself, the host does and re-pushes the locals.
type DebugPanel struct {
	gui.Widget

	frames   []core.StackFrame
	vars     []core.Variable
	selected int // index into frames of the highlighted row; 0 = top frame

	stackScrollY float64
	varScrollY   float64
	hoverStack   int // hovered call-stack row, -1 when none
	hoverVar     int // hovered variables row, -1 when none
	rowHeight    float64

	// Double-click detection, mirroring file-explorer.go: the framework
	// has no native double-click event, so we time consecutive clicks on
	// the same frame row ourselves.
	lastClickIdx  int
	lastClickTime time.Time

	cbFrameSelected  func(index int, frame core.StackFrame)
	cbFrameActivated func(frame core.StackFrame)

	// Goroutines section: host-fed goroutine list with its own scroll and
	// hover, plus the row-activated callback. Like the other bands it is pure
	// display — SetGoroutines pushes the data, SigGoroutineActivated reports a
	// click, and the host opens the goroutine's file:line.
	goroutines  []core.Goroutine
	goroScrollY float64
	hoverGoro   int // hovered goroutine row, -1 when none

	cbGoroutineActivated func(g core.Goroutine)

	// Inline local-variable editing. editingVar is the locals index whose
	// value is being edited in place (-1 when none); varInput is the
	// in-progress text, seeded from the row's Value on entry. Its own
	// double-click timer keeps it independent of the call-stack one.
	editingVar       int
	varInput         string
	lastVarClickIdx  int
	lastVarClickTime time.Time

	cbVariableEdited func(name string, newValue string)

	// Watch section: host-fed watched expressions plus an in-line editor the
	// user types new expressions into. Like the rest of the panel it never
	// evaluates anything itself — it emits SigWatchAdded / SigWatchRemoved
	// and renders whatever the host pushes back via SetWatches.
	watches      []WatchEntry
	watchScrollY float64
	hoverWatch   int    // hovered watch row, -1 when none
	watchInput   string // in-progress expression in the input line
	watchFocused bool   // whether the expression input line holds focus

	cbWatchAdded   func(expr string)
	cbWatchRemoved func(expr string)
}

// WatchEntry is one watched expression and its last evaluation. Expr is the
// Go expression the user typed; Value and Type are the dlv Eval result on
// success; Err is the error string when evaluation failed. A non-empty Err
// means Value/Type are not meaningful and the row renders the error. The
// host fills these in and pushes them via SetWatches.
type WatchEntry struct {
	Expr  string
	Value string
	Type  string
	Err   string
}

// NewDebugPanel creates an empty debug panel.
func NewDebugPanel() *DebugPanel {
	p := new(DebugPanel)
	p.Init(p)
	return p
}

func (this *DebugPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverStack = -1
	this.hoverVar = -1
	this.hoverWatch = -1
	this.hoverGoro = -1
	this.lastClickIdx = -1
	this.lastVarClickIdx = -1
	this.editingVar = -1
}

// SetCallStack replaces the call-stack frames and resets the selection to
// the top frame (index 0). It does NOT emit SigFrameSelected — pushing a
// fresh stack is the host telling the panel "we just stopped here", not
// the user picking a frame; the host already knows the top frame and has
// loaded its locals.
func (this *DebugPanel) SetCallStack(frames []core.StackFrame) {
	this.frames = frames
	this.selected = 0
	this.stackScrollY = 0
	this.hoverStack = -1
	this.lastClickIdx = -1
	this.Self().Update()
}

// SetVariables replaces the local-variable rows. The host calls this both
// on the initial stop (locals of the top frame) and again whenever
// SigFrameSelected fires for a different frame.
func (this *DebugPanel) SetVariables(vars []core.Variable) {
	this.vars = vars
	this.varScrollY = 0
	this.hoverVar = -1
	this.Self().Update()
}

// Clear empties the call stack and locals and blanks every watch VALUE, but
// KEEPS the watch expressions — a real debugger keeps your watches across
// stops and only re-evaluates them. The host calls this when the program
// continues (no stopped frame to show); on the next stop it pushes fresh
// frames/locals and re-evaluates the surviving expressions via SetWatches.
// Use ClearAll to drop the expressions too (the session ended entirely).
func (this *DebugPanel) Clear() {
	this.frames = nil
	this.vars = nil
	this.selected = 0
	this.stackScrollY = 0
	this.varScrollY = 0
	this.hoverStack = -1
	this.hoverVar = -1
	this.lastClickIdx = -1
	// Goroutines are stop-scoped like the stack: drop them on continue.
	this.goroutines = nil
	this.goroScrollY = 0
	this.hoverGoro = -1
	// An edit targets a now-gone locals row: close it.
	this.editingVar = -1
	this.varInput = ""
	this.lastVarClickIdx = -1
	// Keep the watched expressions; blank only their evaluated fields so the
	// stale values don't linger while the program runs.
	for i := range this.watches {
		this.watches[i].Value = ""
		this.watches[i].Type = ""
		this.watches[i].Err = ""
	}
	this.Self().Update()
}

// ClearAll resets the panel to empty: call stack, locals, AND every watch
// expression, plus the in-progress input. The host calls this when the
// debug session ends entirely, so there is no session to keep watches for.
func (this *DebugPanel) ClearAll() {
	this.Clear()
	this.watches = nil
	this.watchScrollY = 0
	this.watchInput = ""
	this.watchFocused = false
	this.hoverWatch = -1
	this.Self().Update()
}

// CallStack returns a defensive copy of the call-stack frames in top-down
// order. A copy keeps the host from mutating the panel's backing slice.
func (this *DebugPanel) CallStack() []core.StackFrame {
	out := make([]core.StackFrame, len(this.frames))
	copy(out, this.frames)
	return out
}

// Variables returns a defensive copy of the local-variable rows.
func (this *DebugPanel) Variables() []core.Variable {
	out := make([]core.Variable, len(this.vars))
	copy(out, this.vars)
	return out
}

// SetGoroutines replaces the goroutine rows with a defensive copy, resets the
// band's scroll, and repaints. The host fills this by calling dlv
// ListGoroutines at a stop and pushing the result; the panel only renders it.
func (this *DebugPanel) SetGoroutines(gs []core.Goroutine) {
	out := make([]core.Goroutine, len(gs))
	copy(out, gs)
	this.goroutines = out
	this.goroScrollY = 0
	this.hoverGoro = -1
	this.Self().Update()
}

// Goroutines returns a defensive copy of the goroutine rows in display order.
func (this *DebugPanel) Goroutines() []core.Goroutine {
	out := make([]core.Goroutine, len(this.goroutines))
	copy(out, this.goroutines)
	return out
}

// SelectedFrame returns the index of the highlighted stack frame, default
// 0 (the top frame). This is the frame whose locals the lower section is
// expected to be showing.
func (this *DebugPanel) SelectedFrame() int {
	return this.selected
}

// SigFrameSelected registers the callback fired when the user clicks a
// stack-frame row. The host re-fetches locals for that frame and calls
// SetVariables. The callback receives a copy of the frame so the host can
// hold onto it past a later Clear without aliasing the panel's slice.
func (this *DebugPanel) SigFrameSelected(fn func(index int, frame core.StackFrame)) {
	this.cbFrameSelected = fn
}

// SigFrameActivated registers the callback fired when the user
// double-clicks a stack-frame row or presses Enter on it. The host opens
// frame.File:frame.Line in the editor.
func (this *DebugPanel) SigFrameActivated(fn func(frame core.StackFrame)) {
	this.cbFrameActivated = fn
}

// SigGoroutineActivated registers the callback fired when the user clicks a
// goroutine row. The host opens the goroutine's File:Line in the editor and
// may switch the inspected goroutine. The callback receives a copy so the
// host can hold onto it past a later Clear without aliasing the panel's slice.
func (this *DebugPanel) SigGoroutineActivated(fn func(g core.Goroutine)) {
	this.cbGoroutineActivated = fn
}

// SigVariableEdited registers the callback fired when the user submits a new
// value for a local through the inline editor. The host applies it with dlv
// SetVariable(name, newValue), then re-fetches the locals for the current
// frame and pushes them back via SetVariables (host-driven, like the rest of
// the panel — it never calls dlv itself).
func (this *DebugPanel) SigVariableEdited(fn func(name string, newValue string)) {
	this.cbVariableEdited = fn
}

// SetWatches replaces the watched expressions and their evaluated results
// with a defensive copy, then repaints. The host builds this list by
// evaluating every WatchExprs() entry (dlv Eval) at the current stop and
// pushing the results — Value/Type set on success, Err set on failure.
func (this *DebugPanel) SetWatches(w []WatchEntry) {
	out := make([]WatchEntry, len(w))
	copy(out, w)
	this.watches = out
	this.watchScrollY = 0
	this.hoverWatch = -1
	this.Self().Update()
}

// Watches returns a defensive copy of the watched expressions and their
// last evaluations, in display order.
func (this *DebugPanel) Watches() []WatchEntry {
	out := make([]WatchEntry, len(this.watches))
	copy(out, this.watches)
	return out
}

// WatchExprs returns just the watched expression strings, in display order.
// The host re-evaluates all of them on each stop and pushes the results
// back via SetWatches.
func (this *DebugPanel) WatchExprs() []string {
	out := make([]string, len(this.watches))
	for i, w := range this.watches {
		out[i] = w.Expr
	}
	return out
}

// SigWatchAdded registers the callback fired when the user submits a new
// expression in the watch input. The host evaluates it and pushes the
// refreshed list back via SetWatches.
func (this *DebugPanel) SigWatchAdded(fn func(expr string)) {
	this.cbWatchAdded = fn
}

// SigWatchRemoved registers the callback fired when the user removes a
// watch row (the ✕ affordance) or the host calls RemoveWatch. The host
// stops tracking that expression.
func (this *DebugPanel) SigWatchRemoved(fn func(expr string)) {
	this.cbWatchRemoved = fn
}

// RemoveWatch removes the first watch whose expression equals expr and
// fires SigWatchRemoved. It is the host-callable form of the row ✕ button
// so a host UI (or a test) can drop a watch without a click; a no-op when
// expr is not currently watched.
func (this *DebugPanel) RemoveWatch(expr string) {
	for i, w := range this.watches {
		if w.Expr == expr {
			this.removeWatchAt(i)
			return
		}
	}
}

// truncateValue shortens a variable value to at most max runes, replacing
// the tail with a single-character ellipsis when it overflows. A value
// that is already within max comes back unchanged; max <= 0 yields "".
// Kept as a free function so the truncation rule is pure and testable
// without a widget or GL context.
func truncateValue(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	// Reserve the last column for the ellipsis so the result is exactly
	// max runes wide.
	return string(r[:max-1]) + "…"
}

// frameRowAtY maps a y coordinate to a stack-frame row index within a
// section whose rows start at topOffset, with count rows of height rowH.
// The caller folds the section's scroll offset into y before calling. It
// returns -1 when y lands above the rows or past the last row. Pure so the
// hit-test is testable without a live widget.
func frameRowAtY(y, topOffset, rowH float64, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := int((y - topOffset) / rowH)
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// --- Layout geometry ---

const debugHeaderH = 22.0

// stackBandHeight is the pixel height the call-stack section gets,
// including its header. The variables section takes the rest. The stack
// gets the upper ~45% so a deep stack still leaves room for locals; a
// minimum keeps the header + a couple of rows visible in a short dock.
func (this *DebugPanel) stackBandHeight(totalH float64) float64 {
	h := totalH * 0.45
	min := debugHeaderH + this.rowHeight*2
	if h < min {
		h = min
	}
	if h > totalH-debugHeaderH {
		// Always leave at least the variables header visible.
		h = totalH - debugHeaderH
	}
	if h < debugHeaderH {
		h = debugHeaderH
	}
	return h
}

// watchRemoveW is the width of the ✕ remove hot-zone on the right of a
// watch row: a click past (widget width - watchRemoveW) removes the row.
const watchRemoveW = 20.0

// watchBandHeight is the pixel height reserved at the bottom for the watch
// section: its header, the expression input line, and the watched rows. It
// takes ~30% of the widget with a floor that always keeps the header and
// input line visible, and never exceeds half the widget so the call stack
// and locals keep the upper half.
func (this *DebugPanel) watchBandHeight(totalH float64) float64 {
	h := totalH * 0.3
	min := debugHeaderH + this.rowHeight // header + input line
	if h < min {
		h = min
	}
	if max := totalH * 0.5; h > max {
		h = max
	}
	return h
}

// goroutineBandHeight sizes the goroutines band, carved from the bottom of
// the middle region (everything between the stack and watch bands). It takes
// ~40% of that region with a floor of a header + two rows, capped so the
// locals band above always keeps at least its own header visible.
func (this *DebugPanel) goroutineBandHeight(midH float64) float64 {
	h := midH * 0.4
	if min := debugHeaderH + this.rowHeight*2; h < min {
		h = min
	}
	if max := midH - debugHeaderH; h > max {
		h = max
	}
	if h < debugHeaderH {
		h = debugHeaderH
	}
	return h
}

// sectionLayout returns the vertical band split for the current widget
// height: the call-stack band spans [0, stackH), the locals band spans
// [stackH, varBottom), the goroutines band spans [varBottom, watchTop), and
// the watch band spans [watchTop, height). Kept in one place so Draw and
// every hit-test agree on the boundaries. The stack band is still sized
// against the upper region (everything above the watch band) so
// stackBandHeight's own math and tests stay unchanged; the goroutines band is
// carved from the bottom of that region, leaving the locals band the rest.
func (this *DebugPanel) sectionLayout() (stackH, varBottom, watchTop float64) {
	_, h := this.Size()
	watchTop = h - this.watchBandHeight(h)
	stackH = this.stackBandHeight(watchTop)
	varBottom = watchTop - this.goroutineBandHeight(watchTop-stackH)
	if varBottom < stackH {
		varBottom = stackH
	}
	return
}

// --- Drawing ---

// Draw paints the four stacked sections: call stack on top, variables, then
// goroutines, watch on the bottom, each with a counted header and its own
// scrolled row list.
func (this *DebugPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling panes (log/problems).
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)

	stackH, varBottom, watchTop := this.sectionLayout()
	this.drawStackSection(g, font, w, stackH)
	this.drawVarSection(g, font, w, stackH, varBottom-stackH)
	this.drawGoroutineSection(g, font, w, varBottom, watchTop-varBottom)
	this.drawWatchSection(g, font, w, watchTop, h-watchTop)
}

// drawStackSection paints the call-stack band: a header with the frame
// count, then one row per frame (Function left, file:line dimmed/right).
func (this *DebugPanel) drawStackSection(g paint.Painter, font paint.Font, w, bandH float64) {
	fe := font.FontExtents()

	// Header band.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, debugHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, fe.Ascent+4, "调用栈 / Call Stack ("+strconv.Itoa(len(this.frames))+")")

	if len(this.frames) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := debugHeaderH
	startIdx := int(this.stackScrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((bandH-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.frames); i++ {
		y := areaTop + float64(i)*rh - this.stackScrollY
		if y+rh <= areaTop || y >= bandH {
			continue
		}
		fr := this.frames[i]

		// Selection wins over hover wins over the alternating stripe.
		if i == this.selected {
			g.SetBrush1(paint.Color{R: 55, G: 70, B: 95, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i == this.hoverStack {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// Function name on the left, in light grey.
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		g.DrawText1(8, y+fe.Ascent+2, fr.Function)

		// "file:line" right-aligned in muted blue-grey.
		loc := filepath.Base(fr.File) + ":" + strconv.Itoa(fr.Line)
		locExt := font.TextExtents(loc)
		g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
		g.DrawText1(w-locExt.Width-8, y+fe.Ascent+2, loc)
	}
}

// drawVarSection paints the variables band starting at y=top: a header
// with the variable count, then one row per local "Name  Type  = Value".
func (this *DebugPanel) drawVarSection(g paint.Painter, font paint.Font, w, top, bandH float64) {
	fe := font.FontExtents()

	// Header band, drawn at the section's top.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, top, w, debugHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, top+fe.Ascent+4, "局部变量 / Variables ("+strconv.Itoa(len(this.vars))+")")

	if len(this.vars) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := top + debugHeaderH
	startIdx := int(this.varScrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((bandH-debugHeaderH)/rh) + 2
	bottom := top + bandH

	for i := startIdx; i < startIdx+visibleCount && i < len(this.vars); i++ {
		y := areaTop + float64(i)*rh - this.varScrollY
		if y+rh <= areaTop || y >= bottom {
			continue
		}
		v := this.vars[i]

		// Hover wins over the alternating stripe.
		if i == this.hoverVar {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// Name in accent blue.
		g.SetBrush1(paint.Color{R: 120, G: 170, B: 230, A: 255})
		g.DrawText1(8, y+fe.Ascent+2, v.Name)
		x := 8 + font.TextExtents(v.Name).Width + 8

		// Type, dimmed.
		if v.Type != "" {
			g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, v.Type)
			x += font.TextExtents(v.Type).Width + 8
		}

		// The value column: an inline editor when this row is being edited,
		// otherwise the (truncated) value so a huge struct dump can't run off
		// the row. The editor mirrors the watch input — a focused text line
		// with a caret; only the value is editable, Name/Type stay put.
		if i == this.editingVar {
			g.SetBrush1(paint.Color{R: 40, G: 48, B: 60, A: 255})
			g.Rectangle(x, y, w-x, rh)
			g.Fill()
			g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
			g.DrawText1(x+4, y+fe.Ascent+2, this.varInput)
			cx := x + 4 + font.TextExtents(this.varInput).Width + 1
			g.SetBrush1(paint.Color{R: 150, G: 190, B: 240, A: 255})
			g.Rectangle(cx, y+3, 1.5, rh-6)
			g.Fill()
		} else {
			g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, "= "+truncateValue(v.Value, 80))
		}
	}
}

// drawGoroutineSection paints the goroutines band at y=top: a header with the
// goroutine count, then one row per goroutine as "#ID Function (file:line)":
// the id in accent blue, the function in light grey, the location dimmed and
// right-aligned. Alternating stripe + hover, its own vertical scroll.
func (this *DebugPanel) drawGoroutineSection(g paint.Painter, font paint.Font, w, top, bandH float64) {
	fe := font.FontExtents()

	// Header band, drawn at the section's top.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, top, w, debugHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, top+fe.Ascent+4, "协程 / Goroutines ("+strconv.Itoa(len(this.goroutines))+")")

	if len(this.goroutines) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := top + debugHeaderH
	startIdx := int(this.goroScrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((bandH-debugHeaderH)/rh) + 2
	bottom := top + bandH

	for i := startIdx; i < startIdx+visibleCount && i < len(this.goroutines); i++ {
		y := areaTop + float64(i)*rh - this.goroScrollY
		if y+rh <= areaTop || y >= bottom {
			continue
		}
		gr := this.goroutines[i]

		// Hover wins over the alternating stripe.
		if i == this.hoverGoro {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// "#ID" in accent blue.
		id := "#" + strconv.Itoa(gr.ID)
		g.SetBrush1(paint.Color{R: 120, G: 170, B: 230, A: 255})
		g.DrawText1(8, y+fe.Ascent+2, id)
		x := 8 + font.TextExtents(id).Width + 8

		// Function name, light grey.
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		g.DrawText1(x, y+fe.Ascent+2, gr.Function)

		// "file:line" right-aligned in muted blue-grey.
		loc := filepath.Base(gr.File) + ":" + strconv.Itoa(gr.Line)
		locExt := font.TextExtents(loc)
		g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
		g.DrawText1(w-locExt.Width-8, y+fe.Ascent+2, loc)
	}
}

// drawWatchSection paints the watch band at y=top: a header with the watch
// count, the expression input line (a caret + typed text when focused, a
// dim prompt when empty), then one row per watched expression as
// "expr = value", or "expr: err" in red when the evaluation failed.
func (this *DebugPanel) drawWatchSection(g paint.Painter, font paint.Font, w, top, bandH float64) {
	fe := font.FontExtents()
	rh := this.rowHeight

	// Header band, drawn at the section's top.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, top, w, debugHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, top+fe.Ascent+4, "监视 / Watch ("+strconv.Itoa(len(this.watches))+")")

	// Expression input line, just under the header.
	inputY := top + debugHeaderH
	if this.watchFocused {
		g.SetBrush1(paint.Color{R: 40, G: 48, B: 60, A: 255})
	} else {
		g.SetBrush1(paint.Color{R: 30, G: 30, B: 36, A: 255})
	}
	g.Rectangle(0, inputY, w, rh)
	g.Fill()
	if this.watchInput == "" && !this.watchFocused {
		g.SetBrush1(paint.Color{R: 110, G: 120, B: 135, A: 255})
		g.DrawText1(8, inputY+fe.Ascent+2, "+ 表达式 / add expression")
	} else {
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		g.DrawText1(8, inputY+fe.Ascent+2, this.watchInput)
		if this.watchFocused {
			// Caret at the end of the typed text.
			cx := 8 + font.TextExtents(this.watchInput).Width + 1
			g.SetBrush1(paint.Color{R: 150, G: 190, B: 240, A: 255})
			g.Rectangle(cx, inputY+3, 1.5, rh-6)
			g.Fill()
		}
	}

	if len(this.watches) == 0 {
		return
	}

	rowsTop := inputY + rh
	startIdx := int(this.watchScrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((bandH-debugHeaderH-rh)/rh) + 2
	bottom := top + bandH

	for i := startIdx; i < startIdx+visibleCount && i < len(this.watches); i++ {
		y := rowsTop + float64(i)*rh - this.watchScrollY
		if y+rh <= rowsTop || y >= bottom {
			continue
		}
		e := this.watches[i]

		// Hover wins over the alternating stripe.
		if i == this.hoverWatch {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// Expression in accent blue.
		g.SetBrush1(paint.Color{R: 120, G: 170, B: 230, A: 255})
		g.DrawText1(8, y+fe.Ascent+2, e.Expr)
		x := 8 + font.TextExtents(e.Expr).Width + 8

		if e.Err != "" {
			// Failed eval: the error in red, prefixed with a colon.
			g.SetBrush1(paint.Color{R: 230, G: 110, B: 110, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, ": "+truncateValue(e.Err, 60))
		} else {
			// Type, dimmed, then "= Value" truncated so a big dump can't run
			// off the row.
			if e.Type != "" {
				g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
				g.DrawText1(x, y+fe.Ascent+2, e.Type)
				x += font.TextExtents(e.Type).Width + 8
			}
			g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, "= "+truncateValue(e.Value, 60))
		}

		// ✕ remove affordance on the right, shown on hover.
		if i == this.hoverWatch {
			g.SetBrush1(paint.Color{R: 200, G: 130, B: 130, A: 255})
			g.DrawText1(w-watchRemoveW+4, y+fe.Ascent+2, "✕")
		}
	}
}

// --- Events ---

// OnLeftDown routes a click to the section it lands in. Any click first
// cancels an in-progress inline value edit (a double-click on a locals row,
// handled below, re-opens it). In the watch band it focuses the expression
// input, or removes a row when the ✕ hot-zone is hit; any click outside the
// input blurs it. A goroutine row click fires SigGoroutineActivated so the
// host opens its file:line. In the call stack it selects the clicked frame
// (firing SigFrameSelected) and treats a quick second click on the same frame
// as activation (firing SigFrameActivated). In the variables section a quick
// second click on the same row opens the inline value editor; a single click
// only arms that double-click.
func (this *DebugPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	// A click anywhere first drops any in-progress value edit; a double-click
	// on a locals row re-opens one below.
	this.cancelEditVar()

	// Watch input line: clicking it focuses the expression editor.
	if this.watchInputAt(y) {
		this.focusWatchInput(true)
		return
	}
	// Any other click blurs the watch input.
	this.focusWatchInput(false)

	// Watch row: the ✕ hot-zone on the right removes the row; elsewhere the
	// row is inert (the value is display-only).
	if wi := this.watchRowAt(y); wi >= 0 {
		w, _ := this.Size()
		if x >= w-watchRemoveW {
			this.removeWatchAt(wi)
		}
		return
	}

	// Goroutine row: a click activates it (the host opens its file:line).
	if gi := this.goroutineRowAt(y); gi >= 0 {
		this.activateGoroutine(gi)
		return
	}

	// Locals row: a quick second click on the same row opens the inline value
	// editor (same double-click idiom as the call stack, own timer).
	if vi := this.varRowAt(y); vi >= 0 {
		now := time.Now()
		if vi == this.lastVarClickIdx && now.Sub(this.lastVarClickTime) < 400*time.Millisecond {
			this.lastVarClickTime = time.Time{} // reset to avoid triple-click
			this.beginEditVar(vi)
			return
		}
		this.lastVarClickTime = now
		this.lastVarClickIdx = vi
		return
	}

	idx := this.stackRowAt(y)
	if idx < 0 {
		return
	}

	now := time.Now()
	// Double-click detection (same idiom as file-explorer.go).
	if idx == this.lastClickIdx && now.Sub(this.lastClickTime) < 400*time.Millisecond {
		this.lastClickTime = time.Time{} // reset to avoid triple-click
		this.activateFrame(idx)
		return
	}
	this.lastClickTime = now
	this.lastClickIdx = idx

	this.selectFrame(idx)
}

// selectFrame highlights frame idx and fires SigFrameSelected. The signal
// fires even when idx is already selected so the host can re-pull locals
// for it; the repaint is skipped in that case since nothing visual moved.
func (this *DebugPanel) selectFrame(idx int) {
	if idx < 0 || idx >= len(this.frames) {
		return
	}
	if idx != this.selected {
		this.selected = idx
		this.Self().Update()
	}
	if this.cbFrameSelected != nil {
		this.cbFrameSelected(idx, this.frames[idx])
	}
}

// activateFrame fires SigFrameActivated for frame idx (open in editor).
func (this *DebugPanel) activateFrame(idx int) {
	if idx < 0 || idx >= len(this.frames) {
		return
	}
	this.selected = idx
	this.Self().Update()
	if this.cbFrameActivated != nil {
		this.cbFrameActivated(this.frames[idx])
	}
}

// activateGoroutine fires SigGoroutineActivated for goroutine idx so the host
// opens its file:line. A no-op for an out-of-range index.
func (this *DebugPanel) activateGoroutine(idx int) {
	if idx < 0 || idx >= len(this.goroutines) {
		return
	}
	if this.cbGoroutineActivated != nil {
		this.cbGoroutineActivated(this.goroutines[idx])
	}
}

// beginEditVar opens the inline value editor on locals row idx, seeding it
// with the row's current Value. Only the Value is editable — Name and Type
// stay as shown. A no-op for an out-of-range index. Blurs the watch input so
// only one text field holds keys at a time.
func (this *DebugPanel) beginEditVar(idx int) {
	if idx < 0 || idx >= len(this.vars) {
		return
	}
	this.focusWatchInput(false)
	this.editingVar = idx
	this.varInput = this.vars[idx].Value
	this.Self().Update()
}

// submitEditVar fires SigVariableEdited(name, newValue) with the edited value
// and leaves edit mode. Like adds in the watch section, the panel does NOT
// mutate its own vars: applying the change needs dlv SetVariable, which only
// the host can do — it then re-fetches locals and pushes them via
// SetVariables. A blank value is ignored (the edit simply closes).
func (this *DebugPanel) submitEditVar() {
	idx := this.editingVar
	val := strings.TrimSpace(this.varInput)
	this.editingVar = -1
	this.varInput = ""
	this.Self().Update()
	if idx < 0 || idx >= len(this.vars) || val == "" {
		return
	}
	if this.cbVariableEdited != nil {
		this.cbVariableEdited(this.vars[idx].Name, val)
	}
}

// cancelEditVar leaves the inline value editor without firing (Esc, or a
// click elsewhere). A no-op when no row is being edited.
func (this *DebugPanel) cancelEditVar() {
	if this.editingVar < 0 {
		return
	}
	this.editingVar = -1
	this.varInput = ""
	this.Self().Update()
}

// focusWatchInput sets whether the expression input line holds focus,
// repainting on a change so the caret / placeholder swap.
func (this *DebugPanel) focusWatchInput(on bool) {
	if this.watchFocused == on {
		return
	}
	this.watchFocused = on
	this.Self().Update()
}

// submitWatchInput fires SigWatchAdded with the trimmed expression and
// clears the input line. A blank expression is ignored. The panel does NOT
// append to this.watches itself: adding needs an evaluation only the host
// can do, so the host evaluates the new expression and pushes the full list
// back via SetWatches (host-driven, like the rest of the panel).
func (this *DebugPanel) submitWatchInput() {
	expr := strings.TrimSpace(this.watchInput)
	this.watchInput = ""
	if expr != "" && this.cbWatchAdded != nil {
		this.cbWatchAdded(expr)
	}
	this.Self().Update()
}

// removeWatchAt drops watch idx and fires SigWatchRemoved with its
// expression. Unlike adds, a remove is a direct manipulation of the shown
// rows, so the panel updates its own list immediately for responsiveness;
// the host mirrors the change (and may still re-push via SetWatches).
func (this *DebugPanel) removeWatchAt(idx int) {
	if idx < 0 || idx >= len(this.watches) {
		return
	}
	expr := this.watches[idx].Expr
	this.watches = append(this.watches[:idx:idx], this.watches[idx+1:]...)
	this.hoverWatch = -1
	this.Self().Update()
	if this.cbWatchRemoved != nil {
		this.cbWatchRemoved(expr)
	}
}

// OnKeyDown routes keys to the inline value editor first when a locals row is
// being edited (Enter submits, Esc cancels, Backspace deletes a rune), then
// to the watch input when it holds focus (Enter submits the expression, Esc
// unfocuses, Backspace deletes a rune), otherwise it gives the call stack
// Qt-style keyboard control: Up/Down move the selection (re-firing
// SigFrameSelected so the host refreshes locals), Enter activates the frame.
func (this *DebugPanel) OnKeyDown(key int, repeat bool) {
	// The inline value editor takes keys first while a locals row is edited.
	if this.editingVar >= 0 {
		switch key {
		case gui.KeyEnter:
			this.submitEditVar()
		case gui.KeyEsc:
			this.cancelEditVar()
		case gui.KeyBackSpace:
			if r := []rune(this.varInput); len(r) > 0 {
				this.varInput = string(r[:len(r)-1])
				this.Self().Update()
			}
		}
		return
	}

	// The watch expression input takes keys next while it holds focus.
	if this.watchFocused {
		switch key {
		case gui.KeyEnter:
			this.submitWatchInput()
		case gui.KeyEsc:
			this.focusWatchInput(false)
		case gui.KeyBackSpace:
			if r := []rune(this.watchInput); len(r) > 0 {
				this.watchInput = string(r[:len(r)-1])
				this.Self().Update()
			}
		}
		return
	}

	if len(this.frames) == 0 {
		return
	}
	switch key {
	case gui.KeyDown:
		if this.selected < len(this.frames)-1 {
			this.selectFrame(this.selected + 1)
		}
	case gui.KeyUp:
		if this.selected > 0 {
			this.selectFrame(this.selected - 1)
		}
	case gui.KeyEnter:
		this.activateFrame(this.selected)
	}
}

// OnTextInput feeds typed characters into whichever text field is active: the
// inline value editor when a locals row is being edited, else the watch
// expression input while it holds focus. Enter and Backspace arrive via
// OnKeyDown, not here; with no active field, typing is ignored.
func (this *DebugPanel) OnTextInput(s string) {
	if s == "\r" || s == "\n" {
		return
	}
	// The inline value editor takes typed characters first while active.
	if this.editingVar >= 0 {
		this.varInput += s
		this.Self().Update()
		return
	}
	if !this.watchFocused {
		return
	}
	this.watchInput += s
	this.Self().Update()
}

// OnMouseMove tracks hover state for whichever section the cursor is over.
func (this *DebugPanel) OnMouseMove(x, y float64) {
	hs := this.stackRowAt(y)
	hv := this.varRowAt(y)
	hg := this.goroutineRowAt(y)
	hw := this.watchRowAt(y)
	if hs != this.hoverStack || hv != this.hoverVar || hg != this.hoverGoro || hw != this.hoverWatch {
		this.hoverStack = hs
		this.hoverVar = hv
		this.hoverGoro = hg
		this.hoverWatch = hw
		this.Self().Update()
	}
}

// OnMouseLeave clears every hover highlight.
func (this *DebugPanel) OnMouseLeave() {
	if this.hoverStack != -1 || this.hoverVar != -1 || this.hoverGoro != -1 || this.hoverWatch != -1 {
		this.hoverStack = -1
		this.hoverVar = -1
		this.hoverGoro = -1
		this.hoverWatch = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls whichever of the four sections the cursor is over.
// Each section clamps to its own content height; the watch view height
// excludes its header and input line.
func (this *DebugPanel) OnMouseWheel(x, y, z float64) {
	_, h := this.Size()
	stackH, varBottom, watchTop := this.sectionLayout()

	switch {
	case y < stackH:
		this.stackScrollY -= z * 3 * this.rowHeight
		this.stackScrollY = clampScroll(this.stackScrollY, float64(len(this.frames))*this.rowHeight, stackH-debugHeaderH)
	case y < varBottom:
		this.varScrollY -= z * 3 * this.rowHeight
		this.varScrollY = clampScroll(this.varScrollY, float64(len(this.vars))*this.rowHeight, (varBottom-stackH)-debugHeaderH)
	case y < watchTop:
		this.goroScrollY -= z * 3 * this.rowHeight
		this.goroScrollY = clampScroll(this.goroScrollY, float64(len(this.goroutines))*this.rowHeight, (watchTop-varBottom)-debugHeaderH)
	default:
		this.watchScrollY -= z * 3 * this.rowHeight
		this.watchScrollY = clampScroll(this.watchScrollY, float64(len(this.watches))*this.rowHeight, (h-watchTop)-debugHeaderH-this.rowHeight)
	}
	this.Self().Update()
}

// clampScroll pins a scroll offset into [0, max(0, content-view)].
func clampScroll(scrollY, contentH, viewH float64) float64 {
	if scrollY < 0 {
		return 0
	}
	maxScroll := contentH - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scrollY > maxScroll {
		return maxScroll
	}
	return scrollY
}

// stackRowAt maps a y coordinate to a call-stack frame index, or -1 when y
// is outside the stack section's rows (header, or below the band).
func (this *DebugPanel) stackRowAt(y float64) int {
	stackH, _, _ := this.sectionLayout()
	if y >= stackH {
		return -1
	}
	return frameRowAtY(y+this.stackScrollY, debugHeaderH, this.rowHeight, len(this.frames))
}

// varRowAt maps a y coordinate to a variables-section row index, or -1
// when y is outside that section's rows (the stack band above or the
// goroutines band below).
func (this *DebugPanel) varRowAt(y float64) int {
	stackH, varBottom, _ := this.sectionLayout()
	if y < stackH || y >= varBottom {
		return -1
	}
	return frameRowAtY(y+this.varScrollY, stackH+debugHeaderH, this.rowHeight, len(this.vars))
}

// goroutineRowAt maps a y coordinate to a goroutine-section row index, or -1
// when y is outside that section's rows (the locals band above, its own
// header, or the watch band below).
func (this *DebugPanel) goroutineRowAt(y float64) int {
	_, varBottom, watchTop := this.sectionLayout()
	if y < varBottom || y >= watchTop {
		return -1
	}
	return frameRowAtY(y+this.goroScrollY, varBottom+debugHeaderH, this.rowHeight, len(this.goroutines))
}

// watchInputAt reports whether y lands on the expression input line (the
// row directly under the watch header).
func (this *DebugPanel) watchInputAt(y float64) bool {
	_, _, watchTop := this.sectionLayout()
	inputY := watchTop + debugHeaderH
	return y >= inputY && y < inputY+this.rowHeight
}

// watchRowAt maps a y coordinate to a watched-expression index, or -1 when
// y is outside the watch rows (header, input line, or below the list). The
// rows start one row below the header (past the input line).
func (this *DebugPanel) watchRowAt(y float64) int {
	_, _, watchTop := this.sectionLayout()
	if y < watchTop {
		return -1
	}
	return frameRowAtY(y+this.watchScrollY, watchTop+debugHeaderH+this.rowHeight, this.rowHeight, len(this.watches))
}

func (this *DebugPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 120}
}
