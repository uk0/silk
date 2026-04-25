package paint

import (
	"image/color"
	"testing"
)

// ---------------------------------------------------------------------------
// Color tests
// ---------------------------------------------------------------------------

func TestColorString(t *testing.T) {
	tests := []struct {
		c    Color
		want string
	}{
		{Color{0xFF, 0x00, 0x00, 0xFF}, "#FF0000"},
		{Color{0x00, 0xFF, 0x00, 0xFF}, "#00FF00"},
		{Color{0x00, 0x00, 0xFF, 0xFF}, "#0000FF"},
		{Color{0x00, 0x00, 0x00, 0xFF}, "#000000"},
		{Color{0xFF, 0xFF, 0xFF, 0xFF}, "#FFFFFF"},
		{Color{0xAB, 0xCD, 0xEF, 0x80}, "#ABCDEF80"},
		{Color{0x00, 0x00, 0x00, 0x00}, "#00000000"},
	}
	for _, tt := range tests {
		got := tt.c.String()
		if got != tt.want {
			t.Errorf("Color%v.String() = %q, want %q", tt.c, got, tt.want)
		}
	}
}

func TestColorRGBA(t *testing.T) {
	// Fully opaque white
	c := Color{0xFF, 0xFF, 0xFF, 0xFF}
	r, g, b, a := c.RGBA()
	if a != 0xFFFF {
		t.Errorf("white alpha: got %d, want %d", a, 0xFFFF)
	}
	if r != 0xFFFF || g != 0xFFFF || b != 0xFFFF {
		t.Errorf("white channels: got (%d,%d,%d), want (65535,65535,65535)", r, g, b)
	}

	// Fully transparent
	c = Color{0xFF, 0xFF, 0xFF, 0x00}
	r, g, b, a = c.RGBA()
	if a != 0 {
		t.Errorf("transparent alpha: got %d, want 0", a)
	}
	if r != 0 || g != 0 || b != 0 {
		t.Errorf("transparent channels should be 0: got (%d,%d,%d)", r, g, b)
	}
}

func TestColorNRGBAf(t *testing.T) {
	c := Color{255, 0, 128, 255}
	r, g, b, a := c.NRGBAf()
	if a != 1.0 {
		t.Errorf("NRGBAf alpha: got %f, want 1.0", a)
	}
	if r != 1.0 {
		t.Errorf("NRGBAf red: got %f, want 1.0", r)
	}
	if g != 0.0 {
		t.Errorf("NRGBAf green: got %f, want 0.0", g)
	}
	// b = 128/255 ~= 0.502
	if b < 0.50 || b > 0.51 {
		t.Errorf("NRGBAf blue: got %f, want ~0.502", b)
	}
}

func TestColorImplementsColorInterface(t *testing.T) {
	var c color.Color = Color{10, 20, 30, 255}
	_ = c // Compile-time check that Color satisfies color.Color
}

// ---------------------------------------------------------------------------
// ParseColor tests
// ---------------------------------------------------------------------------

func TestParseColorHex(t *testing.T) {
	tests := []struct {
		input string
		want  Color
	}{
		{"#FF0000", Color{0xFF, 0x00, 0x00, 0xFF}},
		{"#00ff00", Color{0x00, 0xFF, 0x00, 0xFF}},
		{"#0000FF", Color{0x00, 0x00, 0xFF, 0xFF}},
		{"#AABBCCDD", Color{0xAA, 0xBB, 0xCC, 0xDD}},
	}
	for _, tt := range tests {
		got := ParseColor(tt.input)
		if got != tt.want {
			t.Errorf("ParseColor(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseColorEmpty(t *testing.T) {
	c := ParseColor("")
	if c != (Color{0, 0, 0, 255}) {
		t.Errorf("ParseColor(\"\") = %v, want black", c)
	}
}

func TestParseColorNamed(t *testing.T) {
	tests := []struct {
		name string
		want Color
	}{
		{"red", Color{0xFF, 0x00, 0x00, 0xFF}},
		{"Red", Color{0xFF, 0x00, 0x00, 0xFF}},
		{"RED", Color{0xFF, 0x00, 0x00, 0xFF}},
		{"white", Color{0xFF, 0xFF, 0xFF, 0xFF}},
		{"black", Color{0x00, 0x00, 0x00, 0xFF}},
	}
	for _, tt := range tests {
		got := ParseColor(tt.name)
		if got != tt.want {
			t.Errorf("ParseColor(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseColorInvalidHexLength(t *testing.T) {
	// Invalid hex length should return black
	c := ParseColor("#AB")
	if c != (Color{0, 0, 0, 255}) {
		t.Errorf("ParseColor(\"#AB\") = %v, want black", c)
	}
}

func TestParseColorUnknownName(t *testing.T) {
	c := ParseColor("notacolor")
	if c != (Color{0, 0, 0, 255}) {
		t.Errorf("ParseColor(\"notacolor\") = %v, want black", c)
	}
}

// ---------------------------------------------------------------------------
// ColorFromBgrUint32 tests
// ---------------------------------------------------------------------------

func TestColorFromBgrUint32(t *testing.T) {
	// 0xFF0000 means R=0xFF, G=0x00, B=0x00 when treated as RGB
	// But the function name says BGR, so the byte layout is B=0x00, G=0x00, R=0xFF
	// Wait, let's look at the implementation:
	//   B: uint8(c & 0xff), G: uint8((c >> 8) & 0xff), R: uint8((c >> 16) & 0xff)
	// So for 0xFF0000: R = 0xFF, G = 0x00, B = 0x00
	c := ColorFromBgrUint32(0xFF0000)
	if c.R != 0xFF || c.G != 0x00 || c.B != 0x00 {
		t.Errorf("ColorFromBgrUint32(0xFF0000) = %v, want R=255,G=0,B=0", c)
	}
	if c.A != 0xFF {
		t.Errorf("alpha should always be 0xFF, got %d", c.A)
	}

	// 0x0000FF: R = 0x00, G = 0x00, B = 0xFF
	c = ColorFromBgrUint32(0x0000FF)
	if c.R != 0x00 || c.G != 0x00 || c.B != 0xFF {
		t.Errorf("ColorFromBgrUint32(0x0000FF) = %v, want R=0,G=0,B=255", c)
	}
}

// ---------------------------------------------------------------------------
// ColorModel tests
// ---------------------------------------------------------------------------

func TestColorModel(t *testing.T) {
	// Converting a Color through the model should return the same Color
	orig := Color{100, 150, 200, 255}
	converted := ColorModel.Convert(orig)
	if converted != orig {
		t.Errorf("ColorModel.Convert(Color) should return same color, got %v", converted)
	}

	// Converting from an NRGBA color
	nrgba := color.NRGBA{R: 255, G: 0, B: 0, A: 255}
	converted = ColorModel.Convert(nrgba)
	cr, ok := converted.(Color)
	if !ok {
		t.Fatal("ColorModel.Convert should return a Color")
	}
	if cr.R != 255 || cr.G != 0 || cr.B != 0 || cr.A != 255 {
		t.Errorf("ColorModel.Convert(NRGBA red) = %v, want red", cr)
	}
}

// ---------------------------------------------------------------------------
// HSL tests
// ---------------------------------------------------------------------------

func TestHSLRGBA(t *testing.T) {
	// Pure red in HSL: H=0, S=1, L=0.5, A=1
	hsl := HSL{0, 1.0, 0.5, 1.0}
	r, g, b, a := hsl.RGBA()
	if a != 0xFFFF {
		t.Errorf("HSL red alpha: got %d, want %d", a, 0xFFFF)
	}
	// Red channel should be near max
	if r < 0xFE00 {
		t.Errorf("HSL red: r channel too low: %d", r)
	}
	// Green and blue should be near zero
	if g > 0x0100 {
		t.Errorf("HSL red: g channel too high: %d", g)
	}
	if b > 0x0100 {
		t.Errorf("HSL red: b channel too high: %d", b)
	}
}

func TestHSLGrayscale(t *testing.T) {
	// S=0 means grayscale, L controls brightness
	hsl := HSL{0, 0, 0.5, 1.0}
	r, g, b, _ := hsl.RGBA()
	// All channels should be equal for gray
	if r != g || g != b {
		t.Errorf("HSL gray: channels should be equal, got r=%d g=%d b=%d", r, g, b)
	}
}

func TestHSLModelRoundtrip(t *testing.T) {
	// Converting a Color to HSL and back should be approximately the same
	orig := color.NRGBA{R: 200, G: 100, B: 50, A: 255}
	hslColor := HSLModel.Convert(orig)
	hsl, ok := hslColor.(HSL)
	if !ok {
		t.Fatal("HSLModel.Convert should return HSL")
	}
	// Convert back via RGBA
	r, g, b, _ := hsl.RGBA()
	// Check that values are in the ballpark (allowing rounding)
	rOrig, gOrig, bOrig, _ := orig.RGBA()
	tolerance := uint32(512)
	if diff(r, rOrig) > tolerance || diff(g, gOrig) > tolerance || diff(b, bOrig) > tolerance {
		t.Errorf("HSL roundtrip too far off: orig RGBA=(%d,%d,%d), HSL RGBA=(%d,%d,%d)",
			rOrig, gOrig, bOrig, r, g, b)
	}
}

func diff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// ---------------------------------------------------------------------------
// Pen tests
// ---------------------------------------------------------------------------

func TestNewPen(t *testing.T) {
	cr := Color{255, 0, 0, 255}
	p := NewPen(cr, 2.5)
	if p.Color() != cr {
		t.Errorf("Pen.Color() = %v, want %v", p.Color(), cr)
	}
	if p.Width() != 2.5 {
		t.Errorf("Pen.Width() = %f, want 2.5", p.Width())
	}
}

func TestPenImplementsInterfaces(t *testing.T) {
	cr := Color{0, 0, 0, 255}
	p := NewPen(cr, 1.0)

	// pen should implement Pen interface
	var _ Pen = p
	// pen should implement LineStyle interface
	var _ LineStyle = p
}

func TestPenZeroWidth(t *testing.T) {
	p := NewPen(Color{}, 0)
	if p.Width() != 0 {
		t.Errorf("zero-width pen: got %f", p.Width())
	}
}

// ---------------------------------------------------------------------------
// SolidBrush tests
// ---------------------------------------------------------------------------

func TestNewSolidBrush(t *testing.T) {
	cr := Color{10, 20, 30, 255}
	br := NewSolidBrush(cr)
	if br.Color != cr {
		t.Errorf("SolidBrush.Color = %v, want %v", br.Color, cr)
	}
}

func TestSolidBrushImplementsBrush(t *testing.T) {
	var _ Brush = &SolidBrush{}
}

// ---------------------------------------------------------------------------
// Constants tests
// ---------------------------------------------------------------------------

func TestOperatorValues(t *testing.T) {
	// OpClear should be 0
	if OpClear != 0 {
		t.Errorf("OpClear = %d, want 0", OpClear)
	}
	// OpSource should be 1
	if OpSource != 1 {
		t.Errorf("OpSource = %d, want 1", OpSource)
	}
	// OpOver should be 2
	if OpOver != 2 {
		t.Errorf("OpOver = %d, want 2", OpOver)
	}
}

func TestExtendValues(t *testing.T) {
	if ExtNone != 0 {
		t.Errorf("ExtNone = %d, want 0", ExtNone)
	}
	if ExtRepeat != 1 {
		t.Errorf("ExtRepeat = %d, want 1", ExtRepeat)
	}
	if ExtReflect != 2 {
		t.Errorf("ExtReflect = %d, want 2", ExtReflect)
	}
	if ExtPad != 3 {
		t.Errorf("ExtPad = %d, want 3", ExtPad)
	}
}

// ---------------------------------------------------------------------------
// Font construction tests (pure Go, no cairo calls)
// ---------------------------------------------------------------------------

func TestNewFontClampsSize(t *testing.T) {
	// Size < 6 should be clamped to 6
	f := NewFont("Arial", 2, false, false)
	if f.Size() != 6 {
		t.Errorf("NewFont size clamp min: got %d, want 6", f.Size())
	}

	// Size > 72 should be clamped to 72
	f = NewFont("Arial", 100, false, false)
	if f.Size() != 72 {
		t.Errorf("NewFont size clamp max: got %d, want 72", f.Size())
	}

	// Normal size should pass through
	f = NewFont("Arial", 14, false, false)
	if f.Size() != 14 {
		t.Errorf("NewFont normal size: got %d, want 14", f.Size())
	}
}

func TestFontProperties(t *testing.T) {
	f := NewFont("Helvetica", 12, true, true)
	if f.Family() != "Helvetica" {
		t.Errorf("Family() = %q, want \"Helvetica\"", f.Family())
	}
	if f.Size() != 12 {
		t.Errorf("Size() = %d, want 12", f.Size())
	}
	if !f.Bold() {
		t.Error("Bold() should be true")
	}
	if !f.Italic() {
		t.Error("Italic() should be true")
	}
}

func TestFontSetters(t *testing.T) {
	f := NewFont("Arial", 12, false, false)

	f.SetFamily("Times")
	if f.Family() != "Times" {
		t.Errorf("after SetFamily: got %q", f.Family())
	}

	f.SetSize(20)
	if f.Size() != 20 {
		t.Errorf("after SetSize: got %d", f.Size())
	}

	f.SetBold(true)
	if !f.Bold() {
		t.Error("after SetBold(true): Bold() should be true")
	}

	f.SetItalic(true)
	if !f.Italic() {
		t.Error("after SetItalic(true): Italic() should be true")
	}
}

func TestFontString(t *testing.T) {
	f := NewFont("Arial", 12, false, false)
	s := f.String()
	if s != "Arial:12" {
		t.Errorf("String() = %q, want \"Arial:12\"", s)
	}

	f2 := NewFont("Arial", 12, true, false)
	s2 := f2.String()
	if s2 != "Arial:12(b)" {
		t.Errorf("String() bold = %q, want \"Arial:12(b)\"", s2)
	}

	f3 := NewFont("Arial", 12, false, true)
	s3 := f3.String()
	if s3 != "Arial:12(i)" {
		t.Errorf("String() italic = %q, want \"Arial:12(i)\"", s3)
	}

	f4 := NewFont("Arial", 12, true, true)
	s4 := f4.String()
	if s4 != "Arial:12(b)(i)" {
		t.Errorf("String() bold+italic = %q, want \"Arial:12(b)(i)\"", s4)
	}
}

func TestFontEqual(t *testing.T) {
	f1 := NewFont("Arial", 12, false, false)
	f2 := NewFont("Arial", 12, false, false)
	if !f1.Equal(f2) {
		t.Error("identical fonts should be Equal")
	}

	f3 := NewFont("Arial", 14, false, false)
	if f1.Equal(f3) {
		t.Error("fonts with different sizes should not be Equal")
	}

	f4 := NewFont("Times", 12, false, false)
	if f1.Equal(f4) {
		t.Error("fonts with different families should not be Equal")
	}
}

func TestFontDup(t *testing.T) {
	f1 := NewFont("Arial", 12, true, true)
	f2 := f1.Dup()

	if !f1.Equal(f2) {
		t.Error("Dup() should produce an equal font")
	}

	// Modifying the dup should not affect the original
	f2.SetFamily("Times")
	if f1.Family() == "Times" {
		t.Error("modifying Dup should not affect original")
	}
}

// ---------------------------------------------------------------------------
// floatToInt32 helper test
// ---------------------------------------------------------------------------

func TestFloatToInt32(t *testing.T) {
	tests := []struct {
		in   float64
		want uint32
	}{
		{0.0, 0},
		{1.0, 0xFFFF},
		{0.5, 0x7FFF},
	}
	for _, tt := range tests {
		got := floatToInt32(tt.in)
		// Allow small rounding difference
		if diff(got, tt.want) > 1 {
			t.Errorf("floatToInt32(%f) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
