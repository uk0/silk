// Bookmarks demonstrates event-driven row anchors shared with a tool panel.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

type bookmarkDemo struct {
	gui.Form
	editor      *gui.CodeEditor
	buttons     []*gui.Button
	panel       *ged.BookmarksPanel
	unsubscribe func()
}

const initialText = "func demo() {\n    target()\n}\nlast"

func newBookmarkDemo() *bookmarkDemo {
	f := new(bookmarkDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Bookmarks")
	f.editor = gui.NewCodeEditor()
	f.editor.SetParent(f)
	f.editor.SetFilePath("sample.go")
	f.editor.SetText(initialText)
	f.editor.MarkSaved()
	add := func(label string, fn func()) {
		b := gui.NewButton1(label, nil)
		b.SetParent(f)
		b.Action().BindFunc0(fn)
		f.buttons = append(f.buttons, b)
	}
	f.panel = ged.NewBookmarksPanel()
	f.panel.SetParent(f)
	f.unsubscribe = f.editor.SubscribeBookmarksChanged(func() {
		var marks []ged.Bookmark
		for _, row := range f.editor.BookmarkLines() {
			text, _ := f.editor.LineText(row)
			marks = append(marks, ged.Bookmark{File: "sample.go", Line: row + 1, Label: text})
		}
		f.panel.SetBookmarks(marks)
	})
	f.panel.SigActivated(func(_ string, line int) { f.editor.NavigateToLineCol(line-1, 0) })
	f.panel.SigRemoveRequested(func(_ string, line int) { f.editor.SetBookmark(line-1, false) })
	add("Go to", f.editor.ShowGoToLine)
	add("Toggle", func() { f.editor.ToggleBookmark(); f.editor.SetFocus() })
	add("Previous", f.editor.PrevBookmark)
	add("Next", f.editor.NextBookmark)
	add("Undo", func() { f.editor.Undo(); f.editor.SetFocus() })
	add("Redo", func() { f.editor.Redo(); f.editor.SetFocus() })
	add("Fold + caret", func() { f.editor.FoldAll(); f.editor.AddCursorAtLine(0, 0); f.editor.SetFocus() })
	add("Snapshot", func() {
		data, _ := json.Marshal(struct {
			Text                               string
			Marks                              []int
			Rows                               []ged.Bookmark
			Line, Column, Cursors              int
			Modified, CanUndo, CanRedo, Folded bool
		}{f.editor.Text(), f.editor.BookmarkLines(), f.panel.Bookmarks(), f.editor.CursorLine(), f.editor.CursorCol(), f.editor.CursorCount(), f.editor.Modified(), f.editor.CanUndo(), f.editor.CanRedo(), f.editor.IsFolded(0) || f.editor.IsFolded(1)})
		fmt.Println("SNAPSHOT " + string(data))
	})
	return f
}
func (f *bookmarkDemo) OnResize() {
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
	if w >= 600 {
		pw := max(200, w*.3)
		place(f.editor, 12, y, w-pw-32, h-y-12)
		place(f.panel, w-pw-12, y, pw, h-y-12)
	} else {
		place(f.editor, 12, y, w-24, max(0, (h-y-24)*.65))
		place(f.panel, 12, y+max(0, (h-y-24)*.65)+12, w-24, max(0, (h-y-24)*.35))
	}
}
func main() {
	defer core.Close()
	f := newBookmarkDemo()
	defer f.unsubscribe()
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
	f.editor.NavigateToLineCol(1, 0)
	core.EventLoop()
}
