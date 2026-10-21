package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"testing"
)

func TestOutlineActivationReturnsInputToEditor(t *testing.T) {
	tabs := gui.NewTabWidget()
	editor := gui.NewCodeEditor()
	source := "package p\n\nfunc Alpha() {}\nfunc Bravo() {}\n"
	editor.SetText(source)
	tabs.AddTab(editor, "source.go", nil)
	p := ged.NewCodeOutlinePanel()
	p.SetSize(200, 160)
	p.SetEditor(editor)
	configureOutlineNavigation(p, tabs)
	p.SetFocus()
	p.OnKeyDown(gui.KeyEnd, false)
	if !p.HasFocus() {
		t.Fatal("arrow/selection unexpectedly stole outline focus")
	}
	p.OnKeyDown(gui.KeyEnter, false)
	if !editor.HasFocus() || editor.CursorLine() != 3 || editor.CursorCol() != 0 {
		t.Fatal("outline activation did not focus target cursor", editor.HasFocus(), editor.CursorLine(), editor.CursorCol())
	}
	editor.OnTextInput("//QA\n")
	if editor.Text() != "package p\n\nfunc Alpha() {}\n//QA\nfunc Bravo() {}\n" {
		t.Fatal("activation typed at wrong location", editor.Text())
	}
	editor.Undo()
	if editor.Text() != source {
		t.Fatal("activation typing undo lost source")
	}
}
