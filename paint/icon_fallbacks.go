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
	"navigate-back":      drawNavigateBack,
	"navigate-forward":   drawNavigateForward,
	"close-btn":          drawCloseX,
	"checkbox-checked":   drawCheckboxChecked,
	"checkbox-unchecked": drawCheckboxUnchecked,
	"expander-collapsed": drawExpanderCollapsed,
	"expander-expanded":  drawExpanderExpanded,
	"arrow-tool":         drawArrowTool,
	"rect-tool":          drawRectTool,
	"menu":               drawMenuBars,
	"toolbar-overflow":   drawToolbarOverflow,
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
	"folder":             drawFolderOpen,
	"document":           drawGoFile,
	"run":                drawPlayTriangle,
	"save":               drawSaveDisk,
	"preview":            drawPreviewEye,
	"propsheet":          drawOutlineList,
	"outline":            drawOutlineList,
	"tree-view":          drawOutlineList,
	"packages":           drawFolderOpen,
	"bookmarks":          drawBookmarkRibbon,
	"references":         drawFunctionBraces,
	"log":                drawTerminal,
	"todo":               drawCheckboxChecked,
	"silkide":            drawPanelLayout,
	"design":             drawPanelLayout,
	"edit":               drawFunctionBraces,
	"minimize":           drawMinimizeLine,
	"maximize":           drawMaximizeWindow,
	"close":              drawCloseX,
	"error":              drawErrorCircle,
	"question":           drawHelpCircle,
	"help":               drawHelpCircle,
	"window":             drawWindowFrame,
	"form":               drawFormFields,
	"link":               drawLinkChain,
	"keymap":             drawKeyboard,
	"test-explorer":      drawTestChecklist,
	"call-hierarchy":     drawCallHierarchy,
	"type-hierarchy":     drawTypeHierarchy,
	"project":            drawProjectCube,
	"diagram":            drawDiagramNodes,
	"merge":              drawMergeBranches,
	"layout-horizontal":  drawLayoutCells(3, 1),
	"layout-vertical":    drawLayoutCells(1, 3),
	"layout-grid":        drawLayoutCells(2, 2),
	"layout-form":        drawFormFields,
	"layout-break":       drawBreakLayout,
	"layout-adjust":      drawAdjustLayout,
	"handle-0-normal":    drawResizeHandle(0, false),
	"handle-0-active":    drawResizeHandle(0, true),
	"handle-1-normal":    drawResizeHandle(1, false),
	"handle-1-active":    drawResizeHandle(1, true),
	"handle-2-normal":    drawResizeHandle(2, false),
	"handle-2-active":    drawResizeHandle(2, true),
}

// A drawn double chevron avoids dependence on the text font's U+00BB glyph.
// Procedural icons are rasterized at the requested physical size for HiDPI.
func drawToolbarOverflow(size int, cc *cairo.Context) {
	setIconStroke(cc, size)
	s := float64(size)
	for _, x := range []float64{0.24, 0.54} {
		cc.MoveTo(x*s, 0.28*s)
		cc.LineTo((x+0.22)*s, 0.5*s)
		cc.LineTo(x*s, 0.72*s)
		cc.Stroke()
	}
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
	out.draw = draw
	for _, size := range []int{16, 22, 32, 48} {
		s := NewPixmap(size, size)
		cc := s.NewContext()
		draw(size, cc)
		cc.Destroy()
		out.subs = append(out.subs, &subIcon{size, "", s, nil})
	}
	return out
}

// Geometry uses a shared stroke scale. Exact theme/semantic colors live in
// icon_palette.go rather than scattered approximate RGB fractions.

func setIconStroke(cc *cairo.Context, size int) {
	w := float64(size)
	lw := w * (1.75 / 16)
	setIconColor(cc, currentIconPalette.Foreground)
	cc.SetLineWidth(lw)
	cc.SetLineCap(cairo.LINE_CAP_ROUND)
	cc.SetLineJoin(cairo.LINE_JOIN_ROUND)
}

func drawNavigateBack(size int, cc *cairo.Context)    { drawNavigateArrow(size, cc, false) }
func drawNavigateForward(size int, cc *cairo.Context) { drawNavigateArrow(size, cc, true) }
func drawNavigateArrow(size int, cc *cairo.Context, forward bool) {
	w := float64(size)
	setIconStroke(cc, size)
	x := func(v float64) float64 {
		if forward {
			return w * (1 - v)
		}
		return w * v
	}
	cc.MoveTo(x(0.78), w*0.5)
	cc.LineTo(x(0.22), w*0.5)
	cc.MoveTo(x(0.45), w*0.27)
	cc.LineTo(x(0.22), w*0.5)
	cc.LineTo(x(0.45), w*0.73)
	cc.Stroke()
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
	setIconColor(cc, currentIconPalette.Accent)
	pad := w * 0.15
	r := w * 0.1
	roundedRect(cc, pad, pad, w-pad*2, w-pad*2, r)
	cc.Fill()
	// White check mark.
	setIconColor(cc, currentIconPalette.Paper)
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
	setIconColor(cc, currentIconPalette.Foreground)
	// Right-pointing triangle.
	cc.MoveTo(w*0.35, w*0.25)
	cc.LineTo(w*0.65, w*0.5)
	cc.LineTo(w*0.35, w*0.75)
	cc.ClosePath()
	cc.Fill()
}

func drawExpanderExpanded(size int, cc *cairo.Context) {
	w := float64(size)
	setIconColor(cc, currentIconPalette.Foreground)
	// Down-pointing triangle.
	cc.MoveTo(w*0.25, w*0.35)
	cc.LineTo(w*0.5, w*0.65)
	cc.LineTo(w*0.75, w*0.35)
	cc.ClosePath()
	cc.Fill()
}

func drawArrowTool(size int, cc *cairo.Context) {
	w := float64(size)
	setIconColor(cc, currentIconPalette.Foreground)
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
	setIconColor(cc, currentIconPalette.Warning)
	// Open jaws make this a wrench, not the search glyph's closed ring.
	cc.MoveTo(w*.72, w*.14)
	cc.CurveTo(w*.48, w*.10, w*.35, w*.28, w*.40, w*.47)
	cc.LineTo(w*.16, w*.71)
	cc.CurveTo(w*.10, w*.77, w*.10, w*.85, w*.16, w*.89)
	cc.CurveTo(w*.22, w*.94, w*.28, w*.92, w*.33, w*.86)
	cc.LineTo(w*.58, w*.61)
	cc.CurveTo(w*.80, w*.68, w*.98, w*.44, w*.90, w*.23)
	cc.LineTo(w*.75, w*.38)
	cc.LineTo(w*.62, w*.36)
	cc.LineTo(w*.60, w*.23)
	cc.ClosePath()
	cc.Stroke()
}

func drawDebugBug(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	setIconColor(cc, currentIconPalette.Violet)
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
	setIconColor(cc, currentIconPalette.Danger)
	pad := w * 0.24
	r := w * 0.08
	roundedRect(cc, pad, pad, w-pad*2, w-pad*2, r)
	cc.Fill()
}

func drawPlayTriangle(size int, cc *cairo.Context) {
	w := float64(size)
	// Right-pointing filled triangle; shared by "continue" and "play".
	setIconColor(cc, currentIconPalette.Success)
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
	setIconColor(cc, currentIconPalette.Warning)
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
	setIconColor(cc, currentIconPalette.Violet)
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
	setIconColor(cc, currentIconPalette.Accent)
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
	setIconColor(cc, currentIconPalette.Violet)
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
	setIconColor(cc, currentIconPalette.Warning)
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

func drawSaveDisk(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	setIconColor(cc, currentIconPalette.Accent)
	cc.MoveTo(w*.2, w*.15)
	cc.LineTo(w*.7, w*.15)
	cc.LineTo(w*.85, w*.3)
	cc.LineTo(w*.85, w*.85)
	cc.LineTo(w*.15, w*.85)
	cc.LineTo(w*.15, w*.15)
	cc.ClosePath()
	cc.Stroke()
	cc.Rectangle(w*.3, w*.15, w*.35, w*.25)
	cc.Stroke()
	cc.Rectangle(w*.3, w*.58, w*.4, w*.27)
	cc.Stroke()
}

func drawPreviewEye(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	setIconColor(cc, currentIconPalette.Accent)
	cc.MoveTo(w*.08, w*.5)
	cc.CurveTo(w*.3, w*.15, w*.7, w*.15, w*.92, w*.5)
	cc.CurveTo(w*.7, w*.85, w*.3, w*.85, w*.08, w*.5)
	cc.Stroke()
	cc.Arc(w*.5, w*.5, w*.13, 0, 6.28319)
	cc.Stroke()
}

func drawOutlineList(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	for _, y := range []float64{.25, .5, .75} {
		cc.MoveTo(w*.18, w*y)
		cc.LineTo(w*.24, w*y)
		cc.MoveTo(w*.42, w*y)
		cc.LineTo(w*.82, w*y)
	}
	cc.Stroke()
}

func drawBookmarkRibbon(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	setIconColor(cc, currentIconPalette.Warning)
	cc.MoveTo(w*.28, w*.15)
	cc.LineTo(w*.72, w*.15)
	cc.LineTo(w*.72, w*.85)
	cc.LineTo(w*.5, w*.68)
	cc.LineTo(w*.28, w*.85)
	cc.ClosePath()
	cc.Stroke()
}

func drawPanelLayout(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	cc.Rectangle(w*.14, w*.18, w*.72, w*.64)
	cc.MoveTo(w*.14, w*.36)
	cc.LineTo(w*.86, w*.36)
	cc.MoveTo(w*.38, w*.36)
	cc.LineTo(w*.38, w*.82)
	cc.Stroke()
}

func drawMinimizeLine(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	cc.MoveTo(w*.2, w*.65)
	cc.LineTo(w*.8, w*.65)
	cc.Stroke()
}

// Selection handles remain crisp and recognizable without a resource pack.
func drawResizeHandle(shape int, active bool) proceduralIconDrawer {
	return func(size int, cc *cairo.Context) {
		w := float64(size)
		pad := w * 0.22
		switch shape {
		case 1:
			cc.Arc(w/2, w/2, w/2-pad, 0, 6.283185307179586)
		case 2:
			cc.MoveTo(w/2, pad)
			cc.LineTo(w-pad, w/2)
			cc.LineTo(w/2, w-pad)
			cc.LineTo(pad, w/2)
			cc.ClosePath()
		default:
			cc.Rectangle(pad, pad, w-2*pad, w-2*pad)
		}
		if active {
			setIconColor(cc, currentIconPalette.Accent)
		} else {
			setIconColor(cc, currentIconPalette.Paper)
		}
		cc.FillPreserve()
		setIconColor(cc, currentIconPalette.Accent)
		cc.SetLineWidth(w * 0.10)
		cc.Stroke()
	}
}

// Status and panel icons share the same semantic palette as the workbench.
// None relies on a font glyph (which could be missing) or an external PNG.
func drawErrorCircle(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	setIconColor(cc, currentIconPalette.Danger)
	cc.Arc(w*.5, w*.5, w*.35, 0, 6.283185307179586)
	cc.Stroke()
	cc.MoveTo(w*.38, w*.38)
	cc.LineTo(w*.62, w*.62)
	cc.MoveTo(w*.62, w*.38)
	cc.LineTo(w*.38, w*.62)
	cc.Stroke()
}

func drawHelpCircle(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	setIconColor(cc, currentIconPalette.Accent)
	cc.Arc(w*.5, w*.5, w*.35, 0, 6.283185307179586)
	cc.Stroke()
	cc.MoveTo(w*.39, w*.38)
	cc.CurveTo(w*.40, w*.24, w*.66, w*.25, w*.62, w*.41)
	cc.CurveTo(w*.60, w*.48, w*.5, w*.46, w*.5, w*.55)
	cc.Stroke()
	cc.Arc(w*.5, w*.69, w*.032, 0, 6.283185307179586)
	cc.Fill()
}

func drawWindowFrame(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	roundedRect(cc, w*.13, w*.17, w*.74, w*.66, w*.06)
	cc.MoveTo(w*.13, w*.35)
	cc.LineTo(w*.87, w*.35)
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Accent)
	cc.MoveTo(w*.25, w*.26)
	cc.LineTo(w*.36, w*.26)
	cc.Stroke()
}

func drawMaximizeWindow(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	roundedRect(cc, w*.19, w*.19, w*.62, w*.62, w*.03)
	cc.Stroke()
}

func drawFormFields(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	for _, y := range []float64{.3, .7} {
		cc.MoveTo(w*.16, w*y)
		cc.LineTo(w*.32, w*y)
	}
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Accent)
	for _, y := range []float64{.2, .6} {
		roundedRect(cc, w*.46, w*y, w*.38, w*.2, w*.025)
	}
	cc.Stroke()
}

func drawLinkChain(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	setIconColor(cc, currentIconPalette.Accent)
	// Open, interlocking capsules leave a clean gap at their crossing.
	cc.MoveTo(w*.43, w*.28)
	cc.LineTo(w*.49, w*.22)
	cc.CurveTo(w*.71, w*.01, w*.99, w*.30, w*.77, w*.51)
	cc.LineTo(w*.70, w*.58)
	cc.CurveTo(w*.58, w*.70, w*.47, w*.63, w*.43, w*.57)
	cc.Stroke()
	cc.MoveTo(w*.57, w*.72)
	cc.LineTo(w*.51, w*.78)
	cc.CurveTo(w*.29, w*.99, w*.01, w*.70, w*.23, w*.49)
	cc.LineTo(w*.30, w*.42)
	cc.CurveTo(w*.42, w*.30, w*.53, w*.37, w*.57, w*.43)
	cc.Stroke()
}

func drawKeyboard(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	roundedRect(cc, w*.12, w*.25, w*.76, w*.5, w*.05)
	cc.Stroke()
	// Filled keys stay distinct even at 16 logical pixels.
	for _, y := range []float64{.38, .51} {
		for _, x := range []float64{.26, .42, .58, .74} {
			cc.Rectangle(w*(x-.028), w*(y-.028), w*.056, w*.056)
		}
	}
	cc.Fill()
	cc.SetLineWidth(w * .055)
	cc.MoveTo(w*.33, w*.64)
	cc.LineTo(w*.67, w*.64)
	cc.Stroke()
}

func drawTestChecklist(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	for _, y := range []float64{.3, .7} {
		cc.MoveTo(w*.56, w*y)
		cc.LineTo(w*.84, w*y)
	}
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Success)
	for _, y := range []float64{.3, .7} {
		cc.MoveTo(w*.16, w*y)
		cc.LineTo(w*.25, w*(y+.09))
		cc.LineTo(w*.41, w*(y-.11))
	}
	cc.Stroke()
}

func drawTypeHierarchy(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	cc.MoveTo(w*.5, w*.36)
	cc.LineTo(w*.5, w*.52)
	cc.MoveTo(w*.24, w*.64)
	cc.LineTo(w*.24, w*.52)
	cc.LineTo(w*.76, w*.52)
	cc.LineTo(w*.76, w*.64)
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Violet)
	cc.Rectangle(w*.38, w*.14, w*.24, w*.22)
	cc.Rectangle(w*.12, w*.64, w*.24, w*.22)
	cc.Rectangle(w*.64, w*.64, w*.24, w*.22)
	cc.Stroke()
}

func drawCallHierarchy(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	cc.MoveTo(w*.36, w*.5)
	cc.LineTo(w*.49, w*.5)
	cc.LineTo(w*.49, w*.24)
	cc.LineTo(w*.63, w*.24)
	cc.MoveTo(w*.49, w*.5)
	cc.LineTo(w*.49, w*.76)
	cc.LineTo(w*.63, w*.76)
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Accent)
	for _, pt := range [][2]float64{{.24, .5}, {.76, .24}, {.76, .76}} {
		cc.Arc(w*pt[0], w*pt[1], w*.115, 0, 6.283185307179586)
		cc.Stroke()
	}
}

func drawProjectCube(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	setIconColor(cc, currentIconPalette.Warning)
	cc.MoveTo(w*.5, w*.13)
	cc.LineTo(w*.84, w*.31)
	cc.LineTo(w*.84, w*.69)
	cc.LineTo(w*.5, w*.87)
	cc.LineTo(w*.16, w*.69)
	cc.LineTo(w*.16, w*.31)
	cc.ClosePath()
	cc.MoveTo(w*.16, w*.31)
	cc.LineTo(w*.5, w*.5)
	cc.LineTo(w*.84, w*.31)
	cc.MoveTo(w*.5, w*.5)
	cc.LineTo(w*.5, w*.87)
	cc.Stroke()
}

func drawDiagramNodes(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	cc.MoveTo(w*.37, w*.28)
	cc.LineTo(w*.63, w*.28)
	cc.MoveTo(w*.76, w*.40)
	cc.LineTo(w*.76, w*.72)
	cc.LineTo(w*.37, w*.72)
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Accent)
	for _, pt := range [][2]float64{{.12, .16}, {.63, .16}, {.12, .60}} {
		roundedRect(cc, w*pt[0], w*pt[1], w*.25, w*.24, w*.035)
	}
	cc.Stroke()
}

func drawMergeBranches(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	setIconColor(cc, currentIconPalette.Violet)
	cc.MoveTo(w*.30, w*.71)
	cc.LineTo(w*.30, w*.20)
	cc.MoveTo(w*.70, w*.71)
	cc.CurveTo(w*.70, w*.46, w*.30, w*.54, w*.30, w*.34)
	cc.MoveTo(w*.18, w*.31)
	cc.LineTo(w*.30, w*.19)
	cc.LineTo(w*.42, w*.31)
	cc.Stroke()
	for _, x := range []float64{.30, .70} {
		cc.Arc(w*x, w*.80, w*.075, 0, 6.283185307179586)
		cc.Stroke()
	}
}

// Layout glyphs depict the exact operation, not generic grid/file placeholders.
func drawLayoutCells(cols, rows int) proceduralIconDrawer {
	return func(size int, cc *cairo.Context) {
		w := float64(size)
		setIconStroke(cc, size)
		setIconColor(cc, currentIconPalette.Accent)
		const pad, gap = .15, .14
		cw := (1 - 2*pad - float64(cols-1)*gap) / float64(cols)
		ch := (1 - 2*pad - float64(rows-1)*gap) / float64(rows)
		for row := 0; row < rows; row++ {
			for col := 0; col < cols; col++ {
				cc.Rectangle(w*(pad+float64(col)*(cw+gap)), w*(pad+float64(row)*(ch+gap)), w*cw, w*ch)
			}
		}
		cc.Stroke()
	}
}

func drawBreakLayout(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	cc.MoveTo(w*.40, w*.16)
	cc.LineTo(w*.16, w*.16)
	cc.LineTo(w*.16, w*.40)
	cc.MoveTo(w*.60, w*.16)
	cc.LineTo(w*.84, w*.16)
	cc.LineTo(w*.84, w*.40)
	cc.MoveTo(w*.16, w*.60)
	cc.LineTo(w*.16, w*.84)
	cc.LineTo(w*.40, w*.84)
	cc.MoveTo(w*.60, w*.84)
	cc.LineTo(w*.84, w*.84)
	cc.LineTo(w*.84, w*.60)
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Warning)
	cc.MoveTo(w*.38, w*.38)
	cc.LineTo(w*.62, w*.62)
	cc.MoveTo(w*.62, w*.38)
	cc.LineTo(w*.38, w*.62)
	cc.Stroke()
}

func drawAdjustLayout(size int, cc *cairo.Context) {
	w := float64(size)
	setIconStroke(cc, size)
	setIconColor(cc, currentIconPalette.Accent)
	// Four bounds corners enclose the content box. Unlike diagonal arrows,
	// this remains legible at 16px and cannot resemble a broken-link glyph.
	cc.MoveTo(w*.14, w*.32)
	cc.LineTo(w*.14, w*.14)
	cc.LineTo(w*.32, w*.14)
	cc.MoveTo(w*.68, w*.14)
	cc.LineTo(w*.86, w*.14)
	cc.LineTo(w*.86, w*.32)
	cc.MoveTo(w*.14, w*.68)
	cc.LineTo(w*.14, w*.86)
	cc.LineTo(w*.32, w*.86)
	cc.MoveTo(w*.68, w*.86)
	cc.LineTo(w*.86, w*.86)
	cc.LineTo(w*.86, w*.68)
	cc.Rectangle(w*.39, w*.39, w*.22, w*.22)
	cc.Stroke()
}
