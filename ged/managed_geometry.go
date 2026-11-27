package ged

import (
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// Layout ownership constrains geometry, not selection. A regular container
// can move on its own; a form's root layout instead follows the form.
func managedGeometryItem(item graph.IItem) bool {
	if item == nil {
		return false
	}
	if scene, ok := item.Parent().(*GedScene); ok {
		return scene.activeFormLayout() == item
	}
	parent, ok := item.Parent().(*FakeWidget)
	return ok && (parent.activeContentLayout() == item || parent.layoutManaged && isDesignerLayoutFactory(parent.WidgetFactoryName()))
}

func (view *GedView) selectionHasManagedGeometry() bool {
	sel := view.Selection()
	for item := range sel.Items {
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

// CanResizeSelection is the shared paint, hit-test and commit guard for
// a mixed selection. A layout-owned sibling is never resized indirectly.
func (view *GedView) CanResizeSelection() bool {
	return !view.selectionHasManagedGeometry()
}

// CanResizeItem leaves resize authority with the selected top-level item.
// Any selected child of that item follows the owner's normal layout/reflow.
func (view *GedView) CanResizeItem(item graph.IItem) bool {
	if item == nil || view.Scene() == nil || item.Scene() != view.Scene() || managedGeometryItem(item) {
		return false
	}
	for parent := item.Parent(); parent != nil; parent = parent.Parent() {
		if view.Selection().Contains(parent) {
			return false
		}
	}
	return true
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
	// An armed drag can still reach another layout. Keep its short-lived
	// feedback on the canvas instead of leaving a stale "break layout"
	// warning in the status bar after the pointer finds a valid target.
	if view.layoutReorder.item == nil {
		view.showStatus(hint)
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
