package main

import (
	"sort"
	"strconv"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// Keep a complete recovery route even for tools without a dedicated shortcut.
// Use configured instances, never factory-created replacements lacking callbacks.
func workbenchPanelsMenu() *gui.Menu {
	menu := gui.NewPopupMenu()
	w := globalWorkbench
	if w == nil || w.frame == nil {
		return menu
	}
	type entry struct {
		title string
		view  gui.IWidget
		dock  *gui.Dock
	}
	var entries []entry
	for _, candidate := range w.frame.AllDocks() {
		dock, ok := candidate.(*gui.Dock)
		if !ok || dock.IsMainDock() {
			continue
		}
		for _, view := range dock.AvailableViews() {
			title := gui.DockViewTitle(view)
			if title == "" {
				continue
			}
			entries = append(entries, entry{title, view, dock})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].title < entries[j].title })
	for i, panel := range entries {
		button := appendFileMenuCommand(menu, fileMenuCommand{
			"view.panel." + strconv.Itoa(i), panel.title, "window", "", func() {
				if revealToolView(w.tabs, w.canvas, panel.dock, panel.view, modeEdit) {
					panel.view.SetFocus()
				}
			}, true,
		})
		button.Action().SetIcon(gui.DockViewIcon(panel.view))
		button.Action().SetChecked(panel.dock.IsVisible() && panel.dock.IndexOfView(panel.view) >= 0)
	}
	return menu
}

func revealWorkbenchTool(dock *gui.Dock, view gui.IWidget) {
	if w := globalWorkbench; w != nil {
		revealToolView(w.tabs, w.canvas, dock, view, modeEdit)
	} else if dock != nil && !nilWidget(view) {
		dock.RevealView(view)
	}
}

func addWorkbenchPanelsMenu(menu *gui.Menu) {
	menu.AddSeparator()
	menu.AddSubMenu(i18n.T("All Panels"), gui.LoadIcon("window"), workbenchPanelsMenu())
}
