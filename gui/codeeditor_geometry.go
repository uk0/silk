package gui

import (
	"math"
	"strconv"
	"unicode"
)

const editorGutterMarkerWidth = 24.0
const editorGutterNumberInset = 16.0 // separate numbers from fold/diff markers

// Refresh only at text/font boundaries. Idle Draw and pointer hit testing share
// the same width and never measure all line numbers or scan the document.
func (e *CodeEditor) updateGutterWidth() {
	digits := 1
	for n := len(e.lines); n >= 10; n /= 10 {
		digits++
	}
	if e.gutterDigitAdvance <= 0 {
		for i := 0; i < 10; i++ {
			e.gutterDigitAdvance = math.Max(e.gutterDigitAdvance, e.font.TextExtents(strconv.Itoa(i)).XAdvance)
		}
	}
	e.gutterW = math.Max(50, math.Ceil(editorGutterMarkerWidth+float64(digits)*e.gutterDigitAdvance+editorGutterNumberInset))
}

// One coordinate model for painting, click and drag. Rows are visual rows, not
// physical lines: collapsed bodies cannot move the viewport off the overview.
// Small documents retain 3px rows; large documents sample the ENTIRE document
// in at most one row per logical pixel, rather than rendering only its prefix.
type editorMinimapLayout struct {
	top, height, contentHeight  float64
	viewportTop, viewportHeight float64
	maxScroll, lineHeight       float64
	rows, samples               int
}

func makeEditorMinimapLayout(rows int, top, height, lineHeight, scroll float64) editorMinimapLayout {
	m := editorMinimapLayout{top: top, viewportTop: top, height: math.Max(0, height), rows: rows, lineHeight: lineHeight}
	if rows <= 0 || height <= 0 || lineHeight <= 0 {
		return m
	}
	m.contentHeight = math.Min(height, float64(rows)*3)
	m.samples = min(rows, max(1, int(math.Ceil(m.contentHeight))))
	visible := height / lineHeight
	m.maxScroll = math.Max(0, float64(rows)*lineHeight-height)
	m.viewportHeight = math.Min(m.contentHeight, math.Max(8, math.Min(1, visible/float64(rows))*m.contentHeight))
	m.viewportTop = top
	if m.maxScroll > 0 {
		m.viewportTop += (m.contentHeight - m.viewportHeight) * math.Max(0, math.Min(1, scroll/m.maxScroll))
	}
	return m
}
func (m editorMinimapLayout) sampleRow(sample int) int {
	if m.samples <= 1 {
		return 0
	}
	return sample * (m.rows - 1) / (m.samples - 1)
}
func (m editorMinimapLayout) scrollAt(y, grabOffset float64) float64 {
	track := m.contentHeight - m.viewportHeight
	if track <= 0 {
		return 0
	}
	return math.Max(0, math.Min(1, (y-m.top-grabOffset)/track)) * m.maxScroll
}
func (e *CodeEditor) minimapLayout() editorMinimapLayout {
	_, h := e.Size()
	top := e.topOffset()
	return makeEditorMinimapLayout(len(e.visibleLineIndices()), top, h-e.statusBarHeight-top, e.font.FontExtents().Height+2, e.scrollY)
}
func (e *CodeEditor) beginMinimapDrag(x, y float64) bool {
	w, h := e.Size()
	width := e.minimapWidth()
	top := e.topOffset()
	if width <= 0 || x < w-width || x >= w || y < top || y >= h-e.statusBarHeight {
		return false
	}
	m := e.minimapLayout()
	e.mouseDown = false
	e.minimapDragging = true
	e.minimapGrabOffset = m.viewportHeight / 2
	if y >= m.viewportTop && y < m.viewportTop+m.viewportHeight {
		e.minimapGrabOffset = y - m.viewportTop
	}
	e.scrollY = m.scrollAt(y, e.minimapGrabOffset)
	e.Self().Update()
	return true
}
func (e *CodeEditor) dragMinimap(y float64) {
	e.scrollY = e.minimapLayout().scrollAt(y, e.minimapGrabOffset)
	e.Self().Update()
}

// Bound rune work to the 80-column overview, including a single huge token.
func minimapTokenColumns(text string, start, budget, tabSize int) (columns int, ink bool) {
	for _, r := range text {
		if columns >= budget {
			break
		}
		if r == '\t' {
			columns += tabSize - (start+columns)%tabSize
		} else {
			columns++
		}
		ink = ink || !unicode.IsSpace(r)
	}
	return min(columns, budget), ink
}

// Cache bounded overview geometry, not pixels or source strings. Scrolling and
// caret blinking only redraw these small segments; they never hash/tokenize
// hundreds of scattered document lines again. Theme colors are resolved at draw
// time. Text/semantic/fold invalidation shares the existing editor cache seams.
type editorMinimapSegment struct {
	sample, col, columns int
	typ                  tokenType
}
type editorMinimapCache struct {
	valid                  bool
	revision               uint64
	rows, samples, tabSize int
	segments               []editorMinimapSegment
}

func (e *CodeEditor) minimapSegments(m editorMinimapLayout, vis []int) []editorMinimapSegment {
	c := &e.minimapCache
	tabSize := e.TabSize()
	if c.valid && c.revision == e.textRevision && c.samples == m.samples && c.rows == len(vis) && c.tabSize == tabSize {
		return c.segments
	}
	c.valid = false
	c.revision = e.textRevision
	c.samples = m.samples
	c.rows = len(vis)
	c.tabSize = tabSize
	// Do not retain an unusually tall prior viewport's backing array forever.
	if cap(c.segments) > max(256, m.samples*80*2) {
		c.segments = nil
	}
	c.segments = c.segments[:0]
	for sample := 0; sample < m.samples; sample++ {
		col := 0
		for _, tok := range e.tokensForLine(vis[m.sampleRow(sample)]) {
			columns, ink := minimapTokenColumns(tok.text, col, 80-col, tabSize)
			if ink && columns > 0 {
				c.segments = append(c.segments, editorMinimapSegment{sample, col, columns, tok.typ})
			}
			col += columns
			if col >= 80 {
				break
			}
		}
	}
	c.valid = true
	return c.segments
}
