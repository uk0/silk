package svg

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// parsePathData decodes the SVG path "d" attribute into a sequence of
// PathCmd records normalised to absolute coordinates. Implementation
// is a small hand-rolled lexer + dispatcher — each command letter
// consumes a known number of float args, then we apply the relative-
// to-absolute conversion using a running "current point" cursor.
//
// Supported commands (M L H V C S Q T A Z and lowercase forms):
//
//	M / m  moveto
//	L / l  lineto
//	H / h  horizontal line (collapsed to L)
//	V / v  vertical line (collapsed to L)
//	C / c  cubic Bezier (3 control points)
//	S / s  smooth cubic (reflects last C control)
//	Q / q  quadratic Bezier (1 control)
//	T / t  smooth quadratic (reflects last Q control)
//	A / a  arc — kept as PathArc, no decomposition
//	Z / z  closepath
//
// Out-of-spec or unrecognised commands cause an error so a corrupt
// icon surfaces clearly rather than silently rendering with garbage
// geometry.
func parsePathData(d string) ([]PathCmd, error) {
	p := &pathLexer{src: d}
	var (
		cmds                         []PathCmd
		curX, curY                   float64 // current pen position (absolute)
		startX, startY               float64 // last moveto target — closepath returns here
		lastCubicCtlX, lastCubicCtlY float64 // for smooth-cubic reflection
		lastQuadCtlX, lastQuadCtlY   float64 // for smooth-quadratic reflection
		hadCubic, hadQuad            bool
	)

	for {
		p.skipSeparators()
		if p.eof() {
			break
		}
		cmd := p.readByte()
		// Implicit-repeat-after-moveto: M followed by extra coord pairs
		// are treated as L. We track the "active" command and its case
		// to handle this without a per-command branch.
		args := func(n int) []float64 {
			out := make([]float64, 0, n)
			for i := 0; i < n; i++ {
				p.skipSeparators()
				v, err := p.readNumber()
				if err != nil {
					return nil
				}
				out = append(out, v)
			}
			return out
		}

		emitMove := func(rel bool) {
			a := args(2)
			if a == nil {
				return
			}
			x, y := a[0], a[1]
			if rel {
				x += curX
				y += curY
			}
			cmds = append(cmds, PathCmd{Kind: PathMove, X: x, Y: y})
			curX, curY = x, y
			startX, startY = x, y
			hadCubic, hadQuad = false, false
			// Subsequent coord pairs after an M are interpreted as L.
			for {
				p.skipSeparators()
				if p.eof() {
					return
				}
				if !isNumberStart(p.peek()) {
					return
				}
				a := args(2)
				if a == nil {
					return
				}
				x, y := a[0], a[1]
				if rel {
					x += curX
					y += curY
				}
				cmds = append(cmds, PathCmd{Kind: PathLine, X: x, Y: y})
				curX, curY = x, y
			}
		}

		emitLine := func(rel bool) {
			for {
				p.skipSeparators()
				if p.eof() || !isNumberStart(p.peek()) {
					return
				}
				a := args(2)
				if a == nil {
					return
				}
				x, y := a[0], a[1]
				if rel {
					x += curX
					y += curY
				}
				cmds = append(cmds, PathCmd{Kind: PathLine, X: x, Y: y})
				curX, curY = x, y
				hadCubic, hadQuad = false, false
			}
		}

		emitH := func(rel bool) {
			for {
				p.skipSeparators()
				if p.eof() || !isNumberStart(p.peek()) {
					return
				}
				a := args(1)
				if a == nil {
					return
				}
				x := a[0]
				if rel {
					x += curX
				}
				cmds = append(cmds, PathCmd{Kind: PathLine, X: x, Y: curY})
				curX = x
			}
		}

		emitV := func(rel bool) {
			for {
				p.skipSeparators()
				if p.eof() || !isNumberStart(p.peek()) {
					return
				}
				a := args(1)
				if a == nil {
					return
				}
				y := a[0]
				if rel {
					y += curY
				}
				cmds = append(cmds, PathCmd{Kind: PathLine, X: curX, Y: y})
				curY = y
			}
		}

		emitCubic := func(rel bool) {
			for {
				p.skipSeparators()
				if p.eof() || !isNumberStart(p.peek()) {
					return
				}
				a := args(6)
				if a == nil {
					return
				}
				x1, y1, x2, y2, x, y := a[0], a[1], a[2], a[3], a[4], a[5]
				if rel {
					x1 += curX
					y1 += curY
					x2 += curX
					y2 += curY
					x += curX
					y += curY
				}
				cmds = append(cmds, PathCmd{Kind: PathCurve, X1: x1, Y1: y1, X2: x2, Y2: y2, X: x, Y: y})
				lastCubicCtlX, lastCubicCtlY = x2, y2
				curX, curY = x, y
				hadCubic = true
				hadQuad = false
			}
		}

		emitSmoothCubic := func(rel bool) {
			for {
				p.skipSeparators()
				if p.eof() || !isNumberStart(p.peek()) {
					return
				}
				a := args(4)
				if a == nil {
					return
				}
				x2, y2, x, y := a[0], a[1], a[2], a[3]
				// Reflect the last cubic control point. If the previous
				// command wasn't a cubic, the reflected ctrl is the
				// current point per spec.
				var x1, y1 float64
				if hadCubic {
					x1 = 2*curX - lastCubicCtlX
					y1 = 2*curY - lastCubicCtlY
				} else {
					x1, y1 = curX, curY
				}
				if rel {
					x2 += curX
					y2 += curY
					x += curX
					y += curY
				}
				cmds = append(cmds, PathCmd{Kind: PathCurve, X1: x1, Y1: y1, X2: x2, Y2: y2, X: x, Y: y})
				lastCubicCtlX, lastCubicCtlY = x2, y2
				curX, curY = x, y
				hadCubic = true
				hadQuad = false
			}
		}

		emitQuad := func(rel bool) {
			for {
				p.skipSeparators()
				if p.eof() || !isNumberStart(p.peek()) {
					return
				}
				a := args(4)
				if a == nil {
					return
				}
				x1, y1, x, y := a[0], a[1], a[2], a[3]
				if rel {
					x1 += curX
					y1 += curY
					x += curX
					y += curY
				}
				cmds = append(cmds, PathCmd{Kind: PathQuad, X1: x1, Y1: y1, X: x, Y: y})
				lastQuadCtlX, lastQuadCtlY = x1, y1
				curX, curY = x, y
				hadCubic = false
				hadQuad = true
			}
		}

		emitSmoothQuad := func(rel bool) {
			for {
				p.skipSeparators()
				if p.eof() || !isNumberStart(p.peek()) {
					return
				}
				a := args(2)
				if a == nil {
					return
				}
				x, y := a[0], a[1]
				var x1, y1 float64
				if hadQuad {
					x1 = 2*curX - lastQuadCtlX
					y1 = 2*curY - lastQuadCtlY
				} else {
					x1, y1 = curX, curY
				}
				if rel {
					x += curX
					y += curY
				}
				cmds = append(cmds, PathCmd{Kind: PathQuad, X1: x1, Y1: y1, X: x, Y: y})
				lastQuadCtlX, lastQuadCtlY = x1, y1
				curX, curY = x, y
				hadCubic = false
				hadQuad = true
			}
		}

		emitArc := func(rel bool) {
			for {
				p.skipSeparators()
				if p.eof() || !isNumberStart(p.peek()) {
					return
				}
				// Arc args: rx ry xRot largeArcFlag sweepFlag x y
				a := args(7)
				if a == nil {
					return
				}
				rx, ry, xRot, large, sweep, x, y := a[0], a[1], a[2], a[3], a[4], a[5], a[6]
				if rel {
					x += curX
					y += curY
				}
				cmds = append(cmds, PathCmd{
					Kind: PathArc,
					X1:   rx, Y1: ry,
					X2: xRot,
					A:  large,
					B:  sweep,
					C:  x, D: y,
				})
				curX, curY = x, y
				hadCubic, hadQuad = false, false
			}
		}

		switch cmd {
		case 'M':
			emitMove(false)
		case 'm':
			emitMove(true)
		case 'L':
			emitLine(false)
		case 'l':
			emitLine(true)
		case 'H':
			emitH(false)
		case 'h':
			emitH(true)
		case 'V':
			emitV(false)
		case 'v':
			emitV(true)
		case 'C':
			emitCubic(false)
		case 'c':
			emitCubic(true)
		case 'S':
			emitSmoothCubic(false)
		case 's':
			emitSmoothCubic(true)
		case 'Q':
			emitQuad(false)
		case 'q':
			emitQuad(true)
		case 'T':
			emitSmoothQuad(false)
		case 't':
			emitSmoothQuad(true)
		case 'A':
			emitArc(false)
		case 'a':
			emitArc(true)
		case 'Z', 'z':
			cmds = append(cmds, PathCmd{Kind: PathClose})
			curX, curY = startX, startY
			hadCubic, hadQuad = false, false
		default:
			return nil, errors.New("svg path: unknown command " + string(cmd))
		}
	}
	return cmds, nil
}

// pathLexer is a small hand-rolled lexer over the path data string.
type pathLexer struct {
	src string
	pos int
}

func (l *pathLexer) eof() bool      { return l.pos >= len(l.src) }
func (l *pathLexer) peek() byte     { return l.src[l.pos] }
func (l *pathLexer) readByte() byte { c := l.src[l.pos]; l.pos++; return c }
func (l *pathLexer) skipSeparators() {
	for !l.eof() {
		switch l.src[l.pos] {
		case ' ', '\t', '\n', '\r', ',':
			l.pos++
		default:
			return
		}
	}
}

// readNumber consumes the next signed float — the SVG path-data numeric
// grammar covers leading sign, integer part, fractional part, and
// exponent. We use strconv after collecting the lexed range.
func (l *pathLexer) readNumber() (float64, error) {
	start := l.pos
	if !l.eof() && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
		l.pos++
	}
	hasDigit := false
	for !l.eof() && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
		l.pos++
		hasDigit = true
	}
	if !l.eof() && l.src[l.pos] == '.' {
		l.pos++
		for !l.eof() && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
			l.pos++
			hasDigit = true
		}
	}
	if !l.eof() && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		l.pos++
		if !l.eof() && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
			l.pos++
		}
		for !l.eof() && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
			l.pos++
		}
	}
	if !hasDigit {
		return 0, errors.New("svg path: expected number")
	}
	return strconv.ParseFloat(strings.TrimSpace(l.src[start:l.pos]), 64)
}

// isNumberStart reports whether the byte could begin a path-data
// number. Used to decide when to keep consuming repeated coord pairs
// inside a single command.
func isNumberStart(c byte) bool {
	return c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9')
}

// dummy reference so math import doesn't get pruned in builds where
// the renderer hasn't yet imported it.
var _ = math.Pi
