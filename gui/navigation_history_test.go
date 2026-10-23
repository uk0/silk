package gui

import "testing"

func TestNavigationHistoryRoundTripBranchAndBound(t *testing.T) {
	var h NavigationStack
	a, b, c := NavPosition{FilePath: "a.go", Line: 2, Column: 3}, NavPosition{FilePath: "b.go", Line: 4}, NavPosition{FilePath: "c.go", Line: 5}
	h.RecordJump(a, b)
	h.RecordJump(b, c)
	if len(h.stack) != 3 {
		t.Fatalf("duplicate origins: %v", h.stack)
	}
	live := c
	live.Column = 8
	var target NavPosition
	visit := func(p NavPosition) bool { target = p; return true }
	if !h.Navigate(live, false, visit) || target != b {
		t.Fatal("back did not return to b")
	}
	if !h.Navigate(b, true, visit) || target != live {
		t.Fatal("forward lost live column")
	}
	h.Navigate(live, false, visit)
	h.RecordJump(b, a)
	if h.CanGoForward() {
		t.Fatal("new branch retained old forward history")
	}
	for i := 0; i < 1000; i++ {
		h.Push(NavPosition{FilePath: "a.go", Line: i})
	}
	if len(h.stack) != 256 {
		t.Fatal("unbounded history", len(h.stack))
	}
}

func TestNavigationHistoryFailedTargetsAndRemap(t *testing.T) {
	var h NavigationStack
	a, b, c := NavPosition{FilePath: "a"}, NavPosition{FilePath: "deleted"}, NavPosition{FilePath: "c"}
	h.RecordJump(a, b)
	h.RecordJump(b, c)
	if h.Navigate(c, false, func(NavPosition) bool { return false }) || h.current != 2 {
		t.Fatal("failed open advanced history")
	}
	h.MapPaths(func(p string) string {
		if p == "a" {
			return "renamed"
		}
		return p
	})
	var got NavPosition
	if !h.Navigate(c, false, func(p NavPosition) bool { got = p; return p.FilePath != "deleted" }) || got.FilePath != "renamed" {
		t.Fatal("missing target not skipped or rename lost")
	}
}

func TestStandaloneDefinitionHistoryIsPreciseAndRevealsFolds(t *testing.T) {
	ed := NewCodeEditor()
	ed.SetSize(500, 200)
	src := "package p\nfunc caller() {\n target()\n}\nfunc target() {}\n"
	ed.SetText(src)
	ed.MarkSaved()
	ed.NavigateToLineCol(2, 3)
	ed.GoToDefinitionAtCursor()
	if ed.CursorLine() != 4 || ed.CursorCol() != 5 {
		t.Fatalf("definition: %d:%d", ed.CursorLine(), ed.CursorCol())
	}
	ed.FoldAll()
	ed.NavGoBack()
	if ed.CursorLine() != 2 || ed.CursorCol() != 3 || ed.IsFolded(1) {
		t.Fatal("back lost caret or left it folded")
	}
	ed.NavGoForward()
	if ed.CursorLine() != 4 || ed.CursorCol() != 5 || ed.Text() != src || ed.Modified() {
		t.Fatal("forward changed text or location")
	}
}

func TestNavigationClearsSelectionAndUsesUTF16(t *testing.T) {
	ed := NewCodeEditor()
	ed.SetSize(500, 200)
	ed.SetText("func test(){\n /*😀*/ var 目标 = 1\n}\n")
	ed.editorSelectAll()
	ed.AddCursorAtLine(1, 1)
	ed.FoldAll()
	ed.NavigateToUTF16Position(1, 12)
	if ed.CursorCol() != 11 || ed.IsFolded(0) || ed.HasSelection() || len(ed.additionalCursors) != 0 {
		t.Fatalf("bad jump: %+v", ed.NavigationPosition())
	}
}

func TestPreciseHostNavigationUsedWithinSameFile(t *testing.T) {
	ed := NewCodeEditor()
	ed.SetText("func target(){}\nfunc f(){target()}\n")
	ed.cursorLine, ed.cursorCol = 1, 10
	called := false
	ed.SetNavigatePositionCallback(func(p NavPosition) bool { called = true; return p.Line == 0 && p.Column == 5 })
	ed.GoToDefinitionAtCursor()
	if !called {
		t.Fatal("same-file jump bypassed shared host history")
	}
}
