package main

import "testing"

func TestReadOnlyExampleLockReloadAndNarrowLayout(t *testing.T) {
	f := newReadOnlyDemo()
	defer f.editor.CancelGitStatus()
	defer f.editor.CancelCompletion()
	if !f.editor.IsReadOnly() {
		t.Fatal("must start as viewer")
	}
	f.editor.OnTextInput("blocked")
	if f.editor.Text() != initialText {
		t.Fatal("viewer edited")
	}
	f.buttons[0].Action().Trigger(nil)
	f.editor.OnTextInput("!")
	if !f.editor.CanUndo() || f.editor.Text() == initialText {
		t.Fatal("unlock failed")
	}
	f.buttons[0].Action().Trigger(nil)
	f.editor.Undo()
	if f.editor.Text() == initialText {
		t.Fatal("locked undo")
	}
	f.buttons[4].Action().Trigger(nil)
	if f.editor.Text() != initialText || !f.editor.IsReadOnly() || f.editor.CanUndo() {
		t.Fatal("host reload")
	}
	for _, width := range []float64{920, 480, 220} {
		f.SetSize(width, 640)
		f.OnResize()
		for _, b := range f.buttons {
			x, y, w, h := b.Bounds()
			_, ey := f.editor.Pos()
			if x < 0 || x+w > width || y+h > ey {
				t.Fatal("button overlaps editor", width, b.Bounds1())
			}
		}
	}
}
