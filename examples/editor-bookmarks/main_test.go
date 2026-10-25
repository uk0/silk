package main

import "testing"

func TestBookmarksLayout(t *testing.T) {
	f := newBookmarkDemo()
	for _, size := range [][2]float64{{920, 640}, {480, 400}, {220, 200}} {
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
}

func TestBookmarksDemoFoldAndNavigationActions(t *testing.T) {
	f := newBookmarkDemo()
	defer f.unsubscribe()
	f.editor.SetText("new\n" + initialText)
	f.editor.SetBookmark(2, true)
	f.editor.NavigateToLineCol(1, 0)
	f.buttons[6].Action().Trigger(nil)
	if !f.editor.IsFolded(1) || f.editor.CursorCount() != 2 {
		t.Fatal("Fold + caret did not exercise both states")
	}
	f.buttons[3].Action().Trigger(nil)
	if f.editor.IsFolded(1) || f.editor.CursorCount() != 1 || f.editor.CursorLine() != 2 {
		t.Fatal("Next did not reveal sole caret")
	}
}
