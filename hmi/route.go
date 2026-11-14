package hmi

import "github.com/uk0/silk/point"

// Port returns document-space nozzle coordinates. Links follow device moves;
// routing is deterministic orthogonal dogleg routing, not obstacle avoidance.
func Port(n Node, port string) XY {
	x, y := n.X+n.Width/2, n.Y+n.Height/2
	body := max(1, n.Height-26)
	switch port {
	case "north":
		y = n.Y
	case "south":
		y = n.Y + n.Height
		if n.Kind != "value" && n.Kind != "text" {
			y = n.Y + body
		}
	case "west":
		x = n.X
		switch n.Kind {
		case "reactor", "separator":
			y = n.Y + body*42/160
		case "exchanger":
			y = n.Y + body*70/160
		case "pump", "valve", "tank", "indicator":
			y = n.Y + body/2
		}
	case "east":
		x = n.X + n.Width
		switch n.Kind {
		case "reactor", "separator":
			y = n.Y + body*120/160
		case "exchanger":
			y = n.Y + body*94/160
		case "pump", "valve", "tank", "indicator":
			y = n.Y + body/2
		}
	}
	return XY{x, y}
}
func Route(a, b XY, bends []XY) []XY {
	result := []XY{a}
	previous := a
	targets := append(append([]XY(nil), bends...), b)
	for _, p := range targets {
		if p.X != previous.X && p.Y != previous.Y {
			result = append(result, XY{(previous.X + p.X) / 2, previous.Y}, XY{(previous.X + p.X) / 2, p.Y})
		}
		result = append(result, p)
		previous = p
	}
	return result
}

// Template builds a self-contained process drawing; no source is enabled and
// loading it performs no network I/O. Initial point values drive local preview.
func Template() Project {
	return Project{Version: Version, Title: "Synthesis & Heat Recovery", Width: 1000, Height: 620, RetentionDays: 30,
		Points: []Point{{Definition: point.Definition{Name: "DI11", Kind: point.Bool, Category: "DI", Description: "Process running"}, Initial: true}, {Definition: point.Definition{Name: "AI101", Kind: point.Float, Category: "AI", Unit: "°C"}, Initial: 72.5}},
		Nodes:  []Node{{ID: "reactor", Kind: "reactor", Label: "R-001", X: 90, Y: 60, Width: 130, Height: 320, Tag: "DI11"}, {ID: "exchanger", Kind: "exchanger", Label: "E-001", X: 400, Y: 90, Width: 230, Height: 170, Tag: "DI11"}, {ID: "separator", Kind: "separator", Label: "V-001", X: 730, Y: 250, Width: 170, Height: 210, Tag: "DI11"}, {ID: "temperature", Kind: "value", Label: "Reactor temperature", X: 70, Y: 420, Width: 260, Height: 110, Tag: "AI101", Unit: "°C", Colors: []ColorRule{{When: "x>=100", Color: "#ef5350"}, {When: "x>=80", Color: "#ffb300"}, {When: "true", Color: "#26a269"}}}},
		Links:  []Link{{ID: "feed", From: Endpoint{"reactor", "east"}, To: Endpoint{"exchanger", "west"}, Tag: "DI11"}, {ID: "recovery", From: Endpoint{"exchanger", "east"}, To: Endpoint{"separator", "north"}, Tag: "DI11"}}}
}
