package ged

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/uk0/silk/filesearch"
	"github.com/uk0/silk/gui"
)

// The panel's option setters must land on the engine's Options exactly, and
// whole-word matching (which the engine has no flag for) must be compiled into
// the pattern with Regex forced on.
func TestSearchOptionsMapToEngineOptions(t *testing.T) {
	p := NewGlobalSearchPanel()

	// Defaults are the panel's original behaviour: case-insensitive literal,
	// every text file, binaries skipped.
	want := filesearch.Options{IgnoreCase: true, SkipBinary: true}
	if got := p.searchOptions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("default searchOptions = %+v, want %+v", got, want)
	}
	if got := enginePattern("a|b", p.optRegex, p.optWholeWord); got != "a|b" {
		t.Errorf("default enginePattern = %q, want the query verbatim", got)
	}
	if got := searchOptionLabel(p.searchOptions(), p.optWholeWord, p.exclude); got != "" {
		t.Errorf("default searchOptionLabel = %q, want empty", got)
	}

	// Regex + case sensitive.
	p.SetRegex(true)
	p.SetCaseSensitive(true)
	want = filesearch.Options{Regex: true, SkipBinary: true}
	if got := p.searchOptions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("regex+case searchOptions = %+v, want %+v", got, want)
	}
	if got := enginePattern(`func \w+`, p.optRegex, p.optWholeWord); got != `func \w+` {
		t.Errorf("regex enginePattern = %q, want the query verbatim", got)
	}

	// Include is passed through verbatim (filesearch normalises the dot/case).
	p.SetInclude([]string{".go", "txt"})
	want.Include = []string{".go", "txt"}
	if got := p.searchOptions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("include searchOptions = %+v, want %+v", got, want)
	}

	// Whole word over a regex query: grouped so an alternation still binds.
	p.SetWholeWord(true)
	if got := p.searchOptions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("wholeword searchOptions = %+v, want %+v", got, want)
	}
	if got, exp := enginePattern("a|b", p.optRegex, p.optWholeWord), `\b(?:a|b)\b`; got != exp {
		t.Errorf("regex wholeword enginePattern = %q, want %q", got, exp)
	}

	// Whole word over a literal query: quoted, and Regex is forced on even
	// though the user did not ask for regex.
	p.SetRegex(false)
	if got := p.searchOptions(); !got.Regex {
		t.Errorf("wholeword must force Options.Regex on, got %+v", got)
	}
	if got, exp := enginePattern("a|b", p.optRegex, p.optWholeWord), `\ba\|b\b`; got != exp {
		t.Errorf("literal wholeword enginePattern = %q, want %q", got, exp)
	}

	// Exclude is not an engine option — it filters the engine's output.
	p.SetExclude([]string{"*_test.go"})
	if got := p.searchOptions(); !reflect.DeepEqual(got.Include, []string{".go", "txt"}) {
		t.Errorf("exclude leaked into Options: %+v", got)
	}
	if !excludedPath("/w/pkg/a_test.go", p.exclude) {
		t.Error("excludedPath missed a *_test.go glob")
	}
	if excludedPath("/w/pkg/a.go", p.exclude) {
		t.Error("excludedPath dropped a non-matching file")
	}
	if !excludedPath("/w/testdata/a.go", []string{"testdata"}) {
		t.Error("excludedPath missed a directory-segment pattern")
	}
	if got := searchOptionLabel(p.searchOptions(), p.optWholeWord, p.exclude); got == "" {
		t.Error("searchOptionLabel is empty with non-default options")
	}
}

// The options must actually reach the engine: extension filter, exclude globs,
// case sensitivity, regex (with MatchLen taken from the matched text rather
// than the pattern) and whole-word all change the result set.
func TestRunSearchOptHonoursOptions(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "a.go")
	testFile := filepath.Join(dir, "a_test.go")
	txtFile := filepath.Join(dir, "notes.txt")

	writeFile(t, goFile, "func Alpha() {}\nvar alpha = 1\nvar alphabet = 2\n")
	writeFile(t, testFile, "func TestAlpha() {}\n")
	writeFile(t, txtFile, "alpha in text\n")

	files := func(ms []SearchMatch) []string {
		var out []string
		for _, m := range ms {
			out = append(out, filepath.Base(m.FilePath))
		}
		return out
	}

	// Case-sensitive literal: "Alpha" only in the two func lines.
	ms := runSearchOpt(dir, "Alpha", filesearch.Options{SkipBinary: true}, nil)
	if got, want := files(ms), []string{"a.go", "a_test.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("case-sensitive matches in %v, want %v", got, want)
	}

	// Include .go only: notes.txt drops out.
	ms = runSearchOpt(dir, "alpha", filesearch.Options{IgnoreCase: true, Include: []string{".go"}}, nil)
	if got, want := files(ms), []string{"a.go", "a.go", "a.go", "a_test.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("include .go matched %v, want %v", got, want)
	}

	// Exclude *_test.go on top of the include filter.
	ms = runSearchOpt(dir, "alpha", filesearch.Options{IgnoreCase: true, Include: []string{".go"}}, []string{"*_test.go"})
	if got, want := files(ms), []string{"a.go", "a.go", "a.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("exclude *_test.go matched %v, want %v", got, want)
	}

	// Regex: MatchLen must be the matched text's length, not the pattern's.
	ms = runSearchOpt(dir, `func \w+`, filesearch.Options{Regex: true, Include: []string{".go"}}, []string{"*_test.go"})
	if len(ms) != 1 {
		t.Fatalf("regex matches = %d, want 1: %+v", len(ms), ms)
	}
	if ms[0].MatchLen != len("func Alpha") {
		t.Errorf("regex MatchLen = %d, want %d (pattern length is %d)",
			ms[0].MatchLen, len("func Alpha"), len(`func \w+`))
	}

	// Whole word: "alphabet" must not match "alpha".
	pattern := enginePattern("alpha", false, true)
	ms = runSearchOpt(dir, pattern, filesearch.Options{Regex: true, Include: []string{".go"}}, []string{"*_test.go"})
	if len(ms) != 1 {
		t.Fatalf("whole-word matches = %d, want 1: %+v", len(ms), ms)
	}
	if ms[0].Line != 2 || ms[0].MatchLen != len("alpha") {
		t.Errorf("whole-word match = line %d len %d, want line 2 len 5", ms[0].Line, ms[0].MatchLen)
	}
}

// ComputeReplacements is a preview: it returns the per-file before/after pair
// and leaves every byte on disk untouched. Files whose text would not change
// are left out of the preview entirely.
func TestComputeReplacementsPreviewsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	noMatch := filepath.Join(dir, "nomatch.go")

	aContent := "var Widget = 1\n// widget reference\n"
	bContent := "type WIDGET struct{}\n"
	noMatchContent := "package main\n"

	writeFile(t, a, aContent)
	writeFile(t, b, bContent)
	writeFile(t, noMatch, noMatchContent)

	p := NewGlobalSearchPanel()
	p.SetRootDir(dir)
	p.query = "widget"
	p.applyResults(runSearch(p.rootDir, "widget"))
	p.replaceRunes = []rune("Gadget")

	reps := p.ComputeReplacements()

	want := []FileReplacement{
		{Path: a, Before: aContent, After: "var Gadget = 1\n// Gadget reference\n", Count: 2},
		{Path: b, Before: bContent, After: "type Gadget struct{}\n", Count: 1},
	}
	if !reflect.DeepEqual(reps, want) {
		t.Fatalf("ComputeReplacements mismatch:\n got  %+v\n want %+v", reps, want)
	}

	// Nothing may have been written: the preview is read-only.
	assertFileEquals(t, a, aContent)
	assertFileEquals(t, b, bContent)
	assertFileEquals(t, noMatch, noMatchContent)

	// A replacement that produces identical text yields no work item, so no
	// file is rewritten (and no mtime churn) for nothing.
	p.SetCaseSensitive(true)
	p.query = "widget"
	p.applyResults(runSearchOpt(p.rootDir, "widget", p.searchOptions(), nil))
	p.replaceRunes = []rune("widget")
	if reps := p.ComputeReplacements(); len(reps) != 0 {
		t.Errorf("no-op replacement produced %d work item(s): %+v", len(reps), reps)
	}
}

// A committed transaction writes every file and reports each one exactly once
// through SigFileReplaced, with the contents the host should reload.
func TestApplyReplacementsCommitsAndNotifies(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	same := filepath.Join(dir, "same.go")

	writeFile(t, a, "widget\n")
	writeFile(t, b, "widget widget\n")
	writeFile(t, same, "untouched\n")

	p := NewGlobalSearchPanel()
	seen := map[string]string{}
	p.SigFileReplaced(func(path, content string) {
		if _, dup := seen[path]; dup {
			t.Errorf("SigFileReplaced fired twice for %s", path)
		}
		seen[path] = content
	})

	err := p.ApplyReplacements([]FileReplacement{
		{Path: a, Before: "widget\n", After: "gadget\n", Count: 1},
		{Path: b, Before: "widget widget\n", After: "gadget gadget\n", Count: 2},
		// Before == After: nothing to write, nothing to notify.
		{Path: same, Before: "untouched\n", After: "untouched\n"},
	})
	if err != nil {
		t.Fatalf("ApplyReplacements: %v", err)
	}

	assertFileEquals(t, a, "gadget\n")
	assertFileEquals(t, b, "gadget gadget\n")
	assertFileEquals(t, same, "untouched\n")

	want := map[string]string{a: "gadget\n", b: "gadget gadget\n"}
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("SigFileReplaced saw %v, want %v", seen, want)
	}
}

// A write failure part-way through must roll the whole transaction back: the
// files already rewritten return to their Before text, the error surfaces, and
// the host is never told to reload a file whose change was undone.
func TestApplyReplacementsRollsBackOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	writeFile(t, a, "widget\n")
	writeFile(t, b, "widget\n")

	// A directory can never be opened for writing, so the third write fails
	// deterministically (and identically for root) on every platform.
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	p := NewGlobalSearchPanel()
	var notified []string
	p.SigFileReplaced(func(path, content string) { notified = append(notified, path) })

	err := p.ApplyReplacements([]FileReplacement{
		{Path: a, Before: "widget\n", After: "gadget\n", Count: 1},
		{Path: b, Before: "widget\n", After: "gadget\n", Count: 1},
		{Path: blocked, Before: "widget\n", After: "gadget\n", Count: 1},
	})
	if err == nil {
		t.Fatal("ApplyReplacements returned nil writing to a directory")
	}
	// The message pins down that the failure came AFTER two successful writes,
	// i.e. the rollback really had work to undo.
	if !strings.Contains(err.Error(), "rolled back 2 file(s)") {
		t.Errorf("error = %v, want it to report 2 rolled-back files", err)
	}

	assertFileEquals(t, a, "widget\n")
	assertFileEquals(t, b, "widget\n")
	if len(notified) != 0 {
		t.Errorf("SigFileReplaced fired on a rolled-back transaction: %v", notified)
	}
}

// Replace All must use the panel's options for both halves of the operation:
// whole-word regex search AND the rewrite, so "alphabet" survives while the
// standalone "alpha" is replaced.
func TestReplaceAllUsesPanelOptions(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	writeFile(t, a, "var alpha = 1\nvar alphabet = 2\nvar ALPHA = 3\n")

	p := NewGlobalSearchPanel()
	p.SetRootDir(dir)
	p.SetRegex(true)
	p.SetCaseSensitive(true)
	p.SetWholeWord(true)

	p.query = "alpha"
	p.applyResults(runSearchOpt(p.rootDir, enginePattern(p.query, true, true), p.searchOptions(), nil))
	if got := p.totalMatchCount(); got != 1 {
		t.Fatalf("pre-replace match count = %d, want 1", got)
	}

	p.replaceRunes = []rune("beta")
	p.ReplaceAll()

	assertFileEquals(t, a, "var beta = 1\nvar alphabet = 2\nvar ALPHA = 3\n")
	if got := p.totalMatchCount(); got != 0 {
		t.Errorf("post-replace match count = %d, want 0", got)
	}
}

// Replacing line by line must keep each line's own terminator, invent no
// trailing newline, and count occurrences rather than lines.
func TestReplaceInTextPreservesLineEndings(t *testing.T) {
	opt := filesearch.Options{}
	re := searchMatcher("a", opt) // literal + case-sensitive: no matcher needed
	got, n := replaceInText("a\r\nb\na a", "a", "X", opt, re)
	if want := "X\r\nb\nX X"; got != want {
		t.Errorf("replaceInText = %q, want %q", got, want)
	}
	if n != 3 {
		t.Errorf("replaceInText count = %d, want 3", n)
	}
	if got, n := replaceInText("", "a", "X", opt, re); got != "" || n != 0 {
		t.Errorf("replaceInText(\"\") = (%q,%d), want (\"\",0)", got, n)
	}
}

// --- TodoPanel grouping / filter / hit-test ---
// These live in this file (rather than a third new file) to keep this change's
// file set to the two test files it owns; the panel is in the same package.

// Grouping inserts a counted header before each group and keeps the marker rows
// addressable: rowAt maps a display row back to its index in Rows(), and a
// click on a group header activates nothing.
func TestTodoPanelGrouping(t *testing.T) {
	p := NewTodoPanel()
	p.SetSize(300, 400)
	rows := sampleTodoRows() // TODO+FIXME in ged/foo.go, NOTE in core/baz.go
	p.SetRows(rows)

	// Default is flat: display rows line up one-to-one with the marker rows.
	if got := len(p.visRows); got != len(rows) {
		t.Fatalf("flat visRows = %d, want %d", got, len(rows))
	}
	if p.GroupBy() != TodoGroupNone {
		t.Fatalf("default GroupBy = %v, want TodoGroupNone", p.GroupBy())
	}

	// By tag: three kinds -> three headers + three markers.
	p.SetGroupBy(TodoGroupByTag)
	if got := len(p.visRows); got != 6 {
		t.Fatalf("by-tag visRows = %d, want 6: %+v", got, p.visRows)
	}
	if !p.visRows[0].header || p.visRows[0].title != "TODO" || p.visRows[0].count != 1 {
		t.Errorf("by-tag first row = %+v, want header TODO(1)", p.visRows[0])
	}
	if got, want := todoGroupHeaderLabel(p.visRows[0].title, p.visRows[0].count), "TODO (1)"; got != want {
		t.Errorf("todoGroupHeaderLabel = %q, want %q", got, want)
	}

	// By file: ged/foo.go holds two markers, core/baz.go one.
	p.SetGroupBy(TodoGroupByFile)
	if got := len(p.visRows); got != 5 {
		t.Fatalf("by-file visRows = %d, want 5: %+v", got, p.visRows)
	}
	if !p.visRows[0].header || p.visRows[0].title != "ged/foo.go" || p.visRows[0].count != 2 {
		t.Errorf("by-file first row = %+v, want header ged/foo.go(2)", p.visRows[0])
	}

	// Hit-test through the grouped layout: display row 0 is a header (no
	// marker), row 1 is rows[0], row 4 is rows[2].
	rowY := func(i int) float64 { return todoHeaderH + float64(i)*p.rowHeight + p.rowHeight/2 }
	if got := p.rowAt(rowY(0)); got != -1 {
		t.Errorf("rowAt(group header) = %d, want -1", got)
	}
	if got := p.rowAt(rowY(1)); got != 0 {
		t.Errorf("rowAt(first marker) = %d, want 0", got)
	}
	if got := p.rowAt(rowY(4)); got != 2 {
		t.Errorf("rowAt(last marker) = %d, want 2", got)
	}

	var gotFile string
	var gotLine int
	p.SigActivate(func(file string, line int) { gotFile, gotLine = file, line })

	p.OnLeftDown(5, rowY(0)) // group header: inert
	if gotFile != "" {
		t.Errorf("clicking a group header activated %s:%d", gotFile, gotLine)
	}
	p.OnLeftDown(5, rowY(4)) // last marker of the second group
	if gotFile != rows[2].File || gotLine != rows[2].Line {
		t.Errorf("SigActivate = (%q,%d), want (%q,%d)", gotFile, gotLine, rows[2].File, rows[2].Line)
	}

	// The group button cycles flat -> tag -> file -> flat.
	p.SetGroupBy(TodoGroupNone)
	gx, gy, gw, gh := todoGroupButtonRect(p.Width())
	bx, by := gx+gw/2, gy+gh/2
	for _, want := range []TodoGroupMode{TodoGroupByTag, TodoGroupByFile, TodoGroupNone} {
		p.OnLeftDown(bx, by)
		if p.GroupBy() != want {
			t.Fatalf("group button cycled to %v, want %v", p.GroupBy(), want)
		}
	}
}

// The filter box narrows the display rows without touching the row set the
// host handed in, matches case-insensitively across kind/text/path, composes
// with grouping, and is editable through the keyboard.
func TestTodoPanelFilter(t *testing.T) {
	p := NewTodoPanel()
	p.SetSize(300, 400)
	rows := sampleTodoRows()
	p.SetRows(rows)

	// Text match.
	p.SetFilter("scroll")
	if got := len(p.visRows); got != 1 {
		t.Fatalf("filter \"scroll\" kept %d rows, want 1", got)
	}
	if got := p.visRows[0].idx; got != 1 {
		t.Errorf("filter \"scroll\" kept row %d, want 1", got)
	}
	// Rows() is the host's data, not the view: filtering must not shrink it.
	if got := len(p.Rows()); got != len(rows) {
		t.Errorf("Rows() while filtered = %d, want %d", got, len(rows))
	}

	// Kind match, case-insensitive.
	p.SetFilter("fixme")
	if got := len(p.visRows); got != 1 || p.visRows[0].idx != 1 {
		t.Errorf("filter \"fixme\" = %+v, want only row 1", p.visRows)
	}

	// Path match.
	p.SetFilter("core/")
	if got := len(p.visRows); got != 1 || p.visRows[0].idx != 2 {
		t.Errorf("filter \"core/\" = %+v, want only row 2", p.visRows)
	}

	// No match at all.
	p.SetFilter("zzz")
	if got := len(p.visRows); got != 0 {
		t.Errorf("filter \"zzz\" kept %d rows, want 0", got)
	}

	// Composes with grouping: one file group with both of its markers.
	p.SetFilter("foo.go")
	p.SetGroupBy(TodoGroupByFile)
	if got := len(p.visRows); got != 3 {
		t.Fatalf("filtered by-file visRows = %d, want 3: %+v", got, p.visRows)
	}
	if !p.visRows[0].header || p.visRows[0].count != 2 {
		t.Errorf("filtered group header = %+v, want count 2", p.visRows[0])
	}

	// Empty filter restores everything.
	p.SetFilter("")
	p.SetGroupBy(TodoGroupNone)
	if got := len(p.visRows); got != len(rows) {
		t.Errorf("cleared filter kept %d rows, want %d", got, len(rows))
	}

	// Keyboard editing only applies while the filter box is focused.
	p.OnTextInput("scroll")
	if got := p.Filter(); got != "" {
		t.Errorf("typing with the filter unfocused set %q", got)
	}
	fx, fy, fw, fh := todoFilterRect(p.Width())
	p.OnLeftDown(fx+fw/2, fy+fh/2) // focus the box
	p.OnTextInput("scrollX")
	if got := len(p.visRows); got != 0 {
		t.Fatalf("typed filter %q kept %d rows, want 0", p.Filter(), got)
	}
	p.OnKeyDown(gui.KeyBackSpace, false)
	if got, want := p.Filter(), "scroll"; got != want {
		t.Fatalf("after backspace Filter = %q, want %q", got, want)
	}
	if got := len(p.visRows); got != 1 {
		t.Errorf("after backspace visRows = %d, want 1", got)
	}
	p.OnKeyDown(gui.KeyEsc, false)
	if got := p.Filter(); got != "" {
		t.Errorf("Esc left Filter = %q, want empty", got)
	}
	if got := len(p.visRows); got != len(rows) {
		t.Errorf("Esc left %d rows, want %d", got, len(rows))
	}
}
