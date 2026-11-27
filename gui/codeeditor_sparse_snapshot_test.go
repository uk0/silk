package gui

import (
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

func TestEditorSparseSnapshotMixedOrderAndUndo(t *testing.T) {
	e := NewCodeEditor()
	original := "package p\n" + strings.Repeat("// 中文🧩\r\n", 100_000)
	e.SetText(original)
	e.ScrollToLineCol(129, 3)
	for _, row := range []int{128, 127, 260, 99_999} {
		e.AddCursorAtLine(row, 3)
	}
	wantRows := strings.Split(original, "\n")
	for _, row := range []int{127, 128, 129, 260, 99_999} {
		wantRows[row] = "// inserted中文🧩\r"
	}
	want := strings.Join(wantRows, "\n")
	old := e.Text()
	e.OnTextInput("inserted")
	assertEditorBytes(t, e, want)
	if e.CursorCount() != 5 || old != original {
		t.Fatal("live multi-caret edit mutated a previous snapshot or lost a caret")
	}
	e.Undo()
	assertEditorBytes(t, e, original)
	e.Redo()
	assertEditorBytes(t, e, want)
}

func TestEditorSparseSnapshotCorrectsOffsetsWithinAnEditedBlock(t *testing.T) {
	e := NewCodeEditor()
	original := strings.Repeat("a\r\n", 2500) + "end"
	e.SetText(original)
	want := strings.Split(original, "\n")
	for _, edit := range []struct {
		row    int
		insert string
	}{{130, "🧩"}, {132, "qq"}, {129, "\xff"}, {133, "longer"}, {130, "again"}, {258, "later"}} {
		e.replaceByteRange(edit.row, 0, 0, edit.insert)
		want[edit.row] = edit.insert + want[edit.row]
	}
	if got := len(e.textSnapshot.edit.additional); e.textSnapshot.edit.bulk || got != 4 {
		t.Fatalf("distinct local rows were not tracked: %+v", e.textSnapshot.edit)
	}
	assertEditorBytes(t, e, strings.Join(want, "\n"))
	// The next edit starts from the updated block byte totals and may land
	// left of the original edits inside that block.
	e.replaceByteRange(128, 0, 0, "new")
	want[128] = "new" + want[128]
	e.replaceByteRange(800, 0, 0, "later")
	want[800] = "later" + want[800]
	assertEditorBytes(t, e, strings.Join(want, "\n"))
}

func TestEditorSparseSnapshotRandomizedAgainstFullJoin(t *testing.T) {
	r := rand.New(rand.NewSource(51229))
	lines := make([]string, 4096)
	bodies := []string{"", "\xff", "x", "中国🧩", "x\r", "\r\r", "another body"}
	for i := range lines {
		lines[i] = bodies[r.Intn(len(bodies))]
	}
	var s editorTextSnapshot
	s.reset(lines)
	text := strings.Join(lines, "\n")
	for i := 0; i < 400; i++ {
		near := r.Intn(len(lines) - 24)
		for j := 0; j < 12; j++ {
			row := near + r.Intn(24)
			if j%4 == 0 {
				row = r.Intn(len(lines))
			}
			body := bodies[r.Intn(len(bodies))]
			s.note(lines, row, lines[row:row+1], []string{body})
			lines[row] = body
		}
		want := strings.Join(lines, "\n")
		got, changed, ok := s.publishSparse(text, lines)
		if !ok || got != want || changed != (text != want) {
			t.Fatalf("sparse edit %d did not preserve exact contents", i)
		}
		text = got
	}
}

func TestEditorSparseSnapshotNoOpAndBudget(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("x\n", 3000))
	old, rev := e.Text(), e.TextRevision()
	for _, row := range []int{129, 257, 1025} {
		e.replaceByteRange(row, 0, 0, "temp")
	}
	for _, row := range []int{257, 1025, 129} {
		e.replaceByteRange(row, 0, len("temp"), "")
	}
	if got := e.Text(); unsafe.StringData(got) != unsafe.StringData(old) || e.TextRevision() != rev {
		t.Fatal("sparse no-op republished an identical snapshot")
	}
	for i := 0; i <= editorSnapshotMaxLocalRows; i++ {
		e.replaceByteRange(4*i, 0, 0, "bulk")
	}
	if !e.textSnapshot.edit.bulk {
		t.Fatal("the sparse optimization retained more rows than its explicit budget")
	}
	assertEditorBytes(t, e, strings.Join(e.lines, "\n"))
}
