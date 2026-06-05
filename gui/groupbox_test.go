package gui

import (
	"testing"
)

// titleCheckClickPoint returns a point inside the title check box indicator,
// so a test click is guaranteed to land on the toggle region.
func titleCheckClickPoint(gb *GroupBox) (x, y float64) {
	cx, cy, s := gb.titleCheckRect()
	return cx + s*0.5, cy + s*0.5
}

// TestGroupBoxDefaultNotCheckable: a freshly created GroupBox is not checkable,
// matching the previous behaviour where the box was a plain titled frame.
func TestGroupBoxDefaultNotCheckable(t *testing.T) {
	gb := NewGroupBox("Options")
	if gb.IsCheckable() {
		t.Fatalf("new GroupBox IsCheckable = true, want false")
	}
}

// TestGroupBoxSetCheckableDefaultsChecked: enabling checkable mode leaves the
// group checked (enabled) by default, matching Qt's QGroupBox.
func TestGroupBoxSetCheckableDefaultsChecked(t *testing.T) {
	gb := NewGroupBox("Options")
	gb.SetCheckable(true)
	if !gb.IsCheckable() {
		t.Fatalf("after SetCheckable(true): IsCheckable = false, want true")
	}
	if !gb.IsChecked() {
		t.Fatalf("after SetCheckable(true): IsChecked = false, want true (Qt default)")
	}
}

// TestGroupBoxSetCheckedTogglesAndFires: SetChecked changes the state and fires
// SigToggled with the new value; a redundant set neither changes nor re-fires.
func TestGroupBoxSetCheckedTogglesAndFires(t *testing.T) {
	gb := NewGroupBox("Options")
	gb.SetCheckable(true)

	var got []bool
	gb.SigToggled(func(b bool) { got = append(got, b) })

	gb.SetChecked(false)
	if gb.IsChecked() {
		t.Fatalf("after SetChecked(false): IsChecked = true, want false")
	}
	// Redundant set: no change, no callback.
	gb.SetChecked(false)

	gb.SetChecked(true)
	if !gb.IsChecked() {
		t.Fatalf("after SetChecked(true): IsChecked = false, want true")
	}

	want := []bool{false, true}
	if len(got) != len(want) {
		t.Fatalf("SigToggled fired %d times, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SigToggled[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestGroupBoxLeftDownTogglesWhenCheckable: a press on the title check box
// region toggles the checked state and fires SigToggled.
func TestGroupBoxLeftDownTogglesWhenCheckable(t *testing.T) {
	gb := NewGroupBox("Options")
	gb.SetSize(200, 120)
	gb.SetCheckable(true)

	fired := 0
	gb.SigToggled(func(bool) { fired++ })

	x, y := titleCheckClickPoint(gb)
	gb.OnLeftDown(x, y)
	if gb.IsChecked() {
		t.Fatalf("after click: IsChecked = true, want false")
	}
	gb.OnLeftDown(x, y)
	if !gb.IsChecked() {
		t.Fatalf("after second click: IsChecked = false, want true")
	}
	if fired != 2 {
		t.Fatalf("SigToggled fired %d times, want 2", fired)
	}
}

// TestGroupBoxLeftDownNoOpWhenNotCheckable: when not checkable, a press on the
// title row does nothing (no state, no callback) - the plain-frame behaviour.
func TestGroupBoxLeftDownNoOpWhenNotCheckable(t *testing.T) {
	gb := NewGroupBox("Options")
	gb.SetSize(200, 120)

	fired := false
	gb.SigToggled(func(bool) { fired = true })

	// Point that would hit the indicator were it checkable.
	x, y := titleCheckClickPoint(gb)
	gb.OnLeftDown(x, y)

	if fired {
		t.Fatalf("non-checkable GroupBox fired SigToggled on click, want no fire")
	}
	if !gb.IsChecked() {
		t.Fatalf("non-checkable GroupBox: IsChecked changed to false, want unchanged (true)")
	}
}

// TestGroupBoxLeftDownOutsideTitleIgnored: a press below the title row is
// ignored even when checkable, so clicks in the content area do not toggle.
func TestGroupBoxLeftDownOutsideTitleIgnored(t *testing.T) {
	gb := NewGroupBox("Options")
	gb.SetSize(200, 120)
	gb.SetCheckable(true)

	fired := false
	gb.SigToggled(func(bool) { fired = true })

	// Well below the title band.
	gb.OnLeftDown(20, gb.titleHeight()+40)
	if fired || !gb.IsChecked() {
		t.Fatalf("click below title toggled the group, want unchanged")
	}
}

// TestGroupBoxToggleEnablesContent: because GroupBox owns its content widget,
// toggling the checkable group propagates the enabled state to that child.
func TestGroupBoxToggleEnablesContent(t *testing.T) {
	gb := NewGroupBox("Options")
	child := NewLabel("inside")
	gb.SetContent(child)
	gb.SetCheckable(true)

	// Checked by default -> child enabled.
	if !child.IsEnabled() {
		t.Fatalf("checkable group checked: child IsEnabled = false, want true")
	}

	gb.SetChecked(false)
	if child.IsEnabled() {
		t.Fatalf("after uncheck: child IsEnabled = true, want false")
	}

	gb.SetChecked(true)
	if !child.IsEnabled() {
		t.Fatalf("after re-check: child IsEnabled = false, want true")
	}
}

// TestGroupBoxUncheckedThenSetCheckableAppliesToContent: a group switched into
// checkable mode while unchecked disables its content immediately, and leaving
// checkable mode re-enables it.
func TestGroupBoxUncheckedThenSetCheckableAppliesToContent(t *testing.T) {
	gb := NewGroupBox("Options")
	child := NewLabel("inside")
	gb.SetContent(child)

	gb.SetCheckable(true)
	gb.SetChecked(false)
	if child.IsEnabled() {
		t.Fatalf("unchecked checkable group: child IsEnabled = true, want false")
	}

	gb.SetCheckable(false)
	if !child.IsEnabled() {
		t.Fatalf("leaving checkable mode: child IsEnabled = false, want true (re-enabled)")
	}
}
