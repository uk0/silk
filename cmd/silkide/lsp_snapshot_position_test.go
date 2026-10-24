package main

import (
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestLSPInputSnapshotCoordinatesAndCompletionPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, text    string
		col, utf16col int
		complete      bool
	}{
		{"start", "中文😀fmt.Pr", 0, 0, false},
		{"emoji", "中文😀fmt.Pr", 3, 4, false},
		{"letter", "中文😀fmt.Pr", 9, 10, true},
		{"cjk", "中文😀fmt.Pr", 2, 2, true},
		{"astral_letter", "a𐐀", 2, 3, true},
		{"dot", "中文😀fmt.", 7, 8, true},
		{"underscore", "中文😀_", 4, 5, true},
		{"digit", "中文😀1", 4, 5, true},
		{"space", "中文😀 ", 4, 5, false},
		{"nul", "中文😀\x00", 4, 5, false},
		{"invalid", "a\xff", 2, 2, false},
		{"combining", "e\u0301", 2, 2, false},
		{"punctuation", "中文😀(", 4, 5, false},
		{"empty", "", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const path = "/silk-qa/snapshot.go"
			d := installLSPSnapshotProbe(t, path)
			e := gui.NewCodeEditor()
			e.SetText("package p\n" + tc.text + "\n")
			e.ScrollToLineCol(1, tc.col)
			revision := e.TextRevision()
			text := e.Text()
			updateLSPDocument(path, e, text)
			if d.latest.text != text || d.latest.line != 1 || d.latest.col != tc.utf16col || d.latest.complete != tc.complete || d.latest.version != 2 {
				t.Fatalf("snapshot: %+v", d.latest)
			}
			if e.TextRevision() != revision || e.Text() != text {
				t.Fatal("LSP snapshot mutated source")
			}
			updateLSPDocument(path, e, text)
			if d.latest.version != 3 || len(d.wake) != 1 {
				t.Fatal("versioning/coalescing changed")
			}
		})
	}
}

func TestLSPInputSnapshotMismatchAndMissingEditorStillSynchronize(t *testing.T) {
	const path = "/silk-qa/snapshot.go"
	d := installLSPSnapshotProbe(t, path)
	e := gui.NewCodeEditor()
	e.SetText("fmt.Pr")
	e.ScrollToLineCol(0, 6)
	updateLSPDocument(path, e, "other")
	if d.latest.text != "other" || d.latest.complete {
		t.Fatal("mismatched text requested completion with editor coordinates")
	}
	updateLSPDocument(path, nil, "no editor")
	if d.latest.text != "no editor" || d.latest.complete || d.latest.version != 3 {
		t.Fatal("nil editor synchronization failed")
	}
}

func TestLSPCompletionFallbackUnicodePrefixAndStaleCaret(t *testing.T) {
	prefix := "package p\n" + strings.Repeat("// line\n", 5000)
	text := prefix + "func f(){ /*中文😀*/ 变量.Pr }\n"
	e := gui.NewCodeEditor()
	e.SetText(text)
	// Exact rune and UTF-16 positions after Pr, including an astral prefix.
	line := 5001
	column := len([]rune("func f(){ /*中文😀*/ 变量.Pr"))
	e.ScrollToLineCol(line, column)
	s := lspSnapshot{text: text, line: line, col: e.CursorUTF16Col()}
	if err := applyLSPCompletion(e, s, core.LSPCompletionItem{Label: "Println", InsertText: "Println"}); err != nil {
		t.Fatal(err)
	}
	want := prefix + "func f(){ /*中文😀*/ 变量.Println }\n"
	if e.Text() != want {
		t.Fatal("fallback replaced wrong Unicode range")
	}
	e.Undo()
	if e.Text() != text {
		t.Fatal("fallback was not one undo action")
	}
	e.Redo()
	if e.Text() != want {
		t.Fatal("redo failed")
	}
	e.SetText(text)
	e.ScrollToLineCol(line, 0)
	if err := applyLSPCompletion(e, s, core.LSPCompletionItem{Label: "wrong"}); err != nil || e.Text() != text {
		t.Fatal("stale caret edited current buffer", err)
	}
	s.line = 99999
	if err := applyLSPCompletion(e, s, core.LSPCompletionItem{Label: "wrong"}); err != nil || e.Text() != text {
		t.Fatal("malformed line edited buffer", err)
	}
}
