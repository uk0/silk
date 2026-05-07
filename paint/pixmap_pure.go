//go:build silk_no_cairo

package paint

import (
	"image"
	"image/png"
	"io"
	"os"
	"unsafe"
)

// imagePixmap is the Cairo-free Pixmap implementation. Uses
// image.RGBA as the backing buffer; PNG decode/encode goes through
// the standard library. NewPainter returns a stub painter (paintNullCtx)
// because actual rendering happens through silk/glui in this build —
// callers that need to draw INTO a pixmap (theme tile generation,
// TextToPixmap helpers) silently no-op in this mode.
type imagePixmap struct {
	rgba *image.RGBA
}

// NewPixmap allocates an empty RGBA pixmap of the requested size.
// All pixels start at (0,0,0,0) (transparent).
func NewPixmap(w, h int) Pixmap {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &imagePixmap{rgba: image.NewRGBA(image.Rect(0, 0, w, h))}
}

// LoadPngFile decodes a PNG file via image/png and wraps the result
// in an imagePixmap. Non-RGBA decode results are converted to RGBA
// by the standard library wrapper.
func LoadPngFile(filename string) (Pixmap, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		// Convert to RGBA by allocating fresh and drawing.
		rgba = image.NewRGBA(img.Bounds())
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				rgba.Set(x, y, img.At(x, y))
			}
		}
	}
	return &imagePixmap{rgba: rgba}, nil
}

// --- Surface interface --------------------------------------------------

func (p *imagePixmap) SurfaceType() int { return 0 }

// NewPainter returns a no-op painter. Pure-Go callers that want to
// draw INTO the pixmap should use silk/glui through a real GL
// context; this stub exists so the Pixmap interface satisfies its
// contract without importing silk/glui (which would create a cycle).
func (p *imagePixmap) NewPainter() Painter { return newNullPainter() }

func (p *imagePixmap) Flush() {}

// --- Pixmap interface ---------------------------------------------------

func (p *imagePixmap) Format() Format { return FormatARGB32 }

func (p *imagePixmap) Width() int {
	return p.rgba.Bounds().Dx()
}

func (p *imagePixmap) Height() int {
	return p.rgba.Bounds().Dy()
}

func (p *imagePixmap) Stride() int { return p.rgba.Stride }

func (p *imagePixmap) DataPtr() unsafe.Pointer {
	if len(p.rgba.Pix) == 0 {
		return nil
	}
	return unsafe.Pointer(&p.rgba.Pix[0])
}

func (p *imagePixmap) WritePNGToStream(w io.Writer) error {
	return png.Encode(w, p.rgba)
}

func (p *imagePixmap) WritePNG(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, p.rgba)
}

func (p *imagePixmap) Image() (image.Image, error) {
	return p.rgba, nil
}

func (p *imagePixmap) SetData(src []uint8) error {
	if len(src) > len(p.rgba.Pix) {
		src = src[:len(p.rgba.Pix)]
	}
	copy(p.rgba.Pix, src)
	return nil
}

func (p *imagePixmap) SetImage(img image.Image) error {
	if rgba, ok := img.(*image.RGBA); ok && rgba.Bounds().Eq(p.rgba.Bounds()) {
		copy(p.rgba.Pix, rgba.Pix)
		return nil
	}
	// Slow path: per-pixel conversion.
	bounds := p.rgba.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			p.rgba.Set(x, y, img.At(x, y))
		}
	}
	return nil
}

// TextToPixmap is a stub in pure-Go mode. Callers that need pre-
// rendered text bitmaps should use silk/glui directly through a
// real GL context. Returns an empty pixmap so call sites don't
// segfault on a nil result.
func TextToPixmap(text string, font Font, color Color, border bool) Pixmap {
	return NewPixmap(1, 1)
}

// IconTextToPixmap is a stub in pure-Go mode for the same reason.
func IconTextToPixmap(ico Icon, icoSize float64, grayed bool, text string, font Font, textColor Color, border bool) Pixmap {
	return NewPixmap(1, 1)
}
