package main

import (
	"path/filepath"
	"testing"
)

// rankFiles now delegates to the shared locator fuzzy engine, so these
// tests pin the behavior that engine gives the quick-open box: which
// entries survive the subsequence filter, the score-based order (prefix
// and exact hits beating scattered matches), the empty-query contract,
// case folding, and the Path round-trip the open path depends on.

// TestRankFilesMembership: the subsequence filter keeps "gb" -> "gui/button.go"
// (g then b, in order) and drops entries with no such subsequence.
func TestRankFilesMembership(t *testing.T) {
	files := []fileEntry{
		{Display: "main.go"},
		{Display: filepath.Join("gui", "button.go")},
		{Display: filepath.Join("paint", "icon.go")},
	}
	got := rankFiles(files, "gb")
	if len(got) != 1 || got[0].Display != filepath.Join("gui", "button.go") {
		t.Errorf("rankFiles(%q) = %v, want [gui/button.go]", "gb", displays(got))
	}
	if got := rankFiles(files, "xyz"); len(got) != 0 {
		t.Errorf("rankFiles(%q) = %v, want no matches", "xyz", displays(got))
	}
}

// TestRankFilesPrefixBeatsScattered: for query "main", the prefix hit
// "main.go" must rank above a file that only contains m-a-i-n scattered
// across its path. The old length sort could invert this; locator's
// prefix bonus fixes it.
func TestRankFilesPrefixBeatsScattered(t *testing.T) {
	files := []fileEntry{
		{Display: filepath.Join("cmd", "mailer", "main.go")},
		{Display: "main.go"},
	}
	got := rankFiles(files, "main")
	if len(got) != 2 {
		t.Fatalf("rankFiles(%q) returned %d hits, want 2", "main", len(got))
	}
	if got[0].Display != "main.go" {
		t.Errorf("prefix match should rank first; got %v", displays(got))
	}
}

// TestRankFilesExactBeatsPrefix: an exact-name hit outranks a longer
// candidate that merely has the query as a prefix.
func TestRankFilesExactBeatsPrefix(t *testing.T) {
	files := []fileEntry{
		{Display: "main.gold"},
		{Display: "main.go"},
	}
	got := rankFiles(files, "main.go")
	if len(got) != 2 {
		t.Fatalf("rankFiles(%q) returned %d hits, want 2", "main.go", len(got))
	}
	if got[0].Display != "main.go" {
		t.Errorf("exact match should rank first; got %v", displays(got))
	}
}

// TestRankFilesEmptyQueryNameSorted: an empty or all-whitespace query
// returns every entry, Name-sorted (locator.Match's contract). The query
// is trimmed before scoring, so "   " behaves like "".
func TestRankFilesEmptyQueryNameSorted(t *testing.T) {
	files := []fileEntry{
		{Display: "main.go"},
		{Display: "app.go"},
		{Display: "zoo.go"},
	}
	want := []string{"app.go", "main.go", "zoo.go"}
	for _, q := range []string{"", "   "} {
		got := rankFiles(files, q)
		if len(got) != len(want) {
			t.Fatalf("rankFiles(%q) returned %d hits, want %d", q, len(got), len(want))
		}
		for i, w := range want {
			if got[i].Display != w {
				t.Errorf("rankFiles(%q)[%d] = %q, want %q", q, i, got[i].Display, w)
			}
		}
	}
}

// TestRankFilesCaseInsensitive: case variants of the query match the
// same entry (locator folds case internally).
func TestRankFilesCaseInsensitive(t *testing.T) {
	files := []fileEntry{{Display: filepath.Join("gui", "Button.go")}}
	for _, q := range []string{"button", "BUTTON", "BuTtOn"} {
		if got := rankFiles(files, q); len(got) != 1 {
			t.Errorf("rankFiles(%q) = %v, want 1 hit", q, displays(got))
		}
	}
}

// TestRankFilesPreservesPath: ranking carries the absolute Path through
// locator.Item.Detail untouched, so the finder still opens the exact
// file the user selected.
func TestRankFilesPreservesPath(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator)+"abs", "gui", "button.go")
	files := []fileEntry{{Path: abs, Display: filepath.Join("gui", "button.go")}}
	got := rankFiles(files, "button")
	if len(got) != 1 {
		t.Fatalf("rankFiles(%q) returned %d hits, want 1", "button", len(got))
	}
	if got[0].Path != abs {
		t.Errorf("Path not preserved: got %q, want %q", got[0].Path, abs)
	}
}
