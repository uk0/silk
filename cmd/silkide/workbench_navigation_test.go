package main

import (
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func navigationWorkbench(t *testing.T) *workbench {
	t.Helper()
	w := testWorkbench(t)
	oldSearch, oldFiles, oldOutline := globalSearch, globalFileExplorer, globalOutline
	t.Cleanup(func() {
		globalSearch.CloseSearch()
		globalSearch, globalFileExplorer, globalOutline = oldSearch, oldFiles, oldOutline
	})
	globalSearch, globalFileExplorer, globalOutline = ged.NewGlobalSearchPanel(), ged.NewFileExplorer(), ged.NewCodeOutlinePanel()
	globalLeftDock = w.center.SplitNewDock(true, false).(*gui.Dock)
	globalRightDock = w.center.SplitNewDock(false, false).(*gui.Dock)
	globalLeftDock.AddView(globalFileExplorer)
	globalLeftDock.AddView(globalSearch)
	w.paletteView = gui.NewButton()
	globalLeftDock.AddView(w.paletteView)
	globalRightDock.AddView(globalOutline)
	w.activate(modeEdit)
	return w
}

func TestWorkbenchNavigationRetainsSelectedTools(t *testing.T) {
	w := navigationWorkbench(t)
	other := gui.NewButton()
	globalRightDock.AddView(other)
	dockSetActiveView(globalLeftDock, globalSearch)
	dockSetActiveView(globalRightDock, other)
	path := writeFile(t, t.TempDir(), "match.txt", "target\n")
	if !navigateEditor(w.tabs, path, func(ed *gui.CodeEditor) { ed.NavigateToLineCol(0, 2) }) {
		t.Fatal("open")
	}
	if globalLeftDock.ActiveView() != globalSearch || globalRightDock.ActiveView() != other {
		t.Fatal("opening a result reset the user's tool panels")
	}
}

func TestWorkbenchSearchShortcutRevealsFromWelcome(t *testing.T) {
	w := navigationWorkbench(t)
	globalLeftDock.SetVisible(false)
	w.activate(modeWelcome)
	registerShortcuts(w.tabs, w.canvas)
	gui.ShortcutHandler(gui.ModAction|gui.ModShift, 'F')()
	if documentMode(w.tabs, w.canvas) != modeEdit || !globalLeftDock.IsVisible() || globalLeftDock.ActiveView() != globalSearch {
		t.Fatal("search was focused inside a hidden welcome dock")
	}
}

func TestWorkbenchToolTabsSurviveModesAndManualHide(t *testing.T) {
	w := navigationWorkbench(t)
	dockSetActiveView(globalLeftDock, globalSearch)
	globalRightDock.SetVisible(false)
	w.activate(modeWelcome)
	w.activate(modeEdit)
	if globalLeftDock.ActiveView() != globalSearch || globalRightDock.IsVisible() {
		t.Fatal("welcome reset tools or visibility")
	}
	w.activate(modeDesign)
	if globalLeftDock.ActiveView() != w.paletteView {
		t.Fatal("initial design tools missing")
	}
	// Direct center-tab changes use exactly the same mode state as toolbar actions.
	w.center.SetActiveIndex(w.center.IndexOfView(w.tabs))
	w.syncMode()
	if globalLeftDock.ActiveView() != globalSearch || globalRightDock.IsVisible() {
		t.Fatal("return from design reset edit tools")
	}
	for i := 0; i < 10; i++ {
		w.syncMode()
		w.activate(modeEdit)
	}
	if globalLeftDock.ActiveView() != globalSearch || globalRightDock.IsVisible() {
		t.Fatal("idle/repeated activation reset tools")
	}
}

func TestWorkbenchExplicitToolEntrypoints(t *testing.T) {
	for _, entry := range []string{"palette-search", "menu-search", "shortcut-outline", "palette-outline", "menu-outline"} {
		t.Run(entry, func(t *testing.T) {
			w := navigationWorkbench(t)
			old := paletteCommands
			paletteCommands = nil
			t.Cleanup(func() { paletteCommands = old })
			globalLeftDock.SetVisible(false)
			globalRightDock.SetVisible(false)
			w.activate(modeWelcome)
			registerPaletteCommands(w.tabs, w.canvas)
			registerShortcuts(w.tabs, w.canvas)
			switch entry {
			case "menu-search":
				menuCommandButton(buildWorkbenchSubMenu("Edit", w.tabs, w.canvas), "search.files").Action().Trigger(nil)
			case "menu-outline":
				menuCommandButton(buildWorkbenchSubMenu("View", w.tabs, w.canvas), "view.outline").Action().Trigger(nil)
			case "shortcut-outline":
				gui.ShortcutHandler(gui.ModAction|gui.ModShift, 'O')()
			default:
				name := "Show Outline"
				if entry == "palette-search" {
					name = "Find in Files"
				}
				found := false
				for _, c := range paletteCommands {
					if c.Name == name {
						c.Fn()
						found = true
						break
					}
				}
				if !found {
					t.Fatal("missing command", name)
				}
			}
			if documentMode(w.tabs, w.canvas) != modeEdit {
				t.Fatal("wrong mode")
			}
			if entry == "palette-search" || entry == "menu-search" {
				if !globalLeftDock.IsVisible() || globalLeftDock.ActiveView() != globalSearch || globalRightDock.IsVisible() {
					t.Fatal("search reveal touched wrong dock")
				}
			} else if !globalRightDock.IsVisible() || globalRightDock.ActiveView() != globalOutline || globalLeftDock.IsVisible() {
				t.Fatal("outline reveal touched wrong dock")
			}
		})
	}
}

func TestWorkspaceSearchEmptyNavigationDoesNotChangeMode(t *testing.T) {
	w := navigationWorkbench(t)
	registerShortcuts(w.tabs, w.canvas)
	w.activate(modeWelcome)
	for _, mod := range []uint8{0, gui.ModShift} {
		gui.ShortcutHandler(mod, gui.KeyF4)()
		if documentMode(w.tabs, w.canvas) != modeWelcome {
			t.Fatal("empty result changed workspace")
		}
	}
	if menuCommandButton(buildWorkbenchSubMenu("Edit", w.tabs, w.canvas), "search.next").IsEnabled() {
		t.Fatal("empty next command enabled")
	}
}

func TestWorkspaceSearchResultFromDesignRetainsSearch(t *testing.T) {
	w := navigationWorkbench(t)
	dockSetActiveView(globalLeftDock, globalFileExplorer)
	w.activate(modeDesign)
	dockSetActiveView(globalLeftDock, globalSearch)
	path := writeFile(t, t.TempDir(), "match.txt", "中🙂 foo\n")
	openWorkspaceSearchMatch(globalSearch, w.tabs, w.canvas, ged.SearchMatch{FilePath: path, Line: 1, Column: 8, LineText: "中🙂 foo", MatchLen: 3})
	if documentMode(w.tabs, w.canvas) != modeEdit || globalLeftDock.ActiveView() != globalSearch || activeEditor(w.tabs).CursorCol() != 3 {
		t.Fatal("cross-mode search lost context or Unicode column")
	}
}
