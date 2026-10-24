package gui

import (
	"math"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

type menuScrollState struct {
	viewport              geom.Rect
	content, offset, band float64
}

// Called before positioning/showing, so a long menu never allocates a native
// surface at its unbounded natural height. Recompute on every open: another
// monitor, font, or live menu contents may have changed since the last popup.
func (m *Menu) fitPopupWorkArea(area geom.Rect) {
	hints := m.SizeHints()
	// Native window sizes use integer coordinate units, while font metrics
	// and content hints may be fractional. Round OUT before allocating the
	// popup: truncating even 0.1 unit would otherwise look like overflow and
	// reserve two full scroll bands on a menu that fits on screen.
	width, height := math.Ceil(hints.Width), math.Ceil(hints.Height)
	if m.IsVertical() && validPopupArea(area) {
		// A constrained long menu rounds IN to stay inside the work area.
		// Native APIs cannot represent a window smaller than one unit.
		height = math.Min(height, math.Max(1, math.Floor(area.Height)))
	}
	if m.Width() != width || m.Height() != height {
		m.SetSize(width, height)
	}
	// A native backend may round dimensions to the same integer pixels and
	// omit the resize notification. Reopening still needs to reset row bounds
	// after OnHide cleared the offset, even when SetSize did not call Layout.
	m.Layout()
}

func (m *Menu) layoutPopupItems() {
	margin := Theme().MenuMargin
	x, y, w, h := margin.Apply(0, 0, m.Width(), m.Height())
	w, h = math.Max(0, w), math.Max(0, h)
	total := 0.0
	for _, c := range m.items {
		if c.IsVisible() {
			total += math.Max(0, c.SizeHints().Height)
		}
	}
	band := 0.0
	if total > h {
		band = math.Min(18, h/3)
	}
	m.scroll.content, m.scroll.band = total, band
	m.scroll.viewport = geom.Rect{X: x, Y: y + band, Width: w, Height: math.Max(0, h-2*band)}
	m.scroll.offset = math.Max(0, math.Min(m.scroll.offset, total-m.scroll.viewport.Height))
	top := m.scroll.viewport.Y - m.scroll.offset
	for _, c := range m.items {
		if !c.IsVisible() {
			continue
		}
		hints := c.SizeHints()
		width := math.Min(w, hints.Width)
		if hints.Policy&(ExpandHorizontal|GrowHorizontal) != 0 {
			width = w
		}
		height := math.Max(0, hints.Height)
		c.SetBounds(x, top, width, height)
		top += height
	}
}

// ChildViewport implements IChildViewport. Offscreen rows retain logical
// visibility (for search/keyboard navigation), but cannot draw or receive a
// click through the scroll bands. No rows are squeezed to fit the screen.
func (m *Menu) ChildViewport() geom.Rect {
	if !m.IsPopup() || !m.IsVertical() {
		return geom.Rect{Width: m.Width(), Height: m.Height()}
	}
	return m.scroll.viewport
}

func (m *Menu) setScrollOffset(offset float64) bool {
	if !m.IsPopup() || !m.IsVertical() || math.IsNaN(offset) || math.IsInf(offset, 0) {
		return false
	}
	offset = math.Max(0, math.Min(offset, m.scroll.content-m.scroll.viewport.Height))
	delta := offset - m.scroll.offset
	if delta == 0 {
		return false
	}
	// A child popup must not stay anchored to a row that has moved offscreen.
	// Avoid allocating a snapshot of every child on every wheel tick when
	// nothing is open. If one is open, retain HideAllSubs' callback-safe path.
	for _, item := range m.items {
		if b, ok := item.(IButton); ok && b.SubPopup() != nil && b.SubPopup().IsVisible() {
			m.HideAllSubs()
			break
		}
	}
	m.scroll.offset = offset
	for _, item := range m.items {
		if item.IsVisible() {
			item.SetPos(item.X(), item.Y()-delta)
		}
	}
	m.Update()
	return true
}

func (m *Menu) ensureHighlightedVisible() {
	b := m.highlightedButton()
	v := m.scroll.viewport
	if b == nil || !b.IsVisible() || v.Height <= 0 || !m.IsPopup() || !m.IsVertical() {
		return
	}
	if b.Y() < v.Y || b.Height() > v.Height {
		m.setScrollOffset(m.scroll.offset + b.Y() - v.Y)
	} else if b.Y()+b.Height() > v.Bottom() {
		m.setScrollOffset(m.scroll.offset + b.Y() + b.Height() - v.Bottom())
	}
}

func (m *Menu) scrollBy(delta float64) {
	if !m.setScrollOffset(m.scroll.offset + delta) {
		return
	}
	m.resetTypeahead()
	m.setHighlight(-1)
	if mouseHoverWidget != nil && mouseHoverWidget.Parent() == m {
		setMouseHoverWidget(nil)
	}
}

func (m *Menu) OnMouseWheel(x, y, z float64) {
	if !m.IsVisible() || !m.IsEnabled() || !m.IsAllAncentorsVisible() || z == 0 || math.IsNaN(z) || math.IsInf(z, 0) {
		return
	}
	// A nested popup owns capture. Wheel over its parent should scroll the
	// parent and close the child, not scroll a menu under an unrelated window.
	if x < 0 || y < 0 || x >= m.Width() || y >= m.Height() {
		gx, gy := m.MapToGlobal(x, y)
		for p := m.Parent(); p != nil; p = p.Parent() {
			if parent, ok := p.(*Menu); ok && parent.IsPopup() {
				px, py := parent.MapFromGlobal(gx, gy)
				if px >= 0 && py >= 0 && px < parent.Width() && py < parent.Height() {
					parent.OnMouseWheel(px, py, z)
					return
				}
			}
		}
		return
	}
	m.scrollBy(-z * math.Max(16, Theme().ItemHeight) * 3)
}

// Scroll bands respond only to input events; there is no hover timer, idle
// animation, or goroutine. Wheel/Page keys cover continuous/fast navigation.
func (m *Menu) clickScrollBand(x, y float64) bool {
	if !m.IsPopup() || !m.IsVisible() || !m.IsEnabled() || m.scroll.band <= 0 || x < 0 || x >= m.Width() || y < 0 || y >= m.Height() {
		return false
	}
	v := m.scroll.viewport
	if y < v.Y {
		m.scrollBy(-math.Max(16, Theme().ItemHeight) * 3)
		return true
	}
	if y >= v.Bottom() {
		m.scrollBy(math.Max(16, Theme().ItemHeight) * 3)
		return true
	}
	return false
}

func (m *Menu) pageHighlight(direction int, kinds []menuItemKind) {
	if len(kinds) == 0 {
		return
	}
	idx := m.highlight
	if idx < 0 || idx >= len(m.items) || !kinds[idx].selectable {
		start := -1
		if direction < 0 {
			start = len(kinds)
		}
		idx = nextSelectableIndex(kinds, start, direction, false)
		if idx < 0 {
			return
		}
	}
	row := m.items[idx]
	target := row.Y() + float64(direction)*math.Max(row.Height(), m.scroll.viewport.Height-row.Height())
	for {
		next := nextSelectableIndex(kinds, idx, direction, false)
		if next < 0 {
			break
		}
		idx = next
		if direction > 0 && m.items[idx].Y() >= target || direction < 0 && m.items[idx].Y() <= target {
			break
		}
	}
	m.HideAllSubs()
	m.setHighlight(idx)
}

func (m *Menu) DrawOverlay(g paint.Painter) {
	if !m.IsPopup() || m.scroll.band <= 0 {
		return
	}
	t := Theme()
	v := m.scroll.viewport
	for _, direction := range []int{-1, 1} {
		y := v.Y - m.scroll.band/2
		enabled := m.scroll.offset > 0
		if direction > 0 {
			y = v.Bottom() + m.scroll.band/2
			enabled = m.scroll.offset+v.Height < m.scroll.content
		}
		color := t.MenuGrayTextColor
		if enabled {
			color = t.MenuTextColor
		}
		size := math.Min(4, m.scroll.band/3)
		x := m.Width() / 2
		g.SetPen1(color, 1.5)
		g.MoveTo(x-size, y-float64(direction)*size/2)
		g.LineTo(x, y+float64(direction)*size/2)
		g.LineTo(x+size, y-float64(direction)*size/2)
		g.Stroke()
	}
}
