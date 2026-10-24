package gui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/uk0/silk/async"
)

type fakeClipboardRead struct {
	calls int
	ctx   context.Context
	done  func(string, error)
}

func (r *fakeClipboardRead) read(ctx context.Context, done func(string, error)) error {
	r.calls++
	r.ctx = ctx
	r.done = done
	return nil
}

func TestClipboardPasteCoalescesRepeatedKeysAndSingleUndo(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("abc")
	e.cursorCol = 1
	r := new(fakeClipboardRead)
	e.clipboardReader = r.read
	e.pasteClipboard()
	e.pasteClipboard()
	e.pasteClipboard()
	if r.calls != 1 || !e.PastePending() || e.Text() != "abc" {
		t.Fatal("read blocked/mutated or multiplied")
	}
	r.done("😀", nil)
	if e.PastePending() || e.Text() != "a😀😀😀bc" || r.ctx.Err() == nil {
		t.Fatalf("paste result %q", e.Text())
	}
	e.Undo()
	if e.Text() != "abc" {
		t.Fatal("paste burst was not one undo")
	}
	r.done("late", nil)
	if e.Text() != "abc" {
		t.Fatal("duplicate completion applied")
	}
}
func TestClipboardPasteRejectsStaleTargetsAndOwnership(t *testing.T) {
	cases := map[string]func(*CodeEditor){
		"cursor":         func(e *CodeEditor) { e.cursorCol++ },
		"selection":      func(e *CodeEditor) { e.hasSelection = true; e.selEndCol = 1 },
		"text-and-back":  func(e *CodeEditor) { old := e.Text(); e.SetText("changed"); e.SetText(old) },
		"extra-cursor":   func(e *CodeEditor) { e.additionalCursors = append(e.additionalCursors, cursorPos{0, 1}) },
		"hide":           func(e *CodeEditor) { e.Hide(); e.Show() },
		"disable":        func(e *CodeEditor) { e.SetEnabled(false); e.SetEnabled(true) },
		"reparent":       func(e *CodeEditor) { e.SetParent(NewVBox()) },
		"ancestor-hide":  func(e *CodeEditor) { e.Parent().Hide(); e.Parent().Show() },
		"focus-transfer": func(e *CodeEditor) { other := NewCodeEditor(); other.SetFocus(); e.SetFocus() },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			saved := focusWidget
			defer func() { focusWidget = saved }()
			parent := NewVBox()
			e := NewCodeEditor()
			e.SetParent(parent)
			e.SetText("abc")
			e.SetFocus()
			r := new(fakeClipboardRead)
			e.clipboardReader = r.read
			e.pasteClipboard()
			change(e)
			want := e.Text()
			r.done("stale", nil)
			if e.Text() != want || e.PastePending() {
				t.Fatalf("stale text %q pending=%v", e.Text(), e.PastePending())
			}
		})
	}
}
func TestClipboardPasteReplacementCancelsOldReader(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("abc")
	r := new(fakeClipboardRead)
	e.clipboardReader = r.read
	e.pasteClipboard()
	old, ctx := r.done, r.ctx
	e.cursorCol = 2
	e.pasteClipboard()
	if ctx.Err() == nil || r.calls != 2 {
		t.Fatal("old read not cancelled")
	}
	old("stale", nil)
	if e.Text() != "abc" || !e.PastePending() {
		t.Fatal("old result consumed new request")
	}
	r.done("!", nil)
	if e.Text() != "ab!c" {
		t.Fatal(e.Text())
	}
}
func TestClipboardPastePayloadAndAdmissionFailures(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		err        error
	}{
		{"empty", "", nil}, {"nul", "a\x00b", nil}, {"limit", strings.Repeat("x", maxClipboardTextBytes+1), nil}, {"cancel", "", context.Canceled}, {"error", "", errors.New("provider failed")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := NewCodeEditor()
			e.SetText("original")
			r := new(fakeClipboardRead)
			e.clipboardReader = r.read
			e.pasteClipboard()
			r.done(tt.text, tt.err)
			if e.Text() != "original" || e.PastePending() {
				t.Fatal("invalid payload applied/pending")
			}
		})
	}
	e := NewCodeEditor()
	e.clipboardReader = func(context.Context, func(string, error)) error { return async.ErrBusy }
	e.pasteClipboard()
	if e.PastePending() {
		t.Fatal("failed admission stayed pending")
	}
	if err := Clipboard.ReadTextAsync(nil, func(string, error) {}); !errors.Is(err, async.ErrInvalid) {
		t.Fatal(err)
	}
	if err := Clipboard.ReadTextAsync(context.Background(), nil); !errors.Is(err, async.ErrInvalid) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Clipboard.ReadTextAsync(ctx, func(string, error) {}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestClipboardPasteBurstLimitCancelsWithoutPartialInsert(t *testing.T) {
	e := NewCodeEditor()
	r := new(fakeClipboardRead)
	e.clipboardReader = r.read
	for i := 0; i < 65; i++ {
		e.pasteClipboard()
	}
	if e.PastePending() || r.ctx.Err() == nil {
		t.Fatal("unbounded burst")
	}
	r.done("!", nil)
	if e.Text() != "" {
		t.Fatal("failed burst partially applied")
	}
}
func TestClipboardPasteOtherTextControlsPreserveRules(t *testing.T) {
	t.Run("edit", func(t *testing.T) {
		e := NewEdit()
		e.SetText("old")
		e.SelectAll()
		e.SetMaxLength(5)
		r := new(fakeClipboardRead)
		e.clipboardReader = r.read
		e.paste()
		r.done("a\r\nbcdef", nil)
		if e.Text() != "a bcd" {
			t.Fatalf("normalization/limit: %q", e.Text())
		}
		e.paste()
		e.SetReadOnly(true)
		r.done("bad", nil)
		if e.Text() != "a bcd" {
			t.Fatal("readonly changed")
		}
	})
	t.Run("textarea", func(t *testing.T) {
		e := NewTextArea()
		r := new(fakeClipboardRead)
		e.clipboardReader = r.read
		e.paste()
		r.done("中文\n😀", nil)
		if e.Text() != "中文\n😀" {
			t.Fatalf("multi-line: %q", e.Text())
		}
		e.paste()
		e.cursorCol = 0
		r.done("stale", nil)
		if e.Text() != "中文\n😀" {
			t.Fatal("cursor guard")
		}
	})
	t.Run("search", func(t *testing.T) {
		e := NewSearchBox()
		e.SetText("old")
		e.SelectAll()
		r := new(fakeClipboardRead)
		e.clipboardReader = r.read
		e.pasteClipboard()
		r.done("a\r\nb", nil)
		if strings.ContainsAny(e.Text(), "\r\n") || e.Text() == "old" {
			t.Fatalf("search normalization: %q", e.Text())
		}
		before := e.Text()
		e.pasteClipboard()
		e.selectionAnchor = 0
		r.done("stale", nil)
		if e.Text() != before {
			t.Fatal("selection guard")
		}
	})
}
