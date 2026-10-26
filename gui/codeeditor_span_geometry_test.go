package gui

import (
	"github.com/uk0/silk/paint"
	"image"
	"math"
	"strings"
	"testing"
)

func BenchmarkEditorLongSpanPainting(b *testing.B) {
	for _, kind := range []string{"find", "selection", "occurrence"} {
		b.Run(kind, func(b *testing.B) {
			e := NewCodeEditor()
			defer e.CancelFind()
			defer e.CancelSymbolIndex()
			e.SetSize(800, 160)
			e.SetText(strings.Repeat("a", 1<<20))
			switch kind {
			case "find":
				e.findActive = true
				e.findMatches = []findMatch{{0, 0, 1 << 20}}
			case "selection":
				e.hasSelection = true
				e.selEndCol = 1 << 20
			case "occurrence":
				e.occurrences = []findMatch{{0, 0, 1 << 20}}
			}
			p := &editorNoopPainter{}
			e.Draw(p)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				e.Draw(p)
			}
		})
	}
}

func TestEditorSpanGeometryRasterParity(t *testing.T) {
	for _, text := range []string{strings.Repeat("abW09", 2048), strings.Repeat("a\tb", 1000), strings.Repeat("中e\u0301🙂\xff", 600)} {
		e := NewCodeEditor()
		e.SetText(text)
		defer e.CancelSymbolIndex()
		e.SetSize(800, 160)
		for _, warm := range []bool{false, true} {
			if warm {
				e.Draw(&editorNoopPainter{})
			}
			for _, scale := range []float64{1, 1.25, 2} {
				for _, scroll := range []float64{0, 55.125, 5000.375} {
					v := editorInkViewport{20.25, 4.5, 190.75, 26.25}
					origin := v.left - scroll
					at := e.textColumnAtX(text, scroll)
					for _, span := range [][2]int{{0, -1}, {at, at + 5}, {at + 4, at + 4}, {at + 1000, at + 1003}, {at, at + 1000}} {
						render := func(reference bool) image.Image {
							pm := paint.NewPixmap(int(220*scale), int(40*scale))
							defer paint.CloseSurface(pm)
							g := pm.NewPainter()
							g.Scale(scale, scale)
							g.SetBrush1(paint.Color{R: 24, G: 24, B: 24, A: 255})
							g.Paint()
							g.Rectangle(v.left, v.top, v.right-v.left, v.bottom-v.top)
							g.Clip()
							g.SetBrush1(paint.Color{R: 80, G: 140, B: 220, A: 160})
							if reference {
								a := e.measureText(e.linePrefix(0, span[0]))
								b := e.measureText(text)
								if span[1] >= 0 {
									b = e.measureText(e.linePrefix(0, span[1]))
								}
								if paint.NeedsClusterLayout(text) {
									layout := paint.LayoutText(e.font.ScaledFont(nil), text)
									a = layout.Carets[min(span[0], len(layout.Carets)-1)]
									end := span[1]
									if end < 0 {
										end = len(layout.Carets) - 1
									}
									b = layout.Carets[min(end, len(layout.Carets)-1)]
								}
								v.fillRect(g, origin+a, 5.25, math.Max(2, b-a), 18.5)
							} else {
								e.fillColumnSpan(g, v, 0, span[0], span[1], origin, 5.25, 18.5, 2, 0)
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
									t.Fatalf("span raster changed warm=%v scale=%v scroll=%v span=%v text=%q at%d,%d", warm, scale, scroll, span, text[:10], x, y)
								}
							}
						}
					}
				}
			}
		}
	}
}
func TestEditorSpanGeometryInvalidatesCertification(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(800, 160)
	e.SetText("abcd")
	defer e.CancelSymbolIndex()
	e.Draw(&editorNoopPainter{})
	if !e.fixedColumnGeometry(0) {
		t.Fatal("ASCII path not certified")
	}
	for _, text := range []string{"a\tb", "a中b", "e\u0301b", "\xffb"} {
		e.lines[0] = text
		if e.fixedColumnGeometry(0) {
			t.Fatal("stale source certification", text)
		}
	}
}
