package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

const ideVersion = "0.1.5"

func showIDEAbout(parent gui.IWidget) {
	ged.ShowAboutDialogWithInfo(parent, silkIDEAboutInfo())
}

func wireDesignerContextPanels(canvas *ged.GedView) {
	if canvas == nil {
		return
	}
	var code *ged.CodePanel
	ensureCode := func() *ged.CodePanel {
		if globalBottomDock == nil {
			return nil
		}
		if code == nil {
			code = ged.NewCodePanel()
			code.BindGedView(canvas)
			globalBottomDock.AddView(code)
		}
		return code
	}
	canvas.SetEventHandlerCallback(func(widget *ged.FakeWidget, event string) error {
		if w := globalWorkbench; w != nil && w.canvas == canvas && !designDocumentActive(w.tabs, canvas) {
			return fmt.Errorf("请先切换到当前设计文档")
		}
		panel := ensureCode()
		if panel == nil {
			return fmt.Errorf("code panel unavailable")
		}
		if err := panel.OpenEventHandler(widget, event); err != nil {
			return err
		}
		revealWorkbenchTool(globalBottomDock, panel)
		panel.Focus()
		return nil
	})
	canvas.SetContextPanelCallbacks(func() {
		revealWorkbenchTool(globalRightDock, globalPropSheet)
		if globalPropSheet != nil {
			globalPropSheet.SetFocus()
		}
	}, func() {
		if globalBottomDock == nil {
			return
		}
		ensureCode()
		code.ShowTab(1) // read-only generated Go; event source remains a separate tab
		code.RegenerateNow()
		revealWorkbenchTool(globalBottomDock, code)
	})
}

func performSaveAll(tabs *gui.TabWidget, canvas *ged.GedView) bool {
	if !saveAllDocuments(tabs, canvas) {
		silkideToast(i18n.T("Save All did not complete"), gui.ToastWarning)
		return false
	}
	silkideToast(i18n.T("All documents are saved"), gui.ToastSuccess)
	return true
}

func buildProjectSettingsDialog(parent gui.IWidget, root string) (*gui.Dialog, *ged.ProjectSettingsPanel) {
	dlg := gui.NewDialog(i18n.T("Project Settings"), parent)
	panel := ged.NewProjectSettingsPanel()
	// Without an explicit root SaveKits deliberately does not persist edits.
	panel.SetProjectRoot(root)
	panel.SigApply(func() {
		if err := panel.SaveError(); err != nil {
			reportBuildOutput("project settings: " + err.Error())
			gui.ShowToast(dlg, i18n.T("Project settings could not be saved"), 3500, gui.ToastError)
		} else {
			gui.ShowToast(dlg, i18n.T("Project settings saved"), 2000, gui.ToastSuccess)
		}
	})
	dlg.SetContent(panel)
	dlg.AddButton(i18n.T("Close"), gui.DialogOK)
	dlg.SetSize(640, 520)
	return dlg, panel
}

// All export entry points use identical cancellation, naming and feedback.
func performExport(canvas *ged.GedView) {
	if canvas == nil || canvas.GedScene() == nil {
		silkideToast(i18n.T("No design to export"), gui.ToastWarning)
		return
	}
	path := gui.SaveFileDialog()
	if path == "" {
		return
	}
	performExportTo(path, canvas)
}

func designExportPath(path string) string {
	if path == "" {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".svg" && ext != ".pdf" {
		return path + ".svg"
	}
	return path
}

func performExportTo(path string, canvas *ged.GedView) bool {
	if path == "" { // cancelled chooser is not a failure or a success
		return false
	}
	path = designExportPath(path)
	if err := exportDesignCanvas(path, canvas); err != nil {
		reportBuildOutput("export failed: " + err.Error())
		revealWorkbenchTool(globalBottomDock, globalBuildOutput)
		silkideToast(i18n.T("Export failed"), gui.ToastError)
		return false
	}
	reportBuildOutput("export: wrote " + path)
	silkideToast(i18n.Tf("Exported to %s", filepath.Base(path)), gui.ToastSuccess)
	return true
}

type fileMenuCommand struct {
	id, label, icon, hint string
	run                   func()
	enabled               bool
}

func appendFileMenuCommand(menu *gui.Menu, c fileMenuCommand) *gui.Button {
	b := menu.AddButton1(i18n.T(c.label), paint.LoadIcon(c.icon))
	b.Action().SetObjName(c.id)
	b.Action().BindFunc0(c.run)
	b.Action().SetEnabled(c.enabled)
	b.SetEnabled(c.enabled)
	b.SetShortcutText(c.hint)
	tip := i18n.T(c.label)
	if c.hint != "" {
		tip += " (" + c.hint + ")"
	}
	gui.SetToolTip(b, tip)
	return b
}

func buildHamburgerMenu(anchor *gui.Button, tabs *gui.TabWidget, canvas *ged.GedView) *gui.Menu {
	m := buildFileMenu(tabs, canvas)
	m.SetParent(anchor)
	m.AddSeparator()
	appendFileMenuCommand(m, fileMenuCommand{"tools.a11y", "Dump A11y Tree", "tree-view", actionKeyHint() + "+Shift+A", dumpA11yTree, true})
	appendFileMenuCommand(m, fileMenuCommand{"project.settings", "Project Settings", "settings", "", func() { showProjectSettingsDialog(canvas) }, true})
	m.AddSeparator()
	appendFileMenuCommand(m, fileMenuCommand{"help.about", "About", "help", "", func() { showIDEAbout(canvas) }, true})
	return m
}

func buildFileMenu(tabs *gui.TabWidget, canvas *ged.GedView) *gui.Menu {
	m := gui.NewPopupMenu()
	key := actionKeyHint()
	appendFileMenuCommand(m, fileMenuCommand{"file.new", "New", "plus", key + "+N", func() { newDesignCanvas(canvas) }, canvas != nil})
	appendFileMenuCommand(m, fileMenuCommand{"file.open", "Open", "folder", key + "+O", func() {
		if path := gui.OpenFileDialog(); path != "" {
			openFromTree(path, tabs, canvas, nil)
		}
	}, true})
	recent := buildRecentFilesMenu(tabs, canvas)
	m.AddSubMenu(i18n.T("Open Recent"), paint.LoadIcon("folder"), recent)
	appendFileMenuCommand(m, fileMenuCommand{"file.reopen", "Reopen Closed Editor", "edit-undo", key + "+Shift+T", func() { reopenClosedEditor(tabs) }, canReopenEditor(tabs)})
	m.AddSeparator()
	canSave := documentMode(tabs, canvas) == modeEdit && activeEditor(tabs) != nil ||
		documentMode(tabs, canvas) == modeDesign && canvas != nil
	appendFileMenuCommand(m, fileMenuCommand{"file.save", "Save", "save", key + "+S", func() { saveCurrentDocument(tabs, canvas) }, canSave})
	appendFileMenuCommand(m, fileMenuCommand{"file.saveAll", "Save All", "save", key + "+Shift+S", func() { performSaveAll(tabs, canvas) }, canvas != nil || activeEditor(tabs) != nil})
	m.AddSeparator()
	appendFileMenuCommand(m, fileMenuCommand{"file.closeAll", "Close All Editors", "close", "", func() { startEditorBatchClose(tabs, nil, closeAllEditors) }, activeEditorClose == nil && len(editorCloseTargets(tabs, nil, closeAllEditors)) > 0})
	appendFileMenuCommand(m, fileMenuCommand{"file.cancelClose", "Cancel Closing Editors", "stop", "", cancelEditorBatchClose, activeEditorClose != nil && !activeEditorClose.cancelled})
	m.AddSeparator()
	appendFileMenuCommand(m, fileMenuCommand{"file.export", "Export...", "document", "", func() { performExport(canvas) }, designDocumentActive(tabs, canvas)})
	appendFileMenuCommand(m, fileMenuCommand{"file.print", "Print Design...", "printer", "", func() { performPrintDesign(canvas) }, canPrintDesign(canvas)})
	return m
}

func designDocumentActive(tabs *gui.TabWidget, canvas *ged.GedView) bool {
	return canvas != nil && canvas.GedScene() != nil && documentMode(tabs, canvas) == modeDesign
}

func buildRecentFilesMenu(tabs *gui.TabWidget, canvas *ged.GedView) *gui.Menu {
	m := gui.NewPopupMenu()
	var recent []string
	if globalPrefs != nil {
		recent = globalPrefs.RecentFiles()
	}
	if len(recent) == 0 {
		m.AddButton1(i18n.T("(empty)"), nil).SetEnabled(false)
		return m
	}
	counts := make(map[string]int)
	for _, path := range recent {
		counts[filepath.Base(path)]++
	}
	for _, path := range recent {
		label := filepath.Base(path)
		if counts[label] > 1 {
			label = path
		}
		b := m.AddButton1(label, paint.LoadIcon("document"))
		gui.SetToolTip(b, path)
		b.Action().SetObjName("file.recent")
		b.Action().BindFunc0(func() { openFromTree(path, tabs, canvas, nil) })
	}
	return m
}

func showRecentFilesMenu(tabs *gui.TabWidget, canvas *ged.GedView) {
	if globalFrame == nil {
		return
	}
	m := buildRecentFilesMenu(tabs, canvas)
	m.SetParent(globalFrame)
	x, y := globalFrame.MapToGlobal(24, 64)
	m.ShowAsPopup(x, y, true)
}

func editorCommand(name string) bool {
	switch name {
	case "Refresh Editor Git Markers", "Show Diff vs Saved", "Diff vs HEAD", "Go to Definition", "Go to Type Definition", "Go to Implementation", "Go Back", "Go Forward", "Format Document", "Find References", "Code Actions", "Trim Trailing Whitespace", "Toggle Blame", "Add Bookmark", "Rename Symbol":
		return true
	}
	return false
}
