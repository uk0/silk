//go:build !silk_pure_go

package cairo

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"testing"
	"unsafe"
)

func TestSurfaceSetImageGenericSources(t *testing.T) {
	nrgba := image.NewNRGBA(image.Rect(10, 20, 13, 22))
	nrgba.SetNRGBA(10, 20, color.NRGBA{R: 240, G: 120, B: 60, A: 128})
	nrgba.SetNRGBA(11, 20, color.NRGBA{R: 20, G: 80, B: 200, A: 255})
	gray := image.NewGray(image.Rect(4, 5, 7, 7))
	gray.SetGray(4, 5, color.Gray{Y: 170})
	nrgbaParent := image.NewNRGBA(image.Rect(0, 0, 12, 12))
	nrgbaParent.SetNRGBA(5, 7, color.NRGBA{R: 110, G: 60, B: 20, A: 175})
	nrgbaSub := nrgbaParent.SubImage(image.Rect(5, 7, 8, 9))
	rgba := image.NewRGBA(image.Rect(0, 0, 8, 8))
	rgba.SetRGBA(3, 4, color.RGBA{R: 70, G: 20, B: 10, A: 100})
	sub := rgba.SubImage(image.Rect(3, 4, 6, 6))
	for name, src := range map[string]image.Image{"nrgba": nrgba, "gray": gray, "strided-subimage": sub, "strided-nrgba": nrgbaSub} {
		t.Run(name, func(t *testing.T) {
			s := NewImageSurface(FORMAT_ARGB32, 3, 2)
			defer s.Destroy()
			if err := s.SetImage(src); err != nil {
				t.Fatal(err)
			}
			got, err := s.Image()
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < 2; y++ {
				for x := 0; x < 3; x++ {
					want := color.RGBAModel.Convert(src.At(src.Bounds().Min.X+x, src.Bounds().Min.Y+y))
					if got.At(x, y) != want {
						t.Fatalf("pixel %d,%d = %v, want %v", x, y, got.At(x, y), want)
					}
				}
			}
		})
	}
}

func TestSurfaceSetImageFormatsAndCrop(t *testing.T) {
	for _, format := range []Format{FORMAT_ARGB32, FORMAT_RGB24, FORMAT_A8} {
		t.Run(map[Format]string{FORMAT_ARGB32: "argb", FORMAT_RGB24: "rgb", FORMAT_A8: "alpha"}[format], func(t *testing.T) {
			s := NewImageSurface(format, 3, 2)
			defer s.Destroy()
			// A8 rows are padded by Cairo to four-byte alignment.
			initial := bytes.Repeat([]byte{0x66}, s.Stride()*s.Height())
			if err := s.SetData(initial); err != nil {
				t.Fatal(err)
			}
			src := image.NewRGBA(image.Rect(8, 9, 10, 10))
			src.SetRGBA(8, 9, color.RGBA{R: 90, G: 60, B: 30, A: 128})
			src.SetRGBA(9, 9, color.RGBA{R: 10, G: 20, B: 30, A: 255})
			before := append([]byte(nil), src.Pix...)
			if err := s.SetImage(src); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(src.Pix, before) {
				t.Fatal("source was modified")
			}
			s.Flush()
			raw := unsafe.Slice((*byte)(s.DataPtr()), s.Stride()*s.Height())
			pixelSize := 4
			if format == FORMAT_A8 {
				pixelSize = 1
				if raw[0] != 128 || raw[1] != 255 {
					t.Fatalf("wrong alpha: %v", raw[:2])
				}
			} else {
				a := uint32(128)
				if format == FORMAT_RGB24 {
					a = 255
				}
				if got := binary.NativeEndian.Uint32(raw); got != a<<24|90<<16|60<<8|30 {
					t.Fatalf("wrong native pixel: %08x", got)
				}
			}
			if !bytes.Equal(raw[2*pixelSize:], initial[2*pixelSize:]) {
				t.Fatal("pixels or padding outside source overwritten")
			}
		})
	}
	// Source larger than destination, with nonzero origin, is cropped.
	s := NewImageSurface(FORMAT_ARGB32, 1, 1)
	defer s.Destroy()
	src := image.NewNRGBA(image.Rect(-10, -20, -5, -15))
	src.SetNRGBA(-10, -20, color.NRGBA{R: 70, G: 20, B: 10, A: 255})
	if err := s.SetImage(src); err != nil {
		t.Fatal(err)
	}
	got, err := s.Image()
	if err != nil {
		t.Fatal(err)
	}
	if got.At(0, 0) != (color.RGBA{R: 70, G: 20, B: 10, A: 255}) {
		t.Fatal("wrong crop origin")
	}
}

func TestSurfaceSetImageUnsupportedFormat(t *testing.T) {
	s := NewImageSurface(FORMAT_A1, 4, 4)
	defer s.Destroy()
	if err := s.SetImage(image.NewRGBA(image.Rect(0, 0, 4, 4))); err == nil {
		t.Fatal("unsupported packed format accepted")
	}
	empty := NewImageSurface(FORMAT_ARGB32, 0, 0)
	defer empty.Destroy()
	if err := empty.SetImage(image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
}

func TestSurfaceWrittenPixelsAreVisibleWithoutCallerMarkDirty(t *testing.T) {
	for _, raw := range []bool{false, true} {
		t.Run(map[bool]string{false: "image", true: "data"}[raw], func(t *testing.T) {
			s := NewImageSurface(FORMAT_ARGB32, 2, 2)
			defer s.Destroy()
			img := image.NewRGBA(image.Rect(0, 0, 2, 2))
			for y := 0; y < 2; y++ {
				for x := 0; x < 2; x++ {
					img.SetRGBA(x, y, color.RGBA{R: 30, G: 80, B: 200, A: 255})
				}
			}
			if raw {
				pixels := make([]byte, len(img.Pix))
				for i := 0; i < len(pixels); i += 4 {
					binary.NativeEndian.PutUint32(pixels[i:], 0xff1e50c8)
				}
				if err := s.SetData(pixels); err != nil {
					t.Fatal(err)
				}
			} else if err := s.SetImage(img); err != nil {
				t.Fatal(err)
			}
			dst := NewImageSurface(FORMAT_ARGB32, 2, 2)
			defer dst.Destroy()
			c := dst.NewContext()
			defer c.Destroy()
			c.SetSourceSurface(s, 0, 0)
			c.Paint()
			got, err := dst.Image()
			if err != nil {
				t.Fatal(err)
			}
			if got.At(0, 0) != (color.RGBA{R: 30, G: 80, B: 200, A: 255}) {
				t.Fatalf("written pixels not painted: %v", got.At(0, 0))
			}
		})
	}
}

func TestSurfaceEmptyAndNilTransfers(t *testing.T) {
	s := NewImageSurface(FORMAT_ARGB32, 2, 2)
	defer s.Destroy()
	if err := s.SetData([]byte{}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetData(nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetImage(nil); err == nil {
		t.Fatal("nil image accepted")
	}
}

func BenchmarkSurfaceSetImageRGBA(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	s := NewImageSurface(FORMAT_ARGB32, 1024, 768)
	defer s.Destroy()
	b.ReportAllocs()
	b.SetBytes(int64(len(img.Pix)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.SetImage(img); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSurfaceImageOwnsPixels(t *testing.T) {
	s := NewImageSurface(FORMAT_ARGB32, 2, 2)
	defer s.Destroy()
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.SetRGBA(0, 0, color.RGBA{R: 70, G: 20, B: 10, A: 255})
	if err := s.SetImage(src); err != nil {
		t.Fatal(err)
	}
	got, err := s.Image()
	if err != nil {
		t.Fatal(err)
	}
	got.(*image.RGBA).SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	again, err := s.Image()
	if err != nil {
		t.Fatal(err)
	}
	if again.At(0, 0) != src.At(0, 0) {
		t.Fatal("exported image aliases Cairo surface")
	}
}

func BenchmarkSurfaceImageRGBA(b *testing.B) {
	s := NewImageSurface(FORMAT_ARGB32, 1024, 768)
	defer s.Destroy()
	b.ReportAllocs()
	b.SetBytes(int64(1024 * 768 * 4))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Image(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSurfaceSetImageNRGBA(b *testing.B) {
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 768))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	s := NewImageSurface(FORMAT_ARGB32, 1024, 768)
	defer s.Destroy()
	b.ReportAllocs()
	b.SetBytes(int64(len(img.Pix)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.SetImage(img); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSurfaceNRGBAPremultiplicationMatchesGoColor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(2, 3, 258, 259))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			img.SetNRGBA(x+2, y+3, color.NRGBA{R: byte(x), G: byte(255 - x), B: 80, A: byte(y)})
		}
	}
	s := NewImageSurface(FORMAT_ARGB32, 256, 256)
	defer s.Destroy()
	if err := s.SetImage(img); err != nil {
		t.Fatal(err)
	}
	got, err := s.Image()
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			want := color.RGBAModel.Convert(img.At(x+2, y+3))
			if got.At(x, y) != want {
				t.Fatalf("color=%d alpha=%d got %v want %v", x, y, got.At(x, y), want)
			}
		}
	}
}
