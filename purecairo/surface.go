package purecairo

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"unsafe"
)

// Surface mirrors cairo_surface_t. silk uses image surfaces almost
// exclusively (window back-buffer, off-screen pixmaps); we back it
// with image.RGBA for the rasteriser to write into.
//
// Byte-order bridge: silk's window upload path reads DataPtr() as
// Cairo ARGB32 — which on little-endian machines lands as BGRA in
// memory. Go's image.RGBA stores RGBA. To keep Window.paint()
// unchanged we maintain a parallel `dataBGRA` slice that mirrors
// img.Pix with R↔B swapped; Flush() rebuilds it. DataPtr() returns
// dataBGRA, which Window.paint() then ships to gl.TexImage2D as
// gl.BGRA without converting per-channel.
//
// Reference: cairo/src/cairo-image-surface.c (CAIRO_FORMAT_ARGB32
// pixel layout). cairo's native format on little-endian platforms is
// already BGRA byte order, so the byte-swap loop here matches what
// libcairo would have produced internally.
type Surface struct {
	img      *image.RGBA
	dataBGRA []byte
	format   Format
	width    int
	height   int
}

// NewImageSurface creates an image-backed surface of the given format.
// FORMAT_ARGB32 is the dominant case in silk.
func NewImageSurface(format Format, width, height int) *Surface {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	return &Surface{
		img:    image.NewRGBA(image.Rect(0, 0, width, height)),
		format: format,
		width:  width,
		height: height,
	}
}

// NewImageSurfaceFromPNG decodes a PNG from disk into a fresh surface.
func NewImageSurfaceFromPNG(filename string) (*Surface, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return NewImageSurfaceFromPNGStream(f)
}

// NewImageSurfaceFromPNGStream decodes a PNG from any reader.
func NewImageSurfaceFromPNGStream(r io.Reader) (*Surface, error) {
	img, err := png.Decode(r)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	return &Surface{
		img:    rgba,
		format: FORMAT_ARGB32,
		width:  b.Dx(),
		height: b.Dy(),
	}, nil
}

// NewWin is a no-op stub for windows-specific window surfaces — silk's
// glfw path doesn't use it under silk_pure_go.
func NewWin(unused unsafe.Pointer, w, h int) *Surface {
	return NewImageSurface(FORMAT_ARGB32, w, h)
}

func (this *Surface) Format() Format    { return this.format }
func (this *Surface) Width() int        { return this.width }
func (this *Surface) Height() int       { return this.height }
func (this *Surface) Stride() int       { return this.img.Stride }
func (this *Surface) Type() SurfaceType { return SURFACE_TYPE_IMAGE }
func (this *Surface) Status() Status    { return STATUS_SUCCESS }
func (this *Surface) Destroy()          {}
func (this *Surface) MarkDirty()        {}

// Flush rebuilds dataBGRA from img.Pix (R↔B swapped). Called every
// frame by Window.paint() right before DataPtr() / TexImage2D, so
// the GL upload sees Cairo-compatible BGRA byte order. Rebuilding
// is O(W*H) but a 1400×900 frame is ~5MB — well below the GPU's
// per-frame texture upload bandwidth.
func (this *Surface) Flush() {
	src := this.img.Pix
	if cap(this.dataBGRA) < len(src) {
		this.dataBGRA = make([]byte, len(src))
	}
	this.dataBGRA = this.dataBGRA[:len(src)]
	for i := 0; i+3 < len(src); i += 4 {
		this.dataBGRA[i] = src[i+2]   // B = R from RGBA
		this.dataBGRA[i+1] = src[i+1] // G stays
		this.dataBGRA[i+2] = src[i]   // R = B from RGBA
		this.dataBGRA[i+3] = src[i+3] // A stays
	}
}

// Image returns the surface as a Go image.Image. Match Pixmap
// interface signature expected by silk/paint: (image.Image, error).
func (this *Surface) Image() (image.Image, error) { return this.img, nil }

// RGBA returns the underlying *image.RGBA directly. Used by purego
// rasteriser internals.
func (this *Surface) RGBA() *image.RGBA { return this.img }

// DataPtr returns the Cairo-compatible BGRA byte buffer for GL upload.
// Lazy-rebuilt by Flush — call Flush before DataPtr if Drawing has
// happened since the last frame.
func (this *Surface) DataPtr() unsafe.Pointer {
	if this.dataBGRA == nil || len(this.dataBGRA) == 0 {
		this.Flush()
	}
	return unsafe.Pointer(&this.dataBGRA[0])
}

// NewContext allocates a drawing context for this surface. The Context
// holds rasteriser state + transform stack.
func (this *Surface) NewContext() *Context {
	c := &Context{
		surface: this,
	}
	c.state.matrix.InitIdentity()
	c.state.source = color.RGBA{R: 0, G: 0, B: 0, A: 255}
	c.state.lineWidth = 2
	c.state.miter = 10
	c.state.operator = OPERATOR_OVER
	return c
}

// WritePNG saves the surface as PNG. Standard library png.Encode is
// the obvious choice — no need for libcairo's writer.
func (this *Surface) WritePNG(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, this.img)
}

// WritePNGToStream writes the surface as PNG to any io.Writer.
func (this *Surface) WritePNGToStream(w io.Writer) error {
	return png.Encode(w, this.img)
}

// SetData copies the raw pixel byte stream into the surface backing
// store. silk uses this to wrap caller-allocated framebuffers.
func (this *Surface) SetData(src []uint8) error {
	if len(src) > len(this.img.Pix) {
		return errors.New("cairo.Surface.SetData: source too large")
	}
	copy(this.img.Pix, src)
	return nil
}

// SetImage copies an image.Image into the surface. Used by silk's
// TextToPixmap / IconTextToPixmap helpers.
func (this *Surface) SetImage(img image.Image) error {
	if img == nil {
		return errors.New("cairo.Surface.SetImage: nil image")
	}
	draw.Draw(this.img, this.img.Bounds(), img, img.Bounds().Min, draw.Src)
	return nil
}

// MappedImage is a lightweight handle returned by MapToImage —
// purecairo only ever maps to the current surface, so the type is a
// pass-through.
type MappedImage struct {
	Surface *Surface
}

func (this *Surface) MapToImage(extents *RectangleInt) *MappedImage {
	return &MappedImage{Surface: this}
}

func (this *Surface) UnmapImage(im *MappedImage)              {}
func (this *Surface) CreateForRectangle(r Rectangle) *Surface { return this }
func (this *Surface) CreateSimilar(c Content, w, h int) *Surface {
	return NewImageSurface(this.format, w, h)
}
func (this *Surface) CreateSimilarImage(f Format, w, h int) *Surface {
	return NewImageSurface(f, w, h)
}
func (this *Surface) Content() Content                           { return CONTENT_COLOR_ALPHA }
func (this *Surface) DeviceOffset() (x, y float64)               { return 0, 0 }
func (this *Surface) SetDeviceOffset(x, y float64)               {}
func (this *Surface) Device() *Device                            { return nil }
func (this *Surface) HasShowTextGlyphs() bool                    { return false }
func (this *Surface) FallbackResolution() (xppi, yppi float64)   { return 72, 72 }
func (this *Surface) SetFallbackResolution(xppi, yppi float64)   {}
func (this *Surface) CopyPage()                                  {}
func (this *Surface) ShowPage()                                  {}
func (this *Surface) FontOptions() *FontOptions                  { return NewFontOptions() }
func (this *Surface) GetMimeData(mimeType string) ([]byte, bool) { return nil, false }
func (this *Surface) SetMimeData(mimeType string, data []byte)   {}

// Image is the high-level interface silk uses for surfaces;
// ImageSurface is just a *Surface alias.
type Image interface {
	Format() Format
	Width() int
	Height() int
	Stride() int
}
