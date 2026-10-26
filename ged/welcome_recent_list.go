package ged

import (
	"math"
	"path/filepath"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

type welcomeRecentList struct {
	gui.ListWidget
	owner          *WelcomeScreen
	hover, pressed int
}

func newWelcomeRecentList(owner *WelcomeScreen) *welcomeRecentList {
	l := &welcomeRecentList{owner: owner, hover: -1, pressed: -1}
	l.Init(l)
	l.SetFillAvailable(true)
	l.SetFocusPolicy(gui.TabFocus)
	l.VertScrollBar().SetFocusPolicy(gui.NoFocus)
	return l
}
func (l *welcomeRecentList) Draw(g paint.Painter) {
	if l.Count() == 0 {
		return
	}
	w, h := l.ViewportSizePx()
	if w <= 0 || h <= 0 {
		return
	}
	p := currentWelcomePalette()
	s := l.owner.page.scale
	rh := l.RowHeight()
	sy := l.ScrollY()
	f := l.owner.fonts
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	g.Clip()
	end := min(l.Count(), int(math.Ceil(sy+h/rh)))
	for i := max(0, int(sy)); i < end; i++ {
		y := (float64(i) - sy) * rh
		item := l.Item(i)
		path, _ := item.Data.(string)
		text, muted := p.text, p.muted
		if i == l.ActiveIndex() && l.IsEnabled() {
			g.SetBrush1(p.hoverBG)
			welcomeRoundRect(g, 0, y+2*s, w, rh-4*s, 5*s)
			g.Fill()
			if l.HasFocus() {
				g.SetPen1(p.accent, 1)
				welcomeRoundRect(g, .5, y+2*s+.5, max(0, w-1), rh-4*s-1, 5*s)
				g.Stroke()
			}
		} else if i == l.hover && l.IsEnabled() {
			g.SetBrush1(p.hoverBG)
			welcomeRoundRect(g, 0, y+2*s, w, rh-4*s, 5*s)
			g.Fill()
		}
		if !l.IsEnabled() {
			text = p.muted
		}
		g.SetBrush1(fileIconColor(path, &p))
		welcomeRoundRect(g, 6*s, y+10*s, 3*s, rh-20*s, 1.5*s)
		g.Fill()
		top := (rh - f.body.FontExtents().Height - f.small.FontExtents().Height - 2*s) / 2
		welcomeText(g, f.body, text, 18*s, y+top+f.body.FontExtents().Ascent, w-26*s, item.Text)
		welcomeText(g, f.small, muted, 18*s, y+top+f.body.FontExtents().Height+2*s+f.small.FontExtents().Ascent, w-26*s, filepath.Dir(path))
	}
}
func (l *welcomeRecentList) rowAt(x, y float64) int {
	w, h := l.ViewportSizePx()
	if !hitRect(x, y, [4]float64{0, 0, w, h}) {
		return -1
	}
	row, _ := l.HitTest(x, y)
	return row
}
func (l *welcomeRecentList) OnLeftDown(x, y float64) {
	l.pressed = -1
	if !l.IsEnabled() {
		return
	}
	row := l.rowAt(x, y)
	if row < 0 {
		return
	}
	l.pressed = row
	l.SetActiveIndex(row)
	l.SetFocus()
}
func (l *welcomeRecentList) OnLeftUp(x, y float64) {
	row, down := l.rowAt(x, y), l.pressed
	l.pressed = -1
	// Blank space, scrollbar and drag-out release never reopen the old selection.
	if l.IsEnabled() && row >= 0 && row == down {
		l.Submit()
	}
}
func (l *welcomeRecentList) OnMouseMove(x, y float64) {
	row := l.rowAt(x, y)
	if row == l.hover {
		return
	}
	l.hover = row
	tip := ""
	if row >= 0 {
		tip, _ = l.Item(row).Data.(string)
	}
	gui.SetToolTip(l, tip)
	l.Update()
}
func (l *welcomeRecentList) OnMouseLeave() { l.hover = -1; gui.SetToolTip(l, ""); l.Update() }
func (l *welcomeRecentList) OnMouseWheel(x, y, z float64) {
	old := l.ScrollY()
	l.ListWidget.OnMouseWheel(x, y, z)
	if old == l.ScrollY() {
		l.owner.OnMouseWheel(x, y, z)
	}
}
func (l *welcomeRecentList) OnKeyDown(key int, repeat bool) {
	if !l.IsEnabled() || repeat && (key == gui.KeyEnter || key == gui.KeySpace) {
		return
	}
	l.ListWidget.OnKeyDown(key, repeat)
	l.owner.ensureVisible(l)
}
func (l *welcomeRecentList) OnFocusChanged(n, o gui.IWidget) {
	if n == l {
		l.owner.ensureVisible(l)
	}
	l.Widget.OnFocusChanged(n, o)
}

type welcomeButton struct {
	gui.Button
	owner *WelcomeScreen
}

func newWelcomeButton(w *WelcomeScreen, text, icon string) *welcomeButton {
	b := &welcomeButton{owner: w}
	b.Init(b)
	b.SetTextVisible(true)
	b.SetText(text)
	b.SetIcon(paint.LoadIcon(icon))
	return b
}
func (b *welcomeButton) OnFocusChanged(n, o gui.IWidget) {
	if n == b {
		b.owner.ensureVisible(b)
	}
	b.Widget.OnFocusChanged(n, o)
}
func (w *WelcomeScreen) ensureVisible(child gui.IWidget) {
	_, y := child.Pos()
	_, h := child.Size()
	if child == w.recent && w.recent.ActiveIndex() >= 0 {
		// Reveal the selected row, not the entire list. Revealing the whole
		// list on mouse focus would move the clicked row before button-up.
		y += max(0, (float64(w.recent.ActiveIndex())-w.recent.ScrollY())*w.recent.RowHeight())
		h = w.recent.RowHeight()
	}
	_, height := w.Size()
	pad := 8 * w.page.scale
	if y < pad {
		w.SetScrollY(w.ScrollY() + y - pad)
	} else if y+min(h, height-2*pad) > height-pad {
		w.SetScrollY(w.ScrollY() + y + min(h, height-2*pad) - height + pad)
	}
}
