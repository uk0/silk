package gui

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEditCompletionUnicodeBytesAndUndo(t *testing.T) {
	e := NewEdit()
	e.SetText("中🙂 ap 后缀")
	e.SetCaretPos(6)
	e.SetCompletionPrefixStart(len("中🙂 ")) // Public API remains a byte offset.
	c := NewCompleter("apple🍎")
	e.SetCompleter(c)
	if prefix := e.CompletionPrefix(); prefix != "ap " {
		t.Fatalf("prefix %q", prefix)
	}
	e.SetCaretPos(5)
	c.Filter(e.CompletionPrefix())
	changed, edited := 0, 0
	e.SigTextChanged(func(interface{}, string) { changed++ })
	e.SigTextEdited(func(interface{}, string) { edited++ })
	if !e.AcceptCompletion(0) || e.Text() != "中🙂 apple🍎 后缀" || e.CaretPos() != 9 || !utf8.ValidString(e.Text()) {
		t.Fatalf("completion text=%q caret=%d", e.Text(), e.CaretPos())
	}
	if !e.CanUndo() || changed != 1 || edited != 1 || e.CompletionPrefix() != "" {
		t.Fatalf("history/signals %v %d/%d", e.CanUndo(), changed, edited)
	}
	e.Undo()
	if e.Text() != "中🙂 ap 后缀" || e.CaretPos() != 5 || e.CompletionPrefix() != "ap" || !e.CanRedo() {
		t.Fatalf("undo text=%q prefix=%q", e.Text(), e.CompletionPrefix())
	}
	e.Redo()
	if e.Text() != "中🙂 apple🍎 后缀" || e.CompletionPrefix() != "" || changed != 3 || edited != 3 {
		t.Fatal("redo lost completion state")
	}
}

func TestEditCompletionUnicodeBoundariesAndActiveCaret(t *testing.T) {
	e := NewEdit()
	e.SetText("A中🙂后")
	for _, tc := range []struct {
		start  int
		prefix string
	}{{0, "A中🙂"}, {1, "中🙂"}, {2, "中🙂"}, {4, "🙂"}, {5, "🙂"}, {8, ""}, {99, ""}, {-1, ""}} {
		e.SetCaretPos(3)
		e.SetCompletionPrefixStart(tc.start)
		if got := e.CompletionPrefix(); got != tc.prefix || !utf8.ValidString(got) {
			t.Errorf("byte %d: %q want %q", tc.start, got, tc.prefix)
		}
	}
	e.SetCompletionPrefixStart(1)
	// Reverse selections come from Shift+Left / dragging; SetSelection sorts.
	e.sel0, e.sel1 = 3, 2 // Active caret is sel1, not the selection anchor.
	if got := e.CompletionPrefix(); got != "中" {
		t.Fatal(got)
	}
	e.SetCompleter(NewCompleter("中文"))
	e.Completer().Filter(e.CompletionPrefix())
	if !e.AcceptCompletion(0) || e.Text() != "A中文🙂后" {
		t.Fatal(e.Text())
	}
	e.Undo()
	if e.Text() != "A中🙂后" || e.sel0 != 3 || e.sel1 != 2 {
		t.Fatal("selection direction lost")
	}
}

func TestEditCompletionRespectsInputGuards(t *testing.T) {
	for _, guard := range []string{"readonly", "disabled", "length", "validator", "utf8"} {
		t.Run(guard, func(t *testing.T) {
			e := NewEdit()
			e.SetText("1")
			e.SetCaretPos(1)
			c := NewCompleter("123")
			e.SetCompleter(c)
			switch guard {
			case "readonly":
				e.SetReadOnly(true)
			case "disabled":
				e.SetEnabled(false)
			case "length":
				e.SetMaxLength(2)
			case "validator":
				e.SetValidator(&IntValidator{Bottom: 0, Top: 20})
			case "utf8":
				c.Candidates = []string{"1\xff"}
			}
			c.Filter("1")
			changed := 0
			e.SigTextChanged(func(interface{}, string) { changed++ })
			if e.AcceptCompletion(0) || e.Text() != "1" || e.CaretPos() != 1 || e.CanUndo() || changed != 0 {
				t.Fatal("guard bypassed")
			}
		})
	}
}

func TestEditCompletionKeepsPriorHistoryAndCancelsPaste(t *testing.T) {
	e := NewEdit()
	e.SetCompleter(NewCompleter("中文🙂"))
	e.OnTextInput("中")
	var finish func(string, error)
	e.clipboardReader = func(_ context.Context, done func(string, error)) error { finish = done; return nil }
	e.Paste()
	if !e.AcceptCompletion(0) {
		t.Fatal("accept failed")
	}
	finish("late", nil)
	if e.PastePending() || e.Text() != "中文🙂" {
		t.Fatal("late paste applied")
	}
	e.Undo()
	if e.Text() != "中" || !e.CanUndo() {
		t.Fatal("prior input history lost")
	}
	e.Undo()
	if e.Text() != "" {
		t.Fatal(e.Text())
	}
	e.Redo()
	e.Redo()
	if e.Text() != "中文🙂" {
		t.Fatal(e.Text())
	}
	e.SetText("新")
	e.SetCaretPos(1)
	if e.CompletionPrefix() != "新" || e.CanUndo() {
		t.Fatal("programmatic reset kept old prefix")
	}
}

func BenchmarkEditCompletionShortPrefixLongSuffix(b *testing.B) {
	e := NewEdit()
	e.SetText("中🙂 ap" + strings.Repeat("x", 100000))
	e.SetCaretPos(5)
	e.SetCompletionPrefixStart(len("中🙂 "))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.CompletionPrefix()
	}
}
