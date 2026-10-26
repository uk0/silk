package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// Only inspect the active designer's property tree, and only when history is
// invoked. No global focus getter, per-frame polling or stale focus restoration.
func focusedPropertyControl(canvas *ged.GedView) gui.IWidget {
	panel, ok := canvas.GetPropertyView().(gui.IWidget)
	if !ok || panel == nil || panel.RootWidget() != canvas.RootWidget() {
		return nil
	}
	for p := panel; p != nil; p = p.Parent() {
		if !p.IsVisible() || !p.IsEnabled() {
			return nil
		}
	}
	return focusedPropertyDescendant(panel)
}

func focusedPropertyDescendant(p gui.IWidget) gui.IWidget {
	if !p.IsVisible() || !p.IsEnabled() {
		return nil
	}
	if p.HasFocus() {
		return p
	}
	for _, child := range p.Children() {
		if focused := focusedPropertyDescendant(child); focused != nil {
			return focused
		}
	}
	return nil
}
