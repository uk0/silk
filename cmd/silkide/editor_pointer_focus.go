package main

import (
	"path/filepath"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func focusEditorAfterPointerTab(tabs *gui.TabWidget, bar *gui.TabBar) {
	if tabs == nil || bar != tabs.TabBar() || !bar.HasFocus() {
		return
	}
	if w := globalWorkbench; w != nil {
		if w.tabs != tabs || documentMode(tabs, w.canvas) != modeEdit {
			return
		}
	}
	ed := activeEditor(tabs)
	if ed == nil || ed.RootWidget() != tabs.RootWidget() {
		return
	}
	for p := gui.IWidget(ed); p != nil; p = p.Parent() {
		if !p.IsVisible() || !p.IsEnabled() {
			return
		}
	}
	ed.SetFocus()
}

// Tab identity changes update the title immediately. The existing low-rate
// canvas poll remains a fallback, but compares the actual frame title rather
// than a stale private cache that could overwrite an event-driven update.
func syncDocumentWindowTitle(frame *gui.Frame, canvas *ged.GedView) {
	if frame == nil {
		return
	}
	title := idTitle()
	if canvas != nil {
		title = composeTitle(canvas)
	}
	if w := globalWorkbench; w != nil && w.frame == frame && w.canvas == canvas && documentMode(w.tabs, canvas) == modeEdit {
		if ed := activeEditor(w.tabs); ed != nil {
			path := activeEditorPath(w.tabs)
			if path != "" {
				refreshEditorTab(w.tabs, ed, path)
				title = filepath.Base(path)
			} else {
				title = w.tabs.TabTitle(w.tabs.CurrentIndex())
			}
			if ed.Modified() {
				title += " *"
			}
			title += " — " + idTitle()
		}
	}
	if frame.Title() == title {
		return
	}
	frame.SetTitle(title)
	if win := frame.Window(); win != nil {
		win.SetTitle(title)
	}
}
