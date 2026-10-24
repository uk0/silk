package gui

import (
	"github.com/uk0/silk/paint"
	"math"
	"unicode/utf8"
)

// A painter clip is not a substitute for bounding submitted coordinates:
// native raster backends can overflow their fixed-point paths before clipping.
// Retain a generous guard band for fills: tightly cropping a translucent box
// can change the backend's fractional clip-mask rounding. The band scales with
// the view but never with the document. Stroke outlines must be culled, not
// resized (no artificial edge); one pixel of line overscan preserves edge ink.
type editorInkViewport struct{ left, top, right, bottom float64 }

func (v editorInkViewport) intersects(x, y, w, h, pad float64) bool {
	return v.right > v.left && v.bottom > v.top && w >= 0 && h >= 0 &&
		x+w+pad > v.left && x-pad < v.right && y+h+pad > v.top && y-pad < v.bottom
}

func (v editorInkViewport) fillRect(g paint.Painter, x, y, w, h float64) {
	if !v.intersects(x, y, w, h, 1) {
		return
	}
	guard := math.Max(1024, 2*math.Max(v.right-v.left, v.bottom-v.top))
	right, bottom := math.Min(x+w, math.Ceil(v.right)+guard), math.Min(y+h, math.Ceil(v.bottom)+guard)
	x, y = math.Max(x, math.Floor(v.left)-guard), math.Max(y, math.Floor(v.top)-guard)
	if right > x && bottom > y {
		g.Rectangle(x, y, right-x, bottom-y)
		g.Fill()
	}
}

func (v editorInkViewport) horizontalLine(g paint.Painter, x, end, y float64) {
	if !v.intersects(x, y, end-x, 0, 1) {
		return
	}
	g.MoveTo(math.Max(x, v.left-1), y)
	g.LineTo(math.Min(end, v.right+1), y)
	g.Stroke()
}

// Source byte slices preserve malformed UTF-8/NUL; display conversion remains
// the painter's responsibility. These are read-only UI-thread accessors.
func (e *CodeEditor) linePrefix(line, col int) string {
	if line < 0 || line >= len(e.lines) {
		return ""
	}
	s := e.lines[line]
	p := textPositionInLine(s, col)
	return s[:p.ByteColumn]
}
func classifyFixedCellSource(s string) (bool, [2]uint64) {
	var mask [2]uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\t' {
			mask[0] |= 1 << 9
			continue
		}
		if c < 32 || c > 126 {
			return false, mask
		}
		mask[c/64] |= uint64(1) << (c % 64)
	}
	return true, mask
}
func (e *CodeEditor) fixedCellLine(line int) bool {
	entry := e.tokenCache[line]
	return e.monoFont && entry.fixedCells && entry.asciiMask[0]&e.variableASCII[0] == 0 && entry.asciiMask[1]&e.variableASCII[1] == 0
}

// Font-certified overscan preserves edge ink, including italic overhang. Only
// certified fixed-width ASCII takes this path; never split Unicode shaping runs.
func visibleFixedCellRange(n int, x, cw, left, right float64, pad int) (int, int) {
	if n == 0 || cw <= 0 || right <= left || math.IsNaN(x) {
		return 0, 0
	}
	lo := math.Max(0, math.Floor((left-x)/cw)-float64(pad))
	hi := math.Min(float64(n), math.Ceil((right-x)/cw)+float64(pad))
	if lo >= hi {
		return 0, 0
	}
	return int(lo), int(hi)
}

// Exponential bracketing only measures the clicked prefix, instead of starting
// with half the whole line. Binary refinement preserves nearest-caret tie rules.
func (e *CodeEditor) textColumnAtX(s string, x float64) int {
	if x < 0 || len(s) == 0 {
		return 0
	}
	lo, hi := 0, 1
	width := func(c int) (EditorTextPosition, float64) {
		p := textPositionInLine(s, c)
		return p, e.measureText(s[:p.ByteColumn])
	}
	var p EditorTextPosition
	var highWidth float64
	for {
		p, highWidth = width(hi)
		hi = p.RuneColumn
		if highWidth > x {
			break
		}
		if p.ByteColumn == len(s) {
			return hi
		}
		lo = hi
		hi = min(len(s), hi*2)
	}
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		_, w := width(mid)
		if w > x {
			hi = mid
			highWidth = w
		} else {
			lo = mid
		}
	}
	_, lowWidth := width(lo)
	if x-lowWidth < highWidth-x {
		return lo
	}
	return hi
}

// Match in rune coordinates using one forward/backward byte traversal. In
// particular, a closing bracket must not rebuild []rune(line) for every column.
// This retains the existing textual (not parser/lexical) bracket semantics.
func (e *CodeEditor) matchBracketStreaming() (int, int, bool) {
	if e.cursorLine < 0 || e.cursorLine >= len(e.lines) || e.cursorCol < 0 {
		return 0, 0, false
	}
	s := e.lines[e.cursorLine]
	p := textPositionInLine(s, e.cursorCol)
	if p.RuneColumn != e.cursorCol || p.ByteColumn == len(s) {
		return 0, 0, false
	}
	ch, size := utf8.DecodeRuneInString(s[p.ByteColumn:])
	var match rune
	forward := true
	switch ch {
	case '(':
		match = ')'
	case '[':
		match = ']'
	case '{':
		match = '}'
	case ')':
		match = '('
		forward = false
	case ']':
		match = '['
		forward = false
	case '}':
		match = '{'
		forward = false
	default:
		return 0, 0, false
	}
	depth := 1
	if forward {
		start, col := p.ByteColumn+size, p.RuneColumn+1
		for line := e.cursorLine; line < len(e.lines); line++ {
			for _, r := range e.lines[line][start:] {
				if r == ch {
					depth++
				} else if r == match {
					depth--
					if depth == 0 {
						return line, col, true
					}
				}
				col++
			}
			start, col = 0, 0
		}
	} else {
		end, col := p.ByteColumn, p.RuneColumn-1
		for line := e.cursorLine; line >= 0; line-- {
			s := e.lines[line]
			if line != e.cursorLine {
				end = len(s)
				col = utf8.RuneCountInString(s) - 1
			}
			for end > 0 {
				r, n := utf8.DecodeLastRuneInString(s[:end])
				end -= n
				if r == ch {
					depth++
				} else if r == match {
					depth--
					if depth == 0 {
						return line, col, true
					}
				}
				col--
			}
		}
	}
	return 0, 0, false
}

func (e *CodeEditor) drawViewportText(g paint.Painter, s string, x, y float64, fixed bool) {
	if !fixed {
		g.DrawText1(x, y, s)
		return
	}
	w, _ := e.Size()
	lo, hi := visibleFixedCellRange(len(s), x, e.charWidth, e.gutterW, w-e.minimapWidth(), e.monoOverscan)
	if hi > lo {
		g.DrawText1(x+float64(lo)*e.charWidth, y, s[lo:hi])
	}
}

// Include ink, not just path coordinates: a partially clipped wave/guide can
// still contribute antialiased pixels at either edge of a fractional viewport.
func drawClippedErrorWaves(g paint.Painter, x, width, y, left, right float64) {
	const period = 4.
	start := x + math.Max(0, math.Floor((left-x)/period)-1)*period
	end := math.Min(right+1, x+width)
	for sx := start; sx < end; sx += period {
		g.MoveTo(sx, y)
		g.LineTo(sx+2, y-2)
		g.LineTo(sx+4, y)
		g.Stroke()
	}
}

// A cached certification is usable only for the current source. Tabs and
// variable-width glyphs must keep the shaping path, even with a monospace font.
func (e *CodeEditor) fixedColumnGeometry(line int) bool {
	e.ensureFontMetrics()
	c, ok := e.tokenCache[line]
	return ok && c.source == e.lines[line] && e.fixedCellLine(line) && c.asciiMask[0]&(1<<9) == 0 && e.charWidth > 0
}
func (e *CodeEditor) lineColumnAtX(line int, x float64) int {
	if e.fixedColumnGeometry(line) {
		return int(math.Min(float64(len(e.lines[line])), math.Max(0, math.Floor(x/e.charWidth+0.5))))
	}
	return e.textColumnAtX(e.lines[line], x)
}

// Only fill geometry may be shortened outside the viewport guard band. Never
// submit or shape an entire distant suffix merely to fill the visible wash.
// A negative column denotes end-of-line without counting all its runes first.
func (e *CodeEditor) boundedColumnX(line, col int, right float64) float64 {
	s := e.lines[line]
	if e.fixedColumnGeometry(line) {
		if col < 0 {
			col = len(s)
		}
		return math.Min(float64(min(col, len(s)))*e.charWidth, right)
	}
	// Unicode retains prefix shaping, tabs, combining marks and invalid bytes.
	// This bounds right-of-viewport work; it is not a random-access Unicode index.
	limit := min(len(s), e.textColumnAtX(s, right)+4)
	if col < 0 || col > limit {
		col = limit
	}
	return e.measureText(e.linePrefix(line, col))
}
func (e *CodeEditor) fillColumnSpan(g paint.Painter, v editorInkViewport, line, a, b int, origin, y, height, minWidth, endPadding float64) {
	guard := math.Max(1024, 2*math.Max(v.right-v.left, v.bottom-v.top))
	limit := math.Ceil(v.right) + guard - origin
	x := e.boundedColumnX(line, a, limit)
	end := e.boundedColumnX(line, b, limit) + endPadding
	v.fillRect(g, origin+x, y, math.Max(minWidth, end-x), height)
}
