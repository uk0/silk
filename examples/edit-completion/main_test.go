package main

import "testing"

func TestCompletionDemoTransactionAndLayout(t *testing.T) {
	f := newCompletionDemo()
	for _, b := range f.buttons {
		if !b.IsTextVisible() || b.Icon() == nil || b.Icon().IsAir() {
			t.Fatal("button label/icon missing", b.Text())
		}
	}
	for _, size := range [][2]float64{{1000, 400}, {480, 400}, {220, 200}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		for _, c := range f.Children() {
			x, y := c.Pos()
			w, h := c.Size()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0] || y+h > size[1] {
				t.Fatal("out of bounds", size, x, y, w, h)
			}
		}
	}
	f.buttons[0].Action().Trigger(nil)
	if f.input.Text() != "中🙂 apple🍎 后缀" || f.edits != 1 || !f.undo.IsEnabled() {
		t.Fatal("completion not one transaction")
	}
	f.undo.Action().Trigger(nil)
	if f.input.Text() != initialText || f.edits != 2 || !f.redo.IsEnabled() {
		t.Fatal("undo failed")
	}
	f.redo.Action().Trigger(nil)
	if f.input.Text() != "中🙂 apple🍎 后缀" || f.edits != 3 {
		t.Fatal("redo failed")
	}
	f.reset()
	f.buttons[3].Action().Trigger(nil)
	f.buttons[0].Action().Trigger(nil)
	if f.input.Text() != initialText || f.edits != 0 {
		t.Fatal("readonly bypassed")
	}
}
