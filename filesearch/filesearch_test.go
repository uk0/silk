package filesearch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- helpers ---------------------------------------------------------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	writeBytes(t, path, []byte(content))
}

func writeBytes(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// buildTree lays out a mixed tree: source + text files, a NUL binary,
// and .git / vendor / hidden dirs that must be pruned.
func buildTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"),
		"package main\n// TODO: refactor\nfunc Foo() {}\n")
	writeFile(t, filepath.Join(root, "b.txt"),
		"a todo here\nand a TODO there\n")
	writeFile(t, filepath.Join(root, "sub", "c.go"),
		"func Bar() {}\n// nothing to see\n")
	// binary: contains a NUL and the unique token NEEDLE.
	writeBytes(t, filepath.Join(root, "data.bin"),
		[]byte("head\x00NEEDLE tail\n"))
	// pruned dirs, each carrying a TODO that must never surface.
	writeFile(t, filepath.Join(root, ".git", "config"), "// TODO in git\n")
	writeFile(t, filepath.Join(root, "vendor", "v.go"), "// TODO in vendor\n")
	writeFile(t, filepath.Join(root, ".hidden", "h.go"), "// TODO hidden\n")
	// multibyte content for byte-column checks (é is 2 bytes in UTF-8).
	writeFile(t, filepath.Join(root, "uni.txt"), "x café y café z\n")
	return root
}

func colsForBase(ms []Match, base string) []int {
	var cols []int
	for _, m := range ms {
		if filepath.Base(m.Path) == base {
			cols = append(cols, m.Col)
		}
	}
	return cols
}

func forBase(ms []Match, base string) []Match {
	var out []Match
	for _, m := range ms {
		if filepath.Base(m.Path) == base {
			out = append(out, m)
		}
	}
	return out
}

func hasBase(ms []Match, base string) bool {
	return len(forBase(ms, base)) > 0
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Search: literal / case ------------------------------------------

func TestSearchLiteralCaseSensitive(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "TODO", Options{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// Case-sensitive "TODO": a.go line2 col4, b.txt line2 col7 only.
	a := forBase(ms, "a.go")
	if len(a) != 1 || a[0].Line != 2 || a[0].Col != 4 {
		t.Fatalf("a.go matches = %+v, want one at line2 col4", a)
	}
	b := forBase(ms, "b.txt")
	if len(b) != 1 || b[0].Line != 2 || b[0].Col != 7 {
		t.Fatalf("b.txt matches = %+v, want one at line2 col7 (lowercase todo must NOT match)", b)
	}
	if a[0].Text != "// TODO: refactor" {
		t.Fatalf("Text = %q, want the full line", a[0].Text)
	}
	// Pruned dirs must contribute nothing.
	for _, base := range []string{"config", "v.go", "h.go"} {
		if hasBase(ms, base) {
			t.Fatalf("%s should have been pruned but appeared in results", base)
		}
	}
}

func TestSearchIgnoreCase(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "todo", Options{IgnoreCase: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// b.txt: line1 "a todo here" col3, line2 "and a TODO there" col7.
	if got := colsForBase(ms, "b.txt"); !eqInts(got, []int{3, 7}) {
		t.Fatalf("b.txt cols = %v, want [3 7]", got)
	}
	// a.go picks up the uppercase TODO under case folding.
	if got := colsForBase(ms, "a.go"); !eqInts(got, []int{4}) {
		t.Fatalf("a.go cols = %v, want [4]", got)
	}
}

func TestSearchNoMatch(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "zzz-not-present", Options{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ms) != 0 {
		t.Fatalf("want no matches, got %d", len(ms))
	}
}

// --- Search: regex ---------------------------------------------------

func TestSearchRegex(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, `func\s+\w+`, Options{Regex: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got := colsForBase(ms, "a.go"); !eqInts(got, []int{1}) { // "func Foo"
		t.Fatalf("a.go cols = %v, want [1]", got)
	}
	if got := colsForBase(ms, "c.go"); !eqInts(got, []int{1}) { // "func Bar"
		t.Fatalf("c.go cols = %v, want [1]", got)
	}
	if hasBase(ms, "b.txt") {
		t.Fatalf("b.txt has no func line but matched")
	}
}

func TestSearchRegexIgnoreCase(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "FOO", Options{Regex: true, IgnoreCase: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// "func Foo() {}" -> "Foo" at byte 5 => col 6.
	if got := colsForBase(ms, "a.go"); !eqInts(got, []int{6}) {
		t.Fatalf("a.go cols = %v, want [6]", got)
	}
}

func TestSearchBadRegex(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "(unclosed", Options{Regex: true})
	if err == nil {
		t.Fatalf("want a compile error for a bad regex, got nil")
	}
	if ms != nil {
		t.Fatalf("want nil matches on bad regex, got %v", ms)
	}
}

// --- Include filter --------------------------------------------------

func TestIncludeFilterGoOnly(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "todo", Options{IgnoreCase: true, Include: []string{".go"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !hasBase(ms, "a.go") {
		t.Fatalf("a.go (.go) should be included")
	}
	if hasBase(ms, "b.txt") {
		t.Fatalf("b.txt (.txt) should be excluded by Include=[.go]")
	}
}

func TestIncludeFilterTxtOnly(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "todo", Options{IgnoreCase: true, Include: []string{"txt"}}) // dot optional
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got := colsForBase(ms, "b.txt"); !eqInts(got, []int{3, 7}) {
		t.Fatalf("b.txt cols = %v, want [3 7]", got)
	}
	if hasBase(ms, "a.go") {
		t.Fatalf("a.go (.go) should be excluded by Include=[txt]")
	}
}

// --- SkipBinary ------------------------------------------------------

func TestSkipBinaryTrue(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "NEEDLE", Options{SkipBinary: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if hasBase(ms, "data.bin") {
		t.Fatalf("data.bin has a NUL and must be skipped when SkipBinary is set")
	}
	if len(ms) != 0 {
		t.Fatalf("NEEDLE lives only in the binary; want 0 matches, got %d", len(ms))
	}
}

func TestSkipBinaryFalseScansIt(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "NEEDLE", Options{SkipBinary: false})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// "head\x00NEEDLE tail" -> NEEDLE at byte 5 => col 6.
	b := forBase(ms, "data.bin")
	if len(b) != 1 || b[0].Line != 1 || b[0].Col != 6 {
		t.Fatalf("data.bin matches = %+v, want one at line1 col6", b)
	}
}

// --- pruned dirs, explicit ------------------------------------------

func TestPrunedDirs(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "TODO", Options{IgnoreCase: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, base := range []string{"config", "v.go", "h.go"} {
		if hasBase(ms, base) {
			t.Fatalf(".git/vendor/hidden not pruned: %s surfaced", base)
		}
	}
}

// --- empty tree / empty pattern --------------------------------------

func TestEmptyTree(t *testing.T) {
	root := t.TempDir()
	ms, err := Search(root, "anything", Options{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ms) != 0 {
		t.Fatalf("empty tree: want 0 matches, got %d", len(ms))
	}
}

func TestEmptyPattern(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "", Options{})
	if err != nil {
		t.Fatalf("empty pattern should not error, got %v", err)
	}
	if len(ms) != 0 {
		t.Fatalf("empty pattern must match nothing, got %d", len(ms))
	}
}

// --- multibyte columns + multiple matches per line -------------------

func TestUnicodeColumnsAndMultiPerLine(t *testing.T) {
	root := buildTree(t)
	ms, err := Search(root, "café", Options{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// "x café y café z": first café at byte 2 => col 3; second at byte 10
	// => col 11 (byte offsets, so the multibyte é counts as 2). Two hits
	// on one line become two Matches.
	u := forBase(ms, "uni.txt")
	if len(u) != 2 {
		t.Fatalf("want 2 café matches on one line, got %d: %+v", len(u), u)
	}
	if u[0].Line != 1 || u[1].Line != 1 {
		t.Fatalf("both matches should be on line 1, got %+v", u)
	}
	if got := []int{u[0].Col, u[1].Col}; !eqInts(got, []int{3, 11}) {
		t.Fatalf("café byte cols = %v, want [3 11]", got)
	}
}

func TestBoundaryColumns(t *testing.T) {
	root := t.TempDir()
	// match at the very start (col 1) and one ending at end-of-line.
	writeFile(t, filepath.Join(root, "edge.txt"), "abcXYZabc\n")
	ms, err := Search(root, "abc", Options{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got := colsForBase(ms, "edge.txt"); !eqInts(got, []int{1, 7}) {
		t.Fatalf("cols = %v, want [1 7] (start boundary + trailing match)", got)
	}
}

func TestOverlappingLiteralIsNonOverlapping(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "aa.txt"), "aaaa\n")
	ms, err := Search(root, "aa", Options{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// "aaaa" with "aa": non-overlapping => cols 1 and 3, not 1,2,3.
	if got := colsForBase(ms, "aa.txt"); !eqInts(got, []int{1, 3}) {
		t.Fatalf("cols = %v, want [1 3] (non-overlapping)", got)
	}
}

// --- large input -----------------------------------------------------

func TestLargeInputs(t *testing.T) {
	root := t.TempDir()
	// A single very long line (> the 64K reader buffer) with the token at
	// the end proves lines are not truncated and big columns are exact.
	longLine := strings.Repeat("a", 100000) + "NEEDLE\n"
	writeFile(t, filepath.Join(root, "long.txt"), longLine)
	// A tall file proves streaming line counting on many lines.
	var b strings.Builder
	for i := 1; i < 5000; i++ {
		b.WriteString("filler line\n")
	}
	b.WriteString("MARKER here\n") // line 5000
	writeFile(t, filepath.Join(root, "many.txt"), b.String())

	ms, err := Search(root, "NEEDLE", Options{})
	if err != nil {
		t.Fatalf("Search NEEDLE: %v", err)
	}
	l := forBase(ms, "long.txt")
	if len(l) != 1 || l[0].Line != 1 || l[0].Col != 100001 {
		t.Fatalf("long line match = %+v, want line1 col100001", l)
	}

	ms, err = Search(root, "MARKER", Options{})
	if err != nil {
		t.Fatalf("Search MARKER: %v", err)
	}
	m := forBase(ms, "many.txt")
	if len(m) != 1 || m[0].Line != 5000 || m[0].Col != 1 {
		t.Fatalf("marker match = %+v, want line5000 col1", m)
	}
}

// --- single file as root --------------------------------------------

func TestSearchSingleFileRoot(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "solo.go")
	writeFile(t, f, "line one\nfind me\n")
	ms, err := Search(f, "find", Options{}) // root is a file, not a dir
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ms) != 1 || ms[0].Line != 2 || ms[0].Col != 1 {
		t.Fatalf("single-file search = %+v, want one at line2 col1", ms)
	}
}

// --- Replace ---------------------------------------------------------

func TestReplaceLiteralAllOccurrences(t *testing.T) {
	m := Match{Text: "the price is a fair price"}
	got := Replace(m, "price", "cost", Options{})
	want := "the cost is a fair cost"
	if got != want {
		t.Fatalf("Replace = %q, want %q", got, want)
	}
}

func TestReplaceLiteralKeepsDollarLiteral(t *testing.T) {
	m := Match{Text: "value=1"}
	// literal mode: "$9" in the replacement must stay verbatim.
	if got := Replace(m, "1", "$9", Options{}); got != "value=$9" {
		t.Fatalf("Replace = %q, want %q", got, "value=$9")
	}
}

func TestReplaceRegexGroups(t *testing.T) {
	m := Match{Text: "a=1 b=2"}
	got := Replace(m, `(\w+)=(\w+)`, "$2=$1", Options{Regex: true})
	want := "1=a 2=b"
	if got != want {
		t.Fatalf("Replace = %q, want %q ($1/$2 must expand)", got, want)
	}
}

func TestReplaceRegexIgnoreCase(t *testing.T) {
	m := Match{Text: "Color and colour"}
	got := Replace(m, "colou?r", "X", Options{Regex: true, IgnoreCase: true})
	want := "X and X"
	if got != want {
		t.Fatalf("Replace = %q, want %q", got, want)
	}
}

func TestReplaceIgnoreCaseLiteral(t *testing.T) {
	m := Match{Text: "Foo foo FOO"}
	got := Replace(m, "foo", "bar", Options{IgnoreCase: true})
	want := "bar bar bar"
	if got != want {
		t.Fatalf("Replace = %q, want %q", got, want)
	}
	// literal ignore-case must still keep "$" verbatim.
	if got := Replace(Match{Text: "foo"}, "foo", "$1", Options{IgnoreCase: true}); got != "$1" {
		t.Fatalf("Replace = %q, want %q", got, "$1")
	}
}

func TestReplaceEmptyPattern(t *testing.T) {
	m := Match{Text: "unchanged"}
	if got := Replace(m, "", "x", Options{}); got != "unchanged" {
		t.Fatalf("Replace with empty pattern = %q, want unchanged", got)
	}
}

func TestReplaceNoMatchUnchanged(t *testing.T) {
	m := Match{Text: "nothing here"}
	if got := Replace(m, "absent", "x", Options{}); got != "nothing here" {
		t.Fatalf("Replace = %q, want the line unchanged", got)
	}
}
