package ged

import (
	"github.com/uk0/silk/core"
	"strings"
	"testing"
)

// sampleGoTestOutput is a representative `go test -v` capture that
// exercises every branch the parser cares about: a passing test, a
// failing test with a "file:line:" locator embedded in its output, a
// skipped test, a passing subtest under TestX, and the package summary
// line at the bottom.
const sampleGoTestOutput = `=== RUN   TestParseFoo
--- PASS: TestParseFoo (0.02s)
=== RUN   TestParseBar
--- FAIL: TestParseBar (0.05s)
    parse_test.go:42: want 7, got 11
    parse_test.go:43: trailing detail line
=== RUN   TestParseQux
--- SKIP: TestParseQux (0.00s)
    parse_test.go:60: needs network
=== RUN   TestParent
=== RUN   TestParent/sub
--- PASS: TestParent/sub (0.01s)
--- PASS: TestParent (0.01s)
FAIL
FAIL    silk/ged    0.123s`

// TestParseTestOutputFields asserts every TestResult field for the
// fixture: name, package backfill, status, duration, and — for the
// failing row — the captured Output, File, and Line.
func TestParseTestOutputFields(t *testing.T) {
	got := parseTestOutput(sampleGoTestOutput)

	// Five rows: PASS TestParseFoo, FAIL TestParseBar, SKIP TestParseQux,
	// PASS TestParent/sub, PASS TestParent. (Parent is reported after
	// its subtest in -v output.)
	if len(got) != 5 {
		t.Fatalf("parseTestOutput returned %d rows, want 5: %+v", len(got), got)
	}

	// Row 0 — TestParseFoo, passing.
	if got[0].Name != "TestParseFoo" || got[0].Status != TestPassed || got[0].Duration != "0.02s" {
		t.Errorf("row 0 = %+v, want TestParseFoo PASS 0.02s", got[0])
	}
	if got[0].Package != "silk/ged" {
		t.Errorf("row 0 package = %q, want %q (backfilled from summary)", got[0].Package, "silk/ged")
	}
	if got[0].File != "" || got[0].Line != 0 || got[0].Output != "" {
		t.Errorf("row 0 should have no File/Line/Output: %+v", got[0])
	}

	// Row 1 — TestParseBar, failing, with captured output + locator.
	r := got[1]
	if r.Name != "TestParseBar" || r.Status != TestFailed || r.Duration != "0.05s" {
		t.Errorf("row 1 = %+v, want TestParseBar FAIL 0.05s", r)
	}
	if r.File != "parse_test.go" || r.Line != 42 {
		t.Errorf("row 1 locator = %s:%d, want parse_test.go:42", r.File, r.Line)
	}
	// Output must include both captured lines, in order, separated by
	// a single newline. Indentation must be preserved (the runner
	// indents helper-reported lines, and the panel shows them as-is).
	if !strings.Contains(r.Output, "parse_test.go:42: want 7, got 11") {
		t.Errorf("row 1 output missing first failure line: %q", r.Output)
	}
	if !strings.Contains(r.Output, "parse_test.go:43: trailing detail line") {
		t.Errorf("row 1 output missing trailing line: %q", r.Output)
	}
	// Sanity: the next test's `=== RUN` line must NOT bleed into the
	// failure capture.
	if strings.Contains(r.Output, "=== RUN") {
		t.Errorf("row 1 output leaked next test's RUN line: %q", r.Output)
	}

	// Row 2 — TestParseQux, skipped. No File/Line jump for skips even
	// if the runner printed one; the parser only attaches locators to
	// FAILs.
	if got[2].Name != "TestParseQux" || got[2].Status != TestSkipped || got[2].Duration != "0.00s" {
		t.Errorf("row 2 = %+v, want TestParseQux SKIP 0.00s", got[2])
	}
	if got[2].File != "" || got[2].Line != 0 {
		t.Errorf("row 2 should not carry a locator: %+v", got[2])
	}

	// Row 3 — TestParent/sub, the subtest.
	if got[3].Name != "TestParent/sub" || got[3].Status != TestPassed {
		t.Errorf("row 3 = %+v, want TestParent/sub PASS", got[3])
	}

	// Row 4 — TestParent, the parent group.
	if got[4].Name != "TestParent" || got[4].Status != TestPassed {
		t.Errorf("row 4 = %+v, want TestParent PASS", got[4])
	}
}

// TestParseTestOutputCounts asserts Counts() tallies the fixture.
func TestParseTestOutputCounts(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	passed, failed, skipped := p.Counts()
	if passed != 3 || failed != 1 || skipped != 1 {
		t.Errorf("Counts() = (%d, %d, %d), want (3, 1, 1)", passed, failed, skipped)
	}
}

// TestParseTestOutputPackageFromOk verifies an `ok pkg time` summary
// line sets the package for the rows that preceded it.
func TestParseTestOutputPackageFromOk(t *testing.T) {
	in := `=== RUN   TestA
--- PASS: TestA (0.01s)
ok      silk/example    0.42s`
	got := parseTestOutput(in)
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(got), got)
	}
	if got[0].Package != "silk/example" {
		t.Errorf("Package = %q, want silk/example", got[0].Package)
	}
}

// TestParseTestOutputEmpty handles the empty / blank input case
// cleanly without producing rows.
func TestParseTestOutputEmpty(t *testing.T) {
	if got := parseTestOutput(""); len(got) != 0 {
		t.Errorf("empty input produced %d rows: %+v", len(got), got)
	}
	if got := parseTestOutput("\n\n   \n"); len(got) != 0 {
		t.Errorf("whitespace input produced %d rows: %+v", len(got), got)
	}
}

// TestParseTestOutputFailWithoutLocator captures a failure with no
// `file:line:` line in its output: Output is filled, but File / Line
// stay zero so the panel knows there is nothing to jump to.
func TestParseTestOutputFailWithoutLocator(t *testing.T) {
	in := `=== RUN   TestNoLoc
--- FAIL: TestNoLoc (0.01s)
    a free-text complaint with no locator
FAIL
FAIL    silk/x    0.01s`
	got := parseTestOutput(in)
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(got), got)
	}
	if got[0].Status != TestFailed {
		t.Errorf("status = %v, want FAIL", got[0].Status)
	}
	if got[0].File != "" || got[0].Line != 0 {
		t.Errorf("locator should be empty, got %s:%d", got[0].File, got[0].Line)
	}
	if !strings.Contains(got[0].Output, "free-text complaint") {
		t.Errorf("output missing failure text: %q", got[0].Output)
	}
}

// TestSetOutputPopulates verifies the panel API: SetOutput drives the
// parser and Results() exposes the rows.
func TestResultsSetOutputPopulates(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	rs := p.Results()
	if len(rs) != 5 {
		t.Fatalf("Results() len = %d, want 5", len(rs))
	}
	if rs[1].Name != "TestParseBar" || rs[1].Status != TestFailed {
		t.Errorf("Results()[1] = %+v, want the FAIL row", rs[1])
	}
}

// TestResultsClearEmpties verifies Clear drops all rows.
func TestResultsClearEmpties(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)
	p.Clear()
	if got := p.Results(); len(got) != 0 {
		t.Errorf("after Clear, Results() = %+v, want empty", got)
	}
	passed, failed, skipped := p.Counts()
	if passed != 0 || failed != 0 || skipped != 0 {
		t.Errorf("after Clear, Counts() = (%d,%d,%d), want all zero", passed, failed, skipped)
	}
}

// TestResultsOnLeftDownFiresActivated confirms clicking a FAIL row
// with a recoverable File fires SigResultActivated with that exact
// TestResult. Rows sit below the testResultsHeaderH header band at
// rowHeight px each; row 1 (the FAIL) lands at header + 1.5 rows.
func TestResultsOnLeftDownFiresActivated(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	var got TestResult
	fired := false
	p.SigResultActivated(func(r TestResult) {
		got = r
		fired = true
	})

	// Row 1 is the FAIL.
	y := testResultsHeaderH + 1.5*p.rowHeight
	p.OnLeftDown(5, y)

	if !fired {
		t.Fatal("SigResultActivated did not fire for a FAIL row click")
	}
	if got.Name != "TestParseBar" {
		t.Errorf("activated row = %q, want TestParseBar", got.Name)
	}
	if got.File != "parse_test.go" || got.Line != 42 {
		t.Errorf("activated locator = %s:%d, want parse_test.go:42", got.File, got.Line)
	}
}

// TestResultsOnLeftDownPassIgnored verifies a click on a PASS row does
// NOT fire the activated callback — there is no failure to jump to.
func TestResultsOnLeftDownPassIgnored(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)

	fired := false
	p.SigResultActivated(func(TestResult) { fired = true })

	// Row 0 is TestParseFoo (PASS).
	y := testResultsHeaderH + 0.5*p.rowHeight
	p.OnLeftDown(5, y)

	if fired {
		t.Error("SigResultActivated fired for a PASS row")
	}
}

// TestResultsOnLeftDownHeaderIgnored verifies a click in the header
// band does not fire the activated callback.
func TestResultsOnLeftDownHeaderIgnored(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(sampleGoTestOutput)
	fired := false
	p.SigResultActivated(func(TestResult) { fired = true })
	p.OnLeftDown(5, testResultsHeaderH/2)
	if fired {
		t.Error("SigResultActivated fired for a header click")
	}
}

// TestResultsOnLeftDownFailWithoutLocatorInert verifies that a FAIL
// row whose Output contained no parseable `file:line:` is NOT activated
// on click — the IDE has nowhere to jump to.
func TestResultsOnLeftDownFailWithoutLocatorInert(t *testing.T) {
	p := NewTestResultsPanel()
	p.SetOutput(`=== RUN   TestNoLoc
--- FAIL: TestNoLoc (0.01s)
    a free-text complaint with no locator
FAIL    silk/x    0.01s`)

	fired := false
	p.SigResultActivated(func(TestResult) { fired = true })

	y := testResultsHeaderH + 0.5*p.rowHeight
	p.OnLeftDown(5, y)

	if fired {
		t.Error("activated fired for a FAIL row with no recoverable File")
	}
}

// TestResultsPanelFactoryRegistered confirms the factory id resolves to
// a constructible *TestResultsPanel, matching how silkide will build
// the dockable view at session load.
func TestResultsPanelFactoryRegistered(t *testing.T) {
	obj := core.New("ged.TestResults")
	if _, ok := obj.(*TestResultsPanel); !ok {
		t.Fatalf("factory ged.TestResults built %T, want *TestResultsPanel", obj)
	}
}

// TestParseTestOutputSubtestUnderFailing covers a parent test whose
// subtest fails: both rows should be present, and the parent row
// should pick up the parent failure output (the subtest's failure
// itself is on the subtest row).
func TestParseTestOutputSubtestUnderFailing(t *testing.T) {
	in := `=== RUN   TestX
=== RUN   TestX/sub
    sub_test.go:5: subtest detail
--- FAIL: TestX/sub (0.01s)
    sub_test.go:5: subtest detail
--- FAIL: TestX (0.02s)
FAIL
FAIL    silk/x    0.03s`
	got := parseTestOutput(in)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(got), got)
	}
	if got[0].Name != "TestX/sub" || got[0].Status != TestFailed {
		t.Errorf("row 0 = %+v, want TestX/sub FAIL", got[0])
	}
	if got[0].File != "sub_test.go" || got[0].Line != 5 {
		t.Errorf("row 0 locator = %s:%d, want sub_test.go:5", got[0].File, got[0].Line)
	}
	if got[1].Name != "TestX" || got[1].Status != TestFailed {
		t.Errorf("row 1 = %+v, want TestX FAIL", got[1])
	}
}
