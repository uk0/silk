package ged

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"silk/core"
	"silk/gui"
	"silk/paint"
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
//
// The three bands split the widget vertically: the call stack on top, the
// locals in the middle, the watch section at the bottom. Each band has its
// own header (with a count) and its own independent vertical scroll.
//
// Goroutine listing (core.DebugSession.ListGoroutines) is a deliberate
// follow-up: v1 shows call stack + locals only, which is what a "stopped
// at breakpoint" view needs first. A future commit adds a third section
// fed the same data-push way.
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
	this.lastClickIdx = -1
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

// sectionLayout returns the vertical band split for the current widget
// height: the call-stack band spans [0, stackH), the locals band spans
// [stackH, watchTop), and the watch band spans [watchTop, height). Kept in
// one place so Draw and every hit-test agree on the boundaries. The stack
// band is sized against the upper region (everything above the watch band)
// so stackBandHeight's own math and tests stay unchanged.
func (this *DebugPanel) sectionLayout() (stackH, watchTop float64) {
	_, h := this.Size()
	watchTop = h - this.watchBandHeight(h)
	stackH = this.stackBandHeight(watchTop)
	return
}

// --- Drawing ---

// Draw paints the three stacked sections: call stack on top, variables in
// the middle, watch on the bottom, each with a counted header and its own
// scrolled row list.
func (this *DebugPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling panes (log/problems).
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)

	stackH, watchTop := this.sectionLayout()
	this.drawStackSection(g, font, w, stackH)
	this.drawVarSection(g, font, w, stackH, watchTop-stackH)
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

		// "= Value", value truncated to a sensible width so a huge struct
		// dump can't run off the row.
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
		g.DrawText1(x, y+fe.Ascent+2, "= "+truncateValue(v.Value, 80))
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

// OnLeftDown routes a click to the section it lands in. In the watch band
// it focuses the expression input, or removes a row when the ✕ hot-zone is
// hit; any click outside the input blurs it. In the call stack it selects
// the clicked frame (firing SigFrameSelected) and treats a quick second
// click on the same frame as activation (firing SigFrameActivated). Clicks
// in the variables section are inert — locals are display-only.
func (this *DebugPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

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

// OnKeyDown routes keys to the watch input when it holds focus (Enter
// submits the expression, Esc unfocuses, Backspace deletes a rune),
// otherwise it gives the call stack Qt-style keyboard control: Up/Down move
// the selection (re-firing SigFrameSelected so the host refreshes locals),
// Enter activates the selected frame.
func (this *DebugPanel) OnKeyDown(key int, repeat bool) {
	// The watch expression input takes keys first while it holds focus.
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

// OnTextInput feeds typed characters into the watch expression input while
// it holds focus. Enter and Backspace arrive via OnKeyDown, not here; when
// the input is unfocused, typing is ignored (the panel has no other text
// field).
func (this *DebugPanel) OnTextInput(s string) {
	if !this.watchFocused {
		return
	}
	if s == "\r" || s == "\n" {
		return
	}
	this.watchInput += s
	this.Self().Update()
}

// OnMouseMove tracks hover state for whichever section the cursor is over.
func (this *DebugPanel) OnMouseMove(x, y float64) {
	hs := this.stackRowAt(y)
	hv := this.varRowAt(y)
	hw := this.watchRowAt(y)
	if hs != this.hoverStack || hv != this.hoverVar || hw != this.hoverWatch {
		this.hoverStack = hs
		this.hoverVar = hv
		this.hoverWatch = hw
		this.Self().Update()
	}
}

// OnMouseLeave clears every hover highlight.
func (this *DebugPanel) OnMouseLeave() {
	if this.hoverStack != -1 || this.hoverVar != -1 || this.hoverWatch != -1 {
		this.hoverStack = -1
		this.hoverVar = -1
		this.hoverWatch = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls whichever of the three sections the cursor is over.
// Each section clamps to its own content height; the watch view height
// excludes its header and input line.
func (this *DebugPanel) OnMouseWheel(x, y, z float64) {
	_, h := this.Size()
	stackH, watchTop := this.sectionLayout()

	switch {
	case y < stackH:
		this.stackScrollY -= z * 3 * this.rowHeight
		this.stackScrollY = clampScroll(this.stackScrollY, float64(len(this.frames))*this.rowHeight, stackH-debugHeaderH)
	case y < watchTop:
		this.varScrollY -= z * 3 * this.rowHeight
		this.varScrollY = clampScroll(this.varScrollY, float64(len(this.vars))*this.rowHeight, (watchTop-stackH)-debugHeaderH)
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
	stackH, _ := this.sectionLayout()
	if y >= stackH {
		return -1
	}
	return frameRowAtY(y+this.stackScrollY, debugHeaderH, this.rowHeight, len(this.frames))
}

// varRowAt maps a y coordinate to a variables-section row index, or -1
// when y is outside that section's rows (either band above or the watch
// band below).
func (this *DebugPanel) varRowAt(y float64) int {
	stackH, watchTop := this.sectionLayout()
	if y < stackH || y >= watchTop {
		return -1
	}
	return frameRowAtY(y+this.varScrollY, stackH+debugHeaderH, this.rowHeight, len(this.vars))
}

// watchInputAt reports whether y lands on the expression input line (the
// row directly under the watch header).
func (this *DebugPanel) watchInputAt(y float64) bool {
	_, watchTop := this.sectionLayout()
	inputY := watchTop + debugHeaderH
	return y >= inputY && y < inputY+this.rowHeight
}

// watchRowAt maps a y coordinate to a watched-expression index, or -1 when
// y is outside the watch rows (header, input line, or below the list). The
// rows start one row below the header (past the input line).
func (this *DebugPanel) watchRowAt(y float64) int {
	_, watchTop := this.sectionLayout()
	if y < watchTop {
		return -1
	}
	return frameRowAtY(y+this.watchScrollY, watchTop+debugHeaderH+this.rowHeight, this.rowHeight, len(this.watches))
}

func (this *DebugPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 120}
}
