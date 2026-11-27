package ged

import (
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// Layout ownership constrains a child's geometry, not the layout container's
// own geometry and not the child's normal selection/property editing.
func managedGeometryItem(item graph.IItem) bool {
	if item == nil {
		return false
	}
	parent, ok := item.Parent().(*FakeWidget)
	return ok && parent.layoutManaged && isDesignerLayoutFactory(parent.WidgetFactoryName())
}

func (view *GedView) selectionHasManagedGeometry() bool {
	sel := view.Selection()
	for _, item := range sel.ItemList() {
		if !managedGeometryItem(item) {
			continue
		}
		carried := false
		for parent := item.Parent(); parent != nil; parent = parent.Parent() {
			if sel.Contains(parent) {
				carried = true
				break
			}
		}
		if !carried {
			return true
		}
	}
	return false
}

func (view *GedView) managedGeometryAt(x, y float64) bool {
	sx, sy := view.MapToScene(x, y)
	if decor, _ := view.FindHandleAt(sx, sy); decor != nil {
		return managedGeometryItem(decor.Item())
	}
	hit := view.Scene().FindItemAt(sx, sy, nil)
	if managedGeometryItem(hit) {
		return true
	}
	// A drag of a mixed selection must not secretly move its layout-owned
	// members. Do not partially move the selection or create a rollback command.
	return hit != nil && view.Selection().Contains(hit) && view.selectionHasManagedGeometry()
}

func (view *GedView) showManagedGeometryHint(x, y float64) {
	hint := i18n.T(managedGeometryMessage)
	view.managedHint = paletteDropFeedback{active: true, x: x, y: y, hint: hint}
	if frame := gui.FindOwnerFrame(view); frame != nil && frame.StatusBar() != nil {
		frame.StatusBar().ShowMessage(hint)
	}
	gui.SetOverrideCursor(nil)
	view.Self().Update()
}

func (view *GedView) clearManagedGeometryHint() {
	if view.managedHint.active {
		view.managedHint = paletteDropFeedback{}
		view.Self().Update()
	}
}
