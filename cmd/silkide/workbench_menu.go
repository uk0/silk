package main

import (
	"reflect"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func nilWidget(view gui.IWidget) bool {
	if view == nil {
		return true
	}
	v := reflect.ValueOf(view)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// Rebuild only on open, not on every idle/paint. Each header owns exactly one
// popup tree; replacing it releases its native windows, including recent files.
func buildWorkbenchMenu(frame *gui.Frame, tabs *gui.TabWidget, canvas *ged.GedView) {
	frame.MainMenu().SetCompact(true)
	for index, group := range []string{"File", "Edit", "View", "Design", "Build", "Tools", "Help"} {
		_, header := frame.MainMenu().AddSubMenu(i18n.T(group), nil, nil)
		header.Action().SetObjName("menu." + group)
		header.SetSubPopupCallback(func(_ gui.IButton) {
			if old, ok := header.SubPopup().(*gui.Menu); ok {
				releaseWorkbenchMenu(old)
			}
			header.SetSubPopup(buildWorkbenchSubMenu(group, tabs, canvas))
		})
		accelerator := []int{'F', 'E', 'V', 'D', 'B', 'T', 'H'}[index]
		gui.SetToolTip(header, i18n.T(group)+" (Alt+"+string(rune(accelerator))+")")
		gui.RegisterShortcut(gui.ModAlt, accelerator, header.ShowSubPopup)
	}
}

func releaseWorkbenchMenu(menu *gui.Menu) {
	menu.Hide()
	for _, item := range menu.Items() {
		if button, ok := item.(*gui.Button); ok {
			if sub, ok := button.SubPopup().(*gui.Menu); ok {
				releaseWorkbenchMenu(sub)
			}
		}
	}
	menu.DetachWindow()
	menu.Detach()
}

func buildWorkbenchSubMenu(group string, tabs *gui.TabWidget, canvas *ged.GedView) *gui.Menu {
	if group == "File" {
		return buildFileMenu(tabs, canvas)
	}
	menu := gui.NewPopupMenu()
	key := actionKeyHint()
	design := designDocumentActive(tabs, canvas)
	document := design || documentMode(tabs, canvas) == modeEdit && activeEditor(tabs) != nil
	add := func(id, label, icon, hint string, enabled bool, run func()) {
		appendFileMenuCommand(menu, fileMenuCommand{id, label, icon, hint, run, enabled})
	}
	switch group {
	case "Edit":
		undo, redo := document, document
		if design {
			undo = canvas.GedScene().UndoStack().CanUndo()
			redo = canvas.GedScene().UndoStack().CanRedo()
		}
		add("edit.undo", "Undo", "edit-undo", key+"+Z", undo, func() { undoCurrentDocument(tabs, canvas, false) })
		add("edit.redo", "Redo", "edit-redo", key+"+Shift+Z", redo, func() { undoCurrentDocument(tabs, canvas, true) })
		menu.AddSeparator()
		ed := visibleCaretEditor(tabs, canvas)
		for _, spec := range editorCaretCommandSpecs() {
			add(spec.id, spec.label, spec.icon, spec.hint, ed != nil && spec.enabled(ed), func() { runEditorCaretCommand(tabs, canvas, spec, ed) })
		}
		menu.AddSeparator()
		add("search.files", "Find in Files", "search", key+"+Shift+F", globalSearch != nil, func() { showWorkspaceSearch(tabs, canvas) })
		matches := globalSearch != nil && globalSearch.CanNavigateMatches()
		add("search.next", "Next Search Result", "navigate-forward", "F4", matches, func() { navigateWorkspaceSearch(tabs, canvas, false) })
		add("search.previous", "Previous Search Result", "navigate-back", "Shift+F4", matches, func() { navigateWorkspaceSearch(tabs, canvas, true) })
		menu.AddSeparator()
		add("edit.palette", "Command Palette", "search", key+"+Shift+P", true, func() { showCommandPalette(canvas) })
		menu.AddSeparator()
		history := editorNavigation(tabs)
		add("navigate.back", "Go Back", "navigate-back", "Alt+Left", history != nil && history.CanGoBack(), func() { navigateEditorHistory(tabs, false) })
		add("navigate.forward", "Go Forward", "navigate-forward", "Alt+Right", history != nil && history.CanGoForward(), func() { navigateEditorHistory(tabs, true) })
		editor := activeEditor(tabs)
		semantic := !design && document && editor != nil && isGoFile(editor.FilePath())
		add("navigate.definition", "Go to Definition", "function", "F12", !design && document, func() { goToDefinitionViaLSP(tabs) })
		add("navigate.type", "Go to Type Definition", "function", key+"+F12", semantic, func() { requestSymbolNavigation(tabs, "Go to Type Definition") })
		add("navigate.implementation", "Go to Implementation", "function", "Alt+F12", semantic, func() { requestSymbolNavigation(tabs, "Go to Implementation") })
		add("navigate.references", "Find References", "search", "Shift+F12", !design && document, func() { findReferencesViaLSP(tabs) })
	case "View":
		for i, mode := range []workspaceMode{modeWelcome, modeEdit, modeDesign} {
			label := []string{"Welcome", "Edit", "Design"}[i]
			icon := []string{"silkide", "edit", "design"}[i]
			button := appendFileMenuCommand(menu, fileMenuCommand{"view." + string(mode), label, icon, key + "+" + []string{"1", "2", "3"}[i], func() {
				if w := globalWorkbench; w != nil {
					w.activate(mode)
				}
			}, globalWorkbench != nil})
			button.Action().SetChecked(documentMode(tabs, canvas) == mode)
		}
		menu.AddSeparator()
		for _, panel := range []struct {
			id, label, icon string
			dock            *gui.Dock
			view            gui.IWidget
		}{
			{"properties", "Properties", "propsheet", globalRightDock, globalPropSheet},
			{"objects", "Object Inspector", "tree-view", globalRightDock, globalInspector},
			{"outline", "Show Outline", "outline", globalRightDock, globalOutline},
			{"problems", "Show Problems", "warning", globalBottomDock, globalProblems},
			{"output", "Build Output", "build", globalBottomDock, globalBuildOutput},
			{"log", "Show Log", "log", globalBottomDock, globalLog},
		} {
			add("view."+panel.id, panel.label, panel.icon, "", panel.dock != nil && !nilWidget(panel.view), func() {
				mode := modeEdit
				if panel.id == "properties" || panel.id == "objects" {
					mode = modeDesign
				}
				revealToolView(tabs, canvas, panel.dock, panel.view, mode)
			})
		}
		addWorkbenchPanelsMenu(menu)
	case "Design":
		if design && canvas.Selection().Count() == 1 {
			if widget, ok := canvas.Selection().ItemList()[0].(*ged.FakeWidget); ok && len(ged.AvailableEvents(widget.WidgetFactoryName())) > 0 {
				menu.AddSubMenu(i18n.T("Event Handlers"), gui.LoadIcon("function"), canvas.EventHandlerMenu(widget))
				menu.AddSeparator()
			}
		}
		for i, spec := range designerActionSpecs(canvas) {
			if i == 4 || i == 5 {
				menu.AddSeparator()
			}
			add("design."+spec.icon, spec.label, spec.icon, "", design && spec.enabled(), func() {
				if designDocumentActive(tabs, canvas) {
					spec.run()
				}
			})
		}
		menu.AddSeparator()
		add("design.preview", "Preview", "preview", key+"+Shift+R", design, func() {
			if designDocumentActive(tabs, canvas) {
				previewDesign(canvas)
			}
		})
		add("design.export", "Export...", "document", "", design, func() {
			if designDocumentActive(tabs, canvas) {
				performExport(canvas)
			}
		})
	case "Build":
		add("build.run", "Run", "run", "F5", true, func() { runProjectInTerminal(canvas) })
		add("build.build", "Build", "build", "F6", true, func() { buildProject(canvas) })
		add("build.tests", "Run Tests", "test-explorer", "F7", true, func() { runProjectTests(canvas) })
		add("build.vet", "Run go vet", "warning", "Shift+F6", true, func() { runProjectVet(canvas) })
		menu.AddSeparator()
		add("build.debug", "Debug", "debug", "Shift+F5", true, func() { runProjectInDebugger(canvas) })
		add("build.stop", "Stop", "stop", "", true, stopProjectExecution)
	case "Tools":
		add("tools.status", "Toolchain Status", "settings", "", true, func() { showToolchainStatus(globalFrame) })
		add("tools.restartLSP", "Restart LSP", "refresh", "", true, func() { restartLSP(canvas) })
		add("tools.appearance", "Appearance & Editor", "settings", "", true, func() { showAppearanceDialog(globalFrame) })
		add("tools.expandMacro", "Expand Silk Macros", "build", "", canExpandMacro(tabs, canvas), func() { expandActiveMacro(tabs, canvas) })
		add("project.settings", "Project Settings", "settings", "", true, func() { showProjectSettingsDialog(canvas) })
		add("tools.packages", "Refresh Packages", "packages", "", true, func() { refreshPackages(canvas) })
		add("tools.a11y", "Dump A11y Tree", "tree-view", key+"+Shift+A", true, dumpA11yTree)
	case "Help":
		add("help.about", "About", "help", "", true, func() { showIDEAbout(canvas) })
	}
	return menu
}
