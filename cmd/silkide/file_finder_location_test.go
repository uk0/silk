package main

import (
	"context"
	"github.com/uk0/silk/gui"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileFinderLocationSuffixFindsFile(t *testing.T) {
	posts := make(chan func(), 4)
	service := newFileSearchService(func(context.Context, string) ([]fileEntry, error) {
		return []fileEntry{{Path: "/qa/alpha.go", Display: "alpha.go"}, {Path: "/qa/beta.go", Display: "beta.go"}}, nil
	}, rankFiles, func(fn func()) { posts <- fn })
	service.debounce = time.Millisecond
	f := newFileFinder(nil, "/qa", service)
	defer f.close()
	receiveSymbolTest(t, posts)()
	f.input.SetText("beta.go:2:3")
	receiveSymbolTest(t, posts)()
	if f.list.Count() != 1 || f.list.Item(0).Data.(fileEntry).Display != "beta.go" {
		t.Fatalf("file:line:column failed to find beta.go: %d results", f.list.Count())
	}
}

func TestFileFinderLocationInvalidThenSelectedRow(t *testing.T) {
	posts := make(chan func(), 4)
	service := newFileSearchService(func(context.Context, string) ([]fileEntry, error) {
		return []fileEntry{{Path: "/qa/alpha.go", Display: "alpha.go"}, {Path: "/qa/beta.go", Display: "beta.go"}}, nil
	}, rankFiles, func(fn func()) { posts <- fn })
	service.debounce = time.Millisecond
	f := newFileFinder(nil, "/qa", service)
	defer f.close()
	receiveSymbolTest(t, posts)()
	f.input.SetText(".go:0")
	f.choose(0)
	if f.selected != nil || f.list.Count() != 0 {
		t.Fatal("invalid suffix accepted old result")
	}
	receiveSymbolTest(t, posts)()
	if f.list.Count() != 0 || !strings.Contains(f.status.Text(), "file:line") {
		t.Fatal("invalid input lost feedback", f.status.Text())
	}
	f.input.SetText(".go:2:3")
	f.choose(0)
	if f.selected != nil {
		t.Fatal("pending query accepted stale row")
	}
	receiveSymbolTest(t, posts)()
	f.input.OnKeyDown(gui.KeyDown, false)
	f.input.OnKeyDown(gui.KeyEnter, false)
	if f.selected == nil || f.selected.Display != "beta.go" || f.destination != "2:3" {
		t.Fatal("selection/location mismatch", f.selected, f.destination)
	}
}
func TestFileFinderLocationNavigationUsesLiveUnicodeAndHistory(t *testing.T) {
	w := testWorkbench(t)
	dir := t.TempDir()
	a := writeFile(t, dir, "a.go", "package p\nvar A=1\n")
	b := writeFile(t, dir, "b.go", "disk\n")
	openFileInEditor(w.tabs, b)
	target := activeEditor(w.tabs)
	target.ReplaceAllText("first\r\n😀中\tXYZ\r\nlast")
	target.SetReadOnly(true)
	openFileInEditor(w.tabs, a)
	origin := activeEditor(w.tabs)
	origin.NavigateToLineCol(1, 4)
	if !openFileFinderSelection(w.tabs, b, "2:4") || activeEditor(w.tabs) != target || target.CursorLine() != 1 || target.CursorCol() != 3 {
		t.Fatal("wrong live Unicode destination")
	}
	if !target.Modified() || target.Text() != "first\r\n😀中\tXYZ\r\nlast" || !w.navigation.CanGoBack() {
		t.Fatal("dirty text/history lost")
	}
	target.NavGoBack()
	if activeEditor(w.tabs) != origin || origin.CursorCol() != 4 {
		t.Fatal("back lost origin")
	}
	origin.NavGoForward()
	if activeEditor(w.tabs) != target || target.CursorCol() != 3 {
		t.Fatal("forward lost destination")
	}
	target.NavGoBack()
	if openFileFinderSelection(w.tabs, b, "2:99") || !w.navigation.CanGoForward() || target.CursorCol() != 3 {
		t.Fatal("invalid range moved caret/polluted history")
	}
	if openFileFinderSelection(w.tabs, filepath.Join(dir, "missing.go"), "1") || !w.navigation.CanGoForward() {
		t.Fatal("failed open polluted history")
	}
	got, err := os.ReadFile(b)
	if err != nil || string(got) != "disk\n" {
		t.Fatal("navigation saved document")
	}
}
func TestRankFilesLocationAbsoluteAndQuotedColons(t *testing.T) {
	root := t.TempDir()
	normal := fileEntry{Path: filepath.Join(root, "main.go"), Display: "main.go"}
	files := []fileEntry{normal, {Path: filepath.Join(root, "report:2026"), Display: "report:2026"}}
	for _, tc := range []struct {
		query string
		index int
	}{{normal.Path + ":2:3", 0}, {`"report:2026"`, 1}, {`"report:2026":3:2`, 1}} {
		rows := rankFiles(files, tc.query)
		if len(rows) != 1 || rows[0] != files[tc.index] {
			t.Fatalf("%q: %+v", tc.query, rows)
		}
	}
	if len(rankFiles(files, filepath.Join(root, "outside.go")+":2")) != 0 {
		t.Fatal("unindexed absolute path accepted")
	}
}
