package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"reflect"
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
		add("edit.palette", "Command Palette", "search", key+"+Shift+P", true, func() { showCommandPalette(canvas) })
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
				if globalWorkbench != nil && documentMode(tabs, canvas) == modeWelcome {
					globalWorkbench.activate(modeDesign)
				}
				dockSetActiveView(panel.dock, panel.view)
			})
		}
	case "Design":
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
		add("project.settings", "Project Settings", "settings", "", true, func() { showProjectSettingsDialog(canvas) })
		add("tools.packages", "Refresh Packages", "packages", "", true, func() { refreshPackages(canvas) })
		add("tools.a11y", "Dump A11y Tree", "tree-view", key+"+Shift+A", true, dumpA11yTree)
	case "Help":
		add("help.about", "About", "help", "", true, func() { showIDEAbout(canvas) })
	}
	return menu
}
