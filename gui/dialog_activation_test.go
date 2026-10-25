package gui

import (
	"fmt"
	"testing"
)

func TestDialogActivationFocusMatchesDefaultAndTabOrder(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	owner := NewEdit()
	owner.SetFocus()
	d := NewDialog("Close editors", owner)
	d.SetContent(NewLabel("Save before closing?"))
	save := d.AddButton("Save All", DialogOK)
	discard := d.AddButton("Discard All", DialogNo)
	cancel := d.AddButton("Cancel", DialogCancel)
	if focusWidget != owner {
		t.Fatal("constructing a dialog stole focus")
	}
	d.activateInitialFocus()
	if focusWidget != save {
		t.Fatalf("highlighted default is not initial focus: %T", focusWidget)
	}
	if nextFocusable(d, focusWidget, true) != discard || nextFocusable(d, focusWidget, false) != cancel {
		t.Fatal("Tab and Shift+Tab disagree with the visible button order")
	}
	// A reopen must use the new default, not keep a focus target from last time.
	owner.SetFocus()
	d.SetDefaultButton(cancel)
	d.activateInitialFocus()
	if focusWidget != cancel {
		t.Fatal("reopened dialog ignored explicit default")
	}
}

func TestDialogActivationPrefersContentAndExplicitFocus(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	d := NewDialog("Input", nil)
	d.AddButton("OK", DialogOK)
	content := NewVBox()
	disabled := NewVBox()
	disabled.AddWidget(NewEdit())
	disabled.SetEnabled(false)
	content.AddWidget(disabled)
	first, second := NewEdit(), NewEdit()
	content.AddWidget(first)
	content.AddWidget(second)
	d.SetContent(content)
	d.activateInitialFocus()
	if focusWidget != first {
		t.Fatalf("dialog did not choose first usable field: %T", focusWidget)
	}
	d.SetInitialFocus(second)
	d.activateInitialFocus()
	if focusWidget != second {
		t.Fatal("automatic focus overrode an explicit target")
	}
}

func TestDialogDefaultRejectsUnusableExplicitButtons(t *testing.T) {
	for _, state := range []string{"disabled", "hidden", "detached", "foreign", "disabled-bar", "hidden-dialog"} {
		t.Run(state, func(t *testing.T) {
			d := NewDialog("safe", nil)
			button := d.AddButton("Save", DialogOK)
			d.SetDefaultButton(button)
			switch state {
			case "disabled":
				button.SetEnabled(false)
			case "hidden":
				button.SetVisible(false)
			case "detached":
				button.SetParent(nil)
			case "foreign":
				button.SetEnabled(false)
				other := NewDialog("foreign", nil)
				d.SetDefaultButton(other.AddButton("Delete", DialogOK))
			case "disabled-bar":
				d.btnBox.SetEnabled(false)
			case "hidden-dialog":
				d.SetVisible(false)
			}
			if d.resolveDefault() || d.closed {
				t.Fatal("Enter activated an unusable or foreign default")
			}
		})
	}
}

func TestFocusTraversalSkipsDisabledSubtrees(t *testing.T) {
	root := NewVBox()
	group := NewVBox()
	group.AddWidget(NewEdit())
	group.SetEnabled(false)
	root.AddWidget(group)
	allowed := NewEdit()
	root.AddWidget(allowed)
	if firstFocusable(root) != allowed || nextFocusable(root, nil, true) != allowed || nextFocusable(root, allowed, true) != allowed {
		t.Fatal("Tab entered a disabled container")
	}
}

func BenchmarkDialogActivation(b *testing.B) {
	for _, fields := range []int{0, 1000} {
		b.Run(fmt.Sprint(fields), func(b *testing.B) {
			old := focusWidget
			b.Cleanup(func() { focusWidget = old })
			d := NewDialog("Activation", nil)
			d.AddButton("Save", DialogOK)
			d.AddButton("Discard", DialogNo)
			d.AddButton("Cancel", DialogCancel)
			content := NewVBox()
			for range fields {
				content.AddWidget(NewEdit())
			}
			d.SetContent(content)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				focusWidget = nil
				d.activateInitialFocus()
			}
		})
	}
}

func TestDialogActivationWithoutFocusableChildren(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	owner := NewEdit()
	owner.SetFocus()
	d := NewDialog("Information", owner)
	d.SetContent(NewLabel("No interactive children"))
	d.activateInitialFocus()
	if focusWidget != owner {
		t.Fatal("empty dialog captured a foreign focus target")
	}
}

func TestMessageBoxTabCycleHasNoInvisibleFormStop(t *testing.T) {
	box := NewMessageBox()
	box.SetButtons([]string{"Save", "Discard", "Cancel"})
	if nextFocusable(box, box.edit, true) != box.btnBox.btnMap["Save"] ||
		nextFocusable(box, box.btnBox.btnMap["Save"], false) != box.edit {
		t.Fatal("message text/button traversal stopped on the form itself")
	}
}

func TestDialogHiddenConstructionKeepsDefaultMetadataNotActivation(t *testing.T) {
	d := NewDialog("Print", nil)
	d.SetVisible(false)
	b := d.AddButton("Print", DialogOK)
	if d.defaultButton() != b || !b.IsDefault() || d.resolveDefault() || d.closed {
		t.Fatal("hidden construction lost default metadata or activated a command")
	}
	d.SetVisible(true)
	if !d.resolveDefault() || !d.closed {
		t.Fatal("showing the dialog did not enable its valid default")
	}
}
