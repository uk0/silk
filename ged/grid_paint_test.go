package ged

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"math/rand"
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

type gridSegment struct {
	x0, y0, x1, y1 float64
	pen            paint.Color
}
type gridPaintRecorder struct {
	paint.Painter
	lines []gridSegment
	x, y  float64
	pen   paint.Color
	limit int
}

func (r *gridPaintRecorder) SetPen1(c paint.Color, w float64) { r.pen = c; r.Painter.SetPen1(c, w) }
func (r *gridPaintRecorder) MoveTo(x, y float64)              { r.x, r.y = x, y; r.Painter.MoveTo(x, y) }
func (r *gridPaintRecorder) LineTo(x, y float64) {
	if len(r.lines) >= r.limit {
		panic("grid path budget exceeded")
	}
	r.lines = append(r.lines, gridSegment{r.x, r.y, x, y, r.pen})
	r.Painter.LineTo(x, y)
}
func legacyGridPaint(g paint.Painter, x, y, w, h float64, model GridModel) {
	minor, major, want := gridTiers(model)
	if !want {
		return
	}
	g.SetPen1(paint.Color{230, 230, 235, 220}, .1)
	for p := x; p <= x+w; p += minor {
		g.MoveTo(p, y)
		g.LineTo(p, y+h)
	}
	for p := y; p <= y+h; p += minor {
		g.MoveTo(x, p)
		g.LineTo(x+w, p)
	}
	g.Stroke()
	g.SetPen1(paint.Color{200, 210, 220, 240}, .15)
	for p := x; p <= x+w; p += major {
		g.MoveTo(p, y)
		g.LineTo(p, y+h)
	}
	for p := y; p <= y+h; p += major {
		g.MoveTo(x, p)
		g.LineTo(x+w, p)
	}
	g.Stroke()
}
func rasterGrid(t *testing.T, legacy bool, model GridModel, m geom.Mat3x2, clip geom.Rect) image.Image {
	t.Helper()
	pix := paint.NewPixmap(128, 112)
	defer paint.CloseSurface(pix)
	g := pix.NewPainter()
	defer paint.ClosePainter(g)
	g.Rectangle1(clip)
	g.Clip()
	g.SetMatrix(&m)
	g.SetBrush1(paint.Color{29, 30, 36, 255})
	g.Rectangle(-7, -3, 110, 84)
	g.Fill()
	if legacy {
		legacyGridPaint(g, -7, -3, 110, 84, model)
	} else {
		drawDesignGrid(g, -7, -3, 110, 84, model)
	}
	img, err := pix.Image()
	if err != nil {
		t.Fatal(err)
	}
	return img
}
func assertGridImagesEqual(t *testing.T, a, b image.Image) {
	t.Helper()
	if a.Bounds() != b.Bounds() {
		t.Fatal("different raster bounds")
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb || aa != ba {
				t.Fatalf("grid raster changed at %d,%d: %v vs %v", x, y, a.At(x, y), b.At(x, y))
			}
		}
	}
}
func TestDesignGridNormalCairoRasterMatchesLegacy(t *testing.T) {
	matrices := []geom.Mat3x2{{Xx: 1, Yy: 1, X0: 9, Y0: 5}, {Xx: 1.25, Yy: 1.25, X0: 2.3, Y0: -4.7}, {Xx: 2, Yy: 2, X0: -35, Y0: -20}, {Xx: -2, Yy: 2, X0: 120, Y0: 3}, {Xx: 2, Yy: 1.5, Xy: .3, Yx: .2, X0: 15, Y0: -9}}
	var rot geom.Mat3x2
	rot.InitRotate(.17)
	rot.Xx *= 2
	rot.Xy *= 2
	rot.Yx *= 2
	rot.Yy *= 2
	rot.X0 = 20
	matrices = append(matrices, rot)
	for _, pitch := range []float64{2.5, 5, 7.5, 50} {
		for mi, m := range matrices {
			for ci, clip := range []geom.Rect{{0, 0, 128, 112}, {17.25, 11.5, 76.5, 65.25}} {
				model := GridModel{Pitch: pitch, Visible: true, Snap: true}
				t.Run(fmt.Sprintf("pitch-%g/matrix-%d/clip-%d", pitch, mi, ci), func(t *testing.T) {
					a, b := rasterGrid(t, true, model, m, clip), rasterGrid(t, false, model, m, clip)
					assertGridImagesEqual(t, a, b)
				})
			}
		}
	}
}
func TestDesignGridHugeFormRespectsActualCairoClipAndModel(t *testing.T) {
	pix := paint.NewPixmap(64, 64)
	defer paint.CloseSurface(pix)
	base := pix.NewPainter()
	defer paint.ClosePainter(base)
	r := &gridPaintRecorder{Painter: base, limit: 1024}
	scene := NewGedScene()
	scene.SetSize(1e12, 1e12)
	before := scene.SaveDesign().String()
	scene.DrawSelf(r)
	if len(r.lines) != 30 {
		t.Fatal("expected only visible minor/major lines", len(r.lines))
	}
	for _, line := range r.lines {
		if line.x0 == line.x1 {
			if line.x0 < 0 || line.x0 > 64+gridPaintBleed {
				t.Fatal("offscreen vertical line")
			}
		} else if line.y0 < 0 || line.y0 > 64+gridPaintBleed {
			t.Fatal("offscreen horizontal line")
		}
	}
	if scene.SaveDesign().String() != before {
		t.Fatal("paint changed dimensions/grid/serialization")
	}
	img, err := pix.Image()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := img.At(7, 7).RGBA()
	if alpha == 0 {
		t.Fatal("huge background did not cover the visible clip")
	}
	t.Logf("public DrawSelf: clip=64x64 form=1e12x1e12 grid_lines=%d", len(r.lines))
}
func TestDesignGridDenseZoomHasFiniteBoundedWork(t *testing.T) {
	for _, scale := range []float64{1e-12, 1e-6, 1e-3, 1, 100} {
		pix := paint.NewPixmap(64, 64)
		g := pix.NewPainter()
		g.Scale(scale, scale)
		r := &gridPaintRecorder{Painter: g, limit: 4 * maxGridAxisLines}
		drawDesignGrid(r, 0, 0, 1e12, 1e12, defaultGridModel())
		if len(r.lines) > 4*maxGridAxisLines {
			t.Fatal("unbounded draw work")
		}
		for _, line := range r.lines {
			for _, v := range []float64{line.x0, line.y0, line.x1, line.y1} {
				if !finiteGridNumber(v) {
					t.Fatal("nonfinite painter coordinates")
				}
			}
		}
		paint.ClosePainter(g)
		paint.CloseSurface(pix)
	}
	// Raw invalid data may reach the SDK without going through form fields.
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		pix := paint.NewPixmap(16, 16)
		g := pix.NewPainter()
		r := &gridPaintRecorder{Painter: g, limit: 4 * maxGridAxisLines}
		s := NewGedScene()
		s.SetSize(v, 100)
		s.DrawSelf(r)
		if len(r.lines) != 0 {
			t.Fatal("nonfinite scene emitted grid paths")
		}
		paint.ClosePainter(g)
		paint.CloseSurface(pix)
	}
}
func TestDesignGridAxisOriginFringesAndProgress(t *testing.T) {
	a := makeGridAxis(-7, 16.95, 42.05, 5, 3)
	if a.first != 18 || a.pitch != 5 || a.count != 5 {
		t.Fatal("grid drifted to viewport origin", a)
	}
	huge := makeGridAxis(1e20, 1e20, math.Nextafter(1e20, math.Inf(1)), 1, 3)
	if huge.count > maxGridAxisLines {
		t.Fatal("unbounded huge-coordinate span")
	}
	pix := paint.NewPixmap(8, 8)
	g := pix.NewPainter()
	r := &gridPaintRecorder{Painter: g, limit: 4 * maxGridAxisLines}
	huge.draw(r, true, 0, 1)
	if len(r.lines) > 1 {
		t.Fatal("non-advancing float emitted repeated lines")
	}
	paint.ClosePainter(g)
	paint.CloseSurface(pix)
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if a := makeGridAxis(0, 0, 100, v, 1); a.count != 0 {
			t.Fatal("invalid step admitted")
		}
	}
}
func TestDesignGridMatrixNormalSpacingAndNoModelRounding(t *testing.T) {
	sx, sy := gridNormalScales(geom.Mat3x2{Xx: -2, Yy: 3})
	if sx != 2 || sy != 3 {
		t.Fatal("reflection/nonuniform scale", sx, sy)
	}
	sx, sy = gridNormalScales(geom.Mat3x2{Xx: 1000, Xy: 1000, Yy: .001})
	if sx >= .002 || sy >= .002 {
		t.Fatal("shear used long basis rather than perpendicular spacing", sx, sy)
	}
	for _, v := range []string{"NaN", "+Inf", "-Inf", "1e999"} {
		if _, ok := parseMm(v); ok {
			t.Fatal("nonfinite field accepted", v)
		}
	}
	if clampGridPitch(math.NaN()) != defaultGridPitch || clampFormSize(math.Inf(1)) != minFormSize {
		t.Fatal("invalid settings remain nonfinite")
	}
	s := NewGedScene()
	s.SetSize(1e12, 1e12)
	s.SetGrid(GridModel{Pitch: 1.25, Visible: true, Snap: true})
	before := []byte(s.SaveDesign().String())
	pix := paint.NewPixmap(32, 32)
	g := pix.NewPainter()
	g.Scale(.001, .001)
	s.DrawSelf(g)
	paint.ClosePainter(g)
	paint.CloseSurface(pix)
	if !bytes.Equal(before, []byte(s.SaveDesign().String())) || s.Grid().Pitch != 1.25 || !s.Grid().Snap {
		t.Fatal("paint LOD changed authored pitch/snap")
	}
}
func TestDesignGridAxisRandomFiniteAndBudget(t *testing.T) {
	rng := rand.New(rand.NewSource(4049))
	for i := 0; i < 20000; i++ {
		origin := (rng.Float64() - .5) * 1e18
		lo := origin + rng.Float64()*1e18
		hi := lo + rng.Float64()*1e18
		step := math.Pow(10, rng.Float64()*320-160)
		scale := math.Pow(10, rng.Float64()*600-300)
		a := makeGridAxis(origin, lo, hi, step, scale)
		if a.count < 0 || a.count > maxGridAxisLines {
			t.Fatal("bad budget", a)
		}
		if a.count == 0 {
			continue
		}
		if !finiteGridNumber(a.first) || !finiteGridNumber(a.pitch) || a.pitch <= 0 || a.first < lo || a.first > hi {
			t.Fatal("invalid span", a, lo, hi)
		}
	}
}
