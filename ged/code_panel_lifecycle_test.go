package ged

import (
	"testing"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

func TestCodePanelRepeatedSelectionRetainsUnsavedTextAndUndo(t *testing.T) {
	v := NewGedView()
	fake := addFakeTo(t, v.GedScene(), "gui.Button", "button")
	fake.SetCode("func onButtonClick() {}")
	p := NewCodePanel()
	p.BindGedView(v)
	defer p.BindGedView(nil)
	p.SetWidget(fake)
	code := "func onButtonClick() { println(42) }"
	p.editor.ReplaceAllText(code)
	for range 3 {
		v.notifySelectionListeners([]graph.IItem{fake})
	}
	if p.editor.Text() != code || fake.GetCode() == code {
		t.Fatal("selection reset draft or silently saved it")
	}
	p.SaveCode()
	if fake.GetCode() != code || !v.GedScene().UndoStack().CanUndo() {
		t.Fatal("save not undoable")
	}
	v.Undo()
	if fake.GetCode() != "func onButtonClick() {}" || p.editor.Text() != fake.GetCode() {
		t.Fatal("undo left code panel stale")
	}
	v.Redo()
	if p.editor.Text() != code {
		t.Fatal("redo did not refresh code panel")
	}
}

func TestCodePanelRebindRetiresSubscriptions(t *testing.T) {
	a, b := NewGedView(), NewGedView()
	aItem := addFakeTo(t, a.GedScene(), "gui.Button", "a")
	bItem := addFakeTo(t, b.GedScene(), "gui.Button", "b")
	p := NewCodePanel()
	p.BindGedView(a)
	p.SetWidget(aItem)
	for range 10 {
		p.BindGedView(b)
		p.BindGedView(a)
	}
	p.BindGedView(b)
	p.SetWidget(bItem)
	if len(a.selectionListeners) != 0 || len(a.GedScene().designChangedList) != 0 || len(b.selectionListeners) != 1 {
		t.Fatal("rebind leaked callbacks")
	}
	p.genPending = false
	a.notifySelectionListeners([]graph.IItem{aItem})
	a.GedScene().NotifyDesignChanged()
	if p.currentWidget != bItem || p.genPending {
		t.Fatal("old design controls active panel")
	}
	p.BindGedView(nil)
	if len(b.selectionListeners) != 0 || len(b.GedScene().designChangedList) != 0 {
		t.Fatal("detach leaked callbacks")
	}
}

func TestCodePanelNarrowLayoutContainsControls(t *testing.T) {
	p := NewCodePanel()
	for _, size := range [][2]float64{{90, 40}, {180, 200}, {320, 240}, {700, 280}, {0, 0}} {
		p.SetSize(size[0], size[1])
		p.Layout()
		for _, widget := range []gui.IWidget{p.tabs, p.titleBar, p.saveBtn, p.fmtBtn, p.editor, p.genView} {
			x, y, w, h := widget.Bounds()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0]+0.01 || y+h > size[1]+0.01 {
				t.Fatalf("%v: %T out of panel: %v %v %v %v", size, widget, x, y, w, h)
			}
		}
		if size[0] > 0 {
			fx, _, fw, _ := p.fmtBtn.Bounds()
			sx, _, _, _ := p.saveBtn.Bounds()
			if fx+fw > sx {
				t.Fatal("header actions overlap", size)
			}
		}
	}
}

func TestSelectionListenerCanUnsubscribeDuringDispatch(t *testing.T) {
	v := NewGedView()
	first, second := 0, 0
	id := 0
	id = v.AddSelectionListener(func([]graph.IItem) { first++; v.RemoveSelectionListener(id) })
	v.AddSelectionListener(func([]graph.IItem) { second++ })
	v.notifySelectionListeners(nil)
	v.notifySelectionListeners(nil)
	if first != 1 || second != 2 {
		t.Fatalf("dispatch changed under unsubscribe: %d %d", first, second)
	}
}
