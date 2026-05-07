package paint

import (
	"image"
	"io"
	"unsafe"
)

// Format identifies the pixel layout of a Pixmap. The values
// align with cairo_format_t for backward compatibility with the
// Cairo-backed implementation; the silk_no_cairo path uses the
// same values so call sites don't have to switch on backend.
type Format int

const (
	FormatInvalid Format = -1
	FormatARGB32         = 0
	FormatRGB24          = 1
)

// Pixmap is the interface every drawable image surface implements.
// Two concrete types live in this package:
//
//   - cairoSurface (build tag !silk_no_cairo) — wraps cairo.Surface
//   - imagePixmap  (build tag silk_no_cairo)  — wraps image.RGBA
//
// Callers MUST NOT type-assert to either concrete type — use the
// interface methods. NewPixmap / LoadPngFile return Pixmap so the
// build tag can pick the implementation transparently.
type Pixmap interface {
	Surface
	Format() Format
	Width() int
	Height() int
	Stride() int
	DataPtr() unsafe.Pointer
	WritePNGToStream(w io.Writer) error
	WritePNG(filename string) error
	Image() (image.Image, error)
	SetData(src []uint8) error
	SetImage(img image.Image) error
}
