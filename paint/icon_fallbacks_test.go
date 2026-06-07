package paint

import (
	"image/color"
	"reflect"
	"testing"
)

// TestProceduralFallbackPaintsSomething: a known fallback name
// (edit-undo) loads cleanly and yields a non-blank pixmap. Without
// the fallback wiring the loader logs "icon not found" and returns
// the red-X image-missing placeholder; this test pins the routing
// so a regression at LoadIcon's switch wouldn't silently fall back
// to the wrong asset.
func TestProceduralFallbackPaintsSomething(t *testing.T) {
	for _, name := range []string{
		"edit-undo",
		"edit-redo",
		"close-btn",
		"checkbox-checked",
		"checkbox-unchecked",
		"expander-collapsed",
		"expander-expanded",
		"arrow-tool",
		"rect-tool",
		"menu",
		"refresh",
		"plus",
		"search",
	} {
		t.Run(name, func(t *testing.T) {
			ic := LoadIcon(name)
			if ic == nil {
				t.Fatalf("%s: LoadIcon returned nil", name)
			}
			pix := ic.Pixmap(16)
			if pix == nil {
				t.Fatalf("%s: Pixmap(16) returned nil", name)
			}
			img, err := pix.Image()
			if err != nil {
				t.Fatalf("%s: Image: %v", name, err)
			}
			// Walk the surface and confirm at least one painted pixel.
			// The red-X fallback paints too, but we don't need to
			// distinguish here — what we're guarding against is "the
			// drawer drew nothing because of a sign/order bug".
			var painted int
			b := img.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					_, _, _, a := color.RGBAModel.Convert(img.At(x, y)).RGBA()
					if a > 0 {
						painted++
					}
				}
			}
			if painted == 0 {
				t.Errorf("%s fallback rendered 0 painted pixels", name)
			}
		})
	}
}

// TestUnknownNameStillFallsToImageMissing: an icon name not in the
// fallback table should still route to image-missing so the user
// gets a visible "something is wrong" hint instead of an invisible
// hole. Pre-existing behaviour kept intact.
func TestUnknownNameStillFallsToImageMissing(t *testing.T) {
	ic := LoadIcon("definitely-not-a-real-icon-name-123")
	if ic == nil {
		t.Fatal("unknown name returned nil")
	}
	// `Icon` interface doesn't expose the name, but `*icon` does;
	// type-assert through to verify the unknown-name branch routed
	// to image-missing.
	cast, ok := ic.(*icon)
	if !ok {
		t.Fatalf("unexpected concrete type %T", ic)
	}
	if cast.name != "image-missing" {
		t.Errorf("unknown name resolved to %q, want image-missing", cast.name)
	}
}

// newIconNames lists every name added on top of the original fallback
// set (semantic IDE icons + their aliases). Each must resolve through
// the procedural path rather than the red-X placeholder.
var newIconNames = []string{
	"build",
	"debug",
	"stop",
	"continue",
	"play",
	"step-over",
	"step-into",
	"step-out",
	"warning",
	"git-branch",
	"git",
	"terminal",
	"go-file",
	"file",
	"function",
	"gear",
	"settings",
	"folder-open",
}

// TestNewFallbacksRegistered: every new name is present in the
// procedural registry, so LoadIcon hits genProceduralIcon instead of
// logging "icon not found" and falling through to image-missing.
func TestNewFallbacksRegistered(t *testing.T) {
	for _, name := range newIconNames {
		if proceduralFallbacks[name] == nil {
			t.Errorf("%q missing from proceduralFallbacks registry", name)
		}
	}
}

// TestNewFallbackAliases: alias pairs share the same drawer so the
// alternate spelling renders the identical glyph. Func values aren't
// `==`-comparable, so compare the underlying code pointer.
func TestNewFallbackAliases(t *testing.T) {
	pairs := [][2]string{
		{"play", "continue"},
		{"git", "git-branch"},
		{"gear", "settings"},
		{"file", "go-file"},
	}
	for _, p := range pairs {
		a := proceduralFallbacks[p[0]]
		b := proceduralFallbacks[p[1]]
		if a == nil || b == nil {
			t.Errorf("alias pair %q/%q: one side missing from registry", p[0], p[1])
			continue
		}
		if reflect.ValueOf(a).Pointer() != reflect.ValueOf(b).Pointer() {
			t.Errorf("alias %q does not point to the same drawer as %q", p[0], p[1])
		}
	}
}

// TestNewFallbacksResolveViaLoadIcon: each new name routes to the
// procedural branch — i.e. LoadIcon returns an *icon whose name is the
// requested one (not "image-missing"). This exercises the same lookup
// LoadIcon's switch performs and guards the wiring end-to-end.
func TestNewFallbacksResolveViaLoadIcon(t *testing.T) {
	for _, name := range newIconNames {
		t.Run(name, func(t *testing.T) {
			ic := LoadIcon(name)
			if ic == nil {
				t.Fatalf("%s: LoadIcon returned nil", name)
			}
			cast, ok := ic.(*icon)
			if !ok {
				t.Fatalf("%s: unexpected concrete type %T", name, ic)
			}
			if cast.name == "image-missing" {
				t.Errorf("%s routed to image-missing; expected procedural fallback", name)
			}
		})
	}
}

// TestNewFallbackDrawNoPanic: a smoke test that each new drawer runs
// against a real Cairo surface at 16 and 32 without panicking. Mirrors
// the surface-creation idiom used by the other paint tests
// (NewPixmap(...).NewContext() — see icon_fallbacks.go's
// genProceduralIcon and paint_test.go).
func TestNewFallbackDrawNoPanic(t *testing.T) {
	// Deduplicate so aliases don't run the same drawer twice; the set
	// of distinct drawers is what we actually want to smoke.
	seen := map[uintptr]bool{}
	for _, name := range newIconNames {
		draw := proceduralFallbacks[name]
		if draw == nil {
			t.Fatalf("%q missing from registry", name)
		}
		ptr := reflect.ValueOf(draw).Pointer()
		if seen[ptr] {
			continue
		}
		seen[ptr] = true
		for _, size := range []int{16, 32} {
			s := NewPixmap(size, size)
			cc := s.NewContext()
			draw(size, cc)
		}
	}
}
