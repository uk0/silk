package gui

import (
	"testing"
)

// ---------------------------------------------------------------------------
// CodeEditor shortcut surface (public methods, no GL / no Draw)
//
// IsKeyDown polls live keyboard state, which is unreachable in a headless
// test. So we exercise the public methods the OnKeyDown cases dispatch to:
// GoToDefinitionAtCursor / HighlightReferencesAtCursor / FoldAll / UnfoldAll.
// ---------------------------------------------------------------------------

// TestCodeEditorGoToDefinitionAtCursor: placing the cursor on a use site of a
// top-level identifier and invoking GoToDefinitionAtCursor must move the caret
// to the declaration line resolved by FindDefinition.
func TestCodeEditorGoToDefinitionAtCursor(t *testing.T) {
	e := NewCodeEditor()
	src := "package p\n" + // 0
		"\n" + // 1
		"func target() int {\n" + // 2  <- declaration
		"\treturn 0\n" + // 3
		"}\n" + // 4
		"\n" + // 5
		"func caller() int {\n" + // 6
		"\treturn target()\n" + // 7  <- use site
		"}\n" // 8
	e.SetText(src)

	// Place the cursor inside the identifier "target" on the use line.
	e.cursorLine = 7
	e.cursorCol = 9 // somewhere inside "target"

	e.GoToDefinitionAtCursor()

	if e.cursorLine != 2 {
		t.Errorf("GoToDefinitionAtCursor: cursor at line %d, want 2 (decl of target)", e.cursorLine)
	}
}

// TestCodeEditorGoToDefinitionAtCursorEmptyWord: cursor not on an identifier
// must be a no-op (no panic, no movement).
func TestCodeEditorGoToDefinitionAtCursorEmptyWord(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\n\nfunc f() {}\n")
	// Position on the blank line 1, col 0 — no identifier.
	e.cursorLine = 1
	e.cursorCol = 0

	e.GoToDefinitionAtCursor()

	if e.cursorLine != 1 || e.cursorCol != 0 {
		t.Errorf("no-identifier GoToDefinitionAtCursor moved cursor to %d:%d, want 1:0",
			e.cursorLine, e.cursorCol)
	}
}

// TestCodeEditorHighlightReferencesAtCursor: every occurrence of the
// identifier in the buffer must land in findMatches, ready for the find-bar
// overlay to render.
func TestCodeEditorHighlightReferencesAtCursor(t *testing.T) {
	e := NewCodeEditor()
	src := "package p\n" + // 0
		"\n" + // 1
		"func target() int { return 0 }\n" + // 2
		"\n" + // 3
		"func a() int { return target() }\n" + // 4
		"func b() int { return target() + target() }\n" // 5
	e.SetText(src)

	// Cursor sits on the declaration of "target" itself.
	e.cursorLine = 2
	e.cursorCol = 6 // "func t|arget"

	e.HighlightReferencesAtCursor()

	// Expected occurrences of "target": line 2 decl, line 4 once, line 5 twice = 4.
	if got, want := len(e.findMatches), 4; got != want {
		t.Fatalf("HighlightReferencesAtCursor: findMatches=%d, want %d (%v)", got, want, e.findMatches)
	}
	if e.findText != "target" {
		t.Errorf("findText=%q, want %q", e.findText, "target")
	}
	if !e.findActive {
		t.Errorf("findActive=false, want true so the overlay actually paints")
	}
	// Spot-check the first match lands on the declaration's column (after "func ").
	first := e.findMatches[0]
	if first.line != 2 || first.col != 5 {
		t.Errorf("first match = line %d col %d, want line 2 col 5", first.line, first.col)
	}
	// end column must mark the identifier's right edge.
	if got := first.end - first.col; got != len("target") {
		t.Errorf("match width = %d, want %d", got, len("target"))
	}
}

// TestCodeEditorHighlightReferencesAtCursorEmptyWord: a cursor on whitespace
// must clear nothing and not panic.
func TestCodeEditorHighlightReferencesAtCursorEmptyWord(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\n\nfunc f() {}\n")
	e.cursorLine = 1
	e.cursorCol = 0

	e.HighlightReferencesAtCursor()

	if len(e.findMatches) != 0 {
		t.Errorf("expected no matches for empty word, got %v", e.findMatches)
	}
}

// TestCodeEditorFoldAllUnfoldAllShortcuts: FoldAll must mark every region
// folded; UnfoldAll must clear them. Mirrors the public-API contract the
// Cmd/Ctrl+Shift+[ / ] shortcuts depend on.
func TestCodeEditorFoldAllUnfoldAllShortcuts(t *testing.T) {
	e := NewCodeEditor()
	// Nested braces: outer 0..5, inner 1..3 — same shape as the existing
	// fold tests, kept self-contained here for the shortcut surface.
	e.SetText("func f() {\n\tif a {\n\t\tg()\n\t}\n\treturn\n}\n")

	regs := e.FoldRegions()
	if len(regs) != 2 {
		t.Fatalf("want 2 fold regions, got %d (%v)", len(regs), regs)
	}

	e.FoldAll()
	for _, r := range regs {
		if !e.IsFolded(r.startLine) {
			t.Errorf("FoldAll: region starting at %d not folded", r.startLine)
		}
	}

	e.UnfoldAll()
	for _, r := range regs {
		if e.IsFolded(r.startLine) {
			t.Errorf("UnfoldAll: region starting at %d still folded", r.startLine)
		}
	}
}
