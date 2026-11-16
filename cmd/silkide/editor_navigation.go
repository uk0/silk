package main

import (
	"path/filepath"

	"github.com/uk0/silk/gui"
)

func editorNavigation(tabs *gui.TabWidget) *gui.NavigationStack {
	if w := globalWorkbench; w != nil && w.tabs == tabs {
		return &w.navigation
	}
	return nil
}

func editorNavigationPosition(tabs *gui.TabWidget) gui.NavPosition {
	if ed := activeEditor(tabs); ed != nil {
		return ed.NavigationPosition()
	}
	if tabs != nil {
		if p, ok := tabs.Stack().CurrentPage().(*restorePage); ok && p.manual && p.origin.FilePath != "" {
			return p.origin
		}
	}
	if h := editorNavigation(tabs); h != nil {
		if p, ok := h.CurrentPosition(); ok {
			return p
		}
	}
	return gui.NavPosition{}
}

// A true result admits a request; history is recorded only by the owned ready
// callback after real target installation and location resolution.
func navigateEditor(tabs *gui.TabWidget, path string, locate func(*gui.CodeEditor)) bool {
	return navigateEditorReady(tabs, path, func(ed *gui.CodeEditor) bool { locate(ed); return true })
}
func navigateEditorReady(tabs *gui.TabWidget, path string, locate func(*gui.CodeEditor) bool) bool {
	c := editorOpenerFor(tabs)
	if c == nil {
		return false
	}
	from, stamp := editorNavigationPosition(tabs), c.historyStamp
	h := &c.owner.navigation
	intent := c.newIntent()
	return c.open(intent, path, editorOpenOptions{recent: true, ready: func(ed *gui.CodeEditor) bool {
		if !c.current(intent, ed) || c.historyStamp != stamp || !locate(ed) {
			return false
		}
		if !c.current(intent, ed) || c.historyStamp != stamp {
			return false
		}
		if from.FilePath != "" {
			h.RecordJump(from, ed.NavigationPosition())
		}
		return true
	}})
}
func navigateEditorHistory(tabs *gui.TabWidget, forward bool) {
	c := editorOpenerFor(tabs)
	if c == nil {
		return
	}
	h := &c.owner.navigation
	visits := h.PrepareNavigation(editorNavigationPosition(tabs), forward)
	if len(visits) == 0 {
		return
	}
	intent := c.newIntent()
	var attempt func(int)
	attempt = func(start int) {
		for i := start; i < len(visits); i++ {
			visit := visits[i]
			if !c.valid() || c.intent != intent || !h.CanCommitNavigation(visit) {
				return
			}
			pos := visit.Destination()
			if pos.FilePath == "" {
				continue
			}
			next := i + 1
			if c.open(intent, pos.FilePath, editorOpenOptions{recent: true, ready: func(ed *gui.CodeEditor) bool {
				if !c.current(intent, ed) || !h.CanCommitNavigation(visit) {
					return false
				}
				ed.RestoreNavigationPosition(pos)
				return c.current(intent, ed) && h.CommitNavigation(visit)
			}, failed: func(error) { attempt(next) }}) {
				return
			}
		}
	}
	attempt(0)
}

func configureEditorNavigation(tabs *gui.TabWidget, ed *gui.CodeEditor) {
	ed.SetNavigatePositionCallback(func(pos gui.NavPosition) bool {
		if openEditors[ed.FilePath()] != ed {
			return false
		}
		focusEditorTab(tabs, ed)
		return navigateEditor(tabs, pos.FilePath, func(target *gui.CodeEditor) {
			target.NavigateToLineCol(pos.Line, pos.Column)
		})
	})
	if editorNavigation(tabs) != nil {
		bind := func(forward bool) func() {
			return func() {
				if openEditors[ed.FilePath()] == ed {
					focusEditorTab(tabs, ed)
					navigateEditorHistory(tabs, forward)
				}
			}
		}
		ed.SetNavigationHistoryActions(bind(false), bind(true))
	}
}

func canonicalEditorPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return filepath.Clean(abs)
}
