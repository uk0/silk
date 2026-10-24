package main

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/prop"
	"testing"
)

func TestWorkbenchPropertiesPanelHasRegisteredIcon(t *testing.T) {
	sheet := prop.NewPropertySheet()
	if gui.DockViewIcon(sheet) == nil {
		t.Fatal("Properties implements Icon() returning nil; registered propsheet icon is lost")
	}
	custom := gui.LoadIcon("search")
	sheet.SetIcon(custom)
	if gui.DockViewIcon(sheet) != custom {
		t.Fatal("explicit host icon was overwritten")
	}
}

func TestWorkbenchPanelsPropertiesUsesDesignFromWelcome(t *testing.T) {
	w := navigationWorkbench(t)
	old := globalPropSheet
	t.Cleanup(func() { globalPropSheet = old })
	globalPropSheet = prop.NewPropertySheet()
	globalPropSheet.SetTitle("Properties")
	globalRightDock.AddView(globalPropSheet)
	w.activate(modeWelcome)
	menu := workbenchPanelsMenu()
	defer releaseWorkbenchMenu(menu)
	found := false
	for _, item := range menu.Items() {
		if b, ok := item.(*gui.Button); ok && b.Action().Text() == "Properties" {
			b.Action().Trigger(nil)
			found = true
			break
		}
	}
	if !found || documentMode(w.tabs, w.canvas) != modeDesign || globalRightDock.ActiveView() != globalPropSheet {
		t.Fatal("panel chooser Properties differs from View/Properties: not in Design mode")
	}
}
