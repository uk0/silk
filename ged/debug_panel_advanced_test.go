package ged

import (
	"reflect"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// These tests cover the context-aware half of the debugger inspector: the
// breakpoint table, the (goroutine, frame) selection model, the lazily
// expanded variables tree with its separate Arguments group, and the debug
// console. Like the sibling debug_panel_test.go they never call Draw, never
// build a Frame and never measure a font — every assertion runs against the
// model, the band geometry and the hit-tests, so the package stays headless.

// sampleBreakpoints is a small breakpoint table: one armed+bound with hits,
// one conditional, one disabled and unbound.
func sampleBreakpoints() []BreakpointRow {
	return []BreakpointRow{
		{File: "/proj/a.go", Line: 10, HitCount: 3, Enabled: true, Verified: true},
		{File: "/proj/b.go", Line: 20, Cond: "i > 2", Enabled: true, Verified: true},
		{File: "/proj/c.go", Line: 30, Enabled: false, Verified: false},
	}
}

// TestDebugBandsLayoutUnchangedWithoutOptionalBands pins the geometry the four
// original bands had before the breakpoint table and the console existed: with
// no breakpoints and a hidden console both optional bands take ZERO height, so
// a 300x400 panel still splits into stack [0,126), variables [126,218),
// goroutines [218,280), watch [280,400).
func TestDebugBandsLayoutUnchangedWithoutOptionalBands(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)

	b := p.bands()
	if b.bpTop != 0 || b.bpBottom != 0 {
		t.Errorf("breakpoint band = [%v,%v), want collapsed [0,0)", b.bpTop, b.bpBottom)
	}
	if b.consoleTop != 400 || b.consoleBottom != 400 {
		t.Errorf("console band = [%v,%v), want collapsed [400,400)", b.consoleTop, b.consoleBottom)
	}
	cases := []struct {
		name      string
		got, want float64
	}{
		{"stackTop", b.stackTop, 0},
		{"stackBottom", b.stackBottom, 126},
		{"varTop", b.varTop, 126},
		{"varBottom", b.varBottom, 218},
		{"goroTop", b.goroTop, 218},
		{"goroBottom", b.goroBottom, 280},
		{"watchTop", b.watchTop, 280},
		{"watchBottom", b.watchBottom, 400},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestDebugBreakpointBandGeometryAndHitTest checks the table sizes itself to
// its rows (header + 3 rows = 82px at the top of the widget), pushes the call
// stack down by exactly that much, and maps y coordinates to row indices.
func TestDebugBreakpointBandGeometryAndHitTest(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetBreakpoints(sampleBreakpoints())

	b := p.bands()
	if want := debugHeaderH + 3*p.rowHeight; b.bpBottom != want {
		t.Fatalf("bpBottom = %v, want %v (header + 3 rows)", b.bpBottom, want)
	}
	if b.stackTop != b.bpBottom {
		t.Errorf("stackTop = %v, want %v (right under the table)", b.stackTop, b.bpBottom)
	}

	cases := []struct {
		name string
		y    float64
		want int
	}{
		{"header", 10, -1},
		{"row 0", 32, 0},
		{"row 1", 52, 1},
		{"row 2", 72, 2},
		{"past last row", 82, -1},
		{"stack band", 100, -1},
	}
	for _, c := range cases {
		if got := p.breakpointRowAt(c.y); got != c.want {
			t.Errorf("%s: breakpointRowAt(%v) = %d, want %d", c.name, c.y, got, c.want)
		}
	}

	// With an empty table the band collapses and every y misses it.
	p.SetBreakpoints(nil)
	if got := p.breakpointRowAt(32); got != -1 {
		t.Errorf("breakpointRowAt on an empty table = %d, want -1", got)
	}
}

// TestDebugSetBreakpointsRoundTrip verifies SetBreakpoints stores the rows and
// Breakpoints() returns an equal — but independent — copy in both directions.
func TestDebugSetBreakpointsRoundTrip(t *testing.T) {
	p := NewDebugPanel()
	in := sampleBreakpoints()
	p.SetBreakpoints(in)

	got := p.Breakpoints()
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("Breakpoints() = %+v\nwant %+v", got, in)
	}
	got[0].Cond = "MUTATED"
	if p.Breakpoints()[0].Cond != "" {
		t.Error("Breakpoints() returned an aliasing slice, not a copy")
	}
	in[1].Cond = "LEAK"
	if p.Breakpoints()[1].Cond != "i > 2" {
		t.Error("SetBreakpoints aliased the caller's slice instead of copying")
	}
}

// TestDebugBreakpointToggleClick drives the enabled marker hot-zone on the
// left of a row: the click fires SigToggleBreakpoint with the row's location
// and the REQUESTED new state, and flips the row locally so the checkbox
// answers immediately.
func TestDebugBreakpointToggleClick(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetBreakpoints(sampleBreakpoints())

	var (
		gotFile string
		gotLine int
		gotOn   bool
		fires   int
	)
	p.SigToggleBreakpoint(func(file string, line int, enabled bool) {
		gotFile, gotLine, gotOn = file, line, enabled
		fires++
	})

	// Row 0 is armed: the click asks for disabled.
	p.OnLeftDown(5, 32)
	if fires != 1 {
		t.Fatalf("marker click fired SigToggleBreakpoint %d times, want 1", fires)
	}
	if gotFile != "/proj/a.go" || gotLine != 10 || gotOn {
		t.Errorf("SigToggleBreakpoint = (%q,%d,%v), want (\"/proj/a.go\",10,false)", gotFile, gotLine, gotOn)
	}
	if p.Breakpoints()[0].Enabled {
		t.Error("row 0 still Enabled after the toggle click, want flipped locally")
	}

	// Row 2 is disabled: the click asks for enabled.
	p.OnLeftDown(5, 72)
	if fires != 2 {
		t.Fatalf("second marker click fired %d times total, want 2", fires)
	}
	if gotFile != "/proj/c.go" || gotLine != 30 || !gotOn {
		t.Errorf("SigToggleBreakpoint = (%q,%d,%v), want (\"/proj/c.go\",30,true)", gotFile, gotLine, gotOn)
	}
	if !p.Breakpoints()[2].Enabled {
		t.Error("row 2 still disabled after the toggle click, want flipped locally")
	}

	// A click in the middle of a row is the condition-edit gesture, not a
	// toggle: it must not fire.
	p.OnLeftDown(120, 32)
	if fires != 2 {
		t.Errorf("mid-row click fired SigToggleBreakpoint (%d fires), want 2", fires)
	}
}

// TestDebugBreakpointDeleteClick drives the ✕ hot-zone on the right of a row:
// it fires SigDeleteBreakpoint with the location and drops the row.
func TestDebugBreakpointDeleteClick(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetBreakpoints(sampleBreakpoints())

	var (
		gotFile string
		gotLine int
		fires   int
	)
	p.SigDeleteBreakpoint(func(file string, line int) {
		gotFile, gotLine = file, line
		fires++
	})

	// A click on the row body must not delete.
	p.OnLeftDown(120, 52)
	if fires != 0 {
		t.Fatalf("row-body click fired SigDeleteBreakpoint, want no fire")
	}

	// x past (width - debugBPDeleteW) = 280 deletes row 1.
	p.OnLeftDown(290, 52)
	if fires != 1 {
		t.Fatalf("✕ click fired SigDeleteBreakpoint %d times, want 1", fires)
	}
	if gotFile != "/proj/b.go" || gotLine != 20 {
		t.Errorf("SigDeleteBreakpoint = (%q,%d), want (\"/proj/b.go\",20)", gotFile, gotLine)
	}
	left := p.Breakpoints()
	if len(left) != 2 || left[0].File != "/proj/a.go" || left[1].File != "/proj/c.go" {
		t.Errorf("after delete, rows = %+v, want a.go + c.go", left)
	}
}

// TestDebugBreakpointConditionEdit drives the inline condition editor: a quick
// second click on a row's middle opens it seeded with the current condition,
// Enter fires SigEditCondition(file,line,cond), and an EMPTY submit still
// fires (that is how a condition is cleared). Esc cancels without firing.
func TestDebugBreakpointConditionEdit(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetBreakpoints(sampleBreakpoints())

	var (
		gotFile string
		gotLine int
		gotCond string
		fires   int
	)
	p.SigEditCondition(func(file string, line int, cond string) {
		gotFile, gotLine, gotCond = file, line, cond
		fires++
	})

	// Single click only arms the double-click.
	p.OnLeftDown(120, 52)
	if p.editingCond != -1 {
		t.Fatalf("single click opened the condition editor (editingCond=%d)", p.editingCond)
	}
	// Quick second click on the same row opens the editor, seeded.
	p.OnLeftDown(120, 52)
	if p.editingCond != 1 {
		t.Fatalf("double click did not open the condition editor: editingCond=%d, want 1", p.editingCond)
	}
	if p.condInput != "i > 2" {
		t.Fatalf("condInput = %q, want seeded %q", p.condInput, "i > 2")
	}

	// Edit it and submit.
	p.OnKeyDown(gui.KeyBackSpace, false) // "i > 2" -> "i > "
	p.OnTextInput("5")                   // -> "i > 5"
	p.OnKeyDown(gui.KeyEnter, false)
	if fires != 1 {
		t.Fatalf("Enter fired SigEditCondition %d times, want 1", fires)
	}
	if gotFile != "/proj/b.go" || gotLine != 20 || gotCond != "i > 5" {
		t.Errorf("SigEditCondition = (%q,%d,%q), want (\"/proj/b.go\",20,\"i > 5\")", gotFile, gotLine, gotCond)
	}
	if p.editingCond != -1 || p.condInput != "" {
		t.Errorf("after submit: editingCond=%d condInput=%q, want -1/\"\"", p.editingCond, p.condInput)
	}
	// Host-driven: the row keeps its old condition until the host re-pushes.
	if p.Breakpoints()[1].Cond != "i > 2" {
		t.Errorf("condition applied locally (%q), want unchanged (host-driven)", p.Breakpoints()[1].Cond)
	}

	// An empty submit clears the condition — it MUST still fire.
	p.beginEditCond(1)
	p.OnKeyDown(gui.KeyBackSpace, false)
	p.OnKeyDown(gui.KeyBackSpace, false)
	p.OnKeyDown(gui.KeyBackSpace, false)
	p.OnKeyDown(gui.KeyBackSpace, false)
	p.OnKeyDown(gui.KeyBackSpace, false)
	if p.condInput != "" {
		t.Fatalf("condInput = %q, want emptied", p.condInput)
	}
	p.OnKeyDown(gui.KeyEnter, false)
	if fires != 2 || gotCond != "" {
		t.Errorf("empty submit: fires=%d cond=%q, want 2/\"\" (clear the condition)", fires, gotCond)
	}

	// Esc cancels without firing.
	p.beginEditCond(0)
	p.OnTextInput("x == 1")
	p.OnKeyDown(gui.KeyEsc, false)
	if fires != 2 {
		t.Errorf("Esc fired SigEditCondition (fires=%d), want 2", fires)
	}
	if p.editingCond != -1 || p.condInput != "" {
		t.Errorf("after Esc: editingCond=%d condInput=%q, want -1/\"\"", p.editingCond, p.condInput)
	}
}

// TestDebugContextSelectionSignals drives the explicit (goroutine, frame)
// selection model: each real change fires SigContextChanged once, an unchanged
// or out-of-range set is a no-op, and the accessors report the live context.
func TestDebugContextSelectionSignals(t *testing.T) {
	p := NewDebugPanel()
	p.SetCallStack(sampleFrames())

	if p.SelectedGoroutine() != -1 {
		t.Fatalf("SelectedGoroutine() = %d, want -1 (dlv's current goroutine)", p.SelectedGoroutine())
	}

	var (
		gotGID   int64
		gotFrame int
		fires    int
	)
	p.SigContextChanged(func(gid int64, frame int) {
		gotGID, gotFrame = gid, frame
		fires++
	})

	p.SetSelectedGoroutine(42)
	if fires != 1 || gotGID != 42 || gotFrame != 0 {
		t.Fatalf("SetSelectedGoroutine(42): fires=%d ctx=(%d,%d), want 1/(42,0)", fires, gotGID, gotFrame)
	}
	if p.SelectedGoroutine() != 42 {
		t.Errorf("SelectedGoroutine() = %d, want 42", p.SelectedGoroutine())
	}

	p.SetSelectedFrame(2)
	if fires != 2 || gotGID != 42 || gotFrame != 2 {
		t.Fatalf("SetSelectedFrame(2): fires=%d ctx=(%d,%d), want 2/(42,2)", fires, gotGID, gotFrame)
	}
	if p.SelectedFrame() != 2 {
		t.Errorf("SelectedFrame() = %d, want 2", p.SelectedFrame())
	}

	// Re-setting the same context is a no-op: the host must not re-evaluate.
	p.SetSelectedFrame(2)
	p.SetSelectedGoroutine(42)
	if fires != 3 {
		// SetSelectedGoroutine(42) resets the frame to 0, which IS a change.
		t.Fatalf("after re-sets: fires=%d, want 3 (frame reset to 0 only)", fires)
	}
	if gotFrame != 0 || p.SelectedFrame() != 0 {
		t.Errorf("re-selecting the goroutine left frame %d/%d, want 0 (top frame)", gotFrame, p.SelectedFrame())
	}

	// Out-of-range frames are ignored.
	p.SetSelectedFrame(9)
	p.SetSelectedFrame(-1)
	if fires != 3 {
		t.Errorf("out-of-range SetSelectedFrame fired (fires=%d), want 3", fires)
	}
	if p.SelectedFrame() != 0 {
		t.Errorf("SelectedFrame() = %d after invalid sets, want 0", p.SelectedFrame())
	}
}

// TestDebugGoroutineClickSetsContext verifies a goroutine row click switches
// the evaluation scope to that goroutine's top frame (SigContextChanged) while
// still reporting the activation the host opens the file:line from.
//
// Geometry (300x400, no breakpoints/console): goroutine rows start at y=240.
func TestDebugGoroutineClickSetsContext(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetCallStack(sampleFrames())
	gs := sampleGoroutines()
	p.SetGoroutines(gs)
	p.SetSelectedFrame(2) // deep in the current goroutine's stack

	var (
		gotGID     int64
		gotFrame   = -1
		ctxFires   int
		activated  core.Goroutine
		actedFires int
	)
	p.SigContextChanged(func(gid int64, frame int) {
		gotGID, gotFrame = gid, frame
		ctxFires++
	})
	p.SigGoroutineActivated(func(g core.Goroutine) {
		activated = g
		actedFires++
	})

	p.OnLeftDown(10, 270) // goroutine row 1 (#18)

	if ctxFires != 1 {
		t.Fatalf("goroutine click fired SigContextChanged %d times, want 1", ctxFires)
	}
	if gotGID != 18 || gotFrame != 0 {
		t.Errorf("context = (%d,%d), want (18,0) — new goroutine, top frame", gotGID, gotFrame)
	}
	if p.SelectedGoroutine() != 18 || p.SelectedFrame() != 0 {
		t.Errorf("panel context = (%d,%d), want (18,0)", p.SelectedGoroutine(), p.SelectedFrame())
	}
	if actedFires != 1 || !reflect.DeepEqual(activated, gs[1]) {
		t.Errorf("SigGoroutineActivated fires=%d g=%+v, want 1/%+v", actedFires, activated, gs[1])
	}
}

// TestDebugStackClickFiresContextChanged verifies a stack-row click reports the
// new scope through SigContextChanged (keeping the selected goroutine) as well
// as through the legacy SigFrameSelected, and that re-clicking the same row
// re-fires only the latter.
//
// Geometry (300x400): stack row 2 covers [62,82); click the middle.
func TestDebugStackClickFiresContextChanged(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetCallStack(sampleFrames())
	p.SetSelectedGoroutine(7)

	var (
		gotGID    int64
		gotFrame  = -1
		ctxFires  int
		selFires  int
		lastIndex = -1
	)
	p.SigContextChanged(func(gid int64, frame int) {
		gotGID, gotFrame = gid, frame
		ctxFires++
	})
	p.SigFrameSelected(func(index int, _ core.StackFrame) {
		lastIndex = index
		selFires++
	})

	p.OnLeftDown(5, 72)
	if ctxFires != 1 || gotGID != 7 || gotFrame != 2 {
		t.Fatalf("stack click: ctxFires=%d ctx=(%d,%d), want 1/(7,2)", ctxFires, gotGID, gotFrame)
	}
	if selFires != 1 || lastIndex != 2 {
		t.Fatalf("SigFrameSelected fires=%d index=%d, want 1/2", selFires, lastIndex)
	}

	// Re-selecting the same row re-asks for its locals but does not pretend the
	// scope moved. (Driven directly: a second click this soon on the same row
	// is the double-click gesture, which activates instead of selecting.)
	p.selectFrame(2)
	if ctxFires != 1 {
		t.Errorf("re-click fired SigContextChanged (%d), want 1", ctxFires)
	}
	if selFires != 2 {
		t.Errorf("re-click SigFrameSelected fires=%d, want 2", selFires)
	}

	// Keyboard moves are context moves too.
	p.OnKeyDown(gui.KeyUp, false)
	if ctxFires != 2 || gotFrame != 1 {
		t.Errorf("Up: ctxFires=%d frame=%d, want 2/1", ctxFires, gotFrame)
	}
}

// TestDebugVarPath covers the pure path builder: a root row is its own name,
// a field is dotted, and an index-like child name appends directly so the
// result stays a valid Go expression dlv can evaluate.
func TestDebugVarPath(t *testing.T) {
	cases := []struct {
		parent, name, want string
	}{
		{"", "i", "i"},
		{"p", "Next", "p.Next"},
		{"p.Next", "Val", "p.Next.Val"},
		{"s", "[0]", "s[0]"},
		{"m", "[\"k\"]", "m[\"k\"]"},
		{"s[0]", "A", "s[0].A"},
	}
	for _, c := range cases {
		if got := varPath(c.parent, c.name); got != c.want {
			t.Errorf("varPath(%q,%q) = %q, want %q", c.parent, c.name, got, c.want)
		}
	}
}

// TestDebugVariableLazyExpand drives the tree: clicking a composite's twisty
// asks the host for its children (SigExpandVar with the dlv-evaluable path),
// SetChildren renders them under the parent, collapsing hides them again, and
// re-expanding a loaded node does NOT ask twice.
//
// Geometry (300x400): the variables band is [126,218) with rows from y=148, so
// row 0 covers [148,168) and row 1 [168,188). The twisty of a depth-0 row is
// the 12px zone at x=8.
func TestDebugVariableLazyExpand(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetLocals([]VarRow{
		{Name: "p", Type: "*main.T", Value: "0xc000010000", HasChildren: true},
		{Name: "i", Type: "int", Value: "7"},
	})

	var (
		asked []string
	)
	p.SigExpandVar(func(path string) { asked = append(asked, path) })

	// Click the twisty of row 0.
	p.OnLeftDown(12, 158)
	if !reflect.DeepEqual(asked, []string{"p"}) {
		t.Fatalf("SigExpandVar asked for %v, want [p]", asked)
	}
	// Nothing renders yet — the children have not arrived.
	if got := len(p.visRows()); got != 2 {
		t.Fatalf("visible rows = %d before SetChildren, want 2", got)
	}

	// The host answers.
	p.SetChildren("p", []VarRow{
		{Name: "A", Type: "int", Value: "1"},
		{Name: "B", Type: "string", Value: "x"},
	})

	rows := p.visRows()
	gotPaths := make([]string, len(rows))
	for i, r := range rows {
		gotPaths[i] = r.path
	}
	if want := []string{"p", "p.A", "p.B", "i"}; !reflect.DeepEqual(gotPaths, want) {
		t.Fatalf("visible paths = %v, want %v", gotPaths, want)
	}
	if rows[1].depth != 1 || rows[3].depth != 0 {
		t.Errorf("depths = child %d / sibling %d, want 1 / 0", rows[1].depth, rows[3].depth)
	}
	if !rows[0].expanded {
		t.Error("parent row is not marked expanded after SetChildren")
	}
	// The child row is now what a click at y=178 hits.
	if got := p.varRowAt(178); got != 1 {
		t.Errorf("varRowAt(178) = %d, want 1 (the first child)", got)
	}

	// Collapsing hides the children again, without asking anything.
	p.OnLeftDown(12, 158)
	if got := len(p.visRows()); got != 2 {
		t.Errorf("visible rows = %d after collapse, want 2", got)
	}
	if len(asked) != 1 {
		t.Errorf("collapse asked the host again: %v", asked)
	}

	// Re-expanding uses the cache: still exactly one request for "p".
	p.OnLeftDown(12, 158)
	if got := len(p.visRows()); got != 4 {
		t.Errorf("visible rows = %d after re-expand, want 4", got)
	}
	if len(asked) != 1 {
		t.Errorf("re-expand asked the host again: %v, want one request total", asked)
	}

	// A click outside the variables band never asks for children.
	p.OnLeftDown(12, 228) // the goroutines band's header
	if len(asked) != 1 {
		t.Errorf("click outside the tree asked for children: %v", asked)
	}
}

// TestDebugNestedVariableEditUsesPath verifies the inline value editor on a
// nested row reports the row's dlv-evaluable PATH, so the host can apply it
// with SetVariable("p.A", …) rather than guessing from a bare field name.
func TestDebugNestedVariableEditUsesPath(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetLocals([]VarRow{{Name: "p", Type: "*main.T", Value: "0xc000010000", HasChildren: true}})
	p.SetChildren("p", []VarRow{{Name: "A", Type: "int", Value: "1"}})

	var (
		gotName string
		gotVal  string
		fired   bool
	)
	p.SigVariableEdited(func(name, newValue string) {
		gotName, gotVal, fired = name, newValue, true
	})

	// Row 1 is "p.A", at y in [168,188). Two quick clicks off the twisty open
	// the value editor.
	p.OnLeftDown(120, 178)
	p.OnLeftDown(120, 178)
	if p.editingVar != 1 {
		t.Fatalf("editingVar = %d, want 1 (the child row)", p.editingVar)
	}
	if p.varInput != "1" {
		t.Fatalf("varInput = %q, want seeded %q", p.varInput, "1")
	}
	p.OnKeyDown(gui.KeyBackSpace, false)
	p.OnTextInput("9")
	p.OnKeyDown(gui.KeyEnter, false)

	if !fired {
		t.Fatal("Enter did not fire SigVariableEdited for the nested row")
	}
	if gotName != "p.A" || gotVal != "9" {
		t.Errorf("SigVariableEdited = (%q,%q), want (\"p.A\",\"9\")", gotName, gotVal)
	}
}

// TestDebugArgumentsGroupSeparateFromLocals verifies the Arguments group is its
// own list with its own header row, kept out of the Locals accessors, and that
// its rows are editable like locals while the header rows are inert.
func TestDebugArgumentsGroupSeparateFromLocals(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetArguments([]VarRow{{Name: "ctx", Type: "context.Context", Value: "0x1"}})
	p.SetLocals([]VarRow{
		{Name: "i", Type: "int", Value: "42"},
		{Name: "s", Type: "string", Value: "hello"},
	})

	if got := p.Arguments(); len(got) != 1 || got[0].Name != "ctx" {
		t.Fatalf("Arguments() = %+v, want [ctx]", got)
	}
	if got := p.Locals(); len(got) != 2 || got[0].Name != "i" {
		t.Fatalf("Locals() = %+v, want [i s]", got)
	}
	// The legacy flat projection stays locals-only: an argument is not a local.
	vs := p.Variables()
	if len(vs) != 2 || vs[0].Name != "i" || vs[1].Name != "s" {
		t.Errorf("Variables() = %+v, want the two locals only", vs)
	}

	// Defensive copies both ways.
	args := p.Arguments()
	args[0].Value = "MUTATED"
	if p.Arguments()[0].Value != "0x1" {
		t.Error("Arguments() returned an aliasing slice, not a copy")
	}

	// Display order: Arguments header, the argument, Locals header, the locals.
	rows := p.visRows()
	if len(rows) != 5 {
		t.Fatalf("visible rows = %d, want 5 (2 headers + 3 variables)", len(rows))
	}
	if !rows[0].isHeader() || !strings.Contains(rows[0].label, "Arguments") {
		t.Errorf("row 0 = %+v, want the Arguments header", rows[0])
	}
	if rows[1].isHeader() || rows[1].path != "ctx" {
		t.Errorf("row 1 = %+v, want the ctx argument", rows[1])
	}
	if !rows[2].isHeader() || !strings.Contains(rows[2].label, "Locals") {
		t.Errorf("row 2 = %+v, want the Locals header", rows[2])
	}
	if rows[3].path != "i" || rows[4].path != "s" {
		t.Errorf("rows 3/4 = %q/%q, want i/s", rows[3].path, rows[4].path)
	}

	// A header row is inert: two quick clicks on it open no editor.
	p.OnLeftDown(120, 158) // row 0, the Arguments header
	p.OnLeftDown(120, 158)
	if p.editingVar != -1 {
		t.Errorf("double click on a group header opened an editor (editingVar=%d)", p.editingVar)
	}

	// The argument row itself edits like a local, reporting its own path.
	var gotName, gotVal string
	p.SigVariableEdited(func(name, newValue string) { gotName, gotVal = name, newValue })
	p.OnLeftDown(120, 178) // row 1, the ctx argument
	p.OnLeftDown(120, 178)
	if p.editingVar != 1 {
		t.Fatalf("editingVar = %d, want 1 (the argument row)", p.editingVar)
	}
	p.OnTextInput("2")
	p.OnKeyDown(gui.KeyEnter, false)
	if gotName != "ctx" || gotVal != "0x12" {
		t.Errorf("SigVariableEdited = (%q,%q), want (\"ctx\",\"0x12\")", gotName, gotVal)
	}

	// Dropping the arguments drops their header too: the locals go back to
	// being flush rows.
	p.SetArguments(nil)
	rows = p.visRows()
	if len(rows) != 2 || rows[0].isHeader() || rows[0].path != "i" {
		t.Errorf("after SetArguments(nil), rows = %+v, want the two flush locals", rows)
	}
}

// TestDebugConsoleEval drives the console prompt: it is only reachable once the
// band is shown, clicking it takes focus, and Enter fires SigEval with the
// typed expression while a blank submit is ignored.
//
// Geometry (300x400, console shown): the console band is the bottom 100px and
// its prompt is the last row, [380,400).
func TestDebugConsoleEval(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)

	// Hidden by default: the band takes no space and the prompt cannot be hit.
	if p.ConsoleVisible() {
		t.Fatal("ConsoleVisible() = true on a fresh panel, want false")
	}
	if p.consoleInputAt(385) {
		t.Fatal("consoleInputAt hit the prompt while the console was hidden")
	}

	p.SetConsoleVisible(true)
	b := p.bands()
	if b.consoleBottom-b.consoleTop <= 0 {
		t.Fatalf("console band = [%v,%v), want a visible band", b.consoleTop, b.consoleBottom)
	}
	if b.watchBottom != b.consoleTop {
		t.Errorf("watchBottom = %v, consoleTop = %v, want the bands to meet", b.watchBottom, b.consoleTop)
	}

	cases := []struct {
		name string
		y    float64
		want bool
	}{
		{"prompt line", 385, true},
		{"transcript area", 330, false},
		{"watch band", 250, false},
	}
	for _, c := range cases {
		if got := p.consoleInputAt(c.y); got != c.want {
			t.Errorf("%s: consoleInputAt(%v) = %v, want %v", c.name, c.y, got, c.want)
		}
	}

	var (
		got   string
		fires int
	)
	p.SigEval(func(expr string) { got = expr; fires++ })

	// Typing while unfocused goes nowhere.
	p.OnTextInput("ignored")
	if p.consoleInput != "" {
		t.Fatalf("unfocused OnTextInput edited the prompt: %q", p.consoleInput)
	}

	p.OnLeftDown(10, 385)
	if !p.consoleFocused {
		t.Fatal("clicking the prompt did not focus the console input")
	}
	p.OnTextInput("len(")
	p.OnTextInput("xs)")
	if p.consoleInput != "len(xs)" {
		t.Fatalf("consoleInput = %q, want %q", p.consoleInput, "len(xs)")
	}
	p.OnKeyDown(gui.KeyEnter, false)
	if fires != 1 || got != "len(xs)" {
		t.Fatalf("Enter fired SigEval %d times with %q, want 1/%q", fires, got, "len(xs)")
	}
	if p.consoleInput != "" {
		t.Errorf("after submit, consoleInput = %q, want cleared", p.consoleInput)
	}

	// A blank submit is ignored.
	p.OnTextInput("   ")
	p.OnKeyDown(gui.KeyEnter, false)
	if fires != 1 {
		t.Errorf("blank submit fired SigEval (%d fires), want 1", fires)
	}

	// Backspace edits, Esc unfocuses.
	p.OnTextInput("ab")
	p.OnKeyDown(gui.KeyBackSpace, false)
	if p.consoleInput != "a" {
		t.Errorf("after Backspace, consoleInput = %q, want %q", p.consoleInput, "a")
	}
	p.OnKeyDown(gui.KeyEsc, false)
	if p.consoleFocused {
		t.Error("Esc did not unfocus the console prompt")
	}

	// Clicking the watch input moves focus out of the console.
	p.focusConsoleInput(true)
	p.OnLeftDown(10, b.watchTop+debugHeaderH+5)
	if p.consoleFocused {
		t.Error("clicking the watch input left the console focused")
	}
	if !p.watchFocused {
		t.Error("clicking the watch input did not focus it")
	}
}

// TestDebugConsoleLines verifies the transcript round-trips as a copy and that
// pushing output shows the console band on its own.
func TestDebugConsoleLines(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)

	in := []string{"> len(xs)", "3"}
	p.SetConsoleLines(in)

	if !p.ConsoleVisible() {
		t.Error("SetConsoleLines did not show the console band")
	}
	got := p.ConsoleLines()
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("ConsoleLines() = %v, want %v", got, in)
	}
	got[0] = "MUTATED"
	if p.ConsoleLines()[0] != "> len(xs)" {
		t.Error("ConsoleLines() returned an aliasing slice, not a copy")
	}
	in[1] = "LEAK"
	if p.ConsoleLines()[1] != "3" {
		t.Error("SetConsoleLines aliased the caller's slice instead of copying")
	}

	// Hiding it collapses the band again.
	p.SetConsoleVisible(false)
	b := p.bands()
	if b.consoleBottom != b.consoleTop {
		t.Errorf("console band = [%v,%v) while hidden, want collapsed", b.consoleTop, b.consoleBottom)
	}
	if b.watchBottom != 400 {
		t.Errorf("watchBottom = %v with the console hidden, want 400", b.watchBottom)
	}
}

// TestDebugClearKeepsBreakpointsAndConsole pins the scoping rules: a stop-scoped
// Clear drops the frames, both variable groups and the loaded children and
// resets the evaluation context to (-1,0) WITHOUT firing SigContextChanged,
// while the breakpoint table and the console transcript survive. ClearAll then
// drops the transcript but still keeps the breakpoints.
func TestDebugClearKeepsBreakpointsAndConsole(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	p.SetBreakpoints(sampleBreakpoints())
	p.SetCallStack(sampleFrames())
	p.SetArguments([]VarRow{{Name: "ctx", Type: "context.Context", Value: "0x1"}})
	p.SetLocals([]VarRow{{Name: "p", Type: "*main.T", Value: "0x2", HasChildren: true}})
	p.SetChildren("p", []VarRow{{Name: "A", Type: "int", Value: "1"}})
	p.SetConsoleLines([]string{"> 1+1", "2"})
	p.SetSelectedGoroutine(18)

	fired := false
	p.SigContextChanged(func(int64, int) { fired = true })

	p.Clear()

	if fired {
		t.Error("Clear fired SigContextChanged, want a silent host-driven reset")
	}
	if p.SelectedGoroutine() != -1 || p.SelectedFrame() != 0 {
		t.Errorf("after Clear, context = (%d,%d), want (-1,0)", p.SelectedGoroutine(), p.SelectedFrame())
	}
	if got := len(p.Arguments()); got != 0 {
		t.Errorf("after Clear, Arguments() len = %d, want 0", got)
	}
	if got := len(p.Locals()); got != 0 {
		t.Errorf("after Clear, Locals() len = %d, want 0", got)
	}
	if got := len(p.visRows()); got != 0 {
		t.Errorf("after Clear, visible rows = %d, want 0 (children dropped too)", got)
	}
	if got := len(p.Breakpoints()); got != 3 {
		t.Errorf("after Clear, Breakpoints() len = %d, want 3 (not stop-scoped)", got)
	}
	if got := len(p.ConsoleLines()); got != 2 {
		t.Errorf("after Clear, ConsoleLines() len = %d, want 2 (transcript kept)", got)
	}

	p.ClearAll()
	if got := len(p.ConsoleLines()); got != 0 {
		t.Errorf("after ClearAll, ConsoleLines() len = %d, want 0", got)
	}
	if got := len(p.Breakpoints()); got != 3 {
		t.Errorf("after ClearAll, Breakpoints() len = %d, want 3 (breakpoints outlive a session)", got)
	}
}

// TestDebugBreakpointWheelIsolation verifies the wheel scrolls the breakpoint
// table when the cursor is over it and leaves every other band alone.
func TestDebugBreakpointWheelIsolation(t *testing.T) {
	p := NewDebugPanel()
	p.SetSize(300, 400)
	rows := make([]BreakpointRow, 40)
	for i := range rows {
		rows[i] = BreakpointRow{File: "/proj/a.go", Line: i + 1, Enabled: true, Verified: true}
	}
	p.SetBreakpoints(rows)
	p.SetCallStack(sampleFrames())

	p.OnMouseWheel(150, 30, -1) // cursor inside the table
	if p.bpScrollY <= 0 {
		t.Errorf("bpScrollY = %v, want > 0 after a wheel in the table", p.bpScrollY)
	}
	if p.stackScrollY != 0 || p.varScrollY != 0 || p.goroScrollY != 0 || p.watchScrollY != 0 {
		t.Errorf("wheel in the table moved other bands: stack=%v var=%v goro=%v watch=%v",
			p.stackScrollY, p.varScrollY, p.goroScrollY, p.watchScrollY)
	}
}
