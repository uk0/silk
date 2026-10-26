package ged

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.WelcomeScreen", gui.TypeOf(WelcomeScreen{}))
	registerWelcomeTranslations()
}

// WelcomeShortcut is an informational host shortcut. SetShortcuts does not
// register a global binding; hosts must describe their actual commands.
type WelcomeShortcut struct{ Keys, Label string }

// WelcomeScreen is a responsive start page with real, keyboard-accessible
// Button and ListWidget controls. Narrow pages scroll instead of overlapping.
type WelcomeScreen struct {
	gui.ScrollArea
	recentFiles              []string
	cbNewProject, cbOpenFile func()
	cbOpenRecent             func(string)
	branding                 AboutInfo
	newButton, openButton    *welcomeButton
	recent                   *welcomeRecentList
	newLabel, openLabel      string
	shortcuts                []WelcomeShortcut
	page                     welcomePageLayout
	fonts                    welcomeFonts
	layouting                bool
	fontFamily, locale       string
	fontSize                 int
	// Viewport-space regions retained for geometry regression checks.
	btnNewRect, btnOpenRect [4]float64
	recentRects             [][4]float64
}

func NewWelcomeScreen() *WelcomeScreen { p := new(WelcomeScreen); p.Init(p); return p }
func (w *WelcomeScreen) Init(self gui.IWidget) {
	w.ScrollArea.Init(self)
	w.SetFocusPolicy(gui.NoFocus)
	w.newLabel, w.openLabel = "New Project", "Open File"
	action := "Ctrl"
	if runtime.GOOS == "darwin" {
		action = "Cmd"
	}
	w.shortcuts = []WelcomeShortcut{{action + "+N", "New Project"}, {action + "+O", "Open File"}, {action + "+S", "Save"}}
	w.newButton = newWelcomeButton(w, i18n.T(w.newLabel), "plus")
	w.newButton.SetParent(self)
	w.newButton.SetDefault(true)
	w.openButton = newWelcomeButton(w, i18n.T(w.openLabel), "folder")
	w.openButton.SetParent(self)
	w.newButton.Action().BindFunc0(func() {
		if w.cbNewProject != nil {
			w.cbNewProject()
		}
	})
	w.openButton.Action().BindFunc0(func() {
		if w.cbOpenFile != nil {
			w.cbOpenFile()
		}
	})
	w.recent = newWelcomeRecentList(w)
	w.recent.SetParent(self)
	w.recent.SetVisible(false)
	w.recent.SigSubmit(func(interface{}) {
		i := w.recent.ActiveIndex()
		if w.cbOpenRecent != nil && i >= 0 && i < len(w.recentFiles) {
			w.cbOpenRecent(w.recentFiles[i])
		}
	})
	w.syncActions()
	w.Layout()
}
func (w *WelcomeScreen) SetRecentFiles(files []string) {
	w.recentFiles = slices.Clone(files)
	w.recent.hover, w.recent.pressed = -1, -1
	gui.SetToolTip(w.recent, "")
	w.recent.Clear()
	for _, path := range w.recentFiles {
		w.recent.Append(gui.ListItem{Text: filepath.Base(path), Data: path})
	}
	if len(files) > 0 {
		w.recent.SetActiveIndex(0)
	}
	w.recent.SetVisible(len(files) > 0)
	w.Layout()
	w.Update()
}
func (w *WelcomeScreen) SetBranding(info AboutInfo) { w.branding = info; w.Layout(); w.Update() }

// SetActionLabels customizes the two host actions; labels are translated at paint time.
func (w *WelcomeScreen) SetActionLabels(newLabel, openLabel string) {
	if strings.TrimSpace(newLabel) == "" {
		newLabel = "New Project"
	}
	if strings.TrimSpace(openLabel) == "" {
		openLabel = "Open File"
	}
	w.newLabel, w.openLabel = newLabel, openLabel
	w.Layout()
	w.Update()
}

// SetShortcuts copies informational host bindings. An empty list hides the hints.
// Like other widget setters, this must be called on the UI thread.
func (w *WelcomeScreen) SetShortcuts(shortcuts []WelcomeShortcut) {
	w.shortcuts = slices.Clone(shortcuts)
	w.Layout()
	w.Update()
}
func (w *WelcomeScreen) SetNewProjectCallback(cb func())       { w.cbNewProject = cb; w.syncActions() }
func (w *WelcomeScreen) SetOpenFileCallback(cb func())         { w.cbOpenFile = cb; w.syncActions() }
func (w *WelcomeScreen) SetOpenRecentCallback(cb func(string)) { w.cbOpenRecent = cb; w.syncActions() }
func (w *WelcomeScreen) syncActions() {
	if w.newButton == nil {
		return
	}
	for i, b := range []*welcomeButton{w.newButton, w.openButton} {
		enabled := w.cbNewProject != nil
		if i == 1 {
			enabled = w.cbOpenFile != nil
		}
		b.SetEnabled(enabled)
		b.Action().SetEnabled(enabled)
		b.Update()
	}
	w.recent.SetEnabled(w.cbOpenRecent != nil)
}
func (w *WelcomeScreen) Title() string   { return i18n.T("Welcome") }
func (w *WelcomeScreen) SetTitle(string) {}
func (w *WelcomeScreen) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 720, Height: 460, Policy: gui.GrowHorizontal | gui.GrowVertical | gui.ExpandHorizontal | gui.ExpandVertical}
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
