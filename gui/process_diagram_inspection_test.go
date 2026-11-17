package gui

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/point"
)

func TestProcessDiagramInspectionFollowsLiveGeometryWithoutEditing(t *testing.T) {
	p := hmi.Project{Version: hmi.Version, Width: 640, Height: 360,
		Points: []hmi.Point{
			{Definition: point.Definition{Name: "show", Kind: point.Bool}, Initial: true},
			{Definition: point.Definition{Name: "angle", Kind: point.Float}, Initial: 90.0},
		},
		Nodes: []hmi.Node{
			{ID: "pump", Kind: "pump", X: 20, Y: 80, Width: 100, Height: 40, Bindings: []hmi.PropertyBinding{{Property: "rotation", Tag: "angle"}, {Property: "visible", Tag: "show"}}},
			{ID: "tank", Kind: "tank", X: 400, Y: 80, Width: 100, Height: 40},
		},
		Links: []hmi.Link{{ID: "pipe", From: hmi.Endpoint{Node: "pump", Port: "east"}, To: hmi.Endpoint{Node: "tank", Port: "west"}}},
	}
	v := NewProcessDiagram()
	defer v.Close()
	v.SetSize(640, 360)
	if err := v.SetProject(p); err != nil {
		t.Fatal(err)
	}
	v.panX, v.panY, v.zoom = 12, 16, 0.8
	pointAt := func(x, y float64) { v.OnLeftDown(v.panX+x*v.zoom, v.panY+y*v.zoom) }
	pointAt(70, 62)
	if v.InspectionTarget().ID != "" {
		t.Fatal("ordinary SDK diagram unexpectedly selects")
	}
	v.SetInspectionEnabled(true)
	before := v.ProjectJSON()
	pointAt(70, 62) // inside the rendered rotated node, outside its authored rect
	if v.InspectionTarget() != (hmi.SourceTarget{Kind: hmi.SourceNode, ID: "pump"}) {
		t.Fatal("selection did not follow rotation", v.InspectionTarget())
	}
	v.OnMouseMove(160, 190)
	v.OnLeftUp(160, 190)
	v.OnKeyDown(KeyDelete, false)
	if v.gesture != nil || len(v.undo) != 0 || v.ProjectJSON() != before || v.views["pump"].node.X != 20 {
		t.Fatal("inspection routed into authoring")
	}
	points := v.pipePoints(v.project.Links[0])
	var pipe hmi.XY
	// Pick a long segment between the devices using the rendered route.
	for i := 1; i < len(points); i++ {
		x := (points[i-1].X + points[i].X) / 2
		if x > 135 && x < 385 {
			pipe = hmi.XY{X: x, Y: (points[i-1].Y + points[i].Y) / 2}
			break
		}
	}
	pointAt(pipe.X, pipe.Y)
	if v.InspectionTarget() != (hmi.SourceTarget{Kind: hmi.SourceLink, ID: "pipe"}) {
		t.Fatal("selection did not follow rendered pipe", v.InspectionTarget(), points)
	}
	v.applySample("show", core.Value{Raw: false, Quality: core.QualityGood})
	if v.InspectionTarget().ID != "" {
		t.Fatal("hidden pipe remained navigable")
	}
	pointAt(pipe.X, pipe.Y)
	if v.InspectionTarget().ID != "" {
		t.Fatal("picked an invisible pipe")
	}
	pointAt(450, 100)
	if v.InspectionTarget().ID != "tank" {
		t.Fatal("expected visible tank selection")
	}
	v.OnKeyDown(KeyEsc, false)
	if v.InspectionTarget().ID != "" {
		t.Fatal("Escape did not clear inspection")
	}
	pointAt(450, 100)
	if err := v.SetProject(p); err != nil || v.InspectionTarget().ID != "" {
		t.Fatal("new snapshot inherited an old selection", err)
	}
}
