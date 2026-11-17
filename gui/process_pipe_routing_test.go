package gui

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
)

func processRoutingFixture() hmi.Project {
	return hmi.Project{Version: hmi.Version, Title: "Pipe avoidance", Width: 600, Height: 300,
		Points: []hmi.Point{{Definition: point.Definition{Name: "RUN", Kind: point.Bool}, Initial: true}, {Definition: point.Definition{Name: "X", Kind: point.Float}, Initial: float64(220)}, {Definition: point.Definition{Name: "VIS", Kind: point.Bool}, Initial: true}, {Definition: point.Definition{Name: "ROT", Kind: point.Float}, Initial: float64(0)}, {Definition: point.Definition{Name: "EN", Kind: point.Bool}, Initial: true}},
		Nodes:  []hmi.Node{{ID: "a", Kind: "value", X: 40, Y: 100, Width: 80, Height: 60}, {ID: "b", Kind: "value", X: 400, Y: 100, Width: 80, Height: 60}, {ID: "blocker", Kind: "pump", X: 220, Y: 70, Width: 80, Height: 120, Tag: "RUN", Bindings: []hmi.PropertyBinding{{Tag: "X", Property: "x"}, {Tag: "VIS", Property: "visible"}, {Tag: "ROT", Property: "rotation"}, {Tag: "EN", Property: "enabled"}}}},
		Links:  []hmi.Link{{ID: "line", From: hmi.Endpoint{Node: "a", Port: "east"}, To: hmi.Endpoint{Node: "b", Port: "west"}, Tag: "RUN", Routing: hmi.RoutingAvoid}}}
}

func newRoutingDiagram(t *testing.T) *ProcessDiagram {
	t.Helper()
	v := NewProcessDiagram()
	t.Cleanup(v.Close)
	if e := v.SetProject(processRoutingFixture()); e != nil {
		t.Fatal(e)
	}
	v.SetSize(600, 300)
	v.SetEditing(true)
	return v
}

func routingResult(t *testing.T, v *ProcessDiagram) hmi.PipeRouteResult {
	t.Helper()
	r, e := v.PipeRoute("line")
	if e != nil || r.Status != hmi.PipeRouteOK {
		t.Fatal(r, e)
	}
	return r
}

func TestProcessPipeRoutingSharedGeometryPickingAndCache(t *testing.T) {
	v := newRoutingDiagram(t)
	before := v.ProjectJSON()
	r := routingResult(t, v)
	if len(r.Points) < 4 || len(v.PipeRouteDiagnostics()) != 0 || v.pipeRouteBuilds != 1 {
		t.Fatal("avoidance not admitted", r, v.pipeRouteBuilds)
	}
	l := v.project.Links[0]
	cached := v.pipePoints(l)
	work := v.pipeRouter.WorkUsed()
	if n := testing.AllocsPerRun(100, func() { v.pipePoints(l) }); n != 0 {
		t.Fatal("cached geometry allocates", n)
	}
	v.applySample("RUN", core.Value{Raw: false, Quality: core.QualityGood})
	v.SelectNodes([]string{"a"})
	v.SetZoom(1.2)
	v.panX = 3
	v.panY = 7
	if v.pipeRouteBuilds != 1 || &v.pipePoints(l)[0] != &cached[0] || v.pipeRouter.WorkUsed() != work {
		t.Fatal("color/camera/selection rebuilt geometry")
	}
	v.SetZoom(1)
	v.panX = 0
	v.panY = 0
	v.SelectNodes(nil)
	// Click the long horizontal avoidance segment, not the old body-crossing line.
	pick := hmi.XY{}
	longest := 0.
	for i := 1; i < len(r.Points); i++ {
		a, b := r.Points[i-1], r.Points[i]
		if a.Y == b.Y && math.Abs(a.X-b.X) > longest {
			longest = math.Abs(a.X - b.X)
			pick = hmi.XY{X: (a.X + b.X) / 2, Y: a.Y}
		}
	}
	v.pointerDown(pick.X, pick.Y, processModifiers{})
	if v.SelectedLink() != "line" {
		t.Fatal("picking differs from route", pick, v.SelectedLink())
	}
	v.pointerDown(190, 130, processModifiers{})
	v.OnPointerCancel()
	if v.SelectedLink() != "" {
		t.Fatal("invisible old dogleg remained pickable")
	}
	if v.ProjectJSON() != before || len(v.undo) != 0 {
		t.Fatal("route/picking entered authored history")
	}
	r.Points[0].X = -999
	if v.pipePoints(l)[0].X < 0 {
		t.Fatal("public result aliases cache")
	}
	if _, e := v.PipeRoute("missing"); e == nil {
		t.Fatal("unknown pipe accepted")
	}
}

func TestProcessPipeRoutingLiveGeometryQualityAndAuthoringBoundary(t *testing.T) {
	v := newRoutingDiagram(t)
	authored := v.ProjectJSON()
	initial := routingResult(t, v)
	v.SetEditing(false)
	r := routingResult(t, v)
	builds := v.pipeRouteBuilds
	v.applySample("X", core.Value{Raw: float64(190), Quality: core.QualityBad})
	v.applySample("EN", core.Value{Raw: false, Quality: core.QualityGood})
	routingResult(t, v)
	if v.pipeRouteBuilds != builds {
		t.Fatal("bad geometry sample or enabled-only update invalidated route")
	}
	v.applySample("VIS", core.Value{Raw: false, Quality: core.QualityGood})
	clear := routingResult(t, v)
	if v.pipeRouteBuilds != builds+1 || reflect.DeepEqual(r.Points, clear.Points) {
		t.Fatal("hidden obstacle stayed cached", r, clear)
	}
	v.applySample("VIS", core.Value{Raw: true, Quality: core.QualityGood})
	v.applySample("ROT", core.Value{Raw: float64(90), Quality: core.QualityGood})
	routingResult(t, v)
	if v.pipeRouteBuilds != builds+2 || v.routingAnchor(v.views["blocker"], "east").Direction != "south" {
		t.Fatal("rotated geometry/direction not updated")
	}
	v.applySample("X", core.Value{Raw: float64(170), Quality: core.QualityGood})
	routingResult(t, v)
	if v.pipeRouteBuilds != builds+3 {
		t.Fatal("live position stayed cached")
	}
	v.SetEditing(true)
	restored := routingResult(t, v)
	if !reflect.DeepEqual(restored.Points, initial.Points) || v.ProjectJSON() != authored || len(v.undo) != 0 {
		t.Fatal("live geometry leaked into authoring", restored)
	}
	v.SetEditing(false)
	routingResult(t, v)
	if v.views["blocker"].node.X != 170 || v.views["blocker"].rotation != 90 {
		t.Fatal("current live geometry not reapplied")
	}
	for _, angle := range []float64{-450, -90, 0, 44, 90, 180, 270, 450} {
		n := v.views["blocker"]
		n.rotation = angle
		a := v.routingAnchor(n, "east")
		if a.Point != v.port(n, "east") {
			t.Fatal("port position quantized", angle, a)
		}
		i := int(math.Round(angle/90)) % 4
		if i < 0 {
			i += 4
		}
		if a.Direction != []string{"east", "south", "west", "north"}[i] {
			t.Fatal("wrong rotated normal", angle, a)
		}
	}
}

func TestProcessPipeRoutingDragPreviewCancelAndWaypoints(t *testing.T) {
	v := newRoutingDiagram(t)
	before := v.ProjectJSON()
	original := routingResult(t, v)
	v.SelectNodes([]string{"blocker"})
	v.pointerDown(260, 130, processModifiers{})
	v.pointerMove(260, 230, processModifiers{})
	moved := routingResult(t, v)
	if reflect.DeepEqual(original.Points, moved.Points) || v.ProjectJSON() != before {
		t.Fatal("drag route stale or preview persisted")
	}
	builds := v.pipeRouteBuilds
	v.pointerMove(260, 231, processModifiers{})
	routingResult(t, v)
	if v.pipeRouteBuilds != builds {
		t.Fatal("unchanged snapped position rebuilt routes")
	}
	v.OnPointerCancel()
	restored := routingResult(t, v)
	if !reflect.DeepEqual(original.Points, restored.Points) || v.ProjectJSON() != before || len(v.undo) != 0 {
		t.Fatal("canceled geometry remained cached")
	}
	v.selectPipe("line")
	if e := v.AddSelectedPipeBend(); e != nil {
		t.Fatal(e)
	}
	p := v.Project()
	bend := p.Links[0].Bends[0]
	onRoute := false
	for i := 1; i < len(original.Points); i++ {
		a, b := original.Points[i-1], original.Points[i]
		if bend == (hmi.XY{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}) {
			onRoute = true
		}
	}
	if !onRoute {
		t.Fatal("waypoint inserted on old invisible dogleg", bend, original.Points)
	}
	afterAdd := v.ProjectJSON()
	v.pointerDown(bend.X, bend.Y, processModifiers{})
	v.pointerMove(260, 130, processModifiers{})
	bad, e := v.PipeRoute("line")
	if e != nil || bad.Status != hmi.PipeRouteBlocked || len(v.PipeRouteDiagnostics()) != 1 || v.ProjectJSON() != afterAdd {
		t.Fatal("obstructed preview has no honest diagnostic", bad, e)
	}
	v.OnPointerCancel()
	routingResult(t, v)
	if v.ProjectJSON() != afterAdd || len(v.undo) != 1 {
		t.Fatal("canceled waypoint persisted")
	}
	v.pointerDown(bend.X, bend.Y, processModifiers{})
	v.pointerMove(190, 30, processModifiers{})
	v.pointerUp(190, 30, processModifiers{})
	if v.Project().Links[0].Bends[0] != (hmi.XY{X: 190, Y: 30}) || len(v.undo) != 2 {
		t.Fatal("waypoint commit missing")
	}
	routingResult(t, v)
	v.Undo()
	r := routingResult(t, v)
	if v.ProjectJSON() != afterAdd || slices.Index(r.Points, bend) < 0 {
		t.Fatal("undo did not restore geometry")
	}
	v.Redo()
	routingResult(t, v)
}

func TestProcessPipeRoutingLargeSnapshotHasDeterministicBudgetPriority(t *testing.T) {
	p := processRoutingFixture()
	p.Width = 6000
	p.Height = 6000
	for i := 0; i < 509; i++ {
		p.Nodes = append(p.Nodes, hmi.Node{ID: fmt.Sprintf("far-%03d", i), Kind: "text", X: 1000 + float64(i%24)*120, Y: 1000 + float64(i/24)*120, Width: 30, Height: 30})
	}
	link := p.Links[0]
	p.Links = nil
	for i := 0; i < 1024; i++ {
		l := link
		l.ID = fmt.Sprintf("pipe-%04d", i)
		p.Links = append(p.Links, l)
	}
	makeDiagram := func() *ProcessDiagram {
		v := NewProcessDiagram()
		t.Cleanup(v.Close)
		if e := v.SetProject(p); e != nil {
			t.Fatal(e)
		}
		return v
	}
	a, b := makeDiagram(), makeDiagram()
	if _, e := a.PipeRoute(p.Links[1023].ID); e != nil {
		t.Fatal(e)
	}
	if _, e := b.PipeRoute(p.Links[0].ID); e != nil {
		t.Fatal(e)
	}
	if a.pipeRouter.WorkUsed() != 200000 || b.pipeRouter.WorkUsed() != 200000 || a.pipeRouteBuilds != 1 || b.pipeRouteBuilds != 1 || len(a.PipeRouteDiagnostics()) == 0 {
		t.Fatal("large snapshot is unbounded or hides fallback")
	}
	for _, l := range p.Links {
		if !reflect.DeepEqual(a.currentPipeRoute(l), b.currentPipeRoute(l)) {
			t.Fatal("observer order changed budget priority", l.ID)
		}
	}
}

func TestProcessPipeRoutingRowsLegacyAndFallback(t *testing.T) {
	e := NewProcessEditor()
	defer e.Close()
	p := processRoutingFixture()
	if err := e.Diagram.SetProject(p); err != nil {
		t.Fatal(err)
	}
	oldRows := [][]string{{"line", "a", "east", "b", "west", "RUN"}}
	if err := e.ApplyLinkRows(oldRows); err != nil || e.Diagram.Project().Links[0].Routing != hmi.RoutingAvoid {
		t.Fatal("old SDK row update erased mode", err)
	}
	rows := [][]string{append(slices.Clone(oldRows[0]), hmi.RoutingOrthogonal)}
	if err := e.ApplyLinkRows(rows); err != nil {
		t.Fatal(err)
	}
	r := routingResult(t, e.Diagram)
	want := hmi.Route(hmi.Port(p.Nodes[0], "east"), hmi.Port(p.Nodes[1], "west"), nil)
	if !reflect.DeepEqual(r.Points, want) {
		t.Fatal("explicit legacy geometry changed", r, want)
	}
	authored := e.ProjectJSON()
	rows[0][6] = "bad"
	if err := e.ApplyLinkRows(rows); err == nil || e.ProjectJSON() != authored {
		t.Fatal("invalid routing edit committed")
	}
	// A blocked explicit waypoint retains its authored data and renders a fallback.
	p.Links[0].Bends = []hmi.XY{{X: 250, Y: 100}}
	if err := e.Diagram.SetProject(p); err != nil {
		t.Fatal(err)
	}
	r, err := e.Diagram.PipeRoute("line")
	if err != nil || r.Status != hmi.PipeRouteBlocked || len(e.Diagram.PipeRouteDiagnostics()) != 1 {
		t.Fatal(r, err)
	}
	e.Diagram.Close()
	if _, err := e.Diagram.PipeRoute("line"); err == nil || e.Diagram.PipeRouteDiagnostics() != nil {
		t.Fatal("retired routing observed")
	}
}

func BenchmarkProcessPipeRoutingCached(b *testing.B) {
	v := NewProcessDiagram()
	defer v.Close()
	if e := v.SetProject(processRoutingFixture()); e != nil {
		b.Fatal(e)
	}
	l := v.project.Links[0]
	v.pipePoints(l)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		v.pipePoints(l)
	}
}

func TestProcessPipeRoutingFallbackWarningIsViewportChrome(t *testing.T) {
	v := newRoutingDiagram(t)
	p := v.Project()
	p.Links[0].Bends = []hmi.XY{{X: 250, Y: 100}}
	if e := v.SetProject(p); e != nil {
		t.Fatal(e)
	}
	old := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(old) })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		for _, camera := range [][3]float64{{0, 0, 1}, {21, 12, .75}, {-120, 60, 2}} {
			v.panX, v.panY, v.zoom = camera[0], camera[1], camera[2]
			dst := paint.NewPixmap(620, 320)
			g := dst.NewPainter()
			v.Draw(g)
			paint.ClosePainter(g)
			im, e := dst.Image()
			if e != nil {
				t.Fatal(e)
			}
			amber := 0
			for y := 3; y < 30; y++ {
				for x := 10; x < 590; x++ {
					r, g, b, _ := im.At(x, y).RGBA()
					if r>>8 > 180 && g>>8 > 100 && b>>8 < 110 {
						amber++
					}
				}
			}
			if amber < 30 {
				t.Fatal("warning moved/scaled away from viewport", camera, amber)
			}
			_, _, _, alpha := im.At(610, 310).RGBA()
			if alpha != 0 {
				t.Fatal("warning escaped diagram clip")
			}
			paint.CloseSurface(dst)
		}
	}
	builds := v.pipeRouteBuilds
	if n := testing.AllocsPerRun(100, func() { v.currentPipeWarning() }); n != 0 || v.pipeRouteBuilds != builds {
		t.Fatal("cached warning rebuilds or allocates", n)
	}
	messages := v.PipeRouteDiagnostics()
	messages[0] = "caller changed"
	if v.PipeRouteDiagnostics()[0] == messages[0] {
		t.Fatal("diagnostic slice aliases cache")
	}
}
