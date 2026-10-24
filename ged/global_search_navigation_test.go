package ged

import (
	"path/filepath"
	"testing"
)

func TestGlobalSearchNavigateMatchesWrapCollapseAndScroll(t *testing.T) {
	p, q := searchPanelTest(t)
	a, b := filepath.Join(p.rootDir, "a.txt"), filepath.Join(p.rootDir, "b.txt")
	writeFile(t, a, "中🙂 foo foo\n")
	writeFile(t, b, "foo\n")
	p.Search("foo")
	awaitSearchPanel(t, p, q)
	p.SetSize(240, 600)
	p.Layout()
	p.SetSize(240, p.listTop()+2*p.rowHeight)
	p.Layout()
	var opened []SearchMatch
	p.SigOpenMatch(func(m SearchMatch) { opened = append(opened, m) })
	p.collapsed[a], p.collapsed[b] = true, true
	p.rebuildFlatRows()
	for _, previous := range []bool{false, false, false, false, true} {
		if !p.NavigateMatch(previous) {
			t.Fatal("failed to navigate")
		}
	}
	wantFiles := []string{a, a, b, a, b}
	wantCols := []int{8, 12, 0, 8, 0}
	for i, m := range opened {
		if m.FilePath != wantFiles[i] || m.Column != wantCols[i] {
			t.Fatalf("step %d: path=%s col=%d", i, m.FilePath, m.Column)
		}
	}
	if len(opened) != 5 || p.collapsed[a] || p.collapsed[b] || p.scrollY <= 0 {
		t.Fatal("navigation did not expand/scroll", len(opened), p.scrollY)
	}
	// First previous with no selection wraps to the last hit.
	p.selectedIdx = -1
	if !p.NavigateMatch(true) || opened[len(opened)-1].FilePath != b {
		t.Fatal("initial previous")
	}
	// A header enters its first match on Next; Previous crosses the file boundary.
	for i, row := range p.flatRows {
		if row.kind == searchRowFile && row.filePath == b {
			p.selectedIdx = i
			break
		}
	}
	p.NavigateMatch(true)
	if m := opened[len(opened)-1]; m.FilePath != a || m.Column != 12 {
		t.Fatal("header previous", m.Column)
	}
	for i, row := range p.flatRows {
		if row.kind == searchRowFile && row.filePath == b {
			p.selectedIdx = i
			break
		}
	}
	p.NavigateMatch(false)
	if opened[len(opened)-1].FilePath != b {
		t.Fatal("header next")
	}
}

func TestGlobalSearchNavigateDoesNotOpenPreviewOrStaleResults(t *testing.T) {
	p, q := searchPanelTest(t)
	path := filepath.Join(p.rootDir, "a.txt")
	writeFile(t, path, "foo")
	opened, previewed := 0, 0
	p.SigOpenMatch(func(SearchMatch) { opened++ })
	p.SigPreviewReplacement(func(FileReplacement) { previewed++ })
	reject := func() {
		t.Helper()
		if p.CanNavigateMatches() || p.NavigateMatch(false) || p.NavigateMatch(true) || opened != 0 || previewed != 0 {
			t.Fatal("inactive navigation changed state")
		}
	}
	reject()
	p.Search("foo")
	reject()
	awaitSearchPanel(t, p, q)
	p.controls.replacement.OnTextInput("bar")
	p.PreviewReplacements()
	awaitSearchPanel(t, p, q)
	if !p.Snapshot().PreviewReady {
		t.Fatal("no preview")
	}
	reject()
	p.CancelSearch()
	reject()
	p.Search("foo")
	awaitSearchPanel(t, p, q)
	p.SetRootDir(t.TempDir())
	reject()
	p.CloseSearch()
	reject()
}

func BenchmarkGlobalSearchNavigateDense(b *testing.B) {
	p := NewGlobalSearchPanel()
	hits := make([]SearchMatch, 10000)
	for i := range hits {
		hits[i] = SearchMatch{FilePath: "dense.txt", Line: 1, Column: 2 * i, MatchLen: 1}
	}
	p.applyResults(hits)
	p.rebuildFlatRows()
	p.searchReady = true
	p.SetSize(240, 600)
	p.Layout()
	p.selectedIdx = len(p.flatRows) - 1
	p.SigOpenMatch(func(SearchMatch) {})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.NavigateMatch(i%2 == 0)
	}
}
