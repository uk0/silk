package gui

import (
	"errors"
	"strings"
	"testing"
)

// Tests for the production-facing validation surface: the RequiredValidator
// / ValidatorFunc / LengthValidator built-ins, the Edit error-state
// integration (IsValid / ValidationError), and the AllValid form gate.
// Everything is driven through the text-set path (SetText / OnTextInput) so
// no GL Draw is exercised.

// --- RequiredValidator -----------------------------------------------

func TestRequiredValidator(t *testing.T) {
	v := &RequiredValidator{}
	if got := v.Validate(""); got == Acceptable {
		t.Errorf("empty should not be Acceptable, got %v", got)
	}
	if got := v.Validate("   "); got == Acceptable {
		t.Errorf("whitespace-only should not be Acceptable, got %v", got)
	}
	if got := v.Validate("x"); got != Acceptable {
		t.Errorf("non-empty should be Acceptable, got %v", got)
	}
	// Empty must stay typable/clearable, so it is Intermediate, never Invalid.
	if got := v.Validate(""); got != Intermediate {
		t.Errorf("empty should be Intermediate (not Invalid), got %v", got)
	}
	// ErrorMessage: non-empty for empty input, empty once filled.
	if msg := v.ErrorMessage(""); msg == "" {
		t.Errorf("ErrorMessage(empty) should be non-empty")
	}
	if msg := v.ErrorMessage("x"); msg != "" {
		t.Errorf("ErrorMessage(non-empty) should be empty, got %q", msg)
	}
	// Custom message override.
	cv := &RequiredValidator{Message: "name required"}
	if msg := cv.ErrorMessage(""); msg != "name required" {
		t.Errorf("custom message = %q, want %q", msg, "name required")
	}
}

// --- ValidatorFunc ---------------------------------------------------

func TestValidatorFuncAdapter(t *testing.T) {
	calls := 0
	v := ValidatorFunc(func(s string) error {
		calls++
		if !strings.Contains(s, "@") {
			return errors.New("must contain @")
		}
		return nil
	})
	if got := v.Validate("a@b"); got != Acceptable {
		t.Errorf("passing func should be Acceptable, got %v", got)
	}
	// Failing func is Intermediate (not Invalid) so keystrokes aren't dropped.
	if got := v.Validate("nope"); got != Intermediate {
		t.Errorf("failing func should be Intermediate, got %v", got)
	}
	if msg := v.ErrorMessage("nope"); msg != "must contain @" {
		t.Errorf("ErrorMessage should surface the error text, got %q", msg)
	}
	if msg := v.ErrorMessage("a@b"); msg != "" {
		t.Errorf("ErrorMessage on pass should be empty, got %q", msg)
	}
	if calls == 0 {
		t.Errorf("ValidatorFunc never ran the wrapped func")
	}
}

// --- LengthValidator -------------------------------------------------

func TestLengthValidator(t *testing.T) {
	v := &LengthValidator{Min: 2, Max: 5}
	cases := []struct {
		in   string
		want State
	}{
		{"", Intermediate},    // below Min, keep typing
		{"a", Intermediate},   // below Min
		{"ab", Acceptable},    // == Min
		{"abcde", Acceptable}, // == Max
		{"abcdef", Invalid},   // above Max, hard-drop
	}
	for _, c := range cases {
		if got := v.Validate(c.in); got != c.want {
			t.Errorf("LengthValidator{2,5}.Validate(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if msg := v.ErrorMessage("a"); msg == "" {
		t.Errorf("ErrorMessage(too short) should be non-empty")
	}
	if msg := v.ErrorMessage("abcdef"); msg == "" {
		t.Errorf("ErrorMessage(too long) should be non-empty")
	}
	if msg := v.ErrorMessage("abc"); msg != "" {
		t.Errorf("ErrorMessage(in range) should be empty, got %q", msg)
	}
	// Max <= 0 means unbounded.
	un := &LengthValidator{Min: 1, Max: 0}
	if got := un.Validate(strings.Repeat("x", 100)); got != Acceptable {
		t.Errorf("Max<=0 should be unbounded, got %v", got)
	}
}

// --- Built-in classify-only validators (regex / numeric) -------------
//
// The exhaustive tables live in validator_test.go; these are focused
// valid+invalid spot-checks proving the same types back the Edit surface.

func TestBuiltinValidatorStates(t *testing.T) {
	iv := NewIntValidator(0, 100)
	if iv.Validate("42") != Acceptable {
		t.Errorf("int 42 should be Acceptable")
	}
	if iv.Validate("abc") != Invalid {
		t.Errorf("int abc should be Invalid")
	}

	dv := NewDoubleValidator(0, 1, 2)
	if dv.Validate("0.5") != Acceptable {
		t.Errorf("double 0.5 should be Acceptable")
	}
	if dv.Validate("xx") != Invalid {
		t.Errorf("double xx should be Invalid")
	}

	rv, err := NewRegExpValidator(`\d{3}-\d{4}`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if rv.Validate("555-1234") != Acceptable {
		t.Errorf("regex full match should be Acceptable")
	}
	if rv.Validate("abc") != Invalid {
		t.Errorf("regex non-match should be Invalid")
	}
}

// --- Edit integration ------------------------------------------------

func TestEditRequiredValidationState(t *testing.T) {
	e := NewEdit()
	e.SetValidator(&RequiredValidator{})

	// Empty required field: invalid with a non-empty message.
	e.SetText("")
	if e.IsValid() {
		t.Errorf("empty required field: IsValid() should be false")
	}
	if e.ValidationError() == "" {
		t.Errorf("empty required field: ValidationError() should be non-empty")
	}

	// Filled: valid, message cleared.
	e.SetText("hello")
	if !e.IsValid() {
		t.Errorf("filled required field: IsValid() should be true")
	}
	if e.ValidationError() != "" {
		t.Errorf("filled required field: ValidationError() should be empty, got %q", e.ValidationError())
	}
}

func TestEditValidationViaTyping(t *testing.T) {
	// Drive validity through the keystroke path (OnTextInput / Backspace),
	// not just SetText.
	e := NewEdit()
	e.SetValidator(&RequiredValidator{})
	e.SetText("")
	if e.IsValid() {
		t.Fatalf("empty should be invalid before typing")
	}
	e.OnTextInput("a")
	if !e.IsValid() {
		t.Errorf("after typing 'a': IsValid() should be true")
	}
	if e.Text() != "a" {
		t.Fatalf("typing path text = %q, want a", e.Text())
	}
}

func TestEditValidationErrorMessageFallback(t *testing.T) {
	// IntValidator has no ErrorMessager; an Intermediate value should yield
	// the generic fallback message, not an empty string.
	e := NewEdit()
	e.SetValidator(NewIntValidator(10, 20))
	e.SetText("-") // Intermediate
	if e.IsValid() {
		t.Errorf("'-' is Intermediate, IsValid() should be false")
	}
	if e.ValidationError() != "invalid input" {
		t.Errorf("fallback message = %q, want %q", e.ValidationError(), "invalid input")
	}
}

func TestEditNoValidatorAlwaysValid(t *testing.T) {
	e := NewEdit()
	if !e.IsValid() {
		t.Errorf("fresh Edit with no validator: IsValid() should be true")
	}
	if e.ValidationError() != "" {
		t.Errorf("no validator: ValidationError() should be empty, got %q", e.ValidationError())
	}
	e.SetText("anything")
	if !e.IsValid() {
		t.Errorf("no validator after SetText: IsValid() should stay true")
	}
}

func TestEditClearValidatorResetsState(t *testing.T) {
	e := NewEdit()
	e.SetValidator(&RequiredValidator{})
	e.SetText("")
	if e.IsValid() {
		t.Fatalf("precondition: empty required field should be invalid")
	}
	// Clearing the validator makes the field valid again.
	e.SetValidator(nil)
	if !e.IsValid() {
		t.Errorf("after clearing validator: IsValid() should be true")
	}
	if e.ValidationError() != "" {
		t.Errorf("after clearing validator: ValidationError() should be empty, got %q", e.ValidationError())
	}
}

// --- Form gate -------------------------------------------------------

func TestAllValidFormGate(t *testing.T) {
	name := NewEdit()
	name.SetValidator(&RequiredValidator{})
	age := NewEdit()
	age.SetValidator(NewIntValidator(0, 120))

	// One invalid (empty required name) → whole form invalid.
	name.SetText("")
	age.SetText("30")
	if AllValid(name, age) {
		t.Errorf("form with an empty required field should not be AllValid")
	}

	// Both valid → form valid.
	name.SetText("Ada")
	age.SetText("30")
	if !AllValid(name, age) {
		t.Errorf("form with both fields valid should be AllValid")
	}

	// Nil entries are skipped; zero edits is vacuously true.
	if !AllValid() {
		t.Errorf("AllValid() with no edits should be true")
	}
	if !AllValid(nil, name, nil) {
		t.Errorf("AllValid should skip nil entries")
	}
}
