package main

import (
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
)

func TestLSPNavigationUsesLiveUnicodeBuffer(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "target.go", "package demo\n\nvar Target = 7\n")
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open failed")
	}
	ed := activeEditor(w.tabs)
	text := "package demo\n\n/*😀🧪中文*/ var Target = 7\n"
	ed.ReplaceAllText(text)
	prefix := strings.Split(strings.Split(text, "\n")[2], "Target")[0]
	col16 := len(utf16.Encode([]rune(prefix)))
	openFileInEditorAtLSP(w.tabs, path, core.LSPPosition{Line: 2, Character: col16})
	if ed.CursorLine() != 2 || ed.CursorCol() != len([]rune(prefix)) {
		t.Fatalf("LSP caret = %d:%d, want 2:%d (UTF16 column %d)", ed.CursorLine(), ed.CursorCol(), len([]rune(prefix)), col16)
	}
	if ed.Text() != text || !ed.Modified() {
		t.Fatal("navigation changed unsaved text")
	}
}

func TestLSPReferenceAndDiagnosticColumns(t *testing.T) {
	w := testWorkbench(t)
	text := "package demo\n\n\t/*😀🧪中文*/ var Target = 7\n"
	path := writeFile(t, t.TempDir(), "target.go", text)
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open failed")
	}
	ed := activeEditor(w.tabs)
	lineText := strings.Split(text, "\n")[2]
	prefix := strings.Split(lineText, "Target")[0]
	col16 := len(utf16.Encode([]rune(prefix)))
	want := len([]rune(prefix))
	ref := lspReferenceLocation(path, core.LSPPosition{Line: 2, Character: col16}, referenceSourceLine(path, 2))
	if ref.Line != 3 || ref.Col != want || ref.Preview != strings.TrimSpace(lineText) {
		t.Fatalf("reference: %+v", ref)
	}
	openFileInEditorAt(w.tabs, ref.File, ref.Line, ref.Col)
	if ed.CursorCol() != want {
		t.Fatal("reference navigation did not preserve rune column")
	}
	problems := []ged.Problem{{File: path, Line: 3, Col: col16, Severity: ged.SeverityWarning, Message: "test"}, {File: path, Line: 99, Col: 20}}
	normalizeLSPProblemColumns(problems, ed)
	if problems[0].Col != want || problems[1].Col != 0 || problems[0].Message != "test" || problems[0].Severity != ged.SeverityWarning {
		t.Fatalf("diagnostics: %+v", problems)
	}
	openFileInEditorAt(w.tabs, problems[0].File, problems[0].Line, problems[0].Col)
	if ed.CursorCol() != want {
		t.Fatal("diagnostic navigation did not preserve rune column")
	}
	normalizeLSPProblemColumns(problems, nil)
	if problems[0].Col != 0 {
		t.Fatal("missing buffer column must not be guessed")
	}
}

func TestLSPNavigationOpensDestinationBeforeConverting(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "closed.go", "package demo\n\n/*😀*/ var Target = 7\n")
	prefix := "/*😀*/ var "
	openFileInEditorAtLSP(w.tabs, path, core.LSPPosition{Line: 2, Character: len(utf16.Encode([]rune(prefix)))})
	ed := activeEditor(w.tabs)
	if ed == nil || ed.CursorLine() != 2 || ed.CursorCol() != len([]rune(prefix)) {
		t.Fatal("closed destination not navigated correctly")
	}
	before := w.tabs.Count()
	openFileInEditorAtLSP(w.tabs, "", core.LSPPosition{})
	if w.tabs.Count() != before {
		t.Fatal("non-file target opened a tab")
	}
}
