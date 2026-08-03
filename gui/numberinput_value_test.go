package gui

import (
	"strconv"
	"testing"

	"github.com/uk0/silk/core"
)

// TestNumberInputFactoryDefaults: a NumberInput built through the registered
// factory — the path the designer palette and the .silkui form loader take —
// must come up with the same usable range as NewNumberInput. When the defaults
// live only in the constructor the factory hands back max=0/step=0, so every
// SetValue clamps to 0 and no click or key can move the field.
func TestNumberInputFactoryDefaults(t *testing.T) {
	ni, ok := core.New("gui.NumberInput").(*NumberInput)
	if !ok {
		t.Fatal("gui.NumberInput factory did not produce a *NumberInput")
	}
	if ni.Min() != 0 || ni.Max() != 100 {
		t.Errorf("factory range = [%v, %v], want [0, 100]", ni.Min(), ni.Max())
	}
	if ni.Step() != 1 {
		t.Errorf("factory Step() = %v, want 1", ni.Step())
	}
	if ni.Decimals() != 2 {
		t.Errorf("factory Decimals() = %v, want 2", ni.Decimals())
	}
	ni.SetValue(42)
	if ni.Value() != 42 {
		t.Errorf("SetValue(42) on a factory-built input = %v, want 42", ni.Value())
	}
}

// TestNumberInputFractionalStepDoesNotDrift: ten 0.1 steps from 0 must land
// exactly on 1. Adding the step to the raw float accumulates binary error and
// stops at 0.9999999999999999, so the maximum is never reached.
func TestNumberInputFractionalStepDoesNotDrift(t *testing.T) {
	ni := NewNumberInput()
	ni.SetRange(0, 1)
	ni.SetStep(0.1)
	ni.SetValue(0)
	for i := 0; i < 10; i++ {
		ni.StepUp()
	}
	if ni.Value() != 1 {
		t.Errorf("10 x StepUp(0.1) = %.17g, want exactly 1", ni.Value())
	}
	for i := 0; i < 10; i++ {
		ni.StepDown()
	}
	if ni.Value() != 0 {
		t.Errorf("10 x StepDown(0.1) = %.17g, want exactly 0", ni.Value())
	}
}

// TestNumberInputValueMatchesDisplayedText: the value must agree with the text
// the widget paints — parsing the displayed string back has to return exactly
// Value() at every step of a fractional walk.
func TestNumberInputValueMatchesDisplayedText(t *testing.T) {
	ni := NewNumberInput()
	ni.SetRange(0, 10)
	ni.SetStep(0.1)
	ni.SetValue(0)
	for i := 0; i < 40; i++ {
		ni.StepUp()
		shown := ni.formatValue()
		got, err := strconv.ParseFloat(shown, 64)
		if err != nil {
			t.Fatalf("step %d: displayed %q does not parse: %v", i, shown, err)
		}
		if got != ni.Value() {
			t.Fatalf("step %d: displayed %q parses to %.17g but Value() = %.17g",
				i, shown, got, ni.Value())
		}
	}
}

// TestNumberInputTypedValueQuantized: a typed value carrying more precision
// than the field displays is committed on the displayed grid, so Value() and
// the text on screen cannot disagree.
func TestNumberInputTypedValueQuantized(t *testing.T) {
	ni := NewNumberInput()
	ni.SetRange(0, 10)
	ni.SetDecimals(2)
	ni.editing = true
	ni.editText = "1.129"
	ni.OnKeyDown(KeyEnter, false)
	if ni.Value() != 1.13 {
		t.Errorf("typed 1.129 with 2 decimals: Value() = %.17g, want 1.13", ni.Value())
	}
}

// TestNumberInputPageAndHomeEndKeys: a bounded numeric field answers the page
// keys and Home/End everywhere else (see SpinBox.OnKeyDown, Slider.OnKeyDown);
// only Up/Down were wired here, so the other four keys did nothing.
func TestNumberInputPageAndHomeEndKeys(t *testing.T) {
	ni := NewNumberInput()
	ni.SetRange(0, 100)
	ni.SetStep(2)
	ni.SetValue(50)

	ni.OnKeyDown(KeyPageUp, false)
	if ni.Value() != 70 {
		t.Errorf("PageUp with step 2: Value() = %v, want 70", ni.Value())
	}
	ni.OnKeyDown(KeyPageDown, false)
	if ni.Value() != 50 {
		t.Errorf("PageDown with step 2: Value() = %v, want 50", ni.Value())
	}
	ni.OnKeyDown(KeyHome, false)
	if ni.Value() != 0 {
		t.Errorf("Home: Value() = %v, want the minimum 0", ni.Value())
	}
	ni.OnKeyDown(KeyEnd, false)
	if ni.Value() != 100 {
		t.Errorf("End: Value() = %v, want the maximum 100", ni.Value())
	}
}

// TestNumberInputDisabledIgnoresInput: a disabled field must ignore the step
// buttons, the arrow keys and typed characters, not merely paint differently.
// Widget-level enablement is not enforced by the event dispatcher — every
// widget checks it itself (see Slider.OnLeftDown, SpinBox.OnKeyDown).
func TestNumberInputDisabledIgnoresInput(t *testing.T) {
	ni := NewNumberInput()
	ni.SetSize(120, 24)
	ni.SetRange(0, 100)
	ni.SetValue(10)
	ni.SetEnabled(false)

	ni.OnKeyDown(KeyUp, false)
	if ni.Value() != 10 {
		t.Errorf("disabled KeyUp changed value to %v, want 10", ni.Value())
	}
	// Right-hand 20px strip, upper half: the step-up button.
	ni.OnLeftDown(115, 5)
	if ni.Value() != 10 {
		t.Errorf("disabled step-up click changed value to %v, want 10", ni.Value())
	}
	// Text region: must not open an edit session.
	ni.OnLeftDown(10, 12)
	if ni.editing {
		t.Error("disabled click in the text region started an edit session")
	}
	ni.editing = true // even if an edit was open when the field was disabled
	ni.editText = ""
	ni.OnTextInput("7")
	if ni.editText != "" {
		t.Errorf("disabled OnTextInput appended %q, want no change", ni.editText)
	}
}
