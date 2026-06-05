package gui

import (
	"testing"

	"silk/paint"
)

// A representative multi-rule sheet exercising comments, type / state / id
// selectors and a variety of declaration values.
const sampleSheet = `
/* base button look */
Button {
	color: #FFFFFF;
	background: #2563EB;
	radius: 4;
	padding: 6px;
}

/* hover overlay only changes the background */
Button:hover {
	background: #1D4ED8;
}

/* an id-targeted button wins over the type rule */
#saveButton {
	background: #16A34A;
}

Edit {
	border-width: 1;
	color: red;
}
`

func TestStyleSheetParseMultiRule(t *testing.T) {
	sheet, err := ParseStyleSheet(sampleSheet)
	if err != nil {
		t.Fatalf("ParseStyleSheet returned error: %v", err)
	}
	if got, want := len(sheet.Rules), 4; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}

	// Rule 0: Button base.
	r0 := sheet.Rules[0]
	if r0.Selector.Type != "Button" || r0.Selector.State != "" || r0.Selector.ID != "" {
		t.Errorf("rule 0 selector = %+v, want type=Button", r0.Selector)
	}
	if got, want := len(r0.Declarations), 4; got != want {
		t.Errorf("rule 0 declaration count = %d, want %d", got, want)
	}
	if r0.Declarations["color"] != "#FFFFFF" {
		t.Errorf("rule 0 color = %q, want #FFFFFF", r0.Declarations["color"])
	}
	if r0.Declarations["padding"] != "6px" {
		t.Errorf("rule 0 padding = %q, want 6px (value kept verbatim)", r0.Declarations["padding"])
	}

	// Rule 1: Button:hover.
	if sheet.Rules[1].Selector.Type != "Button" || sheet.Rules[1].Selector.State != "hover" {
		t.Errorf("rule 1 selector = %+v, want Button:hover", sheet.Rules[1].Selector)
	}

	// Rule 2: bare #id selector -> universal type.
	if sheet.Rules[2].Selector.Type != "*" || sheet.Rules[2].Selector.ID != "saveButton" {
		t.Errorf("rule 2 selector = %+v, want *#saveButton", sheet.Rules[2].Selector)
	}

	// Selector.String round-trips.
	if got := sheet.Rules[1].Selector.String(); got != "Button:hover" {
		t.Errorf("Selector.String() = %q, want Button:hover", got)
	}
}

func TestStyleSheetSelectorForms(t *testing.T) {
	src := `
		* { color: #000000; }
		Label#title { color: #111111; }
		Label#title:disabled { color: #999999; }
	`
	sheet, err := ParseStyleSheet(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sheet.Rules) != 3 {
		t.Fatalf("rule count = %d, want 3", len(sheet.Rules))
	}
	if sheet.Rules[0].Selector.Type != "*" {
		t.Errorf("rule 0 type = %q, want *", sheet.Rules[0].Selector.Type)
	}
	got := sheet.Rules[2].Selector
	if got.Type != "Label" || got.ID != "title" || got.State != "disabled" {
		t.Errorf("rule 2 selector = %+v, want Label#title:disabled", got)
	}
	if s := got.String(); s != "Label#title:disabled" {
		t.Errorf("Selector.String() = %q, want Label#title:disabled", s)
	}
}

func TestStyleSheetLookupMerge(t *testing.T) {
	sheet, err := ParseStyleSheet(sampleSheet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Plain Button (no id, no state): only the base rule applies.
	base := sheet.Lookup("Button", "", "")
	if base["background"] != "#2563EB" {
		t.Errorf("base background = %q, want #2563EB", base["background"])
	}
	if base["color"] != "#FFFFFF" {
		t.Errorf("base color = %q, want #FFFFFF", base["color"])
	}

	// Button in hover state: :hover overlays the base background, base color stays.
	hover := sheet.Lookup("Button", "", "hover")
	if hover["background"] != "#1D4ED8" {
		t.Errorf("hover background = %q, want #1D4ED8 (overlay)", hover["background"])
	}
	if hover["color"] != "#FFFFFF" {
		t.Errorf("hover color = %q, want inherited #FFFFFF", hover["color"])
	}

	// Button with the targeted id: #id background wins over the type rule.
	withID := sheet.Lookup("Button", "saveButton", "")
	if withID["background"] != "#16A34A" {
		t.Errorf("id background = %q, want #16A34A (id overrides type)", withID["background"])
	}
	if withID["color"] != "#FFFFFF" {
		t.Errorf("id color = %q, want inherited #FFFFFF from base", withID["color"])
	}

	// id + state together: id background still dominates the hover overlay.
	idHover := sheet.Lookup("Button", "saveButton", "hover")
	if idHover["background"] != "#16A34A" {
		t.Errorf("id+hover background = %q, want #16A34A (id beats state)", idHover["background"])
	}

	// A non-matching widget type gets an empty (but non-nil) map.
	none := sheet.Lookup("Slider", "", "")
	if none == nil {
		t.Fatal("Lookup returned nil map, want non-nil")
	}
	if len(none) != 0 {
		t.Errorf("Slider lookup = %v, want empty", none)
	}
}

func TestStyleSheetLookupSourceOrderWithinTier(t *testing.T) {
	// Two same-specificity rules for the same selector: the later one wins.
	src := `
		Button { color: #111111; }
		Button { color: #222222; }
	`
	sheet, err := ParseStyleSheet(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := sheet.Lookup("Button", "", "")
	if got["color"] != "#222222" {
		t.Errorf("color = %q, want #222222 (later rule wins within a tier)", got["color"])
	}
}

func TestStyleSheetColorAccessor(t *testing.T) {
	decls := map[string]string{
		"hex6":    "#2563EB",
		"hex8":    "#11223344",
		"hex3":    "#FFF",
		"named":   "red",
		"black":   "black",
		"badhex":  "#12XY56",
		"badlen":  "#12345",
		"unknown": "notacolor",
		"empty":   "   ",
	}

	if c, ok := Color(decls, "hex6"); !ok || c != (paint.Color{R: 0x25, G: 0x63, B: 0xEB, A: 0xFF}) {
		t.Errorf("hex6 = %v ok=%v, want {37,99,235,255}", c, ok)
	}
	if c, ok := Color(decls, "hex8"); !ok || c != (paint.Color{R: 0x11, G: 0x22, B: 0x33, A: 0x44}) {
		t.Errorf("hex8 = %v ok=%v, want {17,34,51,68}", c, ok)
	}
	// #FFF expands per paint.ParseColor's single-nibble form.
	if c, ok := Color(decls, "hex3"); !ok || c.A != 0xFF {
		t.Errorf("hex3 = %v ok=%v, want a valid opaque color", c, ok)
	}
	if c, ok := Color(decls, "named"); !ok || c != (paint.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}) {
		t.Errorf("named red = %v ok=%v, want {255,0,0,255}", c, ok)
	}
	if c, ok := Color(decls, "black"); !ok || c != (paint.Color{R: 0, G: 0, B: 0, A: 0xFF}) {
		t.Errorf("named black = %v ok=%v, want opaque black", c, ok)
	}

	// Invalid / absent values must report ok=false.
	for _, key := range []string{"badhex", "badlen", "unknown", "empty", "missing"} {
		if c, ok := Color(decls, key); ok {
			t.Errorf("Color(%q) = %v ok=true, want ok=false", key, c)
		}
	}
}

func TestStyleSheetFloatAccessor(t *testing.T) {
	decls := map[string]string{
		"plain":   "12",
		"frac":    "1.5",
		"px":      "8px",
		"pct":     "50%",
		"neg":     "-3.25",
		"bad":     "abc",
		"partial": "12abc34",
		"empty":   "",
	}
	cases := []struct {
		key  string
		want float64
		ok   bool
	}{
		{"plain", 12, true},
		{"frac", 1.5, true},
		{"px", 8, true},
		{"pct", 50, true},
		{"neg", -3.25, true},
		{"bad", 0, false},
		{"partial", 0, false},
		{"empty", 0, false},
		{"missing", 0, false},
	}
	for _, c := range cases {
		got, ok := Float(decls, c.key)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Float(%q) = %v, %v; want %v, %v", c.key, got, ok, c.want, c.ok)
		}
	}
}

func TestStyleSheetIntAccessor(t *testing.T) {
	decls := map[string]string{
		"plain": "42",
		"px":    "16px",
		"frac":  "1.5",
		"bad":   "xyz",
		"empty": "",
	}
	cases := []struct {
		key  string
		want int
		ok   bool
	}{
		{"plain", 42, true},
		{"px", 16, true},
		{"frac", 0, false}, // fractional rejected by Int
		{"bad", 0, false},
		{"empty", 0, false},
		{"missing", 0, false},
	}
	for _, c := range cases {
		got, ok := Int(decls, c.key)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Int(%q) = %v, %v; want %v, %v", c.key, got, ok, c.want, c.ok)
		}
	}
}

func TestStyleSheetColorRoundTripsToScheme(t *testing.T) {
	// A hex value from the stylesheet must produce the same paint.Color the rest
	// of the framework uses for that literal (here: BlueColorScheme.Primary).
	want := BlueColorScheme().Primary // {37,99,235,255}
	decls := map[string]string{"color": want.String()}
	got, ok := Color(decls, "color")
	if !ok {
		t.Fatal("Color() ok=false for a scheme color literal")
	}
	if got != want {
		t.Errorf("round-trip color = %v, want %v", got, want)
	}
}

func TestStyleSheetMalformedDoesNotPanic(t *testing.T) {
	bad := []string{
		"Button { color: #fff",            // missing closing brace
		"Button color: #fff; }",           // missing opening brace -> trailing tokens
		"{ color: #fff; }",                // empty selector
		"123Bad { color: #fff; }",         // invalid type
		"Button { : #fff; }",              // missing property name
		"Button { color; }",               // declaration missing ':'
		"Button:1bad { color: #fff; }",    // invalid state
		"Button#1bad { color: #fff; }",    // invalid id
		"",                                // empty input
		"   \n\t  ",                       // whitespace only
		"/* just a comment */",            // comment only
		"/* unterminated comment",         // unterminated comment
	}
	for _, src := range bad {
		// Must not panic; result sheet is always non-nil.
		sheet, _ := ParseStyleSheet(src)
		if sheet == nil {
			t.Errorf("ParseStyleSheet(%q) returned nil sheet", src)
		}
	}
}

func TestStyleSheetMalformedSurfacesError(t *testing.T) {
	// A sheet mixing one good rule and one broken rule: the good rule survives,
	// and the error is surfaced as a *ParseError.
	src := `
		Button { color: #fff; }
		123Bad { color: #000; }
	`
	sheet, err := ParseStyleSheet(src)
	if err == nil {
		t.Fatal("expected a parse error for the malformed rule, got nil")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("error type = %T, want *ParseError", err)
	}
	if len(pe.Errors) == 0 {
		t.Error("ParseError.Errors is empty, want at least one entry")
	}
	if len(sheet.Rules) != 1 {
		t.Errorf("good-rule count = %d, want 1 (bad rule skipped)", len(sheet.Rules))
	}
	if sheet.Rules[0].Selector.Type != "Button" {
		t.Errorf("surviving rule = %+v, want Button", sheet.Rules[0].Selector)
	}
}

func TestStyleSheetWellFormedNoError(t *testing.T) {
	sheet, err := ParseStyleSheet(sampleSheet)
	if err != nil {
		t.Errorf("well-formed sheet returned error: %v", err)
	}
	if sheet == nil {
		t.Fatal("sheet is nil")
	}
}
