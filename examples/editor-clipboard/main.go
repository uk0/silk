// Editor Clipboard demonstrates safe copy/cut and bounded paste commands.
// All contents are temporary. Snapshot prints the actual editor state as JSON.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type clipboardDemo struct {
	gui.Form
	editor             *gui.CodeEditor
	status             *gui.Label
	buttons            []*gui.Button
	unsubscribeHistory func()
}

func newClipboardDemo() *clipboardDemo {
	f := new(clipboardDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Clipboard Commands")
	f.editor = gui.NewCodeEditor()
	f.editor.SetParent(f)
	f.status = gui.NewLabel("Copy/Cut report errors; Paste restores focus before its guarded native read.")
	f.status.SetParent(f)
	add := func(label string, run func()) *gui.Button {
		b := gui.NewButton1(label, nil)
		b.SetParent(f)
		b.Action().BindFunc0(run)
		f.buttons = append(f.buttons, b)
		return b
	}
	f.editor.SetFilePath("sample.txt")
	f.editor.SetText("中🙂 alpha\n    beta\n\tgamma\nlast\n")
	report := func(err error) {
		if err != nil {
			f.status.SetText("Clipboard error: " + err.Error())
		}
	}
	add("Copy", func() { report(f.editor.Copy()); f.editor.SetFocus() })
	add("Cut", func() { report(f.editor.Cut()); f.editor.SetFocus() })
	add("Paste", func() { f.editor.SetFocus(); f.editor.Paste() })
	add("Select All", func() { f.editor.SelectAll(); f.editor.SetFocus() })
	undo := add("Undo", func() { f.editor.Undo(); f.editor.SetFocus() })
	redo := add("Redo", func() { f.editor.Redo(); f.editor.SetFocus() })
	add("Add Caret", func() { f.editor.AddCursorBelow() })
	add("Snapshot", func() {
		data, _ := json.Marshal(struct {
			Text, Selected                          string
			CanCopy, CanCut, CanPaste, CanSelectAll bool
			CanUndo, CanRedo, PastePending          bool
			Cursors, Line, Column                   int
		}{f.editor.Text(), f.editor.SelectedText(), f.editor.CanCopy(), f.editor.CanCut(), f.editor.CanPaste(), f.editor.CanSelectAll(), f.editor.CanUndo(), f.editor.CanRedo(), f.editor.PastePending(), f.editor.CursorCount(), f.editor.CursorLine(), f.editor.CursorCol()})
		fmt.Println("SNAPSHOT " + string(data))
	})
	refreshHistory := func() {
		for i, b := range []*gui.Button{undo, redo} {
			enabled := f.editor.CanUndo()
			if i == 1 {
				enabled = f.editor.CanRedo()
			}
			if b.IsEnabled() != enabled {
				b.SetEnabled(enabled)
				b.Action().SetEnabled(enabled)
				b.Update()
			}
		}
	}
	f.unsubscribeHistory = f.editor.SubscribeHistoryChanged(refreshHistory)
	refreshHistory()
	return f
}

func (f *clipboardDemo) OnResize() {
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
	f := newClipboardDemo()
	defer f.unsubscribeHistory()
	defer f.editor.CancelPendingPaste()
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
