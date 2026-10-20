package main

import (
	"bytes"
	"fmt"
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
	return !macroBusy && documentMode(tabs, canvas) == modeEdit && activeEditor(tabs) != nil && strings.EqualFold(filepath.Ext(activeEditorPath(tabs)), ".silkgo")
}
func macroOutputPath(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + "_gen.go"
}
func writeMacroOutput(tabs *gui.TabWidget, path string, data []byte) error {
	output := macroOutputPath(path)
	if target := openEditors[output]; target != nil {
		disk, err := os.ReadFile(output)
		if err != nil {
			return err
		}
		if target.Text() != string(disk) {
			return fmt.Errorf("generated output has unsaved edits: %s", output)
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
	if !canExpandMacro(tabs, canvas) {
		silkideToast(i18n.T("Open a saved .silkgo document first"), gui.ToastWarning)
		return
	}
	path := activeEditorPath(tabs)
	editor := activeEditor(tabs)
	source := editor.Text()
	disk, err := os.ReadFile(path)
	if err != nil || string(disk) != source {
		silkideToast(i18n.T("Save the macro source before expanding"), gui.ToastWarning)
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
				silkideToast(i18n.T("Macro source changed; expand again"), gui.ToastWarning)
				return
			}
			if err == nil {
				err = writeMacroOutput(tabs, path, generated)
			}
			if err != nil {
				reportBuildOutput("macro: " + err.Error())
				dockSetActiveView(globalBottomDock, globalBuildOutput)
				silkideToast(i18n.T("Macro expansion failed; output unchanged"), gui.ToastError)
				return
			}
			output := macroOutputPath(path)
			reportBuildOutput("macro: wrote " + output)
			openFileInEditor(tabs, output)
			silkideToast(i18n.T("Macro expanded to standard Go"), gui.ToastSuccess)
		})
	}()
}
