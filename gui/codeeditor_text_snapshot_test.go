package gui

import (
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

func TestEditorSnapshotUsesExactLocalBytesAcrossBlocks(t *testing.T) {
	e := NewCodeEditor()
	var initial strings.Builder
	for i := 0; i < 3000; i++ {
		if i%3 == 0 {
			initial.WriteString("中😀\xff\r\n")
		} else {
			initial.WriteString("é\n")
		}
	}
	original := initial.String()
	e.SetText(original)
	wantLines := strings.Split(original, "\n")
	want := original
	for _, edit := range []struct {
		row    int
		insert string
	}{
		{0, "first"},
		{127, "\xff"},
		{128, "🌟"},
		{1024, "\x00"},
		{3000, "last"},
		{129, "é"},
		{128, "!"},
	} {
		previous, previousCopy := e.Text(), strings.Clone(want)
		e.replaceByteRange(edit.row, 0, 0, edit.insert)
		if !e.textSnapshot.edit.local || e.textSnapshot.edit.bulk {
			t.Fatalf("row %d did not have an exact snapshot edit", edit.row)
		}
		wantLines[edit.row] = edit.insert + wantLines[edit.row]
		want = strings.Join(wantLines, "\n")
		assertEditorBytes(t, e, want)
		if previous != previousCopy {
			t.Fatal("a previously published snapshot was modified")
		}
	}
}

func TestEditorSnapshotNoOpAndBulkFallback(t *testing.T) {
	e := NewCodeEditor()
	original := strings.Repeat("a\r\n", 2000)
	e.SetText(original)
	previous, revision := e.Text(), e.TextRevision()
	e.replaceByteRange(129, 0, 0, "\xc2")
	e.replaceByteRange(129, 1, 0, "\x80")
	if got := e.Text(); got != strings.Repeat("a\r\n", 129)+"\xc2\x80"+strings.Repeat("a\r\n", 1871) {
		t.Fatal("two pending edits on one row lost bytes")
	}

	current, rev := e.Text(), e.TextRevision()
	e.replaceByteRange(129, 0, 2, "\xc2\x80")
	if got := e.Text(); unsafe.StringData(got) != unsafe.StringData(current) || e.TextRevision() != rev {
		t.Fatal("an identical local edit did not preserve the published snapshot")
	}

	e.replaceByteRange(0, 0, 0, "new\n")
	e.replaceByteRange(1000, 0, 3, "")
	if !e.textSnapshot.edit.bulk || len(e.lines) != 2001 {
		t.Fatal("a structural edit must remain bulk when line counts cancel")
	}
	want := strings.Join(e.lines, "\n")
	assertEditorBytes(t, e, want)

	// Mixed-size replacement followed by a local change must use offsets
	// rebuilt from the newly published document, not the previous one.
	e.ReplaceAllText("something\n")
	if len(e.textSnapshot.blocks) != 0 {
		t.Fatal("small replacements retained the large-buffer index")
	}
	e.Undo()
	assertEditorBytes(t, e, want)
	e.replaceByteRange(1764, 0, 0, "safe")
	assertEditorBytes(t, e, strings.Join(e.lines, "\n"))
	if previous != original || e.TextRevision() <= revision {
		t.Fatal("publication overwrote a retained snapshot or failed to advance")
	}
}

func TestEditorSnapshotIndexRandomized(t *testing.T) {
	r := rand.New(rand.NewSource(51220))
	corpus := []string{"", "a", "a\r", "\xfe", "你好😀", "x\r\r"}
	lines := make([]string, 4093)
	for i := range lines {
		lines[i] = corpus[r.Intn(len(corpus))]
	}
	var s editorTextSnapshot
	s.reset(lines)
	text := strings.Join(lines, "\n")
	for i := 0; i < 500; i++ {
		row := r.Intn(len(lines))
		body := corpus[r.Intn(len(corpus))]
		s.note(lines, row, lines[row:row+1], []string{body})
		lines[row] = body
		want := strings.Join(lines, "\n")
		got, changed := s.publish(text, lines)
		if got != want || changed != (text != want) {
			t.Fatalf("edit %d row %d produced a different immutable snapshot", i, row)
		}
		text = got
	}
}
