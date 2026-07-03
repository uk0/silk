// Silk Text Editor
//
// A simple text editor with New/Save buttons, a multi-line edit area,
// and a status bar showing character and line counts.
//
// Build:
//
//	CGO_CFLAGS="$(pkg-config --cflags cairo)" go build -o texteditor ./examples/texteditor/
//
// Run:
//
//	./texteditor
package main

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"os"
	"strings"
)

func main() {
	form := gui.NewForm()
	form.SetTitle("Silk Text Editor")

	// Menu bar using buttons at top
	btnNew := gui.NewButton1("New", nil)
	btnNew.SetParent(form)
	btnNew.SetBounds(5, 2, 50, 24)

	btnOpen := gui.NewButton1("Open", nil)
	btnOpen.SetParent(form)
	btnOpen.SetBounds(60, 2, 50, 24)

	btnSave := gui.NewButton1("Save", nil)
	btnSave.SetParent(form)
	btnSave.SetBounds(115, 2, 50, 24)

	btnWrap := gui.NewButton1("Wrap", nil)
	btnWrap.SetParent(form)
	btnWrap.SetBounds(180, 2, 50, 24)

	// File path label
	fileLabel := gui.NewLabel("Untitled")
	fileLabel.SetParent(form)
	fileLabel.SetBounds(240, 5, 300, 20)
	fileLabel.SetFont(paint.NewFont(gui.Theme().Font.Family(), 11, false, true))

	// Multi-line text editor
	edit := gui.NewEdit()
	edit.SetMultiLine(true)
	edit.SetWrap(true)
	edit.SetParent(form)
	edit.SetBounds(0, 28, 600, 350)

	// Status bar
	status := gui.NewLabel("Lines: 1 | Chars: 0")
	status.SetParent(form)
	status.SetBounds(5, 382, 400, 20)

	// Track current file path
	var currentFile string

	// Update status on text change
	updateStatus := func() {
		text := edit.Text()
		chars := len(text)
		lines := 1
		if chars > 0 {
			lines = strings.Count(text, "\n") + 1
		}
		status.SetText(fmt.Sprintf("Lines: %d | Chars: %d", lines, chars))
	}

	edit.SigTextChanged(func(_ interface{}, _ string) {
		updateStatus()
	})

	// New: clear editor
	btnNew.Action().BindFunc0(func() {
		edit.SetText("")
		currentFile = ""
		fileLabel.SetText("Untitled")
		updateStatus()
	})

	// Open: load file
	btnOpen.Action().BindFunc0(func() {
		filename := gui.OpenFileDialog()
		if filename == "" {
			return
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			gui.ShowMessageDialog(form, "Error", err.Error())
			return
		}
		edit.SetText(string(data))
		currentFile = filename
		fileLabel.SetText(filename)
		updateStatus()
	})

	// Save: write file
	btnSave.Action().BindFunc0(func() {
		filename := currentFile
		if filename == "" {
			filename = gui.SaveFileDialog()
			if filename == "" {
				return
			}
		}
		err := os.WriteFile(filename, []byte(edit.Text()), 0644)
		if err != nil {
			gui.ShowMessageDialog(form, "Error", err.Error())
			return
		}
		currentFile = filename
		fileLabel.SetText(filename)
	})

	// Toggle wrap
	wrapEnabled := true
	btnWrap.Action().BindFunc0(func() {
		wrapEnabled = !wrapEnabled
		edit.SetWrap(wrapEnabled)
		if wrapEnabled {
			btnWrap.SetText("Wrap")
		} else {
			btnWrap.SetText("NoWrap")
		}
	})

	// Initial status update
	updateStatus()

	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(600, 410)
	form.Window().MoveToCenter()
	form.Show()
	core.EventLoop()
}
