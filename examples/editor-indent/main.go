// Editor Indent demonstrates configurable indentation and exact Undo.
// All contents are temporary. Snapshot prints the actual editor state as JSON.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type indentDemo struct {
	gui.Form
	editor  *gui.CodeEditor
	status  *gui.Label
	buttons []*gui.Button
}

func newIndentDemo() *indentDemo {
	f := new(indentDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Indentation")
	f.editor = gui.NewCodeEditor()
	f.editor.SetParent(f)
	f.status = gui.NewLabel("Temporary data. Select lines or add carets; each indent is one Undo.")
	f.status.SetParent(f)
	add := func(label string, run func()) {
		b := gui.NewButton1(label, nil)
		b.SetParent(f)
		b.Action().BindFunc0(run)
		f.buttons = append(f.buttons, b)
	}
	f.editor.SetFilePath("sample.txt")
	f.editor.SetText("中🙂 alpha\n    beta\n\tgamma\nlast\n")
	add("Spaces: 2", func() {
		f.editor.SetTabSize(2)
		f.editor.SetInsertSpaces(true)
		f.status.SetText("Future input: spaces, width 2; existing bytes unchanged.")
		f.editor.SetFocus()
	})
	add("Tabs: 4", func() {
		f.editor.SetTabSize(4)
		f.editor.SetInsertSpaces(false)
		f.status.SetText("Future input: tabs, width 4; existing bytes unchanged.")
		f.editor.SetFocus()
	})
	add("Indent", func() { f.editor.IndentLines(); f.editor.SetFocus() })
	add("Outdent", func() { f.editor.OutdentLines(); f.editor.SetFocus() })
	add("Undo", func() { f.editor.Undo(); f.editor.SetFocus() })
	add("Redo", func() { f.editor.Redo(); f.editor.SetFocus() })
	add("Add Caret", func() { f.editor.AddCursorBelow() })
	add("Snapshot", func() {
		data, _ := json.Marshal(struct {
			Text                           string
			InsertSpaces                   bool
			TabSize, Cursors, Line, Column int
		}{f.editor.Text(), f.editor.InsertSpaces(), f.editor.TabSize(), f.editor.CursorCount(), f.editor.CursorLine(), f.editor.CursorCol()})
		fmt.Println("SNAPSHOT " + string(data))
	})
	return f
}

func (f *indentDemo) OnResize() {
	if f.editor == nil {
		return
	}
	w, h := f.Size()
	w, h = max(0, w), max(0, h)
	place := func(c gui.IWidget, x, y, cw, ch float64) {
		x, y = max(0, min(x, w)), max(0, min(y, h))
		c.SetBounds(x, y, max(0, min(cw, w-x)), max(0, min(ch, h-y)))
	}
	x, y, rowHeight := 12.0, 12.0, 0.0
	for _, b := range f.buttons {
		hint := b.SizeHints()
		bw, bh := hint.Width, hint.Height
		bw, bh = max(96, bw+12), max(32, bh)
		if x > 12 && x+bw > w-12 {
			x = 12
			y += rowHeight + 8
			rowHeight = 0
		}
		rowHeight = max(rowHeight, bh)
		place(b, x, y, bw, bh)
		x += bw + 8
	}
	y += rowHeight + 12
	place(f.status, 12, y, w-24, 28)
	y += 36
	place(f.editor, 12, y, w-24, h-y-12)
}

func main() {
	defer core.Close()
	f := newIndentDemo()
	defer f.editor.CancelFormat()
	defer f.editor.CancelFind()
	defer f.editor.CancelCompletion()
	defer f.editor.CancelSymbolIndex()
	defer f.editor.CancelGitStatus()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(980, 620)
	f.Window().MoveToCenter()
	f.Show()
	f.editor.SetFocus()
	core.EventLoop()
}
