package ged

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReplaceAllRewritesMatchingFiles is an integration test: it points a
// headless GlobalSearchPanel at a temp dir, seeds the results through the
// synchronous engine helper (Search itself runs the walk off-thread and
// marshals back via gui.Post, which a headless test cannot pump), then runs
// Replace All and asserts that every file containing the query is rewritten
// case-insensitively — including non-.go text now that the search is broadened
// — while a file with no match stays byte-identical.
func TestReplaceAllRewritesMatchingFiles(t *testing.T) {
	dir := t.TempDir()

	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	txt := filepath.Join(dir, "notes.txt") // broadened search covers non-.go text
	noMatch := filepath.Join(dir, "nomatch.go")

	aContent := "var Widget = 1\n// widget reference\n"
	bContent := "type WIDGET struct{}\n"
	txtContent := "this widget lives in text\n"
	noMatchContent := "package main\nfunc main() {}\n"

	writeFile(t, a, aContent)
	writeFile(t, b, bContent)
	writeFile(t, txt, txtContent)
	writeFile(t, noMatch, noMatchContent)

	p := NewGlobalSearchPanel()
	p.SetRootDir(dir)

	// Drive the search synchronously; this mirrors what Search's gui.Post
	// closure does on the main thread once the background walk returns.
	p.query = "widget"
	p.applyResults(runSearch(p.rootDir, "widget"))
	p.rebuildFlatRows()

	// Case-insensitive search matches Widget, widget, WIDGET (.go) and the
	// occurrence in notes.txt: 4 matches across 3 files.
	if got := p.totalMatchCount(); got != 4 {
		t.Fatalf("pre-replace match count = %d, want 4", got)
	}
	if got := p.totalFileCount(); got != 3 {
		t.Fatalf("pre-replace file count = %d, want 3", got)
	}

	// Set replace text and run Replace All.
	p.replaceRunes = []rune("Gadget")
	p.ReplaceAll()

	// After replacing, the rewritten files no longer contain "widget" in any
	// casing, so the refreshed search must report zero matches.
	if got := p.totalMatchCount(); got != 0 {
		t.Fatalf("post-replace match count = %d, want 0", got)
	}

	assertFileEquals(t, a, "var Gadget = 1\n// Gadget reference\n")
	assertFileEquals(t, b, "type Gadget struct{}\n")
	assertFileEquals(t, txt, "this Gadget lives in text\n")

	// A file with no match must be left byte-for-byte identical.
	assertFileEquals(t, noMatch, noMatchContent)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileEquals(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", filepath.Base(path), string(got), want)
	}
}
