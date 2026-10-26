package gui

import (
	"context"
	"strings"
	"testing"
)

func TestEditHistoryUnicodeReplacementAndSelection(t *testing.T) {
	e := NewEdit()
	e.SetSize(200, 32)
	e.SetText("A中🙂Z")
	e.SetSelection(1, 3)
	changed, edited := 0, 0
	e.SigTextChanged(func(interface{}, string) { changed++ })
	e.SigTextEdited(func(interface{}, string) { edited++ })
	e.OnTextInput("界🚀")
	if e.Text() != "A界🚀Z" || !e.CanUndo() || e.CanRedo() {
		t.Fatal("missing delta")
	}
	e.Undo()
	a, b := e.Selection()
	if e.Text() != "A中🙂Z" || a != 1 || b != 3 || e.CanUndo() || !e.CanRedo() {
		t.Fatal("undo lost text/selection")
	}
	e.Redo()
	a, b = e.Selection()
	if e.Text() != "A界🚀Z" || a != 3 || b != 3 || !e.CanUndo() || e.CanRedo() {
		t.Fatal("redo lost text/caret")
	}
	if changed != 3 || edited != 3 {
		t.Fatalf("signals %d/%d", changed, edited)
	}
	e.Undo()
	e.OnTextInput("new")
	if e.CanRedo() {
		t.Fatal("new edit retained divergent redo")
	}
}
func TestEditHistoryDeletionKeepsOriginalCaret(t *testing.T) {
	for _, key := range []int{KeyBackSpace, KeyDelete} {
		e := NewEdit()
		e.SetText("abc")
		e.SetCaretPos(1)
		e.OnKeyDown(key, false)
		e.Undo()
		a, b := e.Selection()
		if e.Text() != "abc" || a != 1 || b != 1 {
			t.Fatalf("key %d selection %d:%d", key, a, b)
		}
	}
}
func TestEditHistoryConstraintsAndProgrammaticReset(t *testing.T) {
	e := NewEdit()
	e.SetText("1")
	e.SetCaretPos(1)
	e.OnTextInput("2")
	e.SetReadOnly(true)
	e.Undo()
	if e.Text() != "12" || e.CanUndo() {
		t.Fatal("read-only undo")
	}
	e.SetReadOnly(false)
	if !e.CanUndo() {
		t.Fatal("readonly destroyed history")
	}
	e.SetEnabled(false)
	e.Undo()
	if e.Text() != "12" || e.CanUndo() {
		t.Fatal("disabled undo")
	}
	e.SetEnabled(true)
	e.SetValidator(&IntValidator{Bottom: 0, Top: 100})
	if e.CanUndo() {
		t.Fatal("new constraints retained history")
	}
	e.OnTextInput("3")
	if e.Text() != "12" || e.CanUndo() {
		t.Fatal("invalid typing recorded history")
	}
	e.SelectAll()
	e.OnTextInput("9")
	e.SetText("invalid")
	if e.Text() != "9" || !e.CanUndo() {
		t.Fatal("rejected set erased history")
	}
	e.Undo()
	if e.Text() != "12" {
		t.Fatal("invalid set changed undo")
	}
	e.SetMaxLength(1)
	if e.Text() != "1" || e.CanRedo() || e.CanUndo() {
		t.Fatal("length reset retained history")
	}
	e.SetText("3")
	if e.CanUndo() || e.CanRedo() {
		t.Fatal("SetText retained undo")
	}
}
func TestEditHistoryBoundsAndNoop(t *testing.T) {
	e := NewEdit()
	e.SetText("x")
	for i := 0; i < 300; i++ {
		e.SelectAll()
		e.OnTextInput(string(rune('a' + i%20)))
	}
	if len(e.history.entries) != editHistoryLimit || e.history.bytes > editHistoryBytes {
		t.Fatal("unbounded history")
	}
	e.SelectAll()
	before := len(e.history.entries)
	e.OnTextInput(e.Text())
	if len(e.history.entries) != before {
		t.Fatal("noop recorded")
	}
	// Exercise bounded retention without turning a text-layout test into a MiB render.
	e.recordEdit(editDelta{after: strings.Repeat("x", editHistoryBytes+1)})
	if e.CanUndo() || e.CanRedo() || e.history.bytes != 0 {
		t.Fatal("oversize left a hole in history")
	}
}
func TestEditHistoryCancelsLatePasteAndDoesNotAllocateToQuery(t *testing.T) {
	e := NewEdit()
	e.OnTextInput("one")
	var finish func(string, error)
	e.clipboardReader = func(_ context.Context, done func(string, error)) error { finish = done; return nil }
	e.Paste()
	if !e.PastePending() {
		t.Fatal("paste not pending")
	}
	e.Undo()
	finish("late", nil)
	if e.Text() != "" || e.PastePending() {
		t.Fatal("late paste applied after undo")
	}
	if n := testing.AllocsPerRun(1000, func() { e.CanUndo(); e.CanRedo(); e.CanCopy(); e.CanPaste() }); n != 0 {
		t.Fatalf("queries allocate %g", n)
	}
}
func BenchmarkEditHistoryAvailability(b *testing.B) {
	e := NewEdit()
	e.OnTextInput("test")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.CanUndo()
		e.CanRedo()
		e.CanCopy()
		e.CanPaste()
	}
}
