// Package purecairo is a pure-Go implementation of the Cairo 2D
// graphics API silk uses, with zero CGO dependencies. The package
// builds on every platform Go supports — macOS, Linux, Windows, BSD —
// without libcairo, libpng, libfreetype, or fontconfig.
//
// API surface mirrors `cairo_*` and silk's `_hack` extensions, so silk
// can swap libcairo for purecairo by re-exporting the types and
// functions through a thin alias layer.
//
// Algorithm references: where the C source uses a non-trivial
// algorithm (path stroking, arc → cubic flatten, two-circle radial
// gradients), the Go side either ports the same approach or
// substitutes a published equivalent. The cairo C source under
// `cairo/src/` is the reference; specific functions are cited in
// per-section comments.
//
// Cross-platform parity:
//   - Image surface backed by image.RGBA (every platform).
//   - Text via golang.org/x/image/font/opentype with a per-OS system
//     font discovery walk (font.go). Bundled Go-Regular TrueType is
//     the universal fallback so the binary always has a face.
//   - Window upload: silk's window code reads DataPtr() as cairo
//     ARGB32 (BGRA byte order on little-endian) and passes it to
//     gl.TexImage2D — Surface.Flush() rebuilds that mirror buffer.
//
// Implemented:
//   - Path: MoveTo, LineTo, CurveTo, Arc / ArcNegative, Rectangle,
//     Line, RoundRect, NewPath, ClosePath.
//   - Fill / FillPreserve: anti-aliased via golang.org/x/image/vector.
//     Solid + surface + linear/radial gradient sources.
//   - Stroke / StrokePreserve: per-segment thin-quad rasterisation
//     (caps default to butt, joins to miter).
//   - Save / Restore + transform stack with cairo column-vector
//     semantics.
//   - Clip / ClipPreserve / ResetClip / ClipBounds: AABB approximation.
//     ClipBounds returns user-space rect via CTM inverse.
//   - Paint / PaintWithAlpha: OPERATOR_CLEAR, SOURCE, OVER respected.
//   - ShowGlyphs / ShowGlyphs_hack via opentype faces with CJK
//     fallback chain.
//
// Not implemented:
//   - PDF / SVG / PostScript export surfaces.
//   - Platform-native surfaces (xlib, quartz, win32).
//   - Path-shaped clipping (AABB only for now).
package purecairo

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"unsafe"

	"silk/geom"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// defaultFace returns a fallback face at 12px when no scaled font has
// been set on the context. Used for raw ShowText calls before any
// SetScaledFont. Most painter calls bind the scaled font first, so this
// path mostly serves diagnostics.
func defaultFace() font.Face {
	return loadFace("", 12, false, false)
}

// ===== Constants =====

type Status int32

const (
	STATUS_SUCCESS Status = 0
)

type Format int

const (
	FORMAT_INVALID  Format = -1
	FORMAT_ARGB32   Format = 0
	FORMAT_RGB24    Format = 1
	FORMAT_A8       Format = 2
	FORMAT_A1       Format = 3
	FORMAT_RGB16_565 Format = 4
)

// FORMAT_ARGB is the ARGB32 alias used throughout silk/paint.
const FORMAT_ARGB = FORMAT_ARGB32

type Operator int

const (
	OPERATOR_CLEAR Operator = iota
	OPERATOR_SOURCE
	OPERATOR_OVER
	OPERATOR_IN
	OPERATOR_OUT
	OPERATOR_ATOP
	OPERATOR_DEST
	OPERATOR_DEST_OVER
	OPERATOR_DEST_IN
	OPERATOR_DEST_OUT
	OPERATOR_DEST_ATOP
	OPERATOR_XOR
	OPERATOR_ADD
	OPERATOR_SATURATE
	OPERATOR_MULTIPLY
	OPERATOR_SCREEN
	OPERATOR_OVERLAY
	OPERATOR_DARKEN
	OPERATOR_LIGHTEN
	OPERATOR_COLOR_DODGE
	OPERATOR_COLOR_BURN
	OPERATOR_HARD_LIGHT
	OPERATOR_SOFT_LIGHT
	OPERATOR_DIFFERENCE
	OPERATOR_EXCLUSION
	OPERATOR_HSL_HUE
	OPERATOR_HSL_SATURATION
	OPERATOR_HSL_COLOR
	OPERATOR_HSL_LUMINOSITY
)

type Content int

const (
	CONTENT_COLOR       Content = 0x1000
	CONTENT_ALPHA       Content = 0x2000
	CONTENT_COLOR_ALPHA Content = 0x3000
)

type SurfaceType int

const (
	SURFACE_TYPE_IMAGE SurfaceType = iota
	SURFACE_TYPE_PDF
	SURFACE_TYPE_PS
	SURFACE_TYPE_XLIB
	SURFACE_TYPE_XCB
	SURFACE_TYPE_GLITZ
	SURFACE_TYPE_QUARTZ
	SURFACE_TYPE_WIN32
)

type FontSlant int

const (
	FONT_SLANT_NORMAL FontSlant = iota
	FONT_SLANT_ITALIC
	FONT_SLANT_OBLIQUE
)

type FontWeight int

const (
	FONT_WEIGHT_NORMAL FontWeight = iota
	FONT_WEIGHT_BOLD
)

type Antialias int

const (
	ANTIALIAS_DEFAULT Antialias = iota
	ANTIALIAS_NONE
	ANTIALIAS_GRAY
	ANTIALIAS_SUBPIXEL
)

type FillRule int

const (
	FILL_RULE_WINDING FillRule = iota
	FILL_RULE_EVEN_ODD
)

type LineCap int

const (
	LINE_CAP_BUTT LineCap = iota
	LINE_CAP_ROUND
	LINE_CAP_SQUARE
)

type LineJoin int

const (
	LINE_JOIN_MITER LineJoin = iota
	LINE_JOIN_ROUND
	LINE_JOIN_BEVEL
)

type Extend int

const (
	EXTEND_NONE Extend = iota
	EXTEND_REPEAT
	EXTEND_REFLECT
	EXTEND_PAD
)

type Filter int

const (
	FILTER_FAST Filter = iota
	FILTER_GOOD
	FILTER_BEST
	FILTER_NEAREST
	FILTER_BILINEAR
	FILTER_GAUSSIAN
)

type PathDataType int

// ===== Helper data structures =====

// pathSeg is one path command. Coordinates are in path-local space;
// the CTM is applied at rasterisation time.
type pathSeg struct {
	op        byte // 'M' move, 'L' line, 'C' curve3, 'Z' close
	x1, y1    float64
	x2, y2    float64 // CurveTo control 2
	x3, y3    float64 // CurveTo end
}

// state captures one Save/Restore frame.
type ctxState struct {
	matrix geom.Mat3x2
	source color.Color
	// sourceSurf is non-nil when SetSource(NewPatternForSurface) or
	// SetSourceSurface was called. fillPath / Paint sample from it
	// instead of state.source. (sourceX, sourceY) is the user-space
	// offset where the surface's pixel (0, 0) should land — same
	// convention as cairo_set_source_surface.
	sourceSurf *image.RGBA
	sourceX    float64
	sourceY    float64
	// sourceImage is a generated per-pixel source (linear / radial
	// gradient). If non-nil, it overrides both sourceSurf and the solid
	// state.source on the rasteriser path.
	sourceImage image.Image
	lineWidth  float64
	lineCap    LineCap
	lineJoin   LineJoin
	miter      float64
	operator   Operator
	dash       Dash
	font       *ScaledFont
	clipRect   geom.Rect // AABB approximation
	hasClip    bool
}

// ===== Surface =====

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

func (this *Surface) Format() Format       { return this.format }
func (this *Surface) Width() int           { return this.width }
func (this *Surface) Height() int          { return this.height }
func (this *Surface) Stride() int          { return this.img.Stride }
func (this *Surface) Type() SurfaceType    { return SURFACE_TYPE_IMAGE }
func (this *Surface) Status() Status       { return STATUS_SUCCESS }
func (this *Surface) Destroy()             {}
func (this *Surface) MarkDirty()           {}

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

// MapToImage / UnmapImage / CreateForRectangle / CreateSimilar — stubs.
type MappedImage struct {
	Surface *Surface
}

func (this *Surface) MapToImage(extents *RectangleInt) *MappedImage {
	return &MappedImage{Surface: this}
}

func (this *Surface) UnmapImage(im *MappedImage)               {}
func (this *Surface) CreateForRectangle(r Rectangle) *Surface  { return this }
func (this *Surface) CreateSimilar(c Content, w, h int) *Surface {
	return NewImageSurface(this.format, w, h)
}
func (this *Surface) CreateSimilarImage(f Format, w, h int) *Surface {
	return NewImageSurface(f, w, h)
}
func (this *Surface) Content() Content              { return CONTENT_COLOR_ALPHA }
func (this *Surface) DeviceOffset() (x, y float64)  { return 0, 0 }
func (this *Surface) SetDeviceOffset(x, y float64)  {}
func (this *Surface) Device() *Device               { return nil }
func (this *Surface) HasShowTextGlyphs() bool       { return false }
func (this *Surface) FallbackResolution() (xppi, yppi float64) { return 72, 72 }
func (this *Surface) SetFallbackResolution(xppi, yppi float64) {}
func (this *Surface) CopyPage()                                {}
func (this *Surface) ShowPage()                                {}
func (this *Surface) FontOptions() *FontOptions                { return NewFontOptions() }
func (this *Surface) GetMimeData(mimeType string) ([]byte, bool) { return nil, false }
func (this *Surface) SetMimeData(mimeType string, data []byte)   {}

// Image is the high-level interface silk uses for surfaces; ImageSurface
// is just a *Surface alias.
type Image interface {
	Format() Format
	Width() int
	Height() int
	Stride() int
}

// ===== Pattern =====

// patternKind classifies a Pattern's source. Solid colour patterns
// stay on the existing state.source path; surface and gradient kinds
// route through state.sourceSurf / a generated gradient image.Image.
type patternKind int

const (
	patternKindSolid patternKind = iota
	patternKindSurface
	patternKindLinear
	patternKindRadial
)

// gradStop is one colour stop on a linear or radial gradient.
type gradStop struct {
	offset float64
	col    color.RGBA
}

// Pattern is the source for fill/stroke. silk uses solid colour patterns
// most of the time, surface patterns for image brushes, and gradients
// for theme buttons / cards / radial avatars.
type Pattern struct {
	kind   patternKind
	col    color.Color
	surf   *Surface
	extend Extend
	filter Filter
	matrix geom.Mat3x2

	// Linear gradient endpoints in user space.
	x0, y0, x1, y1 float64
	// Radial gradient: two circles interpolation.
	cx0, cy0, r0 float64
	cx1, cy1, r1 float64
	stops        []gradStop
}

func NewRGBPattern(r, g, b float64) *Pattern {
	return &Pattern{kind: patternKindSolid, col: f64ColorRGB(r, g, b, 1)}
}

func NewRGBAPattern(r, g, b, a float64) *Pattern {
	return &Pattern{kind: patternKindSolid, col: f64ColorRGB(r, g, b, a)}
}

func NewPatternForSurface(s *Surface) *Pattern {
	p := &Pattern{kind: patternKindSurface, surf: s}
	p.matrix.InitIdentity()
	return p
}

// NewLinearPattern creates a linear gradient between (x0, y0) and
// (x1, y1) in user space. Add stops with AddColorStopRGBA.
func NewLinearPattern(x0, y0, x1, y1 float64) *Pattern {
	p := &Pattern{kind: patternKindLinear, x0: x0, y0: y0, x1: x1, y1: y1}
	p.matrix.InitIdentity()
	return p
}

// NewRadialPattern creates a radial gradient between two circles.
// Stops interpolate from the inner (r0) to the outer (r1) circle —
// silk's avatar / card highlights typically pass r0 = 0.
func NewRadialPattern(cx0, cy0, r0, cx1, cy1, r1 float64) *Pattern {
	p := &Pattern{
		kind: patternKindRadial,
		cx0:  cx0, cy0: cy0, r0: r0,
		cx1: cx1, cy1: cy1, r1: r1,
	}
	p.matrix.InitIdentity()
	return p
}

func (this *Pattern) Destroy()       {}
func (this *Pattern) Status() Status { return STATUS_SUCCESS }

// AddColorStopRGB / AddColorStopRGBA push a stop onto the gradient
// stop list. Stops stay sorted by offset because silk's callers add
// them in order; if a future caller breaks that, sampling still
// yields a sensible (clamped) colour for any t.
func (this *Pattern) AddColorStopRGB(off, r, g, b float64) {
	this.AddColorStopRGBA(off, r, g, b, 1)
}
func (this *Pattern) AddColorStopRGBA(off, r, g, b, a float64) {
	col := color.RGBA{
		R: clamp8(r),
		G: clamp8(g),
		B: clamp8(b),
		A: clamp8(a),
	}
	this.stops = append(this.stops, gradStop{offset: off, col: col})
	if this.kind == patternKindSolid && this.col == nil {
		this.col = col
	}
}
func (this *Pattern) ColorStopCount() int { return len(this.stops) }
func (this *Pattern) ColorStopRGBA(idx int) (off, r, g, b, a float64) {
	if idx < 0 || idx >= len(this.stops) {
		return 0, 0, 0, 0, 0
	}
	s := this.stops[idx]
	return s.offset,
		float64(s.col.R) / 255.0,
		float64(s.col.G) / 255.0,
		float64(s.col.B) / 255.0,
		float64(s.col.A) / 255.0
}
func (this *Pattern) RGBA() (r, g, b, a float64) {
	if this.col == nil {
		return 0, 0, 0, 1
	}
	cr, cg, cb, ca := this.col.RGBA()
	return float64(cr) / 0xffff, float64(cg) / 0xffff, float64(cb) / 0xffff, float64(ca) / 0xffff
}
func (this *Pattern) Surface() *Surface     { return this.surf }
func (this *Pattern) SetMatrix(m *geom.Mat3x2) {
	if m != nil {
		this.matrix = *m
	}
}
func (this *Pattern) GetMatrix(m *geom.Mat3x2) {
	if m != nil {
		*m = this.matrix
	}
}
func (this *Pattern) SetExtend(e Extend) { this.extend = e }
func (this *Pattern) Extend() Extend     { return this.extend }
func (this *Pattern) SetFilter(f Filter) { this.filter = f }
func (this *Pattern) Filter() Filter     { return this.filter }
func (this *Pattern) Type() int          { return 0 }

// ===== Font =====

type FontFace struct {
	family string
	slant  FontSlant
	weight FontWeight
}

func NewToyFontFace(family string, slant FontSlant, weight FontWeight) *FontFace {
	return &FontFace{family: family, slant: slant, weight: weight}
}

func (this *FontFace) Destroy()       {}
func (this *FontFace) Status() Status { return STATUS_SUCCESS }
func (this *FontFace) Type() int      { return 0 }
func (this *FontFace) ToyFamily() string { return this.family }
func (this *FontFace) ToySlant() FontSlant { return this.slant }
func (this *FontFace) ToyWeight() FontWeight { return this.weight }

type ScaledFont struct {
	face     *FontFace
	matrix   geom.Mat3x2
	ctm      geom.Mat3x2
	options  *FontOptions
	pixSize  int
	resolved font.Face // lazy: resolved opentype face matching family/weight/size
	cjk      font.Face // lazy: CJK fallback at the same size
}

func NewScaledFont(face *FontFace, matrix, ctm *geom.Mat3x2, options *FontOptions) *ScaledFont {
	sf := &ScaledFont{face: face, options: options}
	if matrix != nil {
		sf.matrix = *matrix
	} else {
		sf.matrix.InitIdentity()
	}
	if ctm != nil {
		sf.ctm = *ctm
	} else {
		sf.ctm.InitIdentity()
	}
	sf.pixSize = pixelSizeFromMatrix(&sf.matrix)
	return sf
}

// pixelSizeFromMatrix derives a pixel font size from cairo's font
// matrix. silk constructs a pure scale matrix via m.InitScale(size,
// size), so reading m.Xx (the scale-x term) gives the size directly.
// We round to int because opentype.NewFace caches by int.
func pixelSizeFromMatrix(m *geom.Mat3x2) int {
	sx := math.Hypot(m.Xx, m.Yx)
	if sx < 1 {
		sx = 12
	}
	return int(math.Round(sx))
}

// face returns the resolved opentype face, lazily loading on first
// access. Cached on the ScaledFont so repeat queries are O(1).
func (this *ScaledFont) latinFace() font.Face {
	if this.resolved != nil {
		return this.resolved
	}
	family := ""
	bold := false
	italic := false
	if this.face != nil {
		family = this.face.family
		bold = this.face.weight == FONT_WEIGHT_BOLD
		italic = this.face.slant != FONT_SLANT_NORMAL
	}
	size := this.pixSize
	if size <= 0 {
		size = 12
	}
	this.resolved = loadFace(family, size, bold, italic)
	return this.resolved
}

// cjkFace returns a CJK-capable fallback at the same size, lazily
// initialised on first request.
func (this *ScaledFont) cjkFace() font.Face {
	if this.cjk != nil {
		return this.cjk
	}
	size := this.pixSize
	if size <= 0 {
		size = 12
	}
	this.cjk = loadCJKFallback(size)
	return this.cjk
}

// glyphFace picks the right face to render rune r — Latin face if it
// has a glyph, otherwise the CJK fallback. Pattern matches cairo's
// fallback chain.
func (this *ScaledFont) glyphFace(r rune) font.Face {
	primary := this.latinFace()
	if _, ok := primary.GlyphAdvance(r); ok {
		return primary
	}
	return this.cjkFace()
}

func (this *ScaledFont) Destroy()       {}
func (this *ScaledFont) Status() Status { return STATUS_SUCCESS }

// FontExtents reports ascent / descent / line-height in pixels. Drives
// silk's text alignment math; needs to be a real number (not the
// placeholder constants the bitmap stub returned) so labels position
// vertically the same as on the libcairo build.
func (this *ScaledFont) FontExtents() *FontExtents {
	face := this.latinFace()
	m := face.Metrics()
	return &FontExtents{
		Ascent:      fixedToFloat(m.Ascent),
		Descent:     fixedToFloat(m.Descent),
		Height:      fixedToFloat(m.Height),
		MaxXAdvance: float64(this.pixSize), // monospace upper bound
		MaxYAdvance: 0,
	}
}

// GlyphExtents totals the advance width of glyphs[] using the real
// face. Heights come from font metrics. Cairo uses this for hit tests
// and selection rectangles.
func (this *ScaledFont) GlyphExtents(glyphs []Glyph) *TextExtents {
	face := this.latinFace()
	cjk := (font.Face)(nil)
	xa := 0.0
	for _, g := range glyphs {
		r := rune(g.index)
		adv, ok := face.GlyphAdvance(r)
		if !ok {
			if cjk == nil {
				cjk = this.cjkFace()
			}
			adv, _ = cjk.GlyphAdvance(r)
		}
		xa += fixedToFloat(adv)
	}
	m := face.Metrics()
	return &TextExtents{
		Width:    xa,
		Height:   fixedToFloat(m.Ascent + m.Descent),
		XAdvance: xa,
		YBearing: -fixedToFloat(m.Ascent),
	}
}

// GlyphExtents_hack reads `n` Glyphs from raw memory and delegates to
// GlyphExtents. silk's CGO bypass passes an unsafe pointer; same
// 24-byte stride as the libcairo build.
func (this *ScaledFont) GlyphExtents_hack(p unsafe.Pointer, n int) *TextExtents {
	if p == nil || n == 0 {
		m := this.latinFace().Metrics()
		return &TextExtents{Height: fixedToFloat(m.Ascent + m.Descent)}
	}
	glyphs := unsafe.Slice((*Glyph)(p), n)
	return this.GlyphExtents(glyphs)
}

// TextToGlyphs_hack lays out a UTF-8 string at (x, y) using the
// resolved opentype face. Each rune becomes a Glyph at the
// advance-incremented X position. Glyphs whose codepoint isn't in the
// primary face fall through to the CJK face — same behaviour as
// libcairo's fallback chain.
//
// Returned glyphs are emitted via the `out` callback; silk's wrapper
// in paint/font.go reads the buffer back into Go memory.
func (this *ScaledFont) TextToGlyphs_hack(x, y float64, text string, out func(buf unsafe.Pointer, num int)) error {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	primary := this.latinFace()
	cjk := (font.Face)(nil)

	glyphs := make([]Glyph, 0, len(runes))
	cx := x
	for _, r := range runes {
		adv, ok := primary.GlyphAdvance(r)
		if !ok {
			if cjk == nil {
				cjk = this.cjkFace()
			}
			adv, ok = cjk.GlyphAdvance(r)
			if !ok {
				adv = fixed.Int26_6(this.pixSize << 6) // 1em fallback
			}
		}
		glyphs = append(glyphs, Glyph{index: uint32(r), X: cx, Y: y})
		cx += fixedToFloat(adv)
	}
	if len(glyphs) > 0 && out != nil {
		out(unsafe.Pointer(&glyphs[0]), len(glyphs))
	}
	return nil
}

// TextExtents measures width/height of a text string at this font's
// scale. Used by silk's alignment + ellipsis logic.
func (this *ScaledFont) TextExtents(text string) *TextExtents {
	if text == "" {
		m := this.latinFace().Metrics()
		return &TextExtents{Height: fixedToFloat(m.Ascent + m.Descent)}
	}
	primary := this.latinFace()
	cjk := (font.Face)(nil)
	xa := 0.0
	for _, r := range text {
		adv, ok := primary.GlyphAdvance(r)
		if !ok {
			if cjk == nil {
				cjk = this.cjkFace()
			}
			adv, _ = cjk.GlyphAdvance(r)
		}
		xa += fixedToFloat(adv)
	}
	m := primary.Metrics()
	return &TextExtents{
		Width:    xa,
		Height:   fixedToFloat(m.Ascent + m.Descent),
		XAdvance: xa,
		YBearing: -fixedToFloat(m.Ascent),
	}
}

// fixedToFloat converts a 26.6 fixed-point value to floating point
// pixels. Used everywhere font metrics need to leave the font.Face
// API.
func fixedToFloat(v fixed.Int26_6) float64 {
	return float64(v) / 64.0
}
func (this *ScaledFont) GlyphPath(glyphs []Glyph) {}
func (this *ScaledFont) FontFace() *FontFace      { return this.face }
func (this *ScaledFont) FontMatrix(m *geom.Mat3x2) {
	if m != nil {
		*m = this.matrix
	}
}
func (this *ScaledFont) ScaleMatrix(m *geom.Mat3x2) {
	if m != nil {
		*m = this.ctm
	}
}
func (this *ScaledFont) Ctm(m *geom.Mat3x2) {
	if m != nil {
		*m = this.ctm
	}
}
func (this *ScaledFont) FontOptions() *FontOptions { return this.options }
func (this *ScaledFont) Type() int                 { return 0 }

type FontOptions struct{}

func NewFontOptions() *FontOptions               { return &FontOptions{} }
func (this *FontOptions) Destroy()               {}
func (this *FontOptions) Copy() *FontOptions     { return &FontOptions{} }
func (this *FontOptions) Status() Status         { return STATUS_SUCCESS }
func (this *FontOptions) Merge(other *FontOptions) {}
func (this *FontOptions) Equal(other *FontOptions) bool { return true }
func (this *FontOptions) Hash() uint32           { return 0 }
func (this *FontOptions) SetAntialias(a Antialias)  {}
func (this *FontOptions) Antialias() Antialias      { return ANTIALIAS_DEFAULT }
func (this *FontOptions) SetSubpixelOrder(int)      {}
func (this *FontOptions) SubpixelOrder() int        { return 0 }
func (this *FontOptions) SetHintStyle(int)          {}
func (this *FontOptions) HintStyle() int            { return 0 }
func (this *FontOptions) SetHintMetrics(int)        {}
func (this *FontOptions) HintMetrics() int          { return 0 }

type FontExtents struct {
	Ascent      float64
	Descent     float64
	Height      float64
	MaxXAdvance float64
	MaxYAdvance float64
}

type TextExtents struct {
	XBearing float64
	YBearing float64
	Width    float64
	Height   float64
	XAdvance float64
	YAdvance float64
}

type Glyph struct {
	index uint32
	A     uint32 // unused for cairo, but use in lib/gui
	X     float64
	Y     float64
}

// ===== Misc types =====

type Rectangle struct {
	X, Y, Width, Height float64
}

type RectangleInt struct {
	X, Y, Width, Height int32
}

type RectangleList struct {
	Status     Status
	Rectangles []Rectangle
}

type Device struct{}

func (this *Device) Destroy()       {}
func (this *Device) Status() Status { return STATUS_SUCCESS }
func (this *Device) Type() int      { return 0 }

type Path struct {
	segs []pathSeg
}

func (this *Path) Destroy() {}

type Dash struct {
	Dashes []float64
	Offset float64
}

// ===== Context =====

type Context struct {
	surface *Surface
	state   ctxState
	stack   []ctxState
	path    []pathSeg
	curX    float64
	curY    float64
	hasCur  bool
}

func (this *Context) Destroy()        {}
func (this *Context) Status() Status  { return STATUS_SUCCESS }
func (this *Context) Target() *Surface { return this.surface }

// --- Save / Restore ---

func (this *Context) Save() {
	this.stack = append(this.stack, this.state)
}

func (this *Context) Restore() {
	if n := len(this.stack); n > 0 {
		this.state = this.stack[n-1]
		this.stack = this.stack[:n-1]
	}
}

// --- Source colour ---

func (this *Context) SetSourceRGB(r, g, b float64) {
	this.state.source = f64ColorRGB(r, g, b, 1)
}

func (this *Context) SetSourceRGBA(r, g, b, a float64) {
	this.state.source = f64ColorRGB(r, g, b, a)
}

// SetSource binds a pattern as the active fill / paint source. Solid
// colours stay on state.source; surfaces feed state.sourceSurf;
// gradients build a per-pixel image.Image on state.sourceImage.
func (this *Context) SetSource(p *Pattern) {
	if p == nil {
		return
	}
	// Reset any prior non-solid source — SetSource is exclusive.
	this.state.sourceSurf = nil
	this.state.sourceImage = nil

	switch p.kind {
	case patternKindSurface:
		if p.surf != nil && p.surf.img != nil {
			this.state.sourceSurf = p.surf.img
			this.state.sourceX = -p.matrix.X0
			this.state.sourceY = -p.matrix.Y0
		}
	case patternKindLinear:
		this.state.sourceImage = newLinearGradient(&this.state.matrix, p)
	case patternKindRadial:
		this.state.sourceImage = newRadialGradient(&this.state.matrix, p)
	default:
		if p.col != nil {
			this.state.source = p.col
		}
	}
}

// SetSourceSurface mirrors cairo_set_source_surface: subsequent
// fill / paint draws sample pixels from `s`, with the surface's
// pixel (0, 0) anchored at user (x, y).
func (this *Context) SetSourceSurface(s *Surface, x, y float64) {
	if s == nil || s.img == nil {
		this.state.sourceSurf = nil
		return
	}
	this.state.sourceSurf = s.img
	this.state.sourceX = x
	this.state.sourceY = y
}

func (this *Context) Source() *Pattern {
	return &Pattern{col: this.state.source}
}

// --- Pen / line attrs ---

func (this *Context) SetLineWidth(w float64) { this.state.lineWidth = w }
func (this *Context) LineWidth() float64     { return this.state.lineWidth }
func (this *Context) SetLineCap(c LineCap)   { this.state.lineCap = c }
func (this *Context) LineCap() LineCap       { return this.state.lineCap }
func (this *Context) SetLineJoin(j LineJoin) { this.state.lineJoin = j }
func (this *Context) LineJoin() LineJoin     { return this.state.lineJoin }
func (this *Context) SetMiterLimit(m float64) { this.state.miter = m }
func (this *Context) MiterLimit() float64    { return this.state.miter }
func (this *Context) SetDash(d Dash)         { this.state.dash = d }
func (this *Context) Dash() Dash             { return this.state.dash }
func (this *Context) SetTolerance(v float64) {}
func (this *Context) Tolerance() float64     { return 0.1 }
func (this *Context) SetAntialias(a Antialias) {}
func (this *Context) Antialias() Antialias   { return ANTIALIAS_DEFAULT }
func (this *Context) SetFillRule(r FillRule) {}
func (this *Context) FillRule() FillRule     { return FILL_RULE_WINDING }
func (this *Context) SetOperator(o Operator) { this.state.operator = o }
func (this *Context) Operator() Operator     { return this.state.operator }

// --- Transform stack ---
//
// silk's geom.Mat3x2 multiplication uses post-multiply semantics
// (`new = old + delta` regardless of scale), which is correct for its
// own internal coord-system math but does NOT match libcairo. In real
// cairo, `cairo_translate(tx, ty)` translates *user space* — so the
// device-space translation is scaled by the current CTM scale. We
// reimplement the transform ops here using cairo semantics so paths
// and glyphs share one consistent device-space mapping.

// Translate concatenates a user-space translation onto the CTM. The
// device shift equals (Xx*tx + Xy*ty, Yx*tx + Yy*ty) — under a 2x
// HiDPI scale, Translate(10, 0) shifts device by 20 pixels. silk's
// geom.Mat3x2.Translate would shift by 10, breaking the coord match
// between Rectangle (which scales) and Translate (which doesn't).
func (this *Context) Translate(tx, ty float64) {
	m := &this.state.matrix
	m.X0 += m.Xx*tx + m.Xy*ty
	m.Y0 += m.Yx*tx + m.Yy*ty
}

// Scale concatenates a user-space scale. Scaling user space by (sx,
// sy) means a unit user vector now maps to (sx, sy) user → applied
// to existing CTM that already maps user→device. So Xx, Xy multiply
// by sx; Yx, Yy multiply by sy.
func (this *Context) Scale(sx, sy float64) {
	m := &this.state.matrix
	m.Xx *= sx
	m.Yx *= sx
	m.Xy *= sy
	m.Yy *= sy
}

// Rotate concatenates a rotation onto the CTM. cairo_rotate semantics:
// rotates user space, so CTM_new = CTM * Rotate(theta).
func (this *Context) Rotate(r float64) {
	c := math.Cos(r)
	s := math.Sin(r)
	m := &this.state.matrix
	xx, xy := m.Xx, m.Xy
	yx, yy := m.Yx, m.Yy
	m.Xx = xx*c + xy*s
	m.Xy = -xx*s + xy*c
	m.Yx = yx*c + yy*s
	m.Yy = -yx*s + yy*c
}

// Transform applies a user-space matrix m onto the CTM. The
// composition is CTM_new = CTM * m, mirroring cairo_transform.
func (this *Context) Transform(m *geom.Mat3x2) {
	if m == nil {
		return
	}
	c := &this.state.matrix
	xx := c.Xx*m.Xx + c.Xy*m.Yx
	yx := c.Yx*m.Xx + c.Yy*m.Yx
	xy := c.Xx*m.Xy + c.Xy*m.Yy
	yy := c.Yx*m.Xy + c.Yy*m.Yy
	x0 := c.Xx*m.X0 + c.Xy*m.Y0 + c.X0
	y0 := c.Yx*m.X0 + c.Yy*m.Y0 + c.Y0
	c.Xx, c.Yx, c.Xy, c.Yy, c.X0, c.Y0 = xx, yx, xy, yy, x0, y0
}
func (this *Context) SetMatrix(m *geom.Mat3x2) {
	if m != nil {
		this.state.matrix = *m
	}
}
func (this *Context) GetMatrix(m *geom.Mat3x2) {
	if m != nil {
		*m = this.state.matrix
	}
}
func (this *Context) ResetMatrix() { this.state.matrix.InitIdentity() }
func (this *Context) UserToDevice(x, y *float64) {
	if x != nil && y != nil {
		nx, ny := this.state.matrix.Transform(*x, *y)
		*x, *y = nx, ny
	}
}
func (this *Context) UserToDeviceDistance(x, y *float64) {
	if x != nil && y != nil {
		nx, ny := this.state.matrix.TransformVec(*x, *y)
		*x, *y = nx, ny
	}
}
// DeviceToUser inverts the CTM at (*x, *y). Mirrors cairo's
// cairo_device_to_user.
func (this *Context) DeviceToUser(x, y *float64) {
	if x == nil || y == nil {
		return
	}
	ux, uy, ok := this.deviceToUser(*x, *y)
	if ok {
		*x, *y = ux, uy
	}
}

// DeviceToUserDistance inverts the linear part of the CTM (no
// translation). Used for distance / size conversions where the
// origin shouldn't matter.
func (this *Context) DeviceToUserDistance(x, y *float64) {
	if x == nil || y == nil {
		return
	}
	m := &this.state.matrix
	det := m.Xx*m.Yy - m.Xy*m.Yx
	if det == 0 {
		return
	}
	dx := *x
	dy := *y
	*x = (m.Yy*dx - m.Xy*dy) / det
	*y = (-m.Yx*dx + m.Xx*dy) / det
}

// --- Path construction ---

func (this *Context) NewPath()      { this.path = this.path[:0] }
func (this *Context) NewSubPath()   {}
func (this *Context) ClosePath()    { this.path = append(this.path, pathSeg{op: 'Z'}) }
func (this *Context) HasCurrentPoint() bool { return this.hasCur }

func (this *Context) MoveTo(x, y float64) {
	this.path = append(this.path, pathSeg{op: 'M', x1: x, y1: y})
	this.curX, this.curY, this.hasCur = x, y, true
}

func (this *Context) LineTo(x, y float64) {
	this.path = append(this.path, pathSeg{op: 'L', x1: x, y1: y})
	this.curX, this.curY, this.hasCur = x, y, true
}

func (this *Context) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	this.path = append(this.path, pathSeg{op: 'C', x1: x1, y1: y1, x2: x2, y2: y2, x3: x3, y3: y3})
	this.curX, this.curY, this.hasCur = x3, y3, true
}

func (this *Context) Line(x0, y0, x1, y1 float64) {
	this.MoveTo(x0, y0)
	this.LineTo(x1, y1)
}

func (this *Context) Rectangle(x, y, w, h float64) {
	this.MoveTo(x, y)
	this.LineTo(x+w, y)
	this.LineTo(x+w, y+h)
	this.LineTo(x, y+h)
	this.ClosePath()
}

func (this *Context) RoundRect(x, y, w, h, r float64) {
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	this.MoveTo(x+r, y)
	this.LineTo(x+w-r, y)
	this.Arc(x+w-r, y+r, r, -math.Pi/2, 0)
	this.LineTo(x+w, y+h-r)
	this.Arc(x+w-r, y+h-r, r, 0, math.Pi/2)
	this.LineTo(x+r, y+h)
	this.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi)
	this.LineTo(x, y+r)
	this.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2)
}

func (this *Context) Arc(xc, yc, radius, a0, a1 float64) {
	this.appendArc(xc, yc, radius, a0, a1, +1)
}

func (this *Context) ArcNegative(xc, yc, radius, a0, a1 float64) {
	this.appendArc(xc, yc, radius, a0, a1, -1)
}

func (this *Context) appendArc(xc, yc, r, a0, a1 float64, sign float64) {
	if sign > 0 {
		for a1 < a0 {
			a1 += 2 * math.Pi
		}
	} else {
		for a1 > a0 {
			a1 -= 2 * math.Pi
		}
	}
	span := math.Abs(a1 - a0)
	steps := int(math.Ceil(span/(math.Pi*0.5))) * 8
	if steps < 4 {
		steps = 4
	}
	startX := xc + r*math.Cos(a0)
	startY := yc + r*math.Sin(a0)
	// Decide M-vs-L by what the path actually contains. Just checking
	// hasCur is not enough: after Stroke / Fill the path is cleared but
	// hasCur stays true (cairo preserves the current point across
	// path-consuming ops). If we emitted LineTo with an empty path, the
	// rasterizer would synthesise a stray line from (0, 0) to the arc
	// start — which then fills as a giant wedge across the surface.
	if len(this.path) == 0 {
		this.MoveTo(startX, startY)
	} else {
		this.LineTo(startX, startY)
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		a := a0 + (a1-a0)*t
		this.LineTo(xc+r*math.Cos(a), yc+r*math.Sin(a))
	}
}

func (this *Context) RelMoveTo(dx, dy float64)             { this.MoveTo(this.curX+dx, this.curY+dy) }
func (this *Context) RelLineTo(dx, dy float64)             { this.LineTo(this.curX+dx, this.curY+dy) }
func (this *Context) RelCurveTo(x1, y1, x2, y2, x3, y3 float64) {
	this.CurveTo(this.curX+x1, this.curY+y1, this.curX+x2, this.curY+y2, this.curX+x3, this.curY+y3)
}
func (this *Context) PathExtens() (x1, y1, x2, y2 float64) {
	if len(this.path) == 0 {
		return 0, 0, 0, 0
	}
	x1, y1 = math.Inf(1), math.Inf(1)
	x2, y2 = math.Inf(-1), math.Inf(-1)
	for _, p := range this.path {
		x, y := p.x1, p.y1
		if p.op == 'C' {
			x, y = p.x3, p.y3
		} else if p.op == 'Z' {
			continue
		}
		if x < x1 {
			x1 = x
		}
		if x > x2 {
			x2 = x
		}
		if y < y1 {
			y1 = y
		}
		if y > y2 {
			y2 = y
		}
	}
	return
}

func (this *Context) CurrentPoint() (x, y float64) { return this.curX, this.curY }

func (this *Context) CopyPath() *Path     { return &Path{segs: append([]pathSeg{}, this.path...)} }
func (this *Context) CopyPathFlat() *Path { return this.CopyPath() }
func (this *Context) AppendPath(p *Path)  {
	if p != nil {
		this.path = append(this.path, p.segs...)
	}
}

func (this *Context) TextPath(s string) {
	// Stub — a future opentype-backed glyph-path traversal would
	// emit MoveTo/LineTo/CurveTo for each glyph contour.
}

// --- Fill / Stroke ---

func (this *Context) Fill() {
	this.fillPath()
	this.path = this.path[:0]
}


func (this *Context) FillPreserve() { this.fillPath() }

func (this *Context) fillPath() {
	if this.surface == nil || this.surface.img == nil {
		return
	}
	w, h := this.surface.width, this.surface.height
	r := vector.NewRasterizer(w, h)
	first := true
	for _, p := range this.path {
		x, y := this.state.matrix.Transform(p.x1, p.y1)
		switch p.op {
		case 'M':
			if !first {
				r.ClosePath()
			}
			r.MoveTo(float32(x), float32(y))
			first = false
		case 'L':
			// A leading L (no prior M) should anchor the subpath at L's
			// own coords — otherwise the rasterizer pulls the line from
			// its default (0, 0) and fills a wedge to the surface origin.
			if first {
				r.MoveTo(float32(x), float32(y))
				first = false
			} else {
				r.LineTo(float32(x), float32(y))
			}
		case 'C':
			x2, y2 := this.state.matrix.Transform(p.x2, p.y2)
			x3, y3 := this.state.matrix.Transform(p.x3, p.y3)
			if first {
				r.MoveTo(float32(x), float32(y))
				first = false
			}
			r.CubeTo(float32(x), float32(y), float32(x2), float32(y2), float32(x3), float32(y3))
		case 'Z':
			r.ClosePath()
		}
	}
	this.rasterizeWithSource(r, this.drawRect())
}

// rasterizeWithSource draws the rasteriser's coverage into dst[r] using
// the active source — surface pattern, gradient pattern, or solid
// colour. The first matching slot wins; missing slots fall through to
// the next.
func (this *Context) rasterizeWithSource(r *vector.Rasterizer, dr image.Rectangle) {
	if this.state.sourceImage != nil {
		// Gradients sample by device pixel directly; sp = dr.Min so
		// At(x, y) sees the actual surface coords.
		r.Draw(this.surface.img, dr, this.state.sourceImage, dr.Min)
		return
	}
	if this.state.sourceSurf != nil {
		dx, dy := this.state.matrix.Transform(this.state.sourceX, this.state.sourceY)
		sp := image.Point{
			X: dr.Min.X - int(math.Round(dx)),
			Y: dr.Min.Y - int(math.Round(dy)),
		}
		r.Draw(this.surface.img, dr, this.state.sourceSurf, sp)
		return
	}
	r.Draw(this.surface.img, dr, &image.Uniform{C: this.state.source}, image.Point{})
}

// ===== Gradient implementations =====
//
// Both linearGradient and radialGradient implement image.Image and
// return per-device-pixel colours. Endpoints / centres are pre-
// transformed to device space at SetSource time, so the per-pixel
// inner loop is only arithmetic — no matrix multiply per call.

type linearGradient struct {
	dx, dy   float64 // device delta from p0 to p1
	dot      float64 // dx*dx + dy*dy (inverse cached as invDot)
	invDot   float64
	p0x, p0y float64
	stops    []gradStop
}

func newLinearGradient(ctm *geom.Mat3x2, p *Pattern) *linearGradient {
	x0, y0 := ctm.Transform(p.x0, p.y0)
	x1, y1 := ctm.Transform(p.x1, p.y1)
	dx := x1 - x0
	dy := y1 - y0
	dot := dx*dx + dy*dy
	invDot := 0.0
	if dot > 1e-12 {
		invDot = 1 / dot
	}
	return &linearGradient{
		dx: dx, dy: dy, dot: dot, invDot: invDot,
		p0x: x0, p0y: y0,
		stops: append([]gradStop(nil), p.stops...),
	}
}

func (g *linearGradient) ColorModel() color.Model { return color.RGBAModel }
func (g *linearGradient) Bounds() image.Rectangle {
	return image.Rect(-1<<30, -1<<30, 1<<30, 1<<30)
}
func (g *linearGradient) At(x, y int) color.Color { return g.RGBA64At(x, y) }
func (g *linearGradient) RGBA64At(x, y int) color.RGBA64 {
	if g.dot == 0 {
		return colorAtOffset(g.stops, 0)
	}
	fx, fy := float64(x), float64(y)
	t := ((fx-g.p0x)*g.dx + (fy-g.p0y)*g.dy) * g.invDot
	return colorAtOffset(g.stops, t)
}

type radialGradient struct {
	cx, cy float64 // outer circle centre in device coords
	rOuter float64 // outer radius in device units
	rInner float64 // inner radius in device units
	stops  []gradStop
}

func newRadialGradient(ctm *geom.Mat3x2, p *Pattern) *radialGradient {
	cx, cy := ctm.Transform(p.cx1, p.cy1)
	// Rough device-radius using CTM linear scale.
	scale := math.Hypot(ctm.Xx, ctm.Yx)
	if scale == 0 {
		scale = 1
	}
	return &radialGradient{
		cx:     cx,
		cy:     cy,
		rOuter: p.r1 * scale,
		rInner: p.r0 * scale,
		stops:  append([]gradStop(nil), p.stops...),
	}
}

func (g *radialGradient) ColorModel() color.Model { return color.RGBAModel }
func (g *radialGradient) Bounds() image.Rectangle {
	return image.Rect(-1<<30, -1<<30, 1<<30, 1<<30)
}
func (g *radialGradient) At(x, y int) color.Color { return g.RGBA64At(x, y) }
func (g *radialGradient) RGBA64At(x, y int) color.RGBA64 {
	span := g.rOuter - g.rInner
	if span <= 0 {
		return colorAtOffset(g.stops, 0)
	}
	dx := float64(x) - g.cx
	dy := float64(y) - g.cy
	d := math.Sqrt(dx*dx + dy*dy)
	t := (d - g.rInner) / span
	return colorAtOffset(g.stops, t)
}

// colorAtOffset interpolates the stop list at parametric position t.
// Out-of-range t clamps to the nearest stop colour. Empty stop lists
// return transparent black so a forgotten AddColorStop doesn't paint
// uninitialised garbage.
func colorAtOffset(stops []gradStop, t float64) color.RGBA64 {
	if len(stops) == 0 {
		return color.RGBA64{}
	}
	if t <= stops[0].offset {
		return rgba8To64(stops[0].col)
	}
	if t >= stops[len(stops)-1].offset {
		return rgba8To64(stops[len(stops)-1].col)
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].offset {
			prev := stops[i-1]
			curr := stops[i]
			span := curr.offset - prev.offset
			if span < 1e-9 {
				return rgba8To64(curr.col)
			}
			f := (t - prev.offset) / span
			return blendRGBA(prev.col, curr.col, f)
		}
	}
	return rgba8To64(stops[len(stops)-1].col)
}

func blendRGBA(a, b color.RGBA, t float64) color.RGBA64 {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	one := 1 - t
	r := one*float64(a.R) + t*float64(b.R)
	g := one*float64(a.G) + t*float64(b.G)
	bb := one*float64(a.B) + t*float64(b.B)
	aa := one*float64(a.A) + t*float64(b.A)
	return color.RGBA64{
		R: uint16(r * 257),
		G: uint16(g * 257),
		B: uint16(bb * 257),
		A: uint16(aa * 257),
	}
}

func rgba8To64(c color.RGBA) color.RGBA64 {
	return color.RGBA64{
		R: uint16(c.R) * 257,
		G: uint16(c.G) * 257,
		B: uint16(c.B) * 257,
		A: uint16(c.A) * 257,
	}
}

func (this *Context) FillExtens() (x1, y1, x2, y2 float64) { return this.PathExtens() }
func (this *Context) InFill(x, y float64) bool              { return false }

func (this *Context) Stroke() {
	this.strokePath()
	this.path = this.path[:0]
}

func (this *Context) StrokePreserve() { this.strokePath() }

func (this *Context) strokePath() {
	// Approximate stroke: rasterise the outline as filled rectangles
	// per segment. Not antialiased corners or proper cap/join — that's
	// a full stroke-flattening pass for a follow-up.
	if this.surface == nil || this.surface.img == nil {
		return
	}
	w, h := this.surface.width, this.surface.height
	wpx := this.state.lineWidth
	if wpx < 1 {
		wpx = 1
	}
	r := vector.NewRasterizer(w, h)
	prevX, prevY := 0.0, 0.0
	have := false
	for _, p := range this.path {
		switch p.op {
		case 'M':
			prevX, prevY = this.state.matrix.Transform(p.x1, p.y1)
			have = true
		case 'L':
			x, y := this.state.matrix.Transform(p.x1, p.y1)
			if have {
				strokeSegment(r, prevX, prevY, x, y, wpx)
			}
			prevX, prevY = x, y
			have = true
		case 'C':
			// Flatten the cubic to 16 line segs, stroke each.
			x0, y0 := prevX, prevY
			cp1x, cp1y := this.state.matrix.Transform(p.x1, p.y1)
			cp2x, cp2y := this.state.matrix.Transform(p.x2, p.y2)
			x3, y3 := this.state.matrix.Transform(p.x3, p.y3)
			for i := 1; i <= 16; i++ {
				t := float64(i) / 16
				mt := 1 - t
				bx := mt*mt*mt*x0 + 3*mt*mt*t*cp1x + 3*mt*t*t*cp2x + t*t*t*x3
				by := mt*mt*mt*y0 + 3*mt*mt*t*cp1y + 3*mt*t*t*cp2y + t*t*t*y3
				if have {
					strokeSegment(r, prevX, prevY, bx, by, wpx)
				}
				prevX, prevY = bx, by
				have = true
			}
		case 'Z':
			have = false
		}
	}
	this.rasterizeWithSource(r, this.drawRect())
}

// strokeSegment rasterises a line as a thin oriented quad (offset by
// half-width perpendicular to the segment).
func strokeSegment(r *vector.Rasterizer, x0, y0, x1, y1, width float64) {
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	hw := width * 0.5
	nx, ny := -dy/l*hw, dx/l*hw
	r.MoveTo(float32(x0+nx), float32(y0+ny))
	r.LineTo(float32(x1+nx), float32(y1+ny))
	r.LineTo(float32(x1-nx), float32(y1-ny))
	r.LineTo(float32(x0-nx), float32(y0-ny))
	r.ClosePath()
}

func (this *Context) StrokeExtens() (x1, y1, x2, y2 float64) { return this.PathExtens() }
func (this *Context) InStroke(x, y float64) bool             { return false }

// --- Paint ---

func (this *Context) Paint() {
	this.PaintWithAlpha(1.0)
}

// PaintWithAlpha fills the active clip region with the current source.
// Honours the active Porter-Duff operator — most importantly OpClear
// (zeroes the destination) and OpOver (alpha-blends source over dest).
//
// The clear case is critical: silk's Window.paint() begins each frame
// with `SetOperator(OpClear); Paint(); SetOperator(OpOver)` to wipe
// the back buffer. If we ignored the operator and painted with the
// source colour instead, every frame would inherit the previous
// frame's last source — visible as huge coloured panels under the
// rest of the UI.
func (this *Context) PaintWithAlpha(alpha float64) {
	if this.surface == nil || this.surface.img == nil {
		return
	}
	dr := this.drawRect()
	if dr.Empty() {
		return
	}

	switch this.state.operator {
	case OPERATOR_CLEAR:
		// Result = (0,0,0,0). Source is ignored entirely.
		clear := color.RGBA{}
		draw.Draw(this.surface.img, dr, &image.Uniform{C: clear}, image.Point{}, draw.Src)
		return
	case OPERATOR_SOURCE:
		// Result = source. Replaces dst even where source is transparent.
		if this.state.sourceSurf != nil {
			draw.Draw(this.surface.img, dr, this.state.sourceSurf,
				this.sourceSamplePoint(dr.Min), draw.Src)
			return
		}
		src := this.state.source
		if src == nil {
			src = color.RGBA{}
		}
		col := scaleAlpha(src, alpha)
		draw.Draw(this.surface.img, dr, &image.Uniform{C: col}, image.Point{}, draw.Src)
		return
	default:
		// OPERATOR_OVER (the cairo default) and any other operator we
		// haven't specialised: alpha-blend source over dst.
		if this.state.sourceSurf != nil {
			draw.Draw(this.surface.img, dr, this.state.sourceSurf,
				this.sourceSamplePoint(dr.Min), draw.Over)
			return
		}
		src := this.state.source
		if src == nil {
			return
		}
		col := scaleAlpha(src, alpha)
		draw.Draw(this.surface.img, dr, &image.Uniform{C: col}, image.Point{}, draw.Over)
	}
}

// sourceSamplePoint returns the (x, y) pixel inside state.sourceSurf
// that maps to dst pixel `dstMin`. Pattern matrices are flattened into
// `sourceX` / `sourceY` ahead of time, so this is the simple
// translate-only case (no pattern rotation / non-uniform scale).
func (this *Context) sourceSamplePoint(dstMin image.Point) image.Point {
	dx, dy := this.state.matrix.Transform(this.state.sourceX, this.state.sourceY)
	return image.Point{
		X: dstMin.X - int(math.Round(dx)),
		Y: dstMin.Y - int(math.Round(dy)),
	}
}

// scaleAlpha returns a color.RGBA at `alpha`-scaled opacity. Inputs
// can be any color.Color; we convert to 0..255 RGBA at the end.
func scaleAlpha(c color.Color, alpha float64) color.RGBA {
	r, g, b, a := c.RGBA()
	if alpha < 1 {
		a = uint32(float64(a) * alpha)
	}
	// RGBA() returns premultiplied 16-bit values; cap at uint8.
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

// --- Clip ---
//
// Clip stores an AABB in device (post-CTM) coordinates. Subsequent
// fill / stroke / paint clamp their draw destination to this rect via
// image.Rectangle.Intersect, so the rasteriser only writes inside the
// clip region. Path-shaped clipping (cairo's true behaviour) is
// approximated by AABB — same simplification glui's CairoCompat uses.

func (this *Context) Clip() {
	this.applyClip()
	this.path = this.path[:0]
}

func (this *Context) ClipPreserve() { this.applyClip() }

func (this *Context) applyClip() {
	if len(this.path) == 0 {
		// Empty path → empty clip.
		this.state.clipRect = geom.Rect{}
		this.state.hasClip = true
		return
	}
	// Path AABB in device (CTM-transformed) coordinates.
	x1, y1 := math.Inf(1), math.Inf(1)
	x2, y2 := math.Inf(-1), math.Inf(-1)
	consider := func(x, y float64) {
		tx, ty := this.state.matrix.Transform(x, y)
		if tx < x1 {
			x1 = tx
		}
		if ty < y1 {
			y1 = ty
		}
		if tx > x2 {
			x2 = tx
		}
		if ty > y2 {
			y2 = ty
		}
	}
	for _, p := range this.path {
		switch p.op {
		case 'M', 'L':
			consider(p.x1, p.y1)
		case 'C':
			consider(p.x1, p.y1)
			consider(p.x2, p.y2)
			consider(p.x3, p.y3)
		}
	}
	if math.IsInf(x1, 1) {
		// No emit-able points — clip to nothing.
		this.state.clipRect = geom.Rect{}
		this.state.hasClip = true
		return
	}
	newClip := geom.Rect{X: x1, Y: y1, Width: x2 - x1, Height: y2 - y1}
	if this.state.hasClip {
		newClip = this.state.clipRect.IntersectCopy(newClip)
	}
	this.state.clipRect = newClip
	this.state.hasClip = true
}

// drawRect computes the destination rectangle for fill/stroke/paint
// operations, intersecting the surface bounds with the active clip
// rect. Used by every drawing op so output stays inside the clip.
func (this *Context) drawRect() image.Rectangle {
	full := this.surface.img.Rect
	if !this.state.hasClip {
		return full
	}
	cr := this.state.clipRect
	cri := image.Rect(
		int(math.Floor(cr.X)),
		int(math.Floor(cr.Y)),
		int(math.Ceil(cr.X+cr.Width)),
		int(math.Ceil(cr.Y+cr.Height)),
	)
	return full.Intersect(cri)
}

func (this *Context) ResetClip() { this.state.hasClip = false }

// ClipBounds returns the clip extents in USER (post-CTM-inverse)
// space — matching libcairo's cairo_clip_extents semantics. silk's
// drawWidgetChildren intersects these with child Bounds(), which are
// also in user space, so returning device coords here would inflate
// the per-widget viewport by the HiDPI scale and overflow children.
//
// We compute the user-space AABB by transforming the four device-clip
// corners through the inverse CTM. For pure scale+translate (the
// common case) this collapses to the obvious axis-aligned answer; the
// matrix path also handles rotated CTMs correctly.
func (this *Context) ClipBounds() (x, y, w, h float64) {
	if !this.state.hasClip {
		w = float64(this.surface.width)
		h = float64(this.surface.height)
		// Convert full surface bounds back to user space.
		ux1, uy1, ok1 := this.deviceToUser(0, 0)
		ux2, uy2, ok2 := this.deviceToUser(w, h)
		if ok1 && ok2 {
			return ux1, uy1, ux2 - ux1, uy2 - uy1
		}
		return 0, 0, w, h
	}
	r := this.state.clipRect
	// Map four device corners through inverse CTM, take AABB.
	corners := [4][2]float64{
		{r.X, r.Y},
		{r.X + r.Width, r.Y},
		{r.X, r.Y + r.Height},
		{r.X + r.Width, r.Y + r.Height},
	}
	var minX, minY, maxX, maxY float64
	have := false
	for _, c := range corners {
		ux, uy, ok := this.deviceToUser(c[0], c[1])
		if !ok {
			continue
		}
		if !have {
			minX, minY, maxX, maxY = ux, uy, ux, uy
			have = true
			continue
		}
		if ux < minX {
			minX = ux
		}
		if uy < minY {
			minY = uy
		}
		if ux > maxX {
			maxX = ux
		}
		if uy > maxY {
			maxY = uy
		}
	}
	if !have {
		return 0, 0, 0, 0
	}
	return minX, minY, maxX - minX, maxY - minY
}

// deviceToUser inverts the active CTM at (dx, dy). Returns ok=false
// if the matrix is singular — should never happen for the affine
// CTMs silk produces, but kept defensive.
func (this *Context) deviceToUser(dx, dy float64) (ux, uy float64, ok bool) {
	m := &this.state.matrix
	det := m.Xx*m.Yy - m.Xy*m.Yx
	if det == 0 {
		return 0, 0, false
	}
	tx := dx - m.X0
	ty := dy - m.Y0
	ux = (m.Yy*tx - m.Xy*ty) / det
	uy = (-m.Yx*tx + m.Xx*ty) / det
	return ux, uy, true
}

func (this *Context) InClip(x, y float64) bool { return true }

// --- Group / mask ---

func (this *Context) PushGroup()                          {}
func (this *Context) PushGroupWidthContent(c Content)     {}
func (this *Context) PopGroup() *Pattern                  { return &Pattern{} }
func (this *Context) PopGroupToSource()                   {}
func (this *Context) GroupTarget() *Surface               { return this.surface }
func (this *Context) Mask(p *Pattern)                     {}
func (this *Context) MaskSurface(s *Surface, x, y float64) {}
func (this *Context) CopyPage()                           {}
func (this *Context) ShowPage()                           {}

// --- Font ---

func (this *Context) SetFontFace(f *FontFace)         {}
func (this *Context) FontFace() *FontFace             { return nil }
func (this *Context) SetFontSize(s float64)           {}
func (this *Context) SetFontMatrix(m *geom.Mat3x2)    {}
func (this *Context) FontMatrix(m *geom.Mat3x2)       {}
func (this *Context) SetFontOptions(o *FontOptions)   {}
func (this *Context) FontOptions(o *FontOptions)      {}
func (this *Context) SetScaledFont(sf *ScaledFont)    { this.state.font = sf }
func (this *Context) ScaledFont() *ScaledFont         { return this.state.font }
func (this *Context) SelectFontFace(family string, slant FontSlant, weight FontWeight) {
	face := NewToyFontFace(family, slant, weight)
	var m geom.Mat3x2
	m.InitScale(12, 12) // default 12pt; SetFontSize updates
	this.state.font = NewScaledFont(face, &m, &this.state.matrix, NewFontOptions())
}

// ShowText renders `text` starting at the current point. Each rune's
// advance moves the current point so consecutive ShowText calls
// concatenate. CTM applies to glyph positions identically to ShowGlyphs.
func (this *Context) ShowText(text string) {
	if text == "" || this.surface == nil {
		return
	}
	if this.state.font == nil {
		this.SelectFontFace("", FONT_SLANT_NORMAL, FONT_WEIGHT_NORMAL)
	}
	x, y := this.curX, this.curY
	glyphs := make([]Glyph, 0, len(text))
	cx := x
	primary := this.state.font.latinFace()
	cjk := (font.Face)(nil)
	for _, r := range text {
		adv, ok := primary.GlyphAdvance(r)
		if !ok {
			if cjk == nil {
				cjk = this.state.font.cjkFace()
			}
			adv, _ = cjk.GlyphAdvance(r)
		}
		glyphs = append(glyphs, Glyph{index: uint32(r), X: cx, Y: y})
		cx += fixedToFloat(adv)
	}
	if len(glyphs) > 0 {
		this.ShowGlyphs_hack(unsafe.Pointer(&glyphs[0]), len(glyphs))
		this.curX = cx
	}
}

// ShowGlyphs renders an explicit glyph slice. Same code path as
// ShowGlyphs_hack but takes a typed slice — used by silk's higher-level
// helpers that don't go through the unsafe-pointer dance.
func (this *Context) ShowGlyphs(glyphs []Glyph) {
	if len(glyphs) == 0 {
		return
	}
	this.ShowGlyphs_hack(unsafe.Pointer(&glyphs[0]), len(glyphs))
}

// ShowGlyphs_hack rasterises the glyph array at p (length n) using
// the active scaled font. CTM applies to each glyph's (X, Y) — that's
// the cairo convention, glyph positions live in user space and the
// rasteriser pushes them through the matrix.
//
// Glyphs whose rune isn't in the primary face flow through the CJK
// fallback face, matching libcairo's substitution behaviour.
func (this *Context) ShowGlyphs_hack(p unsafe.Pointer, n int) {
	if this.surface == nil || this.surface.img == nil || n == 0 || p == nil {
		return
	}
	glyphs := unsafe.Slice((*Glyph)(p), n)

	src := this.state.source
	if src == nil {
		src = color.RGBA{R: 0, G: 0, B: 0, A: 255}
	}
	srcImg := &image.Uniform{C: src}

	sf := this.state.font
	if sf == nil {
		// No scaled font set — synthesise a default at 12px so silk's
		// ShowText path still produces visible glyphs.
		sf = &ScaledFont{pixSize: 12}
	}

	clipR, hasClip := this.drawRect(), this.state.hasClip
	full := this.surface.img.Rect

	for _, g := range glyphs {
		tx, ty := this.state.matrix.Transform(g.X, g.Y)
		dot := fixed.Point26_6{
			X: fixed.Int26_6(math.Round(tx * 64)),
			Y: fixed.Int26_6(math.Round(ty * 64)),
		}
		face := sf.glyphFace(rune(g.index))
		dr, mask, maskp, _, ok := face.Glyph(dot, rune(g.index))
		if !ok {
			continue
		}
		drClipped := dr.Intersect(full)
		if hasClip {
			drClipped = drClipped.Intersect(clipR)
		}
		if drClipped.Empty() {
			continue
		}
		mp := image.Point{
			X: maskp.X + (drClipped.Min.X - dr.Min.X),
			Y: maskp.Y + (drClipped.Min.Y - dr.Min.Y),
		}
		draw.DrawMask(this.surface.img, drClipped, srcImg, image.Point{}, mask, mp, draw.Over)
	}
}

func (this *Context) FontExtents() *FontExtents {
	if this.state.font != nil {
		return this.state.font.FontExtents()
	}
	return &FontExtents{Ascent: 10, Descent: 3, Height: 14, MaxXAdvance: 8}
}

func (this *Context) TextExtents(text string) *TextExtents {
	if this.state.font != nil {
		return this.state.font.TextExtents(text)
	}
	w := float64(len(text)) * 7
	return &TextExtents{Width: w, Height: 12, XAdvance: w}
}

func (this *Context) GlyphExtents(glyphs []Glyph) *TextExtents {
	if this.state.font != nil {
		return this.state.font.GlyphExtents(glyphs)
	}
	return &TextExtents{}
}

// ===== Helpers =====

func f64ColorRGB(r, g, b, a float64) color.Color {
	return color.RGBA{
		R: clamp8(r),
		G: clamp8(g),
		B: clamp8(b),
		A: clamp8(a),
	}
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(v * 255)
}

// Multiplygeom.Mat3x2 mirrors the original cairo.go helper used by
// silk (the misnamed "Multiplygeom.Mat3x2" function).
func Multiplygeom_Mat3x2(result, a, b *geom.Mat3x2) {
	*result = a.Multiply(b)
}

// errStub is reserved for any future error returns from this stub.
var _ = errors.New
