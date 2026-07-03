package ged

import (
	"github.com/uk0/silk/core"
	"testing"
)

// sampleBuild is a representative multi-line Go compiler dump: two
// errors with column info, one go-vet-style warning, a "file:line:"
// error with no column, and two lines that are not diagnostics at all
// (a banner and a bare summary). It exercises every branch of
// parseProblems in one fixture.
const sampleBuild = `# silk/ged
ged/foo.go:12:5: undefined: Bar
ged/baz.go:7:1: missing return at end of function
ged/foo.go:30: syntax error: unexpected }
ged/qux.go:9:2: warning: result of fmt.Sprintf call not used
Build finished with errors`

// TestParseProblemsStructs checks that parseProblems extracts the right
// File / Line / Col / Severity / Message for each recognised line and
// drops the two non-matching lines. This is the testable core the rest
// of the panel is built on, so it is asserted field-by-field.
func TestParseProblemsStructs(t *testing.T) {
	got := parseProblems(sampleBuild)

	want := []Problem{
		{File: "ged/foo.go", Line: 12, Col: 5, Severity: SeverityError, Message: "undefined: Bar"},
		{File: "ged/baz.go", Line: 7, Col: 1, Severity: SeverityError, Message: "missing return at end of function"},
		{File: "ged/foo.go", Line: 30, Col: 0, Severity: SeverityError, Message: "syntax error: unexpected }"},
		{File: "ged/qux.go", Line: 9, Col: 2, Severity: SeverityWarning, Message: "warning: result of fmt.Sprintf call not used"},
	}

	if len(got) != len(want) {
		t.Fatalf("parseProblems returned %d problems, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("problem[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestParseProblemsNoColumn verifies the "file:line:" form (no column)
// parses with Col == 0 while keeping the full message.
func TestParseProblemsNoColumn(t *testing.T) {
	got := parseProblems("main.go:42: cannot use x (type int) as type string")
	if len(got) != 1 {
		t.Fatalf("got %d problems, want 1: %+v", len(got), got)
	}
	p := got[0]
	if p.File != "main.go" || p.Line != 42 || p.Col != 0 {
		t.Errorf("locator = %s:%d:%d, want main.go:42:0", p.File, p.Line, p.Col)
	}
	if p.Message != "cannot use x (type int) as type string" {
		t.Errorf("message = %q", p.Message)
	}
	if p.Severity != SeverityError {
		t.Errorf("severity = %v, want error", p.Severity)
	}
}

// TestParseProblemsIgnoresNonMatching confirms that lines without a
// file:line locator (banners, blank lines, plain prose) produce no
// Problem rows.
func TestParseProblemsIgnoresNonMatching(t *testing.T) {
	in := "# silk/ged\n\nBuild finished\nsome random text without a locator\n"
	got := parseProblems(in)
	if len(got) != 0 {
		t.Fatalf("expected no problems, got %d: %+v", len(got), got)
	}
}

// TestParseProblemsSeverity checks the warning-vs-error classification
// rule (case-insensitive substring match on "warning").
func TestParseProblemsSeverity(t *testing.T) {
	cases := []struct {
		line string
		want Severity
	}{
		{"a.go:1:1: undefined: Foo", SeverityError},
		{"a.go:1:1: Warning: shadows declaration", SeverityWarning},
		{"a.go:1:1: this is a WARNING about something", SeverityWarning},
	}
	for _, c := range cases {
		got := parseProblems(c.line)
		if len(got) != 1 {
			t.Fatalf("%q: got %d problems", c.line, len(got))
		}
		if got[0].Severity != c.want {
			t.Errorf("%q: severity = %v, want %v", c.line, got[0].Severity, c.want)
		}
	}
}

// TestSetOutputPopulatesAndCounts drives the panel API: SetOutput feeds
// the parser, Problems() exposes the rows, and ErrorCount/WarningCount
// tally the fixture correctly (3 errors + 1 warning).
func TestProblemsSetOutputPopulatesAndCounts(t *testing.T) {
	p := NewProblemsPanel()
	p.SetOutput(sampleBuild)

	if n := len(p.Problems()); n != 4 {
		t.Fatalf("Problems() len = %d, want 4", n)
	}
	if ec := p.ErrorCount(); ec != 3 {
		t.Errorf("ErrorCount = %d, want 3", ec)
	}
	if wc := p.WarningCount(); wc != 1 {
		t.Errorf("WarningCount = %d, want 1", wc)
	}
}

// TestSetProblemsRoundTrip checks SetProblems / Problems round-trips a
// slice unchanged.
func TestSetProblemsRoundTrip(t *testing.T) {
	p := NewProblemsPanel()
	in := []Problem{
		{File: "z.go", Line: 1, Col: 1, Severity: SeverityError, Message: "boom"},
		{File: "a.go", Line: 2, Col: 0, Severity: SeverityWarning, Message: "meh"},
	}
	p.SetProblems(in)
	got := p.Problems()
	if len(got) != 2 || got[0] != in[0] || got[1] != in[1] {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

// TestSortByFileStable verifies SortByFile orders by file then line and
// is stable for equal (file,line) keys: the two "a.go:5" rows must keep
// their original first/second arrival order after sorting.
func TestProblemsSortByFileStable(t *testing.T) {
	p := NewProblemsPanel()
	p.SetProblems([]Problem{
		{File: "b.go", Line: 3, Message: "b3"},
		{File: "a.go", Line: 5, Message: "a5-first"},
		{File: "a.go", Line: 1, Message: "a1"},
		{File: "a.go", Line: 5, Message: "a5-second"},
	})
	p.SortByFile()

	got := p.Problems()
	wantMsgs := []string{"a1", "a5-first", "a5-second", "b3"}
	if len(got) != len(wantMsgs) {
		t.Fatalf("got %d rows, want %d", len(got), len(wantMsgs))
	}
	for i, m := range wantMsgs {
		if got[i].Message != m {
			t.Errorf("row[%d] message = %q, want %q (order: %+v)", i, got[i].Message, m, got)
		}
	}
}

// TestOnLeftDownFiresActivated confirms a click on a row invokes the
// activated callback with that row's (file, line, col). The header band
// occupies the top problemsHeaderH px and rows are rowHeight px tall, so
// a y just past the header lands on row 0.
func TestProblemsOnLeftDownFiresActivated(t *testing.T) {
	p := NewProblemsPanel()
	p.SetProblems([]Problem{
		{File: "first.go", Line: 10, Col: 2, Severity: SeverityError, Message: "e0"},
		{File: "second.go", Line: 20, Col: 4, Severity: SeverityWarning, Message: "w1"},
	})

	var gotFile string
	var gotLine, gotCol int
	fired := false
	p.SigProblemActivated(func(file string, line, col int) {
		fired = true
		gotFile, gotLine, gotCol = file, line, col
	})

	// Click row 1 (second.go): y = header + 1.5 rows.
	p.OnLeftDown(5, problemsHeaderH+1.5*p.rowHeight)

	if !fired {
		t.Fatal("activated callback did not fire")
	}
	if gotFile != "second.go" || gotLine != 20 || gotCol != 4 {
		t.Errorf("activated with (%s, %d, %d), want (second.go, 20, 4)", gotFile, gotLine, gotCol)
	}
}

// TestOnLeftDownHeaderIgnored confirms a click inside the header band
// (above the first row) does not fire the activated callback.
func TestProblemsOnLeftDownHeaderIgnored(t *testing.T) {
	p := NewProblemsPanel()
	p.SetProblems([]Problem{{File: "x.go", Line: 1, Message: "e"}})
	fired := false
	p.SigProblemActivated(func(string, int, int) { fired = true })
	p.OnLeftDown(5, problemsHeaderH/2)
	if fired {
		t.Error("callback fired for a header click")
	}
}

// TestProblemsPanelFactoryRegistered checks the factory id resolves to a
// constructible *ProblemsPanel, matching how silkide will instantiate it
// for docking.
func TestProblemsPanelFactoryRegistered(t *testing.T) {
	obj := core.New("ged.ProblemsPanel")
	if _, ok := obj.(*ProblemsPanel); !ok {
		t.Fatalf("factory ged.ProblemsPanel built %T, want *ProblemsPanel", obj)
	}
}
