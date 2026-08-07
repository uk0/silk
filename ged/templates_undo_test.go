package ged

// Picking a template from 新建项目 is one gesture, so it has to cost one Ctrl+Z.
// ApplyTemplate built an AddCommand per widget, so taking back the 8-widget
// 登录表单 took eight presses with a half-built form on the canvas in between —
// the same defect PasteItems shed one file over.
//
// Both cases below read scene membership by item identity. A count would be
// satisfied by any leftovers of the right number, and UndoText() reads "Add 1
// item" whether the stack holds one command or eight.

import "testing"

// TestApplyTemplateUndoesInOneStep drops a real built-in template and takes it
// back with the designer's own 撤销 handler.
func TestApplyTemplateUndoesInOneStep(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	tmpl := templateLoginForm()
	if len(tmpl.Widgets) < 2 {
		t.Fatalf("setup: 登录表单 declares %d widget(s); a one-widget template cannot"+
			" tell one command from one per widget", len(tmpl.Widgets))
	}

	reported := 0
	scene.SigDesignChanged(func() { reported++ })

	ApplyTemplate(scene, tmpl)

	var placed []*FakeWidget
	for _, item := range scene.Children() {
		fake, ok := item.(*FakeWidget)
		if !ok {
			t.Fatalf("the template put a %T on the canvas, want a *FakeWidget", item)
		}
		placed = append(placed, fake)
	}
	if len(placed) != len(tmpl.Widgets) {
		t.Fatalf("setup: a %d-widget template placed %d widgets", len(tmpl.Widgets), len(placed))
	}
	// One gesture, one report: the 生成代码 view used to be told N times that the
	// same drop had happened.
	if reported != 1 {
		t.Errorf("dropping a %d-widget template reported %d design changes, want 1",
			len(placed), reported)
	}

	view.Undo()

	held := sceneChildSet(scene)
	for _, w := range placed {
		if held[w] {
			t.Fatalf("after ONE undo of a %d-widget template the widget %q is still on"+
				" the canvas: applying a template is one gesture and must undo in one step",
				len(placed), w.WidgetName())
		}
	}
	if len(held) != 0 {
		t.Fatalf("after ONE undo the canvas holds %d item(s), want the empty form back",
			len(held))
	}

	view.Redo()

	held = sceneChildSet(scene)
	for _, w := range placed {
		if !held[w] {
			t.Fatalf("redoing the template left the widget %q off the canvas", w.WidgetName())
		}
	}
	if len(held) != len(placed) {
		t.Fatalf("after ONE redo the canvas holds %d item(s), want the template's %d",
			len(held), len(placed))
	}
}

// TestApplyTemplateWithNoBuildableWidgetsLeavesNothingToUndo covers the empty
// command. A template every factory lookup fails on changes nothing, so pushing
// it would leave a Ctrl+Z that does nothing visible and tell the listeners the
// design moved when it did not.
func TestApplyTemplateWithNoBuildableWidgetsLeavesNothingToUndo(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	reported := 0
	scene.SigDesignChanged(func() { reported++ })

	ApplyTemplate(scene, &ProjectTemplate{
		Name:   "坏模板",
		Width:  100,
		Height: 60,
		Widgets: []TemplateWidget{
			{Factory: "gui.NoSuchWidget", Name: "Ghost", X: 5, Y: 5, W: 20, H: 6},
		},
	})

	if got := len(scene.Children()); got != 0 {
		t.Fatalf("a template whose only factory is unknown placed %d item(s)", got)
	}
	if scene.UndoStack().CanUndo() {
		t.Errorf("a template that placed nothing left %q on the stack: Ctrl+Z now spends"+
			" a press on a command with nothing to take back", scene.UndoStack().UndoText())
	}
	if reported != 0 {
		t.Errorf("a template that placed nothing reported %d design change(s), want 0", reported)
	}
}
