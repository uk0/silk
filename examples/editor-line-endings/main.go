// Line Endings demonstrates byte-preserving conversion and CRLF-aware Enter.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type lineEndingsDemo struct {
	gui.Form
	editor                   *gui.CodeEditor
	buttons                  []*gui.Button
	stopHistory, stopEndings func()
}

const initialText = "中🙂 alpha\r\n    beta\n\tgamma\r\nlast"

func newLineEndingsDemo() *lineEndingsDemo {
	f := new(lineEndingsDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Line Endings")
	f.editor = gui.NewCodeEditor()
	f.editor.SetParent(f)
	f.editor.SetFilePath("sample.txt")
	f.editor.SetText(initialText)
	f.editor.MarkSaved()
	add := func(label string, run func()) *gui.Button {
		b := gui.NewButton1(label, nil)
		b.SetParent(f)
		b.Action().BindFunc0(func() { run(); f.refreshCommands() })
		f.buttons = append(f.buttons, b)
		return b
	}
	add("LF", func() {
		if err := f.editor.ConvertLineEndings(gui.LineEndingLF); err != nil {
			fmt.Println("ERROR", err)
		}
		f.editor.SetFocus()
	})
	add("CRLF", func() {
		if err := f.editor.ConvertLineEndings(gui.LineEndingCRLF); err != nil {
			fmt.Println("ERROR", err)
		}
		f.editor.SetFocus()
	})
	add("Undo", func() { f.editor.Undo(); f.editor.SetFocus() })
	add("Redo", func() { f.editor.Redo(); f.editor.SetFocus() })
	add("Mixed", func() { f.editor.SetText(initialText); f.editor.MarkSaved(); f.editor.SetFocus() })
	add("Lock", func() { f.editor.SetReadOnly(!f.editor.IsReadOnly()); f.editor.SetFocus() })
	add("Snapshot", func() {
		data, _ := json.Marshal(struct {
			Text                                 string
			Endings                              gui.EditorLineEndingInfo
			ReadOnly, CanUndo, CanRedo, Modified bool
			Revision                             uint64
			Cursors, Line, Column                int
		}{f.editor.Text(), f.editor.LineEndings(), f.editor.IsReadOnly(), f.editor.CanUndo(), f.editor.CanRedo(), f.editor.Modified(), f.editor.TextRevision(), f.editor.CursorCount(), f.editor.CursorLine(), f.editor.CursorCol()})
		fmt.Println("SNAPSHOT " + string(data))
	})
	f.stopHistory = f.editor.SubscribeHistoryChanged(f.refreshCommands)
	f.stopEndings = f.editor.SubscribeLineEndingsChanged(f.refreshCommands)
	f.refreshCommands()
	return f
}
func (f *lineEndingsDemo) refreshCommands() {
	if len(f.buttons) < 7 {
		return
	}
	for i, enabled := range []bool{f.editor.CanConvertLineEndings(gui.LineEndingLF), f.editor.CanConvertLineEndings(gui.LineEndingCRLF), f.editor.CanUndo(), f.editor.CanRedo()} {
		b := f.buttons[i]
		if b.IsEnabled() != enabled {
			b.SetEnabled(enabled)
			b.Action().SetEnabled(enabled)
			b.Update()
		}
	}
}
func (f *lineEndingsDemo) OnResize() {
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
	f := newLineEndingsDemo()
	defer f.stopHistory()
	defer f.stopEndings()
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
