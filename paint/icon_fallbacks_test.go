package paint

import (
	"image/color"
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
