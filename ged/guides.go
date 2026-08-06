package ged

import (
	"math"
	"strconv"
	"strings"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

// Rulers and user-placed guide lines for the design canvas. The scene owns the
// guides — they belong to the document and are saved with it; the view owns
// the rulers, the drag gesture and the painting. Every mapping and snap rule
// lives in a pure function here so it can be checked without a window.

const (
	// rulerThicknessPx is the height of the top ruler and the width of the
	// left one, in view pixels.
	rulerThicknessPx = 18.0

	// The ruler scale: a tick every rulerTickMm, a number every rulerLabelMm.
	rulerTickMm  = 10.0
	rulerLabelMm = 50.0

	// Tick lengths in view pixels — a numbered tick is drawn longer so the
	// scale can be read at a glance.
	rulerTickPx      = 4.0
	rulerLabelTickPx = 8.0

	// rulerLabelFontSize is the tick-label height. The rulers paint in widget
	// space, so this is 9 pixels — unlike the scene's mm-space size readout.
	rulerLabelFontSize = 9.0

	// guideGrabPx is how close, in view pixels, the cursor must be to a guide
	// for a press to pick it up instead of reaching the widgets under it.
	guideGrabPx = 4.0
)

// sceneGuides is one design's set of user-placed guides, in scene mm: V holds
// the vertical guides (fixed x), H the horizontal ones (fixed y).
type sceneGuides struct {
	V []float64
	H []float64
}

func (this sceneGuides) isEmpty() bool {
	return len(this.V) == 0 && len(this.H) == 0
}

// encodeGuides renders a guide set as one attribute string, e.g. "v:10.5,h:40".
// A single string attr keeps the scene's TDoc shape identical to its existing
// bounds/title attrs rather than introducing a child-node schema.
func encodeGuides(guides sceneGuides) string {
	parts := make([]string, 0, len(guides.V)+len(guides.H))
	for _, x := range guides.V {
		parts = append(parts, "v:"+strconv.FormatFloat(x, 'g', -1, 64))
	}
	for _, y := range guides.H {
		parts = append(parts, "h:"+strconv.FormatFloat(y, 'g', -1, 64))
	}
	return strings.Join(parts, ",")
}

// decodeGuides parses what encodeGuides wrote. Unreadable tokens are skipped
// rather than failing the load: one bad guide must not stop a design file from
// opening, the same tolerance loadChildWidgets applies to unknown widgets.
func decodeGuides(s string) sceneGuides {
	var guides sceneGuides
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if len(tok) < 3 || tok[1] != ':' {
			continue
		}
		v, err := strconv.ParseFloat(tok[2:], 64)
		if err != nil {
			continue
		}
		switch tok[0] {
		case 'v':
			guides.V = append(guides.V, v)
		case 'h':
			guides.H = append(guides.H, v)
		}
	}
	return guides
}

// rulerTick is one mark on a ruler: its scene-mm position, and whether the
// ruler prints the number beside it.
type rulerTick struct {
	mm      float64
	labeled bool
}

// rulerTicks returns the marks covering 0..lengthMm inclusive, one every
// tickMm and a number every labelMm. Positions are i*tickMm rather than an
// accumulated sum so the 50 mm label lands exactly on the 50 mm tick whatever
// the form size. A non-positive step draws nothing instead of looping forever.
func rulerTicks(lengthMm, tickMm, labelMm float64) []rulerTick {
	if tickMm <= 0 || lengthMm < 0 {
		return nil
	}
	var ticks []rulerTick
	for i := 0; ; i++ {
		mm := float64(i) * tickMm
		if mm > lengthMm+1e-9 {
			break
		}
		ticks = append(ticks, rulerTick{
			mm:      mm,
			labeled: labelMm > 0 && math.Abs(math.Remainder(mm, labelMm)) < 1e-9,
		})
	}
	return ticks
}

// rulerZone identifies which ruler strip a view point falls in.
type rulerZone int

const (
	rulerNone rulerZone = iota
	rulerTop
	rulerLeft
)

// rulerZoneAt maps a view point to its ruler strip. The corner square where
// the two strips overlap belongs to neither: a drag started there would have
// no unambiguous axis.
func rulerZoneAt(x, y, thickness float64) rulerZone {
	if thickness <= 0 || x < 0 || y < 0 {
		return rulerNone
	}
	inTop, inLeft := y < thickness, x < thickness
	switch {
	case inTop && inLeft:
		return rulerNone
	case inTop:
		return rulerTop
	case inLeft:
		return rulerLeft
	}
	return rulerNone
}

// nearestGuideIndex returns the index of the guide closest to pos within tol,
// or -1 when none is that close.
func nearestGuideIndex(guides []float64, pos, tol float64) int {
	best, bestDist := -1, tol
	for i, g := range guides {
		if d := math.Abs(g - pos); d <= bestDist {
			best, bestDist = i, d
		}
	}
	return best
}

// snapOffsetToGuides returns the delta that moves the span [lo, hi] so its
// nearest edge — leading, trailing or centre, the same three computeAlignGuides
// matches on — lands exactly on the closest guide within tol. ok is false when
// no guide is in range. A span already sitting on a guide reports (0, true) so
// the caller keeps it there instead of falling back to the grid.
func snapOffsetToGuides(lo, hi float64, guides []float64, tol float64) (delta float64, ok bool) {
	best := tol
	for _, g := range guides {
		for _, edge := range [3]float64{lo, hi, (lo + hi) / 2} {
			d := g - edge
			if math.Abs(d) <= best {
				best, delta, ok = math.Abs(d), d, true
			}
		}
	}
	return
}

// clampGuide keeps a guide inside the form. A guide dropped past the edge
// would be clipped away with the rest of the off-page painting and live on as
// an invisible snap target.
func clampGuide(pos, length float64) float64 {
	if pos < 0 {
		return 0
	}
	if length > 0 && pos > length {
		return length
	}
	return pos
}

// removeGuideAt drops index i, leaving the input slice's backing array intact
// so the value-copied sceneGuides handed out by Guides() cannot be rewritten
// underneath its holder.
func removeGuideAt(guides []float64, i int) []float64 {
	if i < 0 || i >= len(guides) {
		return guides
	}
	return append(guides[:i:i], guides[i+1:]...)
}

// ---------------------------------------------------------------------------
// Scene-side state: guides belong to the design document
// ---------------------------------------------------------------------------

// Guides returns the design's user-placed guides.
func (this *GedScene) Guides() sceneGuides {
	return this.guides
}

// AddGuide records a guide at pos (scene mm), clamped into the form.
func (this *GedScene) AddGuide(vertical bool, pos float64) {
	w, h := this.Size()
	if vertical {
		this.guides.V = append(this.guides.V, clampGuide(pos, w))
		return
	}
	this.guides.H = append(this.guides.H, clampGuide(pos, h))
}

// RemoveGuide drops the guide at index on one axis. An out-of-range index is
// ignored so a stale drag cannot panic the canvas.
func (this *GedScene) RemoveGuide(vertical bool, index int) {
	if vertical {
		this.guides.V = removeGuideAt(this.guides.V, index)
		return
	}
	this.guides.H = removeGuideAt(this.guides.H, index)
}

// ---------------------------------------------------------------------------
// View-side state: visibility, the drag gesture and the painting
// ---------------------------------------------------------------------------

// guideDragState is the in-flight ruler/guide gesture. The dragged guide is
// taken off the scene when the drag starts, so creating and moving share one
// path: the drop either puts a guide back or — over a ruler — does not.
type guideDragState struct {
	active   bool
	vertical bool
	pos      float64 // scene mm
}

// SetShowGuides toggles the rulers and the manual guide lines together.
func (this *GedView) SetShowGuides(show bool) {
	this.showGuides = show
}

// IsShowGuides reports whether the rulers and guides are drawn.
func (this *GedView) IsShowGuides() bool {
	return this.showGuides
}

// activeGuides returns the guides that take part in painting and snapping —
// none while they are hidden, so a guide the user cannot see never pulls a
// widget.
func (this *GedView) activeGuides() sceneGuides {
	scene := this.GedScene()
	if !this.showGuides || scene == nil {
		return sceneGuides{}
	}
	return scene.Guides()
}

// beginGuideDrag starts a guide gesture at the view point (x, y): a press on a
// ruler pulls a new guide out of it, a press on an existing guide picks that
// one up. Reports whether the press was consumed, in which case the canvas
// tools must not see it.
func (this *GedView) beginGuideDrag(x, y float64) bool {
	scene := this.GedScene()
	if !this.showGuides || scene == nil {
		return false
	}

	sx, sy := this.MapToScene(x, y)
	switch rulerZoneAt(x, y, rulerThicknessPx) {
	case rulerLeft:
		this.guideDrag = guideDragState{active: true, vertical: true, pos: sx}
		this.Self().Update()
		return true
	case rulerTop:
		this.guideDrag = guideDragState{active: true, vertical: false, pos: sy}
		this.Self().Update()
		return true
	}

	// Only grab a guide over the form itself: outside it the guide is not
	// drawn, so a press there is about the canvas, not the guide.
	w, h := this.SceneSizeMm()
	if sx < 0 || sy < 0 || sx > w || sy > h {
		return false
	}
	tol := gui.PixelToMm(guideGrabPx) / this.ZoomFactor()
	guides := scene.Guides()
	if i := nearestGuideIndex(guides.V, sx, tol); i >= 0 {
		scene.RemoveGuide(true, i)
		this.guideDrag = guideDragState{active: true, vertical: true, pos: sx}
		this.Self().Update()
		return true
	}
	if i := nearestGuideIndex(guides.H, sy, tol); i >= 0 {
		scene.RemoveGuide(false, i)
		this.guideDrag = guideDragState{active: true, vertical: false, pos: sy}
		this.Self().Update()
		return true
	}
	return false
}

// updateGuideDrag tracks the pointer while a guide is in flight.
func (this *GedView) updateGuideDrag(x, y float64) bool {
	if !this.guideDrag.active {
		return false
	}
	sx, sy := this.MapToScene(x, y)
	if this.guideDrag.vertical {
		this.guideDrag.pos = sx
	} else {
		this.guideDrag.pos = sy
	}
	this.Self().Update()
	return true
}

// endGuideDrag drops the in-flight guide: back onto the canvas at the release
// point, or onto a ruler, which deletes it — the guide left the scene when the
// drag began, so deleting is simply not putting it back.
func (this *GedView) endGuideDrag(x, y float64) bool {
	if !this.guideDrag.active {
		return false
	}
	drag := this.guideDrag
	this.guideDrag = guideDragState{}

	if scene := this.GedScene(); scene != nil && rulerZoneAt(x, y, rulerThicknessPx) == rulerNone {
		sx, sy := this.MapToScene(x, y)
		pos := sy
		if drag.vertical {
			pos = sx
		}
		scene.AddGuide(drag.vertical, pos)
	}
	this.Self().Update()
	return true
}

// drawGuides paints the manual guides, plus the one being dragged, across the
// form. Called in scene-mm space inside the page clip, like drawGrid.
func (this *GedView) drawGuides(g paint.Painter, guides sceneGuides) {
	w, h := this.SceneSizeMm()

	// Teal hairline (0 width = 1px at any zoom) so a placed guide reads as a
	// different thing from the blue drag-time alignment lines.
	g.SetPen1(paint.Color{0, 160, 176, 200}, 0)
	for _, x := range guides.V {
		g.MoveTo(x, 0)
		g.LineTo(x, h)
	}
	for _, y := range guides.H {
		g.MoveTo(0, y)
		g.LineTo(w, y)
	}
	if this.guideDrag.active {
		if this.guideDrag.vertical {
			g.MoveTo(this.guideDrag.pos, 0)
			g.LineTo(this.guideDrag.pos, h)
		} else {
			g.MoveTo(0, this.guideDrag.pos)
			g.LineTo(w, this.guideDrag.pos)
		}
	}
	g.Stroke()
}

// drawRulers paints the top and left scales in widget pixels. Tick positions
// come from MapFromScene, so pan and zoom need no arithmetic of their own here.
func (this *GedView) drawRulers(g paint.Painter) {
	viewW, viewH := this.Size()
	sceneW, sceneH := this.SceneSizeMm()
	theme := gui.Theme()

	g.Save()
	defer g.Restore()

	g.SetBrush1(theme.FormColor)
	g.Rectangle(0, 0, viewW, rulerThicknessPx)
	g.Rectangle(0, 0, rulerThicknessPx, viewH)
	g.Fill()

	g.SetPen1(theme.BorderColor, 0)
	g.MoveTo(0, rulerThicknessPx)
	g.LineTo(viewW, rulerThicknessPx)
	g.MoveTo(rulerThicknessPx, 0)
	g.LineTo(rulerThicknessPx, viewH)
	g.Stroke()

	horz := rulerTicks(sceneW, rulerTickMm, rulerLabelMm)
	vert := rulerTicks(sceneH, rulerTickMm, rulerLabelMm)

	g.SetPen1(theme.TextColor, 0)
	for _, tick := range horz {
		x, _ := this.MapFromScene(tick.mm, 0)
		if x < rulerThicknessPx || x > viewW {
			continue
		}
		g.MoveTo(x, rulerThicknessPx-rulerTickLen(tick))
		g.LineTo(x, rulerThicknessPx)
	}
	for _, tick := range vert {
		_, y := this.MapFromScene(0, tick.mm)
		if y < rulerThicknessPx || y > viewH {
			continue
		}
		g.MoveTo(rulerThicknessPx-rulerTickLen(tick), y)
		g.LineTo(rulerThicknessPx, y)
	}
	g.Stroke()

	g.SetFont(paint.NewFont("", rulerLabelFontSize, false, false))
	g.SetBrush1(theme.TextColor)
	for _, tick := range horz {
		if !tick.labeled {
			continue
		}
		x, _ := this.MapFromScene(tick.mm, 0)
		if x < rulerThicknessPx || x > viewW {
			continue
		}
		g.DrawText1(x+2, rulerLabelFontSize, strconv.FormatFloat(tick.mm, 'f', 0, 64))
	}
	for _, tick := range vert {
		if !tick.labeled {
			continue
		}
		_, y := this.MapFromScene(0, tick.mm)
		if y < rulerThicknessPx || y > viewH {
			continue
		}
		g.DrawText1(1, y+rulerLabelFontSize, strconv.FormatFloat(tick.mm, 'f', 0, 64))
	}
}

// rulerTickLen is how far a tick reaches into the strip: numbered ticks are
// drawn longer.
func rulerTickLen(tick rulerTick) float64 {
	if tick.labeled {
		return rulerLabelTickPx
	}
	return rulerTickPx
}
