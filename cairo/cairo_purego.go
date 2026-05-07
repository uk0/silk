//go:build silk_pure_go

// Package cairo (silk_pure_go build) is a thin re-export of silk's
// purecairo package. The real implementation lives at silk/purecairo
// — this file exists only so silk's existing `import "silk/cairo"`
// statements keep compiling under `-tags silk_pure_go`. Without the
// tag, the libcairo CGO files (cairo.go, cairo_windows.go,
// cgo_unix.go, cgo_windows.go) take over.
//
// Type aliases (`type X = purecairo.X`) preserve identity, so a
// *cairo.Surface returned from this package is the same value as a
// *purecairo.Surface — no wrapping, no conversion cost. Function and
// variable aliases mirror purecairo's exported constructors so silk's
// callers see the same `cairo.NewImageSurface(...)` etc.

package cairo

import "silk/purecairo"

// ===== Constants (re-export) =====

type (
	Status      = purecairo.Status
	Format      = purecairo.Format
	Operator    = purecairo.Operator
	Content     = purecairo.Content
	SurfaceType = purecairo.SurfaceType
	FontSlant   = purecairo.FontSlant
	FontWeight  = purecairo.FontWeight
	Antialias   = purecairo.Antialias
	FillRule    = purecairo.FillRule
	LineCap     = purecairo.LineCap
	LineJoin    = purecairo.LineJoin
	Extend      = purecairo.Extend
	Filter      = purecairo.Filter
	PathDataType = purecairo.PathDataType
)

const (
	STATUS_SUCCESS = purecairo.STATUS_SUCCESS

	FORMAT_INVALID   = purecairo.FORMAT_INVALID
	FORMAT_ARGB32    = purecairo.FORMAT_ARGB32
	FORMAT_RGB24     = purecairo.FORMAT_RGB24
	FORMAT_A8        = purecairo.FORMAT_A8
	FORMAT_A1        = purecairo.FORMAT_A1
	FORMAT_RGB16_565 = purecairo.FORMAT_RGB16_565
	FORMAT_ARGB      = purecairo.FORMAT_ARGB

	OPERATOR_CLEAR          = purecairo.OPERATOR_CLEAR
	OPERATOR_SOURCE         = purecairo.OPERATOR_SOURCE
	OPERATOR_OVER           = purecairo.OPERATOR_OVER
	OPERATOR_IN             = purecairo.OPERATOR_IN
	OPERATOR_OUT            = purecairo.OPERATOR_OUT
	OPERATOR_ATOP           = purecairo.OPERATOR_ATOP
	OPERATOR_DEST           = purecairo.OPERATOR_DEST
	OPERATOR_DEST_OVER      = purecairo.OPERATOR_DEST_OVER
	OPERATOR_DEST_IN        = purecairo.OPERATOR_DEST_IN
	OPERATOR_DEST_OUT       = purecairo.OPERATOR_DEST_OUT
	OPERATOR_DEST_ATOP      = purecairo.OPERATOR_DEST_ATOP
	OPERATOR_XOR            = purecairo.OPERATOR_XOR
	OPERATOR_ADD            = purecairo.OPERATOR_ADD
	OPERATOR_SATURATE       = purecairo.OPERATOR_SATURATE
	OPERATOR_MULTIPLY       = purecairo.OPERATOR_MULTIPLY
	OPERATOR_SCREEN         = purecairo.OPERATOR_SCREEN
	OPERATOR_OVERLAY        = purecairo.OPERATOR_OVERLAY
	OPERATOR_DARKEN         = purecairo.OPERATOR_DARKEN
	OPERATOR_LIGHTEN        = purecairo.OPERATOR_LIGHTEN
	OPERATOR_COLOR_DODGE    = purecairo.OPERATOR_COLOR_DODGE
	OPERATOR_COLOR_BURN     = purecairo.OPERATOR_COLOR_BURN
	OPERATOR_HARD_LIGHT     = purecairo.OPERATOR_HARD_LIGHT
	OPERATOR_SOFT_LIGHT     = purecairo.OPERATOR_SOFT_LIGHT
	OPERATOR_DIFFERENCE     = purecairo.OPERATOR_DIFFERENCE
	OPERATOR_EXCLUSION      = purecairo.OPERATOR_EXCLUSION
	OPERATOR_HSL_HUE        = purecairo.OPERATOR_HSL_HUE
	OPERATOR_HSL_SATURATION = purecairo.OPERATOR_HSL_SATURATION
	OPERATOR_HSL_COLOR      = purecairo.OPERATOR_HSL_COLOR
	OPERATOR_HSL_LUMINOSITY = purecairo.OPERATOR_HSL_LUMINOSITY

	CONTENT_COLOR       = purecairo.CONTENT_COLOR
	CONTENT_ALPHA       = purecairo.CONTENT_ALPHA
	CONTENT_COLOR_ALPHA = purecairo.CONTENT_COLOR_ALPHA

	SURFACE_TYPE_IMAGE  = purecairo.SURFACE_TYPE_IMAGE
	SURFACE_TYPE_PDF    = purecairo.SURFACE_TYPE_PDF
	SURFACE_TYPE_PS     = purecairo.SURFACE_TYPE_PS
	SURFACE_TYPE_XLIB   = purecairo.SURFACE_TYPE_XLIB
	SURFACE_TYPE_XCB    = purecairo.SURFACE_TYPE_XCB
	SURFACE_TYPE_GLITZ  = purecairo.SURFACE_TYPE_GLITZ
	SURFACE_TYPE_QUARTZ = purecairo.SURFACE_TYPE_QUARTZ
	SURFACE_TYPE_WIN32  = purecairo.SURFACE_TYPE_WIN32

	FONT_SLANT_NORMAL  = purecairo.FONT_SLANT_NORMAL
	FONT_SLANT_ITALIC  = purecairo.FONT_SLANT_ITALIC
	FONT_SLANT_OBLIQUE = purecairo.FONT_SLANT_OBLIQUE

	FONT_WEIGHT_NORMAL = purecairo.FONT_WEIGHT_NORMAL
	FONT_WEIGHT_BOLD   = purecairo.FONT_WEIGHT_BOLD

	ANTIALIAS_DEFAULT  = purecairo.ANTIALIAS_DEFAULT
	ANTIALIAS_NONE     = purecairo.ANTIALIAS_NONE
	ANTIALIAS_GRAY     = purecairo.ANTIALIAS_GRAY
	ANTIALIAS_SUBPIXEL = purecairo.ANTIALIAS_SUBPIXEL

	FILL_RULE_WINDING  = purecairo.FILL_RULE_WINDING
	FILL_RULE_EVEN_ODD = purecairo.FILL_RULE_EVEN_ODD

	LINE_CAP_BUTT   = purecairo.LINE_CAP_BUTT
	LINE_CAP_ROUND  = purecairo.LINE_CAP_ROUND
	LINE_CAP_SQUARE = purecairo.LINE_CAP_SQUARE

	LINE_JOIN_MITER = purecairo.LINE_JOIN_MITER
	LINE_JOIN_ROUND = purecairo.LINE_JOIN_ROUND
	LINE_JOIN_BEVEL = purecairo.LINE_JOIN_BEVEL

	EXTEND_NONE    = purecairo.EXTEND_NONE
	EXTEND_REPEAT  = purecairo.EXTEND_REPEAT
	EXTEND_REFLECT = purecairo.EXTEND_REFLECT
	EXTEND_PAD     = purecairo.EXTEND_PAD

	FILTER_FAST     = purecairo.FILTER_FAST
	FILTER_GOOD     = purecairo.FILTER_GOOD
	FILTER_BEST     = purecairo.FILTER_BEST
	FILTER_NEAREST  = purecairo.FILTER_NEAREST
	FILTER_BILINEAR = purecairo.FILTER_BILINEAR
	FILTER_GAUSSIAN = purecairo.FILTER_GAUSSIAN
)

// ===== Types (re-export) =====

type (
	Surface       = purecairo.Surface
	Pattern       = purecairo.Pattern
	Context       = purecairo.Context
	Path          = purecairo.Path
	Device        = purecairo.Device
	FontFace      = purecairo.FontFace
	ScaledFont    = purecairo.ScaledFont
	FontOptions   = purecairo.FontOptions
	FontExtents   = purecairo.FontExtents
	TextExtents   = purecairo.TextExtents
	Glyph         = purecairo.Glyph
	Rectangle     = purecairo.Rectangle
	RectangleInt  = purecairo.RectangleInt
	RectangleList = purecairo.RectangleList
	Dash          = purecairo.Dash
	MappedImage   = purecairo.MappedImage
	Image         = purecairo.Image
)

// ===== Constructors and free functions (re-export) =====

var (
	NewImageSurface             = purecairo.NewImageSurface
	NewImageSurfaceFromPNG      = purecairo.NewImageSurfaceFromPNG
	NewImageSurfaceFromPNGStream = purecairo.NewImageSurfaceFromPNGStream
	NewWin                      = purecairo.NewWin
	NewRGBPattern               = purecairo.NewRGBPattern
	NewRGBAPattern              = purecairo.NewRGBAPattern
	NewPatternForSurface        = purecairo.NewPatternForSurface
	NewLinearPattern            = purecairo.NewLinearPattern
	NewRadialPattern            = purecairo.NewRadialPattern
	NewToyFontFace              = purecairo.NewToyFontFace
	NewScaledFont               = purecairo.NewScaledFont
	NewFontOptions              = purecairo.NewFontOptions
	Multiplygeom_Mat3x2         = purecairo.Multiplygeom_Mat3x2
)
