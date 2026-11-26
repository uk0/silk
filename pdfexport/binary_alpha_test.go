package pdfexport

import (
	"bytes"
	"compress/zlib"
	"image"
	"image/color"
	"io"
	"testing"
)

func TestPDFPixmapPreservesBinaryTransparency(t *testing.T) {
	// A cutout with hard, fully transparent/opaque edges is common in
	// exported bitmap assets. A non-zero origin must not shift its image.
	img := image.NewNRGBA(image.Rect(-3, 4, -1, 5))
	img.SetNRGBA(-2, 4, color.NRGBA{R: 34, G: 167, B: 220, A: 255})
	rgb, mask, ok := encodePixmapToFlatedRGBAndSMask(img, 2, 1)
	if !ok || len(mask) == 0 {
		t.Fatal("one fully transparent pixel was enough to need a soft mask")
	}
	if got := inflateRasterStream(t, mask); !bytes.Equal(got, []byte{0, 255}) {
		t.Fatalf("alpha = %v", got)
	}
	if got := inflateRasterStream(t, rgb); !bytes.Equal(got, []byte{255, 255, 255, 34, 167, 220}) {
		t.Fatalf("RGB = %v", got)
	}
}

func TestPDFRasterKeepsStraightColorsInStridedSubImage(t *testing.T) {
	src := image.NewNRGBA(image.Rect(10, 20, 15, 25))
	src.SetNRGBA(11, 22, color.NRGBA{R: 223, G: 13, B: 219, A: 127})
	src.SetNRGBA(12, 22, color.NRGBA{R: 239, G: 51, B: 64, A: 255})
	src.SetNRGBA(12, 23, color.NRGBA{R: 3, G: 1, B: 7, A: 255})
	view := src.SubImage(image.Rect(11, 22, 13, 24))
	rgb, mask, ok := encodePixmapToFlatedRGBAndSMask(view, 2, 2)
	if !ok || len(mask) == 0 {
		t.Fatal("subimage lost alpha")
	}
	if got := inflateRasterStream(t, mask); !bytes.Equal(got, []byte{127, 255, 0, 255}) {
		t.Fatalf("alpha = %v", got)
	}
	want := []byte{223, 13, 219, 239, 51, 64, 255, 255, 255, 3, 1, 7}
	if got := inflateRasterStream(t, rgb); !bytes.Equal(got, want) {
		t.Fatalf("RGB = %v, want %v", got, want)
	}
}

func TestPDFDrawRasterRejectsInvalidFrameBeforeCreatingXObject(t *testing.T) {
	p := New(42, 42)
	if err := p.DrawRaster(0, 0, 30, 20, nil); err == nil {
		t.Fatal("missing raster was accepted")
	}
	if err := p.DrawRaster(0, 0, 30, 20, image.NewRGBA(image.Rectangle{})); err == nil {
		t.Fatal("empty raster was accepted")
	}
	valid := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if err := p.DrawRaster(0, 0, -30, 20, valid); err == nil {
		t.Fatal("negative destination was accepted")
	}
	if len(p.images) != 0 || p.content.Len() != 0 {
		t.Fatal("rejected draw changed document")
	}
}

func inflateRasterStream(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
