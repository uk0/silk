package ged

import (
	"fmt"
	"math"

	"silk/core"
	"silk/gui"
	"silk/paint"
)

func init() {
	core.RegisterFactory("ged.FormPresetsPanel", gui.TypeOf(FormPresetsPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.FormPresetsPanel",
		Name: "尺寸",
		Icon: "window",
		Desc: "表单尺寸预设 / 设备预览",
	})
}

// FormPreset defines a named form size.
type FormPreset struct {
	Name   string
	Width  float64 // in mm
	Height float64 // in mm
	Icon   string  // device type: desktop, laptop, tablet, phone, custom
}

// defaultPresets provides common device sizes (approximate mm at 96 dpi).
var defaultPresets = []FormPreset{
	{"Desktop HD", 200, 120, "desktop"},
	{"Desktop", 160, 100, "desktop"},
	{"Laptop", 130, 80, "laptop"},
	{"Tablet Portrait", 80, 110, "tablet"},
	{"Tablet Landscape", 110, 80, "tablet"},
	{"Phone Portrait", 40, 70, "phone"},
	{"Phone Landscape", 70, 40, "phone"},
	{"Custom...", 0, 0, "custom"},
}

// FormPresetsPanel displays a grid of preset cards for quickly resizing the
// current form/scene to common device dimensions.
type FormPresetsPanel struct {
	gui.Widget
	presets  []FormPreset
	hoverIdx int
	scene    *GedScene
	scrollY  float64
}

func NewFormPresetsPanel() *FormPresetsPanel {
	p := new(FormPresetsPanel)
	p.Init(p)
	return p
}

func (this *FormPresetsPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.presets = append([]FormPreset{}, defaultPresets...)
	this.hoverIdx = -1
}

// SetScene binds the panel to the current GedScene.
func (this *FormPresetsPanel) SetScene(scene *GedScene) {
	this.scene = scene
	this.Self().Update()
}

// ---------------------------------------------------------------------------
// Layout constants
// ---------------------------------------------------------------------------

const (
	fpPadding   = 8.0
	fpCardGapX  = 8.0
	fpCardGapY  = 8.0
	fpHeaderH   = 26.0
	fpCardH     = 80.0
	fpColumns   = 2
)

func (this *FormPresetsPanel) cardWidth() float64 {
	w, _ := this.Size()
	avail := w - fpPadding*2 - fpCardGapX*float64(fpColumns-1)
	if avail < 40 {
		avail = 40
	}
	return avail / float64(fpColumns)
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

func (this *FormPresetsPanel) Draw(g paint.Painter) {
	t := gui.Theme()
	w, h := this.Size()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header
	g.SetBrush1(paint.Color{235, 238, 245, 255})
	g.Rectangle(0, 0, w, fpHeaderH)
	g.Fill()
	g.SetPen1(paint.Color{200, 200, 210, 255}, 1)
	g.MoveTo(0, fpHeaderH)
	g.LineTo(w, fpHeaderH)
	g.Stroke()

	titleFont := paint.NewFont(t.Font.Family(), 12, true, false)
	g.SetFont(titleFont)
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, fpHeaderH-7, "Form Size Presets")

	// Cards
	nameFont := paint.NewFont(t.Font.Family(), 11, true, false)
	dimFont := paint.NewFont(t.Font.Family(), 10, false, false)
	cardW := this.cardWidth()

	for i, preset := range this.presets {
		col := i % fpColumns
		row := i / fpColumns

		cx := fpPadding + float64(col)*(cardW+fpCardGapX)
		cy := fpHeaderH + fpPadding + float64(row)*(fpCardH+fpCardGapY) - this.scrollY

		if cy+fpCardH < fpHeaderH || cy > h {
			continue
		}

		// Card background
		isHover := i == this.hoverIdx
		if isHover {
			g.SetBrush1(paint.Color{220, 230, 245, 255})
		} else {
			g.SetBrush1(t.FormColor)
		}
		g.Rectangle(cx, cy, cardW, fpCardH)
		g.FillPreserve()
		if isHover {
			g.SetPen1(t.HighLightColor, 1.5)
		} else {
			g.SetPen1(t.BorderColor, 1)
		}
		g.Stroke()

		// Device outline (simplified representation)
		this.drawDeviceIcon(g, preset.Icon, cx+cardW/2, cy+22, cardW*0.35, 28)

		// Name
		g.SetFont(nameFont)
		g.SetBrush1(t.TextColor)
		ext := nameFont.TextExtents(preset.Name)
		nx := cx + (cardW-ext.Width)/2
		g.DrawText1(nx, cy+fpCardH-22, preset.Name)

		// Dimensions
		if preset.Width > 0 && preset.Height > 0 {
			g.SetFont(dimFont)
			g.SetBrush1(paint.Color{130, 130, 140, 255})
			dimLabel := fmt.Sprintf("%.0f x %.0f mm", preset.Width, preset.Height)
			dext := dimFont.TextExtents(dimLabel)
			dx := cx + (cardW-dext.Width)/2
			g.DrawText1(dx, cy+fpCardH-8, dimLabel)
		} else {
			g.SetFont(dimFont)
			g.SetBrush1(paint.Color{130, 130, 140, 255})
			cLabel := "自定义尺寸"
			cext := dimFont.TextExtents(cLabel)
			dx := cx + (cardW-cext.Width)/2
			g.DrawText1(dx, cy+fpCardH-8, cLabel)
		}
	}
}

// drawDeviceIcon draws a simplified device outline.
func (this *FormPresetsPanel) drawDeviceIcon(g paint.Painter, icon string, cx, cy, maxW, maxH float64) {
	t := gui.Theme()

	switch icon {
	case "desktop":
		// Monitor shape
		mw := maxW * 0.9
		mh := maxH * 0.6
		mx := cx - mw/2
		my := cy - mh/2 - 2
		g.Rectangle(mx, my, mw, mh)
		g.SetBrush1(paint.Color{180, 200, 220, 255})
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()
		// Stand
		g.MoveTo(cx-4, my+mh)
		g.LineTo(cx+4, my+mh)
		g.LineTo(cx+6, my+mh+5)
		g.LineTo(cx-6, my+mh+5)
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()

	case "laptop":
		mw := maxW * 0.8
		mh := maxH * 0.5
		mx := cx - mw/2
		my := cy - mh/2 - 2
		g.Rectangle(mx, my, mw, mh)
		g.SetBrush1(paint.Color{180, 200, 220, 255})
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()
		// Base
		bw := mw + 6
		g.MoveTo(cx-bw/2, my+mh)
		g.LineTo(cx+bw/2, my+mh)
		g.LineTo(cx+bw/2-2, my+mh+4)
		g.LineTo(cx-bw/2+2, my+mh+4)
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()

	case "tablet":
		tw := maxW * 0.5
		th := maxH * 0.75
		tx := cx - tw/2
		ty := cy - th/2
		g.Rectangle(tx, ty, tw, th)
		g.SetBrush1(paint.Color{180, 200, 220, 255})
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()
		// Home button circle
		g.SetPen1(t.BorderColor, 1)
		g.MoveTo(cx+2, ty+th-3)
		g.LineTo(cx-2, ty+th-3)
		g.Stroke()

	case "phone":
		pw := maxW * 0.35
		ph := maxH * 0.7
		px := cx - pw/2
		py := cy - ph/2
		g.Rectangle(px, py, pw, ph)
		g.SetBrush1(paint.Color{180, 200, 220, 255})
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()
		// Notch
		g.MoveTo(cx-3, py)
		g.LineTo(cx+3, py)
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()

	case "custom":
		// Dotted rectangle with "+" symbol
		g.SetPen1(paint.Color{150, 150, 160, 200}, 1)
		size := math.Min(maxW, maxH) * 0.5
		rx := cx - size/2
		ry := cy - size/2
		g.Rectangle(rx, ry, size, size)
		g.Stroke()
		// Plus sign
		g.MoveTo(cx-4, cy)
		g.LineTo(cx+4, cy)
		g.Stroke()
		g.MoveTo(cx, cy-4)
		g.LineTo(cx, cy+4)
		g.Stroke()
	}
}

// ---------------------------------------------------------------------------
// Interaction
// ---------------------------------------------------------------------------

func (this *FormPresetsPanel) hitIndex(x, y float64) int {
	if y < fpHeaderH {
		return -1
	}
	cardW := this.cardWidth()
	for i := range this.presets {
		col := i % fpColumns
		row := i / fpColumns
		cx := fpPadding + float64(col)*(cardW+fpCardGapX)
		cy := fpHeaderH + fpPadding + float64(row)*(fpCardH+fpCardGapY) - this.scrollY
		if x >= cx && x <= cx+cardW && y >= cy && y <= cy+fpCardH {
			return i
		}
	}
	return -1
}

func (this *FormPresetsPanel) OnMouseMove(x, y float64) {
	idx := this.hitIndex(x, y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

func (this *FormPresetsPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

func (this *FormPresetsPanel) OnLeftDown(x, y float64) {
	idx := this.hitIndex(x, y)
	if idx < 0 || idx >= len(this.presets) {
		return
	}
	preset := this.presets[idx]

	if preset.Icon == "custom" {
		this.applyCustomSize()
		return
	}
	this.applyPreset(preset)
}

func (this *FormPresetsPanel) applyPreset(p FormPreset) {
	if this.scene == nil {
		return
	}
	this.scene.SetSize(p.Width, p.Height)
	this.Self().Update()
}

func (this *FormPresetsPanel) applyCustomSize() {
	wStr, ok := gui.ShowInputBox(this, nil, "自定义尺寸", "宽度 (mm):", "120")
	if !ok || wStr == "" {
		return
	}
	hStr, ok := gui.ShowInputBox(this, nil, "自定义尺寸", "高度 (mm):", "80")
	if !ok || hStr == "" {
		return
	}

	var w, h float64
	if _, err := fmt.Sscanf(wStr, "%f", &w); err != nil || w <= 0 {
		return
	}
	if _, err := fmt.Sscanf(hStr, "%f", &h); err != nil || h <= 0 {
		return
	}
	if this.scene != nil {
		this.scene.SetSize(w, h)
		this.Self().Update()
	}
}

func (this *FormPresetsPanel) OnMouseWheel(x, y, delta float64) {
	this.scrollY -= delta * 20
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	maxScroll := this.contentHeight() - this.Height() + fpHeaderH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

func (this *FormPresetsPanel) contentHeight() float64 {
	rows := (len(this.presets) + fpColumns - 1) / fpColumns
	return fpHeaderH + fpPadding*2 + float64(rows)*(fpCardH+fpCardGapY)
}

func (this *FormPresetsPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 280, Height: 400}
}
