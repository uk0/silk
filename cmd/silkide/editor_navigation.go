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
	return gui.NavPosition{}
}

// Every explicit location jump goes through here, including diagnostics,
// references, outline and LSP. File open must succeed before history changes.
func navigateEditor(tabs *gui.TabWidget, path string, locate func(*gui.CodeEditor)) bool {
	from := editorNavigationPosition(tabs)
	if !openFileInEditor(tabs, path) {
		return false
	}
	ed := activeEditor(tabs)
	if ed == nil {
		return false
	}
	locate(ed)
	if history := editorNavigation(tabs); history != nil && from.FilePath != "" {
		history.RecordJump(from, ed.NavigationPosition())
	}
	return true
}

func navigateEditorHistory(tabs *gui.TabWidget, forward bool) {
	history := editorNavigation(tabs)
	if history == nil {
		return
	}
	live := editorNavigationPosition(tabs)
	history.Navigate(live, forward, func(pos gui.NavPosition) bool {
		if pos.FilePath == "" || !openFileInEditor(tabs, pos.FilePath) {
			return false
		}
		activeEditor(tabs).RestoreNavigationPosition(pos)
		return true
	})
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
