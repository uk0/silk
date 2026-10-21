package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
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
		{"水平布局", "layout-horizontal", ged.LayoutHorizontal},
		{"垂直布局", "layout-vertical", ged.LayoutVertical},
		{"网格布局", "layout-grid", ged.LayoutGrid},
		{"表单布局", "layout-form", ged.LayoutForm},
	} {
		kind := layout.kind
		enabled := func() bool { return active() && canvas.CanLayoutSelection(kind) }
		specs = append(specs, designActionSpec{layout.label, layout.icon,
			"多选同父的至少两个未锁定控件，或选中同类型自由布局容器直接启用；表单布局左右配对", enabled,
			func() {
				if enabled() {
					canvas.LayoutSelection(kind)
				}
			},
		})
	}
	canBreak := func() bool { return active() && canvas.CanBreakLayout() }
	specs = append(specs, designActionSpec{"解除布局", "layout-break", "选择未锁定的布局容器，保留子控件并解除排列", canBreak,
		func() {
			if canBreak() {
				canvas.BreakLayoutSelection()
			}
		}})
	specs = append(specs, designActionSpec{"适应视口", "layout-adjust", "缩放画布以显示完整表单 (" + actionKeyHint() + "+Shift+0)", active,
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
		gui.SetToolTip(button, spec.label+"："+spec.hint)
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
