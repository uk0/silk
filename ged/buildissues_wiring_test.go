package ged

import "testing"

// wiringSample is a representative go build/vet dump feeding both panel
// parsers. It deliberately includes: a "# pkg" context header, a normal
// error with a column, a "file:line:" line with no column, a "warning:"
// diagnostic, a Windows drive-letter path (C:\...:line:col — the case the
// old hand-rolled parsers dropped), and a plain non-diagnostic log line.
const wiringSample = "# github.com/uk0/silk/ged\n" +
	"main.go:10:6: undefined: Foo\n" +
	"util.go:5: missing return at end of function\n" +
	"sig.go:8:2: warning: redundant nil check\n" +
	"C:\\src\\a.go:3:2: expected declaration\n" +
	"Compiling package ged"

// TestProblemsPanelWiredToBuildissues proves ProblemsPanel's parseProblems
// now routes through the shared buildissues engine: the "# pkg" header and
// the plain log line drop out, the no-column line yields Col 0, the
// "warning:" line is a Warning, and the Windows drive-letter path is
// parsed intact instead of being discarded.
func TestProblemsPanelWiredToBuildissues(t *testing.T) {
	got := parseProblems(wiringSample)
	want := []Problem{
		{File: "main.go", Line: 10, Col: 6, Severity: SeverityError, Message: "undefined: Foo"},
		{File: "util.go", Line: 5, Col: 0, Severity: SeverityError, Message: "missing return at end of function"},
		{File: "sig.go", Line: 8, Col: 2, Severity: SeverityWarning, Message: "warning: redundant nil check"},
		{File: `C:\src\a.go`, Line: 3, Col: 2, Severity: SeverityError, Message: "expected declaration"},
	}
	if len(got) != len(want) {
		t.Fatalf("parseProblems returned %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestBuildOutputWiredToBuildissues proves BuildOutput's parseBuildOutputLines
// now routes error detection through buildissues while still preserving every
// raw line as a log row. The "# pkg" header and the plain line stay as
// non-error text; the four diagnostics become clickable error rows with the
// location buildissues extracted — including the Windows path the old parser
// dropped and the no-column line at Col 0.
func TestBuildOutputWiredToBuildissues(t *testing.T) {
	got := parseBuildOutputLines(wiringSample)
	if len(got) != 6 {
		t.Fatalf("parseBuildOutputLines returned %d rows, want 6: %+v", len(got), got)
	}
	// Non-diagnostic lines are kept as text but not flagged as errors.
	if got[0].IsError {
		t.Errorf("row[0] %q: # pkg context line must not be an error", got[0].Text)
	}
	if got[5].IsError {
		t.Errorf("row[5] %q: plain log line must not be an error", got[5].Text)
	}
	// Diagnostic rows carry the location buildissues parsed.
	wantLoc := []struct {
		idx       int
		file      string
		line, col int
	}{
		{1, "main.go", 10, 6},
		{2, "util.go", 5, 0},
		{3, "sig.go", 8, 2},
		{4, `C:\src\a.go`, 3, 2},
	}
	for _, w := range wantLoc {
		r := got[w.idx]
		if !r.IsError {
			t.Errorf("row[%d] %q: want a clickable error row", w.idx, r.Text)
		}
		if r.File != w.file || r.Line != w.line || r.Col != w.col {
			t.Errorf("row[%d] loc = %s:%d:%d, want %s:%d:%d",
				w.idx, r.File, r.Line, r.Col, w.file, w.line, w.col)
		}
	}
}
