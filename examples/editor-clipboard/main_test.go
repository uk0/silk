package main

import "testing"

func TestClipboardDemoLayoutAndSelection(t *testing.T) {
	f := newClipboardDemo()
	defer f.unsubscribeHistory()
	defer f.editor.CancelGitStatus()
	for _, size := range [][2]float64{{980, 620}, {480, 350}, {180, 100}, {0, 0}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		for _, c := range f.Children() {
			x, y := c.Pos()
			w, h := c.Size()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0]+.01 || y+h > size[1]+.01 {
				t.Fatal(size, x, y, w, h)
			}
		}
	}
	f.buttons[3].Action().Trigger(nil)
	if f.editor.SelectedText() != f.editor.Text() || f.editor.CanUndo() {
		t.Fatal("Select All mutated the document")
	}
	f.editor.DuplicateLines()
	if !f.buttons[4].IsEnabled() || f.buttons[5].IsEnabled() {
		t.Fatal("history buttons")
	}
	f.buttons[4].Action().Trigger(nil)
	if !f.buttons[5].IsEnabled() {
		t.Fatal("Redo not enabled")
	}
}
