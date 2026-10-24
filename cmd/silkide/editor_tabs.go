package main

import "github.com/uk0/silk/gui"

func canNavigateEditors(tabs *gui.TabWidget) bool {
	return tabs != nil && globalWorkbench != nil && globalWorkbench.tabs == tabs && tabs.Count() > 1
}

// Menus/palette can reveal Edit from Welcome/Design; Ctrl+Tab is handled by
// the SDK's nearest TabWidget while a document or its child control has focus.
func navigateEditors(tabs *gui.TabWidget, previous bool) bool {
	if !canNavigateEditors(tabs) {
		return false
	}
	showEditorWorkspace(tabs)
	return tabs.NavigateTab(previous)
}
