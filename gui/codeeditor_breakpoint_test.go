package gui

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Breakpoint state API (pure, no GL / no Draw)
//
// Breakpoints are keyed 0-based, matching cursorLine / bookmarks. These tests
// exercise the UI/state layer only; there is no debugger integration.
// ---------------------------------------------------------------------------

func TestCodeEditorBreakpointToggle(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc\nd")

	if len(e.Breakpoints()) != 0 {
		t.Fatalf("new editor should have no breakpoints, got %v", e.Breakpoints())
	}

	e.ToggleBreakpoint(2)
	if !e.breakpoints[2] {
		t.Errorf("ToggleBreakpoint(2) should set line 2")
	}
	if got := e.Breakpoints(); !reflect.DeepEqual(got, []int{2}) {
		t.Errorf("Breakpoints() = %v, want [2]", got)
	}

	e.ToggleBreakpoint(2)
	if e.breakpoints[2] {
		t.Errorf("ToggleBreakpoint(2) again should clear line 2")
	}
	if got := e.Breakpoints(); len(got) != 0 {
		t.Errorf("Breakpoints() = %v, want empty", got)
	}
}

func TestCodeEditorBreakpointSet(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")

	e.SetBreakpoint(1, true)
	if !e.breakpoints[1] {
		t.Errorf("SetBreakpoint(1, true) should set line 1")
	}
	// Setting an already-on breakpoint stays on.
	e.SetBreakpoint(1, true)
	if got := e.Breakpoints(); !reflect.DeepEqual(got, []int{1}) {
		t.Errorf("Breakpoints() = %v, want [1]", got)
	}

	e.SetBreakpoint(1, false)
	if e.breakpoints[1] {
		t.Errorf("SetBreakpoint(1, false) should clear line 1")
	}
	// Clearing an absent breakpoint is a no-op.
	e.SetBreakpoint(99, false)
	if got := e.Breakpoints(); len(got) != 0 {
		t.Errorf("Breakpoints() = %v, want empty", got)
	}
}

func TestCodeEditorBreakpointsSorted(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc\nd\ne\nf")

	// Insert out of order; Breakpoints() must return sorted unique lines.
	e.SetBreakpoint(4, true)
	e.SetBreakpoint(0, true)
	e.SetBreakpoint(2, true)
	e.SetBreakpoint(4, true) // duplicate enable

	got := e.Breakpoints()
	want := []int{0, 2, 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Breakpoints() = %v, want %v", got, want)
	}
}

func TestCodeEditorClearBreakpoints(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")

	e.SetBreakpoint(0, true)
	e.SetBreakpoint(2, true)
	if len(e.Breakpoints()) != 2 {
		t.Fatalf("setup: want 2 breakpoints, got %v", e.Breakpoints())
	}

	e.ClearBreakpoints()
	if got := e.Breakpoints(); len(got) != 0 {
		t.Errorf("after ClearBreakpoints, Breakpoints() = %v, want empty", got)
	}
}

// TestCodeEditorBreakpointGutterClick verifies that a left-click landing in the
// gutter (x < gutterW) toggles the breakpoint on the resolved line, while a
// click in the text area moves the cursor and leaves breakpoints untouched. The
// click Y is derived from the editor's own font metrics so it stays robust to
// font differences, using the same y -> line math as posFromXY.
func TestCodeEditorBreakpointGutterClick(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(400, 300)
	e.SetText("line0\nline1\nline2\nline3\nline4")

	lh := e.font.FontExtents().Height + 2
	topOff := e.topOffset()

	// yForLine returns a click Y near the vertical centre of the given line,
	// matching posFromXY's: line = (y - topOff + scrollY) / lh  (scrollY == 0).
	yForLine := func(line int) float64 {
		return topOff + float64(line)*lh + lh/2
	}

	// Sanity: the resolver maps our chosen Y back to the intended line.
	if got, _ := e.posFromXY(5, yForLine(3)); got != 3 {
		t.Fatalf("posFromXY resolved line %d, want 3 (font metrics mismatch)", got)
	}

	// Gutter click (x in [0, gutterW)) on line 3 toggles its breakpoint and must
	// NOT move the cursor.
	gutterX := e.gutterW / 2
	e.cursorLine = 0
	e.OnLeftDown(gutterX, yForLine(3))
	if !e.breakpoints[3] {
		t.Errorf("gutter click on line 3 should set a breakpoint")
	}
	if e.cursorLine != 0 {
		t.Errorf("gutter click moved cursor to line %d, want it to stay at 0", e.cursorLine)
	}

	// A second gutter click on the same line toggles it back off.
	e.OnLeftDown(gutterX, yForLine(3))
	if e.breakpoints[3] {
		t.Errorf("second gutter click on line 3 should clear the breakpoint")
	}

	// Text-area click (x > gutterW) must NOT toggle a breakpoint, and SHOULD move
	// the cursor to that line.
	textX := e.gutterW + 20
	e.OnLeftDown(textX, yForLine(2))
	if e.breakpoints[2] {
		t.Errorf("text-area click on line 2 should not set a breakpoint")
	}
	if e.cursorLine != 2 {
		t.Errorf("text-area click should move cursor to line 2, got %d", e.cursorLine)
	}
	if len(e.Breakpoints()) != 0 {
		t.Errorf("no breakpoints expected after toggle-off + text click, got %v", e.Breakpoints())
	}
}
