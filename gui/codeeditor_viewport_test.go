package gui

import (
	"fmt"
	"github.com/uk0/silk/paint"
	"image"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type editorViewportRecorder struct {
	editorNoopPainter
	maxText   int
	textCalls int
}

func (p *editorViewportRecorder) DrawText1(_, _ float64, s string) {
	p.maxText = max(p.maxText, len(s))
	p.textCalls++
}

func TestEditorViewportBoundsASCIITextSubmission(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(800, 600)
	e.SetText(strings.Repeat("abcdef", 1<<17))
	e.ensureFontMetrics()
	if !e.monoFont {
		t.Fatal("default editor font is not monospace; viewport fast path unverified")
	}
	for _, scroll := range []float64{0, 12345, float64(len(e.lines[0])-50) * e.charWidth} {
		e.scrollX = scroll
		p := &editorViewportRecorder{}
		e.Draw(p)
		if p.maxText > 512 {
			t.Fatalf("viewport submitted %d bytes in a draw call at scroll %f", p.maxText, scroll)
		}
	}
}

type editorViewportGeometryRecorder struct {
	editorNoopPainter
	bad [][4]float64
}

func (p *editorViewportGeometryRecorder) Rectangle(x, y, w, h float64) {
	if math.Abs(x) > 10000 || math.Abs(y) > 10000 || w > 10000 || h > 10000 {
		p.bad = append(p.bad, [4]float64{x, y, w, h})
	}
}

func TestEditorViewportRejectsDistantDecorations(t *testing.T) {
	e := NewCodeEditor()
	defer e.CancelSymbolIndex()
	e.SetSize(800, 600)
	e.SetText("(" + strings.Repeat("x", 4<<20) + ")")
	for _, col := range []int{0, len(e.lines[0]) - 1} {
		e.cursorCol = col
		e.scrollX = math.Max(0, e.measureText(e.linePrefix(0, col))-600)
		p := &editorViewportGeometryRecorder{}
		e.Draw(p)
		if len(p.bad) > 0 {
			t.Fatalf("col %d submitted distant geometry: %v", col, p.bad)
		}
	}
	e.hasSelection = true
	e.selStartLine, e.selStartCol, e.selEndLine, e.selEndCol = 0, 0, 0, len(e.lines[0])
	p := &editorViewportGeometryRecorder{}
	e.Draw(p)
	if len(p.bad) > 0 {
		t.Fatalf("selection submitted unbounded geometry: %v", p.bad)
	}
}

func TestEditorViewportHugeBracketRasterParity(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 2} {
		for _, closing := range []bool{false, true} {
			render := func(n int) (image.Image, image.Rectangle) {
				e := NewCodeEditor()
				defer e.CancelSymbolIndex()
				e.SetSize(800, 100)
				e.SetText("(" + strings.Repeat("x", n) + ")")
				if closing {
					e.cursorCol = n + 1
					e.scrollX = e.measureText(e.linePrefix(0, n+1)) - 400
				}
				pm := paint.NewPixmap(int(800*scale), int(100*scale))
				defer paint.CloseSurface(pm)
				g := pm.NewPainter()
				g.Scale(scale, scale)
				e.Draw(g)
				paint.ClosePainter(g)
				im, err := pm.Image()
				if err != nil {
					t.Fatal(err)
				}
				return im, image.Rect(int(math.Ceil(e.gutterW*scale)), int(math.Ceil(e.topOffset()*scale)), int((800-e.minimapWidth())*scale), int((100-e.statusBarHeight)*scale))
			}
			a, area := render(8192) // Matching bracket is offscreen but within native path range.
			b, _ := render(4 << 20)
			for y := area.Min.Y; y < area.Max.Y; y++ {
				for x := area.Min.X; x < area.Max.X; x++ {
					ar, ag, ab, aa := a.At(x, y).RGBA()
					br, bg, bb, ba := b.At(x, y).RGBA()
					if ar != br || ag != bg || ab != bb || aa != ba {
						t.Fatalf("distant bracket changed visible ink scale=%v closing=%v at %d,%d", scale, closing, x, y)
					}
				}
			}
		}
	}
}

func TestEditorViewportDecorationEdgeRasterParity(t *testing.T) {
	v := editorInkViewport{20.25, 4.5, 90.75, 26.25}
	for _, scale := range []float64{1, 1.25, 2} {
		for _, x := range []float64{-8192, 19.5, 20.5, 89.5, 90.5, 8192} {
			for _, width := range []float64{1.5, 20, 16384} {
				render := func(reference bool) image.Image {
					pm := paint.NewPixmap(int(120*scale), int(40*scale))
					defer paint.CloseSurface(pm)
					g := pm.NewPainter()
					g.Scale(scale, scale)
					g.SetBrush1(paint.Color{R: 24, G: 24, B: 24, A: 255})
					g.Paint()
					g.Rectangle(v.left, v.top, v.right-v.left, v.bottom-v.top)
					g.Clip()
					g.SetBrush1(paint.Color{R: 80, G: 140, B: 220, A: 160})
					g.SetPen1(paint.Color{R: 80, G: 160, B: 255, A: 255}, 1)
					if reference {
						g.Rectangle(x, -32, width, 128)
						g.Fill()
						g.MoveTo(x, 15.25)
						g.LineTo(x+width, 15.25)
						g.Stroke()
					} else {
						v.fillRect(g, x, -32, width, 128)
						v.horizontalLine(g, x, x+width, 15.25)
					}
					paint.ClosePainter(g)
					im, err := pm.Image()
					if err != nil {
						t.Fatal(err)
					}
					return im
				}
				a, b := render(true), render(false)
				for y := 0; y < a.Bounds().Dy(); y++ {
					for px := 0; px < a.Bounds().Dx(); px++ {
						ar, ag, ab, aa := a.At(px, y).RGBA()
						br, bg, bb, ba := b.At(px, y).RGBA()
						if ar != br || ag != bg || ab != bb || aa != ba {
							t.Fatalf("decoration clipped ink scale=%v x=%v width=%v at %d,%d ref=%v got=%v", scale, x, width, px, y, []uint32{ar, ag, ab, aa}, []uint32{br, bg, bb, ba})
						}
					}
				}
			}
		}
	}
}

func BenchmarkEditorViewport(b *testing.B) {
	for _, n := range []int{80, 4 << 20} {
		for _, op := range []string{"caret", "hit", "draw-noop", "draw-raster"} {
			b.Run(fmt.Sprintf("%s/%d", op, n), func(b *testing.B) {
				e := NewCodeEditor()
				e.SetSize(800, 600)
				e.SetText("a" + strings.Repeat("z", n))
				e.cursorCol = 1
				defer e.CancelSymbolIndex()
				var g paint.Painter = editorNoopPainter{}
				if op == "draw-raster" {
					surface := paint.NewPixmap(800, 600)
					defer paint.CloseSurface(surface)
					g = surface.NewPainter()
					defer paint.ClosePainter(g)
				}
				run := func() {
					switch op {
					case "caret":
						e.caretLocalXY()
					case "hit":
						e.posFromXY(e.gutterW+10+8, e.topOffset()+5)
					default:
						e.Draw(g)
					}
				}
				run()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					run()
				}
			})
		}
	}
	b.Run("bracket-close/4096", func(b *testing.B) {
		e := NewCodeEditor()
		e.SetText("(" + strings.Repeat("x", 4096) + ")")
		e.cursorCol = 4097
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			e.findMatchingBracket()
		}
	})
}

// Reference is the pre-optimization token renderer, including its tab origins.
func drawViewportReference(e *CodeEditor, g paint.Painter, x, y float64) {
	origin := x
	pal := e.editorColors()
	for _, tok := range e.tokensForLine(0) {
		c, ok := pal.tokens[tok.typ]
		if !ok {
			c = pal.tokens[tokNormal]
		}
		g.SetBrush1(c)
		text := tok.text
		for len(text) > 0 {
			i := strings.IndexByte(text, '\t')
			if i < 0 {
				g.DrawText1(x, y, text)
				x += e.measurePlainText(text)
				break
			}
			if i > 0 {
				g.DrawText1(x, y, text[:i])
				x += e.measurePlainText(text[:i])
			}
			x = origin + e.nextTabStop(x-origin)
			text = text[i+1:]
		}
	}
}
func TestEditorViewportRasterParity(t *testing.T) {
	for _, family := range []string{codeEditorFontFamily(runtime.GOOS), "Arial"} {
		for _, scale := range []float64{1, 1.25, 2} {
			for _, source := range []string{strings.Repeat("fiWz09!()|_", 800), strings.Repeat("ab\t cd\t", 800), strings.Repeat("A中😀e\u0301\tZ ", 20), "a\xffb\x00c\txyz"} {
				for _, scroll := range []float64{0, 17.25, 219.5} {
					e := NewCodeEditor()
					e.SetSize(360, 80)
					e.SetText(source)
					e.SetFont(paint.NewFont(family, 13, false, true))
					e.scrollX = scroll
					render := func(old bool) image.Image {
						surface := paint.NewPixmap(int(360*scale), int(80*scale))
						defer paint.CloseSurface(surface)
						g := surface.NewPainter()
						g.Scale(scale, scale)
						g.SetBrush1(paint.Color{R: 24, G: 24, B: 24, A: 255})
						g.Paint()
						g.SetFont(e.font)
						g.Rectangle(e.gutterW, 0, 360-e.gutterW-e.minimapWidth(), 80)
						g.Clip()
						x := e.gutterW + 10 - scroll
						if old {
							drawViewportReference(e, g, x, 30)
						} else {
							e.drawHighlightedLine(g, 0, x, 30)
						}
						paint.ClosePainter(g)
						img, err := surface.Image()
						if err != nil {
							t.Fatal(err)
						}
						return img
					}
					a, b := render(true), render(false)
					for y := 0; y < a.Bounds().Dy(); y++ {
						for x := 0; x < a.Bounds().Dx(); x++ {
							ar, ag, ab, aa := a.At(x, y).RGBA()
							br, bg, bb, ba := b.At(x, y).RGBA()
							if ar != br || ag != bg || ab != bb || aa != ba {
								t.Fatalf("raster changed font=%s scale=%v scroll=%v fixed=%v at %d,%d source=%q", family, scale, scroll, e.monoFont, x, y, source[:min(len(source), 24)])
							}
						}
					}
					e.CancelSymbolIndex()
				}
			}
		}
	}
}

func TestEditorViewportHitMatchesRuneReference(t *testing.T) {
	for _, family := range []string{codeEditorFontFamily(runtime.GOOS), "Arial"} {
		e := NewCodeEditor()
		e.SetFont(paint.NewFont(family, 13, false, false))
		for _, s := range []string{"", "abc\txyz", "A中😀e\u0301 Z", "a\xffb\x00c", "\u0301\u0301", strings.Repeat("a", 1000)} {
			rs := []rune(s)
			total := e.measureText(s)
			for x := -1.; x < total+10; x += 3.75 {
				want := 0
				if x >= 0 {
					want = len(rs)
					c := sort.Search(len(rs)+1, func(c int) bool { return e.measureText(string(rs[:c])) > x })
					if c <= len(rs) {
						want = c
						if c > 0 {
							lo, hi := e.measureText(string(rs[:c-1])), e.measureText(string(rs[:c]))
							if x-lo < hi-x {
								want = c - 1
							}
						}
					}
				}
				if got := e.textColumnAtX(s, x); got != want {
					t.Fatalf("hit font=%s x=%v got=%d want=%d source=%q", family, x, got, want, s[:min(len(s), 24)])
				}
			}
		}
	}
}

func TestEditorViewportBracketStreaming(t *testing.T) {
	// Exhaustively compare positions to a simple rune-model reference, including
	// malformed bytes, newlines and nested/mismatched bracket kinds.
	r := rand.New(rand.NewSource(24))
	alphabet := []rune("a中😀\x00\ufffd(){}[]\n")
	for n := 0; n < 120; n++ {
		var b strings.Builder
		for j := 0; j < 80; j++ {
			b.WriteRune(alphabet[r.Intn(len(alphabet))])
			if j%19 == 0 {
				b.WriteByte(0xff)
			}
		}
		e := NewCodeEditor()
		e.SetText(b.String())
		rows := make([][]rune, len(e.lines))
		for i := range rows {
			rows[i] = []rune(e.lines[i])
		}
		for l, row := range rows {
			for c := -1; c <= len(row); c++ {
				e.cursorLine, e.cursorCol = l, c
				gl, gc, ok := e.findMatchingBracket()
				wl, wc, wok := viewportBracketReference(rows, l, c)
				if ok != wok || ok && (gl != wl || gc != wc) {
					t.Fatalf("match (%d,%d) got=(%d,%d,%v) want=(%d,%d,%v)", l, c, gl, gc, ok, wl, wc, wok)
				}
			}
		}
	}
	e := NewCodeEditor()
	e.SetText("(" + strings.Repeat("中", 100000) + ")")
	e.cursorCol = 100001
	if l, c, ok := e.findMatchingBracket(); !ok || l != 0 || c != 0 {
		t.Fatal(l, c, ok)
	}
	if n := testing.AllocsPerRun(4, func() { e.findMatchingBracket() }); n != 0 {
		t.Fatalf("bracket allocated %v times", n)
	}
}
func viewportBracketReference(rows [][]rune, l, c int) (int, int, bool) {
	if c < 0 || c >= len(rows[l]) {
		return 0, 0, false
	}
	ch := rows[l][c]
	pair := map[rune]rune{'(': ')', '[': ']', '{': '}', ')': '(', ']': '[', '}': '{'}
	match, ok := pair[ch]
	if !ok {
		return 0, 0, false
	}
	step := 1
	if ch == ')' || ch == ']' || ch == '}' {
		step = -1
	}
	depth := 1
	c += step
	for l >= 0 && l < len(rows) {
		if c >= len(rows[l]) && step > 0 {
			l++
			c = 0
			continue
		}
		if c < 0 && step < 0 {
			l--
			if l >= 0 {
				c = len(rows[l]) - 1
			}
			continue
		}
		if c >= 0 && c < len(rows[l]) {
			v := rows[l][c]
			if v == ch {
				depth++
			} else if v == match {
				depth--
				if depth == 0 {
					return l, c, true
				}
			}
		}
		c += step
	}
	return 0, 0, false
}

type editorDecorationRecorder struct {
	editorNoopPainter
	strokes, fills int
}

func (p *editorDecorationRecorder) Stroke() { p.strokes++ }
func (p *editorDecorationRecorder) Fill()   { p.fills++ }
func TestEditorViewportDecorationBounded(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(800, 600)
	e.SetText(strings.Repeat(" ", 65536) + "x")
	e.SetErrors(map[int]string{0: "error"})
	e.showIndentGuides = true
	for _, x := range []float64{0, 17000} {
		e.scrollX = x
		p := &editorDecorationRecorder{}
		e.Draw(p)
		if p.strokes > 500 || p.fills > 1000 {
			t.Fatalf("offscreen decorations submitted: strokes=%d fills=%d", p.strokes, p.fills)
		}
	}
}
func TestEditorViewportCacheAndBytePrefixes(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("ascii")
	e.tokensForLine(0)
	if !e.tokenCache[0].fixedCells {
		t.Fatal("ASCII not cached")
	}
	e.lines[0] = "a中\xff\x00\t"
	e.rebuildText()
	e.tokensForLine(0)
	if e.tokenCache[0].fixedCells {
		t.Fatal("stale ASCII certificate")
	}
	if got := e.linePrefix(0, 3); got != "a中\xff" {
		t.Fatalf("prefix changed bytes: %q", got)
	}
	e.SetText("ascii")
	e.tokensForLine(0)
	e.SetFont(paint.NewFont("Arial", 15, false, false))
	e.ensureFontMetrics()
	if p := e.linePrefix(-1, 0); p != "" {
		t.Fatal(p)
	}
}

func TestEditorViewportASCIIMetricsUseRealGlyphAdvances(t *testing.T) {
	e := NewCodeEditor()
	for c := byte(32); c <= 126; c++ {
		s := strings.Repeat(string([]byte{c}), 100)
		got, want := e.measurePlainText(s), e.font.TextExtents(s).XAdvance
		if math.Abs(got-want) > 0.001 {
			t.Fatalf("%q advance got %.6f want %.6f", c, got, want)
		}
	}
}

func TestEditorViewportIndentEdgeRasterParity(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(360, 80)
	e.SetText(strings.Repeat(" ", 256) + "x")
	lh := e.font.FontExtents().Height + 2
	tw := e.measurePlainText(" ") * float64(e.TabSize())
	for _, scale := range []float64{1, 1.25, 2} {
		for _, fraction := range []float64{-0.75, -0.5, -0.25, 0, 0.25} {
			e.scrollX = 10 + tw - fraction
			render := func(old bool) image.Image {
				pm := paint.NewPixmap(int(360*scale), int(80*scale))
				defer paint.CloseSurface(pm)
				g := pm.NewPainter()
				g.Scale(scale, scale)
				g.SetBrush1(paint.Color{R: 24, G: 24, B: 24, A: 255})
				g.Paint()
				g.Rectangle(e.gutterW, 0, 360-e.gutterW-e.minimapWidth(), 80)
				g.Clip()
				if old {
					for i := 1; i <= 256/e.TabSize(); i++ {
						gx := e.gutterW + 10 - e.scrollX + float64(i)*tw
						for dy := 0.; dy < lh; dy += 3 {
							g.SetBrush1(e.editorColors().indentGuide)
							g.Rectangle(gx, dy, 1, 1.5)
							g.Fill()
						}
					}
				} else {
					e.drawIndentGuides(g, 0, 0, lh)
				}
				paint.ClosePainter(g)
				im, err := pm.Image()
				if err != nil {
					t.Fatal(err)
				}
				return im
			}
			a, b := render(true), render(false)
			for y := 0; y < a.Bounds().Dy(); y++ {
				for x := 0; x < a.Bounds().Dx(); x++ {
					ar, ag, ab, aa := a.At(x, y).RGBA()
					br, bg, bb, ba := b.At(x, y).RGBA()
					if ar != br || ag != bg || ab != bb || aa != ba {
						t.Fatalf("indent clipped ink scale=%v fraction=%v at %d,%d", scale, fraction, x, y)
					}
				}
			}
		}
	}
}

func TestEditorViewportErrorWaveEdgeRasterParity(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 2} {
		for _, shift := range []float64{0, 17.25, 18.5, 23.75, 96.01} {
			render := func(old bool) image.Image {
				pm := paint.NewPixmap(int(120*scale), int(30*scale))
				defer paint.CloseSurface(pm)
				g := pm.NewPainter()
				g.Scale(scale, scale)
				g.SetBrush1(paint.Color{R: 24, G: 24, B: 24, A: 255})
				g.Paint()
				g.Rectangle(20, 0, 70, 30)
				g.Clip()
				g.SetPen1(paint.Color{R: 220, G: 50, B: 50, A: 200}, 1.2)
				if old {
					for x := 10 - shift; x < 10-shift+8192; x += 4 {
						g.MoveTo(x, 15)
						g.LineTo(x+2, 13)
						g.LineTo(x+4, 15)
						g.Stroke()
					}
				} else {
					drawClippedErrorWaves(g, 10-shift, 8192, 15, 20, 90)
				}
				paint.ClosePainter(g)
				im, err := pm.Image()
				if err != nil {
					t.Fatal(err)
				}
				return im
			}
			a, b := render(true), render(false)
			for y := 0; y < a.Bounds().Dy(); y++ {
				for x := 0; x < a.Bounds().Dx(); x++ {
					ar, ag, ab, aa := a.At(x, y).RGBA()
					br, bg, bb, ba := b.At(x, y).RGBA()
					if ar != br || ag != bg || ab != bb || aa != ba {
						t.Fatalf("wave clipped ink scale=%v shift=%v at %d,%d", scale, shift, x, y)
					}
				}
			}
		}
	}
}
