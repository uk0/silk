package main

import (
	"bytes"
	"github.com/uk0/silk/diagnostic"
	"os"
	"path/filepath"
	"strings"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/macro"
)

var macroBusy bool // UI-thread-owned; serializes explicit expansion requests.
func canExpandMacro(tabs *gui.TabWidget, canvas *ged.GedView) bool {
	return !macroBusy && activeFileRename == nil && documentMode(tabs, canvas) == modeEdit && activeEditor(tabs) != nil && strings.EqualFold(filepath.Ext(activeEditorPath(tabs)), ".silkgo")
}
func macroOutputPath(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + "_gen.go"
}
func writeMacroOutput(tabs *gui.TabWidget, path string, data []byte) error {
	output := macroOutputPath(path)
	if fileRenameAffects(path) || fileRenameAffects(output) {
		return renameBusyError(path, "rename is still running")
	}
	if target := openEditors[output]; target != nil {
		disk, err := os.ReadFile(output)
		if err != nil {
			return diagnostic.New(diagnostic.FileRead, "cannot read generated output", err).At(diagnostic.Location{File: output})
		}
		if target.Text() != string(disk) {
			return diagnostic.New(diagnostic.MacroUnsaved, "generated output has unsaved edits", nil).At(diagnostic.Location{File: output})
		}
	}
	if err := macro.WriteFile(output, data); err != nil {
		return err
	}
	if target := openEditors[output]; target != nil {
		target.ReplaceAllText(string(data))
		target.MarkSaved()
		refreshEditorTab(tabs, target, output)
	}
	return nil
}
func expandActiveMacro(tabs *gui.TabWidget, canvas *ged.GedView) {
	if rejectDuringRename() {
		return
	}
	if !canExpandMacro(tabs, canvas) {
		silkideToast(diagnostic.New(diagnostic.MacroUnsaved, i18n.T("Open a saved .silkgo document first"), nil).Error(), gui.ToastWarning)
		return
	}
	path := activeEditorPath(tabs)
	editor := activeEditor(tabs)
	source := editor.Text()
	disk, err := os.ReadFile(path)
	if err != nil || string(disk) != source {
		silkideToast(diagnostic.New(diagnostic.MacroUnsaved, i18n.T("Save the macro source before expanding"), err).At(diagnostic.Location{File: path}).Error(), gui.ToastWarning)
		return
	}
	macroBusy = true
	go func() {
		generated, err := macro.Expand(path, []byte(source))
		gui.Post(func() {
			defer func() { macroBusy = false }()
			// Do not install stale results after edits, close, or external replacement.
			if openEditors[path] != editor || editor.Text() != source {
				return
			}
			current, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(current, disk) {
				silkideToast(diagnostic.New(diagnostic.MacroStale, i18n.T("Macro source changed; expand again"), readErr).Error(), gui.ToastWarning)
				return
			}
			if err == nil {
				err = writeMacroOutput(tabs, path, generated)
			}
			if err != nil {
				reportBuildOutput("macro: " + err.Error())
				dockSetActiveView(globalBottomDock, globalBuildOutput)
				silkideToast("["+string(diagnostic.CodeOf(err))+"] "+i18n.T("Macro expansion failed; output unchanged"), gui.ToastError)
				return
			}
			output := macroOutputPath(path)
			reportBuildOutput("macro: wrote " + output)
			openFileInEditor(tabs, output)
			silkideToast(i18n.T("Macro expanded to standard Go"), gui.ToastSuccess)
		})
	}()
}
