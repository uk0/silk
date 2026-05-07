package paint

// Surface is the abstract drawing target interface. Concrete impls
// vary by build tag:
//
//   - default build (Cairo enabled): cairoSurface in surface_cairo.go
//   - silk_no_cairo:                imagePixmap in pixmap_pure.go
//
// Surface is embedded by Pixmap so every drawable target has a
// SurfaceType / NewPainter / Flush triple. SurfaceType returns an
// integer; the cairo.SurfaceType constants are mapped through it
// when Cairo is in play, else 0.
//
// Why interface-only here: this file must compile in BOTH build
// configurations. The Cairo-specific cairoSurface struct used to
// live alongside; it has been moved to surface_cairo.go to keep
// this file Cairo-free.
type Surface interface {
	SurfaceType() int
	NewPainter() Painter
	Flush()
}
