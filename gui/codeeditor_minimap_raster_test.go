package gui

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// Independent pre-cache renderer: do not share drawMinimapContent, so changes
// to rectangle ordering, alpha or clipping cannot silently change the oracle.
func drawMinimapVectorReference(e *CodeEditor, g paint.Painter, x, y, w, h, lh float64) {
	g.Save()
	defer g.Restore()
	g.Rectangle(x, y, w, h)
	g.Clip()
	pal := e.editorColors()
	g.SetBrush1(pal.minimapBG)
	g.Rectangle(x, y, w, h)
	g.Fill()
	g.SetPen1(pal.minimapBorder, 1)
	g.Line(x, y, x, y+h)
	g.Stroke()
	vis := e.visibleLineIndices()
	m := makeEditorMinimapLayout(len(vis), y, h, lh, e.scrollY)
	if m.samples == 0 {
		return
	}
	rowHeight, scale := m.contentHeight/float64(m.samples), math.Max(0, w-6)
	for _, s := range e.minimapSegments(m, vis) {
		color := pal.minimapDim
		if c, ok := pal.tokens[s.typ]; ok && s.typ != tokNormal {
			color = paint.Color{R: c.R, G: c.G, B: c.B, A: 200}
		}
		g.SetBrush1(color)
		g.Rectangle(x+3+scale*float64(s.col)/80, y+float64(s.sample)*rowHeight, scale*float64(s.columns)/80, rowHeight)
		g.Fill()
	}
	g.SetBrush1(paint.Color{R: 60, G: 100, B: 180, A: 50})
	g.Rectangle(x, m.viewportTop, w, m.viewportHeight)
	g.Fill()
	g.SetPen1(paint.Color{R: 80, G: 130, B: 210, A: 100}, 1)
	g.Rectangle(x, m.viewportTop, w, m.viewportHeight)
	g.Stroke()
}

func minimapRasterImage(t *testing.T, e *CodeEditor, scale, phase float64, partial, reference bool) image.Image {
	t.Helper()
	pm := paint.NewPixmap(int(100*scale)+4, int(160*scale)+4)
	defer paint.CloseSurface(pm)
	g := pm.NewPainter()
	g.SetBrush1(paint.Color{R: 211, G: 173, B: 109, A: 255})
	g.Paint()
	g.Scale(scale, scale)
	g.Translate(phase, phase)
	if partial {
		g.Rectangle(23.75, 42.25, 37.5, 60.5)
		g.Clip()
	}
	if reference {
		drawMinimapVectorReference(e, g, 20, 4, 60, 148-phase, 16)
	} else {
		e.drawMinimap(g, 20, 4, 60, 148-phase, 16)
	}
	paint.ClosePainter(g)
	im, err := pm.Image()
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func assertMinimapPixels(t *testing.T, got, want image.Image) {
	t.Helper()
	for y := 0; y < got.Bounds().Dy(); y++ {
		for x := 0; x < got.Bounds().Dx(); x++ {
			ar, ag, ab, aa := got.At(x, y).RGBA()
			br, bg, bb, ba := want.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb || aa != ba {
				t.Fatalf("minimap raster differs at %d,%d: %v vs %v", x, y, got.At(x, y), want.At(x, y))
			}
		}
	}
}

func TestMinimapRasterExactPixelsAndIncrementalDamage(t *testing.T) {
	nativeCache := minimapNativeCacheAvailable()
	oldMode := CurrentThemeMode()
	defer SetThemeMode(oldMode)
	for _, mode := range []ThemeMode{ThemeDark, ThemeLight} {
		SetThemeMode(mode)
		for _, scale := range []float64{1, 1.25, 1.5, 2, 2.5} {
			for _, phase := range []float64{0, 0.125, 0.955078125} {
				t.Run(fmt.Sprint(mode, "/", scale, "/", phase), func(t *testing.T) {
					e := NewCodeEditor()
					defer e.minimapRaster.release()
					e.SetText(strings.Repeat("func f(x int) string { return `hi` }\n\t// comment\n", 100))
					compare := func(partial bool) {
						got := minimapRasterImage(t, e, scale, phase, partial, false)
						want := minimapRasterImage(t, e, scale, phase, partial, true)
						assertMinimapPixels(t, got, want)
					}
					compare(false)
					if nativeCache && e.minimapRaster.pixmap == nil {
						t.Fatal("render failed to exercise interior-pixel caching")
					}
					previous := e.minimapRaster.pixmap
					e.scrollY = 2200
					compare(false)
					if previous != e.minimapRaster.pixmap {
						t.Fatal("scroll or partial clip replaced a stable cache")
					}
					compare(true)
					// Exercise changed/deleted/inserted tokens, adjacent row damage
					// at fractional pixel heights, syntax and full-layout changes.
					for _, text := range []string{
						"func f(x int) string { return `hi` }\n\t// comment\n",
						"\tname\n\"wide\"\n// hi\n",
						"\tname\n\"x\"\n// hi\n",
						"\tname\n\n// hi\n",
						"\tname\nvar n=3\n// hi\n",
						"",
					} {
						e.SetText(text)
						compare(false)
						compare(true)
					}
					e.SetText(strings.Repeat("var n = 123\n", 70))
					compare(false)
					e.SetText(strings.Repeat("var n = 123\n", 69) + "// replaced\n")
					compare(false)
					e.SetSyntaxHighlights(e.TextRevision(), []SyntaxHighlight{{Line: 0, StartCol: 0, EndCol: 3, Kind: SyntaxString}})
					compare(false)
					e.SetTabSize(8)
					compare(false)
					e.OnHide()
					if e.minimapRaster.pixmap != nil || e.minimapRaster.painter != nil {
						t.Fatal("hidden editor retained native overview resources")
					}
				})
			}
		}
	}
}

func TestMinimapRasterKeyChangesAndLimits(t *testing.T) {
	if !minimapNativeCacheAvailable() {
		t.Skip("native exact-rectangle clip capability unavailable; pixels tested through fallback")
	}
	pm := paint.NewPixmap(400, 400)
	defer paint.CloseSurface(pm)
	g := pm.NewPainter()
	defer paint.ClosePainter(g)
	m := makeEditorMinimapLayout(100, 0, 148, 16, 0)
	pal := editorColorsFor(ThemeDark)
	key, _, _, ok := minimapRasterKey(g, 20, 4, 60, 148, m, pal)
	if !ok {
		t.Fatal("ordinary raster target rejected")
	}
	changed := *pal
	changed.tokens = map[tokenType]paint.Color{tokKeyword: {R: 241, A: 255}}
	other, _, _, _ := minimapRasterKey(g, 20, 4, 60, 148, m, &changed)
	if key == other {
		t.Fatal("palette mutation not part of raster identity")
	}
	for _, dims := range [][2]float64{{1e8, 1}, {1, 1e8}, {2000, 2000}, {0, 1}, {math.NaN(), 1}, {1, math.Inf(1)}} {
		if _, _, _, ok := minimapRasterKey(g, 0, 0, dims[0], dims[1], m, pal); ok {
			t.Fatalf("unbounded/invalid dimensions accepted %v", dims)
		}
	}
	for _, matrix := range []geom.Mat3x2{{Xx: -1, Yy: 1}, {Xx: 1, Yy: 1, Xy: .2}, {Xx: 1, Yy: 1, Yx: .2}} {
		g.SetMatrix(&matrix)
		if _, _, _, ok := minimapRasterKey(g, 0, 0, 60, 148, m, pal); ok {
			t.Fatal("non-axis-aligned transform accepted")
		}
	}
	g.ResetMatrix()
	g.SetOperator(paint.OpSource)
	if _, _, _, ok := minimapRasterKey(g, 0, 0, 60, 148, m, pal); ok {
		t.Fatal("non-OVER operation cached")
	}
}

func TestMinimapChangedSamples(t *testing.T) {
	a := []editorMinimapSegment{{0, 0, 1, tokNormal}, {4, 0, 1, tokString}, {9, 0, 1, tokComment}}
	for _, tc := range []struct {
		next        []editorMinimapSegment
		first, last int
	}{
		{a, 1, 0}, {nil, 0, 9}, {a[:2], 9, 9},
		{[]editorMinimapSegment{{0, 0, 2, tokNormal}, a[1], a[2]}, 0, 0},
		{[]editorMinimapSegment{a[0], {2, 0, 1, tokKeyword}, a[1], a[2]}, 2, 2},
	} {
		first, last := changedMinimapSamples(a, tc.next)
		if first != tc.first || last != tc.last {
			t.Fatalf("range=(%d,%d), want=(%d,%d)", first, last, tc.first, tc.last)
		}
	}
}

func TestMinimapRasterRetiresOwnedResources(t *testing.T) {
	if !minimapNativeCacheAvailable() {
		t.Skip("native exact-rectangle clip capability unavailable")
	}
	for _, action := range []string{"hide", "disable", "detach", "reparent"} {
		t.Run(action, func(t *testing.T) {
			e := NewCodeEditor()
			defer e.minimapRaster.release()
			e.SetText("func main() {}")
			minimapRasterImage(t, e, 1, 0.125, false, false)
			if e.minimapRaster.pixmap == nil || e.minimapRaster.scratch == nil || e.minimapRaster.ink == nil {
				t.Fatal("resource ownership test did not populate both native surfaces")
			}
			switch action {
			case "hide":
				e.OnHide()
			case "disable":
				e.SetShowMinimap(false)
			case "detach":
				e.SetParent(nil)
			case "reparent":
				e.SetParent(NewFrame())
			}
			if e.minimapRaster.pixmap != nil || e.minimapRaster.painter != nil || e.minimapRaster.scratch != nil || e.minimapRaster.ink != nil {
				t.Fatal("native raster ownership outlived editor transition")
			}
		})
	}
}

func minimapNativeCacheAvailable() bool {
	pm := paint.NewPixmap(10, 10)
	defer paint.CloseSurface(pm)
	p := pm.NewPainter()
	defer paint.ClosePainter(p)
	capability, ok := p.(interface{ SupportsRasterCache() bool })
	return ok && capability.SupportsRasterCache()
}

func BenchmarkMinimapRaster(b *testing.B) {
	for _, cached := range []bool{false, true} {
		b.Run(fmt.Sprint("cached=", cached), func(b *testing.B) {
			e := NewCodeEditor()
			defer e.minimapRaster.release()
			e.SetText(strings.Repeat("func f(x int) string { return `hi` }\n", 5000))
			pm := paint.NewPixmap(80, 720)
			defer paint.CloseSurface(pm)
			g := pm.NewPainter()
			defer paint.ClosePainter(g)
			e.drawMinimap(g, 0, 0, 60, 700, 16)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if cached {
					e.drawMinimap(g, 0, 0, 60, 700, 16)
				} else {
					drawMinimapVectorReference(e, g, 0, 0, 60, 700, 16)
				}
			}
		})
	}
}
