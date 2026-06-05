package gui

import "testing"

// These tests drive the Dialog Enter/Esc behavior headlessly. A real modal
// loop (Window.ShowModal) needs a GLFW window and an event pump, so we cannot
// run ShowModal in a unit test. Instead we exercise the internal resolve
// methods (resolveDefault / resolveCancel) that OnKeyDown delegates to, plus
// OnKeyDown itself. These take the exact same path a button click takes
// (onBtnClick), which records the result in dlg.Result() and flips dlg.closed,
// so asserting on Result() verifies the same outcome a click would produce.
// Without an attached window onBtnClick simply skips the EndModal call, which
// is fine — the result bookkeeping is what we assert.

func newButtonDialog() (*Dialog, *Button, *Button) {
	dlg := NewDialog("t", nil)
	ok := dlg.AddButton("OK", DialogOK)
	cancel := dlg.AddButton("Cancel", DialogCancel)
	return dlg, ok, cancel
}

// Enter activates the explicitly-set default button (OK).
func TestDialogEnterTriggersDefaultButton(t *testing.T) {
	dlg, ok, _ := newButtonDialog()
	dlg.SetDefaultButton(ok)

	dlg.OnKeyDown(KeyEnter, false)

	if dlg.Result() != DialogOK {
		t.Fatalf("Enter: want DialogOK, got %v", dlg.Result())
	}
	if !dlg.closed {
		t.Fatalf("Enter: dialog should be marked closed")
	}
}

// Esc rejects with the cancel result regardless of the default button.
func TestDialogEscTriggersCancel(t *testing.T) {
	dlg, ok, _ := newButtonDialog()
	dlg.SetDefaultButton(ok)

	dlg.OnKeyDown(KeyEsc, false)

	if dlg.Result() != DialogCancel {
		t.Fatalf("Esc: want DialogCancel, got %v", dlg.Result())
	}
	if !dlg.closed {
		t.Fatalf("Esc: dialog should be marked closed")
	}
}

// resolveDefault drives the Enter path directly and reports success.
func TestDialogResolveDefaultDirect(t *testing.T) {
	dlg, ok, _ := newButtonDialog()
	dlg.SetDefaultButton(ok)

	if !dlg.resolveDefault() {
		t.Fatalf("resolveDefault: want true with a default button")
	}
	if dlg.Result() != DialogOK {
		t.Fatalf("resolveDefault: want DialogOK, got %v", dlg.Result())
	}
}

// resolveCancel drives the Esc path directly.
func TestDialogResolveCancelDirect(t *testing.T) {
	dlg, _, _ := newButtonDialog()

	dlg.resolveCancel()

	if dlg.Result() != DialogCancel {
		t.Fatalf("resolveCancel: want DialogCancel, got %v", dlg.Result())
	}
}

// With no explicit default, the first affirmative button (OK) is chosen even
// when it was not added first — Qt's "first AcceptRole" heuristic.
func TestDialogImplicitDefaultPrefersAffirmative(t *testing.T) {
	dlg := NewDialog("t", nil)
	dlg.AddButton("No", DialogNo)
	dlg.AddButton("Yes", DialogYes)
	dlg.AddButton("OK", DialogOK)

	if got := dlg.defaultButton(); got == nil || dlg.resultMap[btnKey(got)] != DialogOK {
		t.Fatalf("implicit default: want the OK button, got %v", got)
	}

	dlg.OnKeyDown(KeyEnter, false)
	if dlg.Result() != DialogOK {
		t.Fatalf("implicit default Enter: want DialogOK, got %v", dlg.Result())
	}
}

// When no affirmative button exists, the implicit default falls back to the
// last-added button (Qt makes the trailing button default).
func TestDialogImplicitDefaultFallsBackToLast(t *testing.T) {
	dlg := NewDialog("t", nil)
	dlg.AddButton("No", DialogNo)
	last := dlg.AddButton("Maybe", DialogCancel)

	if got := dlg.defaultButton(); got != last {
		t.Fatalf("implicit default fallback: want last-added button, got %v", got)
	}
}

// A disabled default button is skipped by the implicit heuristic.
func TestDialogImplicitDefaultSkipsDisabled(t *testing.T) {
	dlg := NewDialog("t", nil)
	ok := dlg.AddButton("OK", DialogOK)
	ok.SetEnabled(false)
	yes := dlg.AddButton("Yes", DialogYes)

	if got := dlg.defaultButton(); got != yes {
		t.Fatalf("disabled default: want the Yes button, got %v", got)
	}
}

// SetCancelResult overrides what Esc returns.
func TestDialogSetCancelResult(t *testing.T) {
	dlg := NewDialog("t", nil)
	dlg.AddButton("No", DialogNo)
	dlg.SetCancelResult(DialogNo)

	dlg.OnKeyDown(KeyEsc, false)

	if dlg.Result() != DialogNo {
		t.Fatalf("custom cancel: want DialogNo, got %v", dlg.Result())
	}
}

// Esc yields the cancel result even when no Cancel button was added (onBtnClick
// falls back to DialogCancel for an unknown key).
func TestDialogEscWithoutCancelButton(t *testing.T) {
	dlg := NewDialog("t", nil)
	dlg.AddButton("OK", DialogOK)

	dlg.OnKeyDown(KeyEsc, false)

	if dlg.Result() != DialogCancel {
		t.Fatalf("Esc w/o cancel button: want DialogCancel, got %v", dlg.Result())
	}
}

// A dialog with no buttons: Enter is a no-op (no default), Esc still cancels.
func TestDialogNoButtons(t *testing.T) {
	dlg := NewDialog("t", nil)

	if dlg.resolveDefault() {
		t.Fatalf("resolveDefault: want false with no buttons")
	}
	dlg.OnKeyDown(KeyEnter, false) // must not panic
	if dlg.closed {
		t.Fatalf("Enter with no default should not close the dialog")
	}

	dlg.OnKeyDown(KeyEsc, false)
	if dlg.Result() != DialogCancel || !dlg.closed {
		t.Fatalf("Esc with no buttons: want DialogCancel+closed, got %v closed=%v", dlg.Result(), dlg.closed)
	}
}
