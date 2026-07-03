package pdfexport

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

// makeAlphaPixmap builds a 4×4 paint.Pixmap with 50% alpha throughout.
// Used to drive the SMask code path: pixmaps with any pixel below
// fully-opaque alpha trigger the SMask companion XObject.
func makeAlphaPixmap(t *testing.T) paint.Pixmap {
	t.Helper()
	pix := paint.NewPixmap(4, 4)
	if pix == nil {
		t.Fatal("paint.NewPixmap returned nil")
	}
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			// Half-transparent red. SetImage in pure-Go backend
			// stores straight RGBA; in cairo it premultiplies. Either
			// way the resulting pixmap.Image() reports alpha < 255.
			src.Set(x, y, color.RGBA{R: 255, A: 128})
		}
	}
	if err := pix.SetImage(src); err != nil {
		t.Fatalf("pixmap.SetImage: %v", err)
	}
	return pix
}

// TestOpaquePixmapEmitsNoSMask: an image with every pixel alpha=255
// should NOT generate a soft mask XObject. Tests the no-regression
// path: the existing 4×4 chequerboard pixmap from the prior test
// suite has alpha=255, so behaviour stays identical.
func TestOpaquePixmapEmitsNoSMask(t *testing.T) {
	pix := makeTestPixmap(t) // chequerboard, fully opaque
	p := New(200, 200)
	p.DrawPixmap1(0, 0, pix)

	out := p.String()
	if strings.Contains(out, "/ColorSpace /DeviceGray") {
		t.Errorf("opaque pixmap should not produce a /DeviceGray SMask XObject:\n%s", out)
	}
	if strings.Contains(out, "/SMask ") {
		t.Errorf("opaque image dict should not carry /SMask:\n%s", out)
	}
}

// TestTranslucentPixmapEmitsSMask: a 50%-alpha pixmap should produce
// a second XObject with /ColorSpace /DeviceGray, and the main image's
// dict should reference it via /SMask N 0 R.
func TestTranslucentPixmapEmitsSMask(t *testing.T) {
	pix := makeAlphaPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(0, 0, pix)

	out := p.String()
	for _, want := range []string{
		"/ColorSpace /DeviceGray",
		"/SMask 7 0 R", // image at id 6, smask at id 7 for a single-page no-prior-images doc
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n----\n%s", want, out)
		}
	}
}

// TestSMaskSizeMatchesImageDimensions: the SMask XObject must carry
// /Width and /Height matching the main image. Mismatch breaks the
// rasteriser's alpha lookup.
func TestSMaskSizeMatchesImageDimensions(t *testing.T) {
	pix := makeAlphaPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(0, 0, pix)

	out := p.String()
	// Both the main image and the SMask have "/Width 4 /Height 4".
	if got := strings.Count(out, "/Width 4 /Height 4"); got != 2 {
		t.Errorf("expected 2 '/Width 4 /Height 4' occurrences (main + SMask); got %d\n%s",
			got, out)
	}
}

// TestSMaskAddsObjectToXrefCount: a translucent pixmap takes 2
// objects (main image + SMask) instead of 1, so trailer /Size grows
// accordingly. With base 5 + 2 = 7 objects, /Size = 8.
func TestSMaskAddsObjectToXrefCount(t *testing.T) {
	pix := makeAlphaPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(0, 0, pix)

	out := p.String()
	// Single page (5 base objects: Catalog, Pages, Page, Contents, Font)
	// + 1 main image + 1 SMask = 7 objects → /Size 8.
	if !strings.Contains(out, "/Size 8 /Root 1 0 R") {
		t.Errorf("trailer /Size should be 8 for one alpha pixmap; got:\n%s", out)
	}
}

// TestXrefAccurateAfterSMask: each xref entry must point at the
// literal "N 0 obj" header, including the new SMask object.
func TestXrefAccurateAfterSMask(t *testing.T) {
	pix := makeAlphaPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(0, 0, pix)
	out := p.String()

	xrefIdx := strings.Index(out, "xref\n")
	cur := xrefIdx + len("xref\n")
	nl := strings.IndexByte(out[cur:], '\n')
	cur += nl + 1
	cur += 20 // free-list entry

	for i := 1; i <= 7; i++ {
		entry := out[cur : cur+20]
		cur += 20
		off := 0
		for j := 0; j < 10; j++ {
			off = off*10 + int(entry[j]-'0')
		}
		want := []byte{'0' + byte(i), ' ', '0', ' ', 'o', 'b', 'j'}
		if off+len(want) > len(out) || string(out[off:off+len(want)]) != string(want) {
			t.Errorf("xref entry %d offset %d does not point at %q (saw %q)",
				i, off, string(want),
				out[off:min(off+len(want), len(out))])
		}
	}
}

// TestMixedOpaqueAndTranslucentImages: opaque image consumes 1
// object, translucent consumes 2. After two pixmaps we should have
// objects: 5 base + 1 (opaque image) + 2 (translucent main + smask) = 8.
func TestMixedOpaqueAndTranslucentImages(t *testing.T) {
	opaque := makeTestPixmap(t)
	alpha := makeAlphaPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(0, 0, opaque)
	p.DrawPixmap1(50, 50, alpha)

	out := p.String()
	if !strings.Contains(out, "/Size 9 /Root 1 0 R") {
		t.Errorf("expected /Size 9 for opaque+translucent pair; got:\n%s", out)
	}
	// Image 1 (opaque) should NOT carry /SMask.
	// Image 2 (alpha) should carry /SMask 8 0 R (main is at id 7,
	// smask at id 8 because: 5 base + Im1 at 6 + Im2 main at 7 + Im2 smask at 8).
	if !strings.Contains(out, "/SMask 8 0 R") {
		t.Errorf("translucent image should reference SMask object 8\n%s", out)
	}
	// Exactly one /DeviceGray (the alpha image's SMask).
	if got := strings.Count(out, "/ColorSpace /DeviceGray"); got != 1 {
		t.Errorf("expected exactly 1 /DeviceGray (only the translucent image's SMask); got %d\n%s",
			got, out)
	}
}
