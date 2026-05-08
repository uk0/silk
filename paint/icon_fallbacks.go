package paint

import "silk/cairo"

// proceduralIconDrawer draws the fallback for a known icon name into
// a square cairoSurface of side `size`. Implementations work in
// pixel space — origin at top-left, x grows right, y grows down.
type proceduralIconDrawer func(size int, cc *cairo.Context)

// proceduralFallbacks holds drawers for the common UI affordances
// silkide expects so a missing-resource theme still renders sensible
// glyphs instead of the red-X placeholder. Names match the strings
// LoadIcon is called with elsewhere in the tree.
var proceduralFallbacks = map[string]proceduralIconDrawer{
	"edit-undo":           drawUndoArrow,
	"edit-redo":           drawRedoArrow,
	"close-btn":           drawCloseX,
	"checkbox-checked":    drawCheckboxChecked,
	"checkbox-unchecked":  drawCheckboxUnchecked,
	"expander-collapsed":  drawExpanderCollapsed,
	"expander-expanded":   drawExpanderExpanded,
	"arrow-tool":          drawArrowTool,
	"rect-tool":           drawRectTool,
	"menu":                drawMenuBars,
	"refresh":             drawRefreshArrow,
	"plus":                drawPlus,
}

// genProceduralIcon constructs an icon whose subs come from running
// `draw` at the standard sizes. Returns nil if `draw` is nil so the
// caller can fall through to genMissingIcon for unknown names.
func genProceduralIcon(name string, draw proceduralIconDrawer) *icon {
	if draw == nil {
		return nil
	}
	out := new(icon)
	out.name = name
	for _, size := range []int{16, 22, 32, 48} {
		s := NewPixmap(size, size)
		cc := s.NewContext()
		draw(size, cc)
		out.subs = append(out.subs, &subIcon{size, "", s, nil})
	}
	return out
}

// All drawers below paint a single shape in mid-grey on a transparent
// background. Stroke width scales with the icon size so 16px and 48px
// versions stay visually consistent. Colours match the typical
// JetBrains/Qt Creator monochrome icon look.

func setIconStroke(cc *cairo.Context, size int) {
	w := float64(size)
	lw := 1 + w*0.06
	cc.SetSourceRGBA(0.35, 0.35, 0.4, 0.95)
	cc.SetLineWidth(lw)
	cc.SetLineCap(cairo.LINE_CAP_ROUND)
	cc.SetLineJoin(cairo.LINE_JOIN_ROUND)
}

func drawUndoArrow(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Curved arrow returning right-to-left: arc + arrowhead.
	cc.Arc(w*0.55, w*0.55, w*0.32, 1.0, 5.5)
	cc.Stroke()
	// Arrowhead at the arc's start (top-left).
	tipX := w * 0.28
	tipY := w * 0.40
	cc.MoveTo(tipX, tipY)
	cc.LineTo(tipX+w*0.18, tipY-w*0.08)
	cc.MoveTo(tipX, tipY)
	cc.LineTo(tipX+w*0.05, tipY+w*0.18)
	cc.Stroke()
}

func drawRedoArrow(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Mirror of undo: arc opens left-to-right.
	cc.Arc(w*0.45, w*0.55, w*0.32, -2.5, 2.0)
	cc.Stroke()
	tipX := w * 0.72
	tipY := w * 0.40
	cc.MoveTo(tipX, tipY)
	cc.LineTo(tipX-w*0.18, tipY-w*0.08)
	cc.MoveTo(tipX, tipY)
	cc.LineTo(tipX-w*0.05, tipY+w*0.18)
	cc.Stroke()
}

func drawCloseX(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	pad := w * 0.25
	cc.MoveTo(pad, pad)
	cc.LineTo(w-pad, w-pad)
	cc.MoveTo(w-pad, pad)
	cc.LineTo(pad, w-pad)
	cc.Stroke()
}

func drawCheckboxUnchecked(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	pad := w * 0.15
	r := w * 0.1
	roundedRect(cc, pad, pad, w-pad*2, w-pad*2, r)
	cc.Stroke()
}

func drawCheckboxChecked(size int, cc *cairo.Context) {
	w := float64(size)
	// Filled rounded background.
	cc.SetSourceRGBA(0.20, 0.50, 0.85, 0.95)
	pad := w * 0.15
	r := w * 0.1
	roundedRect(cc, pad, pad, w-pad*2, w-pad*2, r)
	cc.Fill()
	// White check mark.
	cc.SetSourceRGB(1, 1, 1)
	cc.SetLineWidth(1 + w*0.07)
	cc.SetLineCap(cairo.LINE_CAP_ROUND)
	cc.SetLineJoin(cairo.LINE_JOIN_ROUND)
	cc.MoveTo(w*0.30, w*0.52)
	cc.LineTo(w*0.45, w*0.66)
	cc.LineTo(w*0.72, w*0.36)
	cc.Stroke()
}

func drawExpanderCollapsed(size int, cc *cairo.Context) {
	w := float64(size)
	cc.SetSourceRGBA(0.35, 0.35, 0.4, 0.95)
	// Right-pointing triangle.
	cc.MoveTo(w*0.35, w*0.25)
	cc.LineTo(w*0.65, w*0.5)
	cc.LineTo(w*0.35, w*0.75)
	cc.ClosePath()
	cc.Fill()
}

func drawExpanderExpanded(size int, cc *cairo.Context) {
	w := float64(size)
	cc.SetSourceRGBA(0.35, 0.35, 0.4, 0.95)
	// Down-pointing triangle.
	cc.MoveTo(w*0.25, w*0.35)
	cc.LineTo(w*0.5, w*0.65)
	cc.LineTo(w*0.75, w*0.35)
	cc.ClosePath()
	cc.Fill()
}

func drawArrowTool(size int, cc *cairo.Context) {
	w := float64(size)
	cc.SetSourceRGBA(0.20, 0.20, 0.25, 1.0)
	// Cursor-arrow silhouette.
	cc.MoveTo(w*0.20, w*0.15)
	cc.LineTo(w*0.20, w*0.78)
	cc.LineTo(w*0.36, w*0.62)
	cc.LineTo(w*0.46, w*0.85)
	cc.LineTo(w*0.55, w*0.80)
	cc.LineTo(w*0.46, w*0.58)
	cc.LineTo(w*0.65, w*0.55)
	cc.ClosePath()
	cc.Fill()
}

func drawRectTool(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	pad := w * 0.18
	cc.Rectangle(pad, pad, w-pad*2, w-pad*2)
	cc.Stroke()
}

func drawMenuBars(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	pad := w * 0.20
	for _, y := range []float64{w * 0.30, w * 0.50, w * 0.70} {
		cc.MoveTo(pad, y)
		cc.LineTo(w-pad, y)
	}
	cc.Stroke()
}

func drawRefreshArrow(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Open arc with a small triangle arrowhead at one end.
	cc.Arc(w*0.5, w*0.5, w*0.32, -2.6, 1.8)
	cc.Stroke()
	// Arrowhead at the start of the arc (top-right).
	tipX := w*0.5 + w*0.32*0.85
	tipY := w*0.5 - w*0.32*0.5
	cc.MoveTo(tipX, tipY)
	cc.LineTo(tipX-w*0.10, tipY-w*0.08)
	cc.MoveTo(tipX, tipY)
	cc.LineTo(tipX+w*0.05, tipY+w*0.14)
	cc.Stroke()
}

func drawPlus(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	pad := w * 0.25
	cc.MoveTo(w*0.5, pad)
	cc.LineTo(w*0.5, w-pad)
	cc.MoveTo(pad, w*0.5)
	cc.LineTo(w-pad, w*0.5)
	cc.Stroke()
}

// roundedRect emits a rounded rectangle path on the cairo context.
// Local helper to avoid pulling cmd-side rounded-rect helpers into
// the paint package.
func roundedRect(cc *cairo.Context, x, y, w, h, r float64) {
	if r <= 0 {
		cc.Rectangle(x, y, w, h)
		return
	}
	const piHalf = 1.5707963267948966
	cc.MoveTo(x+r, y)
	cc.LineTo(x+w-r, y)
	cc.Arc(x+w-r, y+r, r, -piHalf, 0)
	cc.LineTo(x+w, y+h-r)
	cc.Arc(x+w-r, y+h-r, r, 0, piHalf)
	cc.LineTo(x+r, y+h)
	cc.Arc(x+r, y+h-r, r, piHalf, 2*piHalf)
	cc.LineTo(x, y+r)
	cc.Arc(x+r, y+r, r, 2*piHalf, 3*piHalf)
	cc.ClosePath()
}
