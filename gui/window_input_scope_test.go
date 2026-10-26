package gui

import "testing"

// These are logical surfaces, with no OS handles or native API calls.
func inputScopeWindow(widget IWidget, kind WindowType, owner *Window) *Window {
	win := &Window{widget: widget, wt: kind, popupOwner: owner}
	widget.NakedWidget().win, widget.NakedWidget().wt = win, kind
	return win
}

func TestInputScopeChecksWholeAncestorChain(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	root := NewLabel("root")
	win := inputScopeWindow(root, WtForm, nil)
	grandparent, parent := NewLabel("grandparent"), NewLabel("parent")
	grandparent.SetParent(root)
	parent.SetParent(grandparent)
	field := NewEdit()
	field.SetParent(parent)
	focusWidget = field
	if win.keyTarget() != field || win.textInputTarget() != field {
		t.Fatal("visible descendant lost focus")
	}
	for _, widget := range []IWidget{root, grandparent, parent, field} {
		widget.NakedWidget().hidden = true
		if win.keyTarget() != root || win.textInputTarget() != nil {
			t.Fatalf("hidden %T still accepts input", widget)
		}
		widget.NakedWidget().hidden = false
		widget.NakedWidget().disabled = true
		if win.keyTarget() != root || win.textInputTarget() != nil {
			t.Fatalf("disabled %T still accepts input", widget)
		}
		widget.NakedWidget().disabled = false
	}
	field.SetParent(nil)
	if win.textInputTarget() != nil {
		t.Fatal("detached focus accepted text")
	}
}

func TestInputScopeHasNoTextFallbackAndStopsAtNativeSurface(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	root := NewEdit()
	win := inputScopeWindow(root, WtForm, nil)
	focusWidget = nil
	if win.textInputTarget() != nil {
		t.Fatal("unfocused root accepted text")
	}
	focusWidget = root
	if win.textInputTarget() != root {
		t.Fatal("focused root lost text")
	}
	root.NakedWidget().hidden = true
	if win.textInputTarget() != nil {
		t.Fatal("hidden focused root accepted text")
	}
	// A child Form is independent even when its logical parent is hidden.
	child := NewEdit()
	child.SetParent(root)
	childWin := inputScopeWindow(child, WtForm, nil)
	focusWidget = child
	if childWin.textInputTarget() != child || win.textInputTarget() != nil {
		t.Fatal("independent Form inherited parent scope")
	}
	child.NakedWidget().disabled = true
	if childWin.textInputTarget() != nil {
		t.Fatal("disabled root accepted text")
	}
	child.NakedWidget().disabled = false
	childWin.widget = nil
	if childWin.textInputTarget() != nil {
		t.Fatal("tearing-down surface accepted text")
	}
}

func TestInputScopeOwnedAndForeignPopupChains(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	root := NewLabel("root")
	win := inputScopeWindow(root, WtForm, nil)
	other := inputScopeWindow(NewLabel("other"), WtForm, nil)
	popup := NewEdit()
	popup.SetParent(root)
	popupWin := inputScopeWindow(popup, WtPopup, win)
	nested := NewEdit()
	nested.SetParent(popup)
	nestedWin := inputScopeWindow(nested, WtPopup, popupWin)
	focusWidget = nested
	for _, w := range []*Window{win, popupWin, nestedWin} {
		if w.textInputTarget() != nested {
			t.Fatal("owned popup lost text")
		}
	}
	if other.textInputTarget() != nil {
		t.Fatal("foreign surface accepted popup input")
	}
	popupWin.popupOwner = other
	if win.textInputTarget() != nil || other.textInputTarget() != nested {
		t.Fatal("reparented popup retained old input owner")
	}
}

func TestInputScopeLookupDoesNotAllocate(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	root := NewLabel("root")
	win := inputScopeWindow(root, WtForm, nil)
	var parent IWidget = root
	for i := 0; i < 24; i++ {
		next := NewLabel("parent")
		next.SetParent(parent)
		parent = next
	}
	field := NewEdit()
	field.SetParent(parent)
	focusWidget = field
	if n := testing.AllocsPerRun(1000, func() {
		if win.textInputTarget() != field {
			panic("wrong target")
		}
	}); n != 0 {
		t.Fatalf("allocations: %v", n)
	}
}
