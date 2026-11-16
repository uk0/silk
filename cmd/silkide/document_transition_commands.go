package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func requestDesignTransition(key string, canvas *ged.GedView, done func(bool)) bool {
	w := globalWorkbench
	if w == nil || w.canvas != canvas || canvas == nil {
		return false
	}
	return transitionController(w).begin(key, canvas, nil, false, done)
}

func requestQuitDocuments(tabs *gui.TabWidget, canvas *ged.GedView, proceed func()) bool {
	w := globalWorkbench
	if w == nil || w.tabs != tabs || proceed == nil {
		return false
	}
	cancelOwnedDesignOpen(w.designOpen)
	var targets []transitionSource
	for _, path := range orderedEditorPaths(tabs) {
		if ed := openEditors[path]; ed != nil {
			targets = append(targets, transitionSource{editor: ed, path: path})
		}
	}
	return transitionController(w).begin("quit", canvas, targets, true, func(ok bool) {
		if ok {
			proceed()
		}
	})
}

// Native TabBar close callbacks return true only if a page is already removed.
// Admitted background inspection/save returns false; owned completion removes
// the captured page later, never the page currently occupying its old index.
func requestCloseEditorTab(tabs *gui.TabWidget, index int) bool {
	if tabs == nil || tabs.Stack() == nil || index < 0 || index >= tabs.Count() {
		return false
	}
	target := tabs.Stack().Page(index)
	ed, path := (*gui.CodeEditor)(nil), ""
	if e, ok := target.(*gui.CodeEditor); ok {
		ed = e
		_, path = resolveEditorTabTarget(tabs, e)
	}
	if path == "" {
		return detachEditorTab(tabs, target, ed, path)
	}
	w := globalWorkbench
	if w == nil || w.tabs != tabs {
		return false
	}
	focus := gui.CurrentFocusWidget()
	restoreFocus := focus == ed || focus == tabs.TabBar()
	detach := func() bool {
		before := gui.CurrentFocusWidget()
		canRestore := restoreFocus && (before == focus || before == ed)
		if detachEditorTab(tabs, target, ed, path) {
			syncDocumentWindowTitle(w.frame, w.canvas)
			current := gui.CurrentFocusWidget()
			if canRestore && (current == before || current == nil) {
				if next := activeEditor(tabs); next != nil {
					focusEditorTab(tabs, next)
				}
			}
			return true
		}
		return false
	}
	if cleanSourceCloseDuringPrompt(ed, path) {
		if detach() {
			noteTransitionSourceClosed(ed, path)
			return true
		}
		return false
	}
	transitionController(w).begin("close:"+workspaceSearchKey(path), nil, []transitionSource{{editor: ed, path: path}}, false, func(ok bool) {
		if ok {
			detach()
		}
	})
	return false
}
