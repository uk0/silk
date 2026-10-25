// Editor Format demonstrates offline Go/JSON formatting with one-step Undo.
// All contents are temporary. Snapshot prints the actual editor state as JSON.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type formatterDemo struct {
	gui.Form
	editor  *gui.CodeEditor
	status  *gui.Label
	buttons []*gui.Button
}

func newFormatterDemo() *formatterDemo {
	f := new(formatterDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Offline Formatting")
	f.editor = gui.NewCodeEditor()
	f.editor.SetParent(f)
	f.status = gui.NewLabel("Temporary data. Formatting runs in the background; Undo restores exact bytes.")
	f.status.SetParent(f)
	load := func(path, text string) {
		f.editor.CancelFormat()
		f.editor.SetFilePath(path)
		f.editor.SetText(text)
		f.status.SetText("Loaded " + path)
		f.editor.SetFocus()
	}
	add := func(label string, run func()) {
		b := gui.NewButton1(label, nil)
		b.SetParent(f)
		b.Action().BindFunc0(run)
		f.buttons = append(f.buttons, b)
	}
	format := func(compact bool) {
		f.status.SetText("Formatting...")
		err := f.editor.FormatDocumentAsync(gui.EditorFormatOptions{Compact: compact}, func(err error) {
			if err != nil {
				f.status.SetText(err.Error())
			} else {
				f.status.SetText("Formatted. Undo preserves the previous document.")
			}
		})
		if err != nil {
			f.status.SetText(err.Error())
		}
		f.editor.SetFocus()
	}
	add("Load JSON", func() {
		load("sample.json", `{"name":"Silk 中🙂","n":9007199254740993,"n":1e+009,"items":[true,null,{}],"escaped":"\u0041"}`)
	})
	add("Load Go", func() { load("sample.go", "package main\nfunc main( ){println(\"Silk 中🙂\")}\n") })
	add("Pretty", func() { format(false) })
	add("Compact", func() { format(true) })
	add("Undo", func() { f.editor.Undo(); f.editor.SetFocus() })
	add("Redo", func() { f.editor.Redo(); f.editor.SetFocus() })
	add("Cancel", func() { f.editor.CancelFormat(); f.status.SetText("Formatting cancelled; text kept.") })
	add("Snapshot", func() {
		data, _ := json.Marshal(struct {
			Text, Language string
			Formatting     bool
		}{f.editor.Text(), f.editor.Language(), f.editor.Formatting()})
		fmt.Println("SNAPSHOT " + string(data))
	})
	f.buttons[0].Action().Trigger(nil)
	return f
}

func (f *formatterDemo) OnResize() {
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
	f := newFormatterDemo()
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
