package gui

import (
	"math"
	"silk/core"
	"silk/paint"
	"runtime"
)

// ThemeMode represents the current color scheme.
type ThemeMode int

const (
	ThemeLight ThemeMode = iota
	ThemeDark
)

var currentThemeMode ThemeMode = ThemeLight

// SetThemeMode switches between light and dark themes.
func SetThemeMode(mode ThemeMode) {
	currentThemeMode = mode
	defaultThemeSingleton = nil // Force re-creation
}

// CurrentThemeMode returns the active theme mode.
func CurrentThemeMode() ThemeMode { return currentThemeMode }

func defaultFontFamily() string {
	switch runtime.GOOS {
	case "darwin":
		return "PingFang SC"
	case "windows":
		return "Microsoft YaHei"
	default:
		return "Noto Sans CJK SC"
	}
}

type defaultTheme struct {
	FormColor      paint.Color
	FormLightColor paint.Color
	FormDarkColor  paint.Color
	IconSize       float64
	Font           paint.Font
	HighLightColor paint.Color
	BorderColor    paint.Color
	BorderPen      paint.Pen
	TextColor      paint.Color
	ViewBGColor    paint.Color
	Margin         float64
	Spacing        float64
	//	HAlign         HAlign
	//	VAlign         VAlign
	ScrollWidth float64
	//ButtonHeight   float64
	ItemHeight    float64
	SeparatorSize float64
	//VMenuMargin    Margin
	ButtonMargin Margin

	MenuBorderColor     paint.Color
	MenuBGColor         paint.Color
	SeperatorColor      paint.Color
	MenuTextColor       paint.Color
	MenuActiveBGColor   paint.Color
	MenuActiveTextColor paint.Color
	MenuGrayTextColor   paint.Color
	MenuSubMarkWidth    float64
	MenuItemMargin      Margin

	MenuMargin    Margin
	MenuBarMargin Margin

	ButtonActiveFace      *pixmapFace
	ButtonPushedFace      *pixmapFace
	VertScrollFace        *pixmapFace
	HorzScrollFace        *pixmapFace
	VertScrollTrack       *pixmapFace
	HorzScrollTrack       *pixmapFace
	VertScrollActiveTrack *pixmapFace
	HorzScrollActiveTrack *pixmapFace
	TabFace               *pixmapFace
	TabHoverFace          *pixmapFace

	EditPadding Padding

	TabBarHeight float64
	MinTabWidth  float64
	//TabBarMargin       Margin
	TabMargin          Margin
	TabActiveTextColor paint.Color
	TabTextColor       paint.Color
	TabCloseIcon       paint.Icon
	TabCloseSize       float64
	SplitterSize       float64

	CheckBoxSize  float64
	CheckedIcon   paint.Icon
	UncheckedIcon paint.Icon

	ExpandedIcon  paint.Icon
	CollapsedIcon paint.Icon
}

var defaultThemeSingleton *defaultTheme = nil

// GUI风格(待改进)
func Theme() *defaultTheme {
	if defaultThemeSingleton == nil {
		t := new(defaultTheme)
		// Modern light theme palette
		t.FormColor = paint.Color{245, 245, 248, 255}        // slightly blue-gray
		t.FormLightColor = paint.Color{255, 255, 255, 255}
		t.FormDarkColor = paint.Color{107, 114, 128, 255}     // gray-500
		defaultThemeSingleton = t
		t.IconSize = 16
		t.Margin = 4
		t.Spacing = 4
		t.Font = paint.NewFont(defaultFontFamily(), 14, false, false)

		t.TextColor = paint.Color{33, 37, 41, 255}            // near-black, softer than pure black
		t.HighLightColor = paint.Color{59, 130, 246, 255}     // modern blue (Tailwind blue-500)
		t.BorderColor = paint.Color{209, 213, 219, 255}       // softer border (gray-300)
		t.BorderPen = paint.NewPen(t.BorderColor, 1)
		t.ViewBGColor = paint.Color{255, 255, 255, 255}
		t.ScrollWidth = 14
		t.ItemHeight = 24
		t.SeparatorSize = 1

		// Modern menu colors
		t.MenuBorderColor = paint.Color{229, 231, 235, 255}   // subtle border (gray-200)
		t.MenuBGColor = paint.Color{255, 255, 255, 255}
		t.MenuTextColor = paint.Color{55, 65, 81, 255}         // dark gray text (gray-700)
		t.MenuActiveBGColor = paint.Color{59, 130, 246, 255}   // blue highlight
		t.MenuActiveTextColor = paint.Color{255, 255, 255, 255}
		t.MenuGrayTextColor = paint.Color{156, 163, 175, 255}  // gray-400
		t.SeperatorColor = paint.Color{243, 244, 246, 255}     // very light separator (gray-100)
		//t.MenuItemHeight = 24
		//t.MenuTextIndent = 32
		t.MenuSubMarkWidth = 8

		t.MenuMargin = Margin{4, 4, 4, 4}
		t.MenuBarMargin = Margin{4, 4, 2, 2}
		t.MenuItemMargin = Margin{8, 12, 4, 4}
		t.ButtonMargin = Margin{8, 8, 4, 4}

		// Button and scrollbar drawing is now programmatic; only tab faces still use pixmaps
		t.TabFace = newPixmapFace(core.ResourceDir() + `/theme/default/tab.png`)
		t.TabHoverFace = newPixmapFace(core.ResourceDir() + `/theme/default/tab-hover.png`)

		t.EditPadding = Padding{3, 2, 2, 2}

		t.TabBarHeight = 26
		t.MinTabWidth = 32
		//t.TabBarMargin = Margin{0, 2, 1, 2}
		t.TabMargin = Margin{2, 4, 1, 2}

		t.TabActiveTextColor = paint.Color{0, 0, 0, 255}
		t.TabTextColor = paint.Color{255, 255, 255, 255}
		t.TabCloseIcon = LoadIcon("close-btn")
		t.TabCloseSize = 12

		t.SplitterSize = 4

		t.CheckBoxSize = 14
		t.CheckedIcon = LoadIcon("checkbox-checked")
		t.UncheckedIcon = LoadIcon("checkbox-unchecked")

		t.CollapsedIcon = LoadIcon("expander-collapsed")
		t.ExpandedIcon = LoadIcon("expander-expanded")

		// Modern dark theme
		if currentThemeMode == ThemeDark {
			t.FormColor = paint.Color{24, 24, 27, 255}        // zinc-900
			t.FormLightColor = paint.Color{39, 39, 42, 255}   // zinc-800
			t.FormDarkColor = paint.Color{9, 9, 11, 255}      // zinc-950
			t.TextColor = paint.Color{228, 228, 231, 255}     // zinc-200
			t.ViewBGColor = paint.Color{24, 24, 27, 255}      // zinc-900
			t.HighLightColor = paint.Color{96, 165, 250, 255} // blue-400
			t.BorderColor = paint.Color{63, 63, 70, 255}      // zinc-700
			t.BorderPen = paint.NewPen(t.BorderColor, 1)
			t.MenuBGColor = paint.Color{39, 39, 42, 255}      // zinc-800
			t.MenuBorderColor = paint.Color{63, 63, 70, 255}  // zinc-700
			t.MenuTextColor = paint.Color{228, 228, 231, 255} // zinc-200
			t.MenuActiveBGColor = paint.Color{59, 130, 246, 255} // blue-500
			t.MenuActiveTextColor = paint.Color{255, 255, 255, 255}
			t.MenuGrayTextColor = paint.Color{113, 113, 122, 255} // zinc-500
			t.SeperatorColor = paint.Color{63, 63, 70, 255}   // zinc-700
			t.TabActiveTextColor = paint.Color{255, 255, 255, 255}
			t.TabTextColor = paint.Color{161, 161, 170, 255}  // zinc-400
		}
	}
	return defaultThemeSingleton
}

// roundedRect builds a rounded rectangle path on the painter.
// r is the corner radius; if it exceeds half the width or height it is clamped.
func roundedRect(g paint.Painter, x, y, w, h, r float64) {
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	// top-right corner
	g.Arc(x+w-r, y+r, r, -math.Pi/2, 0)
	// bottom-right corner
	g.Arc(x+w-r, y+h-r, r, 0, math.Pi/2)
	// bottom-left corner
	g.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi)
	// top-left corner
	g.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2)
	// close back to start
	g.LineTo(x+w-r, y)
}

func (t *defaultTheme) DrawScroll(g paint.Painter, scroll *ScrollBar) {
	ss := t.ScrollWidth
	w, h := scroll.Size()
	part, _ := scroll.ActivePart()

	// Colors
	trackColor := paint.Color{243, 244, 246, 255}    // gray-100
	thumbColor := paint.Color{156, 163, 175, 255}    // gray-400
	thumbHover := paint.Color{107, 114, 128, 255}    // gray-500
	arrowColor := paint.Color{107, 114, 128, 255}    // gray-500
	if currentThemeMode == ThemeDark {
		trackColor = paint.Color{39, 39, 42, 255}    // zinc-800
		thumbColor = paint.Color{82, 82, 91, 255}    // zinc-600
		thumbHover = paint.Color{113, 113, 122, 255}  // zinc-500
		arrowColor = paint.Color{161, 161, 170, 255}  // zinc-400
	}

	thumbRadius := 3.0
	inset := 2.0 // padding inside the track

	if scroll.IsVertical() {
		// Track background
		g.Rectangle(0, 0, w, h)
		g.SetBrush1(trackColor)
		g.Fill()

		if scroll.IsValid() {
			tx, ty, tw, th := scroll.TrackRect()
			// Draw rounded thumb
			tc := thumbColor
			if part == 3 {
				tc = thumbHover
			}
			roundedRect(g, tx+inset, ty+inset, tw-inset*2, th-inset*2, thumbRadius)
			g.SetBrush1(tc)
			g.Fill()
		}

		// Up arrow
		hw := w * 0.5
		b := 2.5
		a := 3.0
		g.MoveTo(hw, ss*0.5-b*0.5)
		g.LineTo(hw-a, ss*0.5+b*0.5)
		g.LineTo(hw+a, ss*0.5+b*0.5)
		g.LineTo(hw, ss*0.5-b*0.5)
		g.SetBrush1(arrowColor)
		g.Fill()

		// Down arrow
		g.MoveTo(hw, h-ss*0.5+b*0.5)
		g.LineTo(hw-a, h-ss*0.5-b*0.5)
		g.LineTo(hw+a, h-ss*0.5-b*0.5)
		g.LineTo(hw, h-ss*0.5+b*0.5)
		g.SetBrush1(arrowColor)
		g.Fill()
	} else {
		// Track background
		g.Rectangle(0, 0, w, h)
		g.SetBrush1(trackColor)
		g.Fill()

		if scroll.IsValid() {
			tx, ty, tw, th := scroll.TrackRect()
			tc := thumbColor
			if part == 3 {
				tc = thumbHover
			}
			roundedRect(g, tx+inset, ty+inset, tw-inset*2, th-inset*2, thumbRadius)
			g.SetBrush1(tc)
			g.Fill()
		}

		// Left arrow
		hh := h * 0.5
		b := 2.5
		a := 3.0
		g.MoveTo(ss*0.5-b*0.5, hh)
		g.LineTo(ss*0.5+b*0.5, hh-a)
		g.LineTo(ss*0.5+b*0.5, hh+a)
		g.LineTo(ss*0.5-b*0.5, hh)
		g.SetBrush1(arrowColor)
		g.Fill()

		// Right arrow
		g.MoveTo(w-ss*0.5+b*0.5, hh)
		g.LineTo(w-ss*0.5-b*0.5, hh-a)
		g.LineTo(w-ss*0.5-b*0.5, hh+a)
		g.LineTo(w-ss*0.5+b*0.5, hh)
		g.SetBrush1(arrowColor)
		g.Fill()
	}
}

func (t *defaultTheme) DrawTab(g paint.Painter, icon paint.Icon, text string,
	w, h float64, active, hover, closeBtn, hoverCloseBtn, downCloseBtn bool) {
	//core.Debug(w, h)
	m := t.TabMargin
	if active {
		g.Translate(0, m.T)
		t.TabFace.Draw(g, w, h-m.T-m.B)
		g.Translate(0, -m.T)
	} else if hover {
		g.Translate(0, m.T)
		t.TabHoverFace.Draw(g, w, h-m.T-m.B)
		g.Translate(0, -m.T)
	} else {
	}

	if icon != nil {
		xi := m.L
		yi := 0.5 * (h - t.IconSize)
		g.Translate(xi, yi)
		g.DrawIcon(icon, t.IconSize, !active && !hover)
		g.Translate(-xi, -yi)
	}

	if text != "" {
		if active {
			g.SetBrush1(t.TabActiveTextColor)
		} else {
			g.SetBrush1(t.TabTextColor)
		}
		g.SetFont(t.Font)
		//		text := btn.Text()
		ext := g.Font().TextExtents(text)
		xt := m.L
		if icon != nil {
			xt = m.L*2 + t.IconSize
		}
		xt -= ext.XBearing
		yt := 0.5*(h+ext.YBearing) - ext.YBearing
		g.Translate(xt, yt)
		g.DrawText(text)
		g.Translate(-xt, -yt)
	}

	if closeBtn {
		xc := w - m.R - t.TabCloseSize
		yc := m.T + (h-m.T-m.B-t.TabCloseSize)*0.5
		if downCloseBtn && hoverCloseBtn {
			//g.SetLineWidth(1)
			g.SetPen1(t.BorderColor, 1)
			g.Rectangle(xc-1, yc-1, t.TabCloseSize+2, t.TabCloseSize+2)
			g.Stroke()
		}
		g.Translate(xc, yc)
		g.DrawIcon(t.TabCloseIcon, t.TabCloseSize, !hoverCloseBtn)
		g.Translate(-xc, -yc)
	}
}

func (t *defaultTheme) DrawButton(g paint.Painter, btn *Button) {
	//	x, y := 0.0, 0.0
	w, h := btn.Size()
	//	fe := t.Font.FontExtents()
	if btn.IsInPopupMenu() {
		//ml, mr, _, _ := t.MenuItemMargin.Margin()
		m := t.MenuItemMargin
		highlight := btn.IsEnabled() && (btn.IsHover() || btn.IsSubPopupVisible())
		if highlight {
			g.SetBrush1(t.MenuActiveBGColor)
			g.Rectangle(0, 0, w, h)
			g.Fill()
		}

		icon := btn.Icon()
		if icon != nil {
			xi := m.L
			yi := 0.5 * (h - t.IconSize)
			g.Translate(xi, yi)
			g.DrawIcon(icon, t.IconSize, !btn.IsEnabled())
			g.Translate(-xi, -yi)
		}

		if highlight {
			g.SetBrush1(t.MenuActiveTextColor)
		} else if btn.IsEnabled() {
			g.SetBrush1(t.MenuTextColor)
		} else {
			g.SetBrush1(t.MenuGrayTextColor)
		}

		if btn.IsTextVisible() {
			g.SetFont(t.Font)
			text := btn.Text()
			ext := g.Font().TextExtents(text)
			xt := m.L*2 + t.IconSize - ext.XBearing
			yt := 0.5*(h+ext.YBearing) - ext.YBearing
			g.Translate(xt, yt)
			g.DrawText(text)
			g.Translate(-xt, -yt)

		}
		if btn.subPopup != nil {
			b := 3.0
			a := 1.2 * 3.0
			g.MoveTo(w-m.R, h*0.5)
			g.LineTo(w-m.R-b, h*0.5-a)
			g.LineTo(w-m.R-b, h*0.5+a)
			g.LineTo(w-m.R, h*0.5)
			g.SetBrush1(t.TextColor)
			g.Fill()
		}
	} else {
		// Modern programmatic button drawing (no pixmap faces)
		m := t.ButtonMargin
		radius := 4.0

		if !btn.IsEnabled() {
			// Disabled state: light gray fill
			roundedRect(g, 0, 0, w, h, radius)
			g.SetBrush1(paint.Color{243, 244, 246, 200})
			g.FillPreserve()
			g.SetPen1(paint.Color{209, 213, 219, 200}, 1)
			g.Stroke()
		} else if btn.IsSubPopupVisible() || btn.IsChecked() || (btn.IsPushed() && btn.IsHover()) {
			// Pressed / checked: darker blue
			roundedRect(g, 0, 0, w, h, radius)
			g.SetBrush1(paint.Color{37, 99, 235, 255}) // blue-600
			g.FillPreserve()
			g.SetPen1(paint.Color{29, 78, 216, 255}, 1) // blue-700
			g.Stroke()
		} else if btn.IsPushed() || btn.IsHover() {
			// Hover: light blue tint
			roundedRect(g, 0, 0, w, h, radius)
			g.SetBrush1(paint.Color{239, 246, 255, 255}) // blue-50
			g.FillPreserve()
			g.SetPen1(paint.Color{147, 197, 253, 255}, 1) // blue-300
			g.Stroke()
		}

		icon := btn.Icon()
		if icon != nil {
			xi := m.L
			yi := 0.5 * (h - t.IconSize)
			g.Translate(xi, yi)
			g.DrawIcon(icon, t.IconSize, !btn.IsEnabled())
			g.Translate(-xi, -yi)
		}

		if !btn.IsEnabled() {
			g.SetBrush1(t.MenuGrayTextColor)
		} else if btn.IsSubPopupVisible() || btn.IsChecked() || (btn.IsPushed() && btn.IsHover()) {
			g.SetBrush1(paint.Color{255, 255, 255, 255}) // white text on pressed
		} else {
			g.SetBrush1(t.TextColor)
		}

		if btn.IsTextVisible() {
			g.SetFont(t.Font)
			text := btn.Text()
			ext := g.Font().TextExtents(text)
			xt := m.L
			if icon != nil {
				xt = m.L*2 + t.IconSize
			}
			xt -= ext.XBearing
			yt := 0.5*(h+ext.YBearing) - ext.YBearing
			g.Translate(xt, yt)
			g.DrawText(text)
			g.Translate(-xt, -yt)
		}
	}
}

func (t *defaultTheme) DrawCheckBox(g paint.Painter, box *CheckBox) {
	_, h := box.Size()
	m := t.ButtonMargin
	//if box.IsChecked() || box.IsPushed() && box.IsHover() {
	//	t.ButtonPushedFace.Draw(g, w, h)

	//} else if (box.IsPushed() || box.IsHover()) && box.IsEnabled() {
	//	t.ButtonActiveFace.Draw(g, w, h)
	//}

	icon := box.Icon()
	if icon != nil {
		xi := m.L
		yi := 0.5 * (h - t.IconSize)
		g.Translate(xi, yi)
		g.DrawIcon(icon, t.IconSize, !box.IsEnabled())
		g.Translate(-xi, -yi)
	}

	if box.IsEnabled() {
		g.SetBrush1(t.MenuTextColor)
	} else {
		g.SetBrush1(t.MenuGrayTextColor)
	}

	if box.Text() != "" {
		g.SetFont(t.Font)
		text := box.Text()
		ext := g.Font().TextExtents(text)
		xt := m.L
		if icon != nil {
			xt = m.L*2 + t.IconSize
		}
		xt -= ext.XBearing
		yt := 0.5*(h+ext.YBearing) - ext.YBearing
		g.Translate(xt, yt)
		g.DrawText(text)
		g.Translate(-xt, -yt)

	}

}

func (t *defaultTheme) DrawCaret(g paint.Painter, x, y, w, h float64) {
	g.SetBrush1(paint.Color{255, 0, 0, 255})
	w = 2.0
	g.Rectangle(x, y, w, h)
	g.Fill()
}

func (t *defaultTheme) DrawEditFrame(c paint.Painter,
	x, y, width, height float64, focus, hover, readonly bool) {
	radius := 4.0
	roundedRect(c, x, y, width, height, radius)
	// Fill with white background
	c.SetBrush1(paint.Color{255, 255, 255, 255})
	c.FillPreserve()
	if focus && !readonly {
		c.SetPen1(t.HighLightColor, 2) // thicker blue border when focused
	} else if hover && !readonly {
		c.SetPen1(paint.Color{147, 197, 253, 255}, 1) // blue-300 on hover
	} else {
		c.SetPen1(t.BorderColor, 1)
	}
	c.Stroke()
}

func (t *defaultTheme) DrawViewFrame(c paint.Painter, x, y, width, height float64) {
	radius := 4.0
	roundedRect(c, x, y, width, height, radius)
	c.SetPen1(t.BorderColor, 1)
	c.Stroke()
}

func (t *defaultTheme) DrawSeperator(c paint.Painter, w, h float64, vertical bool) {
	x, y := 0.0, 0.0
	//	c.SetLineWidth(1)
	if !vertical {
		y0 := y + h/2
		//	y1 := y0 + 1
		x1 := x + w
		c.Line(x, y0, x1, y0)
		c.SetPen1(t.SeperatorColor, 1)
		c.Stroke()
		//		c.Line(x, y1, x1, y1)
		//	c.SetSourceColor(t.FormLightColor)
		//	c.Stroke()
	} else {
		x0 := x + w/2
		//	x1 := x0 + 1
		y1 := y + h
		c.Line(x0, y, x0, y1)
		c.SetPen1(t.SeperatorColor, 1)
		c.Stroke()
		//	c.Line(x1, y, x1, y1)
		//	c.SetSourceColor(t.FormLightColor)
		//	c.Stroke()
	}
}

func (t *defaultTheme) DrawMenu(g paint.Painter, m *Menu) {
	w, h := m.Size()
	if m.IsPopup() {
		// Modern popup menu: white background, rounded corners, subtle shadow
		radius := 6.0

		// Shadow layer (offset down-right by 2px, slightly larger)
		roundedRect(g, 2, 2, w, h, radius)
		g.SetBrush1(paint.Color{0, 0, 0, 30})
		g.Fill()

		// Main background
		roundedRect(g, 0, 0, w, h, radius)
		g.SetBrush1(t.MenuBGColor)
		g.FillPreserve()
		g.SetPen1(t.MenuBorderColor, 1)
		g.Stroke()
	} else {
		// Menu bar: flat background, bottom border only
		g.Rectangle(0, 0, w, h)
		g.SetBrush1(t.FormColor)
		g.Fill()

		// Bottom separator line
		g.Line(0, h-1, w, h-1)
		g.SetPen1(t.SeperatorColor, 1)
		g.Stroke()
	}
}

//func (t *defaultTheme) DrawToolBar(g paint.Painter, w, h float64, vertical bool) {
//	var x, y float64 = 0, 0
//	//	x1, y1, _, h1 := t.HMenuMargin.ApplyMargin(x, y, w, h)
//	g.Rectangle(x, y, w, h)
//	g.SetSourceColor(t.FormColor)
//	g.FillPreserve()
//	g.SetPen(t.BorderPen)
//	g.Stroke()
//}
/*
func DrawIconText(c paint.Painter, icon paint.Icon, grayed bool, iconSize float64, text string,
	x, y, width, height float64, ha HAlign, va VAlign, margin Margin) {
	x, y, width, height = margin.Apply(x, y, width, height)
	is := iconSize
	is1 := is + x
	if icon == nil {
		is = 0
		is1 = 0
	}
	ext := c.Font().TextExtents(text)
	ew := ext.Width + is1
	eh := ext.Height
	var x0, y0, x1, y1 float64
	switch ha {
	default:
		fallthrough
	case ALIGN_LEFT:
		x0 = x
	case ALIGN_CENTER:
		x0 = x + (width-ew)*0.5
	case ALIGN_RIGHT:
		x0 = x + width - ew
	}
	x1 = x0 + is1

	switch va {
	default:
		fallthrough
	case VALIGN_TOP:
		y0 = y
		y1 = y + eh
	case VALIGN_CENTER:
		y0 = y + (height-is)*0.5
		y1 = y + (height-eh)*0.5 + eh
	case VALIGN_BOTTOM:
		y0 = y + height - is
		y1 = y + height
	}

	if icon != nil {
		//r, g, b, a, _ := c.Source().RGBA()
		c.Translate(x0, y0)
		c.DrawIcon(icon, iconSize, grayed)
		c.Translate(-x0, -y0)
		//		c.SetSourceRGBA(r, g, b, a)
	}
	c.Translate(x1, y1)
	c.DrawText(text)
	c.Translate(-x1, -y1)
}
*/

/*
func (t *defaultTheme) ItemSizeHints(item *Item) SizeHints {
	ml, mr, mt, mb := t.ButtonMargin.Margin()
	ext := t.Font.TextExtents(item.Text())

	w := ext.Width + t.Margin*2
	if item.Icon() != nil {
		w += t.IconSize + t.Spacing
	}
	h := math.Max(t.IconSize, ext.Height) + t.Margin*2
	w += ml + mr
	h += mt + mb
	return SizeHints{W: w, H: h, Policy: GrowHorizontal}
}
*/

type pixmapFace struct {
	CC, LC, TC, RC, BC, TL, TR, BL, BR paint.Brush
	w0, h0, w1, h1, w2, h2             float64
}

func newPixmapFace(filename string) *pixmapFace {
	p := new(pixmapFace)

	src, err := paint.LoadPngFile(filename)
	if err != nil {
		core.Warn(err)
		return p
	}

	sw := src.Width()
	sh := src.Height()

	//if sw < 3 || sh < 3 {
	//	core.Warn(err)
	//	return p
	//}

	w0 := sw / 2
	w1 := 1
	w2 := w0
	x0 := 0.0
	x1 := float64(w0)
	x2 := float64(w0 + 1)

	h0 := sh / 2
	h1 := 1
	h2 := h0
	y0 := 0.0
	y1 := float64(h0)
	y2 := float64(h0 + 1)

	{
		TL := paint.NewPixmap(w0, h0)
		g := TL.NewPainter()
		//g.SetSourceSurface(src, -x0, -y0)
		g.SetOperator(paint.OpSource)
		//g.Paint()
		g.DrawPixmap2(0, 0, src, -x0, -y0)
		p.TL = paint.NewPixmapBrush(TL)
	}
	{
		TC := paint.NewPixmap(w1, h0)
		g := TC.NewPainter()
		//g.SetSourceSurface(src, -x1, -y0)
		g.SetOperator(paint.OpSource)
		//g.Paint()
		g.DrawPixmap2(0, 0, src, -x1, -y0)

		br := paint.NewPixmapBrush(TC)
		br.SetExtend(paint.ExtRepeat)
		p.TC = br
	}
	{
		TR := paint.NewPixmap(w2, h0)
		g := TR.NewPainter()
		//g.SetSourceSurface(src, -x2, -y0)
		g.SetOperator(paint.OpSource)
		//g.Paint()
		g.DrawPixmap2(0, 0, src, -x2, -y0)

		p.TR = paint.NewPixmapBrush(TR)
	}
	{
		LC := paint.NewPixmap(w0, h1)
		g := LC.NewPainter()
		//g.SetSourceSurface(src, -x0, -y1)
		g.SetOperator(paint.OpSource)
		//g.Paint()
		g.DrawPixmap2(0, 0, src, -x0, -y1)

		br := paint.NewPixmapBrush(LC)
		br.SetExtend(paint.ExtRepeat)
		p.LC = br
	}
	{
		CC := paint.NewPixmap(w1, h1)
		g := CC.NewPainter()
		//		g.SetSourceSurface(src, -x1, -y1)
		g.SetOperator(paint.OpSource)
		//g.Paint()
		g.DrawPixmap2(0, 0, src, -x1, -y1)

		br := paint.NewPixmapBrush(CC)
		br.SetExtend(paint.ExtRepeat)
		p.CC = br
	}
	{
		RC := paint.NewPixmap(w2, h1)
		g := RC.NewPainter()
		//		g.SetSourceSurface(src, -x2, -y1)
		g.SetOperator(paint.OpSource)
		//g.Paint()
		g.DrawPixmap2(0, 0, src, -x2, -y1)

		br := paint.NewPixmapBrush(RC)
		br.SetExtend(paint.ExtRepeat)
		p.RC = br
	}
	{
		BL := paint.NewPixmap(w0, h2)
		g := BL.NewPainter()
		//		g.SetSourceSurface(src, -x0, -y2)
		g.SetOperator(paint.OpSource)
		//g.Paint()
		g.DrawPixmap2(0, 0, src, -x0, -y2)

		p.BL = paint.NewPixmapBrush(BL)
	}
	{
		BC := paint.NewPixmap(w1, h2)
		g := BC.NewPainter()
		//		g.SetSourceSurface(src, -x1, -y2)
		g.SetOperator(paint.OpSource)
		//	g.Paint()
		g.DrawPixmap2(0, 0, src, -x1, -y2)

		br := paint.NewPixmapBrush(BC)
		br.SetExtend(paint.ExtRepeat)
		p.BC = br
	}
	{
		BR := paint.NewPixmap(w2, h2)
		g := BR.NewPainter()
		//		g.SetSourceSurface(src, -x2, -y2)
		g.SetOperator(paint.OpSource)
		//g.Paint()
		g.DrawPixmap2(0, 0, src, -x2, -y2)

		p.BR = paint.NewPixmapBrush(BR)
	}

	p.w0 = float64(w0)
	p.w1 = float64(w1)
	p.w2 = float64(w2)
	p.h0 = float64(h0)
	p.h1 = float64(h1)
	p.h2 = float64(h2)

	//core.Debug(p)

	return p
}

func (this *pixmapFace) Draw(g paint.Painter, w, h float64) {
	if this.TL == nil {
		g.Rectangle(0, 0, w, h)
		g.SetBrush1(paint.Color{127, 127, 127, 127})
		g.FillPreserve()
		g.SetPen1(paint.Color{127, 127, 127, 127}, 1)
		g.Stroke()
		return
	}
	g.SetBrush(this.TL)
	g.Paint()
	g.Translate(w-this.w2, 0)
	g.SetBrush(this.TR)
	g.Paint()
	g.Translate(0, h-this.h2)
	g.SetBrush(this.BR)
	g.Paint()
	g.Translate(-(w - this.w2), 0)
	g.SetBrush(this.BL)
	g.Paint()
	g.Translate(0, -(h - this.h2))

	g.Save()
	g.Rectangle(this.w0, 0, w-this.w2-this.w0, this.h0)
	g.Clip()
	g.SetBrush(this.TC)
	g.Paint()
	g.Restore()

	g.Save()
	g.Rectangle(0, this.h0, this.w0, h-this.h2-this.h0)
	g.Clip()
	g.SetBrush(this.LC)
	g.Paint()

	g.Restore()
	g.Save()
	g.Translate(this.w0, h-this.h2)
	g.Rectangle(0, 0, w-this.w2-this.w0, this.h2)
	g.Clip()
	g.SetBrush(this.BC)
	g.Paint()
	g.Restore()

	g.Save()
	g.Translate(w-this.w2, this.h0)
	g.Rectangle(0, 0, this.w0, h-this.h2-this.h0)
	g.Clip()
	g.SetBrush(this.RC)
	g.Paint()
	g.Restore()

	g.Save()
	g.Rectangle(this.w0, this.h0, w-this.w2-this.w0, h-this.h2-this.h0)
	g.Clip()
	g.SetBrush(this.CC)
	g.Paint()
	g.Restore()

}
