package main

import (
	"path/filepath"
	"sort"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

type workspaceMode string

const (
	modeWelcome workspaceMode = "welcome"
	modeEdit    workspaceMode = "edit"
	modeDesign  workspaceMode = "design"
)

// The active dock view is the authority, even when the user switches its tabs
// directly. Never infer the save target from the presence of a hidden editor.
type workbench struct {
	frame          *gui.Frame
	center         *gui.Dock
	tabs           *gui.TabWidget
	canvas         *ged.GedView
	welcome        *ged.WelcomeScreen
	palette        *ged.WidgetList
	paletteView    gui.IWidget
	buttons        map[workspaceMode]*gui.Button
	lastMode       workspaceMode
	designToolbar  *gui.ToolBar
	designActions  []designActionButton
	dockVisibility map[*gui.Dock]bool
	navigation     gui.NavigationStack
	toolViews      map[workspaceMode]workbenchToolViews
}

type workbenchToolViews struct{ left, right gui.IWidget }

var globalWorkbench *workbench

func documentMode(tabs *gui.TabWidget, canvas *ged.GedView) workspaceMode {
	var views []gui.IWidget
	if tabs != nil {
		views = append(views, tabs)
	}
	if canvas != nil {
		views = append(views, canvas)
	}
	for _, view := range views {
		if dock, ok := view.Parent().(*gui.Dock); ok {
			switch dock.ActiveView() {
			case tabs:
				return modeEdit
			case canvas:
				return modeDesign
			default:
				return modeWelcome
			}
		}
	}
	// Unattached widgets are used by headless hosts and tests.
	if activeEditor(tabs) != nil {
		return modeEdit
	}
	return modeDesign
}

func (w *workbench) activate(mode workspaceMode) {
	if w == nil || w.center == nil {
		return
	}
	var view gui.IWidget
	switch mode {
	case modeWelcome:
		view = w.welcome
		if w.welcome == nil {
			return
		}
		if globalPrefs != nil {
			w.welcome.SetRecentFiles(globalPrefs.RecentFiles())
		}
	case modeEdit:
		view = w.tabs
	case modeDesign:
		view = w.canvas
	default:
		return
	}
	dockSetActiveView(w.center, view)
	w.syncMode()
	if mode == modeEdit {
		if ed := activeEditor(w.tabs); ed != nil {
			ed.SetFocus()
		}
	} else if mode == modeDesign && w.canvas != nil {
		w.canvas.SetFocus()
	}
}

func (w *workbench) syncMode() {
	if w == nil {
		return
	}
	mode := documentMode(w.tabs, w.canvas)
	for m, button := range w.buttons {
		if button.Action().IsChecked() != (m == mode) {
			button.Action().SetChecked(m == mode)
			button.Update()
		}
	}
	w.syncDesignActions(mode)
	// Welcome temporarily hides tool docks. Do not force them open on every
	// heartbeat: manual dock visibility must survive both redraws and Welcome.
	if mode == w.lastMode {
		return
	}
	// Only a mode transition may restore mode-specific tools. Opening another
	// editor, following a search hit or repainting must keep the chosen tabs.
	if w.toolViews == nil {
		w.toolViews = make(map[workspaceMode]workbenchToolViews)
	}
	if w.lastMode == modeEdit || w.lastMode == modeDesign {
		var previous workbenchToolViews
		if globalLeftDock != nil {
			previous.left = globalLeftDock.ActiveView()
		}
		if globalRightDock != nil {
			previous.right = globalRightDock.ActiveView()
		}
		w.toolViews[w.lastMode] = previous
	}
	previousMode := w.lastMode
	w.lastMode = mode
	if mode == modeEdit || mode == modeDesign {
		views, saved := w.toolViews[mode]
		if !saved {
			if mode == modeEdit {
				views = workbenchToolViews{globalFileExplorer, globalOutline}
			} else {
				palette := w.paletteView
				if nilWidget(palette) {
					palette = w.palette
				}
				views = workbenchToolViews{palette, globalPropSheet}
			}
		}
		dockSetActiveView(globalLeftDock, views.left)
		dockSetActiveView(globalRightDock, views.right)
	}
	changed := false
	if mode == modeWelcome {
		w.dockVisibility = make(map[*gui.Dock]bool)
		for _, dock := range []*gui.Dock{globalLeftDock, globalRightDock, globalBottomDock} {
			if dock == nil {
				continue
			}
			w.dockVisibility[dock] = dock.IsVisible()
			if dock.IsVisible() {
				dock.SetVisible(false)
				changed = true
			}
		}
	} else if previousMode == modeWelcome {
		for dock, visible := range w.dockVisibility {
			if dock.IsVisible() != visible {
				dock.SetVisible(visible)
				changed = true
			}
		}
		w.dockVisibility = nil
	}
	if changed && w.frame != nil {
		w.frame.Layout()
	}
}

// revealToolView is for explicit user requests, not background refreshes. A
// selected tab inside a hidden dock is not visible and must not receive focus.
func revealToolView(tabs *gui.TabWidget, canvas *ged.GedView, dock *gui.Dock, view gui.IWidget, preferred workspaceMode) bool {
	if dock == nil || nilWidget(view) || dock.IndexOfView(view) < 0 {
		return false
	}
	if w := globalWorkbench; w != nil && w.tabs == tabs && w.canvas == canvas && documentMode(tabs, canvas) == modeWelcome {
		w.activate(preferred)
	}
	dockSetActiveView(dock, view)
	if !dock.IsVisible() {
		dock.SetVisible(true)
		if frame := dock.Frame(); frame != nil {
			frame.Layout()
		}
	}
	return true
}

func showWorkspaceSearch(tabs *gui.TabWidget, canvas *ged.GedView) {
	if revealToolView(tabs, canvas, globalLeftDock, globalSearch, modeEdit) {
		globalSearch.FocusSearch()
	}
}

func addWorkspaceActions(tb *gui.ToolBar) {
	w := globalWorkbench
	if w == nil {
		return
	}
	w.buttons = make(map[workspaceMode]*gui.Button)
	for i, mode := range []workspaceMode{modeWelcome, modeEdit, modeDesign} {
		label := []string{"Welcome", "Edit", "Design"}[i]
		button := tb.AddAction(i18n.T(label), nil, func() { w.activate(mode) })
		gui.SetToolTip(button, i18n.T(label)+" ("+actionKeyHint()+"+"+[]string{"1", "2", "3"}[i]+")")
		w.buttons[mode] = button
	}
	tb.AddSeparator()
	w.syncMode()
}

func showEditorWorkspace(tabs *gui.TabWidget) {
	if w := globalWorkbench; w != nil && w.tabs == tabs {
		w.activate(modeEdit)
	} else if tabs != nil {
		if dock, ok := tabs.Parent().(*gui.Dock); ok {
			dockSetActiveView(dock, tabs)
		}
	}
}

func showDesignWorkspace(canvas *ged.GedView) {
	if w := globalWorkbench; w != nil && w.canvas == canvas {
		w.activate(modeDesign)
	} else if canvas != nil {
		if dock, ok := canvas.Parent().(*gui.Dock); ok {
			dockSetActiveView(dock, canvas)
		}
	}
}

func saveCurrentDocument(tabs *gui.TabWidget, canvas *ged.GedView) bool {
	switch documentMode(tabs, canvas) {
	case modeEdit:
		// Failure is NOT permission to save another document.
		return saveActiveEditorToDisk(tabs)
	case modeDesign:
		return performSave(canvas)
	}
	return false
}

func undoCurrentDocument(tabs *gui.TabWidget, canvas *ged.GedView, redo bool) {
	switch documentMode(tabs, canvas) {
	case modeEdit:
		if ed := activeEditor(tabs); ed != nil {
			if redo {
				ed.Redo()
			} else {
				ed.Undo()
			}
		}
	case modeDesign:
		if canvas == nil || canvas.GedScene() == nil {
			return
		}
		if redo {
			canvas.Redo()
		} else {
			canvas.Undo()
		}
	}
	if w := globalWorkbench; w != nil {
		w.syncMode()
	}
}

// Preserve visible tab order. Sorted leftovers keep the legacy unattached
// document hosts deterministic without discarding any tracked buffer.
func orderedEditorPaths(tabs *gui.TabWidget) []string {
	byEditor := make(map[*gui.CodeEditor]string, len(openEditors))
	for path, ed := range openEditors {
		byEditor[ed] = path
	}
	var paths []string
	seen := make(map[string]bool)
	if tabs != nil && tabs.Stack() != nil {
		for i := 0; i < tabs.Count(); i++ {
			ed, _ := tabs.Stack().Page(i).(*gui.CodeEditor)
			if path := byEditor[ed]; path != "" && !seen[path] {
				paths = append(paths, path)
				seen[path] = true
			}
		}
	}
	var remaining []string
	for path := range openEditors {
		if !seen[path] {
			remaining = append(remaining, path)
		}
	}
	sort.Strings(remaining)
	return append(paths, remaining...)
}

func saveAllDocuments(tabs *gui.TabWidget, canvas *ged.GedView) bool {
	for _, path := range orderedEditorPaths(tabs) {
		ed := openEditors[path]
		if ed != nil && editorDiffersFromDisk(ed, path) && !saveEditorToDisk(ed, path) {
			return false
		}
	}
	if canvas != nil && canvas.GedScene() != nil {
		if !canvas.GedScene().IsClean() {
			return performSave(canvas)
		}
	}
	return true
}

func refreshEditorTab(tabs *gui.TabWidget, ed *gui.CodeEditor, path string) {
	if tabs == nil || tabs.Stack() == nil {
		return
	}
	title := filepath.Base(path)
	if ed.Modified() {
		title += " *"
	}
	for i := 0; i < tabs.Count(); i++ {
		if tabs.Stack().Page(i) == gui.IWidget(ed) {
			tabs.SetTabTitle(i, title)
			return
		}
	}
}

// Preview is a distinct action from export; all entrypoints select the design
// workspace before launching the generated, driver-disabled runtime form.
func previewDesign(canvas *ged.GedView) {
	if canvas == nil || ged.PreviewCallback == nil {
		return
	}
	showDesignWorkspace(canvas)
	ged.PreviewCallback()
}
