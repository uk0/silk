package pdfexport

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

// makeTestPixmap returns a 4×4 paint.Pixmap with a chequerboard
// red/blue pattern. Reused across both export packages — keeps the
// per-package test helper small.
func makeTestPixmap(t *testing.T) paint.Pixmap {
	t.Helper()
	pix := paint.NewPixmap(4, 4)
	if pix == nil {
		t.Fatal("paint.NewPixmap returned nil")
	}
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if (x+y)%2 == 0 {
				src.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				src.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}
	if err := pix.SetImage(src); err != nil {
		t.Fatalf("pixmap.SetImage: %v", err)
	}
	return pix
}

// TestDrawPixmapAppendsXObjectAndDoOperator: a single DrawPixmap1
// should produce one /XObject /Subtype /Image entry plus a "/Im1 Do"
// operator inside the content stream.
func TestDrawPixmapAppendsXObjectAndDoOperator(t *testing.T) {
	pix := makeTestPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(20, 30, pix)

	out := p.String()
	for _, want := range []string{
		"/Type /XObject /Subtype /Image",
		"/Width 4 /Height 4",
		"/ColorSpace /DeviceRGB /BitsPerComponent 8",
		"/Filter /FlateDecode",
		"/Im1 Do",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n----\n%s", want, out)
		}
	}
}

// TestDrawPixmapPageResourcesContainXObjectDict: the page's /Resources
// dictionary must include a /XObject sub-dict referencing the new
// image XObject by name. Without this, "Do" operators target an
// undefined name and the page renders blank.
func TestDrawPixmapPageResourcesContainXObjectDict(t *testing.T) {
	pix := makeTestPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(20, 30, pix)

	out := p.String()
	if !strings.Contains(out, "/XObject << /Im1 6 0 R >>") {
		t.Errorf("page Resources should reference the image XObject; got:\n%s", out)
	}
}

// TestDrawMultiplePixmapsAddsSequentialNames: each DrawPixmap call
// produces a new XObject Im1, Im2, ... reference. Resource dict must
// list them all.
func TestDrawMultiplePixmapsAddsSequentialNames(t *testing.T) {
	pix := makeTestPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(0, 0, pix)
	p.DrawPixmap1(50, 50, pix)
	p.DrawPixmap1(100, 100, pix)

	out := p.String()
	for _, want := range []string{"/Im1 ", "/Im2 ", "/Im3 "} {
		if !strings.Contains(out, want) {
			t.Errorf("missing image name %q\n----\n%s", want, out)
		}
	}
	for _, want := range []string{"/Im1 Do", "/Im2 Do", "/Im3 Do"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in content stream\n----\n%s", want, out)
		}
	}
}

// TestXrefOffsetsAccurateAfterImages: adding image XObjects must not
// break the xref table. Each "<offset> 00000 n" entry must point at
// the literal "N 0 obj" header.
func TestXrefOffsetsAccurateAfterImages(t *testing.T) {
	pix := makeTestPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(0, 0, pix)
	p.DrawPixmap1(50, 50, pix)
	out := p.String()

	xrefIdx := strings.Index(out, "xref\n")
	if xrefIdx < 0 {
		t.Fatal("no xref")
	}

	// Skip "xref\n" + count line + free-list entry.
	cur := xrefIdx + len("xref\n")
	// Count line: "0 N\n" — read N then advance past it.
	nl := strings.IndexByte(out[cur:], '\n')
	if nl < 0 {
		t.Fatalf("malformed xref count line")
	}
	cur += nl + 1
	cur += 20 // free-list entry

	// Now we have N entries (5 base + 2 images = 7). Verify each
	// offset points at the right "N 0 obj" header.
	for i := 1; i <= 7; i++ {
		entry := out[cur : cur+20]
		cur += 20
		off := 0
		for j := 0; j < 10; j++ {
			off = off*10 + int(entry[j]-'0')
		}
		// The header at the offset should start with "i 0 obj" (single-
		// or multi-digit). For i ≤ 9 the prefix is "i 0 obj" (7 bytes);
		// keep this test bounded to single-digit i for simplicity since
		// we're checking the first 7 objects.
		want := []byte{'0' + byte(i), ' ', '0', ' ', 'o', 'b', 'j'}
		if off+len(want) > len(out) || string(out[off:off+len(want)]) != string(want) {
			t.Errorf("xref entry %d offset %d does not point at %q (saw %q)",
				i, off, string(want), out[off:min(off+len(want), len(out))])
		}
	}
}

// TestSizeInTrailerMatchesObjectCount: trailer /Size must equal the
// total object count (objects + 1 for slot 0).
func TestSizeInTrailerMatchesObjectCount(t *testing.T) {
	pix := makeTestPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(0, 0, pix)
	p.DrawPixmap1(50, 50, pix)

	out := p.String()
	// 5 base + 2 images = 7 objects → /Size 8.
	if !strings.Contains(out, "/Size 8 /Root 1 0 R") {
		t.Errorf("trailer should record /Size 8 (5 base + 2 image objects + slot 0); got:\n%s", out)
	}
}

// TestDrawPixmapEmitsCMTransform: the cm matrix in the content stream
// should encode (w, 0, 0, h, x, y_after_flip).
func TestDrawPixmapEmitsCMTransform(t *testing.T) {
	const W, H = 200.0, 200.0
	pix := makeTestPixmap(t)
	p := New(W, H)
	// Place image at top-left (0, 0) with size 50×50.
	// After Y-flip: image's bottom-left in PDF coords = (0, H - 0 - 50) = (0, 150).
	// Expected cm: "50 0 0 50 0 150 cm".
	p.DrawPixmap5(0, 0, 50, 50, pix)

	out := p.String()
	if !strings.Contains(out, "50 0 0 50 0 150 cm") {
		t.Errorf("DrawPixmap5(0,0,50,50) should emit cm '50 0 0 50 0 150'; got:\n%s", out)
	}
}

// TestNilPixmapIsNoOp guards nil pixmaps from crashing the painter.
func TestNilPixmapIsNoOp(t *testing.T) {
	p := New(200, 200)
	p.DrawPixmap(nil)
	p.DrawPixmap1(0, 0, nil)
	p.DrawPixmap5(0, 0, 50, 50, nil)
	out := p.String()
	if strings.Contains(out, "/XObject /Subtype /Image") {
		t.Errorf("nil pixmap should not produce an image XObject:\n%s", out)
	}
}
