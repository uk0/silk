package ged

import (
	"math"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// Only visible rows are painted. The body has its own clip so a partially
// scrolled row can never cover the fixed header or its actions.
func (e *FileExplorer) Draw(g paint.Painter) {
	e.ensureFonts()
	e.clampScroll()
	w, h := e.Size()
	if w <= 0 || h <= 0 {
		return
	}
	t := gui.Theme()
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	g.Clip()
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()
	hh := e.headerHeight()
	g.SetBrush1(t.FormColor)
	g.Rectangle(0, 0, w, hh)
	g.Fill()
	g.SetPen1(t.BorderColor, 1)
	g.MoveTo(0, hh-.5)
	g.LineTo(w, hh-.5)
	g.Stroke()
	label := filepath.Base(e.rootDir)
	if e.rootDir == "" {
		label = i18n.T("Project")
	}
	if e.filter != "" {
		label = i18n.T("Filter Files") + ": " + e.filter
	}
	if e.loading {
		label += " …"
	}
	reserve := 56.0
	if e.scanErr != nil {
		reserve += 28
	}
	e.drawText(g, e.boldFont, label, 8, 0, math.Max(0, w-reserve-12), hh, t.TextColor)
	for i, name := range []string{"refresh", "search", "warning"} {
		if i == 2 && e.scanErr == nil {
			continue
		}
		x := w - float64(i+1)*28
		if x < 0 {
			continue
		}
		if i == 1 && e.filter != "" {
			g.SetBrush1(t.ButtonHoverBGColor)
			g.Rectangle(x, 1, 28, hh-2)
			g.Fill()
		}
		g.DrawIcon1(gui.LoadIcon(name), x+6, math.Floor((hh-16)/2), 16, false)
	}
	if h <= hh {
		return
	}
	g.Rectangle(0, hh, w, h-hh)
	g.Clip()
	first, last := e.visibleRows(h)
	for i := first; i < last; i++ {
		entry := e.flatList[i]
		y := hh - e.scrollY + float64(i)*e.rowHeight
		selected := i == e.selectedIdx
		if selected {
			g.SetBrush1(t.HighLightColor)
		} else {
			g.SetBrush1(t.ButtonHoverBGColor)
		}
		if selected || i == e.hoverIdx {
			g.Rectangle(0, y, w, e.rowHeight)
			g.Fill()
		}
		x := 8 + float64(entry.depth)*16
		iy := y + math.Floor((e.rowHeight-16)/2)
		if entry.isDir && (!entry.loaded || len(entry.children) > 0) {
			name := "expander-collapsed"
			if entry.expanded {
				name = "expander-expanded"
			}
			g.DrawIcon1(gui.LoadIcon(name), x, iy, 16, false)
		}
		x += 16
		g.DrawIcon1(gui.LoadIcon(iconNameForEntry(entry.isDir, strings.ToLower(filepath.Ext(entry.name)))), x, iy, 16, false)
		x += 20
		font := e.normalFont
		if entry.isDir {
			font = e.boldFont
		}
		color := t.TextColor
		if selected {
			color = t.MenuActiveTextColor
		}
		right := w - 8
		badge := e.gitStatusFor(entry.path)
		if !entry.isDir && badge != 0 {
			right -= 20
			c := gitBadgeColor(badge)
			if selected {
				c = color
			}
			e.drawText(g, e.boldFont, string(badge), w-24, y, 16, e.rowHeight, c)
		}
		width := math.Max(0, right-x)
		// Entry cache is UI-owned, discarded with a new filesystem snapshot.
		if !entry.textCached || entry.textWidth != width || entry.textRevision != e.fontRevision {
			entry.text = explorerElide(font, explorerDisplayName(entry.name), width)
			entry.textWidth, entry.textRevision, entry.textCached = width, e.fontRevision, true
		}
		e.drawText(g, font, entry.text, x, y, width, e.rowHeight, color)
	}
	if e.root == nil && !e.loading {
		label := i18n.T("Open a project folder")
		if e.scanErr != nil {
			label = e.scanErr.Error()
		}
		e.drawText(g, e.normalFont, label, 8, hh+8, w-16, e.rowHeight, t.TextColor)
	} else if e.root != nil && e.filter != "" && len(e.flatList) == 1 && len(e.root.children) == 0 {
		e.drawText(g, e.normalFont, i18n.T("No matching files"), 8, hh+e.rowHeight, w-16, e.rowHeight, t.TextColor)
	}
}

func (e *FileExplorer) visibleRows(height float64) (int, int) {
	first := max(0, min(len(e.flatList), int(math.Floor(e.scrollY/e.rowHeight))))
	last := max(first, min(len(e.flatList), int(math.Ceil((e.scrollY+height-e.headerHeight())/e.rowHeight))))
	return first, last
}

func (e *FileExplorer) drawText(g paint.Painter, f paint.Font, text string, x, y, width, height float64, c paint.Color) {
	drawPanelText(g, f, text, x, y, width, height, c)
}

// drawPanelText shares font metrics and UTF-8-safe ellipsis across dock panels.
func drawPanelText(g paint.Painter, f paint.Font, text string, x, y, width, height float64, c paint.Color) {
	text = explorerElide(f, explorerDisplayName(text), width)
	if text == "" {
		return
	}
	ext := f.TextExtents(text)
	fe := f.FontExtents()
	g.SetFont(f)
	g.SetBrush1(c)
	g.DrawText1(x-math.Min(0, ext.XBearing), y+(height-fe.Ascent-fe.Descent)/2+fe.Ascent, text)
}

func explorerDisplayName(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, s)
}
func explorerTextWidth(f paint.Font, s string) float64 {
	ext := f.TextExtents(s)
	return math.Max(ext.XAdvance, ext.XBearing+ext.Width) - math.Min(0, ext.XBearing)
}

// Measure whole candidates to account for kerning/ink overhang, never cut a
// UTF-8 rune. Binary search bounds measurement work for long path components.
func explorerElide(f paint.Font, s string, width float64) string {
	if width <= 0 {
		return ""
	}
	if explorerTextWidth(f, s) <= width {
		return s
	}
	const mark = "…"
	if explorerTextWidth(f, mark) > width {
		return ""
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if explorerTextWidth(f, string(r[:mid])+mark) <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(r[:lo]) + mark
}
