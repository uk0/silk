package gui

import (
	"math"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/paint"
)

var processInspectionColor = paint.Color{R: 120, G: 201, B: 255, A: 255}

// SetInspectionEnabled opts a non-editing diagram into single-item selection.
// Inspection owns no gesture, source mutation, history entry or point write.
func (v *ProcessDiagram) SetInspectionEnabled(enabled bool) {
	if v.inspectionEnabled == enabled {
		return
	}
	v.inspectionEnabled = enabled
	v.inspection = hmi.SourceTarget{}
	v.Update()
}

func (v *ProcessDiagram) canInspect() bool {
	return v.inspectionEnabled && !v.editing && !v.closed && v.IsEnabled()
}

// InspectionTarget returns the selected visible device or pipe. A live binding
// can hide the object after the click; that selection must not drive navigation.
func (v *ProcessDiagram) InspectionTarget() hmi.SourceTarget {
	if v.canInspect() {
		switch v.inspection.Kind {
		case hmi.SourceNode:
			if n := v.views[v.inspection.ID]; n != nil && n.visible {
				return v.inspection
			}
		case hmi.SourceLink:
			for _, l := range v.project.Links {
				if l.ID == v.inspection.ID && v.inspectablePipe(l) {
					return v.inspection
				}
			}
		}
	}
	return hmi.SourceTarget{}
}

func (v *ProcessDiagram) inspectablePipe(l hmi.Link) bool {
	a, b := v.views[l.From.Node], v.views[l.To.Node]
	return a != nil && b != nil && a.visible && b.visible
}

func (v *ProcessDiagram) setInspection(target hmi.SourceTarget) {
	if target != v.inspection {
		v.inspection = target
		v.Update()
	}
}

func (v *ProcessDiagram) inspectAt(x, y float64) {
	if x < 0 || y < 0 || x >= v.Width() || y >= v.Height() {
		return
	}
	v.SetFocus()
	if !v.canInspect() {
		return
	}
	x, y = (x-v.panX)/v.zoom, (y-v.panY)/v.zoom
	// Invert the draw transform for the topmost visible device. Disabled
	// devices can still be inspected; their live state is not edited.
	for i := len(v.project.Nodes) - 1; i >= 0; i-- {
		view := v.views[v.project.Nodes[i].ID]
		if view == nil || !view.visible {
			continue
		}
		n := view.node
		dx, dy := x-n.X-n.Width/2, y-n.Y-n.Height/2
		if view.rotation != 0 {
			s, c := math.Sincos(view.rotation * math.Pi / 180)
			dx, dy = dx*c+dy*s, -dx*s+dy*c
		}
		if math.Abs(dx) <= n.Width/2 && math.Abs(dy) <= n.Height/2 {
			v.setInspection(hmi.SourceTarget{Kind: hmi.SourceNode, ID: n.ID})
			return
		}
	}
	for i := len(v.project.Links) - 1; i >= 0; i-- {
		l := v.project.Links[i]
		if !v.inspectablePipe(l) {
			continue
		}
		pts := v.pipePoints(l)
		for j := 1; j < len(pts); j++ {
			if processSegmentDistance(x, y, pts[j-1], pts[j]) <= 6/v.zoom {
				v.setInspection(hmi.SourceTarget{Kind: hmi.SourceLink, ID: l.ID})
				return
			}
		}
	}
	v.setInspection(hmi.SourceTarget{})
}
