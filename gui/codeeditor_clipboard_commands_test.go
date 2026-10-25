package gui

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEditorCutRejectedClipboardPreservesDocument(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("keep\x00me\nnext")
	e.editorSelectAll()
	rev := e.TextRevision()
	e.editorCut() // Embedded NUL is rejected before any native clipboard write.
	if e.Text() != "keep\x00me\nnext" || e.TextRevision() != rev || e.CanUndo() || !e.HasSelection() {
		t.Fatal("failed clipboard write destroyed selected text or history")
	}
}

func TestEditorCopyCutWriterFailureIsAtomic(t *testing.T) {
	for _, selectAll := range []bool{false, true} {
		e := NewCodeEditor()
		e.SetText("中🙂\r\nbeta\r\nlast")
		e.DuplicateLines()
		e.Undo() // A failed cut must preserve an existing redo branch too.
		if selectAll {
			e.SelectAll()
		}
		before, rev, selected := e.Text(), e.TextRevision(), e.SelectedText()
		busy := errors.New("clipboard busy")
		called := 0
		write := func(text string) error {
			called++
			if e.Text() != before || text == "" {
				t.Fatal("mutation before write")
			}
			return busy
		}
		for _, cut := range []bool{false, true} {
			if err := e.copyOrCut(cut, write); !errors.Is(err, busy) {
				t.Fatal(err)
			}
			if e.Text() != before || e.TextRevision() != rev || e.SelectedText() != selected || e.CanUndo() || !e.CanRedo() {
				t.Fatal("failure changed editor state")
			}
		}
		if called != 2 {
			t.Fatal(called)
		}
	}
}

func TestEditorClipboardSuccessUnicodeCRLFAndPrimaryLine(t *testing.T) {
	for _, selected := range []bool{false, true} {
		e := NewCodeEditor()
		original := "中🙂\r\nbeta\r\nlast"
		e.SetText(original)
		e.additionalCursors = []cursorPos{{2, 0}}
		want := "中🙂\r"
		if selected {
			e.SelectAll()
			want = original
		}
		writes, changed := 0, 0
		e.SigChanged(func(string) { changed++ })
		write := func(text string) error {
			writes++
			if text != want {
				t.Fatalf("clipboard bytes %q want %q", text, want)
			}
			return nil
		}
		if err := e.copyOrCut(false, write); err != nil || e.CanUndo() || changed != 0 {
			t.Fatal("copy changed history", err)
		}
		if err := e.copyOrCut(true, write); err != nil {
			t.Fatal(err)
		}
		after := "beta\r\nlast"
		if selected {
			after = ""
		}
		if e.Text() != after || changed != 1 || writes != 2 {
			t.Fatal(e.Text(), changed, writes)
		}
		e.Undo()
		if e.Text() != original {
			t.Fatal("cut undo lost bytes")
		}
		e.Redo()
		if e.Text() != after {
			t.Fatal("cut redo lost bytes")
		}
	}
}

func TestEditorClipboardEmptyDisabledAndInvalidPayload(t *testing.T) {
	e := NewCodeEditor()
	write := func(string) error { t.Fatal("unexpected clipboard write"); return nil }
	if e.CanCopy() || e.CanCut() || e.CanSelectAll() || !e.CanPaste() {
		t.Fatal("empty availability")
	}
	e.SelectAll()
	if e.HasSelection() {
		t.Fatal("empty phantom selection")
	}
	e.SetText("\nlast")
	if e.CanCopy() || !e.CanCut() {
		t.Fatal("empty primary line")
	}
	if err := e.copyOrCut(true, write); err != nil || e.Text() != "last" {
		t.Fatal("empty line cut", err)
	}
	e.Undo()
	if e.Text() != "\nlast" {
		t.Fatal("empty line undo")
	}
	e.SetText("bad\x00text")
	if err := e.copyOrCut(true, write); err == nil || e.Text() != "bad\x00text" || e.CanUndo() {
		t.Fatal("invalid payload accepted")
	}
	e.SetText("enabled")
	e.SetEnabled(false)
	r := new(fakeClipboardRead)
	e.clipboardReader = r.read
	if e.CanCopy() || e.CanCut() || e.CanPaste() || e.CanSelectAll() {
		t.Fatal("disabled availability")
	}
	_ = e.copyOrCut(true, write)
	_ = e.copyOrCut(false, write)
	e.SelectAll()
	e.Paste()
	if e.Text() != "enabled" || e.HasSelection() || e.PastePending() || r.done != nil {
		t.Fatal("disabled command executed")
	}
}

func TestEditorPublicPasteAndSelectAllRetirePendingReads(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb")
	e.additionalCursors = []cursorPos{{1, 0}}
	r := new(fakeClipboardRead)
	e.clipboardReader = r.read
	e.Paste()
	r.done("中🙂", nil)
	if e.Text() != "中🙂a\n中🙂b" {
		t.Fatal("multi-caret paste", e.Text())
	}
	e.Undo()
	if e.Text() != "a\nb" {
		t.Fatal("multi-caret undo")
	}
	e.Paste()
	done := r.done
	e.SelectAll()
	if e.PastePending() || r.ctx.Err() == nil || e.CursorCount() != 1 || e.SelectedText() != "a\nb" {
		t.Fatal("SelectAll did not retire carets/read")
	}
	done("stale", nil)
	if e.Text() != "a\nb" {
		t.Fatal("late paste inserted after SelectAll")
	}
	e.Paste()
	r.done("replacement\r\n", nil)
	if e.Text() != "replacement\r\n" {
		t.Fatal("selection paste")
	}
	e.Undo()
	if e.Text() != "a\nb" {
		t.Fatal("selection paste undo")
	}
}

func TestEditorClipboardQueriesDoNotReadOrAllocate(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("row\n", 100000))
	e.SelectAll()
	e.clipboardReader = func(_ context.Context, _ func(string, error)) error { t.Fatal("query read clipboard"); return nil }
	if n := testing.AllocsPerRun(100, func() { _ = e.CanCopy(); _ = e.CanCut(); _ = e.CanPaste(); _ = e.CanSelectAll() }); n != 0 {
		t.Fatal(n)
	}
	for _, item := range e.contextMenuItems("") {
		if item.Label == "粘贴" && !item.Enabled {
			t.Fatal("paste disabled")
		}
	}
}

func TestEditorClipboardOversizedCutIsNonDestructive(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(strings.Repeat("a", maxClipboardTextBytes/2) + "\n" + strings.Repeat("b", maxClipboardTextBytes/2))
	e.SelectAll()
	rev := e.TextRevision()
	if n := testing.AllocsPerRun(5, func() {
		text, err := e.clipboardCommandText()
		if text != "" || !errors.Is(err, ErrClipboardTooLarge) {
			t.Fatal("oversized selection accepted")
		}
	}); n != 0 {
		t.Fatal("oversized selection materialized", n)
	}
	if err := e.copyOrCut(true, func(string) error { t.Fatal("oversized native write"); return nil }); !errors.Is(err, ErrClipboardTooLarge) {
		t.Fatal(err)
	}
	if e.TextRevision() != rev || e.CanUndo() || !e.HasSelection() {
		t.Fatal("oversized cut changed document")
	}
}
