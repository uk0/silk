// Cleanup demonstrates explicit, byte-preserving whitespace/EOF commands.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type cleanupDemo struct {
	gui.Form
	editor    *gui.CodeEditor
	buttons   []*gui.Button
	status    *gui.Label
	stop      func()
	lastError string
}

const initialText = "中🙂 alpha \t\r\n    beta\t\n\tgamma  \r\nlast  "

func newCleanupDemo() *cleanupDemo {
	f := new(cleanupDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Text Cleanup")
	f.editor = gui.NewCodeEditor()
	f.editor.SetParent(f)
	f.editor.SetFilePath("sample.txt")
	f.status = gui.NewLabel("Explicit cleanup; original separators and Undo are preserved.")
	f.status.SetParent(f)
	add := func(label string, fn func()) {
		b := gui.NewButton1(label, nil)
		b.SetParent(f)
		b.Action().BindFunc0(func() { fn(); f.refresh() })
		f.buttons = append(f.buttons, b)
	}
	run := func(fn func() error) {
		f.lastError = ""
		if err := fn(); err != nil {
			f.lastError = err.Error()
			f.status.SetText(f.lastError)
		} else {
			f.status.SetText("Done; Undo restores the exact original bytes and anchors.")
		}
		f.editor.SetFocus()
	}
	add("Select all", func() { f.editor.SelectAll(); f.editor.SetFocus() })
	add("Carets", func() { f.editor.NavigateToLineCol(3, 6); f.editor.AddCursorAtLine(0, 10); f.editor.SetFocus() })
	add("Trim", func() { run(f.editor.TrimTrailingWhitespaceChecked) })
	add("Final newline", func() { run(f.editor.EnsureFinalNewline) })
	add("Undo", func() { f.editor.Undo(); f.editor.SetFocus() })
	add("Redo", func() { f.editor.Redo(); f.editor.SetFocus() })
	add("Reset", f.reset)
	add("Lock", func() { f.editor.SetReadOnly(!f.editor.IsReadOnly()); f.editor.SetFocus() })
	add("Snapshot", func() {
		data, _ := json.Marshal(struct {
			Text, Selected, Error                              string
			Endings                                            gui.EditorLineEndingInfo
			Marks                                              []int
			ReadOnly, CanUndo, CanRedo, Modified, FinalNewline bool
			Revision                                           uint64
			Cursors, Line, Column                              int
		}{f.editor.Text(), f.editor.SelectedText(), f.lastError, f.editor.LineEndings(), f.editor.BookmarkLines(), f.editor.IsReadOnly(), f.editor.CanUndo(), f.editor.CanRedo(), f.editor.Modified(), f.editor.HasFinalNewline(), f.editor.TextRevision(), f.editor.CursorCount(), f.editor.CursorLine(), f.editor.CursorCol()})
		fmt.Println("SNAPSHOT " + string(data))
	})
	f.reset()
	f.stop = f.editor.SubscribeHistoryChanged(f.refresh)
	f.refresh()
	return f
}
func (f *cleanupDemo) reset() {
	f.editor.SetReadOnly(false)
	f.editor.SetText(initialText)
	f.editor.SetBookmarkLines([]int{1, 3})
	f.editor.MarkSaved()
	f.lastError = ""
	f.status.SetText("Explicit cleanup; original separators and Undo are preserved.")
	f.editor.SetFocus()
}
func (f *cleanupDemo) refresh() {
	if len(f.buttons) < 9 {
		return
	}
	for i, enabled := range map[int]bool{2: !f.editor.IsReadOnly(), 3: !f.editor.IsReadOnly() && !f.editor.HasFinalNewline() && f.editor.Text() != "", 4: f.editor.CanUndo(), 5: f.editor.CanRedo()} {
		b := f.buttons[i]
		if b.IsEnabled() != enabled {
			b.SetEnabled(enabled)
			b.Action().SetEnabled(enabled)
			b.Update()
		}
	}
}
func (f *cleanupDemo) OnResize() {
	if f.editor == nil {
		return
	}
	w, h := f.Size()
	w, h = max(0, w), max(0, h)
	place := func(c gui.IWidget, x, y, cw, ch float64) {
		x, y = max(0, min(x, w)), max(0, min(y, h))
		c.SetBounds(x, y, max(0, min(cw, w-x)), max(0, min(ch, h-y)))
	}
	x, y, row := 12.0, 12.0, 0.0
	for _, b := range f.buttons {
		hint := b.SizeHints()
		bw, bh := max(112, hint.Width+12), max(32, hint.Height)
		if x > 12 && x+bw > w-12 {
			x = 12
			y += row + 8
			row = 0
		}
		row = max(row, bh)
		place(b, x, y, bw, bh)
		x += bw + 8
	}
	y += row + 12
	place(f.status, 12, y, w-24, 28)
	y += 36
	place(f.editor, 12, y, w-24, h-y-12)
}
func main() {
	defer core.Close()
	f := newCleanupDemo()
	defer f.stop()
	defer f.editor.CancelPendingPaste()
	defer f.editor.CancelFormat()
	defer f.editor.CancelFind()
	defer f.editor.CancelCompletion()
	defer f.editor.CancelSymbolIndex()
	defer f.editor.CancelGitStatus()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(1200, 640)
	f.Window().MoveToCenter()
	f.Show()
	f.editor.SetFocus()
	core.EventLoop()
}
