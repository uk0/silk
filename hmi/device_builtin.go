package hmi

import "github.com/uk0/silk/point"

// BuiltinDeviceLibrary returns independent schematic subgraphs, not live PLC
// drivers or certified P&ID symbols. Placement creates independent point names.
func BuiltinDeviceLibrary() DeviceLibrary {
	run := Point{Definition: point.Definition{Name: "RUN", Kind: point.Bool, Category: "DI", Description: "Device running"}, Initial: false}
	level := Point{Definition: point.Definition{Name: "LEVEL", Kind: point.Float, Category: "AI", Unit: "%"}, Initial: 62.5}
	pressure := Point{Definition: point.Definition{Name: "PRESSURE", Kind: point.Float, Category: "AI", Unit: "bar"}, Initial: 3.2}
	temperature := Point{Definition: point.Definition{Name: "TEMP", Kind: point.Float, Category: "AI", Unit: "°C"}, Initial: 72.5}
	return DeviceLibrary{Version: 1, Devices: []DeviceTemplate{
		{ID: "pump-station", Name: "Pump station", Project: Project{Version: Version, Title: "Pump station", Width: 600, Height: 300,
			Points: []Point{run, level, pressure},
			Nodes: []Node{
				{ID: "tank", Kind: "tank", Label: "Storage tank", X: 0, Y: 0, Width: 140, Height: 200},
				{ID: "valve", Kind: "valve", Label: "Isolation valve", X: 230, Y: 80, Width: 80, Height: 80, Tag: "RUN"},
				{ID: "pump", Kind: "pump", Label: "Transfer pump", X: 410, Y: 60, Width: 120, Height: 120, Tag: "RUN"},
				{ID: "level", Kind: "value", Label: "Tank level", X: 0, Y: 210, Width: 190, Height: 80, Tag: "LEVEL", Unit: "%", Colors: []ColorRule{{When: "x>=90", Color: "#ef5350"}, {When: "true", Color: "#26a269"}}},
				{ID: "pressure", Kind: "value", Label: "Discharge pressure", X: 350, Y: 210, Width: 230, Height: 80, Tag: "PRESSURE", Unit: "bar"},
			},
			Links: []Link{{ID: "suction", From: Endpoint{"tank", "east"}, To: Endpoint{"valve", "west"}, Tag: "RUN"}, {ID: "discharge", From: Endpoint{"valve", "east"}, To: Endpoint{"pump", "west"}, Tag: "RUN"}},
		}},
		{ID: "heat-recovery", Name: "Heat recovery", Project: Project{Version: Version, Title: "Heat recovery", Width: 650, Height: 350,
			Points: []Point{run, temperature},
			Nodes: []Node{
				{ID: "reactor", Kind: "reactor", Label: "Synthesis reactor", X: 0, Y: 0, Width: 130, Height: 320, Tag: "RUN"},
				{ID: "exchanger", Kind: "exchanger", Label: "Heat exchanger", X: 300, Y: 50, Width: 250, Height: 170, Tag: "RUN"},
				{ID: "temperature", Kind: "value", Label: "Outlet temperature", X: 300, Y: 240, Width: 260, Height: 90, Tag: "TEMP", Unit: "°C", Colors: []ColorRule{{When: "x>=100", Color: "#ef5350"}, {When: "x>=80", Color: "#ffb300"}, {When: "true", Color: "#26a269"}}},
			},
			Links: []Link{{ID: "hot-feed", From: Endpoint{"reactor", "east"}, To: Endpoint{"exchanger", "west"}, Tag: "RUN", Bends: []XY{{X: 210, Y: 150}}}},
		}},
		{ID: "valve-station", Name: "Valve station", Project: Project{Version: Version, Title: "Valve station", Width: 450, Height: 200,
			Points: []Point{run, pressure},
			Nodes: []Node{
				{ID: "valve", Kind: "valve", Label: "Control valve", X: 0, Y: 10, Width: 110, Height: 110, Tag: "RUN"},
				{ID: "status", Kind: "indicator", Label: "Open feedback", X: 190, Y: 10, Width: 90, Height: 90, Tag: "RUN"},
				{ID: "pressure", Kind: "value", Label: "Line pressure", X: 190, Y: 110, Width: 230, Height: 80, Tag: "PRESSURE", Unit: "bar"},
			},
		}},
	}}
}
