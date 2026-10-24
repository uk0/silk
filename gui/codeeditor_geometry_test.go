package gui

import (
	"math"
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

type editorClipRect struct{ x, y, w, h float64 }
type editorGeometryText struct {
	text string
	x    float64
	clip editorClipRect
}
type editorGeometryRect struct {
	rect  editorClipRect
	color paint.Color
}
type editorGeometryPainter struct {
	editorNoopPainter
	clip, path editorClipRect
	stack      []editorClipRect
	texts      []editorGeometryText
	rects      []editorGeometryRect
	color      paint.Color
}

func (p *editorGeometryPainter) Save() int { p.stack = append(p.stack, p.clip); return len(p.stack) }
func (p *editorGeometryPainter) Restore() int {
	p.clip = p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	return len(p.stack)
}
func (p *editorGeometryPainter) Rectangle(x, y, w, h float64) {
	p.path = editorClipRect{x, y, w, h}
	p.rects = append(p.rects, editorGeometryRect{p.path, p.color})
}
func (p *editorGeometryPainter) Clip() {
	x, y := math.Max(p.clip.x, p.path.x), math.Max(p.clip.y, p.path.y)
	r, b := math.Min(p.clip.x+p.clip.w, p.path.x+p.path.w), math.Min(p.clip.y+p.clip.h, p.path.y+p.path.h)
	p.clip = editorClipRect{x, y, math.Max(0, r-x), math.Max(0, b-y)}
}
func (p *editorGeometryPainter) SetBrush1(c paint.Color) { p.color = c }
func (p *editorGeometryPainter) DrawText1(x, y float64, s string) {
	p.texts = append(p.texts, editorGeometryText{s, x, p.clip})
}
func newEditorGeometryPainter() *editorGeometryPainter {
	return &editorGeometryPainter{clip: editorClipRect{0, 0, 2000, 2000}}
}

func TestEditorGeometryGutterReservesNumbersAndMarkers(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(800, 500)
	e.SetText(strings.Repeat("// line\n", 100000))
	for _, size := range []int{13, 24} {
		e.SetFont(paint.NewFont(codeEditorFontFamily("windows"), size, false, false))
		width := e.font.TextExtents("100001").XAdvance
		if e.gutterW-width-16 < 24 {
			t.Fatalf("font=%v gutter=%v, six digits collide with marker slots (width=%v)", size, e.gutterW, width)
		}
	}
	large := e.gutterW
	e.SetText("small")
	if e.gutterW >= large {
		t.Fatal("gutter did not shrink after replacing large document")
	}
}

func TestEditorGeometryScrolledTextCannotPaintGutter(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(800, 400)
	e.SetText("var LongIdentifier = 1234567890")
	e.scrollX = 120
	e.hasSelection = true
	e.selStartCol = 0
	e.selEndCol = 20
	p := newEditorGeometryPainter()
	e.Draw(p)
	// Viewport culling may omit a wholly offscreen prefix. Require every
	// visible character, its original x position, and the gutter clip instead
	// of requiring an unnecessarily submitted whole token.
	const full = "LongIdentifier"
	fullX := e.gutterW + 10 - e.scrollX + e.measureText("var ")
	first := 0
	for first < len(full)-1 && fullX+e.measureText(full[:first+1]) < e.gutterW {
		first++
	}
	visible := full[first:]
	found := false
	for _, v := range p.texts {
		if v.text != "" && strings.HasSuffix(full, v.text) && strings.HasSuffix(v.text, visible) {
			found = true
			wantX := fullX + e.measureText(full[:len(full)-len(v.text)])
			if math.Abs(v.x-wantX) > 0.001 {
				t.Fatalf("source shifted: x=%v want=%v", v.x, wantX)
			}
			if v.clip.x < e.gutterW {
				t.Fatalf("source text clip starts at %v, gutter ends at %v", v.clip.x, e.gutterW)
			}
		}
	}
	if !found {
		t.Fatal("missing source text draw")
	}
	if len(p.stack) != 0 {
		t.Fatal("unbalanced painter state")
	}
}

func TestEditorGeometryMinimapLargeDocumentBoundsAndTail(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(800, 400)
	e.SetText(strings.Repeat("// top\n", 100000) + "\"TAIL\"")
	lh := e.font.FontExtents().Height + 2
	e.scrollY = 100000 * lh
	p := newEditorGeometryPainter()
	e.drawMinimap(p, 740, 22, 60, 358, lh)
	tail := false
	c := e.editorColors().tokens[tokString]
	for _, v := range p.rects {
		r := v.rect
		if r.w < 0 || r.h < 0 || r.y < 22 || r.y+r.h > 380.0001 {
			t.Fatalf("minimap rectangle outside bounds: %+v", r)
		}
		if v.color.R == c.R && v.color.G == c.G && v.color.B == c.B && r.y > 370 {
			tail = true
		}
	}
	if !tail {
		t.Fatal("minimap only rendered prefix, not document tail")
	}
}

func TestEditorGeometryMinimapMappingRoundTrip(t *testing.T) {
	for _, rows := range []int{0, 1, 10, 1000, 1000000} {
		for _, height := range []float64{0, 0.5, 80, 501} {
			for _, scroll := range []float64{-100, 0, 500, 1e12} {
				m := makeEditorMinimapLayout(rows, 70, height, 18, scroll)
				if m.samples > int(math.Ceil(height)) || m.samples > rows {
					t.Fatalf("unbounded samples: %+v", m)
				}
				if m.viewportHeight < 0 || m.viewportTop < 70 || m.viewportTop+m.viewportHeight > 70+height+1e-8 {
					t.Fatalf("invalid viewport: %+v", m)
				}
				if m.samples > 1 && (m.sampleRow(0) != 0 || m.sampleRow(m.samples-1) != rows-1) {
					t.Fatalf("missing endpoints: %+v", m)
				}
				if m.maxScroll > 0 && m.contentHeight > m.viewportHeight {
					got := m.scrollAt(m.viewportTop+m.viewportHeight/2, m.viewportHeight/2)
					want := math.Max(0, math.Min(m.maxScroll, scroll))
					if math.Abs(got-want) > 1e-6 {
						t.Fatalf("roundtrip %v want %v", got, want)
					}
				}
			}
		}
	}
}

func TestEditorGeometryMinimapDragPreservesCaretAndSelection(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(800, 500)
	e.SetText(strings.Repeat("var x = 1\n", 1000))
	e.MarkSaved()
	e.cursorLine = 12
	e.cursorCol = 4
	e.hasSelection = true
	e.selStartLine = 10
	e.selStartCol = 1
	e.selEndLine = 12
	e.selEndCol = 4
	e.OnLeftDown(780, 450)
	if !e.minimapDragging || e.scrollY <= 0 {
		t.Fatal("minimap did not begin scrolling")
	}
	e.OnMouseMove(200, 10000)
	m := e.minimapLayout()
	if e.scrollY != m.maxScroll {
		t.Fatalf("drag did not clamp to end: %v != %v", e.scrollY, m.maxScroll)
	}
	e.OnLeftUp(200, 10000)
	before := e.scrollY
	e.OnMouseMove(780, 30)
	if e.scrollY != before || e.minimapDragging {
		t.Fatal("released minimap kept dragging")
	}
	if e.cursorLine != 12 || e.cursorCol != 4 || !e.hasSelection || e.selStartLine != 10 || e.selEndLine != 12 || e.Modified() {
		t.Fatal("minimap navigation mutated caret/selection/document")
	}
	e.OnLeftDown(780, 10)
	if e.cursorLine != 12 || e.scrollY != before {
		t.Fatal("breadcrumb click navigated document")
	}
	e.OnLeftDown(780, 450)
	e.SetShowMinimap(false)
	if e.minimapDragging {
		t.Fatal("hidden minimap retained drag")
	}
}

func TestEditorGeometryMinimapFoldedRowsAndWheelExtent(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(800, 500)
	e.SetText("func f() {\n" + strings.Repeat("x++\n", 200) + "}\n" + strings.Repeat("// tail\n", 1000))
	e.ToggleFold(0)
	rows := len(e.visibleLineIndices())
	m := e.minimapLayout()
	if m.rows != rows || m.rows >= len(e.lines) {
		t.Fatal("overview ignored folded rows")
	}
	e.OnMouseWheel(400, 200, -1e6)
	if e.scrollY != m.maxScroll {
		t.Fatalf("wheel used physical lines: %v != %v", e.scrollY, m.maxScroll)
	}
	e.ScrollToLine(len(e.lines) - 1)
	if e.scrollY != m.maxScroll {
		t.Fatal("line navigation omitted top/status bar height")
	}
}

func TestEditorGeometryMinimapTokenBudget(t *testing.T) {
	for _, s := range []string{strings.Repeat("a", 1000000), strings.Repeat("😀", 100000), "\t\tlong", strings.Repeat(" ", 100000)} {
		n, _ := minimapTokenColumns(s, 0, 80, 4)
		if n < 1 || n > 80 {
			t.Fatalf("unbounded token columns %d", n)
		}
	}
	e := NewCodeEditor()
	e.SetSize(70, 40)
	if e.minimapWidth() != 0 {
		t.Fatal("minimap stole the entire narrow editor")
	}
}

func TestEditorGeometryMinimapCacheInvalidation(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(800, 500)
	e.SetText("\tname\nfunc f() {\n x++\n}\n")
	render := func() []editorMinimapSegment {
		vis := e.visibleLineIndices()
		return e.minimapSegments(e.minimapLayout(), vis)
	}
	first := render()
	if len(first) == 0 || !e.minimapCache.valid {
		t.Fatal("empty overview cache")
	}
	ptr := &first[0]
	e.scrollY = 100
	if next := render(); &next[0] != ptr {
		t.Fatal("scroll rebuilt stable geometry")
	}
	layout, vis := e.minimapLayout(), e.visibleLineIndices()
	if n := testing.AllocsPerRun(100, func() { e.minimapSegments(layout, vis) }); n != 0 {
		t.Fatalf("idle overview allocated %v", n)
	}
	e.SetTabSize(8)
	next := render()
	if next[0].col != 8 {
		t.Fatalf("tab change stale: %+v", next[0])
	}
	e.SetSyntaxHighlights(e.TextRevision(), []SyntaxHighlight{{Line: 0, StartCol: 1, EndCol: 5, Kind: SyntaxString}})
	if e.minimapCache.valid {
		t.Fatal("semantic change retained geometry")
	}
	if next = render(); next[0].typ != tokString {
		t.Fatal("semantic colors stale")
	}
	e.ToggleFold(1)
	if e.minimapCache.valid {
		t.Fatal("fold change retained geometry")
	}
	render()
	if e.minimapCache.rows != len(e.visibleLineIndices()) {
		t.Fatal("folded map stale")
	}
	e.SetText("// replacement")
	if e.minimapCache.valid {
		t.Fatal("new text retained geometry")
	}
	if next = render(); len(next) != 1 || next[0].typ != tokComment {
		t.Fatal("new text map stale")
	}
	e.SetFilePath("document.txt")
	if e.minimapCache.valid {
		t.Fatal("language change retained geometry")
	}
	if next = render(); next[0].typ != tokNormal {
		t.Fatal("plaintext kept Go color")
	}
}
