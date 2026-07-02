package gui

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Run-test gutter marker (pure, no GL / no Draw)
//
// A ▶ marker is drawn in the gutter beside every top-level Go test function when
// the edited file is a *_test.go. These tests cover the name predicate, the
// per-line detection, the pure hit-test slot, and the OnLeftDown click that
// fires SigTestRunRequested without toggling a breakpoint or moving the caret.
// filePath is set directly (not via SetFilePath) to keep the tests hermetic and
// avoid the git exec RefreshGitStatus would trigger.
// ---------------------------------------------------------------------------

// TestGutterFuncNameRule pins isGoTestFuncName to cmd/go's isTest rule: a
// recognized prefix (Test/Benchmark/Fuzz/Example) followed by nothing or a
// non-lowercase rune. Notably "Testfoo"/"Testing"/"TestfOO" are NOT tests (the
// rune right after "Test" is lowercase), while bare "Test", "Test_foo" and
// "Test1" ARE.
func TestGutterFuncNameRule(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"TestFoo", true},
		{"Test", true},     // bare prefix is a test func (cmd/go rule)
		{"Test_foo", true}, // '_' after prefix is not lowercase
		{"Test1", true},    // digit after prefix is not lowercase
		{"TestfOO", false}, // 'f' (lowercase) right after "Test"
		{"Testfoo", false}, // lowercase after "Test"
		{"Testing", false}, // "Test"+"ing": 'i' is lowercase
		{"BenchmarkX", true},
		{"Benchmark", true},
		{"FuzzX", true},
		{"Fuzz", true},
		{"ExampleX", true},
		{"Example", true},
		{"Foo", false},
		{"helper", false},
		{"TestingT", false}, // still lowercase after "Test"
		{"", false},
	}
	for _, c := range cases {
		if got := isGoTestFuncName(c.name); got != c.want {
			t.Errorf("isGoTestFuncName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestGutterFuncLines checks that testFuncLines returns 0-based line -> name for
// every top-level test func in a *_test.go file, excluding helpers and types,
// and returns nothing for a non-test file.
func TestGutterFuncLines(t *testing.T) {
	src := "package sample\n" + // 0
		"\n" + // 1
		"func TestAlpha(t *testing.T) {}\n" + // 2
		"\n" + // 3
		"func helper() {}\n" + // 4
		"\n" + // 5
		"func BenchmarkBeta(b *testing.B) {}\n" + // 6
		"\n" + // 7
		"type Thing struct{}\n" // 8

	e := NewCodeEditor()
	e.SetText(src)

	// Non-test file: no markers regardless of content.
	e.filePath = "sample.go"
	if got := e.testFuncLines(); len(got) != 0 {
		t.Errorf("non-_test.go should yield no test-func lines, got %v", got)
	}

	// _test.go file: only the test funcs, keyed by 0-based line index.
	e.filePath = "sample_test.go"
	got := e.testFuncLines()
	want := map[int]string{
		2: "TestAlpha",
		6: "BenchmarkBeta",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("testFuncLines() = %v, want %v", got, want)
	}
}

// TestGutterRunMarkerHitX exercises the pure horizontal hit-test slot.
func TestGutterRunMarkerHitX(t *testing.T) {
	// Inside the left-edge slot.
	for _, x := range []float64{0, testRunGutterX, testRunGutterX + testRunGutterSize} {
		if !testRunMarkerHitX(x) {
			t.Errorf("testRunMarkerHitX(%v) = false, want true (inside slot)", x)
		}
	}
	// Outside the slot: just past it, and the right/inner gutter (fold/diff zone).
	for _, x := range []float64{testRunGutterX + testRunGutterSize + 0.5, 40} {
		if testRunMarkerHitX(x) {
			t.Errorf("testRunMarkerHitX(%v) = true, want false (outside slot)", x)
		}
	}
}

// TestGutterRunMarkerClick verifies the region helper and the OnLeftDown click:
// a click on the ▶ marker fires SigTestRunRequested with the func name and
// consumes the event (no caret move, no breakpoint), while a gutter click on a
// non-test line falls through to the existing breakpoint toggle.
func TestGutterRunMarkerClick(t *testing.T) {
	src := "package p\n" + // 0
		"\n" + // 1
		"func TestAlpha(t *testing.T) {\n" + // 2
		"\tx := 1\n" + // 3
		"\t_ = x\n" + // 4
		"}\n" + // 5
		"\n" + // 6
		"func helper() {}\n" // 7

	e := NewCodeEditor()
	e.SetSize(400, 300)
	e.SetText(src)
	e.filePath = "p_test.go"

	// Region helper: left-edge x on the TestAlpha line (index 2) returns it.
	if name, ok := e.testRunMarkerAt(5, 2); !ok || name != "TestAlpha" {
		t.Fatalf("testRunMarkerAt(5, 2) = (%q, %v), want (\"TestAlpha\", true)", name, ok)
	}
	// The helper line (index 7) is not a marker.
	if _, ok := e.testRunMarkerAt(5, 7); ok {
		t.Errorf("helper line should not carry a run marker")
	}
	// Right/inner gutter x (fold strip) on the test line is outside the slot.
	if _, ok := e.testRunMarkerAt(40, 2); ok {
		t.Errorf("x=40 on the test line should be outside the run-marker slot")
	}

	var fired string
	e.SigTestRunRequested(func(name string) { fired = name })

	lh := e.font.FontExtents().Height + 2
	topOff := e.topOffset()
	yForLine := func(line int) float64 { return topOff + float64(line)*lh + lh/2 }

	// Sanity: our chosen Y resolves back to the TestAlpha line.
	if got, _ := e.posFromXY(5, yForLine(2)); got != 2 {
		t.Fatalf("posFromXY resolved line %d, want 2 (font metrics mismatch)", got)
	}

	// Click the ▶ marker on the test line: fires the callback, keeps the caret,
	// sets no breakpoint.
	e.cursorLine = 0
	e.OnLeftDown(testRunGutterX+2, yForLine(2))
	if fired != "TestAlpha" {
		t.Errorf("run-marker click should fire SigTestRunRequested(\"TestAlpha\"), got %q", fired)
	}
	if e.cursorLine != 0 {
		t.Errorf("run-marker click moved caret to line %d, want it to stay at 0", e.cursorLine)
	}
	if len(e.Breakpoints()) != 0 {
		t.Errorf("run-marker click should not toggle a breakpoint, got %v", e.Breakpoints())
	}

	// A gutter click on the helper line's left edge is not a marker, so it falls
	// through to the breakpoint toggle and does NOT fire the run callback.
	fired = ""
	e.OnLeftDown(testRunGutterX+2, yForLine(7))
	if fired != "" {
		t.Errorf("gutter click on a non-test line should not fire the run callback, got %q", fired)
	}
	if !e.breakpoints[7] {
		t.Errorf("gutter click on a non-marker line should fall through to the breakpoint toggle")
	}
}
