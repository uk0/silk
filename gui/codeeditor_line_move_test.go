package gui

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEditorMoveLinesKeepsUndoHistory(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	e.ReplaceAllText("a\nb\nc!")
	e.cursorLine = 1
	e.moveLineDown()
	if e.Text() != "a\nc!\nb" {
		t.Fatal(e.Text())
	}
	e.Undo()
	if e.Text() != "a\nb\nc!" {
		t.Fatalf("move has no Undo entry: %q", e.Text())
	}
	e.Redo()
	if e.Text() != "a\nc!\nb" {
		t.Fatal("redo", e.Text())
	}
	e.Undo()
	e.Undo()
	if e.Text() != "a\nb\nc" {
		t.Fatal("lost older edit")
	}
}

func TestEditorMoveLinesMovesSelectionBlock(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc\nd")
	e.hasSelection = true
	e.selStartLine, e.selEndLine = 1, 3
	e.cursorLine = 3
	e.moveLineUp()
	if e.Text() != "b\nc\na\nd" || !e.hasSelection || e.selStartLine != 0 || e.selEndLine != 2 {
		t.Fatalf("selection block not moved: %q, %v", e.Text(), e.selectionSnapshot())
	}
}

func TestEditorMoveLinesPreservesFinalNewline(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\r\nb\r\n")
	e.cursorLine = 1
	e.moveLineDown()
	if e.Text() != "a\r\nb\r\n" || len(e.undoStack) != 0 {
		t.Fatalf("moved into EOF sentinel: %q", e.Text())
	}
}

func TestEditorMoveLinesSelectionRoundTripsAndEOF(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, source := range []string{"a\n中🙂\nc\nd", "a\n中🙂\nc\nd\n"} {
			e := NewCodeEditor()
			e.SetText(source)
			e.hasSelection = true
			e.selStartLine, e.selStartCol = 1, 1
			e.selEndLine, e.selEndCol = 3, 0
			if reverse {
				e.selStartLine, e.selEndLine = e.selEndLine, e.selStartLine
				e.selStartCol, e.selEndCol = e.selEndCol, e.selStartCol
			}
			e.cursorLine, e.cursorCol = e.selEndLine, e.selEndCol
			before := e.selectionSnapshot()
			caret := cursorPos{e.cursorLine, e.cursorCol}
			changed := 0
			e.SigChanged(func(string) { changed++ })
			e.MoveLinesDown()
			want := "a\nd\n中🙂\nc"
			if strings.HasSuffix(source, "\n") {
				want += "\n"
			}
			if e.Text() != want || changed != 1 || !e.hasSelection {
				t.Fatal(e.Text(), changed)
			}
			after := e.selectionSnapshot()
			if !strings.HasSuffix(source, "\n") {
				end := after.end
				if reverse {
					end = after.start
				}
				if end != (cursorPos{3, 1}) {
					t.Fatal("EOF endpoint", end)
				}
			}
			e.Undo()
			if e.Text() != source || e.selectionSnapshot() != before || (cursorPos{e.cursorLine, e.cursorCol}) != caret {
				t.Fatal("undo selection")
			}
			e.Redo()
			if e.Text() != want || e.selectionSnapshot() != after || changed != 3 {
				t.Fatal("redo selection")
			}
		}
	}
}
func TestEditorMoveLinesMultipleCaretGroups(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("0\n1\n2\n3\n4\n5\n6\n")
	e.cursorLine = 0
	e.additionalCursors = []cursorPos{{2, 1}, {3, 0}, {5, 1}, {2, 0}, {7, 0}}
	before := append([]cursorPos{{0, 0}}, e.additionalCursors...)
	if e.CanMoveLinesUp() != true {
		t.Fatal("boundary group blocked other groups")
	}
	e.MoveLinesUp()
	if e.Text() != "0\n2\n3\n1\n5\n4\n6\n" {
		t.Fatal(e.Text())
	}
	after := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
	want := []cursorPos{{0, 0}, {1, 1}, {2, 0}, {4, 1}, {1, 0}, {7, 0}}
	if !reflect.DeepEqual(after, want) {
		t.Fatal(after, want)
	}
	e.Undo()
	if !reflect.DeepEqual(append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...), before) {
		t.Fatal("undo carets")
	}
	e.Redo()
	if !reflect.DeepEqual(append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...), want) {
		t.Fatal("redo carets")
	}
	e.SetText("a\nb\nc\n")
	e.cursorLine = 2
	e.additionalCursors = []cursorPos{{3, 0}}
	e.MoveLinesUp()
	if e.Text() != "a\nc\nb\n" {
		t.Fatal("EOF secondary blocked valid caret", e.Text())
	}
}
func TestEditorMoveLinesBoundaryNoSideEffects(t *testing.T) {
	for _, source := range []string{"", "one", "one\n", "a\nb\n"} {
		e := NewCodeEditor()
		e.SetText(source)
		e.MarkSaved()
		e.cursorLine = len(e.lines) - 1
		e.snippetSess = &snippetSession{}
		session := e.snippetSess
		changed := 0
		e.SigChanged(func(string) { changed++ })
		revision := e.TextRevision()
		if e.CanMoveLinesDown() {
			t.Fatal("EOF enabled")
		}
		e.MoveLinesDown()
		e.cursorLine = 0
		if e.CanMoveLinesUp() {
			t.Fatal("BOF enabled")
		}
		e.MoveLinesUp()
		if e.Text() != source || e.Modified() || e.TextRevision() != revision || len(e.undoStack) != 0 || changed != 0 || e.snippetSess != session {
			t.Fatal("boundary changed state")
		}
	}
}
func TestLineMoveRotationKeepsSeparatorSlots(t *testing.T) {
	// Exhaustive small combinations include invalid bytes and a literal CR at EOF.
	parts := []string{"", "a", "中🙂", "\xff", "x\r", "\r"}
	for _, a := range parts {
		for _, b := range parts {
			for _, c := range parts {
				original := []string{a, b, c}
				for _, down := range []bool{false, true} {
					lines := append([]string(nil), original...)
					m := editorLineMove{lo: 0, hi: 2, down: down}
					m.captureSeparators(lines)
					bodies := append([]string(nil), original...)
					for i := 0; i < 2; i++ {
						bodies[i] = strings.TrimSuffix(bodies[i], "\r")
					}
					order := []int{1, 2, 0}
					if down {
						order = []int{2, 0, 1}
					}
					m.apply(lines, false)
					for i, from := range order {
						want := bodies[from]
						if i < 2 && strings.HasSuffix(original[i], "\r") {
							want += "\r"
						}
						if lines[i] != want {
							t.Fatalf("move %q down=%v got=%q want=%q", original, down, lines[i], want)
						}
					}
					m.apply(lines, true)
					if !reflect.DeepEqual(lines, original) {
						t.Fatal("inverse bytes", original, lines)
					}
				}
			}
		}
	}
}
func TestEditorMoveLinesByteExactHistoryAndTransientState(t *testing.T) {
	for _, source := range []string{"a\r\n中\xff🙂\r\nc", "a\nb\r\nc\r", "a\r\nb\nc\n"} {
		e := NewCodeEditor()
		e.SetText(source)
		e.cursorLine = 2
		e.cursorCol = utf8.RuneCountInString(e.lines[2])
		e.hasSelection = true
		e.selStartLine = 1
		e.selEndLine = 2
		e.selEndCol = 1
		if err := e.SetFindInSelection(true); err != nil {
			t.Fatal(err)
		}
		e.snippetSess = &snippetSession{}
		e.renameActive = true
		e.gotoLineActive = true
		originalSelection := e.selectionSnapshot()
		e.MoveLinesUp()
		after := e.Text()
		if e.snippetSess != nil || e.renameActive || e.gotoLineActive || e.findState.scope.valid || !e.findState.scope.enabled {
			t.Fatal("stale transient/scope state")
		}
		e.Undo()
		if e.Text() != source || e.selectionSnapshot() != originalSelection {
			t.Fatal("exact undo", e.Text())
		}
		e.Redo()
		if e.Text() != after {
			t.Fatal("exact redo")
		}
	}
}
func TestEditorMoveLinesHistoryRetainsNoDocumentBodies(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("small\n" + strings.Repeat("x", 1<<20) + "\nlast")
	e.cursorLine = 1
	e.MoveLinesUp()
	a := e.undoStack[len(e.undoStack)-1]
	if a.kind != 5 || a.text != "" || a.oldText != "" || len(a.group) != 0 || len(a.lineMoves) != 1 || len(a.lineMoves[0].crlf) != 0 {
		t.Fatal("line move retained document text")
	}
	e.Undo()
	if !strings.HasPrefix(e.Text(), "small\n") {
		t.Fatal("undo")
	}
}
func TestEditorMoveLinesUnfoldsAffectedRegion(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("func f() {\n a\n b\n}\nlast")
	e.ToggleFold(0)
	e.cursorLine = 2
	e.MoveLinesUp()
	if e.foldedLines[0] || e.Text() != "func f() {\n b\n a\n}\nlast" {
		t.Fatal("moved text remained hidden")
	}
}
func BenchmarkEditorLineMoveRotationLargeBodies(b *testing.B) {
	lines := []string{strings.Repeat("a", 1<<20), strings.Repeat("b", 1<<20), "c"}
	m := editorLineMove{lo: 0, hi: 1}
	m.captureSeparators(lines)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.apply(lines, false)
		m.apply(lines, true)
	}
}

func TestLineMoveSeparatorBitsetBoundaries(t *testing.T) {
	for _, count := range []int{64, 65, 127, 128, 129} {
		original := make([]string, count)
		for i := range original {
			original[i] = "x"
			if i%3 == 0 {
				original[i] += "\r"
			}
		}
		for _, down := range []bool{false, true} {
			lines := append([]string(nil), original...)
			m := editorLineMove{lo: 1, hi: count - 1, down: down}
			m.captureSeparators(lines)
			if len(m.crlf) != (count-1+63)/64 {
				t.Fatal("mask length", count, len(m.crlf))
			}
			m.apply(lines, false)
			m.apply(lines, true)
			if !reflect.DeepEqual(lines, original) {
				t.Fatal("bitset boundary", count, down)
			}
		}
	}
}
