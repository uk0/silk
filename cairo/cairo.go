//go:build !silk_pure_go

package cairo

// #include <cairo/cairo.h>
// #include <stdlib.h>
// #include <memory.h>
// #include <string.h>
// int writeFuncBound(void *closure, void*data, unsigned int length);
// int readFuncBound(void *closure, void*data, unsigned int length);
import "C"

import (
	//"silk/diag"
	"silk/geom"
	"errors"
	"image"
	"image/draw"
	"io"
	"math"
	"os"
	"runtime"
	"unsafe"
)

func init() {
	if unsafe.Sizeof(C.cairo_glyph_t{}) != unsafe.Sizeof(Glyph{}) {
		panic("size missmatch: C.cairo_glyph_t and Glyph")
	}
	if unsafe.Sizeof(C.cairo_text_cluster_t{}) != unsafe.Sizeof(TextCluster{}) {
		panic("size missmatch: C.cairo_text_cluster_t and TextCluster")
	}
}

//export writeFunc
func writeFunc(closure, data unsafe.Pointer, length C.uint) C.int {
	w := *(*io.Writer)(closure)
	n := int(length)
	nw, err := w.Write((*[1 << 30]byte)(data)[0:n])
	if err != nil || nw != n {
		return C.int(STATUS_WRITE_ERROR)
	}
	return C.int(STATUS_SUCCESS)
}

//export readFunc
func readFunc(closure, data unsafe.Pointer, length C.uint) C.int {
	r := *(*io.Reader)(closure)
	n := int(length)
	nr, err := r.Read((*[1 << 30]byte)(data)[0:n])
	if err != nil || nr != n {
		return C.int(STATUS_READ_ERROR)
	}
	return C.int(STATUS_SUCCESS)
}

// ----------------------------------------------------------------
//type geom.Mat3x2 geom.Mat3x2

func matrix_t(m *geom.Mat3x2) *C.cairo_matrix_t {
	return (*C.cairo_matrix_t)(unsafe.Pointer(m))
}

// ----------------------------------------------------------------
type Status int32

const (
	STATUS_SUCCESS Status = iota
	STATUS_NO_MEMORY
	STATUS_INVALID_RESTORE
	STATUS_INVALID_POP_GROUP
	STATUS_NO_CURRENT_POINT
	STATUS_INVALID_MATRIX
	STATUS_INVALID_STATUS
	STATUS_NULL_POINTER
	STATUS_INVALID_STRING
	STATUS_INVALID_PATH_DATA
	STATUS_READ_ERROR
	STATUS_WRITE_ERROR
	STATUS_SURFACE_FINISHED
	STATUS_SURFACE_TYPE_MISMATCH
	STATUS_PATTERN_TYPE_MISMATCH
	STATUS_INVALID_CONTENT
	STATUS_INVALID_FORMAT
	STATUS_INVALID_VISUAL
	STATUS_FILE_NOT_FOUND
	STATUS_INVALID_DASH
	STATUS_INVALID_DSC_COMMENT
	STATUS_INVALID_INDEX
	STATUS_CLIP_NOT_REPRESENTABLE
	STATUS_TEMP_FILE_ERROR
	STATUS_INVALID_STRIDE
	STATUS_FONT_TYPE_MISMATCH
	STATUS_USER_FONT_IMMUTABLE
	STATUS_USER_FONT_ERROR
	STATUS_NEGATIVE_COUNT
	STATUS_INVALID_CLUSTERS
	STATUS_INVALID_SLANT
	STATUS_INVALID_WEIGHT
	STATUS_INVALID_SIZE
	STATUS_USER_FONT_NOT_IMPLEMENTED
	STATUS_DEVICE_TYPE_MISMATCH
	STATUS_DEVICE_ERROR
	STATUS_INVALID_MESH_CONSTRUCTION
	STATUS_DEVICE_FINISHED
	STATUS_LAST_STATUS
)

func (s Status) String() string {
	return C.GoString(C.cairo_status_to_string(C.cairo_status_t(s)))
}

// ----------------------------------------------------------------
//type Context struct {
//	c  *C.cairo_t
//	f  *font
//	sf *ScaledFont
//}

type Context C.cairo_t

func cairo_t(p *Context) *C.cairo_t {
	return (*C.cairo_t)(p)
}

func (this *Context) Destroy() {
	C.cairo_destroy(cairo_t(this))
}

//func newContext(c *C.cairo_t) *Context {
//	e := Status(C.cairo_status(c))
//	if e != STATUS_SUCCESS {
//		core.Warn(e)
//	}
//	p := &Context{c, nil, nil}
//	runtime.SetFinalizer(p, destoryContext)
//	debugContextCount++
//	core.MoreDebug("newContext(): ", p)
//	return p
//}

func (this *Context) Status() Status {
	return Status(C.cairo_status(cairo_t(this)))
}

/*func (this *Context) applyFont() {
	if this.sf == nil {
		this.Font()
		var ctm geom.Mat3x2
		this.Getgeom.Mat3x2(&ctm)
		this.sf = this.f.getScaledFont(&ctm)
		this.SetScaledFont(this.sf)
	}
}
*/

func (this *Context) Save() {
	C.cairo_save(cairo_t(this))
}

func (this *Context) Restore() {
	C.cairo_restore(cairo_t(this))
}

func (this *Context) Target() *Surface {
	s := C.cairo_get_target(cairo_t(this))
	return (*Surface)(s)
}

func (this *Context) PushGroup() {
	C.cairo_push_group(cairo_t(this))
}

func (this *Context) PushGroupWidthContent(c Content) {
	C.cairo_push_group_with_content(cairo_t(this), C.cairo_content_t(c))
}

func (this *Context) PopGroup() *Pattern {
	p := C.cairo_pop_group(cairo_t(this))
	return (*Pattern)(p)
}

func (this *Context) PopGroupToSource() {
	C.cairo_pop_group_to_source(cairo_t(this))
}

func (this *Context) GroupTarget() *Surface {
	s := C.cairo_get_group_target(cairo_t(this))
	return (*Surface)(s)
}

func (this *Context) SetSourceRGB(r, g, b float64) {
	C.cairo_set_source_rgb(cairo_t(this), C.double(r), C.double(g), C.double(b))
}

func (this *Context) SetSourceRGBA(r, g, b, a float64) {
	C.cairo_set_source_rgba(cairo_t(this), C.double(r), C.double(g), C.double(b), C.double(a))
}

func (this *Context) SetSource(p *Pattern) {
	C.cairo_set_source(cairo_t(this), pattern_t(p))
}

func (this *Context) SetSourceSurface(s *Surface, x, y float64) {
	C.cairo_set_source_surface(cairo_t(this), surface_t(s), C.double(x), C.double(y))
}

func (this *Context) Source() *Pattern {
	p := C.cairo_get_source(cairo_t(this))
	C.cairo_pattern_reference(p)
	return (*Pattern)(p)
}

func (this *Context) SetAntialias(a Antialias) {
	C.cairo_set_antialias(cairo_t(this), C.cairo_antialias_t(a))
}

func (this *Context) Antialias() Antialias {
	return Antialias(C.cairo_get_antialias(cairo_t(this)))
}

func (this *Context) SetDash(d Dash) {

	if len(d.Dashes) == 0 {
		C.cairo_set_dash(cairo_t(this), (*C.double)(nil), C.int(0), C.double(0))
		return
	}
	p := (*C.double)(&d.Dashes[0])
	C.cairo_set_dash(cairo_t(this), p, C.int(len(d.Dashes)), C.double(d.Offset))
}

func (this *Context) Dash() Dash {
	n := C.cairo_get_dash_count(cairo_t(this))
	if n == 0 {
		return Dash{}
	}
	d := Dash{}
	d.Dashes = make([]float64, n)
	p := (*C.double)(&d.Dashes[0])
	C.cairo_get_dash(cairo_t(this), p, (*C.double)(&d.Offset))
	return d
}

func (this *Context) SetFillRule(rule FillRule) {
	C.cairo_set_fill_rule(cairo_t(this), C.cairo_fill_rule_t(rule))
}

func (this *Context) FillRule() FillRule {
	return FillRule(C.cairo_get_fill_rule(cairo_t(this)))
}

func (this *Context) SetLineCap(c LineCap) {
	C.cairo_set_line_cap(cairo_t(this), C.cairo_line_cap_t(c))
}

func (this *Context) LineCap() LineCap {
	return LineCap(C.cairo_get_line_cap(cairo_t(this)))
}

func (this *Context) SetLineJoin(j LineJoin) {
	C.cairo_set_line_join(cairo_t(this), C.cairo_line_join_t(j))
}

func (this *Context) LineJoin() LineJoin {
	return LineJoin(C.cairo_get_line_join(cairo_t(this)))
}

func (this *Context) SetLineWidth(w float64) {
	C.cairo_set_line_width(cairo_t(this), C.double(w))
}

func (this *Context) LineWidth() float64 {
	return float64(C.cairo_get_line_width(cairo_t(this)))
}

func (this *Context) SetMiterLimit(v float64) {
	C.cairo_set_miter_limit(cairo_t(this), C.double(v))
}

func (this *Context) MiterLimit() float64 {
	return float64(C.cairo_get_miter_limit(cairo_t(this)))
}

func (this *Context) SetOperator(v Operator) {
	C.cairo_set_operator(cairo_t(this), C.cairo_operator_t(v))
}

func (this *Context) Operator() Operator {
	return Operator(C.cairo_get_operator(cairo_t(this)))
}

func (this *Context) SetTolerance(v float64) {
	C.cairo_set_tolerance(cairo_t(this), C.double(v))
}

func (this *Context) Tolerance() float64 {
	return float64(C.cairo_get_tolerance(cairo_t(this)))
}

func (this *Context) Clip() {
	C.cairo_clip(cairo_t(this))
}

func (this *Context) ClipPreserve() {
	C.cairo_clip_preserve(cairo_t(this))
}

func (this *Context) ClipBounds() (x, y, width, height float64) {
	var x2, y2 float64
	C.cairo_clip_extents(cairo_t(this), (*C.double)(&x), (*C.double)(&y), (*C.double)(&x2), (*C.double)(&y2))
	width = x2 - x
	height = y2 - y
	return
}

func (this *Context) InClip(x, y float64) bool {
	return C.cairo_in_clip(cairo_t(this), C.double(x), C.double(y)) != 0
}

func (this *Context) ResetClip() {
	C.cairo_reset_clip(cairo_t(this))
}

//func (this *Context) SetBrush(br paint.Brush) {
//	switch x := br.(type) {
//	case *paint.SolidBrush:
//		this.SetSourceRGBA(x.Color.NRGBAf())
//	}
//}

func (this *Context) Fill() {
	C.cairo_fill(cairo_t(this))
}

func (this *Context) FillPreserve() {
	C.cairo_fill_preserve(cairo_t(this))
}

func (this *Context) FillExtens() (x1, y1, x2, y2 float64) {
	C.cairo_fill_extents(cairo_t(this), (*C.double)(&x1), (*C.double)(&y1), (*C.double)(&x2), (*C.double)(&y2))
	return
}

func (this *Context) InFill(x, y float64) bool {
	return C.cairo_in_fill(cairo_t(this), C.double(x), C.double(y)) != 0
}

func (this *Context) Mask(p *Pattern) {
	C.cairo_mask(cairo_t(this), pattern_t(p))
}

func (this *Context) MaskSurface(s *Surface, x, y float64) {
	C.cairo_mask_surface(cairo_t(this), surface_t(s), C.double(x), C.double(y))
}

func (this *Context) Paint() {
	C.cairo_paint(cairo_t(this))
}

func (this *Context) PaintWithAlpha(alpha float64) {
	C.cairo_paint_with_alpha(cairo_t(this), C.double(alpha))
}

func (this *Context) Stroke() {
	C.cairo_stroke(cairo_t(this))
}

func (this *Context) StrokePreserve() {
	C.cairo_stroke_preserve(cairo_t(this))
}

func (this *Context) StrokeExtens() (x1, y1, x2, y2 float64) {
	C.cairo_stroke_extents(cairo_t(this), (*C.double)(&x1), (*C.double)(&y1), (*C.double)(&x2), (*C.double)(&y2))
	return
}

func (this *Context) InStroke(x, y float64) bool {
	return C.cairo_in_stroke(cairo_t(this), C.double(x), C.double(y)) != 0
}

func (this *Context) CopyPage() {
	C.cairo_copy_page(cairo_t(this))
}

func (this *Context) ShowPage() {
	C.cairo_show_page(cairo_t(this))
}

func (this *Context) Line(x0, y0, x1, y1 float64) {
	cx, cy := this.CurrentPoint()
	if cx != x0 || cy != y0 {
		this.MoveTo(x0, y0)
	}
	this.LineTo(x1, y1)
}

func (this *Context) RoundRect(x, y, width, height, r float64) {
	if r < 0 {
		r = 0
	}

	if r > width*0.5 {
		r = width * 0.5
	}

	if r > height*0.5 {
		r = height * 0.5
	}

	if r == 0 {
		this.Rectangle(x, y, width, height)
		return
	}

	this.NewSubPath()
	this.Arc(x+r, y+r, r, math.Pi, -math.Pi*0.5)
	this.Arc(x+width-r, y+r, r, -math.Pi*0.5, 0)
	this.Arc(x+width-r, y+height-r, r, 0, math.Pi*0.5)
	this.Arc(x+r, y+height-r, r, math.Pi*0.5, math.Pi)
	this.ClosePath()

}

func (this *Context) CopyPath() *Path {
	p := C.cairo_copy_path(cairo_t(this))
	return (*Path)(p)
}

func (this *Context) CopyPathFlat() *Path {
	p := C.cairo_copy_path_flat(cairo_t(this))
	return (*Path)(p)
}

func (this *Context) AppendPath(p *Path) {
	C.cairo_append_path(cairo_t(this), path_t(p))
}

func (this *Context) HasCurrentPoint() bool {
	return C.cairo_has_current_point(cairo_t(this)) != 0
}

func (this *Context) CurrentPoint() (x, y float64) {
	C.cairo_get_current_point(cairo_t(this), (*C.double)(&x), (*C.double)(&y))
	return
}

func (this *Context) NewPath() {
	C.cairo_new_path(cairo_t(this))
}

func (this *Context) NewSubPath() {
	C.cairo_new_sub_path(cairo_t(this))
}

func (this *Context) ClosePath() {
	C.cairo_close_path(cairo_t(this))
}

func (this *Context) Arc(xc, yc, radius, angle1, angle2 float64) {
	C.cairo_arc(cairo_t(this), C.double(xc), C.double(yc),
		C.double(radius), C.double(angle1), C.double(angle2))
}

func (this *Context) ArcNegative(xc, yc, radius, angle1, angle2 float64) {
	C.cairo_arc_negative(cairo_t(this), C.double(xc), C.double(yc),
		C.double(radius), C.double(angle1), C.double(angle2))
}

func (this *Context) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	C.cairo_curve_to(cairo_t(this), C.double(x1), C.double(y1),
		C.double(x2), C.double(y2), C.double(x3), C.double(y3))
}

func (this *Context) LineTo(x, y float64) {
	C.cairo_line_to(cairo_t(this), C.double(x), C.double(y))
}

func (this *Context) MoveTo(x, y float64) {
	C.cairo_move_to(cairo_t(this), C.double(x), C.double(y))
}

func (this *Context) Rectangle(x, y, width, height float64) {
	C.cairo_rectangle(cairo_t(this), C.double(x), C.double(y),
		C.double(width), C.double(height))
}

// cairo_glyph_path ()
func (this *Context) TextPath(s string) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	C.cairo_text_path(cairo_t(this), cs)
}

func (this *Context) RelCurveTo(x1, y1, x2, y2, x3, y3 float64) {
	C.cairo_rel_curve_to(cairo_t(this), C.double(x1), C.double(y1),
		C.double(x2), C.double(y2), C.double(x3), C.double(y3))
}

func (this *Context) RelLineTo(x, y float64) {
	C.cairo_rel_line_to(cairo_t(this), C.double(x), C.double(y))
}

func (this *Context) RelMoveTo(x, y float64) {
	C.cairo_rel_move_to(cairo_t(this), C.double(x), C.double(y))
}

func (this *Context) PathExtens() (x1, y1, x2, y2 float64) {
	C.cairo_path_extents(cairo_t(this), (*C.double)(&x1), (*C.double)(&y1), (*C.double)(&x2), (*C.double)(&y2))
	return
}

func (this *Context) Translate(tx, ty float64) {
	C.cairo_translate(cairo_t(this), C.double(tx), C.double(ty))
}

func (this *Context) Scale(sx, sy float64) {
	C.cairo_scale(cairo_t(this), C.double(sx), C.double(sy))
}

func (this *Context) Rotate(radians float64) {
	C.cairo_rotate(cairo_t(this), C.double(radians))
}

func (this *Context) SetMatrix(mat *geom.Mat3x2) {
	C.cairo_set_matrix(cairo_t(this), matrix_t(mat))
}

func (this *Context) GetMatrix(mat *geom.Mat3x2) {
	C.cairo_get_matrix(cairo_t(this), matrix_t(mat))
}

func (this *Context) Transform(mat *geom.Mat3x2) {
	C.cairo_transform(cairo_t(this), matrix_t(mat))
}

func (this *Context) SetIdentityMatrix() {
	C.cairo_identity_matrix(cairo_t(this))
}

func (this *Context) ResetMatrix() {
	C.cairo_identity_matrix(cairo_t(this))
}

func (this *Context) UserToDevice(x, y float64) (x1, y1 float64) {
	x1 = x
	y1 = y
	C.cairo_user_to_device(cairo_t(this), (*C.double)(&x1), (*C.double)(&y1))
	return
}

func (this *Context) UserToDeviceDistance(dx, dy float64) (dx1, dy1 float64) {
	dx1 = dx
	dy1 = dy
	C.cairo_user_to_device_distance(cairo_t(this), (*C.double)(&dx1), (*C.double)(&dy1))
	return
}

func (this *Context) DeviceToUser(x, y float64) (x1, y1 float64) {
	x1 = x
	y1 = y
	C.cairo_device_to_user(cairo_t(this), (*C.double)(&x1), (*C.double)(&y1))
	return
}

func (this *Context) DeviceToUserDistance(dx, dy float64) (dx1, dy1 float64) {
	dx1 = dx
	dy1 = dy
	C.cairo_device_to_user_distance(cairo_t(this), (*C.double)(&dx1), (*C.double)(&dy1))
	return
}

func (this *Context) SelectFontFace(family string, slant FontSlant, weight FontWeight) {
	cfamily := C.CString(family)
	defer C.free(unsafe.Pointer(cfamily))
	C.cairo_select_font_face(cairo_t(this), cfamily,
		C.cairo_font_slant_t(slant),
		C.cairo_font_weight_t(weight))
}

func (this *Context) SetFontSize(size float64) {
	C.cairo_set_font_size(cairo_t(this), C.double(size))
}

func (this *Context) SetFontMatrix(mat *geom.Mat3x2) {
	C.cairo_set_font_matrix(cairo_t(this), matrix_t(mat))
}

func (this *Context) GetFontMatrix(mat *geom.Mat3x2) {
	C.cairo_get_font_matrix(cairo_t(this), matrix_t(mat))
}

func (this *Context) SetFontOptions(options *FontOptions) {
	C.cairo_set_font_options(cairo_t(this), font_options_t(options))
}

func (this *Context) FontOptions() *FontOptions {
	options := C.cairo_font_options_create()
	C.cairo_get_font_options(cairo_t(this), options)
	return (*FontOptions)(options)
}

func (this *Context) SetFontFace(face *FontFace) {
	C.cairo_set_font_face(cairo_t(this), font_face_t(face))
}

func (this *Context) FontFace() *FontFace {
	f := C.cairo_get_font_face(cairo_t(this))
	C.cairo_font_face_reference(f)
	return (*FontFace)(f)
}

func (this *Context) SetScaledFont(scaled_font *ScaledFont) {
	C.cairo_set_scaled_font(cairo_t(this), scaled_font_t(scaled_font))
}

func (this *Context) ScaledFont() *ScaledFont {
	//	this.applyFont()
	f := C.cairo_get_scaled_font(cairo_t(this))
	C.cairo_scaled_font_reference(f)
	return (*ScaledFont)(f)
}

func (this *Context) ShowText(text string) {
	//	this.applyFont()
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	C.cairo_show_text(cairo_t(this), cs)
}

func (this *Context) ShowGlyphs_hack(p unsafe.Pointer, n int) {
	if p == nil || n == 0 {
		return
	}
	C.cairo_show_glyphs(cairo_t(this), (*C.cairo_glyph_t)(p), C.int(n))
}

func (this *Context) ShowGlyphs(glyphs []Glyph) {
	n := len(glyphs)
	if n == 0 {
		return
	}
	//	this.applyFont()
	this.ShowGlyphs_hack(unsafe.Pointer(&glyphs[0]), n)
}

//func (this *Context) ShowTextGlyphs(text string, glyphs *Glyphs, clusters *TextClusters) {
//	cs := C.CString(text)
//	defer C.free(unsafe.Pointer(cs))
//	csLen := C.int(C.strlen(cs))
//	g, gn := glyphs.native()
//	c, cn, cf := clusters.native()
//	C.cairo_show_text_glyphs(cairo_t(this), cs, csLen, g, gn, c, cn, cf)
//}

func (this *Context) FontExtents() *FontExtents {
	ext := new(FontExtents)
	C.cairo_font_extents(cairo_t(this), (*C.cairo_font_extents_t)(unsafe.Pointer(ext)))
	return ext
}

func (this *Context) TextExtents(text string) *TextExtents {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	ext := new(TextExtents)
	C.cairo_text_extents(cairo_t(this), cs, (*C.cairo_text_extents_t)(unsafe.Pointer(ext)))
	return ext
}

func (this *Context) GlyphExtents(glyphs []Glyph) *TextExtents {
	n := len(glyphs)
	var p *C.cairo_glyph_t
	if n == 0 {
		p = nil
	} else {
		p = (*C.cairo_glyph_t)(unsafe.Pointer(&glyphs[0]))
	}
	ext := new(TextExtents)
	C.cairo_glyph_extents(cairo_t(this), p, C.int(n), (*C.cairo_text_extents_t)(unsafe.Pointer(ext)))
	return ext
}

//func (this *Context) setPen(pen paint.Pen) {
//	this.SetLineWidth(pen.Width())
//	this.SetSourceRGBA(pen.Color().NRGBAf())
//}

/*
// ----------------------------------------------------------------
type Surface interface {
	native() *C.cairo_surface_t
	NewContext() Context
	NewSimilar(c Content, width, height int) Surface
	Finish()
	Flush()
	Device() *Device
	FontOptions() *FontOptions
	Content() Content
	MarkDirty()
	MarkDirtyRectange(x, y, width, height int)
	SetDeviceOffset(x, y float64)
	DeviceOffset() (x, y float64)
	SetFallbackResolution(xPpi, yPpi float64)
	FallbackResolution() (xPpi, yPpi float64)
	Type() SurfaceType
	CopyPage()
	ShowPage()
}
*/

// ----------------------------------------------------------------
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
	SURFACE_TYPE_BEOS
	SURFACE_TYPE_DIRECTFB
	SURFACE_TYPE_SVG
	SURFACE_TYPE_OS2
	SURFACE_TYPE_WIN32_PRINTING
	SURFACE_TYPE_QUARTZ_IMAGE
	SURFACE_TYPE_SCRIPT
	SURFACE_TYPE_QT
	SURFACE_TYPE_RECORDING
	SURFACE_TYPE_VG
	SURFACE_TYPE_GL
	SURFACE_TYPE_DRM
	SURFACE_TYPE_TEE
	SURFACE_TYPE_XML
	SURFACE_TYPE_SKIA
	SURFACE_TYPE_SUBSURFACE
	SURFACE_TYPE_COGL // 1.12
)

func (v SurfaceType) String() string {
	switch v {
	case SURFACE_TYPE_IMAGE:
		return "SURFACE_TYPE_IMAGE"
	case SURFACE_TYPE_PDF:
		return "SURFACE_TYPE_PDF"
	case SURFACE_TYPE_PS:
		return "SURFACE_TYPE_PS"
	case SURFACE_TYPE_XLIB:
		return "SURFACE_TYPE_XLIB"
	case SURFACE_TYPE_XCB:
		return "SURFACE_TYPE_XCB"
	case SURFACE_TYPE_GLITZ:
		return "SURFACE_TYPE_GLITZ"
	case SURFACE_TYPE_QUARTZ:
		return "SURFACE_TYPE_QUARTZ"
	case SURFACE_TYPE_WIN32:
		return "SURFACE_TYPE_WIN32"
	case SURFACE_TYPE_BEOS:
		return "SURFACE_TYPE_BEOS"
	case SURFACE_TYPE_DIRECTFB:
		return "SURFACE_TYPE_DIRECTFB"
	case SURFACE_TYPE_SVG:
		return "SURFACE_TYPE_SVG"
	case SURFACE_TYPE_OS2:
		return "SURFACE_TYPE_OS2"
	case SURFACE_TYPE_WIN32_PRINTING:
		return "SURFACE_TYPE_WIN32_PRINTING"
	case SURFACE_TYPE_QUARTZ_IMAGE:
		return "SURFACE_TYPE_QUARTZ_IMAGE"
	case SURFACE_TYPE_SCRIPT:
		return "SURFACE_TYPE_SCRIPT"
	case SURFACE_TYPE_QT:
		return "SURFACE_TYPE_QT"
	case SURFACE_TYPE_RECORDING:
		return "SURFACE_TYPE_RECORDING"
	case SURFACE_TYPE_VG:
		return "SURFACE_TYPE_VG"
	case SURFACE_TYPE_GL:
		return "SURFACE_TYPE_GL"
	case SURFACE_TYPE_DRM:
		return "SURFACE_TYPE_DRM"
	case SURFACE_TYPE_TEE:
		return "SURFACE_TYPE_TEE"
	case SURFACE_TYPE_XML:
		return "SURFACE_TYPE_XML"
	case SURFACE_TYPE_SKIA:
		return "SURFACE_TYPE_SKIA"
	case SURFACE_TYPE_SUBSURFACE:
		return "SURFACE_TYPE_SUBSURFACE"
	case SURFACE_TYPE_COGL:
		return "SURFACE_TYPE_COGL" // 1.12
	default:
		return "SURFACE_TYPE_<Unkown>"
	}

}

// ----------------------------------------------------------------
type Surface C.cairo_surface_t

func surface_t(p *Surface) *C.cairo_surface_t {
	return (*C.cairo_surface_t)(p)
}

func (this *Surface) Destroy() {
	C.cairo_surface_destroy(surface_t(this))
}

//func newSurface(s *C.cairo_surface_t) *Surface {
//	e := Status(C.cairo_surface_status(s))
//	if e != STATUS_SUCCESS {
//		core.Warn(e)
//		return nil
//	}
//	//switch SurfaceType(C.cairo_surface_get_type(s)) {
//	//case SURFACE_TYPE_IMAGE:
//	//	return newImageSurface(s)
//	//case SURFACE_TYPE_WIN32:
//	//	return newWin32Surface(s)
//	//default:
//	ss := new(Surface)
//	ss.s = s
//	runtime.SetFinalizer(ss, destroySurface)
//	return ss
//	//}
//}

func (this *Surface) NewContext() *Context {
	c := C.cairo_create(surface_t(this))
	return (*Context)(c)
}

func (this *Surface) NewSimilar(c Content, width, height int) *Surface {
	s := C.cairo_surface_create_similar(surface_t(this),
		C.cairo_content_t(c), C.int(width), C.int(height))
	return (*Surface)(s)
}

func (this *Surface) NewSurfaceForRectangle(x, y, width, height float64) *Surface {
	s := C.cairo_surface_create_for_rectangle(surface_t(this),
		C.double(x), C.double(y), C.double(width), C.double(height))
	return (*Surface)(s)
}

/*
1.12
func (this *Surface) CreateSimilarImage(f Format, width, height int) {
	s := C.cairo_surface_create_similar_image(surface_t(this),
		C.cairo_format_t(f), C.int(width), C.int(height))
	return newSurface(s)

}
*/

func (this *Surface) Finish() {
	C.cairo_surface_finish(surface_t(this))
}

func (this *Surface) Flush() {
	C.cairo_surface_flush(surface_t(this))
}

func (this *Surface) Device() *Device {
	d := C.cairo_surface_get_device(surface_t(this))
	if d == nil {
		return nil
	}
	return newDevice(d, false)
}

func (this *Surface) FontOptions() *FontOptions {
	options := C.cairo_font_options_create()
	C.cairo_surface_get_font_options(surface_t(this), options)
	return (*FontOptions)(options)
}

func (this *Surface) Content() Content {
	c := C.cairo_surface_get_content(surface_t(this))
	return Content(c)
}

func (this *Surface) MarkDirty() {
	C.cairo_surface_mark_dirty(surface_t(this))
}

func (this *Surface) MarkDirtyRectange(x, y, width, height int) {
	C.cairo_surface_mark_dirty_rectangle(surface_t(this), C.int(x), C.int(y), C.int(width), C.int(height))
}

func (this *Surface) SetDeviceOffset(x, y float64) {
	C.cairo_surface_set_device_offset(surface_t(this), C.double(x), C.double(y))
}

func (this *Surface) DeviceOffset() (x, y float64) {
	C.cairo_surface_get_device_offset(surface_t(this), (*C.double)(&x), (*C.double)(&y))
	return
}

func (this *Surface) SetFallbackResolution(xPpi, yPpi float64) {
	C.cairo_surface_set_fallback_resolution(surface_t(this), C.double(xPpi), C.double(yPpi))
}

func (this *Surface) FallbackResolution() (xPpi, yPpi float64) {
	C.cairo_surface_get_fallback_resolution(surface_t(this), (*C.double)(&xPpi), (*C.double)(&yPpi))
	return
}

func (this *Surface) Type() SurfaceType {
	return SurfaceType(C.cairo_surface_get_type(surface_t(this)))
}

func (this *Surface) CopyPage() {
	C.cairo_surface_copy_page(surface_t(this))
}

func (this *Surface) ShowPage() {
	C.cairo_surface_show_page(surface_t(this))
}

// cairo_surface_has_show_text_glyphs ()
// cairo_surface_set_mime_data ()
// cairo_surface_get_mime_data ()
// cairo_surface_supports_mime_type ()

/*
func (this *Surface) MapToImage(rect *RectangleInt) *MappedImage {
	s := C.cairo_surface_map_to_image(surface_t(this), (*C.cairo_rectangle_int_t)(rect))
	m := new(MappedImage)
	m.Surface.s = s
	m.src = this
	runtime.SetFinalizer(m, Unmap)
}

// ----------------------------------------------------------------
type MappedImage struct {
	Surface
	src *Surface
}

func (this *MappedImage) Unmap() {
	if this.src == nil {
		return
	}
	C.cairo_surface_unmap_image(this.src, this.Surface.s)
	this.src = nil
	this.Surface.s = nil
}
*/
/*
// ----------------------------------------------------------------
type Image interface {
	Surface
	Image() (image.Image, error)
	SetImage(img image.Image) error
	SetData(src []uint8) error
	Format() Format
	Width() int
	Height() int
	Stride() int
	WidthF() float64
	HeightF() float64
}
*/
// ----------------------------------------------------------------
//type Surface struct {
//	Surface
//}

//func newImageSurface(s *C.cairo_surface_t) *Surface {
//	e := Status(C.cairo_surface_status(s))
//	if e != STATUS_SUCCESS {
//		core.Warn(e)
//	}
//	ss := new(Surface)
//	ss.Surface.s = s
//	runtime.SetFinalizer(ss, destroySurface)
//	return ss
//}

//func NewWin32Surface(dc uintptr) *Win32Surface {
//	s := C.cairo_win32_surface_create((*_Ctype_struct_HDC__)(unsafe.Pointer(dc)))
//	return newWin32Surface(s)
//}

//func NewImageSurface(format Format, width, height int) *Surface {
//	s := C.cairo_image_surface_create(C.cairo_format_t(format), C.int(width), C.int(height))
//	return newImageSurface(s)
//}

func NewImageSurface(format Format, width, height int) *Surface {
	s := C.cairo_image_surface_create(C.cairo_format_t(format), C.int(width), C.int(height))
	return (*Surface)(s)

}

func NewImageSurfaceFromPNGStream(r io.Reader) (*Surface, error) {
	// Write to temp file and use C API to avoid Go-to-C pointer issues
	tmpf, err := os.CreateTemp("", "cairo-png-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmpf.Name()
	defer os.Remove(tmpName)
	_, err = io.Copy(tmpf, r)
	tmpf.Close()
	if err != nil {
		return nil, err
	}
	return NewImageSurfaceFromPNG(tmpName)
}

func NewImageSurfaceFromPNG(filename string) (*Surface, error) {
	cs := C.CString(filename)
	defer C.free(unsafe.Pointer(cs))
	s := C.cairo_image_surface_create_from_png(cs)
	e := Status(C.cairo_surface_status(s))
	if e != STATUS_SUCCESS {
		C.cairo_surface_destroy(s)
		return nil, errors.New(e.String())
	}
	return (*Surface)(s), nil
}

func (this *Surface) WritePNGToStream(w io.Writer) error {
	var pinner runtime.Pinner
	pinner.Pin(&w)
	defer pinner.Unpin()
	e0 := C.cairo_surface_write_to_png_stream(
		surface_t(this), C.cairo_write_func_t(C.writeFuncBound), unsafe.Pointer(&w))
	e := Status(e0)
	if e != STATUS_SUCCESS {
		return errors.New(e.String())
	}
	return nil
}

func (this *Surface) WritePNG(filename string) error {
	cs := C.CString(filename)
	defer C.free(unsafe.Pointer(cs))
	e := Status(C.cairo_surface_write_to_png(surface_t(this), cs))
	if e != STATUS_SUCCESS {
		return errors.New(e.String())
	}
	return nil
}
func rgbaToBgra(src []uint8) []uint8 {
	sz := len(src) - 3
	dst := make([]uint8, 0)
	for i := 0; i < sz; i += 4 {
		x, y, z, w := src[i], src[i+1], src[i+2], src[i+3]
		dst = append(dst, z, y, x, w)
	}
	return dst
}

func rgbnToBgra(src []uint8) []uint8 {
	sz := len(src) - 3
	dst := make([]uint8, 0)
	for i := 0; i < sz; i += 4 {
		x, y, z, _ := src[i], src[i+1], src[i+2], src[i+3]
		dst = append(dst, z, y, x, 255)
	}
	return dst
}

func (this *Surface) Image() (image.Image, error) {
	format := this.Format()
	stride := this.Stride()
	width := this.Width()
	height := this.Height()

	switch format {
	case FORMAT_ARGB32:
		fallthrough
	case FORMAT_RGB24:
		this.Flush()
		data := C.cairo_image_surface_get_data(surface_t(this))
		if data == nil {
			return nil, errors.New("failed to get data.")
		}
		rect := image.Rect(0, 0, width, height)
		pix := C.GoBytes(unsafe.Pointer(data), C.int(stride*height))
		var ret image.Image
		if format == FORMAT_RGB24 {
			pix = rgbnToBgra(pix)
		} else {
			pix = rgbaToBgra(pix)
		}
		ret = &image.RGBA{Pix: pix, Stride: stride, Rect: rect}
		return ret, nil
	case FORMAT_A8:
		this.Flush()
		data := C.cairo_image_surface_get_data(surface_t(this))
		if data == nil {
			return nil, errors.New("failed to get data.")
		}
		rect := image.Rect(0, 0, width, height)
		pix := C.GoBytes(unsafe.Pointer(data), C.int(stride*height))
		ret := &image.Alpha{Pix: pix, Stride: stride, Rect: rect}
		return ret, nil
	case FORMAT_A1:
		return nil, errors.New("unsupported FORMAT_A1")
	case FORMAT_RGB16_565:
		return nil, errors.New("unsupported FORMAT_RGB16_565")
	case FORMAT_RGB30:
		return nil, errors.New("unsupported FORMAT_RGB30")
	default:
		return nil, errors.New("unsupported unkown format")
	}

}

func (this *Surface) SetData(src []uint8) error {

	if src == nil {
		return nil
	}
	sz := len(src)
	data := C.cairo_image_surface_get_data(surface_t(this))
	if data == nil {
		return errors.New("failed to get data.")
	}
	stride := this.Stride()
	height := this.Height()
	if sz > stride*height {
		sz = stride * height
	}

	C.memcpy(unsafe.Pointer(data), unsafe.Pointer(&src[0]), C.size_t(sz))
	return nil
}

func (this *Surface) SetImage(img image.Image) error {
	format := this.Format()
	stride := this.Stride()
	width := this.Width()
	height := this.Height()

	switch format {
	case FORMAT_ARGB32:
		fallthrough
	case FORMAT_RGB24:
		p, ok := img.(*image.RGBA)
		if ok && p.Rect.Dx() == width && p.Rect.Dy() == height && p.Stride == stride {
			pix := p.Pix
			if format == FORMAT_ARGB32 {
				pix = rgbaToBgra(p.Pix)
			} else {
				pix = rgbnToBgra(p.Pix)
			}
			this.SetData(pix)
			return nil
		}
		dstImg, err := this.Image()
		if err != nil {
			return err
		}
		dst := dstImg.(*image.RGBA)
		draw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, draw.Src)
		pix := dst.Pix
		if format == FORMAT_ARGB32 {
			pix = rgbaToBgra(p.Pix)
		} else {
			pix = rgbnToBgra(p.Pix)
		}
		return this.SetData(pix)
	case FORMAT_A8:
		p, ok := img.(*image.Alpha)
		if ok && p.Rect.Dx() == width && p.Rect.Dy() == height && p.Stride == stride {
			pix := p.Pix
			this.SetData(pix)
			return nil
		}
		dstImg, err := this.Image()
		if err != nil {
			return err
		}
		dst := dstImg.(*image.Alpha)
		draw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, draw.Src)
		pix := dst.Pix
		return this.SetData(pix)

	case FORMAT_A1:
		return errors.New("unsupported FORMAT_A1")
	case FORMAT_RGB16_565:
		return errors.New("unsupported FORMAT_RGB16_565")
	case FORMAT_RGB30:
		return errors.New("unsupported FORMAT_RGB30")
	default:
		return errors.New("unsupported unkown format")
	}

}

// DataPtr returns the raw pixel data pointer for the image surface.
func (this *Surface) DataPtr() unsafe.Pointer {
	this.Flush()
	return unsafe.Pointer(C.cairo_image_surface_get_data(surface_t(this)))
}

func (this *Surface) Format() Format {
	return Format(C.cairo_image_surface_get_format(surface_t(this)))
}

func (this *Surface) Width() int {
	return int(C.cairo_image_surface_get_width(surface_t(this)))
}

func (this *Surface) Height() int {
	return int(C.cairo_image_surface_get_height(surface_t(this)))
}

func (this *Surface) WidthF() float64 {
	return float64(C.cairo_image_surface_get_width(surface_t(this)))
}

func (this *Surface) HeightF() float64 {
	return float64(C.cairo_image_surface_get_height(surface_t(this)))
}

func (this *Surface) Stride() int {
	return int(C.cairo_image_surface_get_stride(surface_t(this)))
}

func (this *Surface) DupGrayed() *Surface {
	s := NewImageSurface(FORMAT_ARGB32, this.Width(), this.Height())
	cc := s.NewContext()
	cc.SetSourceSurface(this, 0, 0)
	cc.SetOperator(OPERATOR_SOURCE)
	cc.Paint()

	img, err := s.Image()
	if err != nil {
		return nil
	}
	p, ok := img.(*image.RGBA)
	if !ok {
		return nil
	}
	src := p.Pix
	sz := len(src) - 3
	dst := make([]uint8, 0)
	for i := 0; i < sz; i += 4 {
		x, y, z, a := src[i], src[i+1], src[i+2], src[i+3]
		l := uint8((int(x) + int(y) + int(z)) / 3)
		dst = append(dst, l, l, l, a)
	}
	s.SetData(dst)
	return s
}

func (this *Surface) Dup() *Surface {
	s := NewImageSurface(this.Format(), this.Width(), this.Height())
	cc := s.NewContext()
	cc.SetSourceSurface(this, 0, 0)
	cc.SetOperator(OPERATOR_SOURCE)
	cc.Paint()
	return s
}

// ----------------------------------------------------------------
type Content int

const (
	CONTENT_COLOR       Content = 0x1000
	CONTENT_ALPHA       Content = 0x2000
	CONTENT_COLOR_ALPHA Content = 0x3000
)

// ----------------------------------------------------------------
type Pattern C.cairo_pattern_t

func pattern_t(p *Pattern) *C.cairo_pattern_t {
	return (*C.cairo_pattern_t)(p)
}

func (this *Pattern) Destroy() {
	C.cairo_pattern_destroy(pattern_t(this))
}

//func (this *Pattern) Reference() *Pattern {
//	C.cairo_pattern_reference(pattern_t(this))
//	return this
//}

//func newPattern(p *C.cairo_pattern_t, addRef bool) *Pattern {
//	pp := &Pattern{p}
//	if addRef {
//		C.cairo_pattern_reference(p)
//	}
//	runtime.SetFinalizer(pp, destroyPattern)
//	return pp
//}

func NewRGBPattern(r, g, b float64) *Pattern {
	p := C.cairo_pattern_create_rgb(C.double(r), C.double(g), C.double(b))
	return (*Pattern)(p)
}

func NewRGBAPattern(r, g, b, a float64) *Pattern {
	p := C.cairo_pattern_create_rgb(C.double(r), C.double(g), C.double(b))
	return (*Pattern)(p)
}

func NewPatternForSurface(s *Surface) *Pattern {
	p := C.cairo_pattern_create_for_surface(surface_t(s))
	return (*Pattern)(p)
}

func NewLinearPattern(x0, y0, x1, y1 float64) *Pattern {
	p := C.cairo_pattern_create_linear(C.double(x0), C.double(y0), C.double(x1), C.double(y1))
	return (*Pattern)(p)
}

func NewRadialPattern(cx0, cy0, radius0, cx1, cy1, radius1 float64) *Pattern {
	p := C.cairo_pattern_create_radial(C.double(cx0), C.double(cy0), C.double(radius0),
		C.double(cx1), C.double(cy1), C.double(radius1))
	return (*Pattern)(p)
}

// 1.12
//func NewMeshPattern() *Pattern {
//	p := C.cairo_pattern_create_mesh()
//	return newPattern(p, false)
//}

func (this *Pattern) AddColorStopRGB(offset, r, g, b float64) {
	C.cairo_pattern_add_color_stop_rgb(pattern_t(this),
		C.double(offset), C.double(r), C.double(g), C.double(b))
}

func (this *Pattern) AddColorStopRGBA(offset, r, g, b, a float64) {
	C.cairo_pattern_add_color_stop_rgba(pattern_t(this),
		C.double(offset), C.double(r), C.double(g), C.double(b), C.double(a))

}

func (this *Pattern) ColorStopCount() (count int32, err error) {
	st := Status(C.cairo_pattern_get_color_stop_count(pattern_t(this), (*C.int)(&count)))
	if st != STATUS_SUCCESS {
		err = errors.New(st.String())
		return
	}
	err = nil
	return
}

func (this *Pattern) ColorStopRGBA(index int) (offset, r, g, b, a float64, err error) {
	st0 := C.cairo_pattern_get_color_stop_rgba(pattern_t(this),
		C.int(index), (*C.double)(&offset),
		(*C.double)(&r), (*C.double)(&g), (*C.double)(&b), (*C.double)(&a))
	st := Status(st0)
	if st != STATUS_SUCCESS {
		err = errors.New(st.String())
		return
	}
	err = nil
	return
}

func (this *Pattern) RGBA() (r, g, b, a float64, err error) {
	st0 := C.cairo_pattern_get_rgba(pattern_t(this),
		(*C.double)(&r), (*C.double)(&g), (*C.double)(&b), (*C.double)(&a))
	st := Status(st0)
	if st != STATUS_SUCCESS {
		err = errors.New(st.String())
		return
	}
	err = nil
	return
}

func (this *Pattern) Surfface() (s *Surface, err error) {
	st0 := C.cairo_pattern_get_surface(pattern_t(this), (**C.cairo_surface_t)(unsafe.Pointer(&s)))
	st := Status(st0)
	if st != STATUS_SUCCESS {
		err = errors.New(st.String())
		return
	}
	err = nil
	return
}

func (this *Pattern) LinearPoints() (x0, y0, x1, y1 float64, err error) {
	st0 := C.cairo_pattern_get_linear_points(pattern_t(this),
		(*C.double)(&x0), (*C.double)(&y0), (*C.double)(&x1), (*C.double)(&y1))
	st := Status(st0)
	if st != STATUS_SUCCESS {
		err = errors.New(st.String())
		return
	}
	err = nil
	return
}

func (this *Pattern) RadialCircles() (x0, y0, r0, x1, y1, r1 float64, err error) {
	st0 := C.cairo_pattern_get_radial_circles(pattern_t(this),
		(*C.double)(&x0), (*C.double)(&y0), (*C.double)(&r0),
		(*C.double)(&x1), (*C.double)(&y1), (*C.double)(&r1))
	st := Status(st0)
	if st != STATUS_SUCCESS {
		err = errors.New(st.String())
		return
	}
	err = nil
	return
}

//func (this *Pattern)
// cairo_mesh_pattern_begin_patch () 1.12
// cairo_mesh_pattern_end_patch ()
// cairo_mesh_pattern_move_to ()
// cairo_mesh_pattern_line_to ()
// cairo_mesh_pattern_curve_to ()
// cairo_mesh_pattern_set_control_point ()
// cairo_mesh_pattern_set_corner_color_rgb ()
// cairo_mesh_pattern_set_corner_color_rgba ()
// cairo_mesh_pattern_get_patch_count ()
// cairo_mesh_pattern_get_path ()
// cairo_mesh_pattern_get_control_point ()
// cairo_mesh_pattern_get_corner_color_rgba ()
// cairo_pattern_reference ()

func (this *Pattern) Status() Status {
	return Status(C.cairo_pattern_status(pattern_t(this)))
}

func (this *Pattern) SetExtend(ext Extend) {
	C.cairo_pattern_set_extend(pattern_t(this), C.cairo_extend_t(ext))
}

func (this *Pattern) Extend() Extend {
	return Extend(C.cairo_pattern_get_extend(pattern_t(this)))
}

func (this *Pattern) SetFilter(f Filter) {
	C.cairo_pattern_set_filter(pattern_t(this), C.cairo_filter_t(f))
}

func (this *Pattern) Filter() Filter {
	return Filter(C.cairo_pattern_get_filter(pattern_t(this)))
}

func (this *Pattern) SetMatrix(mat *geom.Mat3x2) {
	C.cairo_pattern_set_matrix(pattern_t(this), matrix_t(mat))
}

func (this *Pattern) GetMatrix(mat *geom.Mat3x2) {
	C.cairo_pattern_get_matrix(pattern_t(this), matrix_t(mat))
	return
}

// ----------------------------------------------------------------
type Antialias int

const (
	ANTIALIAS_DEFAULT Antialias = iota

	/* method */
	ANTIALIAS_NONE
	ANTIALIAS_GRAY
	ANTIALIAS_SUBPIXEL

	/* hints */
	ANTIALIAS_FAST
	ANTIALIAS_GOOD
	ANTIALIAS_BEST
)

// ----------------------------------------------------------------
type Dash struct {
	Dashes []float64
	Offset float64
}

func (this *Dash) Dup() (d Dash) {
	copy(d.Dashes, this.Dashes)
	d.Offset = this.Offset
	return
}

// ----------------------------------------------------------------
type FillRule int

const (
	FILL_RULE_WINDING FillRule = iota
	FILL_RULE_EVEN_ODD
)

// ----------------------------------------------------------------
type LineCap int

const (
	LINE_CAP_BUTT LineCap = iota
	LINE_CAP_ROUND
	LINE_CAP_SQUARE
)

// ----------------------------------------------------------------
type LineJoin int

const (
	LINE_JOIN_MITER LineJoin = iota
	LINE_JOIN_ROUND
	LINE_JOIN_BEVEL
)

// ----------------------------------------------------------------
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

// ----------------------------------------------------------------
type Rectangle struct {
	X, Y, Width, Height float64
}

// ----------------------------------------------------------------
type RectangleInt struct {
	X, Y, Width, Height int32
}

// ----------------------------------------------------------------
type RectangleList struct {
	l *C.cairo_rectangle_list_t
}

// ----------------------------------------------------------------
type Format int

const (
	FORMAT_INVALID   Format = -1
	FORMAT_ARGB32           = 0
	FORMAT_RGB24            = 1
	FORMAT_A8               = 2
	FORMAT_A1               = 3
	FORMAT_RGB16_565        = 4
	FORMAT_RGB30            = 5
)

// ----------------------------------------------------------------
type Device struct {
	d *C.cairo_device_t
}

func destroyDevice(d *Device) {
	if d.d != nil {
		C.cairo_device_destroy(d.d)
		d.d = nil
	}
}

func newDevice(d *C.cairo_device_t, addRef bool) *Device {
	dev := &Device{d}
	if addRef {
		C.cairo_device_reference(d)
	}
	runtime.SetFinalizer(dev, destroyDevice)
	return dev
}

// ----------------------------------------------------------------
type FontOptions C.cairo_font_options_t

func font_options_t(fo *FontOptions) *C.cairo_font_options_t {
	return (*C.cairo_font_options_t)(fo)
}

func (this *FontOptions) Destroy() {
	C.cairo_font_options_destroy(font_options_t(this))
}

//func newFontOptions(o *C.cairo_font_options_t) *FontOptions {
//	fo := &FontOptions{o}
//	runtime.SetFinalizer(fo, destroyFontOptions)
//	return fo
//}

//var globalFontOptions = NewFontOptions()

func NewFontOptions() *FontOptions {
	o := C.cairo_font_options_create()
	return (*FontOptions)(o)
}

func (this *FontOptions) Dup() *FontOptions {
	o := C.cairo_font_options_copy(font_options_t(this))
	return (*FontOptions)(o)
}

func (this *FontOptions) Status() Status {
	s := C.cairo_font_options_status(font_options_t(this))
	return Status(s)
}

func (this *FontOptions) Merge(other *FontOptions) {
	C.cairo_font_options_merge(font_options_t(this), font_options_t(other))
}

func (this *FontOptions) Hash() uint32 {
	return uint32(C.cairo_font_options_hash(font_options_t(this)))
}

func (this *FontOptions) Equal(other *FontOptions) bool {
	b := C.cairo_font_options_equal(font_options_t(this), font_options_t(other))
	return b != 0
}

func (this *FontOptions) SetAntialias(antialias Antialias) {
	C.cairo_font_options_set_antialias(font_options_t(this), C.cairo_antialias_t(antialias))
}

func (this *FontOptions) Antialias() (antialias Antialias) {
	antialias = Antialias(C.cairo_font_options_get_antialias(font_options_t(this)))
	return
}

func (this *FontOptions) SetSubpixelOrder(order SubpixelOrder) {
	C.cairo_font_options_set_subpixel_order(font_options_t(this), C.cairo_subpixel_order_t(order))
}

func (this *FontOptions) SubpixelOrder() (order SubpixelOrder) {
	order = SubpixelOrder(C.cairo_font_options_get_subpixel_order(font_options_t(this)))
	return
}

func (this *FontOptions) SetHintStyle(v HintStyle) {
	C.cairo_font_options_set_hint_style(font_options_t(this), C.cairo_hint_style_t(v))
}

func (this *FontOptions) HintStyle() (v HintStyle) {
	v = HintStyle(C.cairo_font_options_get_hint_style(font_options_t(this)))
	return
}

func (this *FontOptions) SetHintMetrics(v HintMetrics) {
	C.cairo_font_options_set_hint_metrics(font_options_t(this), C.cairo_hint_metrics_t(v))
}

func (this *FontOptions) HintMetrics() (v HintMetrics) {
	v = HintMetrics(C.cairo_font_options_get_hint_metrics(font_options_t(this)))
	return
}

// ----------------------------------------------------------------
type Path C.cairo_path_t

func path_t(p *Path) *C.cairo_path_t {
	return (*C.cairo_path_t)(p)
}

func (this *Path) Destroy() {
	C.cairo_path_destroy(path_t(this))
}

//func newPath(p *C.cairo_path_t) *Path {
//	pp := &Path{p}
//	runtime.SetFinalizer(p, destroyPath)
//	return pp
//}

type PathDataType int

const (
	PATH_MOVE_TO PathDataType = iota
	PATH_LINE_TO
	PATH_CURVE_TO
	PATH_CLOSE_PATH
)

// ----------------------------------------------------------------
type Extend int

const (
	EXTEND_NONE Extend = iota
	EXTEND_REPEAT
	EXTEND_REFLECT
	EXTEND_PAD
)

// ----------------------------------------------------------------
type Filter int

const (
	FILTER_FAST Filter = iota
	FILTER_GOOD
	FILTER_BEST
	FILTER_NEAREST
	FILTER_BILINEAR
	FILTER_GAUSSIAN
)

/*
// ----------------------------------------------------------------
type geom.Mat3x2 struct {
	Xx, Yx, Xy, Yy, X0, Y0 float64
}

func (this *geom.Mat3x2) native() *C.cairo_matrix_t {
	return (*C.cairo_matrix_t)(unsafe.Pointer(this))
}

func (this *geom.Mat3x2) Init(xx, yx, xy, yy, x0, y0 float64) {
	C.cairo_matrix_init(this.native(),
		C.double(xx), C.double(yx),
		C.double(xy), C.double(yy),
		C.double(x0), C.double(y0))
}

func (this *geom.Mat3x2) InitIdentity() {
	C.cairo_matrix_init_identity(this.native())
}

func (this *geom.Mat3x2) InitTranslate(tx, ty float64) {
	C.cairo_matrix_init_translate(this.native(), C.double(tx), C.double(ty))
}

func (this *geom.Mat3x2) InitScale(sx, sy float64) {
	C.cairo_matrix_init_scale(this.native(), C.double(sx), C.double(sy))
}

func (this *geom.Mat3x2) InitRotate(radians float64) {
	C.cairo_matrix_init_rotate(this.native(), C.double(radians))
}

func (this *geom.Mat3x2) Translate(tx, ty float64) {
	C.cairo_matrix_translate(this.native(), C.double(tx), C.double(ty))
}

func (this *geom.Mat3x2) Scale(sx, sy float64) {
	C.cairo_matrix_scale(this.native(), C.double(sx), C.double(sy))
}

func (this *geom.Mat3x2) Rotate(radians float64) {
	C.cairo_matrix_rotate(this.native(), C.double(radians))
}

func (this *geom.Mat3x2) Invert() error {
	st := Status(C.cairo_matrix_invert(this.native()))
	if st != STATUS_SUCCESS {
		return errors.New(st.String())
	}
	return nil
}

func Multiplygeom.Mat3x2(result, a, b *geom.Mat3x2) {
	C.cairo_matrix_multiply(result.native(), a.native(), b.native())
}

func (this *geom.Mat3x2) TransformDistance(dx, dy float64) (dx1, dy1 float64) {
	dx1 = dx
	dy1 = dy
	C.cairo_matrix_transform_distance(this.native(), (*C.double)(&dx1), (*C.double)(&dy1))
	return
}

func (this *geom.Mat3x2) TransformPoint(x, y float64) (x1, y1 float64) {
	x1 = x
	y1 = y
	C.cairo_matrix_transform_point(this.native(), (*C.double)(&x1), (*C.double)(&y1))
	return
}
*/
// ----------------------------------------------------------------
type Glyph struct {
	index uint32
	A     uint32 // unused for cairo, but use in lib/gui
	X     float64
	Y     float64
}

//func (g *Glyph) X() float64 {
//	return g.X
//}

//func (g *Glyph) Y() float64 {
//	return g.Y
//}

func (g *Glyph) native() *C.cairo_glyph_t {
	return (*C.cairo_glyph_t)(unsafe.Pointer(g))
}

// ----------------------------------------------------------------
//type Glyphs struct {
//	g *Glyph
//	n int32
//}

//func (gg *Glyphs) native() (*C.cairo_glyph_t, C.int) {
//	return gg.g.native(), C.int(gg.n)
//}

//func destroyGlyphs(g *Glyphs) {
//	if g.g != nil {
//		C.cairo_glyph_free(g.g.native())
//		g.g = nil
//	}
//}

//func newGlyphs(g *Glyph, n int32) *Glyphs {
//	gg := &Glyphs{g, n}
//	runtime.SetFinalizer(gg, destroyGlyphs)
//	return gg
//}

//func (this *Glyphs) At(idx int) *Glyph {
//	p := uintptr(unsafe.Pointer(this.g)) + 24*uintptr(idx)
//	return (*Glyph)(unsafe.Pointer(p))
//}

//func (this *Glyphs) Len() int {
//	return int(this.n)
//}

// ----------------------------------------------------------------
type FontSlant int

const (
	FONT_SLANT_NORMAL FontSlant = iota
	FONT_SLANT_ITALIC
	FONT_SLANT_OBLIQUE
)

// ----------------------------------------------------------------
type FontWeight int

const (
	FONT_WEIGHT_NORMAL FontWeight = iota
	FONT_WEIGHT_BOLD
)

// ----------------------------------------------------------------
type TextCluster struct {
	num_bytes  int32
	num_glyphs int32
}

func (c *TextCluster) native() *C.cairo_text_cluster_t {
	return (*C.cairo_text_cluster_t)(unsafe.Pointer(c))
}

//// ----------------------------------------------------------------
//type TextClusters struct {
//	c *TextCluster
//	n int32
//	f TextClusterFlags
//}

//func (tc *TextClusters) native() (*C.cairo_text_cluster_t, C.int, C.cairo_text_cluster_flags_t) {
//	return tc.c.native(), C.int(tc.n), C.cairo_text_cluster_flags_t(tc.f)
//}

//func destroyTextClusters(c *TextClusters) {
//	if c.c != nil {
//		C.cairo_text_cluster_free(c.c.native())
//		c.c = nil
//	}
//}

//func newTextClusters(c *TextCluster, n int32, f TextClusterFlags) *TextClusters {
//	cc := &TextClusters{c, n, f}
//	runtime.SetFinalizer(cc, destroyTextClusters)
//	return cc
//}

//func (this *TextClusters) At(idx int) *TextCluster {
//	p := uintptr(unsafe.Pointer(cairo_t(this))) + 8*uintptr(idx)
//	return (*TextCluster)(unsafe.Pointer(p))
//}

//func (this *TextClusters) Len() int {
//	return int(this.n)
//}

//func (this *TextClusters) Flags() TextClusterFlags {
//	return this.f
//}

// ----------------------------------------------------------------
type TextClusterFlags int

const (
	TEXT_CLUSTER_FLAG_BACKWARD TextClusterFlags = 1
)

// ----------------------------------------------------------------
type FontFace C.cairo_font_face_t

func font_face_t(f *FontFace) *C.cairo_font_face_t {
	return (*C.cairo_font_face_t)(f)
}

func (this *FontFace) Destroy() {
	C.cairo_font_face_destroy(font_face_t(this))
}

//func newFontFace(f *C.cairo_font_face_t, addRef bool) *FontFace {
//	ff := &FontFace{f}
//	if addRef {
//		C.cairo_font_face_reference(f)
//	}
//	runtime.SetFinalizer(ff, destroyFontFace)
//	return ff
//}

func (this *FontFace) Status() Status {
	return Status(C.cairo_font_face_status(font_face_t(this)))
}

func (this *FontFace) Type() FontType {
	return FontType(C.cairo_font_face_get_type(font_face_t(this)))
}

// cairo_font_face_set_user_data
// cairo_font_face_get_user_data

func NewToyFontFace(family string, slant FontSlant, weight FontWeight) *FontFace {
	cfamily := C.CString(family)
	defer C.free(unsafe.Pointer(cfamily))
	f := C.cairo_toy_font_face_create(cfamily,
		C.cairo_font_slant_t(slant), C.cairo_font_weight_t(weight))
	return (*FontFace)(f)
}

func (this *FontFace) Family() string {
	cs := C.cairo_toy_font_face_get_family(font_face_t(this))
	return C.GoString(cs)
}

func (this *FontFace) Slant() FontSlant {
	return FontSlant(C.cairo_toy_font_face_get_slant(font_face_t(this)))
}

func (this *FontFace) Weight() FontWeight {
	return FontWeight(C.cairo_toy_font_face_get_weight(font_face_t(this)))
}

// cairo_glyph_allocate
// cairo_glyph_free
// cairo_text_cluster_allocate
// cairo_text_cluster_free

// ----------------------------------------------------------------
type FontType int

const (
	FONT_TYPE_TOY FontType = iota
	FONT_TYPE_FT
	FONT_TYPE_WIN32
	FONT_TYPE_QUARTZ
	FONT_TYPE_USER
)

// ----------------------------------------------------------------
type FontExtents struct {
	Ascent      float64
	Descent     float64
	Height      float64
	MaxXAdvance float64
	MaxYAdvance float64
}

// ----------------------------------------------------------------
type TextExtents struct {
	XBearing float64
	YBearing float64
	Width    float64
	Height   float64
	XAdvance float64
	YAdvance float64
}

// ----------------------------------------------------------------
type ScaledFont C.cairo_scaled_font_t

func scaled_font_t(p *ScaledFont) *C.cairo_scaled_font_t {
	return (*C.cairo_scaled_font_t)(p)
}

func (f *ScaledFont) Destroy() {
	C.cairo_scaled_font_destroy(scaled_font_t(f))
}

//func newScaledFont(f *C.cairo_scaled_font_t, addRef bool) *ScaledFont {
//	ff := &ScaledFont{f}
//	if addRef {
//		C.cairo_scaled_font_reference(f)
//	}
//	runtime.SetFinalizer(ff, destroyScaledFont)
//	return ff
//}

func NewScaledFont(face *FontFace, matrix, ctm *geom.Mat3x2, options *FontOptions) *ScaledFont {
	f := C.cairo_scaled_font_create(font_face_t(face), matrix_t(matrix), matrix_t(ctm), font_options_t(options))
	return (*ScaledFont)(f)
}

func (this *ScaledFont) Status() Status {
	return Status(C.cairo_scaled_font_status(scaled_font_t(this)))
}

func (this *ScaledFont) FontExtents() *FontExtents {
	ext := new(FontExtents)
	C.cairo_scaled_font_extents(scaled_font_t(this), (*C.cairo_font_extents_t)(unsafe.Pointer(ext)))
	return ext
}

func (this *ScaledFont) TextExtents(text string) *TextExtents {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	ext := new(TextExtents)
	C.cairo_scaled_font_text_extents(scaled_font_t(this), cs, (*C.cairo_text_extents_t)(unsafe.Pointer(ext)))
	return ext
}

func (this *ScaledFont) GlyphExtents_hack(p unsafe.Pointer, n int) *TextExtents {
	ext := new(TextExtents)
	C.cairo_scaled_font_glyph_extents(scaled_font_t(this), (*C.cairo_glyph_t)(p), C.int(n), (*C.cairo_text_extents_t)(unsafe.Pointer(ext)))
	return ext
}

func (this *ScaledFont) GlyphExtents(glyphs []Glyph) *TextExtents {
	n := len(glyphs)
	var p unsafe.Pointer
	if n == 0 {
		p = nil
	} else {
		p = unsafe.Pointer(&glyphs[0])
	}
	return this.GlyphExtents_hack(p, n)
}

func (this *ScaledFont) TextToGlyphs_hack(x, y float64, text string, out func(buf unsafe.Pointer, num int)) error {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	var glyphs *C.cairo_glyph_t
	var num C.int
	status := C.cairo_scaled_font_text_to_glyphs(scaled_font_t(this), C.double(x), C.double(y),
		cs, C.int(C.strlen(cs)),
		&glyphs, &num,
		(**C.cairo_text_cluster_t)(nil), (*C.int)(nil), (*C.cairo_text_cluster_flags_t)(nil))
	if Status(status) != STATUS_SUCCESS {
		return errors.New(Status(status).String())
	}
	if num == 0 {
		C.cairo_glyph_free(glyphs)
		return nil
	}
	out(unsafe.Pointer(glyphs), (int)(num))
	C.cairo_glyph_free(glyphs)
	return nil
}

func (this *ScaledFont) TextToGlyphs(x, y float64, text string) (ret []Glyph, err error) {
	err = this.TextToGlyphs_hack(x, y, text,
		func(buf unsafe.Pointer, num int) {
			ret = make([]Glyph, num)
			C.memcpy(unsafe.Pointer(&ret[0]), buf, C.size_t(num*24))
		})
	return
}

//func (this *ScaledFont) TextToGlyphsWithClusters(x, y float64, text string) (*Glyphs, *TextClusters) {
//	cs := C.CString(text)
//	defer C.free(unsafe.Pointer(cs))
//	var glyphs *C.cairo_glyph_t
//	var num C.int
//	var clusters *C.cairo_text_cluster_t
//	var num_clusters C.int
//	var cluster_flags C.cairo_text_cluster_flags_t
//	status := C.cairo_scaled_font_text_to_glyphs(scaled_font_t(this), C.double(x), C.double(y),
//		cs, C.int(C.strlen(cs)),
//		&glyphs, &num,
//		&clusters, &num_clusters, &cluster_flags)
//	if Status(status) != STATUS_SUCCESS {
//		core.Warn("TextToGlyphsWithClusters: ", Status(status))
//		return nil, nil
//	}
//	return newGlyphs((*Glyph)(unsafe.Pointer(glyphs)), int32(num)),
//		newTextClusters((*TextCluster)(unsafe.Pointer(clusters)), int32(num_clusters), TextClusterFlags(cluster_flags))
//}

func (this *ScaledFont) FontFace() *FontFace {
	f := C.cairo_scaled_font_get_font_face(scaled_font_t(this))
	C.cairo_font_face_reference(f)
	return (*FontFace)(f)
}

func (this *ScaledFont) FontOptions() *FontOptions {
	options := C.cairo_font_options_create()
	C.cairo_scaled_font_get_font_options(scaled_font_t(this), options)
	return (*FontOptions)(options)
}

func (this *ScaledFont) GetFontMatrix(mat *geom.Mat3x2) {
	C.cairo_scaled_font_get_font_matrix(scaled_font_t(this), matrix_t(mat))
}

func (this *ScaledFont) GetCtm(mat *geom.Mat3x2) {
	C.cairo_scaled_font_get_ctm(scaled_font_t(this), matrix_t(mat))
}

func (this *ScaledFont) GetScaleMatrix(mat *geom.Mat3x2) {
	C.cairo_scaled_font_get_scale_matrix(scaled_font_t(this), matrix_t(mat))
}

func (this *ScaledFont) Type() FontType {
	t := C.cairo_scaled_font_get_type(scaled_font_t(this))
	return FontType(t)
}

// ----------------------------------------------------------------
type SubpixelOrder int

const (
	SUBPIXEL_ORDER_DEFAULT SubpixelOrder = iota
	SUBPIXEL_ORDER_RGB
	SUBPIXEL_ORDER_BGR
	SUBPIXEL_ORDER_VRGB
	SUBPIXEL_ORDER_VBGR
)

// ----------------------------------------------------------------
type HintStyle int

const (
	HINT_STYLE_DEFAULT HintStyle = iota
	HINT_STYLE_NONE
	HINT_STYLE_SLIGHT
	HINT_STYLE_MEDIUM
	HINT_STYLE_FULL
)

// ----------------------------------------------------------------
type HintMetrics int

const (
	HINT_METRICS_DEFAULT HintMetrics = iota
	HINT_METRICS_OFF
	HINT_METRICS_ON
)
