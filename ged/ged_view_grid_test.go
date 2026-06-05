package ged

import (
	"testing"
)

// TestSnapToGrid exercises the pure rounding helper: round-to-nearest-multiple,
// negatives, exact multiples staying put, and the step<=0 passthrough guard.
func TestSnapToGrid(t *testing.T) {
	cases := []struct {
		name string
		v    float64
		step float64
		want float64
	}{
		// Rounds to the nearest multiple (down, then up across the .5 line).
		{"round down", 11, 5, 10},
		{"round up", 13, 5, 15},
		{"halfway rounds up", 12.5, 5, 15}, // math.Round: ties away from zero
		{"just below half", 12.49, 5, 10},

		// Exact multiples must stay exactly put.
		{"exact multiple", 20, 5, 20},
		{"exact zero", 0, 5, 0},

		// Negatives round symmetrically.
		{"negative round toward zero", -11, 5, -10},
		{"negative round away", -13, 5, -15},
		{"negative exact", -15, 5, -15},

		// A non-positive step disables snapping and returns v untouched.
		{"zero step passthrough", 7.3, 0, 7.3},
		{"negative step passthrough", 7.3, -5, 7.3},

		// Fractional step works too (grid need not be an integer).
		{"fractional step", 7.4, 2.5, 7.5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := snapToGrid(tc.v, tc.step); got != tc.want {
				t.Errorf("snapToGrid(%g, %g) = %g, want %g", tc.v, tc.step, got, tc.want)
			}
		})
	}
}

// TestSnapToGridIsMultiple property-checks that every snapped value is an exact
// multiple of the step (for a positive step), across a spread of inputs.
func TestSnapToGridIsMultiple(t *testing.T) {
	const step = 5.0
	for _, v := range []float64{-23.7, -5, -0.1, 0, 0.1, 3.3, 7.5, 18.9, 100.4} {
		got := snapToGrid(v, step)
		if mod := got / step; mod != float64(int(mod)) {
			t.Errorf("snapToGrid(%g, %g) = %g is not a multiple of %g", v, step, got, step)
		}
	}
}

// TestSnapSelectionToGridMovesToMultiple drives the real post-drag snap path:
// a widget is parked at an off-grid position, the selection is snapped, and
// its top-left must land on the nearest grid intersection. Width/height stay
// untouched. Uses the same fake-widget harness as the align/nudge tests.
func TestSnapSelectionToGridMovesToMultiple(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	view.SetGridStep(5)
	view.SetSnapToGrid(true)

	// 13 -> 15, 22 -> 20 on a 5mm grid. Size is non-grid on purpose; it must
	// be preserved (snap only touches position).
	fake := addFakeAt(t, scene, "btn", 13, 22, 7, 3)
	view.Selection().Clear()
	view.Selection().Add(fake)

	view.snapSelectionToGrid()

	if x, y := fake.Pos(); x != 15 || y != 20 {
		t.Errorf("after snap: pos = (%g, %g), want (15, 20)", x, y)
	}
	if w, h := fake.Size(); w != 7 || h != 3 {
		t.Errorf("after snap: size = (%g, %g), want (7, 3) unchanged", w, h)
	}
}

// TestSnapSelectionToGridDisabledNoOp confirms snap is skipped entirely when
// SetSnapToGrid(false) — the widget keeps its off-grid position.
func TestSnapSelectionToGridDisabledNoOp(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	view.SetGridStep(5)
	view.SetSnapToGrid(false)

	fake := addFakeAt(t, scene, "btn", 13, 22, 7, 3)
	view.Selection().Clear()
	view.Selection().Add(fake)

	view.snapSelectionToGrid()

	if x, y := fake.Pos(); x != 13 || y != 22 {
		t.Errorf("snap disabled: pos = (%g, %g), want (13, 22) unchanged", x, y)
	}
}

// TestSnapSelectionToGridSkipsLocked confirms a position-locked widget is left
// alone even when snap is on, mirroring GenerateMoveCommand's lock handling.
func TestSnapSelectionToGridSkipsLocked(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	view.SetGridStep(5)
	view.SetSnapToGrid(true)

	fake := addFakeAt(t, scene, "btn", 13, 22, 7, 3)
	fake.SetLockPos(true)
	view.Selection().Clear()
	view.Selection().Add(fake)

	view.snapSelectionToGrid()

	if x, y := fake.Pos(); x != 13 || y != 22 {
		t.Errorf("locked widget moved: pos = (%g, %g), want (13, 22) unchanged", x, y)
	}
}

// TestGridToggleAccessors checks the new toggle/setter API round-trips state
// and that the documented defaults hold for a fresh view: snap ON (inherits
// the existing snapEnabled default), grid overlay OFF.
func TestGridToggleAccessors(t *testing.T) {
	view := NewGedView()

	// Documented defaults.
	if !view.IsSnapToGrid() {
		t.Errorf("default IsSnapToGrid() = false, want true")
	}
	if view.IsShowGrid() {
		t.Errorf("default IsShowGrid() = true, want false")
	}

	view.SetSnapToGrid(false)
	if view.IsSnapToGrid() {
		t.Errorf("after SetSnapToGrid(false), IsSnapToGrid() = true")
	}

	view.SetShowGrid(true)
	if !view.IsShowGrid() {
		t.Errorf("after SetShowGrid(true), IsShowGrid() = false")
	}

	view.SetGridStep(8)
	if view.GridSize() != 8 {
		t.Errorf("after SetGridStep(8), GridSize() = %g, want 8", view.GridSize())
	}

	// A non-positive step is rejected (guards the snap divide-by-zero path).
	view.SetGridStep(0)
	if view.GridSize() != 8 {
		t.Errorf("SetGridStep(0) changed GridSize() to %g, want 8 unchanged", view.GridSize())
	}
}
