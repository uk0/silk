package gui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEditorRuneDeltaPublishesOnlyAtTextBoundary(t *testing.T) {
	e := NewCodeEditor()
	original := "中\r\n\xc2\n\x80\n"
	e.SetText(original)
	initialCount, rev := e.textRuneCount, e.TextRevision()

	// Removing the separator both drops LF and joins two invalid bytes.
	e.replaceByteRange(1, 1, 1, "")
	if e.runeDeltaUnknown || e.pendingRuneDelta != -2 || e.textRuneCount != initialCount || e.textRevision != rev {
		t.Fatalf("unpublished edit: unknown=%v delta=%d count=%d revision=%d", e.runeDeltaUnknown, e.pendingRuneDelta, e.textRuneCount, e.textRevision)
	}
	assertEditorBytes(t, e, "中\r\n\xc2\x80\n")
	if e.textDirty || e.pendingRuneDelta != 0 || e.TextRevision() != rev+1 {
		t.Fatal("reading Text did not publish and consume the delta exactly once")
	}

	large := strings.Repeat("界", 2000)
	e.replaceByteRange(1, 0, 0, large)
	if !e.runeDeltaUnknown {
		t.Fatal("large replacement missed the bounded fallback")
	}
	// More low-level edits before publishing must not turn an incomplete
	// delta back into a trustworthy one.
	e.replaceByteRange(0, 0, 0, "😀")
	assertEditorBytes(t, e, "😀中\r\n"+large+"\xc2\x80\n")
	e.SetText(original)
	assertEditorBytes(t, e, original)
}

func TestEditorRuneDeltaAcrossCommandsAndHistory(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("中\r\nalpha\n😀\r\n\xff")
	assertStatus := func() {
		t.Helper()
		if got, want := e.textRuneCount, utf8.RuneCountInString(e.Text()); got != want {
			t.Fatalf("status count = %d, want %d for %q", got, want, e.Text())
		}
	}
	calls := 0
	e.SigChanged(func(text string) {
		calls++
		if got, want := e.textRuneCount, utf8.RuneCountInString(text); got != want {
			t.Fatalf("change callback count = %d, want %d for %q", got, want, text)
		}
	})
	commands := []struct {
		name string
		run  func()
	}{
		{"insert at multiple carets", func() {
			e.ScrollToLineCol(0, 1)
			e.AddCursorAtLine(2, 1)
			e.OnTextInput("界")
		}},
		{"duplicate", e.DuplicateLines},
		{"delete", e.DeleteLines},
		{"move", func() { e.ScrollToLineCol(0, 0); e.MoveLinesDown() }},
		{"convert CRLF", func() {
			if err := e.ConvertLineEndings(LineEndingCRLF); err != nil {
				t.Fatal(err)
			}
		}},
		{"replace all", func() { e.ReplaceAllTextAt("new😀\n\xff\n", 0, 0) }},
	}
	for _, cmd := range commands {
		t.Log(cmd.name)
		before, beforeCalls := e.Text(), calls
		cmd.run()
		assertStatus()
		after := e.Text()
		if before == after || calls != beforeCalls+1 {
			t.Fatalf("%s: expected one changed notification and changed text, before=%q after=%q calls=%d", cmd.name, before, after, calls-beforeCalls)
		}
		e.Undo()
		assertEditorBytes(t, e, before)
		e.Redo()
		assertEditorBytes(t, e, after)
	}
	e.ToggleLineComment()
	assertStatus()
	e.JoinLines()
	assertStatus()
}
