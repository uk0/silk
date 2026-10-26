package main

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"strings"
	"testing"
)

func paletteReadOnlyDetails(w gui.IWidget) gui.IEdit {
	if e, ok := w.(interface {
		gui.IEdit
		IsReadOnly() bool
	}); ok && e.IsReadOnly() {
		return e
	}
	for _, c := range w.Children() {
		if e := paletteReadOnlyDetails(c); e != nil {
			return e
		}
	}
	return nil
}

func TestPaletteFullDescriptionFollowsKeyboardSelection(t *testing.T) {
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	long := strings.Repeat("Detailed command 中文🙂 ", 30)
	paletteCommands = []paletteCommand{{Name: "First", Hint: long, Fn: func() {}}, {Name: "Second", Hint: "Ctrl+Shift+S", Available: func() bool { return false }}}
	dlg, _ := newCommandPalette(nil)
	input, list := paletteInputAndList(t, dlg)
	details := paletteReadOnlyDetails(dlg)
	if details == nil {
		t.Fatal("no selectable full description: long hint is only clipped in the row")
	}
	if !strings.Contains(details.Text(), long) {
		t.Fatal("full hint lost")
	}
	input.OnKeyDown(gui.KeyDown, false)
	if list.ActiveIndex() != 1 || !strings.Contains(details.Text(), "Second") || strings.Contains(details.Text(), long) {
		t.Fatal("keyboard selection left stale details")
	}
	input.SetText("notfound")
	if list.Count() != 0 || strings.Contains(details.Text(), "Second") {
		t.Fatal("empty results kept old command details")
	}
	input.SetText("First")
	details.SelectAll()
	if e, ok := details.(interface{ SelectionText() string }); !ok || !strings.Contains(e.SelectionText(), long) {
		t.Fatal("full description is not selectable for copying")
	}
}

func TestPaletteDetailsRemainReadOnlyAndDoNotSubmit(t *testing.T) {
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	enabled := true
	calls := 0
	paletteCommands = []paletteCommand{{Name: "Action", Hint: strings.Repeat("Full detail ", 80), Available: func() bool { return enabled }, Fn: func() { calls++ }}}
	dlg, selection := newCommandPalette(nil)
	input, _ := paletteInputAndList(t, dlg)
	d := paletteReadOnlyDetails(dlg).(*paletteDetails)
	original := d.Text()
	d.SelectAll()
	d.OnTextInput("must not replace")
	d.OnKeyDown(gui.KeyEnter, false)
	if d.Text() != original || selection.command != nil || calls != 0 {
		t.Fatal("read-only detail mutated or ran command")
	}
	enabled = false
	input.OnKeyDown(gui.KeyEnter, false)
	if selection.command != nil || !strings.Contains(d.Text(), i18n.T("Unavailable in the current context.")) {
		t.Fatal("unavailable submit did not refresh feedback")
	}
}

func TestPaletteDetailsLayoutStaysAboveButtons(t *testing.T) {
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	for i := 0; i < 50; i++ {
		paletteCommands = append(paletteCommands, paletteCommand{Name: "Command", Hint: "Long explanation"})
	}
	for _, size := range [][2]float64{{320, 340}, {620, 520}, {900, 700}} {
		dlg, _ := newCommandPalette(nil)
		dlg.SetSize(size[0], size[1])
		dlg.Layout()
		box := dlg.Content().(*gui.VBox)
		box.Layout()
		_, list := paletteInputAndList(t, dlg)
		d := paletteReadOnlyDetails(dlg)
		if list.Height() < list.RowHeight() || list.Y()+list.Height() > d.Y() || d.Y()+d.Height() > box.Height()+.1 {
			t.Fatalf("overlap at %v: list=%v detail=%v box=%v", size, list.Bounds1(), d.Bounds1(), box.Bounds1())
		}
	}
}

func TestPaletteRowPresentationKeepsStatusSeparate(t *testing.T) {
	old := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocale(old) })
	for _, locale := range []string{"en", "zh-CN"} {
		i18n.SetLocale(locale)
		c := paletteCommand{Name: strings.Repeat("Long name ", 30), Hint: strings.Repeat("Long detail 中文🙂 ", 50), Available: func() bool { return false }}
		row := paletteRowPresentation(c)
		if row.TrailingText != i18n.T("Unavailable") || row.SecondaryText != c.Hint || !row.Muted {
			t.Fatal("status lost behind description")
		}
		if !strings.Contains(paletteDescription(c), c.Hint) {
			t.Fatal("full hint truncated")
		}
	}
}
