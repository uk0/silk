package gui

import "testing"

func TestComboPopupShowOwnsLogicalKeyboardFocus(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	c := newTestCombo("Dark", "Light")
	c.SetEditable(false)
	c.SetActiveIndex(0)
	c.SetFocus()
	c.sub.SetVisible(true)
	if focusWidget != c.sub {
		t.Fatalf("shown popup left focus on %T; owner dialog would consume Escape", focusWidget)
	}
}
