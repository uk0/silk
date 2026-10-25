package main

import "github.com/uk0/silk/gui"

func editorNavigationAvailable(tabs *gui.TabWidget, forward bool) bool {
	history := editorNavigation(tabs)
	if history == nil {
		return false
	}
	if forward {
		return history.CanGoForward()
	}
	return history.CanGoBack()
}
func editorNavigationCount(tabs *gui.TabWidget) int {
	if history := editorNavigation(tabs); history != nil {
		return history.Len()
	}
	return 0
}
func clearEditorNavigation(tabs *gui.TabWidget) {
	if history := editorNavigation(tabs); history != nil {
		history.Clear()
	}
}

type workbenchNavigationActions struct {
	back, forward *gui.Button
	stop          func()
}

func (w *workbench) closeNavigationActions() {
	if a := w.navigationActions; a != nil {
		if a.stop != nil {
			a.stop()
		}
		gui.SetToolTip(a.back, "")
		gui.SetToolTip(a.forward, "")
	}
	w.navigationActions = nil
}
func (w *workbench) bindNavigationActions(back, forward *gui.Button) {
	w.closeNavigationActions()
	a := &workbenchNavigationActions{back: back, forward: forward}
	w.navigationActions = a
	a.stop = w.navigation.SubscribeChanged(func() {
		if globalWorkbench == w && w.navigationActions == a {
			w.syncNavigationActions()
		}
	})
	w.syncNavigationActions()
}
func (w *workbench) syncNavigationActions() {
	a := w.navigationActions
	if a == nil {
		return
	}
	for i, b := range []*gui.Button{a.back, a.forward} {
		enabled := editorNavigationAvailable(w.tabs, i == 1)
		if b.IsEnabled() != enabled || b.Action().IsEnabled() != enabled {
			b.SetEnabled(enabled)
			b.Action().SetEnabled(enabled)
			b.Update()
		}
	}
}
