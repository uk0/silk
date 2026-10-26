package gui

import (
	"strings"
	"testing"
)

type editorCaretCommands interface {
	AddCursorAbove() bool
	AddCursorBelow() bool
	CanAddCursorAbove() bool
	CanAddCursorBelow() bool
	CursorCount() int
}

func caretCommandEditor(t *testing.T, text string) (*CodeEditor, editorCaretCommands) {
	t.Helper()
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	e := NewCodeEditor()
	e.SetText(text)
	api, ok := any(e).(editorCaretCommands)
	if !ok {
		t.Fatal("SDK has no discoverable multi-caret command API")
	}
	return e, api
}
func TestCaretCommandsKeepPrimaryAndDoNotEdit(t *testing.T) {
	e, api := caretCommandEditor(t, "one a\xffb\ntwo c\xfed\nthree")
	e.cursorCol = 4
	revision := e.TextRevision()
	if api.CanAddCursorAbove() || api.AddCursorAbove() {
		t.Fatal("above BOF must be unavailable")
	}
	if !api.AddCursorBelow() || api.CursorCount() != 2 || e.cursorLine != 0 || e.cursorCol != 4 {
		t.Fatal("below did not add secondary caret")
	}
	if !api.AddCursorBelow() || api.CursorCount() != 3 || api.CanAddCursorBelow() || api.AddCursorBelow() {
		t.Fatal("edge/boundary commands")
	}
	if e.TextRevision() != revision || len(e.undoStack) != 0 || e.Text() != "one a\xffb\ntwo c\xfed\nthree" {
		t.Fatal("caret commands edited document/history")
	}
	e.OnTextInput("!")
	if e.Text() != "one !a\xffb\ntwo !c\xfed\nthre!e" {
		t.Fatalf("wrong edit: %q", e.Text())
	}
	e.Undo()
	if e.Text() != "one a\xffb\ntwo c\xfed\nthree" || api.CursorCount() != 3 {
		t.Fatal("undo did not retain command carets")
	}
	e.ClearAdditionalCursors()
	if api.CursorCount() != 1 {
		t.Fatal("clear command")
	}
}
func TestCaretCommandsRevealFoldAndNewCaretViewport(t *testing.T) {
	e, api := caretCommandEditor(t, "func f() {\n\tone\n\ttwo\n}\nlast\nend")
	e.SetSize(400, 80)
	e.ToggleFold(0)
	if !e.IsFolded(0) {
		t.Fatal("fold fixture")
	}
	if !api.AddCursorBelow() || e.IsFolded(0) {
		t.Fatal("new caret hidden in folded body")
	}
	for api.CanAddCursorBelow() {
		if !api.AddCursorBelow() {
			t.Fatal("below unexpectedly failed")
		}
	}
	if e.scrollY <= 0 || e.cursorLine != 0 {
		t.Fatal("secondary caret did not reveal without moving primary")
	}
}
func TestCaretCommandsClampUnevenUnicodeLinesAndClearSelection(t *testing.T) {
	e, api := caretCommandEditor(t, "longer line\n中😀\n\nabc")
	e.cursorLine = 3
	e.cursorCol = 3
	e.hasSelection = true
	e.selStartLine = 0
	e.selStartCol = 0
	e.selEndLine = 3
	e.selEndCol = 3
	if !api.AddCursorAbove() || e.hasSelection || e.additionalCursors[0] != (cursorPos{2, 0}) {
		t.Fatal("blank line/selection")
	}
	if !api.AddCursorAbove() || e.additionalCursors[1] != (cursorPos{1, 0}) {
		t.Fatal("adjacent caret column anchor")
	}
	if !api.AddCursorAbove() || api.CanAddCursorAbove() {
		t.Fatal("above boundary")
	}
}
func BenchmarkEditorEnsureCaretVisiblePrefix(b *testing.B) {
	for _, size := range []struct {
		name string
		n    int
	}{{"short", 80}, {"4MB", 4 << 20}} {
		b.Run(size.name, func(b *testing.B) {
			e := NewCodeEditor()
			e.filePath = "plain.txt"
			e.SetSize(640, 480)
			e.SetText("a" + strings.Repeat("z", size.n))
			e.cursorCol = 1
			e.ensureCursorVisible()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				e.ensureCursorVisible()
			}
		})
	}
}

func TestCaretCommandsCancelTransientInputsAndBoundaryIsPure(t *testing.T) {
	e, api := caretCommandEditor(t, "first\nsecond")
	e.findActive, e.replaceVisible, e.replaceFocused = true, true, true
	e.gotoLineActive, e.renameActive = true, true
	e.gotoLineText = "2"
	e.completion = NewCompletionPopup(e)
	e.completion.open = true
	e.completion.visible = true
	e.snippetSess = &snippetSession{}
	e.signatureHintVisible = true
	revision, hints := e.TextRevision(), e.LanguageHintRevision()
	if api.AddCursorAbove() || !e.findActive || !e.completion.visible || e.LanguageHintRevision() != hints {
		t.Fatal("boundary command mutated input state")
	}
	if !api.AddCursorBelow() {
		t.Fatal("add")
	}
	if e.findActive || e.replaceVisible || e.replaceFocused || e.gotoLineActive || e.renameActive || e.completion.open || e.completion.visible || e.snippetSess != nil || e.signatureHintVisible {
		t.Fatal("input overlays still consume body typing")
	}
	if e.TextRevision() != revision || e.LanguageHintRevision() == hints {
		t.Fatal("wrong document/hint revision")
	}
	e.OnTextInput("!")
	if e.Text() != "!first\n!second" {
		t.Fatalf("typing did not reach every caret: %q", e.Text())
	}
}

func TestCaretKeepPrimaryAndEscapeRevealPrimaryWithoutEditing(t *testing.T) {
	e, api := caretCommandEditor(t, "a\nb\nc\nd\ne\nf")
	e.SetSize(400, 60)
	for api.CanAddCursorBelow() {
		api.AddCursorBelow()
	}
	if e.scrollY == 0 {
		t.Fatal("fixture did not scroll")
	}
	revision := e.TextRevision()
	e.OnKeyDown(KeyEsc, false)
	if api.CursorCount() != 1 || e.scrollY != 0 || e.cursorLine != 0 || e.TextRevision() != revision {
		t.Fatal("Escape did not reveal primary caret")
	}
	if e.KeepPrimaryCursor() {
		t.Fatal("nothing to clear")
	}
	api.AddCursorBelow()
	e.hasSelection = true
	e.selStartLine = 0
	e.selEndLine = 0
	e.selStartCol = 0
	e.selEndCol = 1
	if !e.KeepPrimaryCursor() || !e.hasSelection || e.SelectedText() != "a" {
		t.Fatal("clear discarded primary selection")
	}
}

func TestCaretContextMenuActionsAndStatus(t *testing.T) {
	e, api := caretCommandEditor(t, "abcd\nefgh\nijkl")
	items := e.contextMenuItems("")
	for _, label := range []string{"Add Cursor Above", "Add Cursor Below", "Keep Primary Cursor"} {
		item := findMenuItem(items, label)
		if item == nil || item.Icon == "" || item.Action == nil {
			t.Fatal("missing context action/icon", label)
		}
	}
	if findMenuItem(items, "Add Cursor Above").Enabled || findMenuItem(items, "Keep Primary Cursor").Enabled {
		t.Fatal("wrong boundary enablement")
	}
	findMenuItem(items, "Add Cursor Below").Action()
	if api.CursorCount() != 2 || !strings.Contains(e.cursorStatusLabel(false), "2 carets") || !strings.Contains(e.cursorStatusLabel(true), "2 carets") {
		t.Fatal("missing count/status")
	}
	findMenuItem(e.contextMenuItems(""), "Keep Primary Cursor").Action()
	if api.CursorCount() != 1 || strings.Contains(e.cursorStatusLabel(false), "carets") {
		t.Fatal("clear count/status")
	}
}

func TestCaretVisibilityClampsAndPreservesBytes(t *testing.T) {
	for _, text := range []string{"", "a中😀\xff\x00z", "abc\ndef"} {
		e := NewCodeEditor()
		e.SetSize(50, 50)
		e.SetText(text)
		for _, line := range []int{-1, 0, 100} {
			for _, col := range []int{-8, 0, 1, 100} {
				e.cursorLine, e.cursorCol = line, col
				e.ensureCursorVisible()
				p, ok := e.CursorTextPosition()
				if !ok || p.RuneColumn != e.cursorCol || e.scrollX < 0 || e.scrollY < 0 || e.Text() != text {
					t.Fatal("invalid clamp or mutated bytes", p, text)
				}
			}
		}
	}
}

func TestCaretRightClickSecondaryPromotesWithoutDuplicate(t *testing.T) {
	e, api := caretCommandEditor(t, "abcdef\nghijkl")
	e.SetSize(600, 400)
	e.cursorCol = 2
	api.AddCursorBelow()
	lh := e.font.FontExtents().Height + 2
	x := e.gutterW + 10 + e.measureText("gh")
	y := e.topOffset() + lh*1.5
	e.repositionCaretForRightClick(x, y)
	if e.cursorLine != 1 || e.cursorCol != 2 || e.additionalCursors[0] != (cursorPos{0, 2}) {
		t.Fatal("right click duplicated secondary caret", e.cursorLine, e.cursorCol, e.additionalCursors)
	}
	e.OnTextInput("!")
	if e.Text() != "ab!cdef\ngh!ijkl" {
		t.Fatal("click promotion changed multi-edit", e.Text())
	}
}
