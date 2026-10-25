package main

import "testing"

func TestCleanupDemoLayoutAndActions(t *testing.T) {
	f := newCleanupDemo()
	defer f.stop()
	defer f.editor.CancelSymbolIndex()
	for _, size := range [][2]float64{{1200, 640}, {720, 480}, {480, 560}, {220, 200}, {0, 0}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		for _, c := range f.Children() {
			x, y := c.Pos()
			w, h := c.Size()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0] || y+h > size[1] {
				t.Fatal(size, x, y, w, h)
			}
		}
	}
	invoke := func(i int) { f.buttons[i].Action().Trigger(f.buttons[i]) }
	invoke(0)
	invoke(2)
	want := "中🙂 alpha\r\n    beta\n\tgamma\r\nlast"
	if f.editor.Text() != want || f.editor.SelectedText() != want || !f.buttons[4].IsEnabled() {
		t.Fatal("trim selection")
	}
	invoke(3)
	if f.editor.Text() != want+"\r\n" || f.buttons[3].IsEnabled() {
		t.Fatal("append")
	}
	invoke(4)
	invoke(4)
	if f.editor.Text() != initialText || f.editor.Modified() {
		t.Fatal("undo")
	}
	invoke(6)
	invoke(1)
	invoke(2)
	if f.editor.CursorCount() != 2 || f.editor.CursorLine() != 3 || f.editor.CursorCol() != 4 {
		t.Fatal("carets")
	}
	invoke(7)
	if f.buttons[2].IsEnabled() || f.buttons[3].IsEnabled() {
		t.Fatal("readonly commands")
	}
}
