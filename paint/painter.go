package paint

import (
	"math"
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/uk0/silk/cairo"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
)

// Atomic because the decrement happens in a finalizer, which the runtime runs
// on its own goroutine concurrently with whatever thread is allocating.
var cairoPainterCount atomic.Int64

// 此函数为四舍五入操作
// 注: 此函数只用于实现"对齐到像素"功能, 不能正确处理nan,inf等非法值
func Round(x float64) float64 {
	if x < 0.0 {
		return float64(int(x - 0.5))
	} else if x > 0.0 {
		return float64(int(x + 0.5))
	} else {
		return 0
	}
}

type Painter interface {
	Target() Surface

	Save() int
	Restore() int
	RestoreTo(int) bool
	CurrentState() int

	CurrentPoint() (x, y float64)

	Arc(xc, yc, radius, angle1, angle2 float64)
	ArcNegative(xc, yc, radius, angle1, angle2 float64)
	CurveTo(x1, y1, x2, y2, x3, y3 float64)
	Line(x1, y1, x2, y2 float64)
	LineTo(x, y float64)
	MoveTo(x, y float64)
	Rectangle(x, y, width, height float64)
	Rectangle1(rect geom.Rect)

	Stroke()
	StrokePreserve()

	// 填充
	Fill()
	FillPreserve()

	// 在整个剪裁区域内填充
	Paint()
	PaintWithAlpha(alpha uint8)

	ResetClip()
	Clip()
	ClipPreserve()
	ClipBounds() (x, y, width, height float64)
	ClipBounds1() geom.Rect

	SetOperator(op Operator)

	ResetMatrix()
	Translate(tx, ty float64)
	Scale(sx, sy float64)
	Rotate(radians float64)
	Transform(m *geom.Mat3x2)
	SetMatrix(m *geom.Mat3x2)
	GetMatrix(m *geom.Mat3x2)

	SetPen(pen Pen)
	SetPen1(cr Color, width float64)
	//SetSimplePen4(width float64, r, g, b uint8)
	//SetSimplePen5(width float64, r, g, b, a uint8)

	SetBrush(br Brush)
	SetBrush1(cr Color)
	//SetBrush13(r, g, b uint8)
	//SetBrush1(r, g, b, a uint8)

	SetFont(f Font)
	Font() Font

	//SetSourcePattern(pat *cairo.Pattern)
	//SetSourceColor(cr Color)
	//SetSourceRGBA(r, g, b, a uint8)
	//SetSourceRGB(r, g, b uint8)
	//SetLineWidth(w float64)

	//SetSourceSurface(src Surface, xSrc, ySrc float64)

	//FontExtents() *FontExtents
	//TextExtents(text string) *TextExtents

	ScaledFont() ScaledFont

	DrawText(text string)
	DrawText1(x, y float64, text string)
	DrawGlyphs(glyphs []Glyph)
	DrawGlyph(glyph *Glyph)
	//GlyphExtents(glyphs []Glyph) *TextExtents

	//TextToGlyphs(x, y float64, text string) []Glyph

	DrawPixmap(pixmap Pixmap)
	DrawPixmap1(x, y float64, pixmap Pixmap)
	DrawPixmap2(x, y float64, pixmap Pixmap, x0, y0 float64)
	DrawPixmap5(x, y, w, h float64, pixmap Pixmap)

	DrawIcon(ico Icon, fSize float64, grayed bool)
	DrawIcon1(ico Icon, x, y, fSize float64, grayed bool)
}

type painterStateEx struct {
	pen   Pen
	brush Brush
	font  *font
}

type cairoPainter struct {
	cairo *cairo.Context

	state      painterStateEx
	stateStack []painterStateEx

	sfont *scaledFont

	// glyphBuf is a reusable scratch buffer for DrawText/DrawText1 glyph
	// shaping. Text drawn every frame (labels, list rows) reshapes the same
	// string repeatedly; reusing this buffer avoids a per-call []Glyph
	// allocation. Safe because DrawGlyphs consumes the glyphs synchronously
	// and nothing retains them, and painters are single-threaded during paint.
	glyphBuf []Glyph

	// solid is a reusable SolidBrush for SetBrush1 / SetBrush(Color), the color
	// setter every widget calls before each fill. Pointing state.brush at this
	// shared field instead of allocating a fresh *SolidBrush per call removes a
	// pervasive per-fill allocation. Safe because Restore() does not read
	// Go-side state.brush back (it relies on the next Set* before drawing), so
	// the shared brush is never aliased across a save boundary; if Restore ever
	// starts restoring state.brush, this must switch to storing the color by
	// value in painterStateEx.
	solid SolidBrush

	// penScratch is the SetPen1 analogue of solid: a reusable pen so the
	// stroke-color setter (SetPen1, called on every bordered/lined draw)
	// does not allocate a fresh *pen per call. Same safety rationale as solid.
	penScratch pen

	surface Surface
	//saveNum int
}

func (this *cairoPainter) setFinalizer() {
	n := cairoPainterCount.Add(1)
	//core.Warn("y")
	//fmt.Println("cairoPainterCount =", n)
	if n > 1500 && n%100 == 0 {
		core.Warn("seems cairo painter leaks, count = ", n)
	}
	runtime.SetFinalizer(this, func(p *cairoPainter) {
		if p.CurrentState() != 0 {
			core.Warn("unbalance save/restore of g: depth =", p.CurrentState())
		}
		p.releaseNative()
	})
}

// ClosePainter releases an exclusively owned painter immediately. It does
// not change Painter's interface, so custom painters remain compatible. The
// caller must not draw through any aliases afterwards. Close painters before
// closing their target surfaces; finalizers remain a safety net for old hosts.
func ClosePainter(p Painter) {
	if native, ok := p.(*cairoPainter); ok && native != nil {
		runtime.SetFinalizer(native, nil)
		native.releaseNative()
	}
}

func (p *cairoPainter) releaseNative() {
	if p.cairo == nil {
		return
	}
	p.cairo.Destroy()
	p.cairo = nil
	p.sfont = nil
	p.surface = nil
	p.state = painterStateEx{}
	p.stateStack = nil
	p.glyphBuf = nil
	cairoPainterCount.Add(-1)
}

func (this *cairoPainter) SetFont(f Font) {
	if f == nil {
		f = NewFont("宋体", 9, false, false)
	}
	this.state.font = f.(*font)
	this.sfont = nil
}

func (this *cairoPainter) Font() Font {
	if this.state.font == nil {
		this.SetFont(nil) // 使用默认字体
	}
	return this.state.font
}

func (this *cairoPainter) SetBrush(br Brush) {
	switch p := br.(type) {
	case *SolidBrush:
	case *PixmapBrush:
	case *LinearGradient:
	case *RadialGradient:
	case nil:
	case Color:
		this.SetBrush1(p)
		return
	default:
		core.Warn(core.TypeErr(this.state.brush))
		return
	}
	this.state.brush = br
}

func (this *cairoPainter) SetBrush1(cr Color) {
	// Reuse the painter's embedded solid brush instead of allocating a fresh
	// *SolidBrush every call (see the solid field's doc for why this is safe).
	this.solid.Color = cr
	this.state.brush = &this.solid
}

func (this *cairoPainter) applyBrush() {
	switch p := this.state.brush.(type) {
	case *SolidBrush:
		this.cairo.SetSourceRGBA(p.Color.NRGBAf())
	case *PixmapBrush:
		this.cairo.SetSource(p.pat)
	case *LinearGradient:
		this.cairo.SetSource(p.cairoPattern())
	case *RadialGradient:
		this.cairo.SetSource(p.cairoPattern())
	case nil:
		this.cairo.SetSourceRGBA(0, 0, 0, 0)
	default:
		core.Warn(core.TypeErr(this.state.brush))
	}
}

func (this *cairoPainter) SetPen(pen Pen) {
	this.state.pen = pen
}

func (this *cairoPainter) applyPen() {
	if this.state.pen == nil {
		this.cairo.SetLineWidth(0)
		this.cairo.SetSourceRGBA(0, 0, 0, 0)
		this.cairo.SetDash(cairo.Dash{}) // clear any prior dash
		return
	}

	this.cairo.SetSourceRGBA(this.state.pen.Color().NRGBAf())

	w := this.state.pen.Width()

	if w == 0 {
		// 0宽度为发丝线, 无论缩放率是多少, 线宽永远是一个像素
		var m geom.Mat3x2
		this.GetMatrix(&m)
		scale := math.Sqrt(m.Det())
		//core.Debug("scale=", scale)
		//core.Debug("Xx=", m.Xx)
		//core.Debug("Yy=", m.Yy)
		w = 1 / scale
	}

	this.cairo.SetLineWidth(w)

	// DashedPen: dashed-stroke contract mirrors the SVG/PDF painters.
	// An empty / nil pattern resets to a solid stroke so the cairo
	// context doesn't carry dash state from a previous styled pen.
	if dp, ok := this.state.pen.(DashedPen); ok {
		this.cairo.SetDash(cairo.Dash{Dashes: dp.Dash(), Offset: dp.DashOffset()})
	} else {
		this.cairo.SetDash(cairo.Dash{})
	}

	// CappedPen: paint.LineCap and cairo.LineCap agree on enum order
	// (Butt=0/Round=1/Square=2) so a direct cast is faithful, but
	// paint.LineJoin (Miter/Bevel/Round) and cairo.LineJoin
	// (MITER/ROUND/BEVEL) disagree at indices 1 and 2 — a translation
	// table is required for joins.
	if cp, ok := this.state.pen.(CappedPen); ok {
		this.cairo.SetLineCap(cairo.LineCap(cp.LineCap()))
		this.cairo.SetLineJoin(cairoLineJoin(cp.LineJoin()))
	} else {
		this.cairo.SetLineCap(cairo.LINE_CAP_BUTT)
		this.cairo.SetLineJoin(cairo.LINE_JOIN_MITER)
	}
}

// cairoLineJoin maps the paint package's LineJoin enum (Miter, Bevel,
// Round) onto cairo's enum (MITER, ROUND, BEVEL). The paint side was
// declared in alphabetical order; cairo follows the C library, which
// chose visual order (MITER → softer ROUND → flat BEVEL). Without
// this remap, asking for paint.LineJoinBevel would render with cairo's
// ROUND join.
func cairoLineJoin(j LineJoin) cairo.LineJoin {
	switch j {
	case LineJoinBevel:
		return cairo.LINE_JOIN_BEVEL
	case LineJoinRound:
		return cairo.LINE_JOIN_ROUND
	default: // LineJoinMiter and any unknown value
		return cairo.LINE_JOIN_MITER
	}
}

//func (this *cairoPainter) SetSourcePattern(pat *cairo.Pattern) {
//	this.cairo.SetSource(pat)
//}

//func (this *cairoPainter) SetSourceColor(cr Color) {
//	this.cairo.SetSourceRGBA(cr.NRGBAf())
//}

//func (this *cairoPainter) SetSourceRGBA(r, g, b, a uint8) {
//	this.cairo.SetSourceRGBA(float64(r)/255.0, float64(g)/255.0, float64(b)/255.0, float64(a)/255.0)
//}

//func (this *cairoPainter) SetSourceRGB(r, g, b uint8) {
//	this.cairo.SetSourceRGB(float64(r)/255.0, float64(g)/255.0, float64(b)/255.0)
//}

func (this *cairoPainter) Target() Surface {
	//core.Warn("a")
	return this.surface
}

func (this *cairoPainter) SetSourceSurface(src Surface, xSrc, ySrc float64) {
	switch p := src.(type) {
	case *cairoSurface:
		this.cairo.SetSourceSurface(p.Surface, xSrc, ySrc)
	default:
		core.Warn(core.TypeErr(src))
	}
}

//func (this *cairoPainter) TextExtents(text string) *TextExtents {
//	return (*TextExtents)(this.Context.TextExtents(text))
//}

func (this *cairoPainter) DrawIcon1(ico Icon, x, y, fSize float64, grayed bool) {
	this.Translate(x, y)
	this.DrawIcon(ico, fSize, grayed)
	this.Translate(-x, -y)
}

// iconDeviceSize chooses source resolution, not layout size. The largest
// singular value covers rotation, non-uniform scale and shear. Bound custom
// Icon.Pixmap allocations even if a caller supplies an extreme zoom transform.
func iconDeviceSize(size float64, m geom.Mat3x2) int {
	a, b := math.Hypot(m.Xx+m.Yy, m.Yx-m.Xy), math.Hypot(m.Xx-m.Yy, m.Yx+m.Xy)
	pixels := size * (a + b) / 2
	if pixels <= 0 || math.IsNaN(pixels) || math.IsInf(pixels, 0) {
		return 0
	}
	return int(math.Min(4096, math.Ceil(pixels-1e-9)))
}

func (this *cairoPainter) DrawIcon(ico Icon, fSize float64, grayed bool) {
	if ico == nil || ico.IsAir() || fSize <= 0 || math.IsNaN(fSize) || math.IsInf(fSize, 0) {
		return
	}
	if x, ok := ico.(*icon); ok {
		if override := satinOverride(x.name); override != nil {
			ico = override
		}
	}
	if x, ok := ico.(*icon); ok && x.draw != nil {
		drawProceduralIcon(this.cairo, x.draw, fSize, grayed)
		return
	}
	var ctm geom.Mat3x2
	this.cairo.GetMatrix(&ctm)
	size := iconDeviceSize(fSize, ctm)
	if size <= 0 {
		return
	}
	var img *cairoSurface
	var pat *cairo.Pattern
	var retainedSub *subIcon
	switch x := ico.(type) {
	case *atlasIcon:
		sub := x.getSub(size)
		retainedSub = sub
		img = sub.img
		if sub.pat == nil {
			sub.pat = cairo.NewPatternForSurface(img.Surface)
			runtime.SetFinalizer(sub, func(s *subIcon) { s.pat.Destroy() })
		}
		pat = sub.pat
	case *icon:
		sub := x.getNearest(size)
		retainedSub = sub
		if sub == nil {
			return
		}
		img = sub.img
		if sub.pat == nil {
			sub.pat = cairo.NewPatternForSurface(img.Surface)
			runtime.SetFinalizer(sub, func(s *subIcon) { s.pat.Destroy() })
		}
		pat = sub.pat
	default:
		var ok bool
		img, ok = ico.Pixmap(size).(*cairoSurface)
		if !ok || img == nil || img.Width() <= 0 || img.Height() <= 0 {
			return
		}
		pat = cairo.NewPatternForSurface(img.Surface)
		defer pat.Destroy()
	}
	// Keep the requested fractional logical extent: rounding it to an integer
	// enlarges small icons and adds a second resampling step at fractional DPI.
	var m geom.Mat3x2
	m.InitScale(float64(img.Width())/fSize, float64(img.Height())/fSize)
	pat.SetMatrix(&m)
	this.cairo.Save()
	this.cairo.SetSource(pat)
	if grayed {
		// Alpha-mask with a neutral theme color; HSL on a transparent
		// destination leaked the original saturated bitmap colors.
		this.cairo.SetSourceRGBA(currentIconPalette.Disabled.NRGBAf())
		this.cairo.Mask(pat)
	} else {
		this.cairo.Paint()
	}
	this.cairo.Restore()
	runtime.KeepAlive(retainedSub) // cache eviction must not finalize a pattern still in use
}

func (this *cairoPainter) DrawText(text string) {
	sf := this.applyFont()
	this.glyphBuf = sf.textToGlyphsInto(0, 0, text, this.glyphBuf[:0])
	// Most native UI/icon fonts already cover their text. Inspect the glyphs
	// we needed to shape anyway, rather than resolving/measuring extra runs
	// on every Chinese menu repaint.
	if needsTextFallback(text) && textHasMissingGlyph(this.glyphBuf) {
		this.drawFallbackText(sf, text)
		return
	}
	this.DrawGlyphs(this.glyphBuf)
}

func (this *cairoPainter) DrawText1(x, y float64, text string) {
	this.Translate(x, y)
	this.DrawText(text)
	this.Translate(-x, -y)
}

func (this *cairoPainter) DrawGlyphs(glyphs []Glyph) {
	this.applyFont()
	this.applyBrush()
	n := len(glyphs)
	if n == 0 {
		return
	}
	//	this.applyFont()
	this.cairo.ShowGlyphs_hack(unsafe.Pointer(&glyphs[0]), n)
}

func (this *cairoPainter) applyFont() *scaledFont {
	if this.sfont == nil {
		this.Font()
		var ctm geom.Mat3x2
		this.cairo.GetMatrix(&ctm)
		this.sfont = this.state.font.scaledFont(&ctm)
		this.cairo.SetScaledFont(this.sfont.ScaledFont)
	}
	return this.sfont
}

func (this *cairoPainter) Save() (ret int) {
	ret = len(this.stateStack)
	//this.saveNum++
	this.stateStack = append(this.stateStack, this.state)
	this.cairo.Save()
	return
}

func (this *cairoPainter) Restore() (ret int) {
	ret = len(this.stateStack) - 1

	if ret >= 0 {
		this.cairo.Restore()
		this.stateStack = this.stateStack[:ret]
		this.sfont = nil
	} else {
		core.Warn("unbalance save/restore of g: depth =", ret)
	}
	return
}

func (this *cairoPainter) RestoreTo(n int) bool {
	if n < 0 || n > len(this.stateStack) {
		core.Warn("try to restore to invalid state:", n)
		return false
	}

	for len(this.stateStack) > n {
		this.Restore()
	}
	return true
}

func (this *cairoPainter) CurrentState() int {
	return len(this.stateStack)
}

func (this *cairoPainter) ScaledFont() ScaledFont {
	return this.applyFont()
}

func (this *cairoPainter) SetOperator(op Operator) {
	this.cairo.SetOperator(cairo.Operator(op))
}

func (this *cairoPainter) DrawGlyph(glyph *Glyph) {
	this.applyFont()
	this.applyBrush()
	this.cairo.ShowGlyphs_hack(unsafe.Pointer(glyph), 1)
}

func (this *cairoPainter) PaintWithAlpha(alpha uint8) {
	this.applyBrush()
	this.cairo.PaintWithAlpha(float64(alpha) / 255.0)
}

func (this *cairoPainter) ClipBounds() (x, y, width, height float64) {
	return this.cairo.ClipBounds()
}

func (this *cairoPainter) ClipBounds1() geom.Rect {
	x, y, width, height := this.cairo.ClipBounds()
	return geom.Rect{x, y, width, height}
}

//func (this *cairoPainter) Rectangle1(rect geom.Rect) {
//	this.cairo.Rectangle(rect.X, rect.Y, rect.Width, rect.Height)
//}

func (this *cairoPainter) Clip() {
	this.cairo.Clip()
}

func (this *cairoPainter) ClipPreserve() {
	this.cairo.ClipPreserve()
}

func (this *cairoPainter) Fill() {
	this.applyBrush()
	this.cairo.Fill()
}

func (this *cairoPainter) FillPreserve() {
	this.applyBrush()
	this.cairo.FillPreserve()
}

func (this *cairoPainter) SetMatrix(m *geom.Mat3x2) {
	this.cairo.SetMatrix(m)
}

func (this *cairoPainter) GetMatrix(m *geom.Mat3x2) {
	this.cairo.GetMatrix(m)
}

func (this *cairoPainter) Line(x1, y1, x2, y2 float64) {
	this.cairo.Line(x1, y1, x2, y2)
}

func (this *cairoPainter) LineTo(x, y float64) {
	this.cairo.LineTo(x, y)
}

func (this *cairoPainter) MoveTo(x, y float64) {
	this.cairo.MoveTo(x, y)
}

func (this *cairoPainter) Rectangle(x, y, width, height float64) {
	this.cairo.Rectangle(x, y, width, height)
}

func (this *cairoPainter) Rectangle1(rect geom.Rect) {
	this.cairo.Rectangle(rect.X, rect.Y, rect.Width, rect.Height)
}

func (this *cairoPainter) CurrentPoint() (x, y float64) {
	return this.cairo.CurrentPoint()
}

func (this *cairoPainter) Arc(xc, yc, radius, angle1, angle2 float64) {
	this.cairo.Arc(xc, yc, radius, angle1, angle2)
}

func (this *cairoPainter) ArcNegative(xc, yc, radius, angle1, angle2 float64) {
	this.cairo.ArcNegative(xc, yc, radius, angle1, angle2)
}

func (this *cairoPainter) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	this.cairo.CurveTo(x1, y1, x2, y2, x3, y3)
}

func (this *cairoPainter) Paint() {
	this.applyBrush()
	this.cairo.Paint()
}

func (this *cairoPainter) Stroke() {
	this.applyPen()
	this.cairo.Stroke()
}

func (this *cairoPainter) StrokePreserve() {
	this.applyPen()
	this.cairo.StrokePreserve()
}

func (this *cairoPainter) ResetClip() {
	this.cairo.ResetClip()
}

func (this *cairoPainter) ResetMatrix() {
	this.cairo.ResetMatrix()
}

func (this *cairoPainter) Translate(tx, ty float64) {
	this.cairo.Translate(tx, ty)

}

func (this *cairoPainter) Scale(sx, sy float64) {
	this.cairo.Scale(sx, sy)
}

func (this *cairoPainter) Rotate(radians float64) {
	this.cairo.Rotate(radians)

}

func (this *cairoPainter) Transform(m *geom.Mat3x2) {
	this.cairo.Transform(m)
}

func (this *cairoPainter) DrawPixmap5(x, y, w, h float64, pixmap Pixmap) {
	cs := pixmap.(*cairoSurface).Surface
	pat := cairo.NewPatternForSurface(cs)
	var m geom.Mat3x2
	sx := float64(pixmap.Width()) / w
	sy := float64(pixmap.Height()) / h
	m.InitScale(sx, sy)
	pat.SetMatrix(&m)
	this.cairo.SetSource(pat)
	this.cairo.Paint()
}

func (this *cairoPainter) DrawPixmap(pixmap Pixmap) {
	cs := pixmap.(*cairoSurface).Surface
	this.cairo.SetSourceSurface(cs, 0, 0)
	this.cairo.Paint()
}

func (this *cairoPainter) DrawPixmap1(x, y float64, pixmap Pixmap) {
	this.Translate(x, y)
	cs := pixmap.(*cairoSurface).Surface
	this.cairo.SetSourceSurface(cs, 0, 0)
	this.cairo.Paint()
	this.Translate(-x, -y)
}

func (this *cairoPainter) DrawPixmap2(x, y float64, pixmap Pixmap, x0, y0 float64) {
	this.Translate(x, y)
	cs := pixmap.(*cairoSurface).Surface
	this.cairo.SetSourceSurface(cs, x0, y0)
	this.cairo.Paint()
	this.Translate(-x, -y)
}

//func (this *cairoPainter) SetBrush1(r, g, b, a uint8) {
//	this.SetBrush1(Color{r, g, b, a})
//}

//func (this *cairoPainter) SetBrush13(r, g, b uint8) {
//	this.SetBrush1(Color{r, g, b, 255})
//}

func (this *cairoPainter) SetPen1(cr Color, width float64) {
	// Reuse the painter's embedded pen instead of allocating a fresh *pen every
	// call (same safety rationale as SetBrush1 / the solid field).
	this.penScratch.color = cr
	this.penScratch.width = width
	this.state.pen = &this.penScratch
}

//func (this *cairoPainter) SetSimplePen4(width float64, r, g, b uint8) {
//	this.SetPen(NewPen4(width, r, g, b))

//}

//func (this *cairoPainter) SetSimplePen5(width float64, r, g, b, a uint8) {
//	this.SetPen(NewPen5(width, r, g, b, a))
//}
