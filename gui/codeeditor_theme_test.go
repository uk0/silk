package gui

import (
	"silk/paint"
	"testing"
)

// ---------------------------------------------------------------------------
// CodeEditor theme-aware palette (pure, no GL / no Draw)
//
// The editor selects a colour palette per Draw via editorColorsFor(mode). Dark
// is the default and must stay byte-identical to the pre-theme render; light is
// a readable-on-white counterpart. These tests exercise the pure selector and
// confirm construction is panic-free under each mode.
// ---------------------------------------------------------------------------

// perceivedLum returns a rough luminance (0..255) so a palette's background can
// be classified as dark (< 128) or light (> 128).
func perceivedLum(c paint.Color) float64 {
	return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
}

// TestEditorColorsForBackgrounds checks the selector returns a dark background
// for ThemeDark — byte-identical to the original {30,30,35} — and a light one
// for ThemeLight.
func TestEditorColorsForBackgrounds(t *testing.T) {
	dark := editorColorsFor(ThemeDark)
	light := editorColorsFor(ThemeLight)

	wantDarkBG := paint.Color{R: 30, G: 30, B: 35, A: 255}
	if dark.bg != wantDarkBG {
		t.Errorf("dark bg = %+v, want %+v (dark palette must stay byte-identical)", dark.bg, wantDarkBG)
	}
	if l := perceivedLum(dark.bg); l >= 128 {
		t.Errorf("dark bg luminance = %.1f, want < 128 (should be dark)", l)
	}
	if l := perceivedLum(light.bg); l <= 128 {
		t.Errorf("light bg luminance = %.1f, want > 128 (should be light)", l)
	}
	if dark.bg == light.bg {
		t.Errorf("dark and light bg are identical (%+v); light palette not applied", dark.bg)
	}
}

// TestEditorColorsForTable table-tests the selector across modes and confirms
// the caret and keyword token hue differ, so light mode is genuinely distinct.
func TestEditorColorsForTable(t *testing.T) {
	cases := []struct {
		name   string
		mode   ThemeMode
		darkBG bool // true => bg should be dark
	}{
		{"dark", ThemeDark, true},
		{"light", ThemeLight, false},
		// Any non-dark mode falls through to the light palette.
		{"default-light", ThemeLight, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pal := editorColorsFor(tc.mode)
			if pal == nil {
				t.Fatalf("editorColorsFor(%v) = nil", tc.mode)
			}
			if isDark := perceivedLum(pal.bg) < 128; isDark != tc.darkBG {
				t.Errorf("mode %v: bg %+v isDark=%v, want darkBG=%v", tc.mode, pal.bg, isDark, tc.darkBG)
			}
			if pal.tokens[tokKeyword] == (paint.Color{}) {
				t.Errorf("mode %v: keyword token colour is zero", tc.mode)
			}
		})
	}

	// The syntax hues and caret must differ between the two modes, otherwise the
	// light theme would render dark-mode colours on a white background.
	d := editorColorsFor(ThemeDark)
	l := editorColorsFor(ThemeLight)
	if d.tokens[tokKeyword] == l.tokens[tokKeyword] {
		t.Errorf("keyword hue identical across modes (%+v); light syntax set not applied", d.tokens[tokKeyword])
	}
	if d.caret == l.caret {
		t.Errorf("caret identical across modes (%+v)", d.caret)
	}
	// Dark palette must reuse the original tokenColors map, unchanged.
	if d.tokens[tokKeyword] != tokenColors[tokKeyword] {
		t.Errorf("dark keyword hue %+v != tokenColors[tokKeyword] %+v", d.tokens[tokKeyword], tokenColors[tokKeyword])
	}
}

// TestCodeEditorThemeConstructNoPanic constructs an editor and sets text under
// each theme mode; neither the state layer nor palette selection should panic.
// The original mode is restored via t.Cleanup.
func TestCodeEditorThemeConstructNoPanic(t *testing.T) {
	orig := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(orig) })

	for _, mode := range []ThemeMode{ThemeDark, ThemeLight} {
		SetThemeMode(mode)
		e := NewCodeEditor()
		e.SetText("package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n")
		if got := editorColorsFor(CurrentThemeMode()); got == nil {
			t.Fatalf("editorColorsFor returned nil for mode %v", mode)
		}
	}
}
