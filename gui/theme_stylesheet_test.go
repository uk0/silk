package gui

import (
	"strings"
	"testing"

	"silk/paint"
)

// resetThemeForTest forces Theme() to rebuild and clears any snapshot left by
// a previous LoadStyleSheet call so each test starts from a clean baseline.
func resetThemeForTest(t *testing.T) {
	t.Helper()
	defaultThemeSingleton = nil
	themeDefaultsSnapshot = nil
	_ = Theme()
}

func TestThemeLoadStyleSheetOverridesColorFields(t *testing.T) {
	resetThemeForTest(t)
	defer resetThemeForTest(t)

	src := `
		Frame {
			background: #112233;
			color: #445566;
			border: #778899;
			highlight: #abcdef;
			view-background: #fefefe;
			separator: #010203;
		}
		Menu {
			background: #1a1a1a;
			color: #2b2b2b;
			border: #3c3c3c;
		}
		Menu:active {
			background: #4d4d4d;
			color: #5e5e5e;
		}
		Menu:disabled {
			color: #6f6f6f;
		}
	`
	if err := Theme().LoadStyleSheet(src); err != nil {
		t.Fatalf("LoadStyleSheet returned unexpected error: %v", err)
	}

	cases := []struct {
		name string
		got  paint.Color
		want paint.Color
	}{
		{"FormColor", Theme().FormColor, paint.Color{0x11, 0x22, 0x33, 0xff}},
		{"TextColor", Theme().TextColor, paint.Color{0x44, 0x55, 0x66, 0xff}},
		{"BorderColor", Theme().BorderColor, paint.Color{0x77, 0x88, 0x99, 0xff}},
		{"HighLightColor", Theme().HighLightColor, paint.Color{0xab, 0xcd, 0xef, 0xff}},
		{"ViewBGColor", Theme().ViewBGColor, paint.Color{0xfe, 0xfe, 0xfe, 0xff}},
		{"SeperatorColor", Theme().SeperatorColor, paint.Color{0x01, 0x02, 0x03, 0xff}},
		{"MenuBGColor", Theme().MenuBGColor, paint.Color{0x1a, 0x1a, 0x1a, 0xff}},
		{"MenuTextColor", Theme().MenuTextColor, paint.Color{0x2b, 0x2b, 0x2b, 0xff}},
		{"MenuBorderColor", Theme().MenuBorderColor, paint.Color{0x3c, 0x3c, 0x3c, 0xff}},
		{"MenuActiveBGColor", Theme().MenuActiveBGColor, paint.Color{0x4d, 0x4d, 0x4d, 0xff}},
		{"MenuActiveTextColor", Theme().MenuActiveTextColor, paint.Color{0x5e, 0x5e, 0x5e, 0xff}},
		{"MenuGrayTextColor", Theme().MenuGrayTextColor, paint.Color{0x6f, 0x6f, 0x6f, 0xff}},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestThemeLoadStyleSheetFocusFallbackHighlight(t *testing.T) {
	resetThemeForTest(t)
	defer resetThemeForTest(t)

	// No Frame{highlight} present — the *:focus { color } rule must populate
	// HighLightColor instead.
	src := `*:focus { color: #aabbcc; }`
	if err := Theme().LoadStyleSheet(src); err != nil {
		t.Fatalf("LoadStyleSheet returned unexpected error: %v", err)
	}
	want := paint.Color{0xaa, 0xbb, 0xcc, 0xff}
	if got := Theme().HighLightColor; got != want {
		t.Errorf("HighLightColor = %v, want %v (via *:focus { color })", got, want)
	}
}

func TestThemeLoadStyleSheetBumpsRevision(t *testing.T) {
	resetThemeForTest(t)
	defer resetThemeForTest(t)

	before := ThemeRev()
	if err := Theme().LoadStyleSheet(`Frame { background: #010101; }`); err != nil {
		t.Fatalf("LoadStyleSheet returned unexpected error: %v", err)
	}
	if ThemeRev() == before {
		t.Errorf("ThemeRev did not advance after LoadStyleSheet (still %d)", before)
	}
}

func TestThemeLoadStyleSheetMalformedStillAppliesGood(t *testing.T) {
	resetThemeForTest(t)
	defer resetThemeForTest(t)

	// One malformed rule (missing ':'), one well-formed rule. The parser is
	// forgiving: it returns the error AND the partially-populated sheet, so
	// LoadStyleSheet must apply the good rule even though it reports the error.
	src := `
		Frame { totally broken without a colon }
		Frame { background: #abcdef; }
	`
	err := Theme().LoadStyleSheet(src)
	if err == nil {
		t.Fatalf("expected a parse error for the malformed rule, got nil")
	}
	if !strings.Contains(err.Error(), "stylesheet") {
		t.Errorf("error %q does not look like a stylesheet ParseError", err.Error())
	}
	want := paint.Color{0xab, 0xcd, 0xef, 0xff}
	if got := Theme().FormColor; got != want {
		t.Errorf("FormColor = %v, want %v (good rule must still apply)", got, want)
	}
}

func TestThemeResetStyleSheetRestoresDefaults(t *testing.T) {
	resetThemeForTest(t)
	defer resetThemeForTest(t)

	origForm := Theme().FormColor
	origText := Theme().TextColor
	origBorder := Theme().BorderColor
	origMenuBG := Theme().MenuBGColor

	src := `
		Frame { background: #111111; color: #222222; border: #333333; }
		Menu  { background: #444444; }
	`
	if err := Theme().LoadStyleSheet(src); err != nil {
		t.Fatalf("LoadStyleSheet returned unexpected error: %v", err)
	}
	if Theme().FormColor == origForm {
		t.Fatalf("LoadStyleSheet did not change FormColor; test precondition failed")
	}

	Theme().ResetStyleSheet()

	if got := Theme().FormColor; got != origForm {
		t.Errorf("FormColor after reset = %v, want %v", got, origForm)
	}
	if got := Theme().TextColor; got != origText {
		t.Errorf("TextColor after reset = %v, want %v", got, origText)
	}
	if got := Theme().BorderColor; got != origBorder {
		t.Errorf("BorderColor after reset = %v, want %v", got, origBorder)
	}
	if got := Theme().MenuBGColor; got != origMenuBG {
		t.Errorf("MenuBGColor after reset = %v, want %v", got, origMenuBG)
	}
}
