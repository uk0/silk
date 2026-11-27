package gui

import "github.com/uk0/silk/geom"

// Schedule a partial repaint for both footprints of an inline widget. The
// footprints start in parent coordinates. Each ancestor uses the same child
// clip as painting and hit testing; desktop coordinates never enter damage.
func (w *Widget) damageInlineGeometry(next geom.Rect) {
	if w.win != nil || w.wt != WtInherit || w.hidden || w.parent == nil {
		return
	}
	before := geom.Rect{X: w.x, Y: w.y, Width: w.w, Height: w.h}
	beforeVisible, nextVisible := validPopupArea(before), validPopupArea(next)
	for parent := w.parent; parent != nil && (beforeVisible || nextVisible); parent = parent.Parent() {
		if !parent.IsVisible() {
			return
		}
		clip := widgetChildViewport(parent)
		if beforeVisible {
			before = before.IntersectCopy(clip)
			beforeVisible = before.Width > 0 && before.Height > 0
		}
		if nextVisible {
			next = next.IntersectCopy(clip)
			nextVisible = next.Width > 0 && next.Height > 0
		}
		if owner := parent.Window(); owner != nil {
			switch {
			case beforeVisible && nextVisible:
				owner.markGeometryDamage(before.UniteCopy(next))
			case beforeVisible:
				owner.markGeometryDamage(before)
			case nextVisible:
				owner.markGeometryDamage(next)
			}
			return
		}
		x, y := parent.Pos()
		before.X, before.Y = before.X+x, before.Y+y
		next.X, next.Y = next.X+x, next.Y+y
	}
}

// A popup can extend past the bounds of its button or even its parent menu.
// On repositioning, repair the exposed screen rectangle in the popup family
// through its first independent form; never dirty unrelated top-level forms.
func damagePopupUnderlay(win *Window, screen geom.Rect) {
	if win == nil || win.widget == nil || !validPopupArea(screen) {
		return
	}
	for owner := win.ParentWindow(); owner != nil && owner != win; owner = owner.ParentWindow() {
		if owner.widget == nil {
			break
		}
		if owner.widget.IsVisible() {
			x, y := owner.mapFromGlobal(screen.X, screen.Y)
			local := geom.Rect{X: x, Y: y, Width: screen.Width, Height: screen.Height}
			w, h := owner.widget.Size()
			local = local.IntersectCopy(geom.Rect{Width: w, Height: h})
			if local.Width > 0 && local.Height > 0 {
				owner.markGeometryDamage(local)
			}
		}
		if owner.wt == WtForm {
			break
		}
	}
}
