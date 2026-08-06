package ged

import (
	"strconv"
	"strings"
)

// The designer grid used to be three unrelated literals: a 5mm/50mm pair
// hard-coded inside GedScene.DrawSelf, a 5mm step on GedView that only the
// snap paths read, and a 10mm constant that only the Shift+arrow nudge read.
// GridModel is the one value all of them now read; it lives on GedScene so it
// travels with the design document instead of resetting on every reopen.
type GridModel struct {
	Pitch   float64 // spacing between grid lines, mm
	Visible bool    // paint the grid on the canvas
	Snap    bool    // round drags/drops/pastes onto the grid
}

const (
	// defaultGridPitch is what a new scene starts with and what a document
	// saved before the grid attrs existed loads as. It is the literal the
	// canvas minor grid and the snap rounding both used before the model,
	// so old documents keep their exact appearance.
	defaultGridPitch = 5.0

	// minGridPitch / maxGridPitch bound the pitch. Below 1mm the canvas
	// paints a solid smear and snapping stops being a constraint; above
	// 50mm the grid is too coarse to align anything against.
	minGridPitch = 1.0
	maxGridPitch = 50.0

	// majorGridRatio is how many minor cells the darker "anchor" line tier
	// spans. 10 keeps the default canvas at the 5mm/50mm pair it has always
	// painted, and gives the eye an every-ten-squares count at any pitch.
	majorGridRatio = 10

	// minFormSize is the floor (mm) for the form's own width and height. A
	// form smaller than this cannot hold a single widget and makes the
	// canvas unusable.
	minFormSize = 10.0
)

// defaultGridModel returns the grid a fresh scene starts with.
func defaultGridModel() GridModel {
	return GridModel{Pitch: defaultGridPitch, Visible: true, Snap: true}
}

// clampGridPitch pins v into [minGridPitch, maxGridPitch].
func clampGridPitch(v float64) float64 {
	if v < minGridPitch {
		return minGridPitch
	}
	if v > maxGridPitch {
		return maxGridPitch
	}
	return v
}

// clampFormSize floors v at minFormSize. There is no upper bound: a design may
// legitimately target a page far larger than any screen.
func clampFormSize(v float64) float64 {
	if v < minFormSize {
		return minFormSize
	}
	return v
}

// gridTiers returns the two line pitches GedScene.DrawSelf paints and whether
// it should paint at all. Pure so the two-tier rule is testable without a
// painter.
func gridTiers(g GridModel) (minor, major float64, paint bool) {
	if !g.Visible || g.Pitch <= 0 {
		return 0, 0, false
	}
	return g.Pitch, g.Pitch * majorGridRatio, true
}

// parseMm parses a millimetre field typed into the form settings dialog.
// Garbage is rejected (ok=false) rather than coerced, so the caller can keep
// the previous value — the property sheet behaves the same way: a numeric
// property whose scanner fails is left untouched (prop.parsePropertyValue).
func parseMm(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
