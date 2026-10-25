// Editor Lines demonstrates reversible line-block movement.
// All contents are temporary. Snapshot prints the actual editor state as JSON.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type linesDemo struct {
	gui.Form
	editor  *gui.CodeEditor
	status  *gui.Label
	buttons []*gui.Button
}

func newLinesDemo() *linesDemo {
	f := new(linesDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Line Movement")
	f.editor = gui.NewCodeEditor()
	f.editor.SetParent(f)
	f.status = gui.NewLabel("Alt+Up/Down or buttons move selected lines; Undo restores the whole group.")
	f.status.SetParent(f)
	add := func(label string, run func()) {
		b := gui.NewButton1(label, nil)
		b.SetParent(f)
		b.Action().BindFunc0(run)
		f.buttons = append(f.buttons, b)
	}
	f.editor.SetFilePath("sample.txt")
	f.editor.SetText("中🙂 alpha\n    beta\n\tgamma\nlast\n")
	add("Move Up", func() { f.editor.MoveLinesUp(); f.editor.SetFocus() })
	add("Move Down", func() { f.editor.MoveLinesDown(); f.editor.SetFocus() })
	add("Undo", func() { f.editor.Undo(); f.editor.SetFocus() })
	add("Redo", func() { f.editor.Redo(); f.editor.SetFocus() })
	add("Add Caret", func() { f.editor.AddCursorBelow() })
	add("Snapshot", func() {
		data, _ := json.Marshal(struct {
			Text                   string
			CanMoveUp, CanMoveDown bool
			Cursors, Line, Column  int
		}{f.editor.Text(), f.editor.CanMoveLinesUp(), f.editor.CanMoveLinesDown(), f.editor.CursorCount(), f.editor.CursorLine(), f.editor.CursorCol()})
		fmt.Println("SNAPSHOT " + string(data))
	})
	return f
}

func (f *linesDemo) OnResize() {
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
	f := newLinesDemo()
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
