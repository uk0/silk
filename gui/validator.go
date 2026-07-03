package gui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// State is the result of a Validator's classification of an input string.
// Mirrors Qt's QValidator::State three-value enum, which is the canonical
// shape for input validators that need to distinguish "not valid yet, but
// the user might still finish typing" from "definitively wrong".
//
// Edit / SpinBox / NumberInput call Validator.Validate on each keypress and
// react per state:
//   - Invalid       → reject the change, current text stays
//   - Intermediate  → accept the change, but disable Submit / OK actions
//   - Acceptable    → accept and treat as final
type State int

const (
	// Invalid means the input cannot become valid no matter what the user
	// types next. Edit reverts to the previous accepted text.
	Invalid State = iota

	// Intermediate means the input is incomplete but could become valid
	// after additional keystrokes. Edit accepts the change but the host
	// should refuse to commit (e.g. greyed-out OK button).
	Intermediate

	// Acceptable means the input fully satisfies the validator. Safe to
	// commit / submit.
	Acceptable
)

// String aids debug and error logs. Mirrors the Qt naming.
func (s State) String() string {
	switch s {
	case Invalid:
		return "Invalid"
	case Intermediate:
		return "Intermediate"
	case Acceptable:
		return "Acceptable"
	}
	return "<unknown>"
}

// Validator classifies an input string into one of the three states above.
// Implementations are stateless w.r.t. the input — Validate must not mutate
// the receiver based on the call. This lets a single Validator instance be
// safely shared by multiple Edit widgets, mirroring Qt's expectation.
type Validator interface {
	Validate(input string) State
}

// Fixupper is the optional companion to Validator: a chance to normalise
// input on lose-focus (or whenever the host calls Fixup). Typical fixups:
//   - trimming whitespace
//   - upper / lower casing
//   - clamping a numeric value into the validator's range
//
// Implement on the same struct as Validator when the validator naturally
// produces a canonical form; Edit detects the interface via type assertion
// and applies it transparently.
type Fixupper interface {
	Fixup(input string) string
}

// --- IntValidator ----------------------------------------------------
//
// Mirrors QIntValidator. Accepts integer strings in the closed range
// [Bottom, Top]. An empty string or a leading sign with no digits is
// Intermediate so the user can keep typing. Non-digit characters are
// always Invalid.

// IntValidator constrains input to integers in the inclusive range
// [Bottom, Top]. Bottom <= Top is the host's responsibility — a swapped
// pair degenerates to "no value is acceptable", which is observable but
// not a panic.
type IntValidator struct {
	Bottom int64
	Top    int64
}

// NewIntValidator constructs a validator over the given range. Bottom
// and Top are inclusive, matching QIntValidator.
func NewIntValidator(bottom, top int64) *IntValidator {
	return &IntValidator{Bottom: bottom, Top: top}
}

// Validate classifies input. The state machine:
//
//	""        → Intermediate (user just emptied the field)
//	"-" "+"   → Intermediate (typed sign, no digits yet)
//	"abc"     → Invalid     (non-numeric)
//	"42"      → Acceptable  if Bottom <= 42 <= Top
//	          → Intermediate if 42 outside but a longer prefix could land in range
//	          → Invalid     if even with appended digits it can't reach the range
//
// The Intermediate-vs-Invalid distinction outside the range is a deliberate
// simplification: we only check whether the absolute-value is below |Top|,
// which catches the common "user typed first digit of a multi-digit value"
// case without expensive search.
func (v *IntValidator) Validate(input string) State {
	if input == "" {
		return Intermediate
	}
	if input == "-" || input == "+" {
		return Intermediate
	}
	n, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		return Invalid
	}
	if n >= v.Bottom && n <= v.Top {
		return Acceptable
	}
	// Out of range. If appending more digits could still produce an in-range
	// value, treat as Intermediate so the user can keep typing. We use the
	// magnitude bound: |n| < max(|Bottom|, |Top|) means another digit could
	// scale it into range.
	abs := n
	if abs < 0 {
		abs = -abs
	}
	maxMag := absInt64(v.Top)
	if absInt64(v.Bottom) > maxMag {
		maxMag = absInt64(v.Bottom)
	}
	if abs < maxMag {
		return Intermediate
	}
	return Invalid
}

// Fixup clamps the input into the validator's range. An unparseable
// input is left unchanged (Validate would already have returned Invalid).
// Whitespace is trimmed unconditionally — the canonical form has no spaces.
func (v *IntValidator) Fixup(input string) string {
	s := strings.TrimSpace(input)
	if s == "" || s == "-" || s == "+" {
		return s
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return s
	}
	if n < v.Bottom {
		return strconv.FormatInt(v.Bottom, 10)
	}
	if n > v.Top {
		return strconv.FormatInt(v.Top, 10)
	}
	return strconv.FormatInt(n, 10)
}

func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// --- DoubleValidator -------------------------------------------------
//
// Mirrors QDoubleValidator with a single notation slot — we don't carry
// Qt's StandardNotation/ScientificNotation distinction because Go's
// strconv.ParseFloat handles both transparently.

// DoubleValidator constrains input to a floating-point value in
// [Bottom, Top] with at most Decimals fractional digits. Decimals == 0
// means integer-only; -1 (the zero value) means unlimited fractional
// digits.
type DoubleValidator struct {
	Bottom   float64
	Top      float64
	Decimals int // -1 = unlimited
}

// NewDoubleValidator: decimals = -1 for unlimited, 0 for integer, N for
// at most N digits past the decimal point.
func NewDoubleValidator(bottom, top float64, decimals int) *DoubleValidator {
	return &DoubleValidator{Bottom: bottom, Top: top, Decimals: decimals}
}

// Validate. The state machine matches IntValidator's intermediate rules
// (signs, empty), with an additional "trailing decimal point" carve-out:
// "3." is Intermediate (the user is mid-typing) even though strconv
// rejects it.
func (v *DoubleValidator) Validate(input string) State {
	if input == "" || input == "-" || input == "+" || input == "." || input == "-." || input == "+." {
		return Intermediate
	}
	// Trailing decimal point: "3." is intermediate (user may type more).
	if strings.HasSuffix(input, ".") && !strings.HasSuffix(input[:len(input)-1], ".") {
		// Try parsing without the trailing dot.
		if _, err := strconv.ParseFloat(input[:len(input)-1], 64); err == nil {
			return Intermediate
		}
	}

	f, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return Invalid
	}
	if v.Decimals >= 0 {
		// Count digits after the decimal point.
		if i := strings.IndexByte(input, '.'); i >= 0 {
			frac := len(input) - i - 1
			if frac > v.Decimals {
				return Invalid
			}
		}
	}
	if f >= v.Bottom && f <= v.Top {
		return Acceptable
	}
	// Out of range: defer the Intermediate vs Invalid choice to magnitude
	// like IntValidator. For floats the heuristic is fuzzier; we accept
	// any out-of-range value as Intermediate so user can keep typing.
	return Intermediate
}

// Fixup clamps the input into the range and rounds to Decimals if set.
// Empty / sign-only inputs are returned unchanged.
func (v *DoubleValidator) Fixup(input string) string {
	s := strings.TrimSpace(input)
	if s == "" || s == "-" || s == "+" {
		return s
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	if f < v.Bottom {
		f = v.Bottom
	}
	if f > v.Top {
		f = v.Top
	}
	if v.Decimals >= 0 {
		return strconv.FormatFloat(f, 'f', v.Decimals, 64)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// --- RegExpValidator ------------------------------------------------
//
// Mirrors QRegExpValidator. A full match against the regular expression
// is Acceptable; a string that is a prefix of some matching string is
// Intermediate; everything else is Invalid.

// RegExpValidator validates input against a Go regexp. Construction with
// an invalid pattern returns nil + the parse error so callers can report
// rather than rendering with a panic.
type RegExpValidator struct {
	re *regexp.Regexp
	// anchored holds the same pattern wrapped in ^...$ so we can do an
	// exact-match check independent of the user's pattern.
	anchored *regexp.Regexp
}

// NewRegExpValidator compiles pattern. Returns (nil, err) on a bad
// pattern; callers should surface the error to the developer rather
// than silently fall through to "always Invalid".
func NewRegExpValidator(pattern string) (*RegExpValidator, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	anchoredPattern := pattern
	if !strings.HasPrefix(anchoredPattern, "^") {
		anchoredPattern = "^" + anchoredPattern
	}
	if !strings.HasSuffix(anchoredPattern, "$") {
		anchoredPattern = anchoredPattern + "$"
	}
	anchored, err := regexp.Compile(anchoredPattern)
	if err != nil {
		return nil, err
	}
	return &RegExpValidator{re: re, anchored: anchored}, nil
}

// Validate. The classifier:
//
//	full match (^pattern$)     → Acceptable
//	prefix of a matching string → Intermediate
//	otherwise                   → Invalid
//
// "Prefix of a matching string" is determined by checking whether the
// underlying regex engine can match the input as the start of a string
// when the pattern is run as unanchored. This catches the common UI
// patterns (phone numbers, IPs, dates, emails) without a hand-rolled
// partial matcher.
//
// Method: take the loc returned by FindStringIndex on the unanchored
// pattern. If the regex matches starting at position 0 AND the match
// reaches the end of input, the input is a viable prefix. We further
// approximate by appending probe completions and trying again — this
// covers cases where the pattern includes non-greedy quantifiers that
// FindStringIndex could otherwise consume only zero chars.
func (v *RegExpValidator) Validate(input string) State {
	if input == "" {
		return Intermediate
	}
	if v.anchored.MatchString(input) {
		return Acceptable
	}

	// Heuristic 1: anchored prefix probe — append diverse completion
	// strings and see if any results in a full match. The probe set
	// covers patterns up to 24 characters which spans common UI
	// shapes (phone, IP, date, email).
	probes := []string{
		"0", "00", "000", "0000", "00000", "000000", "0000000",
		"abc", "abcdef", "abcdefgh",
		"-0", "-00", "-0000", "-00000",
		"-abc", "-abcdef",
		":00", ":00:00", "/00", "/00/00",
		"0-0", "0-00", "0-000", "0-0000",
		"00-0", "00-00", "00-0000",
		"000-0", "000-00", "000-0000",
		".00", "..", "@a", "@a.b",
		"abc-defg", "abcd-efgh",
	}
	for _, probe := range probes {
		if v.anchored.MatchString(input + probe) {
			return Intermediate
		}
	}

	// Heuristic 2: anchored prefix at start. If FindStringIndex on
	// the unanchored pattern matches starting at position 0, the
	// input contains the prefix of a valid match — useful for free-
	// form patterns where probes alone don't terminate at the right
	// shape.
	if loc := v.re.FindStringIndex(input); loc != nil && loc[0] == 0 && loc[1] == len(input) {
		return Intermediate
	}

	return Invalid
}

// --- ErrorMessager ---------------------------------------------------
//
// ErrorMessager is the optional companion to Validator (sibling of
// Fixupper): it supplies a human-readable reason when Validate returns a
// non-Acceptable state. Edit detects it via type assertion and stores the
// message in Edit.ValidationError() so a form can tell the user *why* a
// field is rejected. Classify-only validators (Int / Double / RegExp) omit
// it; Edit then falls back to a generic message.
type ErrorMessager interface {
	// ErrorMessage returns the reason input is not Acceptable, or "" when
	// input is in fact Acceptable. Edit calls it only for non-Acceptable
	// input.
	ErrorMessage(input string) string
}

// --- RequiredValidator ----------------------------------------------
//
// RequiredValidator rejects empty (whitespace-only) input. It deliberately
// never returns Invalid: an empty value must stay typable and
// programmatically settable so the user can clear the field and a form
// reset works. Empty is therefore Intermediate ("allowed in the buffer but
// not submittable"); non-empty is Acceptable. A required field is thus a
// submit-gate plus red-border affordance, not a keystroke filter.
type RequiredValidator struct {
	// Message overrides the default "this field is required" text returned
	// by ErrorMessage. Leave empty for the default.
	Message string
}

// Validate: whitespace-only input is Intermediate, anything else is
// Acceptable.
func (v *RequiredValidator) Validate(input string) State {
	if strings.TrimSpace(input) == "" {
		return Intermediate
	}
	return Acceptable
}

// ErrorMessage returns the required-field message for empty input, or ""
// once the field carries a value.
func (v *RequiredValidator) ErrorMessage(input string) string {
	if strings.TrimSpace(input) != "" {
		return ""
	}
	if v.Message != "" {
		return v.Message
	}
	return "this field is required"
}

// --- ValidatorFunc --------------------------------------------------
//
// ValidatorFunc adapts an ordinary func(string) error into a Validator so
// callers can express one-off rules ("must contain @", "must equal the
// confirm-password field") without declaring a type. A nil error is
// Acceptable; a non-nil error is Intermediate — NOT Invalid — so a partial
// value the user is still typing is never keystroke-dropped: the func only
// gates submission and drives the error border. The error's text becomes
// the ValidationError message.
type ValidatorFunc func(input string) error

// Validate runs the wrapped func: nil error → Acceptable, else Intermediate.
func (f ValidatorFunc) Validate(input string) State {
	if f(input) == nil {
		return Acceptable
	}
	return Intermediate
}

// ErrorMessage runs the wrapped func and returns its error text, or "" when
// the func is satisfied.
func (f ValidatorFunc) ErrorMessage(input string) string {
	if err := f(input); err != nil {
		return err.Error()
	}
	return ""
}

// --- LengthValidator ------------------------------------------------
//
// LengthValidator constrains input to a rune-count range [Min, Max]. Below
// Min is Intermediate (keep typing to reach the minimum); within range is
// Acceptable; above Max is Invalid (a hard cap like maxlength — the extra
// keystroke is dropped, since appending can only make it longer). Max <= 0
// means no upper bound.
type LengthValidator struct {
	Min int
	Max int // <= 0 means unbounded
}

// Validate classifies by rune count: below Min is Intermediate, above Max
// is Invalid, otherwise Acceptable.
func (v *LengthValidator) Validate(input string) State {
	n := len([]rune(input))
	if v.Max > 0 && n > v.Max {
		return Invalid
	}
	if n < v.Min {
		return Intermediate
	}
	return Acceptable
}

// ErrorMessage explains a below-Min or above-Max count, "" when in range.
func (v *LengthValidator) ErrorMessage(input string) string {
	n := len([]rune(input))
	if v.Max > 0 && n > v.Max {
		return fmt.Sprintf("must be at most %d characters", v.Max)
	}
	if n < v.Min {
		return fmt.Sprintf("must be at least %d characters", v.Min)
	}
	return ""
}
