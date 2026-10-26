package ged

import (
	"math"
	"runtime"
	"runtime/debug"
	"strings"
	"unicode"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// Kept for the existing welcome page; these are SDK metadata, not an IDE
// version or an unverified copyright/feature-count claim.
const aboutAppVersion = "v" + core.Version
const aboutCopyright = "Silk SDK"

// AboutInfo identifies the host application separately from the Silk SDK.
// An empty Name defaults to Silk Designer; an unknown Version is omitted.
type AboutInfo struct {
	Name    string
	Version string
	Logo    paint.Icon // Optional host identity; nil retains the SDK default.
}

// ShowAboutDialog keeps the standalone designer entry point compatible.
func ShowAboutDialog(parent gui.IWidget) {
	ShowAboutDialogWithInfo(parent, AboutInfo{})
}

// ShowAboutDialogWithInfo lets an IDE or SDK host supply its own identity.
func ShowAboutDialogWithInfo(parent gui.IWidget, info AboutInfo) {
	if parent == nil {
		parent = gui.DefaultFrame()
	}
	showAboutDialog(parent, info, (*gui.Dialog).ShowModal)
}

// Keep native presentation outside construction so layout, result handling and
// focus restoration are testable without opening a desktop window.
func showAboutDialog(parent gui.IWidget, info AboutInfo, show func(*gui.Dialog) gui.DialogResult) gui.DialogResult {
	restore := aboutFocusedWidget(parent)
	dlg := newAboutDialog(parent, info)
	defer func() {
		if aboutFocusedWidget(dlg) != nil && restore != nil && restore.IsAllAncentorsVisible() && restore.IsEnabled() && aboutContainsWidget(parent, restore) {
			restore.SetFocus()
		}
	}()
	// Let ShowModal capture the current owner focus before activating its
	// initial button. The caller may be a hidden designer canvas while the
	// active editor lives in a sibling panel of the same native owner.
	return show(dlg)
}

func newAboutDialog(parent gui.IWidget, info AboutInfo) *gui.Dialog {
	info.Name = strings.TrimSpace(info.Name)
	if info.Name == "" {
		info.Name = "Silk Designer"
	}
	dlg := gui.NewDialog(i18n.T("About")+" "+info.Name, parent)
	if info.Logo != nil {
		dlg.SetIcon(info.Logo)
	}
	body := &aboutContent{info: info}
	body.Init(body)
	body.SetFocusPolicy(gui.NoFocus)
	dlg.SetContent(body)
	close := dlg.AddButton(i18n.T("Close"), gui.DialogOK)
	dlg.SetDefaultButton(close)
	hints := dlg.SizeHints()
	dlg.SetSize(hints.Width, hints.Height)
	return dlg
}

func aboutFocusedWidget(root gui.IWidget) gui.IWidget {
	if root == nil {
		return nil
	}
	if root.HasFocus() {
		return root
	}
	for _, child := range root.Children() {
		if found := aboutFocusedWidget(child); found != nil {
			return found
		}
	}
	return nil
}

func aboutContainsWidget(root, widget gui.IWidget) bool {
	for p := widget; p != nil; p = p.Parent() {
		if p == root {
			return true
		}
	}
	return false
}

func aboutGoVersion(version string) string {
	return "Go " + strings.TrimPrefix(version, "go")
}

func aboutBuildDescription(info *debug.BuildInfo, ok bool) string {
	if !ok || info == nil {
		return i18n.T("Build information unavailable")
	}
	build := info.Main.Version
	if build == "" || build == "(devel)" {
		build = i18n.T("Development build")
	}
	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
			if len(revision) > 12 {
				revision = revision[:12]
			}
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision != "" {
		build += " · " + revision
	}
	if modified {
		build += " · " + i18n.T("Working tree modified")
	}
	return build
}

// The current native window implementations use Cairo on all platforms.
// Only the GLFW implementation presents that buffer through OpenGL; Windows
// uses the Win32 Cairo surface. Do not advertise OpenGL on the latter.
func aboutRenderer(goos string) string {
	switch goos {
	case "windows":
		return "Cairo · Win32"
	case "darwin", "linux":
		return "Cairo · OpenGL (GLFW)"
	default:
		return "Cairo"
	}
}

type aboutDetail struct{ label, value string }
type aboutTextLine struct {
	text   string
	x, y   float64
	width  float64
	height float64
	font   paint.Font
	muted  bool
}

// Label.SetWrap does not currently measure/draw wrapped paragraphs. This
// small, dialog-local reader measures each actual line, including CJK and
// enlarged theme fonts, and scrolls if a small window cannot contain it.
type aboutContent struct {
	gui.ScrollArea
	info       AboutInfo
	inLayout   bool
	layoutFont string
}

func (c *aboutContent) details() []aboutDetail {
	var rows []aboutDetail
	if version := strings.TrimSpace(c.info.Version); version != "" {
		rows = append(rows, aboutDetail{i18n.T("App version"), "v" + strings.TrimPrefix(version, "v")})
	}
	buildInfo, ok := debug.ReadBuildInfo()
	return append(rows,
		aboutDetail{i18n.T("SDK version"), "v" + core.Version},
		aboutDetail{i18n.T("Build"), aboutBuildDescription(buildInfo, ok)},
		aboutDetail{i18n.T("Runtime"), aboutGoVersion(runtime.Version())},
		aboutDetail{i18n.T("Platform"), runtime.GOOS + "/" + runtime.GOARCH},
		aboutDetail{i18n.T("Rendering"), aboutRenderer(runtime.GOOS)},
	)
}

func (c *aboutContent) textLayout(width float64) ([]aboutTextLine, float64) {
	if width <= 0 {
		return nil, 0
	}
	body := gui.Theme().Font.Dup()
	title := body.Dup()
	title.SetSize(body.Size() + 8)
	title.SetBold(true)
	var lines []aboutTextLine
	y := 2.0
	add := func(text string, font paint.Font, x, top, width float64, muted bool) float64 {
		height := aboutLineHeight(font)
		for _, line := range aboutWrapText(text, font, width) {
			lines = append(lines, aboutTextLine{line, x, top, width, height, font, muted})
			top += height
		}
		return top
	}
	titleX := 0.0
	if c.info.Logo != nil && !c.info.Logo.IsAir() && width >= 120 {
		titleX = 64
	}
	y = add(c.info.Name, title, titleX, y, width-titleX, false)
	if titleX > 0 {
		y = math.Max(y, 50)
	}
	y += 8
	y = add(i18n.T("A visual designer for Go interfaces."), body, 0, y, width, false)
	y = add(i18n.T("Drag, arrange and generate Go."), body, 0, y, width, true) + 20
	rows := c.details()
	keyWidth := 0.0
	for _, row := range rows {
		keyWidth = math.Max(keyWidth, body.TextExtents(row.label).Width)
	}
	keyWidth = math.Min(math.Ceil(keyWidth)+2, width*0.42)
	valueX := keyWidth + 20
	for _, row := range rows {
		bottomKey := add(row.label, body, 0, y, keyWidth, true)
		bottomValue := add(row.value, body, valueX, y, math.Max(1, width-valueX), false)
		y = math.Max(bottomKey, bottomValue) + 8
	}
	return lines, y + 4
}

func aboutLineHeight(font paint.Font) float64 {
	fe := font.FontExtents()
	return math.Ceil(math.Max(fe.Height, fe.Ascent+fe.Descent)) + 4
}

// Wrap by measured ink width, preferring word boundaries but splitting long
// tokens/CJK by rune. Explicit line breaks never reach Cairo as a square glyph.
func aboutWrapText(text string, font paint.Font, width float64) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		rest := []rune(paragraph)
		if len(rest) == 0 {
			lines = append(lines, "")
		}
		for len(rest) > 0 {
			end, lastSpace := 0, -1
			for end < len(rest) {
				if font.TextExtents(string(rest[:end+1])).Width > width && end > 0 {
					break
				}
				if unicode.IsSpace(rest[end]) {
					lastSpace = end
				}
				end++
			}
			if end < len(rest) && lastSpace > 0 {
				end = lastSpace
			}
			lines = append(lines, strings.TrimRightFunc(string(rest[:end]), unicode.IsSpace))
			rest = []rune(strings.TrimLeftFunc(string(rest[end:]), unicode.IsSpace))
		}
	}
	return lines
}

func (c *aboutContent) SizeHints() gui.SizeHints {
	_, height := c.textLayout(480)
	return gui.SizeHints{Width: 480, Height: height, Policy: gui.GrowHorizontal | gui.GrowVertical}
}

func (c *aboutContent) Layout() {
	if c.inLayout {
		return
	}
	c.inLayout = true
	defer func() { c.inLayout = false }()
	c.layoutFont = gui.Theme().Font.String()
	w, h := c.Size()
	bar := c.VertScrollBar()
	_, height := c.textLayout(math.Max(0, w-4))
	bar.SetVisible(h > 0 && height > h && w > gui.Theme().ScrollWidth)
	c.ScrollArea.Layout()
	viewportW, viewportH := c.ViewportSizePx()
	_, height = c.textLayout(math.Max(0, viewportW-4))
	bar.SetRange(0, math.Max(0, height-viewportH))
	bar.SetDelta(aboutLineHeight(gui.Theme().Font), math.Max(0, viewportH))
}

func (c *aboutContent) OnMouseWheel(_, _, z float64) {
	c.SetScrollY(c.ScrollY() - z*3*aboutLineHeight(gui.Theme().Font))
}

func (c *aboutContent) Draw(g paint.Painter) {
	// ScrollArea.Layout requests a repaint. Do not run it on every Draw or
	// a stationary About dialog continually repaints itself.
	if c.layoutFont != gui.Theme().Font.String() {
		c.Layout()
	}
	w, h := c.ViewportSizePx()
	if w <= 4 || h <= 0 {
		return
	}
	state := g.Save()
	defer g.RestoreTo(state)
	g.Rectangle(0, 0, w, h)
	g.Clip()
	if c.info.Logo != nil && !c.info.Logo.IsAir() && w-4 >= 120 {
		g.DrawIcon1(c.info.Logo, 0, 2-c.ScrollY(), 48, false)
	}
	lines, _ := c.textLayout(w - 4)
	for _, line := range lines {
		y := line.y - c.ScrollY()
		if y+line.height <= 0 || y >= h {
			continue
		}
		color := gui.Theme().TextColor
		if line.muted {
			color = gui.Theme().TabTextColor
		}
		g.SetFont(line.font)
		g.SetBrush1(color)
		ext := line.font.TextExtents(line.text)
		g.DrawText1(line.x-ext.XBearing, y+line.font.FontExtents().Ascent+2, line.text)
	}
}
