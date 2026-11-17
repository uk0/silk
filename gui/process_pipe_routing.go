package gui

import (
	"fmt"
	"math"
	"slices"

	"github.com/uk0/silk/hmi"
)

// All routing state belongs to the UI thread. A geometry change invalidates one
// snapshot, not one worker per pipe; colors, selection and camera changes don't.
func (v *ProcessDiagram) invalidatePipeRoutes() { v.pipeRoutesDirty = true }

func processRoutingBounds(n *processNodeView) hmi.Rect {
	w, h := n.node.Width, n.node.Height
	a := n.rotation * math.Pi / 180
	rx, ry := (math.Abs(w*math.Cos(a))+math.Abs(h*math.Sin(a)))/2, (math.Abs(w*math.Sin(a))+math.Abs(h*math.Cos(a)))/2
	return hmi.Rect{X: n.node.X + w/2 - rx, Y: n.node.Y + h/2 - ry, Width: 2 * rx, Height: 2 * ry}
}

func (v *ProcessDiagram) routingAnchor(n *processNodeView, port string) hmi.PipeAnchor {
	direction := 0
	switch port {
	case "south":
		direction = 1
	case "west":
		direction = 2
	case "north":
		direction = 3
	}
	// Snap the outward normal, not the position, to the closest cardinal axis.
	direction = (direction + int(math.Round(n.rotation/90))) % 4
	if direction < 0 {
		direction += 4
	}
	return hmi.PipeAnchor{Point: v.port(n, port), Direction: []string{"east", "south", "west", "north"}[direction], Owner: n.node.ID}
}

func (v *ProcessDiagram) preparePipeRoutes() {
	if !v.pipeRoutesDirty && v.pipeRoutes != nil {
		return
	}
	v.pipeRoutesDirty = false
	v.pipeRoutes = make(map[string]hmi.PipeRouteResult, len(v.project.Links))
	v.pipeRouter = nil
	v.pipeRouterError = nil
	v.pipeRouteMessages = nil
	v.pipeRouteWarning = ""
	// Budget priority follows authored link order, not whether paint, picking or
	// an SDK observer happened to request a particular pipe first.
	for _, l := range v.project.Links {
		r := v.computePipeRoute(l)
		v.pipeRoutes[l.ID] = r
		if r.Status != hmi.PipeRouteOK {
			v.pipeRouteMessages = append(v.pipeRouteMessages, fmt.Sprintf("%s: %s (%s)", l.ID, r.Message, r.Status))
		}
	}
	if len(v.pipeRouteMessages) > 0 {
		v.pipeRouteWarning = fmt.Sprintf("Routing fallback (%d): %s", len(v.pipeRouteMessages), v.pipeRouteMessages[0])
	}
}

func (v *ProcessDiagram) currentPipeRoute(l hmi.Link) hmi.PipeRouteResult {
	if v.closed {
		return hmi.PipeRouteResult{Status: hmi.PipeRouteOK}
	}
	v.preparePipeRoutes()
	return v.pipeRoutes[l.ID]
}

func (v *ProcessDiagram) computePipeRoute(l hmi.Link) hmi.PipeRouteResult {
	a, b := v.views[l.From.Node], v.views[l.To.Node]
	if a == nil || b == nil || !a.visible || !b.visible {
		return hmi.PipeRouteResult{Status: hmi.PipeRouteOK}
	}
	bends := v.linkBends(l)
	result := hmi.PipeRouteResult{Points: hmi.Route(v.port(a, l.From.Port), v.port(b, l.To.Port), bends), Status: hmi.PipeRouteOK}
	if l.Routing == hmi.RoutingAvoid {
		if v.pipeRouter == nil && v.pipeRouterError == nil {
			obstacles := make([]hmi.PipeObstacle, 0, len(v.project.Nodes))
			for _, n := range v.project.Nodes {
				if view := v.views[n.ID]; view != nil && view.visible {
					obstacles = append(obstacles, hmi.PipeObstacle{ID: n.ID, Bounds: processRoutingBounds(view)})
				}
			}
			v.pipeRouter, v.pipeRouterError = hmi.NewPipeRouter(obstacles, hmi.PipeRouteOptions{Bounds: hmi.Rect{Width: v.project.Width, Height: v.project.Height}})
			v.pipeRouteBuilds++
		}
		if v.pipeRouterError != nil {
			result = hmi.PipeRouteResult{Status: hmi.PipeRouteInvalid, Message: v.pipeRouterError.Error()}
		} else {
			result = v.pipeRouter.Route(v.routingAnchor(a, l.From.Port), v.routingAnchor(b, l.To.Port), bends)
		}
	}
	return result
}

// PipeRoute returns a defensive copy of the current document-space route. It is
// UI-thread-only and includes transient drag/live geometry, never authored JSON.
// A hidden endpoint returns an empty successful route, matching what is painted.
func (v *ProcessDiagram) PipeRoute(id string) (hmi.PipeRouteResult, error) {
	if v.closed {
		return hmi.PipeRouteResult{}, ErrUIInvalid
	}
	for _, l := range v.project.Links {
		if l.ID == id {
			r := v.currentPipeRoute(l)
			r.Points = slices.Clone(r.Points)
			return r, nil
		}
	}
	return hmi.PipeRouteResult{}, fmt.Errorf("hmi: unknown pipe %q", id)
}

// PipeRouteDiagnostics returns fallback reasons in authored pipe order. It may
// lazily compute missing routes against the same bounded geometry snapshot.
func (v *ProcessDiagram) PipeRouteDiagnostics() []string {
	if v.closed {
		return nil
	}
	v.preparePipeRoutes()
	return slices.Clone(v.pipeRouteMessages)
}

func (v *ProcessDiagram) currentPipeWarning() string {
	if v.closed {
		return ""
	}
	v.preparePipeRoutes()
	return v.pipeRouteWarning
}
