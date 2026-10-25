package gui

import (
	"errors"
	"strings"
	"testing"
)

func gotoTestEditor(t *testing.T, text string) *CodeEditor {
	t.Helper()
	old := focusWidget
	e := NewCodeEditor()
	e.SetSize(800, 500)
	e.SetText(text)
	e.MarkSaved()
	t.Cleanup(func() { e.HideGoToLine(); e.CancelFind(); focusWidget = old })
	return e
}
func TestGoToLocationUnicodeCoordinatesAndCRLF(t *testing.T) {
	e := gotoTestEditor(t, "zero\r\n中🙂\tx\r\n\r\nlast\n")
	for _, tc := range []struct {
		query                      string
		line, column, bytes, utf16 int
	}{
		{"1", 0, 0, 0, 0}, {"2:3", 1, 2, 7, 3}, {" 2:5 ", 1, 4, 9, 5}, {"3:1", 2, 0, 0, 0}, {"5", 4, 0, 0, 0},
	} {
		p, err := e.ResolveGoToLocation(tc.query)
		if err != nil || p.Line != tc.line || p.RuneColumn != tc.column || p.ByteColumn != tc.bytes || p.UTF16Column != tc.utf16 {
			t.Fatalf("%q: %+v %v", tc.query, p, err)
		}
	}
	for _, s := range []string{"", "0", "-1", "+1", "1:0", "1:-2", "1:", ":1", "1:2:3", "1 :2", "1: 2", "two", "２", "6", "2:6", "3:2", "99999999999999999999999999999", strings.Repeat("1", 65)} {
		before := e.gotoCursorSnapshotForTest()
		if err := e.NavigateToLocation(s); !errors.Is(err, ErrEditorLocation) {
			t.Fatalf("%q accepted: %v", s, err)
		}
		if e.gotoCursorSnapshotForTest() != before {
			t.Fatal("invalid input changed caret", s)
		}
	}
	e.SetText("a\r")
	if _, err := e.ResolveGoToLocation("1:3"); err != nil {
		t.Fatal("standalone CR was discarded", err)
	}
	e.SetText(string([]byte{'a', 0xff, 'b'}))
	p, err := e.ResolveGoToLocation("1:3")
	if err != nil || p.ByteColumn != 2 {
		t.Fatal(p, err)
	}
}
func (e *CodeEditor) gotoCursorSnapshotForTest() [4]int {
	return [4]int{e.cursorLine, e.cursorCol, len(e.undoStack), len(e.redoStack)}
}

func TestGoToInputKeepsColonAndRejectsClampedDestinations(t *testing.T) {
	e := gotoTestEditor(t, "zero\n中🙂xyz\nlast")
	e.ShowGoToLine()
	if !e.gotoBar.accept.IsTextVisible() {
		t.Fatal("Go button label is hidden")
	}
	e.OnTextInput("999")
	e.gotoLineAccept()
	if !e.GoToLineVisible() || e.CursorLine() != 0 || e.gotoBar.accept.IsEnabled() || !strings.Contains(e.gotoBar.status.Text(), "1-3") {
		t.Fatal("invalid destination silently clamped")
	}
	e.gotoBar.query.SelectAll()
	e.OnTextInput("2:3")
	e.gotoLineAccept()
	if e.GoToLineVisible() || e.CursorLine() != 1 || e.CursorCol() != 2 || !e.HasFocus() {
		t.Fatal("column syntax lost", e.CursorLine(), e.CursorCol())
	}
	if e.Text() != "zero\n中🙂xyz\nlast" || e.Modified() || e.CanUndo() {
		t.Fatal("navigation mutated document")
	}
}
func TestGoToInputSelectionNavigationAndCancellation(t *testing.T) {
	e := gotoTestEditor(t, "zero\n中🙂xyz\nlast")
	e.editorSelectAll()
	before := e.selectionSnapshot()
	e.AddCursorAtLine(1, 2)
	e.ShowGoToLine()
	b := e.gotoBar
	b.query.OnTextInput("2:3")
	b.query.OnKeyDown(KeyLeft, false)
	b.query.OnKeyDown(KeyBackSpace, false)
	if b.query.Text() != "23" {
		t.Fatal("field caret editing not used", b.query.Text())
	}
	b.query.SelectAll()
	b.query.pasteString("2:3")
	if b.query.Text() != "2:3" || !b.accept.IsEnabled() {
		t.Fatal("field paste not validated")
	}
	b.query.OnKeyDown(KeyEsc, false)
	if e.GoToLineVisible() || e.selectionSnapshot() != before || e.CursorCount() != 2 || !e.HasFocus() {
		t.Fatal("cancel moved document selection/carets")
	}
	if e.TextRevision() != 1 || e.Modified() {
		t.Fatal("field edited document")
	}
}
func TestGoToInputReadonlyFoldsAndPrimaryCaret(t *testing.T) {
	e := gotoTestEditor(t, "package p\nfunc f() {\n if true {\n  target()\n }\n}\n")
	e.FoldAll()
	e.editorSelectAll()
	e.AddCursorAtLine(1, 1)
	e.SetReadOnly(true)
	if err := e.NavigateToLocation("4:3"); err != nil {
		t.Fatal(err)
	}
	if e.CursorLine() != 3 || e.CursorCol() != 2 || e.HasSelection() || e.CursorCount() != 1 || !e.HasFocus() {
		t.Fatal("navigation retained stale targets")
	}
	for _, r := range e.FoldRegions() {
		if r.startLine < 3 && 3 <= r.endLine && e.IsFolded(r.startLine) {
			t.Fatal("target hidden")
		}
	}
	if e.TextRevision() != 1 || e.Modified() || len(e.undoStack) != 0 {
		t.Fatal("readonly navigation mutated source")
	}
}
func TestGoToInputTracksReloadAndBodyFocus(t *testing.T) {
	e := gotoTestEditor(t, "a\nb\nc")
	e.ShowGoToLine()
	b := e.gotoBar
	b.query.SetText("3")
	if !b.accept.IsEnabled() {
		t.Fatal("valid query disabled")
	}
	e.SetText("only")
	if b.accept.IsEnabled() {
		t.Fatal("reload left stale valid destination")
	}
	b.query.SetText("1:2")
	e.SetFocus()
	e.OnTextInput("X")
	if e.Text() != "Xonly" || b.query.Text() != "1:2" {
		t.Fatal("visible bar hijacked document typing", e.Text(), b.query.Text())
	}
	e.ShowGoToLine()
	if b.query.Text() != "1:2" {
		t.Fatal("reopen discarded input")
	}
	e.HideGoToLine()
	e.SetText("a\r\nb")
	e.cursorCol = 2
	e.ShowGoToLine()
	if b.query.Text() != "1:2" || !b.accept.IsEnabled() {
		t.Fatal("End after CR seeded invalid destination", b.query.Text())
	}
}
func TestGoToBarOwnedResponsiveControls(t *testing.T) {
	for _, w := range []float64{0, 64, 120, 200, 240, 400, 559, 560, 800} {
		e := gotoTestEditor(t, "a\nb")
		e.SetSize(w, 500)
		e.ShowGoToLine()
		b := e.gotoBar
		if b.Parent() != e || b.Window() != e.Window() {
			t.Fatal("detached bar")
		}
		if b.query.Font().Family() != defaultFontFamily() {
			t.Fatal("wrong platform font")
		}
		var controls []IWidget
		for _, c := range b.Children() {
			x, y := c.Pos()
			cw, ch := c.Size()
			if c.Parent() != b || x < 0 || y < 0 || cw < 0 || ch < 0 || x+cw > w || y+ch > e.gotoBarHeight {
				t.Fatalf("width %v control %T out of bounds: %v %v %v %v", w, c, x, y, cw, ch)
			}
			if cw > 0 && ch > 0 {
				controls = append(controls, c)
			}
		}
		for i, a := range controls {
			ax, ay := a.Pos()
			aw, ah := a.Size()
			for _, b := range controls[i+1:] {
				bx, by := b.Pos()
				bw, bh := b.Size()
				if ax < bx+bw && ax+aw > bx && ay < by+bh && ay+ah > by {
					t.Fatalf("width %v overlap %T %T", w, a, b)
				}
			}
		}
		e.HideGoToLine()
	}
}
func TestGoToBarFindStackingAndTooltipRetirement(t *testing.T) {
	e := gotoTestEditor(t, "a\nb")
	e.SetSize(240, 500)
	e.ShowFind(true)
	e.ShowGoToLine()
	b := e.gotoBar
	_, y := b.Pos()
	if y != e.findBarHeight || e.topOffset() != e.findBarHeight+e.gotoBarHeight+e.breadcrumbHeight {
		t.Fatal("auxiliary bars overlap")
	}
	e.closeFindBar()
	_, y = b.Pos()
	if y != 0 {
		t.Fatal("closing find left stale gap")
	}
	b.tooltips(true)
	if GetToolTip(b.query) == "" {
		t.Fatal("missing hint")
	}
	e.OnHide()
	if e.GoToLineVisible() || b.IsVisible() {
		t.Fatal("hidden parent retained goto")
	}
	for _, c := range b.Children() {
		if GetToolTip(c) != "" {
			t.Fatal("tooltip retained", c)
		}
	}
}
func TestGoToFieldOwnsDocumentShortcutsOnly(t *testing.T) {
	e := gotoTestEditor(t, "a\nb")
	e.ShowGoToLine()
	b := e.gotoBar
	for _, key := range []int{'Z', 'Y', 'I', '.', KeyF2} {
		mods := ModAction
		if key == KeyF2 {
			mods = 0
		}
		if !b.query.HandleShortcut(key, mods) || !b.accept.HandleShortcut(key, mods) {
			t.Fatal("document shortcut leaked", key)
		}
	}
	for _, key := range []int{'C', 'V', 'X', 'A', 'P', 'S', 'W', 'Q'} {
		if b.query.HandleShortcut(key, ModAction) {
			t.Fatal("unrelated shortcut stolen", key)
		}
	}
	b.query.SetText("2")
	if !b.query.HandleShortcut('G', ModAction) || min(b.query.sel0, b.query.sel1) != 0 || max(b.query.sel0, b.query.sel1) != 1 {
		t.Fatal("GoTo shortcut did not select query")
	}
}
func TestGoToValidQueryAllocations(t *testing.T) {
	e := gotoTestEditor(t, strings.Repeat("中🙂x\n", 100000))
	if a := testing.AllocsPerRun(100, func() { _, _ = e.ResolveGoToLocation("99999:3") }); a != 0 {
		t.Fatal("valid location allocates", a)
	}
}
func BenchmarkEditorGoToLocation(b *testing.B) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("中🙂x\n", 100000))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = e.ResolveGoToLocation("99999:3")
	}
}
