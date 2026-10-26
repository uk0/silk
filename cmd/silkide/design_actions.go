package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

type designActionSpec struct {
	label, icon, hint string
	enabled           func() bool
	run               func()
}
type designActionButton struct {
	button *gui.Button
	spec   designActionSpec
}

// The palette and toolbar use the same guarded operations. Layout commands
// cannot mutate a hidden design while a source document is being edited.
func designerActionSpecs(canvas *ged.GedView) []designActionSpec {
	active := func() bool {
		return canvas != nil && (globalWorkbench == nil || documentMode(globalWorkbench.tabs, canvas) == modeDesign)
	}
	var specs []designActionSpec
	for _, layout := range []struct {
		label, icon string
		kind        ged.LayoutKind
	}{
		{i18n.T("Lay Out Horizontally"), "layout-horizontal", ged.LayoutHorizontal},
		{i18n.T("Lay Out Vertically"), "layout-vertical", ged.LayoutVertical},
		{i18n.T("Lay Out in a Grid"), "layout-grid", ged.LayoutGrid},
		{i18n.T("Lay Out in a Form"), "layout-form", ged.LayoutForm},
	} {
		kind := layout.kind
		enabled := func() bool { return active() && canvas.CanLayoutSelection(kind) }
		specs = append(specs, designActionSpec{layout.label, layout.icon,
			i18n.T("Select two or more unlocked siblings, or a matching free-layout container. Form layout pairs labels and fields."), enabled,
			func() {
				if enabled() {
					canvas.LayoutSelection(kind)
				}
			},
		})
	}
	canBreak := func() bool { return active() && canvas.CanBreakLayout() }
	specs = append(specs, designActionSpec{i18n.T("Break Layout"), "layout-break", i18n.T("Select an unlocked layout container; keep its children and remove the layout."), canBreak,
		func() {
			if canBreak() {
				canvas.BreakLayoutSelection()
			}
		}})
	specs = append(specs, designActionSpec{i18n.T("Fit to View"), "layout-adjust", i18n.Tf("Show the complete form (%s+Shift+0)", actionKeyHint()), active,
		func() {
			if active() {
				fitCanvasToView(canvas)
			}
		}})
	return specs
}

func buildDesignToolBar(frame *gui.Frame, canvas *ged.GedView) {
	if frame == nil || canvas == nil {
		return
	}
	bar := gui.NewToolBar()
	bar.SetCompact(true)
	bar.SetPreserveContentFocus(true)
	bar.SetSpacing(2)
	var actions []designActionButton
	for i, spec := range designerActionSpecs(canvas) {
		if i == 4 || i == 5 {
			bar.AddSeparator()
		}
		button := bar.AddAction(spec.label, paint.LoadIcon(spec.icon), func() {
			spec.run()
			if w := globalWorkbench; w != nil {
				w.syncMode()
			}
		})
		button.SetTextVisible(true)
		gui.SetToolTip(button, spec.label+" — "+spec.hint)
		actions = append(actions, designActionButton{button, spec})
	}
	frame.SetContextToolBar(bar)
	if w := globalWorkbench; w != nil && w.frame == frame {
		if w.designToolbar == nil {
			canvas.AddSelectionCallback(func(_ []graph.IItem) { w.syncMode() })
		}
		w.designToolbar, w.designActions = bar, actions
		w.syncDesignActions(documentMode(w.tabs, canvas))
	}
}

func (w *workbench) syncDesignActions(mode workspaceMode) {
	if w.designToolbar == nil {
		return
	}
	visible := mode == modeDesign
	if w.designToolbar.IsVisible() != visible {
		w.designToolbar.SetVisible(visible)
		if w.frame != nil {
			w.frame.Layout()
		}
	}
	for _, action := range w.designActions {
		enabled := action.spec.enabled()
		if action.button.IsEnabled() != enabled {
			action.button.SetEnabled(enabled)
			action.button.Action().SetEnabled(enabled)
			action.button.Update()
		}
	}
}
