package ged

import (
	"reflect"
	"strconv"
	"testing"

	"silk/core"
	"silk/gui"
)

// sampleFrames is a small call stack used across the tests: top frame
// first, matching dlv's Stacktrace ordering.
func sampleFrames() []core.StackFrame {
	return []core.StackFrame{
		{File: "/proj/a.go", Line: 10, Function: "main.foo"},
		{File: "/proj/b.go", Line: 20, Function: "main.bar"},
		{File: "/proj/c.go", Line: 30, Function: "main.baz"},
	}
}

// sampleVars is a small locals set: name / type / value.
func sampleVars() []core.Variable {
	return []core.Variable{
		{Name: "i", Type: "int", Value: "42"},
		{Name: "s", Type: "string", Value: "hello"},
	}
}

// TestDebugSetCallStackRoundTrip verifies SetCallStack stores the frames
// and CallStack() returns an equal — but independent — copy.
func TestDebugSetCallStackRoundTrip(t *testing.T) {
	p := NewDebugPanel()
	in := sampleFrames()
	p.SetCallStack(in)

	got := p.CallStack()
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("CallStack() = %+v\nwant %+v", got, in)
	}

	// Mutating the returned copy must not disturb the panel's state.
	got[0].Function = "MUTATED"
	if p.CallStack()[0].Function != "main.foo" {
		t.Error("CallStack() returned an aliasing slice, not a copy")
	}
}

// TestDebugSetVariablesRoundTrip verifies SetVariables / Variables() round
// trips and returns a copy.
func TestDebugSetVariablesRoundTrip(t *testing.T) {
	p := NewDebugPanel()
	in := sampleVars()
	p.SetVariables(in)

	got := p.Variables()
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("Variables() = %+v\nwant %+v", got, in)
	}

	got[0].Value = "MUTATED"
	if p.Variables()[0].Value != "42" {
		t.Error("Variables() returned an aliasing slice, not a copy")
	}
}

// TestDebugClear verifies Clear empties both sections and resets the
// selection to the top frame.
func TestDebugClear(t *testing.T) {
	p := NewDebugPanel()
	p.SetCallStack(sampleFrames())
	p.SetVariables(sampleVars())

	p.Clear()
	if got := p.CallStack(); len(got) != 0 {
		t.Errorf("after Clear, CallStack() = %+v, want empty", got)
	}
	if got := p.Variables(); len(got) != 0 {
		t.Errorf("after Clear, Variables() = %+v, want empty", got)
	}
	if p.SelectedFrame() != 0 {
		t.Errorf("after Clear, SelectedFrame() = %d, want 0", p.SelectedFrame())
	}
}

// TestDebugSelectedFrameDefault verifies the top frame (index 0) is
// selected by default after a stack is pushed.
func TestDebugSelectedFrameDefault(t *testing.T) {
	p := NewDebugPanel()
	p.SetCallStack(sampleFrames())
	if p.SelectedFrame() != 0 {
		t.Errorf("SelectedFrame() = %d, want 0 (top frame)", p.SelectedFrame())
	}
}

// TestDebugFrameClickSelectsRow2 simulates a click that lands on stack row
// 2 and checks SelectedFrame() updates and SigFrameSelected fires with the
// right index and frame.
//
// Geometry: with the panel sized 300x400, stackBandHeight = 180 (well past
// the header), rows start at debugHeaderH=22 with rowHeight=20 and no
// scroll. Row 2 occupies y in [22+40, 22+60) = [62, 82); click the middle.
func TestDebugFrameClickSelectsRow2(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	frames := sampleFrames()
	p.SetCallStack(frames)

	var (
		gotIdx   = -1
		gotFrame core.StackFrame
		fired    bool
	)
	p.SigFrameSelected(func(index int, frame core.StackFrame) {
		gotIdx = index
		gotFrame = frame
		fired = true
	})

	y := debugHeaderH + 2*p.rowHeight + p.rowHeight/2 // 72
	p.OnLeftDown(5, y)

	if !fired {
		t.Fatal("OnLeftDown did not fire SigFrameSelected")
	}
	if p.SelectedFrame() != 2 {
		t.Errorf("SelectedFrame() = %d, want 2", p.SelectedFrame())
	}
	if gotIdx != 2 {
		t.Errorf("SigFrameSelected index = %d, want 2", gotIdx)
	}
	if !reflect.DeepEqual(gotFrame, frames[2]) {
		t.Errorf("SigFrameSelected frame = %+v, want %+v", gotFrame, frames[2])
	}
}

// TestDebugFrameClickHeaderNoop verifies a click in the call-stack header
// band does not select or fire.
func TestDebugFrameClickHeaderNoop(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetCallStack(sampleFrames())

	fired := false
	p.SigFrameSelected(func(int, core.StackFrame) { fired = true })
	p.OnLeftDown(5, 5) // inside the 22px header
	if fired {
		t.Error("OnLeftDown in header region fired SigFrameSelected")
	}
	if p.SelectedFrame() != 0 {
		t.Errorf("SelectedFrame() = %d after header click, want 0", p.SelectedFrame())
	}
}

// TestFrameRowAtY exercises the pure hit-test helper directly: rows start
// at topOffset, rowH tall; out-of-range and degenerate inputs yield -1.
func TestFrameRowAtY(t *testing.T) {
	const (
		top = 22.0
		rh  = 20.0
		n   = 3
	)
	cases := []struct {
		name string
		y    float64
		want int
	}{
		{"above rows (header)", 10, -1},
		{"top of row 0", top, 0},
		{"middle of row 0", top + rh/2, 0},
		{"middle of row 2", top + 2*rh + rh/2, 2},
		{"last pixel of row 2", top + 3*rh - 0.5, 2},
		{"just past last row", top + 3*rh, -1},
		{"far below", 10000, -1},
	}
	for _, c := range cases {
		if got := frameRowAtY(c.y, top, rh, n); got != c.want {
			t.Errorf("%s: frameRowAtY(%v,%v,%v,%d) = %d, want %d",
				c.name, c.y, top, rh, n, got, c.want)
		}
	}
	// Degenerate row height must not divide by zero — return -1.
	if got := frameRowAtY(50, top, 0, n); got != -1 {
		t.Errorf("frameRowAtY with rowH=0 = %d, want -1", got)
	}
	// Empty list: every y is out of range.
	if got := frameRowAtY(top+5, top, rh, 0); got != -1 {
		t.Errorf("frameRowAtY with count=0 = %d, want -1", got)
	}
}

// TestTruncateValue covers the pure truncation helper: short strings pass
// through, over-length strings get an ellipsis, and the exact-boundary
// length is left untouched.
func TestTruncateValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"short unchanged", "hi", 10, "hi"},
		{"exact boundary unchanged", "abcde", 5, "abcde"},
		{"one over truncates", "abcdef", 5, "abcd…"},
		{"long truncates", "abcdefghij", 4, "abc…"},
		{"max one keeps only ellipsis", "abc", 1, "…"},
		{"max zero empty", "abc", 0, ""},
		{"max negative empty", "abc", -3, ""},
		{"empty input", "", 5, ""},
	}
	for _, c := range cases {
		if got := truncateValue(c.in, c.max); got != c.want {
			t.Errorf("%s: truncateValue(%q,%d) = %q, want %q",
				c.name, c.in, c.max, got, c.want)
		}
	}
	// Result width never exceeds max (in runes) once truncation kicks in.
	got := truncateValue("αβγδεζη", 4) // multi-byte runes
	if want := "αβγ…"; got != want {
		t.Errorf("multibyte truncate = %q, want %q", got, want)
	}
	if r := []rune(got); len(r) != 4 {
		t.Errorf("multibyte truncate rune-width = %d, want 4", len(r))
	}
}

// TestDebugKeyNavigation drives OnKeyDown: Down advances the selection
// (re-firing SigFrameSelected) and clamps at the last frame; Up walks back
// and clamps at the top; Enter fires SigFrameActivated for the selected
// frame.
func TestDebugKeyNavigation(t *testing.T) {
	p := NewDebugPanel()
	frames := sampleFrames()
	p.SetCallStack(frames)

	var lastSelected = -1
	p.SigFrameSelected(func(index int, _ core.StackFrame) { lastSelected = index })

	p.OnKeyDown(gui.KeyDown, false)
	if p.SelectedFrame() != 1 || lastSelected != 1 {
		t.Fatalf("after Down: SelectedFrame=%d lastSelected=%d, want 1/1", p.SelectedFrame(), lastSelected)
	}
	p.OnKeyDown(gui.KeyDown, false)
	p.OnKeyDown(gui.KeyDown, false) // clamp at last (index 2)
	if p.SelectedFrame() != 2 {
		t.Fatalf("after Down*3: SelectedFrame=%d, want 2 (clamped)", p.SelectedFrame())
	}

	p.OnKeyDown(gui.KeyUp, false)
	p.OnKeyDown(gui.KeyUp, false)
	p.OnKeyDown(gui.KeyUp, false) // clamp at top (index 0)
	if p.SelectedFrame() != 0 {
		t.Fatalf("after Up*3: SelectedFrame=%d, want 0 (clamped)", p.SelectedFrame())
	}

	var activated core.StackFrame
	activatedFired := false
	p.SigFrameActivated(func(f core.StackFrame) {
		activated = f
		activatedFired = true
	})
	p.OnKeyDown(gui.KeyEnter, false)
	if !activatedFired {
		t.Fatal("Enter did not fire SigFrameActivated")
	}
	if !reflect.DeepEqual(activated, frames[0]) {
		t.Errorf("activated frame = %+v, want %+v", activated, frames[0])
	}
}

// TestDebugStackBandHeightSplit checks the section-split geometry stays
// sane: the stack band always leaves the variables header room, and never
// collapses below its own header. (No Draw call — like the sibling panel
// tests, this package keeps Draw out of the test path so cairo/GL is never
// touched headless.)
func TestDebugStackBandHeightSplit(t *testing.T) {
	p := NewDebugPanel()

	// Comfortable height: stack gets ~45%, variables header still fits.
	if h := p.stackBandHeight(400); h <= debugHeaderH || h > 400-debugHeaderH {
		t.Errorf("stackBandHeight(400) = %v, want in (%v, %v]", h, debugHeaderH, 400-debugHeaderH)
	}
	// Mid height where the 2-row minimum kicks in but the variables header
	// still fits: result is the floor (header + 2 rows), not 45%.
	if h := p.stackBandHeight(120); h != debugHeaderH+p.rowHeight*2 {
		t.Errorf("stackBandHeight(120) = %v, want %v (2-row floor)", h, debugHeaderH+p.rowHeight*2)
	}
	// Degenerate tiny height (< two headers): the one-header floor wins so
	// the stack header never disappears.
	if h := p.stackBandHeight(30); h != debugHeaderH {
		t.Errorf("stackBandHeight(30) = %v, want %v (one-header floor)", h, debugHeaderH)
	}
}

// sampleWatches is a small watch set: one evaluated OK, one in error.
func sampleWatches() []WatchEntry {
	return []WatchEntry{
		{Expr: "i + 1", Value: "43", Type: "int"},
		{Expr: "bogus", Err: "could not find symbol"},
	}
}

// TestDebugSetWatchesRoundTrip verifies SetWatches stores the entries and
// Watches() returns an equal — but independent — copy.
func TestDebugSetWatchesRoundTrip(t *testing.T) {
	p := NewDebugPanel()
	in := sampleWatches()
	p.SetWatches(in)

	got := p.Watches()
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("Watches() = %+v\nwant %+v", got, in)
	}

	// Mutating the returned copy must not disturb the panel's state.
	got[0].Value = "MUTATED"
	if p.Watches()[0].Value != "43" {
		t.Error("Watches() returned an aliasing slice, not a copy")
	}
	// And mutating the input after SetWatches must not leak in either.
	in[0].Value = "LEAK"
	if p.Watches()[0].Value != "43" {
		t.Error("SetWatches aliased the caller's slice instead of copying")
	}
}

// TestDebugWatchExprs verifies WatchExprs returns just the expressions in
// display order.
func TestDebugWatchExprs(t *testing.T) {
	p := NewDebugPanel()
	p.SetWatches(sampleWatches())
	got := p.WatchExprs()
	want := []string{"i + 1", "bogus"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WatchExprs() = %v, want %v", got, want)
	}
}

// TestDebugClearKeepsWatchExprs verifies Clear() empties call stack + locals
// and blanks watch VALUES while KEEPING the expressions (persist-across-stops
// semantics of a real debugger).
func TestDebugClearKeepsWatchExprs(t *testing.T) {
	p := NewDebugPanel()
	p.SetCallStack(sampleFrames())
	p.SetVariables(sampleVars())
	p.SetWatches(sampleWatches())

	p.Clear()

	if got := p.CallStack(); len(got) != 0 {
		t.Errorf("after Clear, CallStack() = %+v, want empty", got)
	}
	if got := p.Variables(); len(got) != 0 {
		t.Errorf("after Clear, Variables() = %+v, want empty", got)
	}
	ws := p.Watches()
	if len(ws) != 2 {
		t.Fatalf("after Clear, Watches() len = %d, want 2 (expressions kept)", len(ws))
	}
	if ws[0].Expr != "i + 1" || ws[1].Expr != "bogus" {
		t.Errorf("after Clear, exprs = %q/%q, want kept", ws[0].Expr, ws[1].Expr)
	}
	for i, w := range ws {
		if w.Value != "" || w.Type != "" || w.Err != "" {
			t.Errorf("after Clear, watch %d still has values: %+v, want blanked", i, w)
		}
	}
}

// TestDebugClearAll verifies ClearAll drops the watch expressions and the
// in-progress input as well.
func TestDebugClearAll(t *testing.T) {
	p := NewDebugPanel()
	p.SetWatches(sampleWatches())
	p.focusWatchInput(true)
	p.OnTextInput("half typed")

	p.ClearAll()

	if got := p.Watches(); len(got) != 0 {
		t.Errorf("after ClearAll, Watches() = %+v, want empty", got)
	}
	if p.watchInput != "" {
		t.Errorf("after ClearAll, watchInput = %q, want empty", p.watchInput)
	}
	if p.watchFocused {
		t.Error("after ClearAll, watch input still focused")
	}
}

// TestDebugWatchSubmitFiresAdded drives the input path: focus the line, type
// an expression, press Enter. SigWatchAdded fires with the typed text and
// the input clears afterward. The panel does not append to its own list —
// the host owns that via SetWatches.
func TestDebugWatchSubmitFiresAdded(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)

	var got string
	fired := false
	p.SigWatchAdded(func(expr string) { got = expr; fired = true })

	// Typing while unfocused is ignored (no other text field on the panel).
	p.OnTextInput("ignored")
	if p.watchInput != "" {
		t.Fatalf("unfocused OnTextInput edited the input: %q", p.watchInput)
	}

	p.focusWatchInput(true)
	p.OnTextInput("i")
	p.OnTextInput(" + 1")
	if p.watchInput != "i + 1" {
		t.Fatalf("watchInput = %q, want %q", p.watchInput, "i + 1")
	}
	p.OnKeyDown(gui.KeyEnter, false)

	if !fired {
		t.Fatal("Enter did not fire SigWatchAdded")
	}
	if got != "i + 1" {
		t.Errorf("SigWatchAdded expr = %q, want %q", got, "i + 1")
	}
	if p.watchInput != "" {
		t.Errorf("after submit, watchInput = %q, want cleared", p.watchInput)
	}

	// A blank submit is ignored (no fire, input stays empty).
	fired = false
	p.focusWatchInput(true)
	p.OnTextInput("   ")
	p.OnKeyDown(gui.KeyEnter, false)
	if fired {
		t.Error("blank expression should not fire SigWatchAdded")
	}
}

// TestDebugWatchBackspaceEsc verifies Backspace deletes a rune in the input
// and Esc unfocuses it (leaving the text alone).
func TestDebugWatchBackspaceEsc(t *testing.T) {
	p := NewDebugPanel()
	p.focusWatchInput(true)
	p.OnTextInput("ab")
	p.OnKeyDown(gui.KeyBackSpace, false)
	if p.watchInput != "a" {
		t.Errorf("after Backspace, watchInput = %q, want %q", p.watchInput, "a")
	}
	p.OnKeyDown(gui.KeyEsc, false)
	if p.watchFocused {
		t.Error("Esc did not unfocus the watch input")
	}
}

// TestDebugWatchRemoveHelper verifies RemoveWatch drops the matching row and
// fires SigWatchRemoved with its expression.
func TestDebugWatchRemoveHelper(t *testing.T) {
	p := NewDebugPanel()
	p.SetWatches(sampleWatches())

	var got string
	fired := false
	p.SigWatchRemoved(func(expr string) { got = expr; fired = true })

	p.RemoveWatch("i + 1")
	if !fired {
		t.Fatal("RemoveWatch did not fire SigWatchRemoved")
	}
	if got != "i + 1" {
		t.Errorf("SigWatchRemoved expr = %q, want %q", got, "i + 1")
	}
	if left := p.WatchExprs(); !reflect.DeepEqual(left, []string{"bogus"}) {
		t.Errorf("after remove, WatchExprs() = %v, want [bogus]", left)
	}

	// Removing a non-watched expression is a no-op.
	fired = false
	p.RemoveWatch("not there")
	if fired {
		t.Error("removing a non-watched expression fired SigWatchRemoved")
	}
}

// TestDebugWatchRemoveClick drives the ✕ hot-zone: a click on the right edge
// of a watch row removes it; a click elsewhere on the row does not.
//
// Geometry (300x400): watchTop=280, input line [302,322), row 0 [322,342).
// The ✕ zone is x >= 300-watchRemoveW = 280.
func TestDebugWatchRemoveClick(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetWatches(sampleWatches())

	removed := ""
	p.SigWatchRemoved(func(expr string) { removed = expr })

	// Click on the row body (not the ✕ zone): no removal.
	p.OnLeftDown(10, 332)
	if removed != "" {
		t.Fatalf("row-body click removed %q, want no removal", removed)
	}
	if len(p.WatchExprs()) != 2 {
		t.Fatalf("row-body click changed the list: %v", p.WatchExprs())
	}

	// Click in the ✕ zone of row 0: removes "i + 1".
	p.OnLeftDown(290, 332)
	if removed != "i + 1" {
		t.Errorf("✕ click removed %q, want %q", removed, "i + 1")
	}
	if left := p.WatchExprs(); !reflect.DeepEqual(left, []string{"bogus"}) {
		t.Errorf("after ✕ click, WatchExprs() = %v, want [bogus]", left)
	}
}

// TestDebugWatchHitTest exercises the watch-band hit-tests against the known
// geometry at 300x400 (watchTop=280, input [302,322), rows start at 322).
func TestDebugWatchHitTest(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetWatches([]WatchEntry{{Expr: "a"}, {Expr: "b"}, {Expr: "c"}})

	// Input line detection.
	if !p.watchInputAt(310) {
		t.Error("watchInputAt(310) = false, want true (input line)")
	}
	if p.watchInputAt(290) {
		t.Error("watchInputAt(290) = true, want false (that's the header)")
	}
	if p.watchInputAt(332) {
		t.Error("watchInputAt(332) = true, want false (that's a row)")
	}

	// Row hit-test.
	cases := []struct {
		name string
		y    float64
		want int
	}{
		{"above watch band", 200, -1},
		{"header", 290, -1},
		{"input line", 310, -1},
		{"row 0", 332, 0},
		{"row 1", 352, 1},
		{"row 2", 372, 2},
	}
	for _, c := range cases {
		if got := p.watchRowAt(c.y); got != c.want {
			t.Errorf("%s: watchRowAt(%v) = %d, want %d", c.name, c.y, got, c.want)
		}
	}
}

// TestDebugWatchWheelIsolation verifies the wheel routes to the watch band's
// own scroll when the cursor is over it, leaving stack/var scroll untouched.
func TestDebugWatchWheelIsolation(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	// Enough rows that the watch band can scroll.
	ws := make([]WatchEntry, 40)
	for i := range ws {
		ws[i] = WatchEntry{Expr: "w" + strconv.Itoa(i)}
	}
	p.SetWatches(ws)

	// Scroll up (z<0) with the cursor in the watch band (y past watchTop=280).
	p.OnMouseWheel(150, 350, -1)
	if p.watchScrollY <= 0 {
		t.Errorf("watchScrollY = %v, want > 0 after wheel in watch band", p.watchScrollY)
	}
	if p.stackScrollY != 0 || p.varScrollY != 0 {
		t.Errorf("wheel in watch band moved other sections: stack=%v var=%v", p.stackScrollY, p.varScrollY)
	}
}

// sampleGoroutines is a small goroutine set: id / file / line / function, in
// the shape core.DebugSession.ListGoroutines returns.
func sampleGoroutines() []core.Goroutine {
	return []core.Goroutine{
		{ID: 1, File: "/proj/a.go", Line: 10, Function: "main.foo"},
		{ID: 18, File: "/proj/b.go", Line: 20, Function: "main.bar"},
		{ID: 42, File: "/proj/c.go", Line: 30, Function: "main.worker"},
	}
}

// TestDebugSetGoroutinesRoundTrip verifies SetGoroutines stores the rows and
// Goroutines() returns an equal — but independent — copy in both directions.
func TestDebugSetGoroutinesRoundTrip(t *testing.T) {
	p := NewDebugPanel()
	in := sampleGoroutines()
	p.SetGoroutines(in)

	got := p.Goroutines()
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("Goroutines() = %+v\nwant %+v", got, in)
	}

	// Mutating the returned copy must not disturb the panel's state.
	got[0].Function = "MUTATED"
	if p.Goroutines()[0].Function != "main.foo" {
		t.Error("Goroutines() returned an aliasing slice, not a copy")
	}
	// And mutating the input after SetGoroutines must not leak in either.
	in[0].Function = "LEAK"
	if p.Goroutines()[0].Function != "main.foo" {
		t.Error("SetGoroutines aliased the caller's slice instead of copying")
	}
}

// TestDebugClearClearsGoroutines verifies Clear() empties the goroutines band
// too (they are stop-scoped like the call stack, dropped on continue).
func TestDebugClearClearsGoroutines(t *testing.T) {
	p := NewDebugPanel()
	p.SetCallStack(sampleFrames())
	p.SetVariables(sampleVars())
	p.SetGoroutines(sampleGoroutines())

	p.Clear()

	if got := p.Goroutines(); len(got) != 0 {
		t.Errorf("after Clear, Goroutines() = %+v, want empty", got)
	}
}

// TestDebugGoroutineHitTest exercises the goroutine-band hit-test against the
// known geometry at 300x400: stack [0,126), locals [126,218), goroutines
// [218,280) with header [218,240) and rows starting at 240, watch [280,400).
func TestDebugGoroutineHitTest(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetGoroutines(sampleGoroutines())

	cases := []struct {
		name string
		y    float64
		want int
	}{
		{"stack band", 60, -1},
		{"locals band", 150, -1},
		{"goroutine header", 225, -1},
		{"row 0", 250, 0},
		{"row 1", 270, 1},
		{"watch band", 300, -1},
	}
	for _, c := range cases {
		if got := p.goroutineRowAt(c.y); got != c.want {
			t.Errorf("%s: goroutineRowAt(%v) = %d, want %d", c.name, c.y, got, c.want)
		}
	}
}

// TestDebugGoroutineClickActivates simulates a click on goroutine row 0 and
// checks SigGoroutineActivated fires with the right goroutine (the host opens
// its file:line). Geometry: row 0 occupies y in [240,260); click the middle.
func TestDebugGoroutineClickActivates(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	gs := sampleGoroutines()
	p.SetGoroutines(gs)

	var (
		got   core.Goroutine
		fired bool
	)
	p.SigGoroutineActivated(func(g core.Goroutine) { got = g; fired = true })

	p.OnLeftDown(10, 250)

	if !fired {
		t.Fatal("OnLeftDown on a goroutine row did not fire SigGoroutineActivated")
	}
	if !reflect.DeepEqual(got, gs[0]) {
		t.Errorf("SigGoroutineActivated goroutine = %+v, want %+v", got, gs[0])
	}
}

// TestDebugVariableEditSubmit drives the inline value editor: begin-edit on a
// locals row seeds the input with the current value; editing then Enter fires
// SigVariableEdited(name, newText) and leaves edit mode. The panel does NOT
// mutate its own vars — the host owns that via dlv SetVariable + SetVariables.
func TestDebugVariableEditSubmit(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetVariables(sampleVars()) // {i int 42} {s string hello}

	var (
		gotName string
		gotVal  string
		fired   bool
	)
	p.SigVariableEdited(func(name, newValue string) {
		gotName = name
		gotVal = newValue
		fired = true
	})

	// Begin editing row 0 (i = 42): the editor seeds with the current value.
	p.beginEditVar(0)
	if p.editingVar != 0 {
		t.Fatalf("editingVar = %d, want 0", p.editingVar)
	}
	if p.varInput != "42" {
		t.Fatalf("varInput = %q, want seeded %q", p.varInput, "42")
	}

	// Edit the value and submit.
	p.OnKeyDown(gui.KeyBackSpace, false) // "42" -> "4"
	p.OnKeyDown(gui.KeyBackSpace, false) // "4"  -> ""
	p.OnTextInput("100")
	if p.varInput != "100" {
		t.Fatalf("varInput = %q, want %q", p.varInput, "100")
	}
	p.OnKeyDown(gui.KeyEnter, false)

	if !fired {
		t.Fatal("Enter did not fire SigVariableEdited")
	}
	if gotName != "i" || gotVal != "100" {
		t.Errorf("SigVariableEdited = (%q,%q), want (%q,%q)", gotName, gotVal, "i", "100")
	}
	if p.editingVar != -1 {
		t.Errorf("after submit, editingVar = %d, want -1 (edit mode exited)", p.editingVar)
	}
	if p.varInput != "" {
		t.Errorf("after submit, varInput = %q, want cleared", p.varInput)
	}
	// The value was not applied locally — the host round-trips it.
	if p.Variables()[0].Value != "42" {
		t.Errorf("var value changed locally to %q, want unchanged (host-driven)", p.Variables()[0].Value)
	}
}

// TestDebugVariableEditEscCancels verifies Esc leaves the inline editor
// without firing SigVariableEdited and clears the edit state.
func TestDebugVariableEditEscCancels(t *testing.T) {
	p := NewDebugPanel()
	p.SetVariables(sampleVars())

	fired := false
	p.SigVariableEdited(func(string, string) { fired = true })

	p.beginEditVar(1) // editing s = "hello"
	if p.varInput != "hello" {
		t.Fatalf("varInput = %q, want seeded %q", p.varInput, "hello")
	}
	p.OnTextInput("X") // "helloX"
	p.OnKeyDown(gui.KeyEsc, false)

	if fired {
		t.Error("Esc fired SigVariableEdited, want no signal on cancel")
	}
	if p.editingVar != -1 {
		t.Errorf("after Esc, editingVar = %d, want -1", p.editingVar)
	}
	if p.varInput != "" {
		t.Errorf("after Esc, varInput = %q, want cleared", p.varInput)
	}
}

// TestDebugVariableDoubleClickBeginsEdit drives the UI entry point: a single
// click on a locals row only arms the double-click, a quick second click on
// the same row opens the inline editor seeded with that row's value.
//
// Geometry (300x400): locals band [126,218), header [126,148), row 0 [148,168).
func TestDebugVariableDoubleClickBeginsEdit(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetVariables(sampleVars())

	y := 158.0
	p.OnLeftDown(10, y) // first click: arms the double-click, no edit yet
	if p.editingVar != -1 {
		t.Fatalf("single click began edit (editingVar=%d), want -1", p.editingVar)
	}
	p.OnLeftDown(10, y) // quick second click on the same row: begins edit
	if p.editingVar != 0 {
		t.Fatalf("double click did not begin edit: editingVar=%d, want 0", p.editingVar)
	}
	if p.varInput != "42" {
		t.Errorf("varInput = %q, want seeded %q", p.varInput, "42")
	}
}
