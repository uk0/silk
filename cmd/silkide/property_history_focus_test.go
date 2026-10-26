package main

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/prop"
	"testing"
)

type historyFocusObject struct{ value float64 }

func (o *historyFocusObject) EnumProperties(l prop.IPropertyList) {
	l.AddProperty("width", func() float64 { return o.value }, func(v float64) { o.value = v })
}

func setupHistoryFocus(t *testing.T) (*workbench, *prop.PropertySheet, *prop.FloatEdit, *historyFocusObject) {
	t.Helper()
	w := testWorkbench(t)
	w.activate(modeDesign)
	sheet := prop.NewPropertySheet()
	sheet.SetParent(w.frame)
	sheet.SetSize(240, 400)
	w.canvas.SetPropertyView(sheet)
	obj := &historyFocusObject{65.08749999999999}
	sheet.Bind([]interface{}{obj}, "", w.canvas.GedScene())
	var find func(gui.IWidget) *prop.FloatEdit
	find = func(p gui.IWidget) *prop.FloatEdit {
		if e, ok := p.(*prop.FloatEdit); ok {
			return e
		}
		for _, c := range p.Children() {
			if e := find(c); e != nil {
				return e
			}
		}
		return nil
	}
	edit := find(sheet)
	if edit == nil {
		t.Fatal("no float control")
	}
	edit.SetFocus()
	edit.OnKeyDown(gui.KeyUp, false)
	if obj.value != 66.08749999999999 {
		t.Fatal("step failed")
	}
	return w, sheet, edit, obj
}

func TestDesignHistoryKeepsPropertyInputFocus(t *testing.T) {
	w, _, edit, obj := setupHistoryFocus(t)
	for _, redo := range []bool{false, true, false} {
		undoCurrentDocument(w.tabs, w.canvas, redo)
		if !edit.HasFocus() {
			t.Fatal("history stole property focus")
		}
		want := 65.08749999999999
		if redo {
			want++
		}
		if obj.value != want {
			t.Fatalf("history value %g", obj.value)
		}
	}
	edit.SelectAll()
	edit.OnTextInput("70.125")
	if obj.value != 70.125 {
		t.Fatal("continuous editing after undo lost binding")
	}
	undoCurrentDocument(w.tabs, w.canvas, false)
	if obj.value != 65.08749999999999 || !edit.HasFocus() {
		t.Fatal("replacement undo lost value or focus")
	}
}

func TestDesignHistoryDoesNotRestoreUnavailableProperty(t *testing.T) {
	for _, kind := range []string{"hidden", "disabled", "detached", "foreign", "rebound", "removed-on-undo"} {
		t.Run(kind, func(t *testing.T) {
			w, sheet, edit, obj := setupHistoryFocus(t)
			switch kind {
			case "hidden":
				sheet.SetVisible(false)
			case "disabled":
				sheet.SetEnabled(false)
			case "detached":
				sheet.Detach()
			case "foreign":
				sheet.SetParent(gui.NewFrame())
			case "rebound":
				w.canvas.GedScene().AddDesignChangedCallback(func() { sheet.Bind(nil, "", w.canvas.GedScene()) })
			case "removed-on-undo":
				w.canvas.GedScene().AddDesignChangedCallback(func() { edit.Detach() })
			}
			undoCurrentDocument(w.tabs, w.canvas, false)
			if obj.value != 65.08749999999999 {
				t.Fatalf("unavailable property blur overwrote undo: %g", obj.value)
			}
			if !w.canvas.HasFocus() || edit.HasFocus() {
				t.Fatal("unavailable property retained focus")
			}
		})
	}
}
