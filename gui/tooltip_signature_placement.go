package gui

import (
	"math"

	"github.com/uk0/silk/geom"
)

func (ed *CodeEditor) completionAvoidance() (popup, editor geom.Rect, lineHeight float64) {
	if ed.completion == nil {
		return
	}
	l := ed.completion.geometry(ed)
	if l.rows == 0 {
		return
	}
	x, y := ed.MapToWindow(0, 0)
	w, h := ed.Size()
	top := ed.topOffset()
	popup = geom.Rect{X: x + l.x, Y: y + l.y, Width: l.w, Height: l.h}
	editor = geom.Rect{X: x + ed.gutterW, Y: y + top, Width: w - ed.minimapWidth() - ed.gutterW, Height: h - ed.statusBarHeight - top}
	return popup, editor, ed.font.FontExtents().Height + 2
}

func (tip *hoverToolTipOverlay) signatureBounds() geom.Rect {
	w, h := tip.win.Widget().Size()
	if w <= 0 || h <= 0 {
		return geom.Rect{}
	}
	// Label layout is computed only when the requested text/parameter changes;
	// placement uses its cached dimensions even when temporarily suppressed.
	lw, lh := tip.label.Size()
	current := registeredToolTipBounds(w, h, tip.x, tip.y, SizeHints{Width: lw, Height: lh})
	if owner, ok := tip.owner.(interface {
		completionAvoidance() (geom.Rect, geom.Rect, float64)
	}); ok {
		popup, editor, lineH := owner.completionAvoidance()
		if validPopupArea(popup) {
			x, y, w, h := intersectRect(editor.X, editor.Y, editor.Width, editor.Height, 0, 0, w, h)
			return placeSignatureAlongsideCompletion(current, popup, geom.Rect{X: x, Y: y, Width: w, Height: h}, tip.y, lineH)
		}
	}
	return current
}

func placeSignatureAlongsideCompletion(preferred, completion, editor geom.Rect, caretBottom, lineHeight float64) geom.Rect {
	const gap = 4.0
	wc, hc := preferred.Width, preferred.Height
	if !validPopupArea(editor) || !validPopupArea(preferred) ||
		wc > editor.Width || hc > editor.Height {
		return geom.Rect{}
	}
	occupied := geom.Rect{X: completion.X - gap, Y: completion.Y - gap, Width: completion.Width + 2*gap, Height: completion.Height + 2*gap}
	caretTop := caretBottom - lineHeight
	// Clamp the ordinary placement to the editor. Nothing may cover the
	// current source row; try the opposite side of the caret first.
	x := math.Max(editor.X, math.Min(preferred.X, editor.Right()-wc))
	y := math.Max(editor.Y, math.Min(preferred.Y, editor.Bottom()-hc))
	fit := func(x, y float64) (geom.Rect, bool) {
		r := geom.Rect{X: x, Y: y, Width: wc, Height: hc}
		if r.X < editor.X || r.Y < editor.Y || r.Right() > editor.Right() || r.Bottom() > editor.Bottom() ||
			(r.Y < caretBottom && r.Bottom() > caretTop) ||
			(r.X < occupied.Right() && r.Right() > occupied.X && r.Y < occupied.Bottom() && r.Bottom() > occupied.Y) {
			return geom.Rect{}, false
		}
		return r, true
	}
	for _, point := range [5][2]float64{
		{x, y},
		{x, caretTop - gap - hc},
		{x, math.Max(occupied.Bottom(), caretBottom+gap)},
		{occupied.Right(), y},
		{occupied.X - wc, y},
	} {
		if r, ok := fit(point[0], point[1]); ok {
			return r
		}
	}
	// Do not hide selectable rows or put the passive hint over editor chrome
	// when the pane is tiny. Retain the tooltip so it can reappear once the
	// completion menu collapses.
	return geom.Rect{}
}
