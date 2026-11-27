package gui

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func TestEditorLineByteIndexMatchesExactDenseOffsets(t *testing.T) {
	r := rand.New(rand.NewSource(51233))
	corpus := []string{"", "a", "😀", "x\r", "x\r\r", "\xc2\xff\x80", "é中文"}
	var index editorLineByteIndex
	for _, count := range []int{0, 1, 127, 128, 129, 255, 256, 257, 1000} {
		lines := make([]string, count)
		for i := range lines {
			lines[i] = corpus[r.Intn(len(corpus))]
		}
		index.reset(lines)
		if count == 0 {
			if got := index.cursorAtOffset(lines, 500); got != (cursorPos{}) {
				t.Fatalf("zero rows: %v", got)
			}
			continue
		}
		check := func() {
			t.Helper()
			starts := lineByteStarts(lines)
			for row, want := range starts {
				if got := index.lineStart(lines, row); got != want {
					t.Fatalf("%d rows, row %d: start %d want %d", count, row, got, want)
				}
			}
			eof := starts[count-1] + len(lines[count-1])
			for off := -2; off <= eof+2; off++ {
				want := cursorAtByteOffset(lines, starts, off)
				if got := index.cursorAtOffset(lines, off); got != want {
					t.Fatalf("%d rows, offset %d: position %v want %v", count, off, got, want)
				}
			}
		}
		check()
		for i := 0; i < 12; i++ {
			row := r.Intn(count)
			change := corpus[r.Intn(len(corpus))]
			index.applyRowByteDelta(row, len(change)-len(lines[row]))
			lines[row] = change
		}
		check()
	}
}

func TestEditorIndexedCaretGroupLeavesPublishedIndexUnchanged(t *testing.T) {
	e := NewCodeEditor()
	original := strings.Repeat("x😀\r\n", 4000)
	e.SetText(original)
	model := strings.Split(original, "\n")
	saved := slices.Clone(e.textSnapshot.blocks)
	const row = 129
	e.ScrollToLineCol(row, 1)
	e.AddCursorAtLine(128, 1)
	e.AddCursorAtLine(2003, 1)
	e.editAtAllCursors(byteCursorInsert, "中")
	if !slices.Equal(saved, e.textSnapshot.blocks) {
		t.Fatal("caret mapping mutated the old published block totals")
	}
	if e.cursorLine != row || e.cursorCol != 2 || len(e.additionalCursors) != 2 {
		t.Fatalf("unexpected carets: %+v %+v", cursorPos{e.cursorLine, e.cursorCol}, e.additionalCursors)
	}
	for _, i := range []int{row, 128, 2003} {
		model[i] = "x中😀\r"
	}
	e.rebuildText()
	expected := strings.Join(model, "\n")
	assertEditorBytes(t, e, expected)
	e.Undo()
	assertEditorBytes(t, e, original)
	e.Redo()
	assertEditorBytes(t, e, expected)
}

func TestEditorIndexedCaretGroupRebuildsAfterCRLFRemoval(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("a😀\r\n", 1500) + "end")
	// Both carets delete a CRLF; the second moves up by one row because
	// the first structural edit shortens the prefix.
	e.ScrollToLineCol(126, 2)
	e.AddCursorAtLine(511, 2)
	e.editAtAllCursors(byteCursorDelete, "")
	e.rebuildText()
	if e.cursorLine != 126 || e.cursorCol != 2 ||
		len(e.additionalCursors) != 1 || e.additionalCursors[0] != (cursorPos{510, 2}) {
		t.Fatalf("joined-row carets: %d:%d, %v", e.cursorLine, e.cursorCol, e.additionalCursors)
	}
	if e.LineEndings().CRLF != 1498 || !strings.Contains(e.Text(), "a😀a😀\r\n") {
		t.Fatal("joined CRLF text did not match")
	}
}
