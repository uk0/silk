package ged

import (
	"reflect"
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
