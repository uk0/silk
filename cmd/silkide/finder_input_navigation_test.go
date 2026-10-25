package main

import (
	"github.com/uk0/silk/gui"
	"testing"
)

func paletteInputAndList(t *testing.T, dlg *gui.Dialog) (*gui.Edit, *gui.ListWidget) {
	t.Helper()
	var input *gui.Edit
	var list *gui.ListWidget
	var visit func(gui.IWidget)
	visit = func(w gui.IWidget) {
		if e, ok := w.(*gui.Edit); ok {
			input = e
		}
		if l, ok := w.(*gui.ListWidget); ok {
			list = l
		}
		for _, child := range w.Children() {
			visit(child)
		}
	}
	visit(dlg)
	if input == nil || list == nil {
		t.Fatal("missing search controls")
	}
	return input, list
}

func TestPaletteInputArrowsSelectBeforeSubmit(t *testing.T) {
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	called := ""
	paletteCommands = []paletteCommand{{Name: "First", Fn: func() { called = "first" }}, {Name: "Second", Fn: func() { called = "second" }}}
	dlg, selection := newCommandPalette(nil)
	input, list := paletteInputAndList(t, dlg)
	input.SetText("")
	input.OnKeyDown(gui.KeyDown, false)
	if list.ActiveIndex() != 1 {
		t.Fatalf("input Down left selection at %d, want second row", list.ActiveIndex())
	}
	input.OnKeyDown(gui.KeyEnter, false)
	if called != "" {
		t.Fatal("ran before modal teardown")
	}
	selection.run(gui.DialogOK)
	if called != "second" {
		t.Fatal("Enter ignored selected row", called)
	}
}

func TestPaletteInputSubmitUsesExistingSelection(t *testing.T) {
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	called := ""
	paletteCommands = []paletteCommand{{Name: "First", Fn: func() { called = "first" }}, {Name: "Second", Fn: func() { called = "second" }}}
	dlg, selection := newCommandPalette(nil)
	input, list := paletteInputAndList(t, dlg)
	list.SetActiveIndex(1)
	input.OnKeyDown(gui.KeyEnter, false)
	selection.run(gui.DialogOK)
	if called != "second" {
		t.Fatal("input Enter always picks row zero", called)
	}
}

func TestPaletteInputNavigationUnavailableAndQueryReplacement(t *testing.T) {
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	calls := 0
	paletteCommands = []paletteCommand{
		{Name: "First", Fn: func() { calls++ }},
		{Name: "Second", Fn: func() { calls++ }, Available: func() bool { return false }},
	}
	dlg, selection := newCommandPalette(nil)
	input, list := paletteInputAndList(t, dlg)
	input.OnKeyDown(gui.KeyDown, false)
	input.OnKeyDown(gui.KeyEnter, false)
	if selection.command != nil || calls != 0 {
		t.Fatal("unavailable command activated")
	}
	input.SetText("Fir")
	if list.Count() != 1 || list.ActiveIndex() != 0 {
		t.Fatal("query replacement retained stale selection")
	}
	input.SetText("missing")
	input.OnKeyDown(gui.KeyDown, true)
	input.OnKeyDown(gui.KeyEnter, false)
	if list.Count() != 0 || selection.command != nil || calls != 0 {
		t.Fatal("empty query accepted old result")
	}
}
