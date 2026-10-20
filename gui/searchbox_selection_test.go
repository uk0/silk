package gui

import (
	"github.com/uk0/silk/paint"
	"testing"
)

func TestSearchBoxSelectionRuneReplacementAndDelete(t *testing.T) {
	s := NewSearchBox()
	s.SetText("a中🟦z")
	s.SelectAll()
	if s.SelectionText() != s.Text() {
		t.Fatal("selection split Unicode runes")
	}
	calls := 0
	s.SigTextChanged(func(string) { calls++ })
	s.OnTextInput("Button")
	if s.Text() != "Button" || s.cursorPos != 6 || calls != 1 {
		t.Fatal("replacement did not emit exactly once")
	}
	s.selectionAnchor, s.cursorPos = 2, 5
	s.OnKeyDown(KeyBackSpace, false)
	if s.Text() != "Bun" || s.cursorPos != 2 || s.SelectionText() != "" {
		t.Fatal("backspace did not delete selection")
	}
	s.moveCaret(0, false)
	s.OnKeyDown(KeyBackSpace, false)
	s.moveCaret(99, false)
	s.OnKeyDown(KeyDelete, false)
	if s.Text() != "Bun" || calls != 2 {
		t.Fatal("boundary delete emitted a false change")
	}
	s.SelectAll()
	s.OnTextInput("Button\r\nChart\t中")
	if s.Text() != "Button Chart 中" {
		t.Fatal("paste introduced multiline/control characters")
	}
	s.SetText("Bun")
	s.SetEnabled(false)
	s.SelectAll()
	s.OnTextInput("bad")
	s.OnKeyDown(KeyDelete, false)
	if s.Text() != "Bun" {
		t.Fatal("disabled search query was edited")
	}
}

func TestSearchBoxLongQueryScrollsInsideTextStrip(t *testing.T) {
	s := NewSearchBox()
	s.SetSize(180, 30)
	s.SetText("a very long query ending in 中文 🟦")
	s.OnFocusIn()
	s.SelectAll()
	pix := paint.NewPixmap(180, 30)
	s.Draw(pix.NewPainter())
	if s.scrollX <= 0 {
		t.Fatal("long caret remained offscreen")
	}
	caret := searchBoxTextX + searchBoxCaretX(s.Text(), Theme().Font, s.cursorPos) - s.scrollX
	if caret > 180-searchBoxClearW || caret < searchBoxTextX {
		t.Fatal("caret covers icons")
	}
	s.moveCaret(0, false)
	s.Draw(pix.NewPainter())
	if s.scrollX != 0 {
		t.Fatal("Home did not restore left edge")
	}
}
