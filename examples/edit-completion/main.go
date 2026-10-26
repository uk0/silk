// Field completion demonstrates Unicode-safe SDK Edit transactions and menus.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

const initialText = "中🙂 ap 后缀"

type completionDemo struct {
	gui.Form
	input        *gui.Edit
	help, status *gui.Label
	buttons      []*gui.Button
	undo, redo   *gui.Button
	edits        int
}

func newCompletionDemo() *completionDemo {
	f := new(completionDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Unicode Field Completion")
	f.help = gui.NewLabel("Complete replaces the prefix at the caret; right-click or Shift+F10 opens editing actions.")
	f.help.SetParent(f)
	f.input = gui.NewEdit()
	f.input.SetParent(f)
	f.input.SetCompleter(gui.NewCompleter("apple🍎", "application", "中文🙂"))
	f.status = gui.NewLabel("")
	f.status.SetParent(f)
	add := func(label, icon string, fn func()) *gui.Button {
		b := gui.NewButton1(label, nil)
		b.SetIconName(icon)
		b.SetTextVisible(true)
		b.SetParent(f)
		b.Action().BindFunc0(fn)
		f.buttons = append(f.buttons, b)
		return b
	}
	add("Complete", "edit", func() {
		f.input.Completer().Filter(f.input.CompletionPrefix())
		f.input.AcceptCompletion(0)
		f.input.SetFocus()
		f.refresh()
	})
	f.undo = add("Undo", "edit-undo", func() { f.input.Undo(); f.input.SetFocus() })
	f.redo = add("Redo", "edit-redo", func() { f.input.Redo(); f.input.SetFocus() })
	add("Read-only", "preview", func() { f.input.SetReadOnly(!f.input.IsReadOnly()); f.refresh(); f.input.SetFocus() })
	add("Reset", "refresh", f.reset)
	add("Snapshot", "log", f.snapshot)
	f.input.SigTextEdited(func(interface{}, string) { f.edits++; f.refresh() })
	f.reset()
	return f
}
func (f *completionDemo) reset() {
	f.input.SetReadOnly(false)
	f.input.SetText(initialText)
	f.input.SetCaretPos(5)
	f.input.SetCompletionPrefixStart(len("中🙂 "))
	f.edits = 0
	f.refresh()
	f.input.SetFocus()
}
func (f *completionDemo) refresh() {
	f.undo.SetEnabled(f.input.CanUndo())
	f.redo.SetEnabled(f.input.CanRedo())
	f.status.SetText(fmt.Sprintf("Caret: %d | User edits: %d | Read-only: %t", f.input.CaretPos(), f.edits, f.input.IsReadOnly()))
}
func (f *completionDemo) snapshot() {
	a, b := f.input.Selection()
	data, _ := json.Marshal(struct {
		Text, Prefix                               string
		Caret, SelectionStart, SelectionEnd, Edits int
		CanUndo, CanRedo, ReadOnly                 bool
	}{f.input.Text(), f.input.CompletionPrefix(), f.input.CaretPos(), a, b, f.edits, f.input.CanUndo(), f.input.CanRedo(), f.input.IsReadOnly()})
	fmt.Println("SNAPSHOT " + string(data))
}
func (f *completionDemo) OnResize() {
	if f.input == nil {
		return
	}
	w, h := f.Size()
	w, h = max(0, w), max(0, h)
	place := func(c gui.IWidget, x, y, cw, ch float64) {
		x, y = max(0, min(x, w)), max(0, min(y, h))
		c.SetBounds(x, y, max(0, min(cw, w-x)), max(0, min(ch, h-y)))
	}
	place(f.help, 16, 16, w-32, 32)
	place(f.input, 16, 60, w-32, 36)
	x, y := 16.0, 112.0
	for _, b := range f.buttons {
		bw := max(124, b.SizeHints().Width+12)
		if x > 16 && x+bw > w-16 {
			x = 16
			y += 44
		}
		place(b, x, y, bw, 36)
		x += bw + 8
	}
	place(f.status, 16, y+48, w-32, 32)
}
func main() {
	defer core.Close()
	f := newCompletionDemo()
	defer f.input.CancelPendingPaste()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(1000, 400)
	f.Window().MoveToCenter()
	f.Show()
	f.input.SetFocus()
	core.EventLoop()
}
