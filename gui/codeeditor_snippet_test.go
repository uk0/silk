package gui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// CodeEditor + SnippetSet Tab-trigger wiring.
//
// These tests drive tryExpandSnippetAtCursor (the new-style SnippetSet path
// installed by Init via NewGoSnippetSet). The cursor is positioned at the end
// of the trigger word and expansion is invoked; we assert the buffer was
// replaced with the snippet body (with $0 stripped) and the cursor landed
// where $0 sat.
// ---------------------------------------------------------------------------

// placeCursorAtEnd positions the primary cursor at the end of the last line.
func placeCursorAtEnd(e *CodeEditor) {
	e.cursorLine = len(e.lines) - 1
	e.cursorCol = len([]rune(e.lines[e.cursorLine]))
}

// TestCodeEditorTabExpandsIferr expands "iferr" at end-of-buffer and checks
// the iferr boilerplate replaces the trigger, with cursor at the $0 mark.
func TestCodeEditorTabExpandsIferr(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("iferr")
	placeCursorAtEnd(e)

	if !e.tryExpandSnippetAtCursor() {
		t.Fatal("tryExpandSnippetAtCursor returned false; want true")
	}
	want := "if err != nil {\n\treturn err\n}"
	if got := e.Text(); got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if strings.Contains(e.Text(), "$0") {
		t.Error("$0 marker not stripped from buffer")
	}
	// $0 sat right after "return err" inside the body, on line 1 col len("\treturn err").
	if e.cursorLine != 1 {
		t.Errorf("cursorLine = %d, want 1", e.cursorLine)
	}
	if got, want := e.cursorCol, len([]rune("\treturn err")); got != want {
		t.Errorf("cursorCol = %d, want %d", got, want)
	}
}

// TestCodeEditorTabExpandsForrange checks the forrange trigger at EOB.
func TestCodeEditorTabExpandsForrange(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("forrange")
	placeCursorAtEnd(e)

	if !e.tryExpandSnippetAtCursor() {
		t.Fatal("tryExpandSnippetAtCursor returned false; want true")
	}
	want := "for k, v := range  {\n\t\n}"
	if got := e.Text(); got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	// $0 sat after "range " (one space) on the first line.
	if e.cursorLine != 0 {
		t.Errorf("cursorLine = %d, want 0", e.cursorLine)
	}
	if got, want := e.cursorCol, len([]rune("for k, v := range ")); got != want {
		t.Errorf("cursorCol = %d, want %d", got, want)
	}
}

// TestCodeEditorTabExpandsTest checks the "Test" trigger expands a t.T func.
func TestCodeEditorTabExpandsTest(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("Test")
	placeCursorAtEnd(e)

	if !e.tryExpandSnippetAtCursor() {
		t.Fatal("tryExpandSnippetAtCursor returned false; want true")
	}
	got := e.Text()
	if !strings.Contains(got, "func Test") || !strings.Contains(got, "*testing.T") {
		t.Errorf("Text() = %q, missing testing.T signature", got)
	}
}

// TestCodeEditorTabNoMatchPreservesBuffer asserts an unrelated word does not
// expand and the buffer is untouched.
func TestCodeEditorTabNoMatchPreservesBuffer(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("xyzzy")
	placeCursorAtEnd(e)
	before := e.Text()
	beforeLine, beforeCol := e.cursorLine, e.cursorCol

	if e.tryExpandSnippetAtCursor() {
		t.Fatal("tryExpandSnippetAtCursor returned true for non-matching word")
	}
	if e.Text() != before {
		t.Errorf("buffer mutated: %q, want %q", e.Text(), before)
	}
	if e.cursorLine != beforeLine || e.cursorCol != beforeCol {
		t.Errorf("cursor moved: (%d,%d), want (%d,%d)",
			e.cursorLine, e.cursorCol, beforeLine, beforeCol)
	}
}

// TestCodeEditorTabMidWordNoExpand verifies that placing the cursor in the
// middle of an identifier (with identifier runes to the right) does NOT
// expand even when the prefix would otherwise be a valid trigger.
func TestCodeEditorTabMidWordNoExpand(t *testing.T) {
	e := NewCodeEditor()
	// "iferrxyz" — cursor placed after "iferr" but "xyz" follows; not end-of-word.
	e.SetText("iferrxyz")
	e.cursorLine = 0
	e.cursorCol = len([]rune("iferr"))
	before := e.Text()

	if e.tryExpandSnippetAtCursor() {
		t.Fatal("tryExpandSnippetAtCursor returned true mid-word; want false")
	}
	if e.Text() != before {
		t.Errorf("buffer mutated mid-word: %q, want %q", e.Text(), before)
	}
}

// TestCodeEditorTabExpandsWithIndent ensures the snippet body is re-indented
// to the column of the trigger when it sits on an indented line.
func TestCodeEditorTabExpandsWithIndent(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("\t\tiferr")
	placeCursorAtEnd(e)

	if !e.tryExpandSnippetAtCursor() {
		t.Fatal("tryExpandSnippetAtCursor returned false on indented trigger")
	}
	got := e.Text()
	if !strings.HasPrefix(got, "\t\tif err != nil {") {
		t.Errorf("Text() = %q, want leading two tabs preserved", got)
	}
	if !strings.Contains(got, "\n\t\t\treturn err") {
		t.Errorf("Text() = %q, want body lines indented to trigger column", got)
	}
}

// TestCodeEditorTabFiresOnChanged verifies expansion fires the SigChanged hook
// (rebuildText -> onChanged), so hosts observe the new buffer.
func TestCodeEditorTabFiresOnChanged(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("iferr")
	placeCursorAtEnd(e)

	var seen string
	var calls int
	e.SigChanged(func(s string) {
		seen = s
		calls++
	})
	if !e.tryExpandSnippetAtCursor() {
		t.Fatal("expansion failed")
	}
	if calls == 0 {
		t.Fatal("SigChanged callback not invoked")
	}
	if seen != e.Text() {
		t.Errorf("SigChanged saw %q, current Text() = %q", seen, e.Text())
	}
}

// TestCodeEditorSetSnippetsOverride installs an empty SnippetSet and confirms
// it disables the new-style path (returns false even on a valid prefix).
func TestCodeEditorSetSnippetsOverride(t *testing.T) {
	e := NewCodeEditor()
	e.SetSnippets(&SnippetSet{byTrig: map[string]*Snippet{}})
	e.SetText("iferr")
	placeCursorAtEnd(e)

	if e.tryExpandSnippetAtCursor() {
		t.Fatal("empty SnippetSet should not expand; got true")
	}
	if e.Text() != "iferr" {
		t.Errorf("Text() = %q, want unchanged 'iferr'", e.Text())
	}

	// Restoring the default set re-enables expansion.
	e.SetSnippets(NewGoSnippetSet())
	if !e.tryExpandSnippetAtCursor() {
		t.Fatal("restored default SnippetSet failed to expand")
	}
}
