package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"path/filepath"
	"strings"
)

func init() {
	core.RegisterFactory("ged.WelcomeScreen", gui.TypeOf(WelcomeScreen{}))
}

// WelcomeScreen is a start page shown when the designer opens, similar to
// Qt Creator's welcome screen. It displays the application title, version,
// a list of recently opened projects, action buttons, and keyboard tips.
// The visual design is theme-aware (works in both light and dark modes).
type WelcomeScreen struct {
	gui.Widget
	recentFiles  []string
	hoverIdx     int
	hoverBtn     int // -1: none, 0: new, 1: open
	cbNewProject func()
	cbOpenFile   func()
	cbOpenRecent func(string)

	// Button hit regions (computed during Draw)
	btnNewRect  [4]float64 // x, y, w, h
	btnOpenRect [4]float64
	recentRects [][4]float64
}

// NewWelcomeScreen creates a new welcome screen widget.
func NewWelcomeScreen() *WelcomeScreen {
	p := new(WelcomeScreen)
	p.Init(p)
	p.hoverIdx = -1
	p.hoverBtn = -1
	return p
}

// SetRecentFiles sets the list of recently opened project file paths.
func (this *WelcomeScreen) SetRecentFiles(files []string) {
	this.recentFiles = files
	this.Self().Update()
}

// SetNewProjectCallback sets the callback for the "New Project" button.
func (this *WelcomeScreen) SetNewProjectCallback(cb func()) {
	this.cbNewProject = cb
}

// SetOpenFileCallback sets the callback for the "Open File" button.
func (this *WelcomeScreen) SetOpenFileCallback(cb func()) {
	this.cbOpenFile = cb
}

// SetOpenRecentCallback sets the callback for clicking a recent project.
func (this *WelcomeScreen) SetOpenRecentCallback(cb func(string)) {
	this.cbOpenRecent = cb
}

func (this *WelcomeScreen) Title() string     { return "Welcome" }
func (this *WelcomeScreen) SetTitle(s string) {}

// SizeHints returns the size hints for the welcome screen.
func (this *WelcomeScreen) SizeHints() gui.SizeHints {
	return gui.SizeHints{
		Width:  600,
		Height: 400,
		Policy: gui.GrowHorizontal | gui.GrowVertical | gui.ExpandHorizontal | gui.ExpandVertical,
	}
}

// welcomePalette computes theme-aware colors used across the welcome screen.
type welcomePalette struct {
	background paint.Color
	surface    paint.Color
	border     paint.Color
	title      paint.Color
	text       paint.Color
	muted      paint.Color
	accent     paint.Color
	accentHi   paint.Color
	hoverBG    paint.Color
}

func currentWelcomePalette() welcomePalette {
	t := gui.Theme()
	dark := gui.CurrentThemeMode() == gui.ThemeDark

	// surface uses the raised FormLightColor (not ViewBGColor) so the card reads
	// as elevated above the page: in dark mode ViewBGColor == FormColor (both
	// zinc-900) which would make the card invisible, whereas FormLightColor is
	// zinc-800 (raised) in dark and white in light — a distinct card in both.
	p := welcomePalette{
		background: t.FormColor,
		surface:    t.FormLightColor,
		border:     t.BorderColor,
		title:      t.TextColor,
		text:       t.TextColor,
		accent:     t.HighLightColor,
	}
	if dark {
		p.muted = paint.Color{R: 156, G: 163, B: 175, A: 255}    // gray-400
		p.accentHi = paint.Color{R: 147, G: 197, B: 253, A: 255} // blue-300
		p.hoverBG = paint.Color{R: 63, G: 63, B: 70, A: 255}     // zinc-700
	} else {
		p.muted = paint.Color{R: 107, G: 114, B: 128, A: 255}   // gray-500
		p.accentHi = paint.Color{R: 29, G: 78, B: 216, A: 255}  // blue-700
		p.hoverBG = paint.Color{R: 219, G: 234, B: 254, A: 255} // blue-100
	}
	return p
}

// Draw renders the welcome screen.
func (this *WelcomeScreen) Draw(g paint.Painter) {
	w, h := this.Self().Size()
	pal := currentWelcomePalette()

	// Full background
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(pal.background)
	g.Fill()

	// Centered content area
	contentW := 720.0
	contentH := 460.0
	if contentW > w-40 {
		contentW = w - 40
	}
	if contentH > h-40 {
		contentH = h - 40
	}
	cx := (w - contentW) / 2
	cy := (h - contentH) / 2

	// Left panel: Logo/Title/Buttons/Tips
	leftW := contentW * 0.44
	leftX := cx

	fontFamily := gui.Theme().Font.Family()

	// Title: "Silk Designer" (28pt bold)
	titleFont := paint.NewFont(fontFamily, 28, true, false)
	g.SetFont(titleFont)
	g.SetBrush1(pal.title)
	titleY := cy + 42
	g.DrawText1(leftX, titleY, "Silk Designer")

	// Subtitle (accent color)
	subFont := paint.NewFont(fontFamily, 13, true, false)
	g.SetFont(subFont)
	g.SetBrush1(pal.accent)
	g.DrawText1(leftX, titleY+22, aboutAppVersion+"  \u00b7  Go UI Framework")

	// Short description (muted)
	descFont := paint.NewFont(fontFamily, 11, false, false)
	g.SetFont(descFont)
	g.SetBrush1(pal.muted)
	g.DrawText1(leftX, titleY+44, "Cross-platform GUI designer with 62+ widgets,")
	g.DrawText1(leftX, titleY+60, "visual designer, and integrated IDE.")

	// Accent underline
	lineY := titleY + 78
	g.SetPen1(pal.accent, 2)
	g.MoveTo(leftX, lineY)
	g.LineTo(leftX+64, lineY)
	g.Stroke()

	// Action buttons
	btnY := lineY + 20
	btnW := 140.0
	btnH := 34.0
	btnSpacing := 12.0

	// Primary "New Project" button (filled blue)
	this.btnNewRect = [4]float64{leftX, btnY, btnW, btnH}
	drawWelcomeButton(g, leftX, btnY, btnW, btnH,
		"\u65b0\u5efa\u9879\u76ee", /* 新建项目 */
		pal.accent, paint.Color{255, 255, 255, 255}, pal.accent,
		this.hoverBtn == 0, false, &pal)

	// Outlined "Open File" button
	openX := leftX + btnW + btnSpacing
	this.btnOpenRect = [4]float64{openX, btnY, btnW, btnH}
	drawWelcomeButton(g, openX, btnY, btnW, btnH,
		"\u6253\u5f00\u6587\u4ef6", /* 打开文件 */
		pal.surface, pal.accent, pal.accent,
		this.hoverBtn == 1, true, &pal)

	// Tips section (bottom of left panel)
	tipsY := btnY + btnH + 28
	tipsTitleFont := paint.NewFont(fontFamily, 11, true, false)
	g.SetFont(tipsTitleFont)
	g.SetBrush1(pal.title)
	g.DrawText1(leftX, tipsY, "\u5feb\u6377\u952e") /* 快捷键 */
	tipsFont := paint.NewFont(fontFamily, 10, false, false)
	g.SetFont(tipsFont)
	g.SetBrush1(pal.muted)
	shortcuts := []string{
		"Ctrl+N  \u00b7  \u65b0\u5efa", // 新建
		"Ctrl+S  \u00b7  \u4fdd\u5b58", // 保存
		"Ctrl+R  \u00b7  \u9884\u89c8", // 预览
		"F5     \u00b7  \u8fd0\u884c",  // 运行
	}
	for i, s := range shortcuts {
		g.DrawText1(leftX, tipsY+18+float64(i)*15, s)
	}

	// Right panel: Recent Projects (card-style surface)
	rightX := cx + leftW + 20
	rightW := contentW - leftW - 20
	rightY := cy + 30
	rightH := contentH - 50

	// Card background: a rounded, elevated surface floating on the page.
	const cardRadius = 8.0
	// Soft drop shadow under the card for elevation (alpha-only, theme-neutral).
	paint.DrawShadowRect(g, rightX, rightY, rightW, rightH, cardRadius, 6,
		paint.Color{0, 0, 0, 60})
	welcomeRoundRect(g, rightX, rightY, rightW, rightH, cardRadius)
	g.SetBrush1(pal.surface)
	g.Fill()
	welcomeRoundRect(g, rightX+0.5, rightY+0.5, rightW-1, rightH-1, cardRadius)
	g.SetPen1(pal.border, 1)
	g.Stroke()

	// Section title
	sectionFont := paint.NewFont(fontFamily, 15, true, false)
	g.SetFont(sectionFont)
	g.SetBrush1(pal.title)
	g.DrawText1(rightX+16, rightY+26, "\u6700\u8fd1\u9879\u76ee") /* 最近项目 */

	// Accent underline for section title
	g.SetPen1(pal.accent, 2)
	g.MoveTo(rightX+16, rightY+34)
	g.LineTo(rightX+16+48, rightY+34)
	g.Stroke()

	// Recent files list
	itemH := 38.0
	listX := rightX + 8
	listY := rightY + 48
	listW := rightW - 16

	this.recentRects = this.recentRects[:0]

	if len(this.recentFiles) == 0 {
		emptyFont := paint.NewFont(fontFamily, 12, false, true)
		g.SetFont(emptyFont)
		g.SetBrush1(pal.muted)
		g.DrawText1(rightX+20, rightY+80, "\u6682\u65e0\u6700\u8fd1\u9879\u76ee") /* 暂无最近项目 */
		emptyHint := paint.NewFont(fontFamily, 10, false, false)
		g.SetFont(emptyHint)
		g.DrawText1(rightX+20, rightY+100,
			"\u70b9\u51fb\u201c\u65b0\u5efa\u9879\u76ee\u201d\u5f00\u59cb\u8bbe\u8ba1")
		/* 点击"新建项目"开始设计 */
	} else {
		maxItems := 10
		if len(this.recentFiles) < maxItems {
			maxItems = len(this.recentFiles)
		}
		for i := 0; i < maxItems; i++ {
			iy := listY + float64(i)*itemH
			if iy+itemH > rightY+rightH-8 {
				break
			}
			rect := [4]float64{listX, iy, listW, itemH}
			this.recentRects = append(this.recentRects, rect)

			hovered := i == this.hoverIdx

			// Hover background (rounded to match the card / buttons)
			if hovered {
				welcomeRoundRect(g, listX, iy, listW, itemH, 6)
				g.SetBrush1(pal.hoverBG)
				g.Fill()
			}

			fullPath := this.recentFiles[i]

			// File icon stripe (left edge, colored by extension)
			iconColor := fileIconColor(fullPath, &pal)
			g.Rectangle(listX+4, iy+8, 3, itemH-16)
			g.SetBrush1(iconColor)
			g.Fill()

			// File name (bold) – accent on hover
			base := filepath.Base(fullPath)
			nameFont := paint.NewFont(fontFamily, 12, true, false)
			g.SetFont(nameFont)
			if hovered {
				g.SetBrush1(pal.accentHi)
			} else {
				g.SetBrush1(pal.title)
			}
			g.DrawText1(listX+16, iy+16, base)

			// Directory (smaller, muted)
			dirFont := paint.NewFont(fontFamily, 10, false, false)
			g.SetFont(dirFont)
			g.SetBrush1(pal.muted)
			dir := filepath.Dir(fullPath)
			if len(dir) > 56 {
				dir = "..." + dir[len(dir)-53:]
			}
			g.DrawText1(listX+16, iy+30, dir)
		}
	}

	// Footer
	footerFont := paint.NewFont(fontFamily, 10, false, false)
	g.SetFont(footerFont)
	g.SetBrush1(pal.muted)
	g.DrawText1(cx, cy+contentH-6,
		"Silk UI Framework  \u00b7  "+aboutAppVersion+"  \u00b7  "+aboutCopyright)
}

// fileIconColor picks a subtle accent color for the given file extension,
// used as a left-edge stripe marker in the recent files list.
func fileIconColor(path string, pal *welcomePalette) paint.Color {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return paint.Color{R: 0, G: 173, B: 216, A: 255} // gopher cyan
	case ".silkui", ".cml", ".silk", ".form", ".xml":
		return paint.Color{R: 139, G: 92, B: 246, A: 255} // violet-500
	case ".json":
		return paint.Color{R: 245, G: 158, B: 11, A: 255} // amber-500
	case ".md":
		return paint.Color{R: 16, G: 185, B: 129, A: 255} // emerald-500
	default:
		return pal.accent
	}
}

// welcomeRoundRect emits a self-contained, closed rounded-rectangle path on g
// (no Fill/Stroke). The caller commits it with Fill() and/or Stroke(). r is the
// corner radius, clamped to half the width/height. gui.roundedRect /
// paint.drawRoundedRectPath are unexported, so the welcome screen draws its own.
func welcomeRoundRect(g paint.Painter, x, y, w, h, r float64) {
	if r <= 0 {
		g.Rectangle(x, y, w, h)
		return
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	const piHalf = 1.5707963267948966 // math.Pi/2
	g.MoveTo(x+r, y)
	g.LineTo(x+w-r, y)
	g.Arc(x+w-r, y+r, r, -piHalf, 0)
	g.LineTo(x+w, y+h-r)
	g.Arc(x+w-r, y+h-r, r, 0, piHalf)
	g.LineTo(x+r, y+h)
	g.Arc(x+r, y+h-r, r, piHalf, 2*piHalf)
	g.LineTo(x, y+r)
	g.Arc(x+r, y+r, r, 2*piHalf, 3*piHalf)
	g.LineTo(x+r, y)
}

// drawWelcomeButton draws a styled button on the welcome screen.
// If outlined is true, the button is drawn with a transparent fill and a
// colored border; otherwise it is a solid, filled button.
func drawWelcomeButton(g paint.Painter, x, y, w, h float64, text string,
	bg, textColor, borderColor paint.Color, hover bool, outlined bool, pal *welcomePalette) {

	bgDraw := bg
	textDraw := textColor
	borderDraw := borderColor

	if hover {
		if outlined {
			// Fill with accent tint, invert text color
			bgDraw = paint.Color{R: borderColor.R, G: borderColor.G, B: borderColor.B, A: 255}
			textDraw = paint.Color{255, 255, 255, 255}
		} else {
			// Slightly darker primary on hover
			bgDraw = paint.Color{R: pal.accentHi.R, G: pal.accentHi.G, B: pal.accentHi.B, A: 255}
		}
	}

	// Button background (rounded)
	const btnRadius = 6.0
	welcomeRoundRect(g, x, y, w, h, btnRadius)
	g.SetBrush1(bgDraw)
	g.Fill()

	// Border (outlined style or subtle for filled)
	if outlined || hover {
		welcomeRoundRect(g, x+0.5, y+0.5, w-1, h-1, btnRadius)
		g.SetPen1(borderDraw, 1)
		g.Stroke()
	}

	// Button text (centered)
	fontFamily := gui.Theme().Font.Family()
	btnFont := paint.NewFont(fontFamily, 12, true, false)
	g.SetFont(btnFont)
	g.SetBrush1(textDraw)
	ext := btnFont.TextExtents(text)
	tx := x + (w-ext.Width)/2 - ext.XBearing
	ty := y + (h+ext.YBearing)/2 - ext.YBearing
	g.DrawText1(tx, ty, text)
}

// OnLeftDown handles mouse clicks on the welcome screen.
func (this *WelcomeScreen) OnLeftDown(x, y float64) {
	// Check "New Project" button
	if hitRect(x, y, this.btnNewRect) {
		if this.cbNewProject != nil {
			this.cbNewProject()
		}
		return
	}

	// Check "Open File" button
	if hitRect(x, y, this.btnOpenRect) {
		if this.cbOpenFile != nil {
			this.cbOpenFile()
		}
		return
	}

	// Check recent file items
	for i, rect := range this.recentRects {
		if hitRect(x, y, rect) && i < len(this.recentFiles) {
			if this.cbOpenRecent != nil {
				this.cbOpenRecent(this.recentFiles[i])
			}
			return
		}
	}
}

// OnMouseMove tracks hover state for interactive elements.
func (this *WelcomeScreen) OnMouseMove(x, y float64) {
	oldHover := this.hoverIdx
	oldBtn := this.hoverBtn
	this.hoverIdx = -1
	this.hoverBtn = -1

	if hitRect(x, y, this.btnNewRect) {
		this.hoverBtn = 0
	} else if hitRect(x, y, this.btnOpenRect) {
		this.hoverBtn = 1
	}

	for i, rect := range this.recentRects {
		if hitRect(x, y, rect) {
			this.hoverIdx = i
			break
		}
	}

	if this.hoverIdx != oldHover || this.hoverBtn != oldBtn {
		this.Self().Update()
	}
}

// hitRect checks if point (x,y) is inside the rectangle [rx, ry, rw, rh].
func hitRect(x, y float64, rect [4]float64) bool {
	return x >= rect[0] && x <= rect[0]+rect[2] &&
		y >= rect[1] && y <= rect[1]+rect[3]
}
