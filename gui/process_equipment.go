package gui

import (
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
)

// ProcessEquipment is a single, clipped vector widget, not a stack of native
// child windows. It supplies schematic reactor/exchanger/separator symbols;
// it is not a certified P&ID symbol library or a semantic piping editor.
type ProcessEquipment struct {
	stateColor    paint.Color
	hasStateColor bool
	Widget
	metallic             *paint.LinearGradient
	kind, label, tagName string
	running              bool
	level                float64
	quality              core.Quality
}

func init()                                  { core.RegisterFactory("gui.ProcessEquipment", core.TypeOf((*ProcessEquipment)(nil))) }
func NewProcessEquipment() *ProcessEquipment { w := new(ProcessEquipment); w.Init(w); return w }
func (w *ProcessEquipment) Init(self IWidget) {
	w.Widget.Init(self)
	w.kind = "reactor"
	w.label = "R-001"
	w.quality = core.QualityUncertain
}
func (w *ProcessEquipment) Kind() string { return w.kind }
func (w *ProcessEquipment) SetKind(s string) {
	if s == "reactor" || s == "exchanger" || s == "separator" || s == "pump" || s == "valve" || s == "tank" || s == "indicator" {
		if w.kind != s {
			w.kind = s
			w.Update()
		}
	}
}
func (w *ProcessEquipment) Label() string { return w.label }
func (w *ProcessEquipment) SetLabel(s string) {
	if w.label != s {
		w.label = s
		w.Update()
	}
}
func (w *ProcessEquipment) TagName() string     { return w.tagName }
func (w *ProcessEquipment) SetTagName(s string) { w.tagName = s }
func (w *ProcessEquipment) IsRunning() bool     { return w.running && w.quality == core.QualityGood }
func (w *ProcessEquipment) SetRunning(b bool) {
	w.SetSample(core.Value{Raw: b, Quality: core.QualityGood})
}
func (w *ProcessEquipment) SetSample(v core.Value) {
	oldRunning, oldQuality := w.running, w.quality
	w.quality = v.Quality
	if v.Quality == core.QualityGood && w.kind == "tank" {
		n, e := point.Convert(v.Raw, point.Float)
		if e != nil {
			w.quality = core.QualityBad
		} else {
			w.level = max(0, min(1, n.(float64)/100))
			w.running = w.level > 0
		}
		w.Update()
		return
	}
	if v.Quality == core.QualityGood {
		b, e := point.Convert(v.Raw, point.Bool)
		if e != nil {
			w.quality = core.QualityBad
		} else {
			w.running = b.(bool)
		}
	}
	if oldRunning != w.running || oldQuality != w.quality {
		w.Update()
	}
}
func (w *ProcessEquipment) EnumProperties(p core.IPropertyList) {
	p.AddProperty("设备类型", w.Kind, w.SetKind)
	p.AddProperty("设备编号", w.Label, w.SetLabel)
	p.AddProperty("tag", w.TagName, w.SetTagName)
}
func (w *ProcessEquipment) SizeHints() SizeHints {
	return SizeHints{Width: 120, Height: 200, Policy: GrowHorizontal | GrowVertical}
}
func (w *ProcessEquipment) Draw(g paint.Painter) {
	width, height := w.Size()
	if width <= 2 || height <= 2 {
		return
	}
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, width, height)
	g.Clip()
	// All paths in normalized coordinates, preserving tiny/HiDPI clip bounds.
	bodyH := math.Max(1, height-26)
	g.Save()
	g.Scale(width/100, bodyH/160)
	edge := paint.Color{R: 98, G: 117, B: 136, A: 255}
	if w.metallic == nil {
		metallic := paint.NewLinearGradient(15, 0, 85, 0)
		metallic.AddStop(0, paint.Color{R: 78, G: 96, B: 115, A: 255})
		metallic.AddStop(.45, paint.Color{R: 226, G: 234, B: 242, A: 255})
		metallic.AddStop(.65, paint.Color{R: 174, G: 190, B: 206, A: 255})
		metallic.AddStop(1, paint.Color{R: 72, G: 92, B: 112, A: 255})
		w.metallic = metallic
	}
	metallic := w.metallic
	g.SetPen1(edge, 1.5)
	g.SetBrush(metallic)
	if w.hasStateColor && w.quality == core.QualityGood {
		g.SetBrush1(w.stateColor)
	}
	switch w.kind {
	case "pump", "indicator":
		g.Arc(50, 80, 32, 0, math.Pi*2)
		g.FillPreserve()
		g.Stroke()
		g.MoveTo(36, 60)
		g.LineTo(70, 80)
		g.LineTo(36, 100)
		g.LineTo(36, 60)
		g.Stroke()
	case "valve":
		g.MoveTo(15, 50)
		g.LineTo(85, 110)
		g.LineTo(85, 50)
		g.LineTo(15, 110)
		g.LineTo(15, 50)
		g.FillPreserve()
		g.Stroke()
		g.Line(50, 80, 50, 26)
		g.Line(30, 26, 70, 26)
		g.Stroke()
	case "tank":
		roundedRect(g, 20, 20, 60, 120, 10)
		g.FillPreserve()
		g.Stroke()
		g.SetBrush1(paint.Color{R: 32, G: 155, B: 210, A: 220})
		g.Rectangle(24, 136-w.level*110, 52, w.level*110)
		g.Fill()
	case "reactor":
		roundedRect(g, 25, 12, 50, 132, 12)
		g.FillPreserve()
		g.Stroke()
		for _, y := range []float64{38, 64, 90, 116} {
			g.Line(31, y, 69, y)
			g.Stroke()
		}
		g.Line(38, 144, 32, 157)
		g.Line(62, 144, 68, 157)
		g.Stroke()
	case "exchanger":
		roundedRect(g, 8, 52, 84, 60, 17)
		g.FillPreserve()
		g.Stroke()
		for _, y := range []float64{64, 74, 84, 94, 104} {
			g.Line(17, y, 83, y)
			g.Stroke()
		}
		g.Line(30, 112, 25, 150)
		g.Line(70, 112, 75, 150)
		g.Stroke()
	case "separator":
		roundedRect(g, 17, 30, 66, 104, 20)
		g.FillPreserve()
		g.Stroke()
		g.Line(23, 94, 77, 94)
		g.Stroke()
		g.Line(32, 134, 28, 155)
		g.Line(68, 134, 72, 155)
		g.Stroke()
	}
	// Four visible nozzles. No hidden OS windows or idle animation timer.
	left, right, top, bottom, y1, y2 := 25., 75., 12., 144., 42., 120.
	if w.kind == "exchanger" {
		left, right, top, bottom, y1, y2 = 8, 92, 52, 112, 70, 94
	}
	if w.kind == "separator" {
		left, right, top, bottom = 17, 83, 30, 134
	}
	switch w.kind {
	case "pump", "indicator":
		left, right, top, bottom, y1, y2 = 18, 82, 48, 112, 80, 80
	case "valve":
		left, right, top, bottom, y1, y2 = 15, 85, 26, 110, 80, 80
	case "tank":
		left, right, top, bottom, y1, y2 = 20, 80, 20, 140, 80, 80
	}
	for _, v := range [][4]float64{{0, y1, left, y1}, {right, y2, 100, y2}, {50, 0, 50, top}, {50, bottom, 50, 160}} {
		g.Line(v[0], v[1], v[2], v[3])
		g.Stroke()
	}
	color := paint.Color{R: 140, G: 148, B: 160, A: 255}
	if w.quality == core.QualityGood {
		color = paint.Color{R: 214, G: 94, B: 74, A: 255}
		if w.running {
			color = paint.Color{R: 39, G: 190, B: 131, A: 255}
		}
	}
	if w.hasStateColor && w.quality == core.QualityGood {
		color = w.stateColor
	}
	g.SetBrush1(color)
	g.Arc(86, 17, 5, 0, math.Pi*2)
	g.Fill()
	g.Restore()
	g.SetFont(Theme().Font)
	g.SetBrush1(Theme().TextColor)
	g.DrawText1(4, math.Max(12, height-5), elideToWidth(Theme().Font, w.label, math.Max(0, width-8)))
}
