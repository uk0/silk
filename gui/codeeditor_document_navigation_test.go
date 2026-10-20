package gui

import (
	"strings"
	"testing"
)

func TestCodeEditorDocumentMotionSelectionAndViewport(t *testing.T) {
	editor := NewCodeEditor()
	editor.SetSize(200, 100)
	text := strings.Repeat("line\n", 30) + "尾😀"
	editor.SetText(text)
	editor.cursorLine, editor.cursorCol = 10, 1
	editor.AddCursorAtLine(20, 1)
	changes := 0
	editor.onChanged = func(string) { changes++ }
	undoCount := len(editor.undoStack)
	editor.moveToDocumentBoundary(true, true)
	if editor.cursorLine != 30 || editor.cursorCol != 2 || editor.selStartLine != 10 || editor.selStartCol != 1 || !editor.HasSelection() {
		t.Fatal("document motion lost rune position or selection anchor")
	}
	if editor.scrollY <= 0 || len(editor.additionalCursors) != 0 {
		t.Fatal("document end stayed offscreen or retained secondary insertion points")
	}
	editor.moveToDocumentBoundary(false, false)
	if editor.cursorLine != 0 || editor.cursorCol != 0 || editor.scrollY != 0 || editor.HasSelection() {
		t.Fatal("document start did not reset viewport/selection")
	}
	if editor.Text() != text || changes != 0 || len(editor.undoStack) != undoCount {
		t.Fatal("navigation changed text, emitted a change or created undo history")
	}
}

func TestCodeEditorDocumentMotionRevealsFoldedEOFAndEmptyFile(t *testing.T) {
	editor := NewCodeEditor()
	editor.SetText("func example() {\n call()\n}")
	editor.FoldAll()
	editor.moveToDocumentBoundary(true, false)
	if editor.IsFolded(0) || editor.cursorLine != 2 || editor.cursorCol != 1 {
		t.Fatal("EOF remained hidden inside a collapsed region")
	}
	editor.SetText("")
	editor.moveToDocumentBoundary(true, true)
	editor.moveToDocumentBoundary(false, false)
	if editor.cursorLine != 0 || editor.cursorCol != 0 || editor.HasSelection() || editor.Text() != "" {
		t.Fatal("empty document boundary is invalid")
	}
}
