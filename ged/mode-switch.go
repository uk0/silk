package ged

import (
	"silk/gui"
)

// ModeConfig holds references to dock panels for each mode.
// Panels may be nil if they haven't been created yet.
type ModeConfig struct {
	// Design mode panels (docks)
	WidgetListDock gui.IWidget // dock containing widget palette
	DesignDock     gui.IWidget // dock containing GedView canvas
	PropertyDock   gui.IWidget // dock containing property sheet + code panel

	// Edit mode panels (docks) - reserved for future use
	FileExplorerDock gui.IWidget // dock containing file browser
	EditorDock       gui.IWidget // dock containing multi-tab editor
	BuildOutputDock  gui.IWidget // dock containing build output

	// Shared panels visible in all modes
	Shared []gui.IWidget
}

// GlobalModeConfig is the application-wide mode configuration.
var GlobalModeConfig ModeConfig

// OnDesignMode is called when switching to design mode.
// Set by the application to control which tabs are active.
var OnDesignMode func()

// OnEditMode is called when switching to code/edit mode.
var OnEditMode func()

// SwitchToDesignMode activates design mode — widget palette + canvas + properties.
func SwitchToDesignMode() {
	// All docks stay visible (they share tabs); we just switch active tabs
	if OnDesignMode != nil {
		OnDesignMode()
	}
	relayoutFrame()
}

// SwitchToEditMode activates code mode — file explorer + editor + build output.
func SwitchToEditMode() {
	if OnEditMode != nil {
		OnEditMode()
	}
	relayoutFrame()
}

// setWidgetVisible safely sets visibility on a possibly-nil widget.
func setWidgetVisible(w gui.IWidget, visible bool) {
	if w == nil {
		return
	}
	w.SetVisible(visible)
}

// relayoutFrame triggers a layout pass on the default frame.
func relayoutFrame() {
	f := gui.DefaultFrame()
	if f != nil {
		f.Layout()
	}
}
