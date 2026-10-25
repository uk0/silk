package gui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEditorCleanupBaselineCRLF(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("first \t\r\nsecond\t\nlast  ")
	e.TrimTrailingWhitespace()
	if got := e.Text(); got != "first\r\nsecond\nlast" {
		t.Fatalf("trailing whitespace not removed without altering separators: %q", got)
	}
}
func TestEditorCleanupBaselineSelection(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("first  \nlast  ")
	e.hasSelection = true
	e.selStartLine = 1
	e.selStartCol = 6
	e.selEndLine = 0
	e.selEndCol = 1
	e.cursorLine = 1
	e.cursorCol = 6
	e.TrimTrailingWhitespace()
	if !e.hasSelection || e.selStartLine != 1 || e.selStartCol != 4 || e.selEndLine != 0 || e.selEndCol != 1 {
		t.Fatalf("cleanup lost reversed selection: %+v", e.selectionSnapshot())
	}
}

func cleanupCursors(e *CodeEditor) []cursorPos {
	return append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
}

func TestEditorCleanupBytesHistoryAndEvents(t *testing.T) {
	cases := [][2]string{
		{"", ""}, {"clean", "clean"}, {"  ", ""}, {"a \r\n\t\r\nlast\t", "a\r\n\r\nlast"},
		{"\ufeff中🙂 \t\r\nb\t\nlast  ", "\ufeff中🙂\r\nb\nlast"},
		{"x\x00\xff \n\xc2\t\r\nlast\r  ", "x\x00\xff\n\xc2\r\nlast\r"},
		{"nbsp\u00a0\u2003\t\r\n", "nbsp\u00a0\u2003\r\n"},
		{"a\r \r\nb\r\r\n", "a\r\r\nb\r\r\n"},
		{"a\rb \n\nlast\r", "a\rb\n\nlast\r"},
	}
	for _, tc := range cases {
		t.Run(tc[0], func(t *testing.T) {
			e := NewCodeEditor()
			e.filePath = "sample.txt" // Pure transformation tests must not launch Git workers.
			e.SetText(tc[0])
			e.MarkSaved()
			e.SetBookmark(len(e.lines)-1, true)
			marks := e.BookmarkLines()
			info := e.LineEndings()
			revision := e.TextRevision()
			changes, bookEvents := 0, 0
			e.SigChanged(func(string) { changes++ })
			stop := e.SubscribeBookmarksChanged(func() { bookEvents++ })
			defer stop()
			if err := e.TrimTrailingWhitespaceChecked(); err != nil {
				t.Fatal(err)
			}
			changed := tc[0] != tc[1]
			if e.Text() != tc[1] || e.Modified() != changed || e.LineEndings() != info || !reflect.DeepEqual(e.BookmarkLines(), marks) {
				t.Fatalf("trim %q info=%+v", e.Text(), e.LineEndings())
			}
			wantEvents := 0
			if changed {
				wantEvents = 1
			}
			if changes != wantEvents || bookEvents != wantEvents || e.TextRevision() != revision+uint64(wantEvents) {
				t.Fatal("event/revision contract", changes, bookEvents, e.TextRevision())
			}
			if changed {
				if len(e.undoStack) != 1 || e.undoStack[0].kind != 4 || e.undoStack[0].text != "" || e.undoStack[0].oldText != "" {
					t.Fatal("not sparse single history")
				}
				e.Undo()
				if e.Text() != tc[0] || e.Modified() || !reflect.DeepEqual(e.BookmarkLines(), marks) {
					t.Fatal("undo", e.Text())
				}
				e.Redo()
				if e.Text() != tc[1] {
					t.Fatal("redo")
				}
			}
			count, rev := len(e.undoStack), e.TextRevision()
			if err := e.TrimTrailingWhitespaceChecked(); err != nil || len(e.undoStack) != count || e.TextRevision() != rev {
				t.Fatal("not idempotent", err)
			}
			if got := trimTrailingInText(tc[0]); got != tc[1] {
				t.Fatal("pure adapter", got)
			}
		})
	}
}

func TestEditorCleanupRestoresDirectedSelectionAndAllCarets(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("中🙂  \r\nlast\t  ")
	e.MarkSaved()
	e.cursorLine, e.cursorCol = 1, 7
	e.additionalCursors = []cursorPos{{0, 4}, {0, 2}, {0, 5}}
	e.hasSelection = true
	e.selStartLine, e.selStartCol = 1, 7
	e.selEndLine, e.selEndCol = 0, 1
	before, sel := cleanupCursors(e), e.selectionSnapshot()
	if err := e.TrimTrailingWhitespaceChecked(); err != nil {
		t.Fatal(err)
	}
	// The endpoint after the CR shifts by the suffix length, not to before CR.
	want := []cursorPos{{1, 4}, {0, 2}, {0, 3}}
	if !reflect.DeepEqual(cleanupCursors(e), want) || e.selStartLine != 1 || e.selStartCol != 4 || e.selEndCol != 1 || !e.hasSelection {
		t.Fatal("mapped anchors", cleanupCursors(e), e.selectionSnapshot())
	}
	after, next := cleanupCursors(e), e.selectionSnapshot()
	e.Undo()
	if !reflect.DeepEqual(cleanupCursors(e), before) || e.selectionSnapshot() != sel || e.Modified() {
		t.Fatal("undo anchors")
	}
	e.Redo()
	if !reflect.DeepEqual(cleanupCursors(e), after) || e.selectionSnapshot() != next {
		t.Fatal("redo anchors")
	}
}

func TestEditorCleanupAmbiguousCRAndReadOnlyAreAtomic(t *testing.T) {
	for _, text := range []string{"safe  \nbody\r \nlast\t", "head\r\t\nend  "} {
		e := NewCodeEditor()
		e.SetText(text)
		e.MarkSaved()
		e.SelectAll()
		e.SetBookmark(1, true)
		rev, sel, marks := e.TextRevision(), e.selectionSnapshot(), e.BookmarkLines()
		if err := e.TrimTrailingWhitespaceChecked(); !errors.Is(err, ErrEditorCleanupCR) {
			t.Fatal(err)
		}
		e.TrimTrailingWhitespace() // legacy void wrapper has the same atomic behavior
		if e.Text() != text || e.TextRevision() != rev || e.selectionSnapshot() != sel || e.CanUndo() || e.Modified() || !reflect.DeepEqual(marks, e.BookmarkLines()) {
			t.Fatal("partial mutation")
		}
		if trimTrailingInText(text) != text {
			t.Fatal("adapter ambiguity")
		}
	}
	e := NewCodeEditor()
	e.SetText("a  ")
	e.SetReadOnly(true)
	rev := e.TextRevision()
	if err := e.TrimTrailingWhitespaceChecked(); !errors.Is(err, ErrEditorReadOnly) {
		t.Fatal(err)
	}
	if err := e.EnsureFinalNewline(); !errors.Is(err, ErrEditorReadOnly) {
		t.Fatal(err)
	}
	if e.Text() != "a  " || e.TextRevision() != rev || e.CanUndo() {
		t.Fatal("readonly mutation")
	}
}

func TestEditorEnsureFinalNewlinePreferenceAnchorsAndUndo(t *testing.T) {
	for _, tc := range []struct {
		source, want string
		style        EditorLineEnding
	}{
		{"", "", LineEndingLF}, {"done\n\n", "done\n\n", LineEndingLF}, {"done\r\n", "done\r\n", LineEndingLF},
		{"中🙂", "中🙂\n", LineEndingLF}, {"bad\xff", "bad\xff\r\n", LineEndingCRLF},
		{"a\nb\r\nlast", "a\nb\r\nlast\r\n", LineEndingCRLF},
		{"a\r", "a\r\r\n", LineEndingCRLF}, {" \t", " \t\n", LineEndingLF},
	} {
		t.Run(tc.source+tc.style.String(), func(t *testing.T) {
			e := NewCodeEditor()
			e.filePath = "sample.txt" // Pure transformation tests must not launch Git workers.
			e.SetText(tc.source)
			e.preferredLineEnding = tc.style
			e.MarkSaved()
			e.cursorLine = len(e.lines) - 1
			e.cursorCol = utf8.RuneCountInString(e.lines[e.cursorLine])
			e.additionalCursors = []cursorPos{{0, 0}}
			e.hasSelection = true
			e.selStartLine, e.selStartCol = e.cursorLine, e.cursorCol
			e.selEndLine, e.selEndCol = 0, 0
			e.SetBookmark(e.cursorLine, true)
			before, sel, marks := cleanupCursors(e), e.selectionSnapshot(), e.BookmarkLines()
			rev, events := e.TextRevision(), 0
			e.SigChanged(func(string) { events++ })
			if err := e.EnsureFinalNewline(); err != nil {
				t.Fatal(err)
			}
			if e.Text() != tc.want || !reflect.DeepEqual(before, cleanupCursors(e)) || e.selectionSnapshot() != sel || !reflect.DeepEqual(marks, e.BookmarkLines()) {
				t.Fatal("append moved anchors", e.Text())
			}
			if e.HasFinalNewline() != strings.HasSuffix(tc.want, "\n") {
				t.Fatal("termination query")
			}
			changed := tc.source != tc.want
			if changed {
				if e.TextRevision() != rev+1 || events != 1 || len(e.undoStack) != 1 || e.undoStack[0].kind != 4 {
					t.Fatal("history/events")
				}
				e.Undo()
				if e.Text() != tc.source || e.Modified() || !reflect.DeepEqual(before, cleanupCursors(e)) || e.selectionSnapshot() != sel {
					t.Fatal("undo")
				}
				e.Redo()
				if e.Text() != tc.want {
					t.Fatal("redo")
				}
			} else if events != 0 || e.TextRevision() != rev || e.CanUndo() {
				t.Fatal("no-op changed state")
			}
			rev = e.TextRevision()
			count := len(e.undoStack)
			if err := e.EnsureFinalNewline(); err != nil || e.TextRevision() != rev || len(e.undoStack) != count {
				t.Fatal("idempotency")
			}
		})
	}
	e := NewCodeEditor()
	e.SetText("last\r")
	e.MarkSaved()
	rev := e.TextRevision()
	if err := e.EnsureFinalNewline(); !errors.Is(err, ErrEditorCleanupCR) || e.Text() != "last\r" || e.TextRevision() != rev || e.CanUndo() {
		t.Fatal("ambiguous LF append", err)
	}
}

func TestEditorCleanupSparseHistoryAndEarlierUndo(t *testing.T) {
	e := NewCodeEditor()
	e.filePath = "sample.txt" // Pure transformation tests must not launch Git workers.
	source := strings.Repeat("body-content-", 100000) + " \r\nlast\t"
	e.SetText(source)
	e.MarkSaved()
	e.ReplaceAllText(source + "  ")
	if err := e.TrimTrailingWhitespaceChecked(); err != nil {
		t.Fatal(err)
	}
	a := e.undoStack[len(e.undoStack)-1]
	bytes := 0
	for _, g := range a.group {
		bytes += len(g.before) + len(g.after)
	}
	if a.kind != 4 || len(a.group) != 2 || bytes != 4 || a.oldText != "" || a.text != "" {
		t.Fatal("retained full document", bytes)
	}
	e.Undo()
	if e.Text() != source+"  " {
		t.Fatal("cleanup undo")
	}
	e.Undo()
	if e.Text() != source || e.Modified() {
		t.Fatal("earlier undo lost")
	}
	clean := strings.Split(strings.Repeat("body\r\n", 100000), "\n")
	if n := testing.AllocsPerRun(5, func() {
		edits, err := trailingWhitespacePlan(clean)
		if len(edits) != 0 || err != nil {
			panic("plan")
		}
	}); n != 0 {
		t.Fatal("clean plan allocates", n)
	}
}

func BenchmarkEditorCleanupCleanPlan(b *testing.B) {
	lines := strings.Split(strings.Repeat("body\r\n", 100000), "\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = trailingWhitespacePlan(lines)
	}
}

var cleanupFinalNewlineSink bool

func BenchmarkEditorCleanupFinalNewlineQuery(b *testing.B) {
	e := NewCodeEditor()
	e.filePath = "sample.txt" // Pure transformation tests must not launch Git workers.
	e.SetText(strings.Repeat("body\n", 100000))
	defer e.CancelSymbolIndex()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cleanupFinalNewlineSink = e.HasFinalNewline()
	}
}
