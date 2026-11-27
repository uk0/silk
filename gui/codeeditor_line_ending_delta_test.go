package gui

import (
	"math/rand"
	"strings"
	"testing"
)

func assertEditorLineEndingCounts(t *testing.T, e *CodeEditor, want string) {
	t.Helper()
	assertEditorBytes(t, e, want)
	state := e.LineEndings()
	crlf := strings.Count(want, "\r\n")
	ambiguous := strings.Count(want, "\r\r\n")
	if state.CRLF != crlf || state.LF != strings.Count(want, "\n")-crlf ||
		e.lineEndingAmbiguousCRs != ambiguous {
		t.Fatalf("separator stats %+v ambiguous=%d for %q", state, e.lineEndingAmbiguousCRs, want)
	}
}

func TestEditorLineEndingDeltaDoesNotScanUntouchedRows(t *testing.T) {
	e := NewCodeEditor()
	prefix := strings.Repeat("prefix\r\n", 100_000)
	e.SetText(prefix + "body\nlast\r\r")
	notifications := 0
	stop := e.SubscribeLineEndingsChanged(func() { notifications++ })
	defer stop()

	e.replaceByteRange(100_000, 0, 0, "x")
	if d := e.pendingLineEndings; !d.local || d.bulk || d.crlf != 0 || d.ambiguous != 0 {
		t.Fatalf("ordinary edit did not keep a local separator delta: %+v", d)
	}
	e.rebuildText()
	assertEditorLineEndingCounts(t, e, prefix+"xbody\nlast\r\r")
	if notifications != 0 {
		t.Fatalf("body-only edit emitted %d separator notifications", notifications)
	}

	e.replaceByteRange(100_000, len("xbody"), 0, "\r")
	e.replaceByteRange(100_000, len("xbody\r"), 0, "\r")
	e.replaceByteRange(100_001, len("last\r\r"), 0, "\r")
	if d := e.pendingLineEndings; !d.local || d.bulk || d.crlf != 1 || d.ambiguous != 1 {
		t.Fatalf("compound local edit miscounted separators or EOF: %+v", d)
	}
	e.rebuildText()
	assertEditorLineEndingCounts(t, e, prefix+"xbody\r\r\nlast\r\r\r")
	if notifications != 1 || e.CanConvertLineEndings(LineEndingLF) {
		t.Fatalf("CRCRLF did not protect conversion: notifications=%d", notifications)
	}
	e.replaceByteRange(100_000, len("xbody\r"), 1, "")
	e.rebuildText()
	assertEditorLineEndingCounts(t, e, prefix+"xbody\r\nlast\r\r\r")
	if notifications != 1 || !e.CanConvertLineEndings(LineEndingLF) {
		t.Fatal("removing one CR did not clear the guard without affecting the visible counts")
	}
}

func TestEditorLineEndingDeltaReconcilesStructuralAndReloadChanges(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\r\nb\nc\r\r")
	e.replaceByteRange(2, len("c\r\r"), 0, "\n")
	e.replaceByteRange(0, len("a\r"), 1, "")
	if d := e.pendingLineEndings; !d.bulk || len(e.lines) != 3 {
		t.Fatalf("net-zero row count shift must still reconcile: %+v", d)
	}
	assertEditorLineEndingCounts(t, e, "a\rb\nc\r\r\n")
	e.SetText("reload\r\r")
	assertEditorLineEndingCounts(t, e, "reload\r\r")
	e.ReplaceAllText("mixed\r\nbody\n")
	assertEditorLineEndingCounts(t, e, "mixed\r\nbody\n")
	e.Undo()
	assertEditorLineEndingCounts(t, e, "reload\r\r")
	e.Redo()
	assertEditorLineEndingCounts(t, e, "mixed\r\nbody\n")
}

func TestEditorLineEndingMetadataMatchesReferenceAfterRandomSplices(t *testing.T) {
	e := NewCodeEditor()
	want := "a\r\r\n🧩\r\nb\n"
	e.SetText(want)
	r := rand.New(rand.NewSource(51224))
	splices := []string{"", "x", "\r", "\r\r", "\n", "\r\n", "\r\r\n", "🧩\n", "\xff", "x\ry"}
	for i := 0; i < 500; i++ {
		for j := 0; j <= i%4; j++ {
			start := r.Intn(len(want) + 1)
			end := start + r.Intn(min(6, len(want)-start)+1)
			body := splices[r.Intn(len(splices))]
			prefix := want[:start]
			row := strings.Count(prefix, "\n")
			column := start - strings.LastIndexByte(prefix, '\n') - 1
			e.replaceByteRange(row, column, end-start, body)
			want = want[:start] + body + want[end:]
		}
		assertEditorLineEndingCounts(t, e, want)
	}
}
