package gui

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func TestEditorCaretOffsetSweepMatchesQuadraticReference(t *testing.T) {
	r := rand.New(rand.NewSource(51282))
	for run := 0; run < 500; run++ {
		carets, count := 2+r.Intn(140), 1+r.Intn(160)
		offsets := make([]int, carets)
		edits := make([]editorByteEdit, count)
		for i := range offsets {
			offsets[i] = r.Intn(600) - 3
		}
		for i := range edits {
			edits[i] = editorByteEdit{offset: r.Intn(550),
				before: strings.Repeat("b", r.Intn(17)), after: strings.Repeat("a", r.Intn(20))}
			// Include duplicate starts, zero-length inserts, touching
			// ranges and carets inside a deletion boundary.
			if i < len(offsets) {
				offsets[i] = edits[i].offset + r.Intn(len(edits[i].before)+1)
			}
		}
		want := slices.Clone(offsets)
		for i, off := range offsets {
			for _, edit := range edits {
				if off < edit.offset {
					continue
				}
				if off <= edit.offset+len(edit.before) {
					want[i] += len(edit.after) - (off - edit.offset)
				} else {
					want[i] += len(edit.after) - len(edit.before)
				}
			}
		}
		shiftEditorCaretOffsets(offsets, edits)
		if !slices.Equal(offsets, want) {
			t.Fatalf("run %d: sweep offsets differ from the reference", run)
		}
	}
}

func TestEditorDenseCaretSweepMaintainsOrderAndHistory(t *testing.T) {
	e := NewCodeEditor()
	before := strings.Repeat("a😀\r\n", 2100)
	e.SetText(before)
	e.ScrollToLineCol(40, 1)
	for i := 1; i < 400; i++ {
		e.AddCursorAtLine(40+3*i, 1)
	}
	if e.CursorCount() != 400 {
		t.Fatal("fixture does not use a dense edit group")
	}
	want := strings.Split(before, "\n")
	for i := 0; i < 400; i++ {
		want[40+3*i] = "a中😀\r"
	}
	expected := strings.Join(want, "\n")
	e.OnTextInput("中")
	if e.cursorLine != 40 || e.cursorCol != 2 || len(e.additionalCursors) != 399 {
		t.Fatal("primary cursor or secondary cursor count changed")
	}
	for i, p := range e.additionalCursors {
		if p != (cursorPos{40 + 3*(i+1), 2}) {
			t.Fatalf("caret %d: %v", i, p)
		}
	}
	assertEditorBytes(t, e, expected)
	e.Undo()
	assertEditorBytes(t, e, before)
	e.Redo()
	assertEditorBytes(t, e, expected)
}
