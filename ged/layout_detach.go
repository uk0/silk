package ged

import (
	"math"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// layoutDetachAt keeps the widget's current size and grab offset. Fit the
// entire rectangle after snapping so the design and generated UI agree about
// ownership and no corner lands outside its new visible parent.
func (view *GedView) layoutDetachAt(arm layoutReorderGesture, target graph.IItem, x, y float64) layoutTransferPlan {
	scene := view.Scene()
	if scene == nil || arm.parent == nil || arm.item == nil {
		return layoutTransferPlan{}
	}
	if target == nil {
		target = scene
	}
	p := layoutTransferPlan{freeParent: target}
	if view.Selection().Count() > 1 {
		p.reject = dropTransferSingle
		return p
	}
	// Graph selection is finalized at release, not at the managed press.
	// The single item to move is the one captured by the press.
	if arm.parent.Scene() != scene || arm.item.Scene() != scene ||
		target.Scene() != scene ||
		!unlockedLayoutAncestors(arm.item) || arm.item.IsLockSize() {
		p.reject = dropLayoutLocked
		return p
	}
	if f, ok := target.(*FakeWidget); ok && !unlockedLayoutAncestors(f) {
		p.reject = dropLayoutLocked
		return p
	}
	if !view.acceptsDetachedChild(target) {
		p.reject = dropCannotPlaceFreely
		return p
	}
	for node := target; node != nil; node = node.Parent() {
		if node == arm.item {
			p.reject = dropCannotPlaceFreely
			return p
		}
	}

	page := scene.MapRectToScene1(scene.Bounds1())
	clip := freeLayoutDropArea(page, target)
	switch {
	case !pointInDropRect(page, x, y):
		p.reject = "Drop inside the form"
	case !pointInDropRect(clip, x, y):
		p.reject = dropIntoFreeContent
	}
	if p.reject != "" {
		return p
	}
	w, h := arm.item.Size()
	if w <= 0 || h <= 0 || !finiteDropCoordinate(w) || !finiteDropCoordinate(h) || w > clip.Width || h > clip.Height {
		p.reject = dropContainerTooSmall
		return p
	}
	x0, y0 := view.snapToGrid(x-arm.grabX*w, y-arm.grabY*h)
	p.freeBounds = geom.Rect{
		X:     math.Max(clip.X, math.Min(x0, clip.Right()-w)),
		Y:     math.Max(clip.Y, math.Min(y0, clip.Bottom()-h)),
		Width: w, Height: h,
	}
	return p
}

func (view *GedView) acceptsDetachedChild(target graph.IItem) bool {
	if target == view.Scene() {
		s := view.GedScene()
		return s != nil && s.activeFormLayout() == nil
	}
	f, ok := target.(*FakeWidget)
	if !ok || f.ManagesChildGeometry() || f.Widget() == nil || !f.Widget().IsVisible() {
		return false
	}
	// Page/pane based widgets still need their own insertion rules; a
	// free-positioned child cannot be made a tab or splitter pane here.
	return isDesignerLayoutFactory(f.factoryName) || f.supportsContentLayout() ||
		f.factoryName == "gui.Form" || f.factoryName == "gui.Dialog"
}

func freeLayoutDropArea(page geom.Rect, target graph.IItem) geom.Rect {
	visible := designDropArea(page, target)
	for node := target; node != nil; node = node.Parent() {
		f, ok := node.(*FakeWidget)
		if !ok || !f.supportsContentLayout() {
			continue
		}
		client := f.Widget().(contentGeometry).ContentBounds()
		r := f.MapRectToScene1(f.Bounds1())
		visible = visible.IntersectCopy(geom.Rect{
			X: r.X + gui.PixelToMm(client.X), Y: r.Y + gui.PixelToMm(client.Y),
			Width: gui.PixelToMm(client.Width), Height: gui.PixelToMm(client.Height),
		})
	}
	return visible
}

func freeDropTargetName(target graph.IItem) string {
	if f, ok := target.(*FakeWidget); ok {
		if name := f.WidgetName(); name != "" {
			return name
		}
		return widgetFriendlyName(f.WidgetFactoryName())
	}
	return i18n.T("Form")
}

func unlockedDetachSubtree(item graph.IItem) bool {
	if item.IsLockPos() {
		return false
	}
	for _, child := range item.Children() {
		if !unlockedDetachSubtree(child) {
			return false
		}
	}
	return true
}

func moveSubtreeToFreeBounds(item *FakeWidget, parent graph.IItem, bounds geom.Rect) {
	px, py := graph.ParentOrigin(parent)
	x, y := bounds.X-px, bounds.Y-py
	dx, dy := x-item.X(), y-item.Y()
	item.SetParentAt(parent, -1)
	item.SetPos(x, y)
	// SetPos reflows a managed subtree through the graph's move hook.
	// Only translate shared-coordinate children that aren't already reflowed.
	if !item.ManagesChildGeometry() {
		translateFreeChildren(item, dx, dy)
	}
}
