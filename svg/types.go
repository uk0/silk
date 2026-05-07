package svg

import "silk/paint"

// Doc is the parsed SVG tree. ViewBox defines the coordinate system the
// shapes are drawn in; Width/Height are the document's intrinsic size.
// Render maps ViewBox to the target rectangle.
//
// When Width/Height are zero the renderer falls back to ViewBox
// dimensions; when ViewBox is zero too, it defaults to the target
// rect. This matches what major SVG icon sets ship.
type Doc struct {
	Width    float64
	Height   float64
	ViewBox  ViewBox
	Children []Shape
}

// ViewBox is the SVG viewBox attribute, parsed into separate fields.
// Zero (all zero) means "no viewBox set".
type ViewBox struct {
	X, Y, W, H float64
}

// Empty reports whether the viewBox is the zero value.
func (v ViewBox) Empty() bool { return v.W == 0 && v.H == 0 }

// Shape is the sealed interface every drawable element implements.
// The renderer type-switches on the concrete type to dispatch to the
// right paint.Painter call sequence.
//
// All shapes carry Style and Transform via Common fields; the renderer
// applies those before emitting geometry.
type Shape interface {
	common() *Common
	isShape()
}

// Common holds the style + transform that every shape supports. We
// embed it rather than duplicate fields in each Shape variant.
type Common struct {
	Style     Style
	Transform Transform
}

// --- Style ----------------------------------------------------------

// Style is the resolved presentation attributes for a shape. nil-valued
// pointer fields mean "inherit from parent group" — the renderer's
// state stack walks the parent chain to fill them in. A non-nil value
// is what the SVG actually specified.
type Style struct {
	Fill        *Color  // nil = inherit; (*Color)(nil) value with .None=true means fill="none"
	Stroke      *Color
	StrokeWidth *float64
	Opacity     *float64
	FillOpacity *float64
	StrokeOpacity *float64
	FillRule    FillRule // 0 = inherit; explicit values follow
}

// Color is a wrapper carrying the explicit-vs-none distinction. SVG's
// "none" is structurally different from "transparent black" — a shape
// with fill="none" doesn't fill at all (no SetBrush call), while
// fill="black; opacity:0" still calls Fill but with zero alpha.
type Color struct {
	None bool
	Val  paint.Color
}

// FillRule mirrors SVG's nonzero / evenodd. 0 = inherit.
type FillRule int

const (
	FillRuleInherit FillRule = iota
	FillRuleNonzero
	FillRuleEvenOdd
)

// --- Transform -----------------------------------------------------

// Transform is a small composite of the canonical SVG transform
// operations. Each entry in Ops applies in left-to-right order, which
// matches SVG's "transform=translate(10) rotate(45)" semantics.
type Transform struct {
	Ops []TransformOp
}

// IsIdentity reports whether no transform is set.
func (t Transform) IsIdentity() bool { return len(t.Ops) == 0 }

// TransformOp is one step of a Transform — the union over the SVG
// transform functions. Only one Kind is set per op.
type TransformOp struct {
	Kind TransformKind
	// Translate / Scale: X, Y
	// Rotate: X = angle deg; Y, A, B = optional center (cx, cy) when Has=true
	// Matrix: A..F as in SVG (a b c d e f)
	X, Y, A, B float64
	C, D, E, F float64
	Has        bool
}

// TransformKind enumerates the supported transform operations.
type TransformKind int

const (
	TransformTranslate TransformKind = iota + 1
	TransformScale
	TransformRotate
	TransformMatrix
)

// --- Concrete shape variants ---------------------------------------

// Rect renders an axis-aligned rectangle, optionally with rounded
// corners (Rx / Ry).
type Rect struct {
	Common
	X, Y, W, H float64
	Rx, Ry     float64
}

func (r *Rect) common() *Common { return &r.Common }
func (*Rect) isShape()          {}

// Circle is a centered circle.
type Circle struct {
	Common
	Cx, Cy, R float64
}

func (c *Circle) common() *Common { return &c.Common }
func (*Circle) isShape()          {}

// Ellipse is a centered ellipse with two radii.
type Ellipse struct {
	Common
	Cx, Cy, Rx, Ry float64
}

func (e *Ellipse) common() *Common { return &e.Common }
func (*Ellipse) isShape()          {}

// Line is a single segment.
type Line struct {
	Common
	X1, Y1, X2, Y2 float64
}

func (l *Line) common() *Common { return &l.Common }
func (*Line) isShape()          {}

// Polygon is a closed polyline.
type Polygon struct {
	Common
	Points []Point
}

func (p *Polygon) common() *Common { return &p.Common }
func (*Polygon) isShape()          {}

// Polyline is an open polyline (no closing edge).
type Polyline struct {
	Common
	Points []Point
}

func (p *Polyline) common() *Common { return &p.Common }
func (*Polyline) isShape()          {}

// Point is a single (x, y) used by Polygon / Polyline.
type Point struct{ X, Y float64 }

// Path is the fully general shape with a list of low-level commands
// produced by the path-data parser. PathCmd union below.
type Path struct {
	Common
	Commands []PathCmd
}

func (p *Path) common() *Common { return &p.Common }
func (*Path) isShape()          {}

// PathCmd is one command from an SVG path "d" attribute. We normalise
// every command to absolute coordinates at parse time so the renderer
// can dispatch without per-command relative bookkeeping.
type PathCmd struct {
	Kind PathCmdKind
	// Move / Line: X, Y
	// CurveTo: X1, Y1 (control1), X2, Y2 (control2), X, Y (end)
	// QuadTo:  X1, Y1 (control), X, Y (end)
	// Arc: X1=rx, Y1=ry, X2=xRot, A=largeArcFlag, B=sweepFlag, C=x, D=y
	// Close: no fields
	X, Y, X1, Y1, X2, Y2 float64
	A, B, C, D           float64
}

// PathCmdKind enumerates the command set we honour. SVG's full set is
// MmLlHhVvCcSsQqTtAaZz but we normalise H/V into L and convert all
// to absolute, so the runtime kinds collapse into the five below.
type PathCmdKind int

const (
	PathMove PathCmdKind = iota + 1
	PathLine
	PathCurve
	PathQuad
	PathArc
	PathClose
)

// Group is a <g> element with nested children. Its Common style /
// transform cascades onto every descendant via the renderer's stack.
type Group struct {
	Common
	Children []Shape
}

func (g *Group) common() *Common { return &g.Common }
func (*Group) isShape()          {}
