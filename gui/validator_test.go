package gui

import "testing"

// --- IntValidator -----------------------------------------------------

func TestIntValidatorAccepts(t *testing.T) {
	v := NewIntValidator(0, 100)
	cases := []struct {
		in   string
		want State
	}{
		{"", Intermediate},
		{"-", Intermediate},
		{"+", Intermediate},
		{"0", Acceptable},
		{"50", Acceptable},
		{"100", Acceptable},
		{"abc", Invalid},
		{"-1", Intermediate}, // -1 below range but |1| < 100, could keep typing
		{"500", Invalid},     // 500 > 100 and |500| > 100, can't get back in range
		{"1000000", Invalid}, // way out of range
	}
	for _, c := range cases {
		if got := v.Validate(c.in); got != c.want {
			t.Errorf("IntValidator(%d,%d).Validate(%q) = %v, want %v",
				v.Bottom, v.Top, c.in, got, c.want)
		}
	}
}

func TestIntValidatorNegativeRange(t *testing.T) {
	v := NewIntValidator(-50, -10)
	cases := []struct {
		in   string
		want State
	}{
		{"-30", Acceptable},
		{"-10", Acceptable},
		{"-50", Acceptable},
		{"-1", Intermediate}, // could become -10..-50
		{"0", Intermediate},  // 0 is above range; |0| < 50, could keep typing
		{"50", Invalid},      // positive 50 outside negative range; can't recover
		{"-100", Invalid},    // beyond -50
	}
	for _, c := range cases {
		if got := v.Validate(c.in); got != c.want {
			t.Errorf("IntValidator(-50,-10).Validate(%q) = %v, want %v",
				c.in, got, c.want)
		}
	}
}

func TestIntValidatorFixupClamps(t *testing.T) {
	v := NewIntValidator(0, 100)
	cases := []struct {
		in, want string
	}{
		{"50", "50"},
		{"-5", "0"},
		{"500", "100"},
		{"  42  ", "42"}, // whitespace trimmed
		{"abc", "abc"},   // unparseable left alone
		{"", ""},
	}
	for _, c := range cases {
		if got := v.Fixup(c.in); got != c.want {
			t.Errorf("Fixup(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- DoubleValidator -------------------------------------------------

func TestDoubleValidatorAccepts(t *testing.T) {
	v := NewDoubleValidator(0, 100, 2)
	cases := []struct {
		in   string
		want State
	}{
		{"", Intermediate},
		{".", Intermediate},
		{"-", Intermediate},
		{"3.", Intermediate}, // trailing dot, mid-typing
		{"3.14", Acceptable},
		{"50", Acceptable},
		{"100", Acceptable},
		{"3.141", Invalid}, // exceeds Decimals=2
		{"abc", Invalid},
		{"-5", Intermediate}, // out of range but Intermediate (lenient)
	}
	for _, c := range cases {
		if got := v.Validate(c.in); got != c.want {
			t.Errorf("DoubleValidator(0,100,2).Validate(%q) = %v, want %v",
				c.in, got, c.want)
		}
	}
}

func TestDoubleValidatorUnlimitedDecimals(t *testing.T) {
	v := NewDoubleValidator(0, 1, -1)
	if got := v.Validate("0.1234567890123"); got != Acceptable {
		t.Errorf("unlimited decimals should accept long fraction, got %v", got)
	}
}

func TestDoubleValidatorFixupClamps(t *testing.T) {
	v := NewDoubleValidator(0, 1, 2)
	cases := []struct {
		in, want string
	}{
		{"0.5", "0.50"},
		{"-1", "0.00"},
		{"5", "1.00"},
		{"  0.314  ", "0.31"}, // trim + round
	}
	for _, c := range cases {
		if got := v.Fixup(c.in); got != c.want {
			t.Errorf("Fixup(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- RegExpValidator -------------------------------------------------

func TestRegExpValidatorPhonePattern(t *testing.T) {
	v, err := NewRegExpValidator(`\d{3}-\d{4}`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cases := []struct {
		in   string
		want State
	}{
		{"555-1234", Acceptable},
		{"555-", Intermediate}, // prefix of a match (digit suffix completes it)
		{"5", Intermediate},    // can extend
		{"abc", Invalid},
		{"", Intermediate},
	}
	for _, c := range cases {
		if got := v.Validate(c.in); got != c.want {
			t.Errorf("RegExpValidator(\\d{3}-\\d{4}).Validate(%q) = %v, want %v",
				c.in, got, c.want)
		}
	}
}

func TestRegExpValidatorAnchors(t *testing.T) {
	// Anchors are added automatically: a pattern without ^...$ still
	// validates the whole string, not a substring.
	v, err := NewRegExpValidator(`abc`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := v.Validate("abc"); got != Acceptable {
		t.Errorf("exact match: got %v", got)
	}
	if got := v.Validate("xabcx"); got == Acceptable {
		t.Errorf("substring match should NOT be Acceptable, got %v", got)
	}
}

func TestRegExpValidatorBadPatternErrors(t *testing.T) {
	if _, err := NewRegExpValidator(`(`); err == nil {
		t.Errorf("malformed pattern should error")
	}
}

// --- Edit integration ------------------------------------------------

func TestEditSetTextRejectsInvalid(t *testing.T) {
	e := NewEdit()
	e.SetText("initial")
	e.SetValidator(NewIntValidator(0, 100))

	// "abc" is Invalid for the int validator — SetText must no-op.
	e.SetText("abc")
	if e.Text() != "initial" {
		t.Errorf("SetText(abc) bypassed validator: text = %q, want initial", e.Text())
	}

	// "42" is Acceptable — SetText commits.
	e.SetText("42")
	if e.Text() != "42" {
		t.Errorf("SetText(42) was rejected: text = %q, want 42", e.Text())
	}

	// "-" is Intermediate — SetText commits (allow mid-typing values
	// to be set programmatically too, e.g. when restoring from disk).
	e.SetText("-")
	if e.Text() != "-" {
		t.Errorf("SetText(-) was rejected: text = %q, want -", e.Text())
	}
}

func TestEditOnTextInputDropsInvalid(t *testing.T) {
	e := NewEdit()
	e.SetText("")
	e.SetValidator(NewIntValidator(0, 100))

	// Typing "1" produces "1" (Intermediate). Allow.
	e.OnTextInput("1")
	if e.Text() != "1" {
		t.Errorf("typing 1: text = %q, want 1", e.Text())
	}

	// Now typing "a" would produce "1a" (Invalid). Reject.
	e.OnTextInput("a")
	if e.Text() != "1" {
		t.Errorf("typing a: text = %q, want 1 (a should be rejected)", e.Text())
	}

	// Typing "5" produces "15" (Acceptable). Allow.
	e.OnTextInput("5")
	if e.Text() != "15" {
		t.Errorf("typing 5: text = %q, want 15", e.Text())
	}
}

func TestEditHasAcceptableInput(t *testing.T) {
	e := NewEdit()
	if !e.HasAcceptableInput() {
		t.Errorf("no validator: HasAcceptableInput() should be true")
	}
	e.SetValidator(NewIntValidator(0, 100))
	e.SetText("42")
	if !e.HasAcceptableInput() {
		t.Errorf("text 42 in [0,100]: should be acceptable")
	}
	e.SetText("-")
	if e.HasAcceptableInput() {
		t.Errorf("text -: Intermediate should NOT be acceptable")
	}
}

func TestEditValidatorFixup(t *testing.T) {
	e := NewEdit()
	e.SetText("0.5")
	e.SetValidator(NewDoubleValidator(0, 1, 2))
	e.ValidatorFixup()
	if e.Text() != "0.50" {
		t.Errorf("Fixup(0.5) = %q, want 0.50", e.Text())
	}

	// Out-of-range value is clamped.
	e.SetText("0.5") // re-set since validator allows
	e.ValidatorFixup()
	if e.Text() != "0.50" {
		t.Errorf("Fixup post-clamp = %q, want 0.50", e.Text())
	}
}

func TestEditSetValidatorNilDoesNotPanic(t *testing.T) {
	e := NewEdit()
	e.SetValidator(NewIntValidator(0, 100))
	e.SetValidator(nil) // clear
	e.SetText("anything")
	if e.Text() != "anything" {
		t.Errorf("after clearing validator, text = %q, want anything", e.Text())
	}
}
