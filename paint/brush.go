package paint

// Brush is the marker interface for any fill source. SolidBrush /
// LinearGradient / RadialGradient stay tag-free here because they
// hold no Cairo state. PixmapBrush has Cairo-specific internals so
// its struct definition + constructor live in brush_cairo.go (with
// a stubbed-out brush_pure.go for silk_no_cairo).
type Brush interface {
}

type SolidBrush struct {
	Color Color
}

func NewSolidBrush(cr Color) *SolidBrush {
	return &SolidBrush{cr}
}

// GradientStop is a single colour stop along a gradient axis.
//
// Offset is in [0,1] where 0 is the start point of the gradient and 1 is
// the end. Stops with offsets outside that range are tolerated by the
// renderer (clamped) but should not appear in well-formed gradients.
type GradientStop struct {
	Offset float32
	Color  Color
}

// LinearGradient is a brush that interpolates colour along a straight axis
// from (X0,Y0) to (X1,Y1) in *local* (pre-transform) coordinates. Add stops
// with AddStop; the first stop sits at offset 0 and the last at offset 1.
type LinearGradient struct {
	X0, Y0, X1, Y1 float32
	Stops          []GradientStop
}

// NewLinearGradient constructs an empty gradient between (x0,y0) and (x1,y1).
// Add stops with AddStop before passing the brush to a Painter.
func NewLinearGradient(x0, y0, x1, y1 float32) *LinearGradient {
	return &LinearGradient{X0: x0, Y0: y0, X1: x1, Y1: y1}
}

// AddStop appends a colour stop at offset (0..1).
func (g *LinearGradient) AddStop(offset float32, c Color) {
	g.Stops = append(g.Stops, GradientStop{Offset: offset, Color: c})
}

// RadialGradient is a brush that interpolates colour radially from a centre
// point at radius R0 (inner) to R1 (outer). Both Cairo and glui render
// every stop: Cairo via cairo_pattern_create_radial; glui via a fragment
// shader that computes per-pixel distance from the centre and samples the
// shared 256×1 colour ramp texture (FillRadialGradientRect). Non-rect
// paths still fall back to a solid fill of the inner stop.
type RadialGradient struct {
	Cx, Cy, R0, R1 float32
	Stops          []GradientStop
}

// NewRadialGradient constructs an empty radial gradient centred at (cx,cy).
// r0 is the inner radius (start) and r1 is the outer radius (end).
func NewRadialGradient(cx, cy, r0, r1 float32) *RadialGradient {
	return &RadialGradient{Cx: cx, Cy: cy, R0: r0, R1: r1}
}

// AddStop appends a colour stop at offset (0..1).
func (g *RadialGradient) AddStop(offset float32, c Color) {
	g.Stops = append(g.Stops, GradientStop{Offset: offset, Color: c})
}

// Pixmap returns the source pixmap so non-Cairo backends can sample
// it directly. Defined on the interface so both Cairo and pure-Go
// PixmapBrush impls expose it.
func (this *PixmapBrush) Pixmap() Pixmap {
	return this.pixmap
}
