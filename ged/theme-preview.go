package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("ged.ThemePreviewPanel", gui.TypeOf(ThemePreviewPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.ThemePreviewPanel",
		Name: "主题",
		Icon: "design",
		Desc: "主题预览与切换",
	})
}

// themeEntry defines a selectable theme option.
type themeEntry struct {
	name    string
	mode    gui.ThemeMode
	primary paint.Color // highlight/accent color
	bg      paint.Color // background preview color
}

// ThemePreviewPanel displays color swatches for available themes.
// Clicking a swatch applies the theme to the entire application.
type ThemePreviewPanel struct {
	gui.Widget
	themes    []themeEntry
	hoverIdx  int
	activeIdx int
	cbChange  func(int)
}

func NewThemePreviewPanel() *ThemePreviewPanel {
	p := new(ThemePreviewPanel)
	p.Init(p)
	return p
}

func (this *ThemePreviewPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.hoverIdx = -1
	this.activeIdx = 0

	this.themes = []themeEntry{
		{
			name:    "Light",
			mode:    gui.ThemeLight,
			primary: paint.Color{127, 192, 255, 255},
			bg:      paint.Color{241, 241, 241, 255},
		},
		{
			name:    "Dark",
			mode:    gui.ThemeDark,
			primary: paint.Color{0, 120, 215, 255},
			bg:      paint.Color{45, 45, 48, 255},
		},
		{
			name:    "Blue Accent",
			mode:    gui.ThemeLight,
			primary: paint.Color{30, 120, 230, 255},
			bg:      paint.Color{241, 241, 241, 255},
		},
		{
			name:    "Green Accent",
			mode:    gui.ThemeLight,
			primary: paint.Color{60, 180, 80, 255},
			bg:      paint.Color{241, 241, 241, 255},
		},
	}

	// Determine active theme from current mode
	if gui.CurrentThemeMode() == gui.ThemeDark {
		this.activeIdx = 1
	}
}

// SetChangeCallback registers a function called when the user switches themes.
func (this *ThemePreviewPanel) SetChangeCallback(cb func(int)) {
	this.cbChange = cb
}

func (this *ThemePreviewPanel) Draw(g paint.Painter) {
	t := gui.Theme()
	w, _ := this.Size()

	// Background
	g.Rectangle(0, 0, w, this.Height())
	g.SetBrush1(t.FormColor)
	g.Fill()

	// Title
	g.SetFont(paint.NewFont(t.Font.Family(), 12, true, false))
	g.SetBrush1(t.TextColor)
	g.Translate(8, 18)
	g.DrawText("主题切换")
	g.Translate(-8, -18)

	// Draw swatches in a row
	const (
		swatchW  = 30.0
		swatchH  = 20.0
		spacing  = 12.0
		topY     = 32.0
		leftX    = 8.0
		labelGap = 3.0
	)

	labelFont := paint.NewFont(t.Font.Family(), 10, false, false)

	for i, entry := range this.themes {
		x := leftX + float64(i)*(swatchW+spacing)
		y := topY

		// Draw swatch background
		g.Rectangle(x, y, swatchW, swatchH)
		g.SetBrush1(entry.bg)
		g.Fill()

		// Draw accent color bar at top of swatch
		g.Rectangle(x, y, swatchW, 6)
		g.SetBrush1(entry.primary)
		g.Fill()

		// Border
		if i == this.activeIdx {
			// Active: thick blue border
			g.Rectangle(x-1, y-1, swatchW+2, swatchH+2)
			g.SetPen1(paint.Color{30, 120, 230, 255}, 2)
			g.Stroke()
		} else if i == this.hoverIdx {
			// Hover: gray border
			g.Rectangle(x-1, y-1, swatchW+2, swatchH+2)
			g.SetPen1(paint.Color{160, 160, 160, 255}, 1)
			g.Stroke()
		} else {
			g.Rectangle(x, y, swatchW, swatchH)
			g.SetPen1(t.BorderColor, 1)
			g.Stroke()
		}

		// Label below swatch
		g.SetFont(labelFont)
		g.SetBrush1(t.TextColor)
		ext := labelFont.TextExtents(entry.name)
		lx := x + (swatchW-ext.Width)*0.5
		ly := y + swatchH + labelGap + 10
		lx = math.Max(lx, x)
		g.Translate(lx, ly)
		g.DrawText(entry.name)
		g.Translate(-lx, -ly)
	}
}

func (this *ThemePreviewPanel) OnMouseMove(x, y float64) {
	idx := this.hitTest(x, y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

func (this *ThemePreviewPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

func (this *ThemePreviewPanel) OnMouseDown(x, y float64, button int) {
	idx := this.hitTest(x, y)
	if idx < 0 || idx >= len(this.themes) {
		return
	}
	if idx == this.activeIdx {
		return
	}
	this.activeIdx = idx
	entry := this.themes[idx]

	// Apply theme mode
	gui.SetThemeMode(entry.mode)

	// Apply accent color override
	gui.Theme().HighLightColor = entry.primary

	// Refresh all windows
	for _, win := range gui.AllWindows() {
		win.Update()
	}

	if this.cbChange != nil {
		this.cbChange(idx)
	}
	this.Self().Update()
}

func (this *ThemePreviewPanel) hitTest(x, y float64) int {
	const (
		swatchW = 30.0
		swatchH = 20.0
		spacing = 12.0
		topY    = 32.0
		leftX   = 8.0
	)
	for i := range this.themes {
		sx := leftX + float64(i)*(swatchW+spacing)
		sy := topY
		if x >= sx && x <= sx+swatchW && y >= sy && y <= sy+swatchH {
			return i
		}
	}
	return -1
}

func (this *ThemePreviewPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 200, Height: 80}
}
