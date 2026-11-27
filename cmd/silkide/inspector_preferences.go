package main

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/prop"
)

const inspectorSplitPreference = "appearance/inspectorColumnSplit"

// SilkIDE constructs its own docks instead of loading a GUI dock session.
// Restore the app-wide split here, and persist it via the same async settings
// store as window geometry. Dragging the splitter itself never writes to disk.
func newDesignerPropertySheet(frame *gui.Frame, prefs *preferences) *prop.PropertySheet {
	sheet := prop.NewPropertySheet()
	sheet.SetTitle(i18n.T("Properties"))
	if prefs == nil {
		return sheet
	}

	defaultSplit := sheet.ColumnSplit()
	sheet.SetColumnSplit(prefs.store.Float64(inspectorSplitPreference, defaultSplit))
	frame.AddClosingCallback(func(*gui.Frame) {
		current := sheet.ColumnSplit()
		if prefs.store.Float64(inspectorSplitPreference, defaultSplit) != current {
			if err := prefs.store.SetFloat64(inspectorSplitPreference, current); err == nil {
				_ = prefs.requestSave()
			}
		}
	})
	return sheet
}
