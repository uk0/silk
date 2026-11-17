package main

import (
	"strings"
	"testing"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/hmiapp"
)

func TestRoutingExampleIsInertPortableAndAvoidsItsReactor(t *testing.T) {
	p := routingProject()
	data, e := p.JSON()
	if e != nil {
		t.Fatal(e)
	}
	p, e = hmi.Parse(data)
	if e != nil || len(p.Sources) != 0 || p.Links[0].Routing != hmi.RoutingAvoid {
		t.Fatal("example is not inert/portable", e)
	}
	var obstacles []hmi.PipeObstacle
	for _, n := range p.Nodes {
		obstacles = append(obstacles, hmi.PipeObstacle{ID: n.ID, Bounds: hmi.Rect{X: n.X, Y: n.Y, Width: n.Width, Height: n.Height}})
	}
	r, e := hmi.NewPipeRouter(obstacles, hmi.PipeRouteOptions{Bounds: hmi.Rect{Width: p.Width, Height: p.Height}})
	if e != nil {
		t.Fatal(e)
	}
	a, b := p.Nodes[0], p.Nodes[1]
	result := r.Route(hmi.PipeAnchor{Point: hmi.Port(a, "east"), Direction: "east", Owner: a.ID}, hmi.PipeAnchor{Point: hmi.Port(b, "west"), Direction: "west", Owner: b.ID}, nil)
	if result.Status != hmi.PipeRouteOK || len(result.Points) < 4 {
		t.Fatal(result)
	}
	code, e := hmiapp.Generate(p)
	if e != nil || !strings.Contains(string(code), `\"routing\": \"avoid\"`) {
		t.Fatal("generated workbench lost routing", e)
	}
}
