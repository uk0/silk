package gui

import (
	"strings"
	"testing"
)

func TestSnippetExpansionHistoryPreservesWholeDocument(t *testing.T) {
	for _, trigger := range []string{"iferr", "fn"} {
		t.Run(trigger, func(t *testing.T) {
			e := NewCodeEditor()
			e.SetSize(800, 500)
			line := "\t/* 你好😀 */ " + trigger
			original := "package p\n\nfunc f() {\n" + line + "\n\tprintln(\"tail\")\n}\n"
			e.SetText(original)
			e.ScrollToLineCol(3, len([]rune(line)))
			beforeLine, beforeCol := e.cursorLine, e.cursorCol
			e.WantsTab(false)
			expanded := e.Text()
			if expanded == original || !strings.Contains(expanded, "println(\"tail\")") {
				t.Fatal("fixture did not expand", expanded)
			}
			afterLine, afterCol := e.cursorLine, e.cursorCol
			e.MarkSaved()
			for i := 0; i < 2; i++ {
				e.Undo()
				if e.Text() != original {
					t.Fatalf("undo %d: %q, want %q", i, e.Text(), original)
				}
				if l, c := e.cursorLine, e.cursorCol; l != beforeLine || c != beforeCol {
					t.Fatalf("undo caret: %d,%d want %d,%d", l, c, beforeLine, beforeCol)
				}
				if e.snippetSess != nil {
					t.Fatal("undo retained byte ranges from expanded document")
				}
				e.Redo()
				if e.Text() != expanded {
					t.Fatalf("redo %d: %q, want %q", i, e.Text(), expanded)
				}
				if l, c := e.cursorLine, e.cursorCol; l != afterLine || c != afterCol {
					t.Fatalf("redo caret: %d,%d want %d,%d", l, c, afterLine, afterCol)
				}
			}
		})
	}
}
