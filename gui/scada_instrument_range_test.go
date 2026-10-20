package gui

// The SCADA instrument family — Gauge, Tank, Thermometer, ValueBar and
// BarChart — reaches a designed screen through core.New: the palette and the
// .tdoc loader build every widget by reflection, which runs Init and never the
// constructor. Each test below drives a factory-built instrument through its
// public API and then through Draw, so a fix that seeds min/max but leaves the
// value path or the paint geometry broken still fails here.

import (
	"fmt"
	"math"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

const scadaEps = 1e-9

// ---------------------------------------------------------------------------
// Gauge
// ---------------------------------------------------------------------------

func TestGaugeFactoryValueAndScaleSpanItsRange(t *testing.T) {
	g, ok := core.New("gui.Gauge").(*Gauge)
	if !ok {
		t.Fatal("gui.Gauge factory did not produce a *Gauge")
	}
	if g.Min() != 0 || g.Max() != 100 {
		t.Fatalf("factory range = [%v, %v], want [0, 100]", g.Min(), g.Max())
	}
	g.SetValue(50)
	if g.Value() != 50 {
		t.Errorf("SetValue(50) on a factory-built gauge = %v, want 50 (the range clamps it)", g.Value())
	}

	g.SetSize(220, 150)
	rec := newScadaRecorder()
	g.Draw(rec)

	// Draw emits the six scale labels first (i = 0,2,4,6,8,10 of eleven ticks),
	// then the readout. Endpoint ticks already show min/max without duplicates.
	want := []string{"0", "20", "40", "60", "80", "100"}
	if len(rec.texts) < len(want)+1 {
		t.Fatalf("gauge drew %d texts %v, want at least %d (six scale labels plus the readout)",
			len(rec.texts), rec.texts, len(want)+1)
	}
	for i, w := range want {
		if rec.texts[i] != w {
			t.Errorf("scale label %d = %q, want %q (full labels drawn: %v)", i, rec.texts[i], w, rec.texts)
		}
	}
	if got := rec.texts[len(want)]; got != "50.0" {
		t.Errorf("readout = %q, want %q — the needle is not tracking SetValue(50)", got, "50.0")
	}
}

// ---------------------------------------------------------------------------
// BarChart
// ---------------------------------------------------------------------------

func TestBarChartFactoryBarsFitTheirHeight(t *testing.T) {
	b, ok := core.New("gui.BarChart").(*BarChart)
	if !ok {
		t.Fatal("gui.BarChart factory did not produce a *BarChart")
	}
	if !b.AutoScale() {
		t.Error("factory-built BarChart has auto-scaling off, so AddBar never reseeds the value axis")
	}

	red := paint.Color{R: 200, G: 60, B: 60, A: 255}
	b.AddBar("a", 10, red)
	b.AddBar("b", 30, red)
	b.AddBar("c", 70, red)

	const w, h = 300, 218
	b.SetSize(w, h)
	rec := newScadaRecorder()
	b.Draw(rec)

	// Draw fills the background first, then one rectangle per bar.
	if len(rec.fills) != 4 {
		t.Fatalf("BarChart filled %d rectangles %v, want 4 (background plus three bars)", len(rec.fills), rec.fills)
	}
	// Same margins Draw uses for the plotting area.
	const top, bottom = 28.0, 30.0
	chartH := h - top - bottom
	for i, r := range rec.fills[1:] {
		if r.h > chartH+scadaEps {
			t.Errorf("bar %d is %.1fpx tall in a %.1fpx chart area — it paints over the rest of the form",
				i, r.h, chartH)
		}
		if r.y < top-scadaEps || r.y+r.h > h-bottom+scadaEps {
			t.Errorf("bar %d spans y %.1f..%.1f, outside the plotting area %.1f..%.1f",
				i, r.y, r.y+r.h, top, h-bottom)
		}
	}
}

// ---------------------------------------------------------------------------
// Tank
// ---------------------------------------------------------------------------

func TestTankFactoryEngValueAndLabel(t *testing.T) {
	k, ok := core.New("gui.Tank").(*Tank)
	if !ok {
		t.Fatal("gui.Tank factory did not produce a *Tank")
	}
	if k.Min() != 0 || k.Max() != 100 {
		t.Fatalf("factory range = [%v, %v], want [0, 100]", k.Min(), k.Max())
	}
	k.SetLevel(0.5)
	if k.EngValue() != 50 {
		t.Errorf("EngValue at half level = %v, want 50 (the engineering range is collapsed)", k.EngValue())
	}
	if !k.ShowLabel() {
		t.Error("factory-built Tank has its value label off")
	}
	if k.Color().A == 0 {
		t.Errorf("liquid color = %v, fully transparent — the fill would be invisible", k.Color())
	}

	const w, h = 80, 140
	k.SetSize(w, h)
	rec := newScadaRecorder()
	k.Draw(rec)

	// Vessel background then the liquid; the outline is stroked, not filled.
	if len(rec.fills) != 2 {
		t.Fatalf("Tank filled %d rectangles %v, want 2 (vessel plus liquid)", len(rec.fills), rec.fills)
	}
	body := rec.fills[0]
	liquid := rec.fills[1]
	if math.Abs(liquid.h-body.h/2) > scadaEps {
		t.Errorf("liquid is %.1fpx tall in a %.1fpx vessel at half level, want %.1f", liquid.h, body.h, body.h/2)
	}
	if !containsText(rec.texts, "50%") {
		t.Errorf("Tank drew %v, want the %q label", rec.texts, "50%")
	}
}

// ---------------------------------------------------------------------------
// Thermometer
// ---------------------------------------------------------------------------

func TestThermometerFactoryColumnFills(t *testing.T) {
	th, ok := core.New("gui.Thermometer").(*Thermometer)
	if !ok {
		t.Fatal("gui.Thermometer factory did not produce a *Thermometer")
	}
	if th.Min() != 0 || th.Max() != 100 {
		t.Fatalf("factory range = [%v, %v], want [0, 100]", th.Min(), th.Max())
	}
	th.SetValue(50)
	if th.Value() != 50 {
		t.Errorf("SetValue(50) on a factory-built thermometer = %v, want 50 (the range clamps it)", th.Value())
	}
	if math.Abs(th.Fraction()-0.5) > scadaEps {
		t.Errorf("Fraction at mid-scale = %v, want 0.5", th.Fraction())
	}
	if th.Color().A == 0 {
		t.Errorf("mercury color = %v, fully transparent — the column would be invisible", th.Color())
	}

	th.SetSize(40, 140)
	rec := newScadaRecorder()
	th.Draw(rec)

	// The tube background then the mercury column; the bulb is an arc, not a
	// rectangle, and the outline is stroked.
	if len(rec.fills) != 2 {
		t.Fatalf("Thermometer filled %d rectangles %v, want 2 (tube plus mercury) — a mid-scale reading draws no column",
			len(rec.fills), rec.fills)
	}
	tube := rec.fills[0]
	mercury := rec.fills[1]
	if math.Abs(mercury.h-tube.h/2) > scadaEps {
		t.Errorf("mercury is %.1fpx tall in a %.1fpx column at mid-scale, want %.1f", mercury.h, tube.h, tube.h/2)
	}
}

// ---------------------------------------------------------------------------
// ValueBar
// ---------------------------------------------------------------------------

func TestValueBarFactoryTrackFills(t *testing.T) {
	v, ok := core.New("gui.ValueBar").(*ValueBar)
	if !ok {
		t.Fatal("gui.ValueBar factory did not produce a *ValueBar")
	}
	if v.Min() != 0 || v.Max() != 100 {
		t.Fatalf("factory range = [%v, %v], want [0, 100]", v.Min(), v.Max())
	}
	v.SetValue(50)
	if v.Value() != 50 {
		t.Errorf("SetValue(50) on a factory-built bar = %v, want 50 (the range clamps it)", v.Value())
	}
	if math.Abs(v.Fraction()-0.5) > scadaEps {
		t.Errorf("Fraction at mid-scale = %v, want 0.5", v.Fraction())
	}

	v.SetSize(48, 160)
	rec := newScadaRecorder()
	v.Draw(rec)

	if len(rec.fills) != 2 {
		t.Fatalf("ValueBar filled %d rectangles %v, want 2 (track plus bar) — a mid-scale reading draws no bar",
			len(rec.fills), rec.fills)
	}
	track := rec.fills[0]
	bar := rec.fills[1]
	if math.Abs(bar.h-track.h/2) > scadaEps {
		t.Errorf("bar is %.1fpx tall in a %.1fpx track at mid-scale, want %.1f", bar.h, track.h, track.h/2)
	}
	if bar.color.A == 0 {
		t.Errorf("bar color = %v, fully transparent — the fill would be invisible", bar.color)
	}
}

// ---------------------------------------------------------------------------
// The family as a group
// ---------------------------------------------------------------------------

// TestSCADAInstrumentFactoryRanges walks the whole instrument family so a
// screen full of them cannot regress one widget at a time: every check names
// the factory it failed for.
func TestSCADAInstrumentFactoryRanges(t *testing.T) {
	cases := []struct {
		factory string
		// check drives the factory-built instrument to a mid-scale reading and
		// reports what it read back, or nil when the instrument tracked it.
		check func(obj interface{}) error
	}{
		{"gui.Gauge", func(obj interface{}) error {
			g := obj.(*Gauge)
			g.SetValue(50)
			return wantMidScale("Value", g.Value(), g.Min(), g.Max())
		}},
		{"gui.Tank", func(obj interface{}) error {
			k := obj.(*Tank)
			k.SetLevel(0.5)
			return wantMidScale("EngValue", k.EngValue(), k.Min(), k.Max())
		}},
		{"gui.Thermometer", func(obj interface{}) error {
			th := obj.(*Thermometer)
			th.SetValue(50)
			return wantMidScale("Value", th.Value(), th.Min(), th.Max())
		}},
		{"gui.ValueBar", func(obj interface{}) error {
			v := obj.(*ValueBar)
			v.SetValue(50)
			return wantMidScale("Value", v.Value(), v.Min(), v.Max())
		}},
		{"gui.BarChart", func(obj interface{}) error {
			b := obj.(*BarChart)
			b.AddBar("t", 50, paint.Color{R: 200, G: 60, B: 60, A: 255})
			const h = 218.0
			b.SetSize(300, h)
			rec := newScadaRecorder()
			b.Draw(rec)
			if len(rec.fills) != 2 {
				return fmt.Errorf("Draw filled %d rectangles, want 2 (background plus one bar)", len(rec.fills))
			}
			chartH := h - 28 - 30
			if bar := rec.fills[1]; bar.h > chartH+scadaEps {
				return fmt.Errorf("its single bar is %.1fpx tall in a %.1fpx chart area", bar.h, chartH)
			}
			return nil
		}},
	}

	for _, c := range cases {
		obj := core.New(c.factory)
		if obj == nil {
			t.Errorf("%s is not registered with the core factory", c.factory)
			continue
		}
		if err := c.check(obj); err != nil {
			t.Errorf("%s built through the factory: %v", c.factory, err)
		}
	}
}

// wantMidScale reports whether a mid-scale reading landed where the [min,max]
// range says it should.
func wantMidScale(getter string, got, min, max float64) error {
	if max <= min {
		return fmt.Errorf("range is [%v, %v], which clamps every reading to %v", min, max, min)
	}
	want := min + (max-min)/2
	if math.Abs(got-want) > scadaEps {
		return fmt.Errorf("%s() = %v at mid-scale of [%v, %v], want %v", getter, got, min, max, want)
	}
	return nil
}

func containsText(texts []string, want string) bool {
	for _, s := range texts {
		if s == want {
			return true
		}
	}
	return false
}

// --- recorder ---

// scadaFill is one filled rectangle and the flat brush color in force when it
// was filled (zero when the fill used a gradient).
type scadaFill struct {
	x, y, w, h float64
	color      paint.Color
}

// scadaRecorder captures the filled rectangles and the text an instrument
// draws. It embeds nopPainter (spinner_test.go) so a draw op none of these
// widgets used before panics here rather than passing silently. Only
// Rectangle+Fill pairs are recorded: arcs and stroked outlines drop the
// pending rectangle so a bulb or a frame cannot be mistaken for a fill.
type scadaRecorder struct {
	paint.Painter
	pending    scadaFill
	hasPending bool
	brush      paint.Color
	fills      []scadaFill
	texts      []string
}

func newScadaRecorder() *scadaRecorder {
	return &scadaRecorder{Painter: nopPainter{}}
}

// This recorder inspects values/geometry, not raster clipping. Pixel-level
// clipping and theme transitions are covered by sprite_widgets_test.go.
func (r *scadaRecorder) Save() int    { return 0 }
func (r *scadaRecorder) Restore() int { return 0 }
func (r *scadaRecorder) Clip()        { r.hasPending = false }

func (r *scadaRecorder) Rectangle(x, y, w, h float64) {
	r.pending = scadaFill{x, y, w, h, r.brush}
	r.hasPending = true
}

func (r *scadaRecorder) Fill() {
	if r.hasPending {
		r.fills = append(r.fills, r.pending)
		r.hasPending = false
	}
}

func (r *scadaRecorder) Arc(xc, yc, radius, a1, a2 float64)         { r.hasPending = false }
func (r *scadaRecorder) ArcNegative(xc, yc, radius, a1, a2 float64) { r.hasPending = false }
func (r *scadaRecorder) MoveTo(x, y float64)                        { r.hasPending = false }
func (r *scadaRecorder) LineTo(x, y float64)                        {}
func (r *scadaRecorder) Stroke()                                    { r.hasPending = false }
func (r *scadaRecorder) SetBrush1(c paint.Color)                    { r.brush = c }
func (r *scadaRecorder) SetBrush(b paint.Brush)                     { r.brush = paint.Color{} }
func (r *scadaRecorder) SetPen1(c paint.Color, width float64)       {}
func (r *scadaRecorder) SetFont(f paint.Font)                       {}
func (r *scadaRecorder) Font() paint.Font                           { return Theme().Font }
func (r *scadaRecorder) DrawText(s string)                          { r.texts = append(r.texts, s) }
func (r *scadaRecorder) DrawText1(x, y float64, s string)           { r.texts = append(r.texts, s) }
