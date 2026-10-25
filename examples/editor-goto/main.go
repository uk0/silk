// Go to Location demonstrates editor-owned navigation input and SDK positions.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type goToDemo struct {
	gui.Form
	editor  *gui.CodeEditor
	buttons []*gui.Button
}

const initialText = "first\r\n中🙂 target\r\n\tthird\nlast"

func newGoToDemo() *goToDemo {
	f := new(goToDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Go to Location")
	f.editor = gui.NewCodeEditor()
	f.editor.SetParent(f)
	f.editor.SetFilePath("sample.txt")
	f.editor.SetText(initialText)
	f.editor.MarkSaved()
	add := func(label string, fn func()) {
		b := gui.NewButton1(label, nil)
		b.SetParent(f)
		b.Action().BindFunc0(fn)
		f.buttons = append(f.buttons, b)
	}
	add("Go to", f.editor.ShowGoToLine)
	add("Lock", func() { f.editor.SetReadOnly(!f.editor.IsReadOnly()); f.editor.SetFocus() })
	add("Reset", func() {
		f.editor.HideGoToLine()
		f.editor.SetText(initialText)
		f.editor.MarkSaved()
		f.editor.SetFocus()
	})
	add("Snapshot", func() {
		data, _ := json.Marshal(struct {
			Text                                                            string
			Revision                                                        uint64
			Modified, CanUndo, CanRedo, ReadOnly, GoToVisible, HasSelection bool
			Line, Column, Cursors                                           int
		}{f.editor.Text(), f.editor.TextRevision(), f.editor.Modified(), f.editor.CanUndo(), f.editor.CanRedo(), f.editor.IsReadOnly(), f.editor.GoToLineVisible(), f.editor.HasSelection(), f.editor.CursorLine(), f.editor.CursorCol(), f.editor.CursorCount()})
		fmt.Println("SNAPSHOT " + string(data))
	})
	return f
}
func (f *goToDemo) OnResize() {
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
	place(f.editor, 12, y, w-24, h-y-12)
}
func main() {
	defer core.Close()
	f := newGoToDemo()
	defer f.editor.HideGoToLine()
	defer f.editor.CancelPendingPaste()
	defer f.editor.CancelFormat()
	defer f.editor.CancelFind()
	defer f.editor.CancelCompletion()
	defer f.editor.CancelSymbolIndex()
	defer f.editor.CancelGitStatus()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(920, 640)
	f.Window().MoveToCenter()
	f.Show()
	f.editor.SetFocus()
	core.EventLoop()
}
