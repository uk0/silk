package gui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEditorEnterPreservesCRLF(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("one\r\ntwo\r\n")
	e.cursorCol = 3
	e.OnKeyDown(KeyEnter, false)
	if got := e.Text(); got != "one\r\n\r\ntwo\r\n" {
		t.Fatalf("Enter introduced a different line ending: %q", got)
	}
}

func TestEditorLineEndingDetectionAndPreference(t *testing.T) {
	for _, tc := range []struct {
		text, label string
		lf, crlf    int
		preferred   EditorLineEnding
	}{
		{"", "LF", 0, 0, LineEndingLF}, {"plain\rtext", "LF", 0, 0, LineEndingLF},
		{"a\nb\n", "LF", 2, 0, LineEndingLF}, {"a\r\nb", "CRLF", 0, 1, LineEndingCRLF},
		{"a\r\nb\nc\r\n", "Mixed", 1, 2, LineEndingCRLF},
		{"a\nb\r\n", "Mixed", 1, 1, LineEndingLF}, {"a\r\nb\n", "Mixed", 1, 1, LineEndingCRLF},
	} {
		e := NewCodeEditor()
		e.SetText(tc.text)
		got := e.LineEndings()
		if got.LF != tc.lf || got.CRLF != tc.crlf || got.Preferred != tc.preferred || got.String() != tc.label {
			t.Fatalf("%q: %+v", tc.text, got)
		}
	}
}
func TestEditorLineEndingConversionBytesHistoryAndSelection(t *testing.T) {
	for _, source := range []string{"", "plain", "a\nb\n", "a\r\nb", "中🙂\r\n\tbad\xff\nlast\r", "\r\n\n", "a\rb\nc"} {
		for _, style := range []EditorLineEnding{LineEndingLF, LineEndingCRLF} {
			t.Run(fmt.Sprintf("%x/%s", source, style), func(t *testing.T) {
				e := NewCodeEditor()
				e.SetText(source)
				e.MarkSaved()
				originalInfo := e.LineEndings()
				e.cursorLine = len(e.lines) - 1
				e.cursorCol = utf8.RuneCountInString(e.lines[e.cursorLine])
				e.additionalCursors = []cursorPos{{0, 0}}
				e.hasSelection = true
				e.selStartLine = e.cursorLine
				e.selStartCol = e.cursorCol
				e.selEndLine = 0
				e.selEndCol = 0
				before := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
				selection := e.selectionSnapshot()
				expected := strings.ReplaceAll(source, "\r\n", "\n")
				if style == LineEndingCRLF {
					expected = strings.ReplaceAll(expected, "\n", "\r\n")
				}
				needed := e.CanConvertLineEndings(style)
				rev := e.TextRevision()
				changes := 0
				e.SigChanged(func(string) { changes++ })
				if err := e.ConvertLineEndings(style); err != nil {
					t.Fatal(err)
				}
				if e.Text() != expected || e.LineEndings().Preferred != style || e.CanUndo() != needed {
					t.Fatalf("convert: %q %+v", e.Text(), e.LineEndings())
				}
				if e.Text() == source && (e.TextRevision() != rev || e.Modified() || changes != 0) {
					t.Fatal("preference changed content state")
				}
				after := e.TextRevision()
				if err := e.ConvertLineEndings(style); err != nil || e.TextRevision() != after {
					t.Fatal("non-idempotent conversion")
				}
				if needed {
					if len(e.undoStack) != 1 || e.undoStack[0].kind != 7 {
						t.Fatal("not one conversion transaction")
					}
					e.Undo()
					if e.Text() != source || e.LineEndings() != originalInfo || e.Modified() || !reflect.DeepEqual(append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...), before) || e.selectionSnapshot() != selection {
						t.Fatalf("Undo lost state: %q", e.Text())
					}
					e.Redo()
					if e.Text() != expected || e.LineEndings().Preferred != style {
						t.Fatal("Redo")
					}
				}
			})
		}
	}
}
func TestEditorLineEndingRejectsReadOnlyInvalidAndAmbiguousWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		text   string
		locked bool
		style  EditorLineEnding
		want   error
	}{
		{"a\nb", true, LineEndingCRLF, ErrEditorReadOnly}, {"a\nb", false, 99, ErrEditorLineEnding}, {"a\r\r\nb", false, LineEndingLF, ErrEditorAmbiguousCR},
	} {
		e := NewCodeEditor()
		e.SetText(tc.text)
		e.SetReadOnly(tc.locked)
		rev := e.TextRevision()
		if e.CanConvertLineEndings(tc.style) {
			t.Fatal("invalid command enabled")
		}
		if err := e.ConvertLineEndings(tc.style); !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
		if e.Text() != tc.text || e.TextRevision() != rev || e.CanUndo() {
			t.Fatal("failed conversion changed document")
		}
	}
}
func TestEditorLineEndingPreferenceOnlyAndInputHistory(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("x")
	e.MarkSaved()
	rev := e.TextRevision()
	calls := 0
	e.SigChanged(func(string) { calls++ })
	if err := e.ConvertLineEndings(LineEndingCRLF); err != nil {
		t.Fatal(err)
	}
	if e.Modified() || calls != 0 || e.TextRevision() != rev {
		t.Fatal("preference became a text edit")
	}
	e.cursorCol = 1
	e.OnKeyDown(KeyEnter, false)
	if e.Text() != "x\r\n" {
		t.Fatal(e.Text())
	}
	e.Undo()
	if e.Text() != "x" || e.LineEndings().Preferred != LineEndingCRLF {
		t.Fatal("Enter Undo")
	}
	e.Undo()
	if e.LineEndings().Preferred != LineEndingLF {
		t.Fatal("preference Undo")
	}
	e.Redo()
	e.Redo()
	if e.Text() != "x\r\n" {
		t.Fatal("interleaved Redo")
	}
	e.SetText("reset\n")
	if e.LineEndings().Preferred != LineEndingLF || e.CanUndo() {
		t.Fatal("host load did not reset preference/history")
	}
}
func TestEditorCRLFEnterAtEndAndMultipleCarets(t *testing.T) {
	for _, multi := range []bool{false, true} {
		e := NewCodeEditor()
		e.SetText("中🙂\r\nnext\r\n")
		e.cursorCol = 3
		if multi {
			e.additionalCursors = []cursorPos{{0, 2}, {1, 5}}
		}
		original := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
		e.OnKeyDown(KeyEnter, false)
		want := "中🙂\r\n\r\nnext\r\n"
		if multi {
			want = "中🙂\r\n\r\nnext\r\n\r\n"
		}
		if e.Text() != want {
			t.Fatalf("multi=%v: %q", multi, e.Text())
		}
		e.Undo()
		if e.Text() != "中🙂\r\nnext\r\n" || !reflect.DeepEqual(append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...), original) {
			t.Fatal("Enter did not restore CRLF and carets")
		}
	}
}
func TestEditorLineEndingClipboardAndHostBytesRemainVerbatim(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\r\n")
	e.cursorLine = 1
	e.pasteText("b\n")
	if e.Text() != "a\r\nb\n" || e.LineEndings().String() != "Mixed" {
		t.Fatal("paste silently normalized")
	}
	e.SetText("raw\xff\r\n")
	if e.Text() != "raw\xff\r\n" {
		t.Fatal("host byte mutation")
	}
}
func TestEditorLineEndingLargeHistoryAndConstantTimeQueries(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("line\n", 100000))
	original := e.Text()
	if n := testing.AllocsPerRun(100, func() { _ = e.LineEndings(); _ = e.CanConvertLineEndings(LineEndingCRLF) }); n != 0 {
		t.Fatal(n)
	}
	if err := e.ConvertLineEndings(LineEndingCRLF); err != nil {
		t.Fatal(err)
	}
	a := e.undoStack[0]
	if len(a.lineEndings.original) != 1563 || a.text != "" || a.oldText != "" || len(a.group) != 0 || len(a.lineSplices) != 0 {
		t.Fatal("history retained more than a separator bitmap")
	}
	e.Undo()
	if e.Text() != original {
		t.Fatal("large Undo bytes")
	}
}
func BenchmarkEditorLineEndingQueries(b *testing.B) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("line\r\n", 100000))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.LineEndings()
		_ = e.CanConvertLineEndings(LineEndingLF)
	}
}

func TestEditorCRLFDeleteAndBackspaceAreAtomic(t *testing.T) {
	for _, tc := range []struct {
		key, line, col int
		want           string
	}{
		{KeyBackSpace, 1, 0, "onetwo\r\n"}, {KeyDelete, 0, 3, "onetwo\r\n"}, {KeyDelete, 0, 4, "onetwo\r\n"},
		{KeyBackSpace, 0, 4, "on\r\ntwo\r\n"},
	} {
		e := NewCodeEditor()
		e.SetText("one\r\ntwo\r\n")
		e.cursorLine, e.cursorCol = tc.line, tc.col
		e.OnKeyDown(tc.key, false)
		if e.Text() != tc.want {
			t.Fatalf("%+v: %q", tc, e.Text())
		}
		e.Undo()
		if e.Text() != "one\r\ntwo\r\n" || e.cursorLine != tc.line || e.cursorCol != tc.col {
			t.Fatal("CRLF boundary undo")
		}
		e.Redo()
		if e.Text() != tc.want {
			t.Fatal("CRLF boundary redo")
		}
	}
}

func TestEditorLineEndingEventsAreDistinctFromTextEvents(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("x")
	metadata, text := 0, 0
	e.SigChanged(func(string) { text++ })
	stop := e.SubscribeLineEndingsChanged(func() { metadata++ })
	defer stop()
	e.ConvertLineEndings(LineEndingCRLF)
	e.ConvertLineEndings(LineEndingLF)
	e.ConvertLineEndings(LineEndingCRLF)
	if metadata != 3 || text != 0 {
		t.Fatal(metadata, text)
	}
	e.Undo()
	e.Undo()
	e.Redo()
	if metadata != 6 || text != 0 {
		t.Fatal("preference history events", metadata, text)
	}
	e.OnTextInput("a")
	if metadata != 6 || text != 1 {
		t.Fatal("ordinary typing notified metadata", metadata, text)
	}
	stop()
	stop()
	e.ConvertLineEndings(LineEndingCRLF)
	if metadata != 6 {
		t.Fatal("unsubscribe")
	}
	e.SubscribeLineEndingsChanged(nil)()
}
func TestEditorLineEndingObserversCanDetachDuringDispatch(t *testing.T) {
	e := NewCodeEditor()
	calls := 0
	var stop func()
	e.SubscribeLineEndingsChanged(func() { stop() })
	stop = e.SubscribeLineEndingsChanged(func() { calls++ })
	e.SetText("a\r\n")
	if calls != 0 {
		t.Fatal("detached observer called")
	}
}
