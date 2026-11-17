package prop

import (
	"math"
	"sort"

	"github.com/uk0/silk/gui"
)

// Editors live in this inline viewport, not in the sheet alongside its fixed
// search and scrollbar. Widget's default child clipping fences both painting
// and pointer routing, including partially visible editor descendants.
type propertyViewport struct {
	gui.Widget
	sheet      *PropertySheet
	focusOrder []gui.IWidget
}

func (v *propertyViewport) ChildFocusOrder() []gui.IWidget { return v.focusOrder }

func (v *propertyViewport) OnLeftDown(x, y float64) {
	v.sheet.OnLeftDown(x, y+v.Y())
}

func (v *propertyViewport) RevealChildFocus(target gui.IWidget) {
	for child := target; child != nil && child != v; child = child.Parent() {
		if child.Parent() != v {
			continue
		}
		for _, row := range v.sheet.vlist {
			if !row.retired && row.control == child {
				v.sheet.revealRow(row)
				return
			}
		}
		return
	}
}

func (s *PropertySheet) revealRow(row *PropertyItem) {
	h := s.body.Height()
	if h <= 0 || !s.body.IsVisible() {
		return
	}
	top := row.ypos - s.contentTop()
	sy := s.ScrollY()
	if top < sy || row.ItemHeight() > h {
		s.SetScrollY(top)
	} else if bottom := top + row.ItemHeight(); bottom > sy+h {
		s.SetScrollY(bottom - h)
	}
}

func (s *PropertySheet) layoutViewport() {
	clear(s.body.focusOrder) // Do not retain retired controls in spare capacity.
	s.body.focusOrder = s.body.focusOrder[:0]
	for _, row := range s.vlist {
		if !row.retired && row.control != nil && row.control.Parent() == s.body {
			s.body.focusOrder = append(s.body.focusOrder, row.control)
		}
	}
	h := max(0, s.Height()-s.contentTop())
	w := max(0, s.Width())
	bar := s.VertScrollBar()
	limit := max(0, s.contentHeight-h)
	show := limit > 0 && h > 0 && w > 0
	if show {
		w = max(0, w-gui.Theme().ScrollWidth)
	}
	bar.SetVisible(show)
	bar.SetBounds(w, s.contentTop(), min(max(0, s.Width()), gui.Theme().ScrollWidth), h)
	bar.SetDelta(s.DefaultItemHeight(), h)
	bar.SetRange(0, limit)
	s.body.SetBounds(0, s.contentTop(), w, h)
	s.body.SetVisible(w > 0 && h > 0)
	s.positionEditors()
}

// Scroll changes only presentation. No value getter, model setter, Bind,
// history command, background worker or animation timer is involved.
func (s *PropertySheet) OnVertScroll(gui.IWidget) {
	if s.layoutBusy {
		return // SetRange can clamp synchronously inside Layout.
	}
	s.positionEditors()
	s.Update()
}

func (s *PropertySheet) SetScrollY(y float64) {
	if math.IsNaN(y) || math.IsInf(y, 0) {
		return
	}
	s.ScrollArea.SetScrollY(y)
}

func (s *PropertySheet) OnMouseWheel(x, y, delta float64) {
	if math.IsNaN(delta) || math.IsInf(delta, 0) || delta == 0 {
		return
	}
	s.SetScrollY(s.ScrollY() - delta*s.DefaultItemHeight()*3)
}

// Edit declares a wheel handler even in single-line mode, where it does
// nothing. Forward that case explicitly; otherwise the native dispatcher
// stops at the editor and the inspector scrolls only over its label column.
// Custom multiline editors retain their own text scrolling behavior.
func (e *TextEdit) OnMouseWheel(x, y, delta float64) {
	if e.MultiLine() || e.item == nil || e.item.retired {
		e.Edit.OnMouseWheel(x, y, delta)
		return
	}
	s := e.item.sheet
	for parent := e.Parent(); parent != nil; parent = parent.Parent() {
		if parent == s.body {
			s.OnMouseWheel(0, 0, delta)
			return
		}
	}
}

func (s *PropertySheet) positionEditors() {
	for _, row := range s.vlist {
		control := row.control
		if row.retired || control == nil || control.Parent() != s.body {
			continue
		}
		r := s.getItemRect(row, pitemControl)
		r.Y -= s.contentTop() + s.ScrollY()
		if control.Bounds1() != r {
			// A native drop-down cannot remain at its old screen coordinates.
			// Do not Deactivate text fields: scrolling must preserve edit sessions.
			dismissPropertyPopups(control)
			if !row.retired && row.control == control && control.Parent() == s.body {
				// Dismiss callbacks may themselves transfer focus and scroll.
				r = s.getItemRect(row, pitemControl)
				r.Y -= s.contentTop() + s.ScrollY()
				control.SetBounds1(r)
			}
		}
	}
}

func dismissPropertyPopups(root gui.IWidget) {
	if popup, ok := root.(interface{ HideSubPopup() }); ok {
		popup.HideSubPopup()
	}
	for _, child := range root.Children() {
		// A native child's parent may only be its logical owner; do not walk
		// into independent popup/form surfaces while retiring inline popups.
		if child.Parent() == root && child.Window() == nil {
			dismissPropertyPopups(child)
		}
	}
}

// Category entries are ordered and non-overlapping. Paint work is proportional
// to visible rows, not the size of the selected object's property catalogue.
func (s *PropertySheet) visibleEntries() (int, int) {
	top := s.contentTop() + s.ScrollY()
	bottom := top + s.body.Height()
	start := sort.Search(len(s.categoryLayout), func(i int) bool {
		e := s.categoryLayout[i]
		h := categoryHeaderHeight
		if e.item != nil {
			h = e.item.ItemHeight()
		}
		return e.ypos+h > top
	})
	end := sort.Search(len(s.categoryLayout), func(i int) bool {
		return s.categoryLayout[i].ypos >= bottom
	})
	return start, max(start, end)
}
