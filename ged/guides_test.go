package ged

import (
	"encoding/json"
	"math"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
)

// guideView returns a laid-out canvas with a 200x150 mm form, so MapToScene /
// MapFromScene produce stable view coordinates without a window.
func guideView(t *testing.T) *GedView {
	t.Helper()
	view := NewGedView()
	view.GedScene().SetSize(200, 150)
	view.SetSize(600, 400)
	view.Layout()
	return view
}

// TestRulerTicks: 0..100 mm at a 10 mm tick / 50 mm label scale gives 11 ticks
// with numbers on 0, 50 and 100 only.
func TestRulerTicks(t *testing.T) {
	ticks := rulerTicks(100, 10, 50)
	if len(ticks) != 11 {
		t.Fatalf("rulerTicks(100, 10, 50) returned %d ticks, want 11: %+v", len(ticks), ticks)
	}
	for i, tick := range ticks {
		wantMm := float64(i) * 10
		if tick.mm != wantMm {
			t.Errorf("tick %d at %g mm, want %g", i, tick.mm, wantMm)
		}
		wantLabel := i%5 == 0
		if tick.labeled != wantLabel {
			t.Errorf("tick at %g mm labeled = %v, want %v", tick.mm, tick.labeled, wantLabel)
		}
	}
}

// TestRulerTicksPartialSpan: a form whose size is not a whole number of ticks
// stops at the last tick inside it — no tick is drawn past the form edge.
func TestRulerTicksPartialSpan(t *testing.T) {
	ticks := rulerTicks(34, 10, 50)
	if len(ticks) != 4 {
		t.Fatalf("rulerTicks(34, 10, 50) returned %d ticks, want 4 (0,10,20,30): %+v", len(ticks), ticks)
	}
	if last := ticks[len(ticks)-1].mm; last != 30 {
		t.Errorf("last tick at %g mm, want 30", last)
	}
}

// TestRulerTicksGuards: a non-positive step or a negative length draws nothing
// instead of looping forever.
func TestRulerTicksGuards(t *testing.T) {
	if got := rulerTicks(100, 0, 50); got != nil {
		t.Errorf("rulerTicks with a zero step returned %+v, want nil", got)
	}
	if got := rulerTicks(-5, 10, 50); got != nil {
		t.Errorf("rulerTicks with a negative length returned %+v, want nil", got)
	}
}

// TestRulerZoneAt covers the strip mapping, including the corner square that
// belongs to neither ruler.
func TestRulerZoneAt(t *testing.T) {
	cases := []struct {
		name string
		x, y float64
		want rulerZone
	}{
		{"top strip", 200, 5, rulerTop},
		{"left strip", 5, 200, rulerLeft},
		{"corner is neither", 5, 5, rulerNone},
		{"canvas", 200, 200, rulerNone},
		{"just past the top strip", 200, 18, rulerNone},
		{"just past the left strip", 18, 200, rulerNone},
		{"negative", -3, -3, rulerNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rulerZoneAt(tc.x, tc.y, 18); got != tc.want {
				t.Errorf("rulerZoneAt(%g, %g, 18) = %v, want %v", tc.x, tc.y, got, tc.want)
			}
		})
	}
}

// TestNearestGuideIndex picks the closest guide within tolerance and reports
// -1 when nothing is close enough.
func TestNearestGuideIndex(t *testing.T) {
	guides := []float64{10, 40, 41}
	if got := nearestGuideIndex(guides, 40.4, 1); got != 1 {
		t.Errorf("nearestGuideIndex(40.4) = %d, want 1", got)
	}
	if got := nearestGuideIndex(guides, 40.9, 1); got != 2 {
		t.Errorf("nearestGuideIndex(40.9) = %d, want 2", got)
	}
	if got := nearestGuideIndex(guides, 25, 1); got != -1 {
		t.Errorf("nearestGuideIndex(25) = %d, want -1", got)
	}
	if got := nearestGuideIndex(nil, 25, 1); got != -1 {
		t.Errorf("nearestGuideIndex on an empty set = %d, want -1", got)
	}
}

// TestSnapOffsetToGuides: the leading edge, trailing edge and centre all snap,
// a span already on a guide reports ok with a zero delta (so the caller does
// not fall back to the grid), and a far span reports nothing.
func TestSnapOffsetToGuides(t *testing.T) {
	guides := []float64{100}

	if d, ok := snapOffsetToGuides(101, 121, guides, 2); !ok || d != -1 {
		t.Errorf("leading edge: delta %g ok %v, want -1 true", d, ok)
	}
	if d, ok := snapOffsetToGuides(79, 99, guides, 2); !ok || d != 1 {
		t.Errorf("trailing edge: delta %g ok %v, want 1 true", d, ok)
	}
	if d, ok := snapOffsetToGuides(89, 109, guides, 2); !ok || d != 1 {
		t.Errorf("centre: delta %g ok %v, want 1 true", d, ok)
	}
	if d, ok := snapOffsetToGuides(100, 120, guides, 2); !ok || d != 0 {
		t.Errorf("already on the guide: delta %g ok %v, want 0 true", d, ok)
	}
	if d, ok := snapOffsetToGuides(200, 220, guides, 2); ok || d != 0 {
		t.Errorf("out of range: delta %g ok %v, want 0 false", d, ok)
	}
	if _, ok := snapOffsetToGuides(101, 121, nil, 2); ok {
		t.Error("no guides at all must not snap")
	}
}

// TestGuidesEncodeRoundTrip: the attribute encoding preserves both axes and
// their order; junk tokens are dropped instead of aborting the parse.
func TestGuidesEncodeRoundTrip(t *testing.T) {
	in := sceneGuides{V: []float64{10.5, 100}, H: []float64{40}}
	out := decodeGuides(encodeGuides(in))
	if len(out.V) != 2 || out.V[0] != 10.5 || out.V[1] != 100 {
		t.Errorf("vertical guides round-tripped to %v, want [10.5 100]", out.V)
	}
	if len(out.H) != 1 || out.H[0] != 40 {
		t.Errorf("horizontal guides round-tripped to %v, want [40]", out.H)
	}

	if got := encodeGuides(sceneGuides{}); got != "" {
		t.Errorf("empty guide set encoded to %q, want the empty string", got)
	}
	if got := decodeGuides("v:abc,,x:5,h:20"); len(got.V) != 0 || len(got.H) != 1 || got.H[0] != 20 {
		t.Errorf("decodeGuides on junk = %+v, want only h=[20]", got)
	}
}

// TestGedSceneGuidesRoundTrip is the persistence requirement: guides saved
// with the design come back identical through a real .silkui file.
func TestGedSceneGuidesRoundTrip(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	scene.AddGuide(true, 25.5)
	scene.AddGuide(false, 40)
	scene.AddGuide(true, 100)

	path := filepath.Join(t.TempDir(), "guides.silkui")
	if err := scene.SaveDesign().SaveFile(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	doc, err := core.LoadTDocFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded := NewGedScene()
	if err := loaded.LoadDesign(doc); err != nil {
		t.Fatalf("LoadDesign: %v", err)
	}

	want, got := scene.Guides(), loaded.Guides()
	if len(got.V) != len(want.V) || len(got.H) != len(want.H) {
		t.Fatalf("guides after reload = %+v, want %+v", got, want)
	}
	for i := range want.V {
		if got.V[i] != want.V[i] {
			t.Errorf("vertical guide %d = %g, want %g", i, got.V[i], want.V[i])
		}
	}
	for i := range want.H {
		if got.H[i] != want.H[i] {
			t.Errorf("horizontal guide %d = %g, want %g", i, got.H[i], want.H[i])
		}
	}
}

// TestGedSceneAddGuideClamps: a guide dropped past the form edge is clamped
// into it, so it can never become an invisible (clipped away) snap target.
func TestGedSceneAddGuideClamps(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	scene.AddGuide(true, 500)
	scene.AddGuide(false, -20)

	g := scene.Guides()
	if len(g.V) != 1 || g.V[0] != 200 {
		t.Errorf("vertical guides = %v, want [200]", g.V)
	}
	if len(g.H) != 1 || g.H[0] != 0 {
		t.Errorf("horizontal guides = %v, want [0]", g.H)
	}
}

// TestComputeAlignGuidesManualGuide: a manual guide is another target in the
// same guide computation as siblings and the page centre.
func TestComputeAlignGuidesManualGuide(t *testing.T) {
	manual := sceneGuides{V: []float64{100}, H: []float64{250}}

	dragged := geom.Rect{X: 101, Y: 500, Width: 30, Height: 10}
	gs := computeAlignGuides(dragged, nil, guideCanvas(), manual, 5)
	if !hasVGuide(gs, 100) {
		t.Fatalf("expected a vertical guide at the manual guide x=100, got %+v", gs)
	}

	dragged = geom.Rect{X: 600, Y: 248, Width: 30, Height: 10}
	gs = computeAlignGuides(dragged, nil, guideCanvas(), manual, 5)
	if !hasHGuide(gs, 250) {
		t.Fatalf("expected a horizontal guide at the manual guide y=250, got %+v", gs)
	}

	dragged = geom.Rect{X: 600, Y: 500, Width: 30, Height: 10}
	if gs = computeAlignGuides(dragged, nil, guideCanvas(), manual, 5); len(gs) != 0 {
		t.Errorf("a rect far from every guide must produce none, got %+v", gs)
	}
}

// TestSnapSelectionGuideBeatsGrid: dropping a widget next to a manual guide
// parks it on the guide, not on the nearest grid line.
func TestSnapSelectionGuideBeatsGrid(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	view.SetGridStep(5)
	view.SetSnapToGrid(true)
	scene.AddGuide(true, 62)

	fake := addFakeAt(t, scene, "btn", 61.5, 30, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(fake)

	view.snapSelectionToGrid()

	if x, y := fake.Pos(); x != 62 || y != 30 {
		t.Errorf("after snap: pos = (%g, %g), want (62, 30) — the guide must win over the 5 mm grid", x, y)
	}
}

// TestSnapSelectionHiddenGuidesIgnored: with 显示参考线 off the guides are gone
// for snapping too, so the grid takes the widget back.
func TestSnapSelectionHiddenGuidesIgnored(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	view.SetGridStep(5)
	view.SetSnapToGrid(true)
	view.SetShowGuides(false)
	scene.AddGuide(true, 62)

	fake := addFakeAt(t, scene, "btn", 61.5, 30, 10, 4)
	view.Selection().Clear()
	view.Selection().Add(fake)

	view.snapSelectionToGrid()

	if x, _ := fake.Pos(); x != 60 {
		t.Errorf("after snap with guides hidden: x = %g, want 60 (grid)", x)
	}
}

// TestShowGuidesDefaultOn: rulers and guides are on for a fresh canvas.
func TestShowGuidesDefaultOn(t *testing.T) {
	view := NewGedView()
	if !view.IsShowGuides() {
		t.Fatal("IsShowGuides() = false on a new canvas, want true")
	}
	view.SetShowGuides(false)
	if view.IsShowGuides() {
		t.Error("SetShowGuides(false) did not stick")
	}
}

// TestGuideDragFromRulerCreates: pressing the left ruler and releasing on the
// canvas leaves a vertical guide at the release point; the top ruler yields a
// horizontal one.
func TestGuideDragFromRulerCreates(t *testing.T) {
	view := guideView(t)
	scene := view.GedScene()

	if !view.beginGuideDrag(5, 200) {
		t.Fatal("a press on the left ruler was not consumed")
	}
	vx, vy := view.MapFromScene(60, 40)
	view.updateGuideDrag(vx, vy)
	if !view.endGuideDrag(vx, vy) {
		t.Fatal("the release was not consumed")
	}
	if g := scene.Guides(); len(g.V) != 1 || math.Abs(g.V[0]-60) > 1e-6 {
		t.Fatalf("vertical guides = %v, want one at 60", g.V)
	}

	if !view.beginGuideDrag(200, 5) {
		t.Fatal("a press on the top ruler was not consumed")
	}
	view.endGuideDrag(vx, vy)
	if g := scene.Guides(); len(g.H) != 1 || math.Abs(g.H[0]-40) > 1e-6 {
		t.Fatalf("horizontal guides = %v, want one at 40", g.H)
	}
}

// TestGuideDragMovesExisting: grabbing a guide and releasing elsewhere moves
// it rather than adding a second one.
func TestGuideDragMovesExisting(t *testing.T) {
	view := guideView(t)
	scene := view.GedScene()
	scene.AddGuide(true, 60)

	vx, vy := view.MapFromScene(60, 40)
	if !view.beginGuideDrag(vx, vy) {
		t.Fatal("a press on an existing guide was not consumed")
	}
	nx, ny := view.MapFromScene(90, 40)
	view.updateGuideDrag(nx, ny)
	view.endGuideDrag(nx, ny)

	if g := scene.Guides(); len(g.V) != 1 || math.Abs(g.V[0]-90) > 1e-6 {
		t.Fatalf("vertical guides = %v, want exactly one at 90", g.V)
	}
}

// TestGuideDragToRulerDeletes: dragging a guide back over its ruler removes it.
func TestGuideDragToRulerDeletes(t *testing.T) {
	view := guideView(t)
	scene := view.GedScene()
	scene.AddGuide(true, 60)

	vx, vy := view.MapFromScene(60, 40)
	view.beginGuideDrag(vx, vy)
	view.updateGuideDrag(5, 200)
	view.endGuideDrag(5, 200)

	if g := scene.Guides(); len(g.V) != 0 {
		t.Fatalf("vertical guides = %v, want none after the drop on the ruler", g.V)
	}
}

// TestPageClearsTheRulers: a page too tall for the viewport rests below the
// top ruler instead of under it — otherwise the widgets in the form's
// top-left corner could never be clicked.
func TestPageClearsTheRulers(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(200, 1500)
	view.SetSize(600, 400)
	view.Layout()

	x, y := view.PageOriginPx()
	if x < rulerThicknessPx || y < rulerThicknessPx {
		t.Errorf("page origin = (%g, %g), want both at least %g so the rulers do not cover it",
			x, y, rulerThicknessPx)
	}
}

// TestGuideDragIgnoredWhenHidden: with 显示参考线 off there is no ruler to grab,
// so the press falls through to the canvas tools.
func TestGuideDragIgnoredWhenHidden(t *testing.T) {
	view := guideView(t)
	view.SetShowGuides(false)
	if view.beginGuideDrag(5, 200) {
		t.Error("a ruler press was consumed while the rulers are hidden")
	}
}

// TestSessionStateGuidesFlag: the 视图 toggle rides along in the designer
// session, and a session written before the flag existed still means "on".
func TestSessionStateGuidesFlag(t *testing.T) {
	data, err := json.Marshal(SessionState{HideGuides: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded SessionState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.HideGuides {
		t.Error("HideGuides did not survive the session round trip")
	}

	var legacy SessionState
	if err := json.Unmarshal([]byte(`{"last_mode":1}`), &legacy); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if legacy.HideGuides {
		t.Error("a session without the flag must leave the guides shown")
	}
}
