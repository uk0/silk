// An inert, editable process drawing demonstrating bounded pipe avoidance.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/hmiapp"
	"github.com/uk0/silk/point"
)

func routingProject() hmi.Project {
	return hmi.Project{Version: hmi.Version, Title: "Obstacle-aware process pipes", Width: 1000, Height: 620,
		Points: []hmi.Point{
			{Definition: point.Definition{Name: "DI_RUN", Kind: point.Bool, Category: "DI"}, Initial: true},
			{Definition: point.Definition{Name: "DI_SHOW", Kind: point.Bool, Category: "DI"}, Initial: true},
			{Definition: point.Definition{Name: "AI_X", Kind: point.Float, Category: "AI"}, Initial: float64(420)},
			{Definition: point.Definition{Name: "AI_ROT", Kind: point.Float, Category: "AI"}, Initial: float64(0)},
		}, Nodes: []hmi.Node{
			{ID: "pump", Kind: "pump", Label: "P-001", X: 80, Y: 190, Width: 160, Height: 180, Tag: "DI_RUN"},
			{ID: "tank", Kind: "tank", Label: "T-001", X: 740, Y: 190, Width: 180, Height: 180, Tag: "DI_RUN"},
			{ID: "reactor", Kind: "reactor", Label: "R-001 (routing obstacle)", X: 420, Y: 120, Width: 150, Height: 320, Tag: "DI_RUN", Bindings: []hmi.PropertyBinding{{Tag: "DI_SHOW", Property: "visible"}, {Tag: "AI_X", Property: "x"}, {Tag: "AI_ROT", Property: "rotation"}}},
			{ID: "help", Kind: "text", Label: "Pipes: avoid / orthogonal. Drag the reactor; Esc cancels.\nRun is optional: live manual writes to DI_SHOW / AI_X / AI_ROT reroute transient geometry.\nAmber means a bounded fallback, not successful avoidance. No HTTP or PLC commands.", X: 30, Y: 490, Width: 940, Height: 110},
		}, Links: []hmi.Link{{ID: "transfer", From: hmi.Endpoint{Node: "pump", Port: "east"}, To: hmi.Endpoint{Node: "tank", Port: "west"}, Tag: "DI_RUN", Routing: hmi.RoutingAvoid}}}
}

func main() {
	project := flag.String("project", filepath.Join(os.TempDir(), "silk-process-routing.silkhmi"), "explicit Save/Load/Generate target outside SDK source")
	database := flag.String("db", filepath.Join(os.TempDir(), "silk-process-routing.db"), "point store, opened only by explicit Run")
	flag.Parse()
	doc, err := routingProject().JSON()
	if err != nil {
		log.Fatal(err)
	}
	if err = hmiapp.Run(hmiapp.Config{ProjectJSON: string(doc), ProjectPath: *project, DatabasePath: *database}); err != nil {
		log.Fatal(err)
	}
}
