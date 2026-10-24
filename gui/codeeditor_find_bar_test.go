package gui

import "testing"

func TestEditorFindBarResponsiveBoundsAndActions(t *testing.T) {
	for _, width := range []float64{200, 240, 320, 379, 380, 699, 700, 1024} {
		e := NewCodeEditor()
		e.SetSize(width, 500)
		e.SetText("Foo food foo")
		e.SetFindQuery("foo")
		e.ShowFind(true)
		b := e.findBar
		var controls []IWidget
		for _, w := range b.Children() {
			if !w.IsVisible() {
				continue
			}
			controls = append(controls, w)
			x, y := w.Pos()
			cw, ch := w.Size()
			if x < 0 || y < 0 || x+cw > width || y+ch > e.findBarHeight {
				t.Fatalf("width%v %T out of bounds: %v,%v %vx%v bar%v", width, w, x, y, cw, ch, e.findBarHeight)
			}
		}
		for i, a := range controls {
			ax, ay := a.Pos()
			aw, ah := a.Size()
			for _, b := range controls[i+1:] {
				bx, by := b.Pos()
				bw, bh := b.Size()
				if ax < bx+bw && ax+aw > bx && ay < by+bh && ay+ah > by {
					t.Fatalf("width%v controls overlap %T %T", width, a, b)
				}
			}
		}
		b.wholeWord.Toggle()
		if e.FindSnapshot().Count != 2 {
			t.Fatal(e.FindSnapshot())
		}
		b.matchCase.Toggle()
		if e.FindSnapshot().Count != 1 {
			t.Fatal(e.FindSnapshot())
		}
		b.close.OnKeyDown(KeyEsc, false)
		if e.findActive || b.IsVisible() {
			t.Fatal("Esc left bar visible")
		}
		e.CancelFind()
	}
}
func TestEditorFindInputSelectionPasteAndDocumentFocus(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelFind)
	e.SetSize(800, 500)
	e.SetText("foo bar foo")
	e.SetFindQuery("foo")
	e.ShowFind(false)
	f := e.findBar.query
	f.SelectAll()
	f.OnTextInput("bar")
	if f.Text() != "bar" || e.findText != "bar" || e.FindSnapshot().Count != 1 {
		t.Fatal("standard selection/input failed", e.FindSnapshot())
	}
	f.OnKeyDown(KeyHome, false)
	f.OnKeyDown(KeyDelete, false)
	if e.findText != "ar" {
		t.Fatal("Delete not routed", e.findText)
	}
	// Clicking into the document must not keep hijacking input for the query.
	e.SetFocus()
	e.cursorLine = 0
	e.cursorCol = 0
	e.OnTextInput("x")
	if e.findText != "ar" || e.Text() != "xfoo bar foo" {
		t.Fatal(e.findText, e.Text())
	}
	if e.FindSnapshot().Count != 1 || e.findMatches[0].col != 6 {
		t.Fatal("external edit did not refresh match ranges", e.findMatches)
	}
	e.closeFindBar()
	e.ReplaceAllText("ar")
	e.ShowFind(false)
	if e.FindSnapshot().Count != 1 || e.findMatches[0].col != 0 {
		t.Fatal("reopen reused obsolete ranges")
	}
}
func TestEditorFindTooltipRetirement(t *testing.T) {
	e := NewCodeEditor()
	e.ShowFind(true)
	b := e.findBar
	b.tooltips(true)
	if GetToolTip(b.query) == "" {
		t.Fatal("missing help")
	}
	e.CancelFind()
	for _, w := range b.Children() {
		if GetToolTip(w) != "" {
			t.Fatal("closed editor retained tooltip", w)
		}
	}
}

func TestEditorFindAndGotoReserveIndependentHeights(t *testing.T) {
	e := NewCodeEditor()
	t.Cleanup(e.CancelFind)
	e.SetSize(240, 500)
	e.ShowFind(true)
	if e.findBar.query.Font().Family() != defaultFontFamily() {
		t.Fatal("find field lost platform CJK font")
	}
	before := e.topOffset()
	e.gotoLineActive = true
	if e.topOffset() != before+codeEditorFindRowHeight {
		t.Fatal("wrapped find stretched goto-line bar")
	}
}
