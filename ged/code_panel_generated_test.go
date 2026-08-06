package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// addFakeViaCommand adds a widget the way the canvas does: through an
// AddCommand on the scene's undo stack, so the add is undoable.
func addFakeViaCommand(t *testing.T, scene *GedScene, factory, name string) *FakeWidget {
	t.Helper()
	fw, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatalf("create %s: %v", factory, err)
	}
	fw.SetWidgetName(name)
	fw.SetBounds(5, 5, 30, 8)
	cmd := graph.NewAddCommand()
	cmd.AddItem(fw, scene)
	scene.PushCommand(cmd)
	return fw
}

// TestCodePanelGeneratedTabShowsFullFile: the 生成代码 tab shows the whole
// generated file for the active design, not the selected widget's handler.
func TestCodePanelGeneratedTabShowsFullFile(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("PanelDemo")
	addFakeTo(t, scene, "gui.Button", "btnOK")
	addFakeTo(t, scene, "gui.Label", "lblHi")

	panel := NewCodePanel()
	panel.BindGedView(view)
	panel.ShowTab(codeTabGen)

	got := panel.genView.Text()
	for _, want := range []string{"func NewPanelDemoUI()", "ui.BtnOK = gui.NewButton1", "ui.LblHi = gui.NewLabel"} {
		if !strings.Contains(got, want) {
			t.Errorf("生成代码 view missing %q\n----\n%s", want, got)
		}
	}
}

// TestCodePanelRegeneratesOnDesignChange: a design change reaches the panel
// through the scene's change signal, and the refreshed view shows the new
// state. Renaming a widget is the case with no signal of its own — the property
// sheet pushes it onto the command stack like every other edit.
func TestCodePanelRegeneratesOnDesignChange(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("Renamed")
	btn := addFakeTo(t, scene, "gui.Button", "btnOld")

	panel := NewCodePanel()
	panel.BindGedView(view)
	panel.ShowTab(codeTabGen)
	if !strings.Contains(panel.genView.Text(), "ui.BtnOld = ") {
		t.Fatalf("initial view does not show the widget\n----\n%s", panel.genView.Text())
	}

	btn.SetWidgetName("btnNew")
	scene.NotifyDesignChanged()
	if !panel.genPending {
		t.Fatal("design change did not reach the code panel")
	}
	// The debounce timer only fires on the UI loop; run what it would run.
	panel.RegenerateNow()

	got := panel.genView.Text()
	if !strings.Contains(got, "ui.BtnNew = ") {
		t.Errorf("view not regenerated after rename\n----\n%s", got)
	}
	if strings.Contains(got, "ui.BtnOld = ") {
		t.Errorf("view still shows the old name\n----\n%s", got)
	}
}

// TestCodePanelPushCommandSchedulesRegenerate: pushing a command onto the
// scene — which is how every undoable design edit lands, property-sheet edits
// included — marks the generated view stale.
func TestCodePanelPushCommandSchedulesRegenerate(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	addFakeTo(t, scene, "gui.Button", "btnOK")

	panel := NewCodePanel()
	panel.BindGedView(view)
	panel.ShowTab(codeTabGen)
	if panel.genPending {
		t.Fatal("view starts stale")
	}

	addFakeViaCommand(t, scene, "gui.Label", "lblNew")
	if !panel.genPending {
		t.Error("PushCommand did not mark the generated view stale")
	}
	panel.RegenerateNow()
	if !strings.Contains(panel.genView.Text(), "ui.LblNew = ") {
		t.Errorf("regenerated view missing the added widget\n----\n%s", panel.genView.Text())
	}
}

// TestCodePanelUndoSchedulesRegenerate: undo runs a command backwards and
// pushes nothing, so it has to report the change itself. Without this the view
// keeps showing the state Ctrl+Z just left.
func TestCodePanelUndoSchedulesRegenerate(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("Undone")
	addFakeViaCommand(t, scene, "gui.Button", "btnOK")

	panel := NewCodePanel()
	panel.BindGedView(view)
	panel.ShowTab(codeTabGen)
	panel.genPending = false

	view.Undo()
	if !panel.genPending {
		t.Fatal("Undo did not mark the generated view stale")
	}
	panel.RegenerateNow()
	if got := panel.genView.Text(); strings.Contains(got, "ui.BtnOK = ") {
		t.Errorf("view still shows the undone widget\n----\n%s", got)
	}

	panel.genPending = false
	view.Redo()
	if !panel.genPending {
		t.Fatal("Redo did not mark the generated view stale")
	}
	panel.RegenerateNow()
	if got := panel.genView.Text(); !strings.Contains(got, "ui.BtnOK = ") {
		t.Errorf("view missing the redone widget\n----\n%s", got)
	}
}

// TestCodePanelSelectionScrollsGeneratedView: selecting a widget on the canvas
// lands the generated view on the line that builds it.
func TestCodePanelSelectionScrollsGeneratedView(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("Scroll")
	addFakeTo(t, scene, "gui.Button", "btnFirst")
	addFakeTo(t, scene, "gui.Label", "lblSecond")
	last := addFakeTo(t, scene, "gui.Edit", "editLast")

	panel := NewCodePanel()
	panel.BindGedView(view)
	panel.ShowTab(codeTabGen)

	want, ok := panel.genLines[last]
	if !ok {
		t.Fatalf("no line recorded for editLast\n----\n%s", panel.genView.Text())
	}
	panel.ScrollToWidget(last)
	if got := panel.genView.CursorLine(); got != want {
		t.Errorf("generated view on line %d, want %d", got, want)
	}
	if line := lineOf(panel.genView.Text(), want); !strings.Contains(line, "ui.EditLast = ") {
		t.Errorf("line %d = %q, want the editLast constructor", want, line)
	}
}

// TestCodePanelShowsGenerationError: a widget codegen has no mapping for makes
// the whole listing untrustworthy, so the view names it instead of showing
// code that builds but is not the design.
func TestCodePanelShowsGenerationError(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("Broken")
	addFakeTo(t, scene, "gui.Button", "btnOK")
	addFakeTo(t, scene, "gui.Separator", "sepBad")

	panel := NewCodePanel()
	panel.BindGedView(view)
	panel.ShowTab(codeTabGen)

	got := panel.genView.Text()
	if !strings.Contains(got, "sepBad") {
		t.Errorf("view does not name the offending widget\n----\n%s", got)
	}
	if strings.Contains(got, "func NewBrokenUI()") {
		t.Errorf("view shows the degraded listing instead of the error\n----\n%s", got)
	}
}

// TestCodePanelHandlerTabStillEdits: the second view is an addition. After a
// round trip through 生成代码 the handler tab is editable again and still saves
// back to the widget.
func TestCodePanelHandlerTabStillEdits(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	btn := addFakeTo(t, scene, "gui.Button", "btnOK")

	panel := NewCodePanel()
	panel.BindGedView(view)
	panel.SetWidget(btn)

	panel.ShowTab(codeTabGen)
	panel.ShowTab(codeTabHandler)
	if !panel.editor.IsVisible() || panel.genView.IsVisible() {
		t.Fatal("handler tab did not come back forward")
	}

	panel.editor.SetText("func onBtnOKClick() { /* mine */ }")
	panel.SaveCode()
	if got := btn.GetCode(); !strings.Contains(got, "/* mine */") {
		t.Errorf("handler code not saved back to the widget: %q", got)
	}
}

// TestGeneratedViewIgnoresEditingKeys: the generated buffer is replaced by the
// next regeneration, so anything typed into it would vanish without a trace.
// Reading keys still work.
func TestGeneratedViewIgnoresEditingKeys(t *testing.T) {
	v := newGeneratedView()
	const src = "package main\n\nfunc main() {\n}\n"
	v.SetText(src)

	v.OnTextInput("X")
	if got := v.Text(); got != src {
		t.Errorf("typing changed the buffer:\n%q", got)
	}
	for _, key := range []int{gui.KeyEnter, gui.KeyBackSpace, gui.KeyDelete, gui.KeyTab} {
		v.OnKeyDown(key, false)
		if got := v.Text(); got != src {
			t.Errorf("key %d changed the buffer:\n%q", key, got)
		}
	}

	// Navigation is not editing: it must still reach the editor.
	v.OnKeyDown(gui.KeyDown, false)
	if v.CursorLine() == 0 {
		t.Error("arrow key did not move the cursor")
	}
}
