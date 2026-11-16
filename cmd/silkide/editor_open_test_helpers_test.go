package main

import (
	"context"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

var testEditorOpenPosts = map[*workbench]chan func(){}

func installTestEditorOpener(t *testing.T, w *workbench) {
	t.Helper()
	posts := make(chan func(), 512)
	testEditorOpenPosts[w] = posts
	w.opens = newEditorOpenController(w, editorOpenSettings{post: func(f func()) { posts <- f }})
	owner := w.opens
	t.Cleanup(func() {
		owner.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := owner.jobs.Wait(ctx); err != nil {
			t.Error(err)
		}
		delete(testEditorOpenPosts, w)
	})
}
func pumpTestEditorOpens(t *testing.T, tabs *gui.TabWidget) {
	t.Helper()
	w := globalWorkbench
	if w == nil || w.tabs != tabs || w.opens == nil {
		return
	}
	posts := testEditorOpenPosts[w]
	if posts == nil {
		t.Fatal("missing owned test dispatcher")
	}
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	for w.opens.Busy() {
		select {
		case f := <-posts:
			f()
		case <-timer.C:
			t.Fatal("manual open completion timeout")
		}
	}
}

// Some SDK/coverage fixtures supply a bare TabWidget. Give that fixture an
// explicit workbench owner; do not add a synchronous/unowned product fallback.
func ownTestEditorTabs(t *testing.T, tabs *gui.TabWidget) {
	t.Helper()
	if tabs == nil {
		return
	}
	if w := globalWorkbench; w != nil && w.tabs == tabs {
		if testEditorOpenPosts[w] == nil {
			installTestEditorOpener(t, w)
		}
		return
	}
	old := globalWorkbench
	frame := gui.NewFrame()
	dock := frame.SuggestDocDock().(*gui.Dock)
	w := &workbench{frame: frame, center: dock, tabs: tabs}
	dock.AddView(tabs)
	globalWorkbench = w
	t.Cleanup(func() { w.closeHistoryActions(); w.closeNavigationActions(); globalWorkbench = old })
	installTestEditorOpener(t, w)
}

// Fixture calls exercise the real asynchronous entrypoint and drive its UI
// dispatcher to readiness. They are not a synchronous branch in product code.
func openTestEditorReady(t *testing.T, tabs *gui.TabWidget, path string) bool {
	t.Helper()
	ownTestEditorTabs(t, tabs)
	accepted := openFileInEditor(tabs, path)
	if !accepted {
		return false
	}
	pumpTestEditorOpens(t, tabs)
	return trackedOpenEditor(tabs, canonicalEditorPath(path)) != nil
}
func loadTestEditorReady(t *testing.T, tabs *gui.TabWidget, path string) bool {
	t.Helper()
	ownTestEditorTabs(t, tabs)
	accepted := loadFileInEditor(tabs, path)
	if !accepted {
		return false
	}
	pumpTestEditorOpens(t, tabs)
	return trackedOpenEditor(tabs, canonicalEditorPath(path)) != nil
}

func openTestFileFinderReady(t *testing.T, tabs *gui.TabWidget, path, location string) bool {
	t.Helper()
	ownTestEditorTabs(t, tabs)
	if !openFileFinderSelection(tabs, path, location) {
		return false
	}
	pumpTestEditorOpens(t, tabs)
	ed := trackedOpenEditor(tabs, canonicalEditorPath(path))
	if ed == nil {
		return false
	}
	if location != "" {
		_, err := ed.ResolveGoToLocation(location)
		return err == nil
	}
	return true
}
func navigateTestEditorAt(t *testing.T, tabs *gui.TabWidget, path string, line, col int) {
	t.Helper()
	openFileInEditorAt(tabs, path, line, col)
	pumpTestEditorOpens(t, tabs)
}
func navigateTestEditorLSP(t *testing.T, tabs *gui.TabWidget, path string, pos core.LSPPosition) {
	t.Helper()
	openFileInEditorAtLSP(tabs, path, pos)
	pumpTestEditorOpens(t, tabs)
}
func navigateTestEditorHistory(t *testing.T, tabs *gui.TabWidget, forward bool) {
	t.Helper()
	navigateEditorHistory(tabs, forward)
	pumpTestEditorOpens(t, tabs)
}
func reopenTestEditorReady(t *testing.T, tabs *gui.TabWidget) bool {
	t.Helper()
	h := closedEditors(tabs)
	if h == nil || len(h.items) == 0 {
		return reopenClosedEditor(tabs)
	}
	path := h.items[len(h.items)-1].Path
	if !reopenClosedEditor(tabs) {
		return false
	}
	pumpTestEditorOpens(t, tabs)
	return trackedOpenEditor(tabs, path) != nil
}

func openTestDocumentReady(t *testing.T, path string, tabs *gui.TabWidget, canvas *ged.GedView, center *gui.Dock) {
	t.Helper()
	openFromTree(path, tabs, canvas, center)
	pumpTestEditorOpens(t, tabs)
	pumpTestDesignOpens(t, globalWorkbench)
}
func openTestDocumentOptionsReady(t *testing.T, path string, tabs *gui.TabWidget, canvas *ged.GedView, center *gui.Dock, recent bool) {
	t.Helper()
	openDocument(path, tabs, canvas, center, recent)
	pumpTestEditorOpens(t, tabs)
	pumpTestDesignOpens(t, globalWorkbench)
}

// Legacy UI tests assert completed document effects. These named getters await
// only the real manual-open dispatcher; asynchronous ownership tests above use
// activeEditor directly and explicitly assert the pending state before draining.
func activeTestEditorReady(t *testing.T, tabs *gui.TabWidget) *gui.CodeEditor {
	t.Helper()
	pumpTestEditorOpens(t, tabs)
	return activeEditor(tabs)
}
func activeTestEditorPathReady(t *testing.T, tabs *gui.TabWidget) string {
	t.Helper()
	pumpTestEditorOpens(t, tabs)
	return activeEditorPath(tabs)
}
