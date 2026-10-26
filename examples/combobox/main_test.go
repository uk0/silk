package main

import (
	"github.com/uk0/silk/gui"
	"testing"
)

func TestComboDemoSeparatesPreviewAndCommittedState(t *testing.T) {
	f := newComboDemo()
	if f.choice.IsEditable() || !f.editable.IsEditable() || f.choice.Count() != 100 {
		t.Fatal("incorrect demo modes")
	}
	f.choice.OnTextInput("简体")
	if f.choice.ActiveIndex() != 2 || f.changes != 1 || f.submits != 0 {
		t.Fatal("closed type-ahead not committed once")
	}
	f.choice.SubPopup().SetVisible(true)
	f.choice.SubPopup().OnKeyDown(gui.KeyEnd, false)
	if f.choice.ActiveIndex() != 2 || f.changes != 1 {
		t.Fatal("preview committed prematurely")
	}
	f.choice.SubPopup().OnKeyDown(gui.KeyEnter, false)
	if f.choice.ActiveIndex() != 99 || f.changes != 2 || f.submits != 1 {
		t.Fatal("popup commit incorrect")
	}
	for _, size := range [][2]float64{{0, 0}, {220, 160}, {680, 330}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		for _, w := range f.widgets {
			if w.Width() < 0 || w.Height() < 0 || w.Y()+w.Height() > size[1]+.1 {
				t.Fatal("layout escapes bounds")
			}
		}
	}
}
