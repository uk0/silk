package ged

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/uk0/silk/filesearch"
	"github.com/uk0/silk/gui"
)

func searchPanelTest(t *testing.T) (*GlobalSearchPanel, chan func()) {
	t.Helper()
	p := NewGlobalSearchPanel()
	q := make(chan func(), 8)
	p.post = func(fn func()) { q <- fn }
	p.SetSize(320, 600)
	p.SetRootDir(t.TempDir())
	t.Cleanup(func() { p.CloseSearch() })
	return p, q
}
func awaitSearchPanel(t *testing.T, p *GlobalSearchPanel, q chan func()) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for p.Snapshot().Pending || p.IsApplying() {
		select {
		case fn := <-q:
			fn()
		case <-timer.C:
			t.Fatal("search did not settle", p.Snapshot())
		}
	}
}
func TestGlobalSearchNativeStateAndPreview(t *testing.T) {
	p, q := searchPanelTest(t)
	path := filepath.Join(p.rootDir, "a.txt")
	writeFile(t, path, "中 foo_12\r\nfoo_34\n")
	p.SetRegex(true)
	p.Search(`foo_(?P<id>\d+)`)
	awaitSearchPanel(t, p, q)
	if s := p.Snapshot(); !s.Ready || !s.Complete || s.Matches != 2 {
		t.Fatal(s)
	}
	p.controls.replacement.OnTextInput("${id}_中")
	p.PreviewReplacements()
	awaitSearchPanel(t, p, q)
	if !p.Snapshot().PreviewReady || len(p.preview) != 1 || p.preview[0].After != "中 12_中\r\n34_中\n" {
		t.Fatal(p.Snapshot(), p.preview)
	}
	assertFileEquals(t, path, "中 foo_12\r\nfoo_34\n")
	opened := false
	p.SigPreviewReplacement(func(r FileReplacement) { opened = r.Path == path })
	p.activateRow(0)
	if !opened {
		t.Fatal("preview did not open")
	}
	released := false
	notified := 0
	p.SetReplacementGuard(func([]FileReplacement) (func(error), error) {
		return func(err error) {
			if err != nil {
				t.Fatal(err)
			}
			released = true
		}, nil
	})
	p.SigFileReplaced(func(path, text string) { notified++ })
	p.ApplyPreview()
	if !p.IsApplying() || p.controls.query.IsEnabled() {
		t.Fatal("write not reserved")
	}
	p.CancelSearch()
	p.CloseSearch() // owner must wait, not orphan the active transaction
	if !p.IsApplying() || p.closed {
		t.Fatal("transaction cancelled mid-write")
	}
	awaitSearchPanel(t, p, q)
	assertFileEquals(t, path, "中 12_中\r\n34_中\n")
	if !released || notified != 1 || p.Snapshot().Matches != 0 || !p.controls.query.IsEnabled() {
		t.Fatal(p.Snapshot(), released, notified)
	}
}
func TestGlobalSearchPreviewConflictAndInvalidation(t *testing.T) {
	p, q := searchPanelTest(t)
	a := filepath.Join(p.rootDir, "a.txt")
	b := filepath.Join(p.rootDir, "b.txt")
	writeFile(t, a, "foo")
	writeFile(t, b, "foo")
	p.Search("foo")
	awaitSearchPanel(t, p, q)
	p.controls.replacement.OnTextInput("bar")
	p.PreviewReplacements()
	awaitSearchPanel(t, p, q)
	writeFile(t, b, "external edit")
	released := false
	p.SetReplacementGuard(func([]FileReplacement) (func(error), error) { return func(err error) { released = err != nil }, nil })
	p.ApplyPreview()
	awaitSearchPanel(t, p, q)
	if p.Snapshot().Err == nil || !released || p.Snapshot().PreviewReady {
		t.Fatal(p.Snapshot())
	}
	assertFileEquals(t, a, "foo")
	assertFileEquals(t, b, "external edit")
	p.Search("foo")
	awaitSearchPanel(t, p, q)
	p.PreviewReplacements()
	awaitSearchPanel(t, p, q)
	p.controls.replacement.SelectAll()
	p.controls.replacement.OnTextInput("baz")
	if p.Snapshot().PreviewReady || p.controls.apply.IsEnabled() {
		t.Fatal("stale replacement preview")
	}
	p.controls.query.SelectAll()
	p.controls.query.OnTextInput("中")
	if p.Snapshot().Ready || len(p.results) != 0 || p.query != "中" {
		t.Fatal("standard selected edit failed", p.Snapshot())
	}
}
func TestGlobalSearchRejectsOldRootOptionsAndEmptyRequests(t *testing.T) {
	p, q := searchPanelTest(t)
	writeFile(t, filepath.Join(p.rootDir, "a.txt"), "foo")
	p.Search("foo")
	var fn func()
	select {
	case fn = <-q:
	case <-time.After(5 * time.Second):
		t.Fatal("no result")
	}
	p.SetCaseSensitive(true)
	fn()
	if p.Snapshot().Ready || p.Snapshot().Matches != 0 {
		t.Fatal("old options accepted")
	}
	p.Search("foo")
	select {
	case fn = <-q:
	case <-time.After(5 * time.Second):
		t.Fatal("no result")
	}
	p.SetRootDir(t.TempDir())
	fn()
	if p.Snapshot().Ready {
		t.Fatal("old root accepted")
	}
	p.Search("foo")
	p.Search("")
	if p.Snapshot().Pending || p.Snapshot().Ready {
		t.Fatal("empty query pending")
	}
	p.SetRootDir("")
	if p.rootDir != "" {
		t.Fatal("empty root became cwd")
	}
	p.Search("foo")
	if p.Snapshot().Pending {
		t.Fatal("empty root searched cwd")
	}
}
func TestGlobalSearchErrorsAndPartialNeverApply(t *testing.T) {
	p, q := searchPanelTest(t)
	writeFile(t, filepath.Join(p.rootDir, "a.txt"), strings.Repeat("x", 300000)+"\nfoo")
	p.SetRegex(true)
	p.Search("[")
	awaitSearchPanel(t, p, q)
	if p.Snapshot().Err == nil || p.Snapshot().Ready || p.controls.preview.IsEnabled() {
		t.Fatal(p.Snapshot())
	}
	p.SetRegex(false)
	p.Search("foo")
	awaitSearchPanel(t, p, q)
	if !p.Snapshot().Ready || p.Snapshot().Complete || p.controls.preview.IsEnabled() || !strings.Contains(p.statusText(), "partial") {
		t.Fatal(p.Snapshot())
	}
	p.PreviewReplacements()
	if p.Snapshot().Pending {
		t.Fatal("preview partial search")
	}
}
func TestGlobalSearchControlsLayoutNavigationAndSnippet(t *testing.T) {
	p, _ := searchPanelTest(t)
	p.SetInclude([]string{".go"})
	p.SetExclude([]string{"*_test.go"})
	for _, width := range []float64{200, 240, 320, 600, 1200} {
		p.SetSize(width, 600)
		p.Layout()
		c := p.controls
		if c.include.Text() != ".go" || c.exclude.Text() != "*_test.go" {
			t.Fatal("filters not reflected")
		}
		children := []gui.IWidget{c.title, c.query, c.search, c.stop, c.replacement, c.preview, c.apply, c.regex, c.matchCase, c.wholeWord, c.typesLabel, c.include, c.excludeLabel, c.exclude, c.status}
		for i, a := range children {
			r := struct{ X, Y, W, H float64 }{a.X(), a.Y(), a.Width(), a.Height()}
			if r.X < 0 || r.Y < 0 || r.X+r.W > width || r.Y+r.H > p.listTop() {
				t.Fatalf("width %.0f child %d %+v", width, i, r)
			}
			for j, b := range children[:i] {
				s := struct{ X, Y, W, H float64 }{b.X(), b.Y(), b.Width(), b.Height()}
				if r.X < s.X+s.W && s.X < r.X+r.W && r.Y < s.Y+s.H && s.Y < r.Y+r.H {
					t.Fatalf("overlap %.0f %d %d %+v %+v", width, i, j, r, s)
				}
			}
		}
	}
	text := strings.Repeat("中🙂", 300) + "\tneedle more"
	col := strings.Index(text, "needle")
	snippet, start, end := searchSnippet(text, col, 6)
	if !utf8.ValidString(snippet) || snippet[start:end] != "needle" || utf8.RuneCountInString(snippet) > 140 {
		t.Fatal(snippet, start, end)
	}
	p.applyResults([]SearchMatch{{FilePath: "a", Line: 1, Column: col, LineText: text, MatchLen: 6}})
	p.rebuildFlatRows()
	var opened SearchMatch
	p.SigOpenMatch(func(m SearchMatch) { opened = m })
	p.OnKeyDown(gui.KeyDown, false)
	p.OnKeyDown(gui.KeyDown, false)
	p.OnKeyDown(gui.KeyEnter, false)
	if opened.Column != col || opened.LineText != text {
		t.Fatal("keyboard match navigation", opened)
	}
}
func TestGlobalSearchWorkerLatestOnly(t *testing.T) {
	posts := make(chan func(), 8)
	started := make(chan globalSearchRequest, 8)
	unblock := make(chan struct{})
	var got []uint64
	w := newGlobalSearchWorker(func(f func()) { posts <- f }, func(r globalSearchResult) { got = append(got, r.request.generation) })
	w.debounce = 0
	w.compute = func(ctx context.Context, r globalSearchRequest) globalSearchResult {
		started <- r
		if r.generation == 1 {
			<-unblock
		}
		return globalSearchResult{request: r}
	}
	w.request(globalSearchRequest{generation: 1})
	<-started
	for i := uint64(2); i <= 500; i++ {
		w.request(globalSearchRequest{generation: i})
	}
	close(unblock)
	select {
	case r := <-started:
		if r.generation != 500 {
			t.Fatal(r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("latest did not run")
	}
	select {
	case f := <-posts:
		f()
	case <-time.After(5 * time.Second):
		t.Fatal("no publish")
	}
	w.close()
	if len(got) != 1 || got[0] != 500 {
		t.Fatal(got)
	}
	select {
	case <-started:
		t.Fatal("unbounded queue")
	default:
	}
}
func TestComputeGlobalSearchPartialPreview(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a"), "aaa")
	r := computeGlobalSearch(context.Background(), globalSearchRequest{root: root, query: "a", replacement: "b", preview: true, options: filesearch.ContextOptions{Limits: filesearch.SearchLimits{Matches: 1}}})
	if !errors.Is(r.err, filesearch.ErrSearchLimit) || len(r.replacements) > 0 {
		t.Fatal(r)
	}
}
