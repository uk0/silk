package main

import (
	"path/filepath"
	"sort"
	"strings"

	"silk/ged"
	"silk/gui"
	"silk/i18n"
)

// paletteCommand is one entry in the Command Palette. Name is the
// translatable string the user types against; Hint shows the
// accelerator (e.g. "F5", "Cmd+S") in a dim sub-label, matching
// JetBrains' "Find Action" affordance. Fn executes when Enter or
// double-click selects the entry.
//
// The Hint is purely informational — actually firing the keystroke
// triggers the same Fn through gui.RegisterShortcut, so the palette
// entry doubles as keyboard documentation when the user types
// "build" and sees "Build (F6)".
type paletteCommand struct {
	Name string
	Hint string
	Fn   func()
}

// paletteCommands is populated by registerPaletteCommands at IDE
// startup. The order here is the default order shown in the palette
// when the search field is empty — most-used actions go first so
// power users can find them with just arrow keys.
var paletteCommands []paletteCommand

// registerPaletteCommands builds the canonical action list. Called
// once from main() after the toolbar + shortcuts are wired so each
// entry can reference the existing handler closure rather than
// re-implementing the action body.
//
// Every silkide action that has a stable user-facing name should
// land here. Actions that aren't safe to run from the palette (e.g.
// modal dialogs that the palette dialog itself can't dismiss
// cleanly) should be left out.
func registerPaletteCommands(editorTabs *gui.TabWidget, designCanvas *ged.GedView) {
	add := func(name, hint string, fn func()) {
		paletteCommands = append(paletteCommands, paletteCommand{
			Name: i18n.T(name),
			Hint: hint,
			Fn:   fn,
		})
	}

	// File actions.
	add("New", "Cmd+N", func() { newDesignCanvas(designCanvas) })
	add("Open", "Cmd+O", func() {
		path := gui.OpenFileDialog()
		if path == "" {
			return
		}
		openFromTree(path, editorTabs, designCanvas, nil)
	})
	// "Open Recent..." reopens the top entry of the MRU — the fast path
	// for "get me back into what I was just editing" without walking the
	// hamburger submenu. No-op when the MRU is empty.
	add("Open Recent...", "", func() {
		if globalPrefs == nil {
			return
		}
		recent := globalPrefs.RecentFiles()
		if len(recent) == 0 {
			return
		}
		openFromTree(recent[0], editorTabs, designCanvas, nil)
	})
	add("Save", "Cmd+S", func() {
		performSave(designCanvas)
	})
	add("Quick Open File", "Cmd+P", func() {
		showFileFinder(designCanvas, projectDir(designCanvas), editorTabs)
	})

	// Run / Build / Test / Export. Run Tests carries two
	// accelerators (F7 + Cmd+Shift+T); the hint surfaces F7 to match
	// the canonical JetBrains label and Cmd+Shift+T stays
	// discoverable through the shortcut wiring in prefs.go.
	add("Run", "F5", func() { runProjectInTerminal(designCanvas) })
	add("Build", "F6", func() { buildProject(designCanvas) })
	add("Run go vet", "Shift+F6", func() { runProjectVet(designCanvas) })
	add("Run Tests", "F7", func() { runProjectTests(designCanvas) })
	add("Run with Coverage", "Cmd+Shift+F7", func() { runProjectWithCoverage(designCanvas) })
	// "Show Coverage" — same handler as Run with Coverage. Discoverable
	// under both names: "coverage" types as either intent and lands at
	// the only thing that actually computes the gutter stripes.
	add("Show Coverage", "", func() { runProjectWithCoverage(designCanvas) })
	// "Show Test Results" — flip the bottom dock to the structured
	// TestResultsPanel. Mirrors the existing Show Problems / Show
	// Outline shape — same dock-flip + SetFocus idiom.
	add("Show Test Results", "", func() {
		if globalTestResults != nil {
			dockSetActiveView(globalBottomDock, globalTestResults)
			globalTestResults.SetFocus()
		}
	})
	add("Export...", "", func() {
		if designCanvas == nil {
			return
		}
		path := gui.SaveFileDialog()
		if path == "" {
			return
		}
		if err := exportDesignCanvas(path, designCanvas); err != nil {
			// exportDesignCanvas already logs via core.Warn; we also
			// push the message into the BuildOutput pane so the user
			// sees something even when the terminal is hidden, and a
			// transient toast for immediate visual feedback (the pane
			// only auto-focuses on real build errors, not export ones).
			reportBuildOutput("export failed: " + err.Error())
			silkideToast(i18n.T("Export failed"), gui.ToastError)
			return
		}
		silkideToast(i18n.Tf("Exported to %s", filepath.Base(path)), gui.ToastSuccess)
	})

	// Edit / canvas actions.
	add("Undo", "Cmd+Z", func() {
		if designCanvas == nil {
			return
		}
		if scene := designCanvas.GedScene(); scene != nil {
			if stack := scene.UndoStack(); stack != nil && stack.CanUndo() {
				stack.Undo()
				designCanvas.Update()
			}
		}
	})
	add("Redo", "Cmd+Shift+Z", func() {
		if designCanvas == nil {
			return
		}
		if scene := designCanvas.GedScene(); scene != nil {
			if stack := scene.UndoStack(); stack != nil && stack.CanRedo() {
				stack.Redo()
				designCanvas.Update()
			}
		}
	})
	add("Refresh", "Cmd+R", func() {
		if designCanvas != nil {
			designCanvas.Update()
		}
	})
	add("Fit to View", "F", func() { fitCanvasToView(designCanvas) })

	// Navigation.
	add("Find in Files", "Cmd+Shift+F", func() {
		if globalSearch != nil {
			dockSetActiveView(globalLeftDock, globalSearch)
			globalSearch.SetFocus()
		}
	})
	add("Show Outline", "Cmd+Shift+O", func() {
		if globalOutline != nil {
			dockSetActiveView(globalRightDock, globalOutline)
			globalOutline.SetFocus()
		}
	})
	// Show Problems — flip the bottom dock to the structured
	// compiler-issues pane. Mirrors "Show Outline" — same dock-flip +
	// SetFocus idiom, just targets globalProblems in the bottom dock.
	add("Show Problems", "", func() {
		if globalProblems != nil {
			dockSetActiveView(globalBottomDock, globalProblems)
			globalProblems.SetFocus()
		}
	})
	// Show Bookmarks — flip the right dock to the cross-file
	// bookmarks pane (sibling of Inspector + Outline).
	add("Show Bookmarks", "", func() {
		if globalBookmarks != nil {
			dockSetActiveView(globalRightDock, globalBookmarks)
			globalBookmarks.SetFocus()
		}
	})
	// Add Bookmark — bookmark the active editor's cursor line. Cmd+F2
	// because plain F2 is now Rename Symbol (JetBrains muscle memory).
	// Same helper the shortcut calls so the entry point doesn't matter.
	add("Add Bookmark", "Cmd+F2", func() {
		addBookmarkAtCursor(editorTabs)
	})
	// Rename Symbol — pop the rename input box for the identifier under
	// the active editor's cursor. The handler is shared with the F2
	// shortcut so this entry doubles as keyboard documentation.
	add("Rename Symbol", "F2", func() {
		renameSymbolAtActiveEditor(editorTabs)
	})

	// Run configuration. No accelerator: rarely-edited, so we don't
	// pollute the keyboard map. Pops an input box, persists the args
	// string into prefs; the next Run picks it up.
	add("Configure Run...", "", configureRunArgs)

	// Surfaces.
	add("Project Settings", "", func() { showProjectSettingsDialog(designCanvas) })
	add("Dump A11y Tree", "Cmd+Shift+A", dumpA11yTree)
	add("About", "", func() { ged.ShowAboutDialog(designCanvas) })
}

// filterCommands returns the subset of paletteCommands whose
// translated Name contains every rune of `query` in order
// (subsequence match — same shape as VSCode's command palette
// filter). Empty query passes everything through unchanged.
//
// Subsequence > substring because a user typing "fb" in a hurry
// should match "Find in Files" and "Build" alike — the shorter
// query matches more entries and the user disambiguates with one
// extra character.
func filterCommands(cmds []paletteCommand, query string) []paletteCommand {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		out := make([]paletteCommand, len(cmds))
		copy(out, cmds)
		return out
	}
	out := make([]paletteCommand, 0, len(cmds))
	for _, c := range cmds {
		if subsequenceMatch(strings.ToLower(c.Name), q) {
			out = append(out, c)
		}
	}
	// Sort so shorter names rank higher — matches user's intuition
	// that "Run" should beat "Run Without Building" for the query
	// "run".
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].Name) < len(out[j].Name)
	})
	return out
}

// dismissDialog ends a Dialog's modal loop with DialogOK. Dialog
// itself only dismisses through its registered button bar (via
// onBtnClick → Window.EndModal); the palette runs its action when
// the user presses Enter or double-clicks the list, so we drive
// the same EndModal call directly.
func dismissDialog(dlg *gui.Dialog) {
	if dlg == nil {
		return
	}
	if win := dlg.Window(); win != nil {
		win.EndModal(gui.DialogOK)
	}
}

// subsequenceMatch reports whether `query`'s runes appear in `text`
// in order, possibly with other runes between. Both args must
// already be lower-cased by the caller.
func subsequenceMatch(text, query string) bool {
	qi := 0
	qr := []rune(query)
	if len(qr) == 0 {
		return true
	}
	for _, tr := range text {
		if tr == qr[qi] {
			qi++
			if qi == len(qr) {
				return true
			}
		}
	}
	return false
}

// showCommandPalette opens the modal command-palette dialog
// anchored at `parent`. Search field on top, filtered list below;
// Enter on the list (or double-click) runs the selected command's
// Fn after the dialog dismisses.
//
// Built fresh per show so the entry list and selection state stay
// independent across invocations. The dialog handles its own input
// focus so the user can type immediately after Cmd+Shift+P.
func showCommandPalette(parent gui.IWidget) {
	if parent == nil {
		return
	}
	if len(paletteCommands) == 0 {
		// registerPaletteCommands hasn't fired yet (e.g. test path
		// invoking the dialog before main()'s wiring). Treat as a
		// no-op so the user gets nothing instead of an empty popup.
		return
	}

	dlg := gui.NewDialog(i18n.T("Command Palette"), parent)
	box := gui.NewVBox()
	box.SetSpacing(6)

	input := gui.NewEdit()
	box.AddWidget(input)

	list := gui.NewListWidget()
	list.SetSelectionVisible(true)
	box.AddWidget(list)

	repopulate := func(query string) {
		list.Clear()
		for _, c := range filterCommands(paletteCommands, query) {
			label := c.Name
			if c.Hint != "" {
				label = c.Name + "    " + c.Hint
			}
			list.Append(gui.ListItem{Text: label, Data: c})
		}
		if list.Count() > 0 {
			list.SetSelectionVisible(true)
		}
	}
	repopulate("")

	input.SigTextChanged(func(_ interface{}, q string) { repopulate(q) })

	// Enter on input runs the first match — common power-user move.
	input.SigSubmit(func(_ interface{}, _ string) {
		if list.Count() == 0 {
			return
		}
		if cmd, ok := list.Item(0).Data.(paletteCommand); ok {
			dismissDialog(dlg)
			cmd.Fn()
		}
	})

	// Enter / double-click on the list runs whatever is selected.
	list.SigSubmit(func(o interface{}) {
		idx := list.ActiveIndex()
		if idx < 0 || idx >= list.Count() {
			return
		}
		if cmd, ok := list.Item(idx).Data.(paletteCommand); ok {
			dismissDialog(dlg)
			cmd.Fn()
		}
	})

	dlg.SetContent(box)
	dlg.AddButton(i18n.T("Cancel"), gui.DialogCancel)
	dlg.SetSize(520, 420)
	input.SetFocus()
	dlg.ShowModal()
}
