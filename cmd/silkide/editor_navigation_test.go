package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

func TestWorkbenchNavigationCrossFileHistory(t *testing.T) {
	w := testWorkbench(t)
	dir := t.TempDir()
	a := writeFile(t, dir, "a.go", "package p\nfunc a(){\n println(1)\n}\n")
	b := writeFile(t, dir, "b.go", "package p\n/*😀*/ var Target=1\n")
	openTestEditorReady(t, w.tabs, a)
	origin := activeTestEditorReady(t, w.tabs)
	origin.NavigateToLineCol(2, 4)
	navigateTestEditorLSP(t, w.tabs, b, core.LSPPosition{Line: 1, Character: 11})
	if activeTestEditorReady(t, w.tabs).CursorCol() != 10 || !w.navigation.CanGoBack() {
		t.Fatal("jump not recorded precisely")
	}
	activeTestEditorReady(t, w.tabs).ReplaceAllText("package p\n/*😀*/ var Target=2\n")
	activeTestEditorReady(t, w.tabs).NavigateToLineCol(1, 15)
	origin.FoldAll()
	activeTestEditorReady(t, w.tabs).NavGoBack()
	if activeTestEditorReady(t, w.tabs) != origin || origin.CursorLine() != 2 || origin.CursorCol() != 4 || origin.IsFolded(1) {
		t.Fatal("back lost source location")
	}
	origin.NavGoForward()
	if activeTestEditorPathReady(t, w.tabs) != b || activeTestEditorReady(t, w.tabs).CursorCol() != 15 || !activeTestEditorReady(t, w.tabs).Modified() {
		t.Fatal("forward lost live location or unsaved buffer")
	}
	navigateTestEditorHistory(t, w.tabs, false)
	if openTestEditorReady(t, w.tabs, filepath.Join(dir, "missing.go")) {
		t.Fatal("unexpected missing file")
	}
	if !w.navigation.CanGoForward() {
		t.Fatal("failed file open polluted history")
	}
	navigateTestEditorAt(t, w.tabs, a, 2, 3)
	if w.navigation.CanGoForward() {
		t.Fatal("new jump did not branch history")
	}
}

func TestWorkbenchNavigationClosedFileRenameAndMenu(t *testing.T) {
	w := testWorkbench(t)
	dir := t.TempDir()
	a := writeFile(t, dir, "a.go", "package p\nvar A=1\n")
	b := writeFile(t, dir, "b.go", "package p\nvar B=2\n")
	openTestEditorReady(t, w.tabs, a)
	activeTestEditorReady(t, w.tabs).NavigateToLineCol(1, 4)
	navigateTestEditorAt(t, w.tabs, b, 2, 4)
	old := openEditors[a]
	old.CancelGitStatus()
	delete(openEditors, a)
	w.tabs.RemoveTab(0)
	next := filepath.Join(dir, "renamed.go")
	if err := os.Rename(a, next); err != nil {
		t.Fatal(err)
	}
	migrateRenamedMetadata(&editorRename{old: a, next: next}, w.tabs, w.canvas)
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	back := menuCommandButton(menu, "navigate.back")
	if back == nil || !back.IsEnabled() {
		t.Fatal("back menu disabled")
	}
	back.Action().Trigger(nil)
	pumpTestEditorOpens(t, w.tabs)
	if activeTestEditorPathReady(t, w.tabs) != next || activeTestEditorReady(t, w.tabs).CursorCol() != 4 {
		t.Fatal("history did not reopen renamed file")
	}
	for _, id := range []string{"navigate.definition", "navigate.type", "navigate.implementation", "navigate.references"} {
		if b := menuCommandButton(menu, id); b == nil || !b.IsEnabled() {
			t.Fatal("missing semantic action", id)
		}
	}
}

func TestNavigationQueueBoundsAndSupersedes(t *testing.T) {
	posts := make(chan func(), 2)
	started := make(chan struct{})
	release := make(chan struct{})
	q := navigationQueue{post: func(f func()) { posts <- f }}
	delivered := 0
	q.submit(navigationJob{run: func(ctx context.Context) ([]core.LSPLocation, error) {
		close(started)
		<-release
		return nil, ctx.Err()
	}, deliver: func([]core.LSPLocation, error) { t.Error("superseded result delivered") }})
	<-started
	runs := 0
	for range 100 {
		q.submit(navigationJob{run: func(context.Context) ([]core.LSPLocation, error) { runs++; return nil, nil }, deliver: func([]core.LSPLocation, error) { delivered++ }})
	}
	close(release)
	for range 2 {
		select {
		case f := <-posts:
			f()
		case <-time.After(3 * time.Second):
			t.Fatal("queue stalled")
		}
	}
	if runs != 1 || delivered != 1 || q.running || q.pending != nil {
		t.Fatalf("unbounded queue: runs=%d delivered=%d", runs, delivered)
	}
	q.submit(navigationJob{run: func(context.Context) ([]core.LSPLocation, error) { return nil, nil }, deliver: func([]core.LSPLocation, error) { t.Error("canceled result delivered") }})
	q.stop()
	select {
	case f := <-posts:
		f()
	case <-time.After(3 * time.Second):
		t.Fatal("cancel stalled")
	}
}

func TestNavigationSnapshotRejectsEditsCaretSwitchAndClosedOwner(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.go", "package p\nvar A=1\n")
	openTestEditorReady(t, w.tabs, path)
	ed := activeTestEditorReady(t, w.tabs)
	s := navigationSnapshot{tabs: w.tabs, editor: ed, client: globalLSP, document: lspDocuments[path], path: path, revision: ed.TextRevision(), line: ed.CursorLine(), column: ed.CursorCol()}
	if !s.current() {
		t.Fatal("fresh snapshot stale")
	}
	ed.NavigateToLineCol(1, 4)
	if s.current() {
		t.Fatal("caret move accepted")
	}
	ed.NavigateToLineCol(s.line, s.column)
	ed.ReplaceAllText(ed.Text() + "\n")
	if s.current() {
		t.Fatal("edit accepted")
	}
	s.revision = ed.TextRevision()
	s.line, s.column = ed.CursorLine(), ed.CursorCol()
	w.activate(modeDesign)
	if s.current() {
		t.Fatal("hidden editor accepted")
	}
	w.activate(modeEdit)
	delete(openEditors, path)
	if s.current() {
		t.Fatal("closed owner accepted")
	}
	openEditors[path] = ed
}

func TestNavigationTargetsRejectInvalidAndDeduplicate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "unicode 空间.go")
	loc := core.LSPLocation{URI: fileURIOf(p), Range: core.LSPRange{Start: core.LSPPosition{Line: 1, Character: 2}}}
	bad := loc
	bad.Range.Start.Line = -1
	got := navigableLocations([]core.LSPLocation{loc, loc, bad, {URI: "https://example.invalid/source"}, {}})
	if len(got) != 1 || got[0] != loc {
		t.Fatalf("bad targets: %+v", got)
	}
}
