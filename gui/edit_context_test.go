package gui

import (
	"errors"
	"github.com/uk0/silk/paint"
	"testing"
)

type editSelectionInk struct {
	paint.Painter
	brush      paint.Color
	highlights int
}

func (g *editSelectionInk) SetBrush1(c paint.Color) { g.brush = c; g.Painter.SetBrush1(c) }
func (g *editSelectionInk) Fill() {
	if g.brush == Theme().HighLightColor {
		g.highlights++
	}
	g.Painter.Fill()
}

func TestEditContextSelectionRemainsPaintedWithoutCaretFocus(t *testing.T) {
	old := focusWidget
	focusWidget = nil
	t.Cleanup(func() { focusWidget = old })
	e := NewEdit()
	e.SetSize(240, 40)
	e.SetText("selected 中🙂")
	e.SelectAll()
	m := NewPopupMenu()
	m.SetParent(e)
	e.contextMenu = m
	draw := func() int {
		p := paint.NewPixmap(240, 40)
		defer paint.CloseSurface(p)
		g := &editSelectionInk{Painter: p.NewPainter()}
		defer paint.ClosePainter(g.Painter)
		e.Draw(g)
		return g.highlights
	}
	m.NakedWidget().hidden = true
	if draw() != 0 {
		t.Fatal("unfocused closed field unexpectedly highlighted")
	}
	m.NakedWidget().hidden = false
	if e.HasFocus() || !e.ownsVisibleContextMenu() || draw() == 0 {
		t.Fatal("popup target highlight lost")
	}
	m.NakedWidget().hidden = true
	if e.ownsVisibleContextMenu() || draw() != 0 {
		t.Fatal("dismissed popup kept highlight")
	}
	m.NakedWidget().hidden = true
	m.SetParent(NewForm())
	m.NakedWidget().hidden = false
	if e.ownsVisibleContextMenu() || draw() != 0 {
		t.Fatal("foreign popup highlighted field")
	}
}

func editMenuTest(t *testing.T) (*Edit, **Menu) {
	t.Helper()
	e := NewEdit()
	e.SetSize(240, 32)
	e.SetText("hello 中🙂")
	old, oldFocus := showEditContextPopup, focusWidget
	var m *Menu
	showEditContextPopup = func(menu *Menu, _, _ float64) { m = menu }
	t.Cleanup(func() { showEditContextPopup = old; focusWidget = oldFocus })
	return e, &m
}
func TestEditContextMenuOwnershipReuseAndRelease(t *testing.T) {
	e, shown := editMenuTest(t)
	e.SelectAll()
	e.OnRightUp(20, 15)
	if *shown != nil {
		t.Fatal("release without press")
	}
	e.OnRightDown(20, 15)
	e.OnRightUp(20, 15)
	m := *shown
	if m == nil || m.Parent() != e || len(m.Items()) != 9 || e.SelectionText() != e.Text() {
		t.Fatal("missing menu or selection lost")
	}
	for i := 0; i < 10; i++ {
		e.OnRightDown(20, 15)
		e.OnRightUp(20, 15)
		if *shown != m || len(m.Items()) != 9 {
			t.Fatal("popup/entries leaked")
		}
	}
	for _, it := range e.editMenuItems() {
		if !it.Separator && paint.LoadIcon(it.Icon) == nil {
			t.Fatalf("missing icon %s", it.Icon)
		}
	}
	old := m.Items()[0]
	m.OnHide()
	if len(m.Items()) != 0 || old.Parent() != nil {
		t.Fatal("commands retained on dismissal")
	}
	*shown = nil
	e.OnRightDown(-1, 15)
	e.OnRightUp(20, 15)
	if *shown != nil {
		t.Fatal("off-field gesture opened")
	}
	e.SetEnabled(false)
	e.OnRightDown(20, 15)
	e.OnRightUp(20, 15)
	if *shown != nil {
		t.Fatal("disabled opened")
	}
}
func TestEditContextAvailabilityAndStaleActions(t *testing.T) {
	e, _ := editMenuTest(t)
	e.SelectAll()
	e.SetReadOnly(true)
	for _, it := range e.editMenuItems() {
		if it.Label == "Copy" || it.Label == "Select All" {
			if !it.Enabled {
				t.Fatal("readonly cannot select/copy")
			}
		} else if it.Label != "" && it.Enabled {
			t.Fatal("readonly mutation enabled", it.Label)
		}
	}
	e.SetReadOnly(false)
	e.showEditContextMenu(0, 0)
	remove := e.contextMenu.Items()[6].(*Button)
	e.SetText("replacement")
	remove.Action().Trigger(nil)
	if e.Text() != "replacement" {
		t.Fatal("stale menu mutated replacement")
	}
	e.SelectAll()
	e.showEditContextMenu(0, 0)
	remove = e.contextMenu.Items()[6].(*Button)
	e.SetReadOnly(true)
	remove.Action().Trigger(nil)
	if e.Text() != "replacement" {
		t.Fatal("menu bypassed changed readonly")
	}
}
func TestEditContextDeleteUndoAndDismissBeforeAction(t *testing.T) {
	e, shown := editMenuTest(t)
	e.SelectAll()
	e.showEditContextMenu(0, 0)
	m := *shown
	m.NakedWidget().hidden = false
	m.setHighlight(6)
	m.OnKeyDown(KeyEnter, false)
	if e.Text() != "" || !e.CanUndo() || len(m.Items()) != 0 {
		t.Fatal("delete did not execute once after dismissal")
	}
	e.Undo()
	if e.Text() != "hello 中🙂" {
		t.Fatal("menu delete not undoable")
	}
}
func TestEditCutPreservesBufferAndClipboardFailures(t *testing.T) {
	e := NewEdit()
	e.SetText("A中🙂Z")
	e.SetSelection(1, 3)
	failure := errors.New("clipboard busy")
	err := e.copyEditSelection(true, func(string) error { return failure })
	if !errors.Is(err, failure) || e.Text() != "A中🙂Z" || e.CanUndo() {
		t.Fatal("failed cut deleted/recorded")
	}
	var copied string
	err = e.copyEditSelection(true, func(s string) error { copied = s; return nil })
	if err != nil || copied != "中🙂" || e.Text() != "AZ" || !e.CanUndo() {
		t.Fatal("successful cut not atomic")
	}
	e.Undo()
	if e.SelectionText() != "中🙂" {
		t.Fatal("cut undo lost selection")
	}
	e.copyEditSelection(true, func(string) error { e.SetText("new value"); return nil })
	if e.Text() != "new value" {
		t.Fatal("clipboard reentry cut changed target")
	}
}
func TestEditHistoryShortcutClaimsOnlyFieldHistory(t *testing.T) {
	e := NewEdit()
	e.OnTextInput("one")
	if !e.HandleShortcut('Z', ModAction) || e.Text() != "" || !e.HandleShortcut('Z', ModAction|ModShift) || e.Text() != "one" {
		t.Fatal("undo/redo routing")
	}
	for _, mods := range []uint8{0, ModAlt, ModAction | ModAlt} {
		if e.HandleShortcut('Z', mods) {
			t.Fatal("wrong modifier claimed")
		}
	}
	if e.HandleShortcut('S', ModAction) {
		t.Fatal("field stole Save")
	}
	e.SetReadOnly(true)
	if !e.HandleShortcut('Z', ModAction) || e.Text() != "one" {
		t.Fatal("readonly leaked Undo to host")
	}
}

func TestEditContextCustomBuilderAndHiddenAncestor(t *testing.T) {
	e, shown := editMenuTest(t)
	AttachContextMenu(e, func(m *Menu, _, _ float64) { m.AddButton1("Custom", nil) })
	t.Cleanup(func() { DetachContextMenu(e) })
	e.showEditContextMenu(0, 0)
	if *shown == nil || len((*shown).Items()) != 1 {
		t.Fatal("custom builder not used")
	}
	*shown = nil
	AttachContextMenu(e, func(*Menu, float64, float64) {})
	e.showEditContextMenu(0, 0)
	if *shown != nil {
		t.Fatal("empty menu shown")
	}
	DetachContextMenu(e)
	root := NewForm()
	e.SetParent(root)
	root.Hide()
	e.showEditContextMenu(0, 0)
	if *shown != nil {
		t.Fatal("hidden ancestor menu")
	}
}
