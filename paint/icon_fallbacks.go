package paint

import "github.com/uk0/silk/cairo"

// proceduralIconDrawer draws the fallback for a known icon name into
// a square cairoSurface of side `size`. Implementations work in
// pixel space — origin at top-left, x grows right, y grows down.
type proceduralIconDrawer func(size int, cc *cairo.Context)

// proceduralFallbacks holds drawers for the common UI affordances
// silkide expects so a missing-resource theme still renders sensible
// glyphs instead of the red-X placeholder. Names match the strings
// LoadIcon is called with elsewhere in the tree.
var proceduralFallbacks = map[string]proceduralIconDrawer{
	"edit-undo":          drawUndoArrow,
	"edit-redo":          drawRedoArrow,
	"close-btn":          drawCloseX,
	"checkbox-checked":   drawCheckboxChecked,
	"checkbox-unchecked": drawCheckboxUnchecked,
	"expander-collapsed": drawExpanderCollapsed,
	"expander-expanded":  drawExpanderExpanded,
	"arrow-tool":         drawArrowTool,
	"rect-tool":          drawRectTool,
	"menu":               drawMenuBars,
	"refresh":            drawRefreshArrow,
	"plus":               drawPlus,
	"search":             drawSearchGlass,
	"build":              drawBuildWrench,
	"debug":              drawDebugBug,
	"stop":               drawStopSquare,
	"continue":           drawPlayTriangle,
	"play":               drawPlayTriangle, // alias of continue
	"step-over":          drawStepOver,
	"step-into":          drawStepInto,
	"step-out":           drawStepOut,
	"warning":            drawWarningTriangle,
	"git-branch":         drawGitBranch,
	"git":                drawGitBranch, // alias of git-branch
	"terminal":           drawTerminal,
	"go-file":            drawGoFile,
	"file":               drawGoFile, // alias of go-file
	"function":           drawFunctionBraces,
	"gear":               drawGearCog,
	"settings":           drawGearCog, // alias of gear
	"folder-open":        drawFolderOpen,
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

func drawSearchGlass(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Magnifying-glass: circle in upper-left, short diagonal handle
	// extending to lower-right corner.
	cx, cy, r := w*0.40, w*0.40, w*0.22
	cc.Arc(cx, cy, r, 0, 6.28319)
	cc.Stroke()
	// Handle: 45-degree line from circle edge to lower-right.
	hx0 := cx + r*0.7071
	hy0 := cy + r*0.7071
	cc.MoveTo(hx0, hy0)
	cc.LineTo(w*0.78, w*0.78)
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

func drawBuildWrench(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Wrench: an open-jaw ring (circle with a gap facing up-left) and a
	// diagonal handle running to the lower-right corner.
	cx, cy, r := w*0.34, w*0.34, w*0.18
	cc.Arc(cx, cy, r, -1.7, 4.2)
	cc.Stroke()
	// Handle from the ring towards the bottom-right.
	cc.MoveTo(cx+r*0.7071, cy+r*0.7071)
	cc.LineTo(w*0.78, w*0.78)
	cc.Stroke()
}

func drawDebugBug(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Oval body centred slightly low.
	bx, by := w*0.5, w*0.55
	rx, ry := w*0.18, w*0.24
	cc.Save()
	cc.Translate(bx, by)
	cc.Scale(rx, ry)
	cc.Arc(0, 0, 1, 0, 6.28319)
	cc.Restore()
	cc.Stroke()
	// Three pairs of legs angling out from the body sides.
	for _, ly := range []float64{by - ry*0.5, by, by + ry*0.5} {
		cc.MoveTo(bx-rx, ly)
		cc.LineTo(bx-rx-w*0.16, ly)
		cc.MoveTo(bx+rx, ly)
		cc.LineTo(bx+rx+w*0.16, ly)
	}
	cc.Stroke()
	// Two short antennae rising from the head.
	cc.MoveTo(bx-rx*0.5, by-ry)
	cc.LineTo(bx-rx*0.9, by-ry-w*0.14)
	cc.MoveTo(bx+rx*0.5, by-ry)
	cc.LineTo(bx+rx*0.9, by-ry-w*0.14)
	cc.Stroke()
}

func drawStopSquare(size int, cc *cairo.Context) {
	w := float64(size)
	// Filled rounded square — the canonical "halt" glyph.
	cc.SetSourceRGBA(0.35, 0.35, 0.4, 0.95)
	pad := w * 0.24
	r := w * 0.08
	roundedRect(cc, pad, pad, w-pad*2, w-pad*2, r)
	cc.Fill()
}

func drawPlayTriangle(size int, cc *cairo.Context) {
	w := float64(size)
	// Right-pointing filled triangle; shared by "continue" and "play".
	cc.SetSourceRGBA(0.35, 0.35, 0.4, 0.95)
	cc.MoveTo(w*0.30, w*0.22)
	cc.LineTo(w*0.78, w*0.50)
	cc.LineTo(w*0.30, w*0.78)
	cc.ClosePath()
	cc.Fill()
}

func drawStepOver(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Arc hopping left-to-right over a dot, with an arrowhead landing
	// on the right.
	cc.Arc(w*0.5, w*0.58, w*0.26, 3.6, 5.8)
	cc.Stroke()
	tipX, tipY := w*0.72, w*0.50
	cc.MoveTo(tipX, tipY)
	cc.LineTo(tipX-w*0.12, tipY-w*0.04)
	cc.MoveTo(tipX, tipY)
	cc.LineTo(tipX-w*0.04, tipY+w*0.12)
	cc.Stroke()
	// The statement being stepped over, drawn as a dot below the arc.
	cc.Arc(w*0.5, w*0.74, w*0.05, 0, 6.28319)
	cc.Fill()
}

func drawStepInto(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Downward arrow descending into a target dot.
	cc.MoveTo(w*0.5, w*0.20)
	cc.LineTo(w*0.5, w*0.58)
	cc.MoveTo(w*0.5, w*0.58)
	cc.LineTo(w*0.38, w*0.46)
	cc.MoveTo(w*0.5, w*0.58)
	cc.LineTo(w*0.62, w*0.46)
	cc.Stroke()
	cc.Arc(w*0.5, w*0.74, w*0.06, 0, 6.28319)
	cc.Fill()
}

func drawStepOut(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Upward arrow rising out of a target dot.
	cc.MoveTo(w*0.5, w*0.80)
	cc.LineTo(w*0.5, w*0.42)
	cc.MoveTo(w*0.5, w*0.42)
	cc.LineTo(w*0.38, w*0.54)
	cc.MoveTo(w*0.5, w*0.42)
	cc.LineTo(w*0.62, w*0.54)
	cc.Stroke()
	cc.Arc(w*0.5, w*0.26, w*0.06, 0, 6.28319)
	cc.Fill()
}

func drawWarningTriangle(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Up-pointing triangle outline.
	cc.MoveTo(w*0.5, w*0.16)
	cc.LineTo(w*0.85, w*0.80)
	cc.LineTo(w*0.15, w*0.80)
	cc.ClosePath()
	cc.Stroke()
	// Exclamation mark: stem plus a separate dot near the base.
	cc.MoveTo(w*0.5, w*0.40)
	cc.LineTo(w*0.5, w*0.60)
	cc.Stroke()
	cc.Arc(w*0.5, w*0.71, w*0.03, 0, 6.28319)
	cc.Fill()
}

func drawGitBranch(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Two nodes joined by a forking line: a trunk node low-left and a
	// branch node high-right, the trunk splitting up towards it.
	trunkX, branchY := w*0.34, w*0.30
	// Trunk line running top-to-bottom on the left.
	cc.MoveTo(trunkX, w*0.22)
	cc.LineTo(trunkX, w*0.78)
	// Fork peeling off to the branch node.
	cc.MoveTo(trunkX, w*0.50)
	cc.LineTo(w*0.66, branchY)
	cc.Stroke()
	// Nodes as filled dots.
	cc.Arc(trunkX, w*0.78, w*0.07, 0, 6.28319)
	cc.Fill()
	cc.Arc(w*0.66, branchY, w*0.07, 0, 6.28319)
	cc.Fill()
}

func drawTerminal(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Rounded window frame.
	pad := w * 0.14
	roundedRect(cc, pad, pad, w-pad*2, w-pad*2, w*0.08)
	cc.Stroke()
	// ">" prompt chevron in the upper-left of the frame.
	cc.MoveTo(w*0.28, w*0.36)
	cc.LineTo(w*0.40, w*0.48)
	cc.LineTo(w*0.28, w*0.60)
	cc.Stroke()
	// Underscore cursor to the right of the prompt.
	cc.MoveTo(w*0.48, w*0.62)
	cc.LineTo(w*0.66, w*0.62)
	cc.Stroke()
}

func drawGoFile(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Document body with a folded top-right corner. Shared by "go-file"
	// and the generic "file" alias.
	left, right, top, bot := w*0.26, w*0.74, w*0.16, w*0.84
	fold := w * 0.16
	cc.MoveTo(left, top)
	cc.LineTo(right-fold, top)
	cc.LineTo(right, top+fold)
	cc.LineTo(right, bot)
	cc.LineTo(left, bot)
	cc.ClosePath()
	cc.Stroke()
	// The dog-ear: a small triangle in the corner.
	cc.MoveTo(right-fold, top)
	cc.LineTo(right-fold, top+fold)
	cc.LineTo(right, top+fold)
	cc.Stroke()
}

func drawFunctionBraces(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// A pair of curly braces "{ }" — the universal "function/code" mark.
	top, mid, bot := w*0.20, w*0.50, w*0.80
	// Left brace.
	cc.MoveTo(w*0.40, top)
	cc.CurveTo(w*0.30, top, w*0.32, mid-w*0.06, w*0.24, mid)
	cc.CurveTo(w*0.32, mid+w*0.06, w*0.30, bot, w*0.40, bot)
	cc.Stroke()
	// Right brace (mirror).
	cc.MoveTo(w*0.60, top)
	cc.CurveTo(w*0.70, top, w*0.68, mid-w*0.06, w*0.76, mid)
	cc.CurveTo(w*0.68, mid+w*0.06, w*0.70, bot, w*0.60, bot)
	cc.Stroke()
}

func drawGearCog(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	cx, cy := w*0.5, w*0.5
	rIn, rOut := w*0.22, w*0.34
	// Six radial teeth approximated as short ticks around the rim.
	for i := 0; i < 6; i++ {
		a := float64(i) * (6.28319 / 6)
		ca, sa := cosApprox(a), sinApprox(a)
		cc.MoveTo(cx+ca*rIn, cy+sa*rIn)
		cc.LineTo(cx+ca*rOut, cy+sa*rOut)
	}
	cc.Stroke()
	// Body ring.
	cc.Arc(cx, cy, rIn, 0, 6.28319)
	cc.Stroke()
	// Hub hole.
	cc.Arc(cx, cy, w*0.08, 0, 6.28319)
	cc.Stroke()
}

func drawFolderOpen(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	// Back panel with a tab, then a slanted front flap suggesting the
	// folder is open. Complements the closed "folder" glyph.
	left, right := w*0.16, w*0.84
	back := w * 0.30
	bot := w * 0.78
	// Folder back: tab on top-left, body below.
	cc.MoveTo(left, back)
	cc.LineTo(left, w*0.26)
	cc.LineTo(w*0.42, w*0.26)
	cc.LineTo(w*0.50, back)
	cc.LineTo(right, back)
	cc.LineTo(right, bot)
	cc.LineTo(left, bot)
	cc.ClosePath()
	cc.Stroke()
	// Open front flap: a parallelogram skewed to the right.
	cc.MoveTo(left, bot)
	cc.LineTo(w*0.30, w*0.50)
	cc.LineTo(w-left*1.0, w*0.50)
	cc.LineTo(right, bot)
	cc.Stroke()
}

// cosApprox / sinApprox give the gear drawer its trig without pulling
// "math" into this file (the rest of the package keeps angles as bare
// float64 radians passed straight to cairo). A short Taylor/range-reduced
// pair is plenty accurate for placing six tick marks.
func cosApprox(x float64) float64 { return sinApprox(x + 1.5707963267948966) }

func sinApprox(x float64) float64 {
	const twoPi = 6.283185307179586
	// Range-reduce into [-pi, pi].
	for x > 3.141592653589793 {
		x -= twoPi
	}
	for x < -3.141592653589793 {
		x += twoPi
	}
	x2 := x * x
	// 7th-order Maclaurin series: error < 1e-4 over the reduced range.
	return x * (1 - x2/6*(1-x2/20*(1-x2/42)))
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
