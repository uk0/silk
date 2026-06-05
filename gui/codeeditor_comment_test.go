package gui

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Line-comment toggle (Cmd/Ctrl+/).
//
// toggleComment is a pure helper (no GL / no Draw): it takes the affected slice
// of lines plus the comment token and returns the transformed slice. The public
// ToggleLineComment method drives it against the editor's buffer + selection.
//
// Rules under test (Qt Creator / VS Code semantics):
//   - prefix is inserted at each line's first non-whitespace column (indentation
//     preserved);
//   - blank / whitespace-only lines are ignored for the comment/uncomment
//     decision and are never commented;
//   - if every non-blank line is already commented, the range is uncommented,
//     otherwise it is commented.
// ---------------------------------------------------------------------------

const testCommentPrefix = "// "

func TestToggleCommentSingleUncommented(t *testing.T) {
	got := toggleComment([]string{"foo()"}, testCommentPrefix)
	want := []string{"// foo()"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toggleComment = %q, want %q", got, want)
	}
}

func TestToggleCommentSingleCommented(t *testing.T) {
	got := toggleComment([]string{"// foo()"}, testCommentPrefix)
	want := []string{"foo()"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toggleComment = %q, want %q", got, want)
	}
}

// A line commented without the conventional trailing space ("//x") must still
// uncomment cleanly back to "x".
func TestToggleCommentNoSpaceUncomment(t *testing.T) {
	got := toggleComment([]string{"//foo()"}, testCommentPrefix)
	want := []string{"foo()"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toggleComment = %q, want %q", got, want)
	}
}

// Mixed range (some lines commented, some not) commits to commenting ALL lines,
// matching VS Code / Qt Creator: only an all-commented range uncomments.
func TestToggleCommentMixedAllCommented(t *testing.T) {
	got := toggleComment([]string{"a()", "// b()", "c()"}, testCommentPrefix)
	want := []string{"// a()", "// // b()", "// c()"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toggleComment = %q, want %q", got, want)
	}
}

// An already fully-commented range uncomments every line.
func TestToggleCommentAllCommentedUncomments(t *testing.T) {
	got := toggleComment([]string{"// a()", "// b()"}, testCommentPrefix)
	want := []string{"a()", "b()"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toggleComment = %q, want %q", got, want)
	}
}

// Indentation is preserved: the token is inserted at the first non-whitespace
// column, not column 0.
func TestToggleCommentPreservesIndent(t *testing.T) {
	got := toggleComment([]string{"\tx := 1", "    y := 2"}, testCommentPrefix)
	want := []string{"\t// x := 1", "    // y := 2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toggleComment = %q, want %q", got, want)
	}
	// Round-trip back to the original.
	back := toggleComment(got, testCommentPrefix)
	orig := []string{"\tx := 1", "    y := 2"}
	if !reflect.DeepEqual(back, orig) {
		t.Errorf("round-trip = %q, want %q", back, orig)
	}
}

// Blank / whitespace-only lines are skipped: they are not commented, and they do
// not block the "all commented?" decision for the surrounding non-blank lines.
func TestToggleCommentBlankLinesSkipped(t *testing.T) {
	// Comment direction: blanks stay blank, non-blank lines get the token.
	got := toggleComment([]string{"a()", "", "  ", "b()"}, testCommentPrefix)
	want := []string{"// a()", "", "  ", "// b()"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("comment with blanks = %q, want %q", got, want)
	}

	// Uncomment direction: a range whose only non-blank lines are commented is
	// treated as fully commented and gets uncommented; blanks are untouched.
	got2 := toggleComment([]string{"// a()", "", "// b()"}, testCommentPrefix)
	want2 := []string{"a()", "", "b()"}
	if !reflect.DeepEqual(got2, want2) {
		t.Errorf("uncomment with blanks = %q, want %q", got2, want2)
	}
}

// A range containing only blank lines is returned unchanged.
func TestToggleCommentAllBlankNoop(t *testing.T) {
	in := []string{"", "   ", "\t"}
	got := toggleComment(in, testCommentPrefix)
	want := []string{"", "   ", "\t"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toggleComment = %q, want %q", got, want)
	}
}

// toggleComment must not mutate its input slice.
func TestToggleCommentDoesNotMutateInput(t *testing.T) {
	in := []string{"a()", "b()"}
	_ = toggleComment(in, testCommentPrefix)
	orig := []string{"a()", "b()"}
	if !reflect.DeepEqual(in, orig) {
		t.Errorf("input mutated to %q, want %q", in, orig)
	}
}

// ---------------------------------------------------------------------------
// Public API: ToggleLineComment against the editor buffer.
// ---------------------------------------------------------------------------

// Cursor-line toggle: with no selection, only the cursor's line is affected, and
// the changed callback fires with the new text.
func TestToggleLineCommentCursorLine(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a()\nb()\nc()")
	e.cursorLine = 1

	var fired string
	e.SigChanged(func(s string) { fired = s })

	e.ToggleLineComment()

	want := "a()\n// b()\nc()"
	if got := e.Text(); got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if fired != want {
		t.Errorf("changed callback got %q, want %q", fired, want)
	}
	if e.cursorLine != 1 {
		t.Errorf("cursorLine = %d, want 1", e.cursorLine)
	}

	// Toggling again on the same line removes the comment.
	e.ToggleLineComment()
	if got := e.Text(); got != "a()\nb()\nc()" {
		t.Errorf("second toggle Text() = %q, want %q", got, "a()\nb()\nc()")
	}
}

// Selection toggle: every line spanned by the selection is commented, and the
// selection is restored to span the same lines after the edit.
func TestToggleLineCommentSelectionAllLines(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a()\nb()\nc()\nd()")

	// Select lines 1..2.
	e.selStartLine, e.selStartCol = 1, 0
	e.selEndLine, e.selEndCol = 2, 3
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 2, 3

	e.ToggleLineComment()

	want := "a()\n// b()\n// c()\nd()"
	if got := e.Text(); got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if !e.HasSelection() {
		t.Fatalf("selection should be preserved after toggle")
	}
	if e.selStartLine != 1 || e.selEndLine != 2 {
		t.Errorf("selection lines = %d..%d, want 1..2", e.selStartLine, e.selEndLine)
	}

	// Toggling the same selection again uncomments both lines.
	e.ToggleLineComment()
	if got := e.Text(); got != "a()\nb()\nc()\nd()" {
		t.Errorf("second toggle Text() = %q, want %q", got, "a()\nb()\nc()\nd()")
	}
}

// A mixed selection commits to commenting all spanned lines on the first toggle.
func TestToggleLineCommentSelectionMixed(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a()\n// b()\nc()")

	e.selStartLine, e.selStartCol = 0, 0
	e.selEndLine, e.selEndCol = 2, 3
	e.hasSelection = true
	e.cursorLine, e.cursorCol = 2, 3

	e.ToggleLineComment()

	want := "// a()\n// // b()\n// c()"
	if got := e.Text(); got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
}
