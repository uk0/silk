package ged

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/gui"
)

// TestDefaultGridModelMatchesTodaysCanvas pins the defaults to the literals the
// designer used before the model existed: a 5mm minor grid, always painted,
// snapping on. A change here silently redraws every existing design.
func TestDefaultGridModelMatchesTodaysCanvas(t *testing.T) {
	g := defaultGridModel()
	if g.Pitch != 5 || !g.Visible || !g.Snap {
		t.Errorf("defaultGridModel() = %+v, want {Pitch:5 Visible:true Snap:true}", g)
	}
}

// TestClampGridPitch covers the [1, 50] mm bound, including the boundaries
// themselves and the garbage a hand-edited document can carry.
func TestClampGridPitch(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{5, 5},
		{1, 1},
		{50, 50},
		{0.5, 1},
		{0, 1},
		{-3, 1},
		{51, 50},
		{1e6, 50},
	}
	for _, tc := range cases {
		if got := clampGridPitch(tc.in); got != tc.want {
			t.Errorf("clampGridPitch(%g) = %g, want %g", tc.in, got, tc.want)
		}
	}
}

// TestClampFormSize covers the 10mm floor. There is deliberately no ceiling —
// a design may target a page larger than any screen.
func TestClampFormSize(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{100, 100},
		{10, 10},
		{9.9, 10},
		{0, 10},
		{-40, 10},
		{5000, 5000},
	}
	for _, tc := range cases {
		if got := clampFormSize(tc.in); got != tc.want {
			t.Errorf("clampFormSize(%g) = %g, want %g", tc.in, got, tc.want)
		}
	}
}

// TestGridTiers checks the two-tier rule the canvas paints: minor lines at the
// pitch, major lines every majorGridRatio cells, nothing at all when the grid
// is switched off or carries a degenerate pitch.
func TestGridTiers(t *testing.T) {
	minor, major, paint := gridTiers(GridModel{Pitch: 5, Visible: true})
	if minor != 5 || major != 50 || !paint {
		t.Errorf("gridTiers(5mm visible) = (%g, %g, %v), want (5, 50, true)", minor, major, paint)
	}

	minor, major, paint = gridTiers(GridModel{Pitch: 8, Visible: true})
	if minor != 8 || major != 80 || !paint {
		t.Errorf("gridTiers(8mm visible) = (%g, %g, %v), want (8, 80, true)", minor, major, paint)
	}

	if _, _, paint = gridTiers(GridModel{Pitch: 5, Visible: false}); paint {
		t.Errorf("gridTiers(hidden) painted")
	}

	// A zero pitch would make the paint loop spin forever.
	if _, _, paint = gridTiers(GridModel{Pitch: 0, Visible: true}); paint {
		t.Errorf("gridTiers(0mm) painted")
	}
}

// TestParseMm covers the reject rule the dialog inherits from the property
// sheet: anything that is not a number is refused so the caller keeps the old
// value, rather than being coerced to zero.
func TestParseMm(t *testing.T) {
	cases := []struct {
		in      string
		want    float64
		wantOk  bool
		comment string
	}{
		{"12", 12, true, "integer"},
		{"12.5", 12.5, true, "decimal"},
		{"  12  ", 12, true, "surrounding spaces are a typing artefact"},
		{"-3", -3, true, "sign parses; the clamp decides what to do with it"},
		{"", 0, false, "empty"},
		{"abc", 0, false, "not a number"},
		{"12mm", 0, false, "trailing unit"},
		{"1,5", 0, false, "comma decimal"},
	}
	for _, tc := range cases {
		got, ok := parseMm(tc.in)
		if got != tc.want || ok != tc.wantOk {
			t.Errorf("parseMm(%q) = (%g, %v), want (%g, %v) — %s",
				tc.in, got, ok, tc.want, tc.wantOk, tc.comment)
		}
	}
}

// TestGridPitchClampedOnLoad: a hand-edited or corrupted document must not be
// able to install a pitch outside the legal range, because a zero pitch spins
// the canvas paint loop and snapping divides by it.
func TestGridPitchClampedOnLoad(t *testing.T) {
	doc := core.NewTDoc()
	doc.SetValue("form")
	doc.WriteAttr("bounds", geom.Rect{Width: 100, Height: 100})
	doc.WriteAttr("grid_pitch", 0.0)

	scene := NewGedScene()
	if err := scene.LoadDesign(doc); err != nil {
		t.Fatal("LoadDesign failed:", err)
	}
	if got := scene.Grid().Pitch; got != minGridPitch {
		t.Errorf("pitch after loading 0 = %g, want %g", got, minGridPitch)
	}
}

// TestCoarseNudgeStepFollowsGrid: Shift+arrow is the "one grid cell" jump, so
// it must track the configured pitch. It used to be a fixed 10mm constant that
// ignored the grid entirely.
func TestCoarseNudgeStepFollowsGrid(t *testing.T) {
	view := NewGedView()
	if got := view.coarseNudgeStep(); got != defaultGridPitch {
		t.Errorf("default coarseNudgeStep() = %g, want %g", got, defaultGridPitch)
	}
	view.SetGridStep(12)
	if got := view.coarseNudgeStep(); got != 12 {
		t.Errorf("coarseNudgeStep() after SetGridStep(12) = %g, want 12", got)
	}
}

// TestGridSurvivesSaveLoad locks the grid model to the design document. Before
// the model existed the pitch/snap flags lived on GedView and were never
// written to the TDoc, so reopening a file silently reverted the designer's
// grid to 5mm/snap-on.
func TestGridSurvivesSaveLoad(t *testing.T) {
	view := NewGedView()
	view.SetGridStep(12)
	view.SetSnapToGrid(false)
	view.GedScene().SetGrid(GridModel{Pitch: 12, Visible: false, Snap: false})

	doc := view.GedScene().SaveDesign()

	view2 := NewGedView()
	if err := view2.GedScene().LoadDesign(doc); err != nil {
		t.Fatal("LoadDesign failed:", err)
	}
	if got := view2.GridSize(); got != 12 {
		t.Errorf("pitch after round-trip = %g, want 12", got)
	}
	if view2.IsSnapToGrid() {
		t.Errorf("snap after round-trip = true, want false")
	}
	if view2.GedScene().Grid().Visible {
		t.Errorf("visible after round-trip = true, want false")
	}
}

// TestLoadWithoutGridAttrsKeepsDefaults covers documents written before the
// grid attrs shipped: absent attrs must land on the values the designer has
// always used (5mm, visible, snapping), not on Go zero values and not on
// whatever the previous design left behind.
func TestLoadWithoutGridAttrsKeepsDefaults(t *testing.T) {
	doc := core.NewTDoc()
	doc.SetValue("form")
	doc.WriteAttr("bounds", geom.Rect{Width: 100, Height: 100})
	doc.WriteAttr("title", "Legacy")

	view := NewGedView()
	view.SetGridStep(12)
	view.SetSnapToGrid(false)

	if err := view.GedScene().LoadDesign(doc); err != nil {
		t.Fatal("LoadDesign failed:", err)
	}
	if got := view.GridSize(); got != 5 {
		t.Errorf("pitch after legacy load = %g, want 5", got)
	}
	if !view.IsSnapToGrid() {
		t.Errorf("snap after legacy load = false, want true")
	}
}

// TestGridAttrsInvisibleToRuntimeLoader: SaveDesign writes the grid attrs into
// the same .silkui root gui.LoadFormFromDoc reads at runtime, and that loader
// materialises every root child it recognises. A grid attr parked under a key
// the loader claims would become a phantom widget on the running form.
func TestGridAttrsInvisibleToRuntimeLoader(t *testing.T) {
	scene := NewGedScene()
	scene.SetGrid(GridModel{Pitch: 8, Visible: false, Snap: false})

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal("failed to create button:", err)
	}
	btn.SetWidgetName("ok")
	btn.SetBounds(10, 10, 30, 8)
	btn.SetParent(scene)

	form, err := gui.LoadFormFromDoc(scene.SaveDesign())
	if err != nil {
		t.Fatal("LoadFormFromDoc failed:", err)
	}
	if n := len(form.Children()); n != 1 {
		t.Errorf("runtime form has %d children, want 1", n)
	}
}

// TestGridIsPerScene: the model belongs to the design, so swapping in a fresh
// scene (silkide's "new design" path) must hand back a fresh grid rather than
// carrying the previous document's pitch over.
func TestGridIsPerScene(t *testing.T) {
	view := NewGedView()
	view.SetGridStep(12)

	view.SetScene(NewGedScene())

	if got := view.GridSize(); got != 5 {
		t.Errorf("pitch after new scene = %g, want 5", got)
	}
}
