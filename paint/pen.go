package paint

type LineStyle interface {
	Width() float64
}

// 考虑到兼容性, 我们的画笔只支持纯色, 不支持图案填充
type Pen interface {
	Width() float64
	Color() Color
}

type pen struct {
	color Color
	width float64
}

func (p *pen) Color() Color {
	return p.color
}

func (p *pen) Width() float64 {
	return p.width
}

func NewPen(cr Color, width float64) *pen {
	return &pen{cr, width}
}

//func NewPen4(width float64, r, g, b uint8) *pen {
//	return &pen{width, Color{r, g, b, 255}}
//}

//func NewPen5(width float64, r, g, b, a uint8) *pen {
//	return &pen{width, Color{r, g, b, a}}
//}

// LineCap selects how the endpoints of a stroked line are rendered.
type LineCap int

const (
	LineCapButt LineCap = iota
	LineCapRound
	LineCapSquare
)

// LineJoin selects how connected line segments meet.
type LineJoin int

const (
	LineJoinMiter LineJoin = iota
	LineJoinBevel
	LineJoinRound
)

// DashedPen is an optional capability interface for Pens that carry
// a dash pattern. Painters can detect dashed pens via type assertion:
//
//	if dp, ok := pen.(paint.DashedPen); ok {
//	    pattern := dp.Dash()
//	    offset := dp.DashOffset()
//	}
//
// Pens that don't implement this stroke as solid lines.
type DashedPen interface {
	Pen
	// Dash returns alternating on/off lengths in points. Empty/nil = solid.
	Dash() []float64
	// DashOffset is the initial phase along the pattern (default 0).
	DashOffset() float64
}

// CappedPen is an optional capability interface for Pens that specify
// a non-default LineCap or LineJoin.
type CappedPen interface {
	Pen
	LineCap() LineCap
	LineJoin() LineJoin
	MiterLimit() float64
}

// RichPen is the union of all extension interfaces — most concrete
// rich pen implementations satisfy this for one-shot type assertion.
type RichPen interface {
	DashedPen
	CappedPen
}

// styledPen is the canonical RichPen implementation. Carries solid colour
// + width like the base pen, plus dash, cap, and join data the GPU stroke
// pipeline can read via the extension-interface type assertions above.
type styledPen struct {
	color    Color
	width    float64
	dash     []float64
	dashOff  float64
	lineCap  LineCap
	lineJoin LineJoin
	miter    float64
}

func (p *styledPen) Color() Color        { return p.color }
func (p *styledPen) Width() float64      { return p.width }
func (p *styledPen) Dash() []float64     { return p.dash }
func (p *styledPen) DashOffset() float64 { return p.dashOff }
func (p *styledPen) LineCap() LineCap    { return p.lineCap }
func (p *styledPen) LineJoin() LineJoin  { return p.lineJoin }

// MiterLimit returns the configured miter limit, or 10 (Cairo's default)
// when the field was left at its zero value. Zero would otherwise force
// every miter into a bevel because of strokeCurrentPath's miter-cap test.
func (p *styledPen) MiterLimit() float64 {
	if p.miter == 0 {
		return 10
	}
	return p.miter
}

// NewStyledPen returns a Pen that carries dash, cap, and join info.
// Callers that need detection by glui's CairoCompat path should use this
// instead of NewPen.
//
// dash: alternating on/off lengths in points (nil for solid).
// dashOffset: initial phase.
// lineCap: end cap style (LineCapButt default).
// lineJoin: join style (LineJoinMiter default).
func NewStyledPen(color Color, width float64, dash []float64,
	dashOffset float64, lineCap LineCap, lineJoin LineJoin) Pen {
	return &styledPen{
		color: color, width: width,
		dash: dash, dashOff: dashOffset,
		lineCap: lineCap, lineJoin: lineJoin,
	}
}
