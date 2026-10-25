// Editor Read-only demonstrates permission changes without losing history.
// SetText is the explicit host reload; it remains allowed while locked.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type readOnlyDemo struct {
	gui.Form
	editor  *gui.CodeEditor
	buttons []*gui.Button
}

const initialText = "中🙂 alpha\n    beta\n\tgamma\nlast\n"

func newReadOnlyDemo() *readOnlyDemo {
	f := new(readOnlyDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Read-only Editor")
	f.editor = gui.NewCodeEditor()
	f.editor.SetParent(f)
	f.editor.SetFilePath("sample.txt")
	f.editor.SetText(initialText)
	f.editor.MarkSaved()
	f.editor.SetReadOnly(true)
	add := func(label string, run func()) *gui.Button {
		b := gui.NewButton1(label, nil)
		b.SetParent(f)
		b.Action().BindFunc0(run)
		f.buttons = append(f.buttons, b)
		return b
	}
	add("Lock / Unlock", func() { f.editor.SetReadOnly(!f.editor.IsReadOnly()); f.editor.SetFocus() })
	add("Copy", func() {
		if err := f.editor.Copy(); err != nil {
			fmt.Println("COPY_ERROR", err)
		}
		f.editor.SetFocus()
	})
	add("Select All", func() { f.editor.SelectAll(); f.editor.SetFocus() })
	add("Find", func() { f.editor.ShowFind(true) })
	add("Reload", func() { f.editor.SetText(initialText); f.editor.MarkSaved(); f.editor.SetFocus() })
	add("Snapshot", func() {
		data, _ := json.Marshal(struct {
			Text, Selected                                                  string
			ReadOnly, CanCopy, CanCut, CanPaste, CanUndo, CanRedo, Modified bool
			Revision                                                        uint64
			Cursors, Line, Column                                           int
		}{f.editor.Text(), f.editor.SelectedText(), f.editor.IsReadOnly(), f.editor.CanCopy(), f.editor.CanCut(), f.editor.CanPaste(), f.editor.CanUndo(), f.editor.CanRedo(), f.editor.Modified(), f.editor.TextRevision(), f.editor.CursorCount(), f.editor.CursorLine(), f.editor.CursorCol()})
		fmt.Println("SNAPSHOT " + string(data))
	})
	return f
}
func (f *readOnlyDemo) OnResize() {
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
	f := newReadOnlyDemo()
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
