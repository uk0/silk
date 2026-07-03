package gui

import (
	"github.com/uk0/silk/paint"
	"testing"
)

func TestColorSchemesNotTransparent(t *testing.T) {
	schemes := []struct {
		name string
		fn   func() ColorScheme
	}{
		{"Light", LightColorScheme},
		{"Dark", DarkColorScheme},
		{"Blue", BlueColorScheme},
		{"Green", GreenColorScheme},
		{"Purple", PurpleColorScheme},
	}

	for _, s := range schemes {
		t.Run(s.name, func(t *testing.T) {
			cs := s.fn()

			if cs.Primary.A == 0 {
				t.Error("Primary color is transparent")
			}
			if cs.Background.A == 0 {
				t.Error("Background color is transparent")
			}
			if cs.TextPrimary.A == 0 {
				t.Error("TextPrimary is transparent")
			}
			if cs.TextOnPrimary.A == 0 {
				t.Error("TextOnPrimary is transparent")
			}
			if cs.Success.A == 0 {
				t.Error("Success color is transparent")
			}
			if cs.Warning.A == 0 {
				t.Error("Warning color is transparent")
			}
			if cs.Error.A == 0 {
				t.Error("Error color is transparent")
			}
		})
	}
}

func TestColorSchemeTextReadability(t *testing.T) {
	schemes := []struct {
		name string
		fn   func() ColorScheme
	}{
		{"Light", LightColorScheme},
		{"Dark", DarkColorScheme},
		{"Blue", BlueColorScheme},
		{"Green", GreenColorScheme},
		{"Purple", PurpleColorScheme},
	}

	for _, s := range schemes {
		t.Run(s.name, func(t *testing.T) {
			cs := s.fn()

			// TextPrimary should differ from Background
			if cs.TextPrimary == cs.Background {
				t.Error("TextPrimary is same as Background; text would be invisible")
			}
			// TextOnPrimary should differ from Primary
			if cs.TextOnPrimary == cs.Primary {
				t.Error("TextOnPrimary is same as Primary; text on buttons would be invisible")
			}
		})
	}
}

func TestGetColorSchemeDispatch(t *testing.T) {
	light := GetColorScheme(StyleLight)
	dark := GetColorScheme(StyleDark)
	blue := GetColorScheme(StyleBlue)
	green := GetColorScheme(StyleGreen)
	purple := GetColorScheme(StylePurple)

	// Each should return a different primary color
	if light.Primary == dark.Primary {
		t.Error("Light and Dark should have different Primary")
	}
	if blue.Primary == green.Primary {
		t.Error("Blue and Green should have different Primary")
	}
	if purple.Primary == light.Primary {
		t.Error("Purple and Light should have different Primary")
	}

	// Default should return light scheme
	def := GetColorScheme(StyleDefault)
	if def.Primary != light.Primary {
		t.Error("StyleDefault should return Light scheme")
	}
}

func TestWidgetStylePresets(t *testing.T) {
	scheme := LightColorScheme()

	tests := []struct {
		name string
		fn   func(ColorScheme) WidgetStyle
	}{
		{"ButtonPrimary", ButtonStylePrimary},
		{"ButtonSecondary", ButtonStyleSecondary},
		{"ButtonDanger", ButtonStyleDanger},
		{"ButtonSuccess", ButtonStyleSuccess},
		{"InputDefault", InputStyleDefault},
		{"CardDefault", CardStyleDefault},
		{"CardElevated", CardStyleElevated},
		{"TagPrimary", TagStylePrimary},
		{"TagOutlined", TagStyleOutlined},
		{"TagSuccess", TagStyleSuccess},
		{"TagWarning", TagStyleWarning},
		{"TagError", TagStyleError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style := tt.fn(scheme)

			// BorderRadius should be non-negative
			if style.BorderRadius < 0 {
				t.Errorf("BorderRadius = %f, should be >= 0", style.BorderRadius)
			}

			// At least some padding should be set for interactive widgets
			hasPadding := style.PaddingTop > 0 || style.PaddingRight > 0 ||
				style.PaddingBottom > 0 || style.PaddingLeft > 0
			if !hasPadding {
				t.Log("style has no padding set (may be intentional)")
			}
		})
	}
}

func TestDarkSchemeHasDarkBackground(t *testing.T) {
	dark := DarkColorScheme()
	// R, G, B each should be < 128 for a dark theme
	avg := (int(dark.Background.R) + int(dark.Background.G) + int(dark.Background.B)) / 3
	if avg > 128 {
		t.Errorf("Dark scheme background avg luminance = %d, expected < 128", avg)
	}
}

func TestLightSchemeHasLightBackground(t *testing.T) {
	light := LightColorScheme()
	avg := (int(light.Background.R) + int(light.Background.G) + int(light.Background.B)) / 3
	if avg < 128 {
		t.Errorf("Light scheme background avg luminance = %d, expected >= 128", avg)
	}
}

func TestColorSchemeFullAlpha(t *testing.T) {
	cs := LightColorScheme()

	// These functional colors should all be fully opaque
	opaqueColors := []struct {
		name string
		c    paint.Color
	}{
		{"Primary", cs.Primary},
		{"Secondary", cs.Secondary},
		{"Accent", cs.Accent},
		{"Background", cs.Background},
		{"Surface", cs.Surface},
		{"TextPrimary", cs.TextPrimary},
		{"Border", cs.Border},
		{"Success", cs.Success},
		{"Warning", cs.Warning},
		{"Error", cs.Error},
		{"Info", cs.Info},
	}

	for _, oc := range opaqueColors {
		if oc.c.A != 255 {
			t.Errorf("%s alpha = %d, want 255 (fully opaque)", oc.name, oc.c.A)
		}
	}
}
