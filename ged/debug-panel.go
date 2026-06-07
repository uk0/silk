package ged

import (
	"path/filepath"
	"strconv"
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
// stack and the local variables at a breakpoint. It is a pure
// display/interaction widget: it never talks to dlv itself. The host
// (silkide) drives a core.DebugSession — Stacktrace() / ListLocals() /
// Eval() — and pushes the results in via SetCallStack / SetVariables.
// The panel renders them and emits two signals back:
//
//	SigFrameSelected  — a stack-frame row was clicked. The host re-fetches
//	                    locals for that frame (ListLocals at the frame's
//	                    depth) and calls SetVariables to refresh the lower
//	                    section.
//	SigFrameActivated — a stack-frame row was double-clicked / Enter'd. The
//	                    host opens frame.File:frame.Line in the editor.
//
// The two sections share the widget vertically: the call stack occupies
// the top band, the variables the bottom. Each band has its own header
// (with a count) and its own independent vertical scroll.
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

// Clear empties both sections. The host calls this when the debug session
// ends or the program continues — there is no stopped frame to show.
func (this *DebugPanel) Clear() {
	this.frames = nil
	this.vars = nil
	this.selected = 0
	this.stackScrollY = 0
	this.varScrollY = 0
	this.hoverStack = -1
	this.hoverVar = -1
	this.lastClickIdx = -1
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

// --- Drawing ---

// Draw paints the two stacked sections: call stack on top, variables on
// the bottom, each with a counted header and its own scrolled row list.
func (this *DebugPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling panes (log/problems).
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)

	stackH := this.stackBandHeight(h)
	this.drawStackSection(g, font, w, stackH)
	this.drawVarSection(g, font, w, stackH, h-stackH)
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

// --- Events ---

// OnLeftDown selects the clicked call-stack frame (firing SigFrameSelected)
// and treats a quick second click on the same frame as activation (firing
// SigFrameActivated). Clicks in the variables section are inert in v1 —
// locals are display-only.
func (this *DebugPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

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

// OnKeyDown gives the call stack Qt-style keyboard control while the panel
// holds focus: Up/Down move the selection (re-firing SigFrameSelected so
// the host refreshes locals), Enter activates the selected frame.
func (this *DebugPanel) OnKeyDown(key int, repeat bool) {
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

// OnMouseMove tracks hover state for whichever section the cursor is over.
func (this *DebugPanel) OnMouseMove(x, y float64) {
	hs := this.stackRowAt(y)
	hv := this.varRowAt(y)
	if hs != this.hoverStack || hv != this.hoverVar {
		this.hoverStack = hs
		this.hoverVar = hv
		this.Self().Update()
	}
}

// OnMouseLeave clears both hover highlights.
func (this *DebugPanel) OnMouseLeave() {
	if this.hoverStack != -1 || this.hoverVar != -1 {
		this.hoverStack = -1
		this.hoverVar = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls whichever section the cursor is over. Each section
// clamps to its own content height.
func (this *DebugPanel) OnMouseWheel(x, y, z float64) {
	_, h := this.Size()
	stackH := this.stackBandHeight(h)

	if y < stackH {
		this.stackScrollY -= z * 3 * this.rowHeight
		this.stackScrollY = clampScroll(this.stackScrollY, float64(len(this.frames))*this.rowHeight, stackH-debugHeaderH)
	} else {
		this.varScrollY -= z * 3 * this.rowHeight
		this.varScrollY = clampScroll(this.varScrollY, float64(len(this.vars))*this.rowHeight, (h-stackH)-debugHeaderH)
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
	_, h := this.Size()
	stackH := this.stackBandHeight(h)
	if y >= stackH {
		return -1
	}
	return frameRowAtY(y+this.stackScrollY, debugHeaderH, this.rowHeight, len(this.frames))
}

// varRowAt maps a y coordinate to a variables-section row index, or -1
// when y is outside that section's rows.
func (this *DebugPanel) varRowAt(y float64) int {
	_, h := this.Size()
	stackH := this.stackBandHeight(h)
	if y < stackH {
		return -1
	}
	return frameRowAtY(y+this.varScrollY, stackH+debugHeaderH, this.rowHeight, len(this.vars))
}

func (this *DebugPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 120}
}
