package gui

import (
	"fmt"
	"math"
	"slices"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/paint"
)

type processModifiers struct{ shift, action, alt bool }

func processInputModifiers() processModifiers {
	return processModifiers{IsKeyDown(KeyShift), IsKeyDown(KeyCtrl) || isActionModifier(), IsKeyDown(KeyMenu)}
}

type processGesture struct {
	kind                    string
	x, y                    float64
	currentX, currentY      float64
	moved, add              bool
	nodes                   []hmi.Node
	bounds                  hmi.Rect
	corner                  int
	link                    string
	bend                    int
	baseBends, previewBends map[string][]hmi.XY
	selection               []string
	primary                 string
}

func (v *ProcessDiagram) canEdit() bool {
	return v.editing && !v.readOnly && !v.closed && v.IsEnabled()
}
func (v *ProcessDiagram) nodeSelected(id string) bool {
	if len(v.selection) == 0 {
		return id != "" && v.selected == id
	}
	return v.selection[id]
}

// SelectedNodes is returned in authored draw order. Selection is transient and
// never enters ProjectJSON, undo snapshots, acquired points or generated Go.
func (v *ProcessDiagram) SelectedNodes() []string {
	var ids []string
	for _, n := range v.project.Nodes {
		if v.nodeSelected(n.ID) {
			ids = append(ids, n.ID)
		}
	}
	return ids
}
func (v *ProcessDiagram) SelectedLink() string          { return v.selectedLink }
func (v *ProcessDiagram) SigSelectionChanged(fn func()) { v.selectionChanged = fn }
func (v *ProcessDiagram) notifySelection() {
	v.Update()
	if v.selectionChanged != nil {
		v.selectionChanged()
	}
}
func (v *ProcessDiagram) SelectNodes(ids []string) {
	v.selection = map[string]bool{}
	v.selected = ""
	v.selectedLink = ""
	v.selectedBend = -1
	for _, id := range ids {
		if v.views[id] != nil {
			v.selection[id] = true
			v.selected = id
		}
	}
	if len(v.selection) < 2 {
		v.selection = nil
	}
	v.notifySelection()
}
func (v *ProcessDiagram) SelectAll() {
	if !v.canEdit() {
		return
	}
	ids := make([]string, 0, len(v.project.Nodes))
	for _, n := range v.project.Nodes {
		ids = append(ids, n.ID)
	}
	v.SelectNodes(ids)
}
func (v *ProcessDiagram) pruneSelection() {
	for id := range v.selection {
		if v.views[id] == nil {
			delete(v.selection, id)
		}
	}
	if v.views[v.selected] == nil {
		v.selected = ""
		for _, n := range v.project.Nodes {
			if v.selection[n.ID] {
				v.selected = n.ID
			}
		}
	}
	found := false
	for _, l := range v.project.Links {
		if l.ID == v.selectedLink {
			found = true
			if v.selectedBend >= len(l.Bends) {
				v.selectedBend = -1
			}
			break
		}
	}
	if !found {
		v.selectedLink = ""
		v.selectedBend = -1
	}
	v.notifySelection()
}
func (v *ProcessDiagram) resetAuthoredGeometry() {
	v.invalidatePipeRoutes()
	for _, n := range v.project.Nodes {
		if view := v.views[n.ID]; view != nil {
			view.node = n
			view.visible = true
			view.enabled = true
			view.rotation = 0
			view.widget.SetSize(n.Width, n.Height)
		}
	}
}
func (v *ProcessDiagram) CanUndo() bool { return v.canEdit() && len(v.undo) > 0 }
func (v *ProcessDiagram) CanRedo() bool { return v.canEdit() && len(v.redo) > 0 }
func (v *ProcessDiagram) applyCommand(p hmi.Project, err error) error {
	if err != nil {
		v.err = err
		v.Update()
		return err
	}
	v.err = v.edit(p)
	return v.err
}
func (v *ProcessDiagram) AlignSelection(mode string) error {
	if !v.canEdit() {
		return fmt.Errorf("hmi: authoring is locked")
	}
	v.OnPointerCancel()
	return v.applyCommand(hmi.AlignNodes(v.project, v.SelectedNodes(), mode))
}
func (v *ProcessDiagram) DistributeSelection(horizontal bool) error {
	if !v.canEdit() {
		return fmt.Errorf("hmi: authoring is locked")
	}
	v.OnPointerCancel()
	return v.applyCommand(hmi.DistributeNodes(v.project, v.SelectedNodes(), horizontal))
}
func (v *ProcessDiagram) ReorderSelection(front bool) error {
	if !v.canEdit() {
		return fmt.Errorf("hmi: authoring is locked")
	}
	v.OnPointerCancel()
	return v.applyCommand(hmi.ReorderNodes(v.project, v.SelectedNodes(), front))
}
func (v *ProcessDiagram) NudgeSelection(dx, dy float64) error {
	if !v.canEdit() {
		return fmt.Errorf("hmi: authoring is locked")
	}
	v.OnPointerCancel()
	return v.applyCommand(hmi.MoveNodes(v.project, v.SelectedNodes(), dx, dy))
}
func (v *ProcessDiagram) DuplicateSelected() error {
	if !v.canEdit() {
		return fmt.Errorf("hmi: authoring is locked")
	}
	v.OnPointerCancel()
	p, ids, err := hmi.DuplicateNodes(v.project, v.SelectedNodes(), 20, 20)
	if err != nil {
		return v.applyCommand(p, err)
	}
	if err = v.applyCommand(p, nil); err == nil {
		v.SelectNodes(ids)
	}
	return err
}
func (v *ProcessDiagram) DeleteSelected() {
	if !v.canEdit() {
		return
	}
	v.OnPointerCancel()
	if v.selectedBend >= 0 {
		_ = v.RemoveSelectedPipeBend()
		return
	}
	if v.selectedLink != "" {
		p := v.Project()
		for i := range p.Links {
			if p.Links[i].ID == v.selectedLink {
				p.Links = slices.Delete(p.Links, i, i+1)
				_ = v.applyCommand(p, nil)
				return
			}
		}
	}
	if ids := v.SelectedNodes(); len(ids) > 0 {
		_ = v.applyCommand(hmi.DeleteNodes(v.project, ids))
	}
}
func (v *ProcessDiagram) selectPipe(id string) {
	v.selection = nil
	v.selected = ""
	v.selectedLink = id
	v.selectedBend = -1
	v.notifySelection()
}
func (v *ProcessDiagram) linkBends(l hmi.Link) []hmi.XY {
	if v.gesture != nil {
		if b, ok := v.gesture.previewBends[l.ID]; ok {
			return b
		}
	}
	return l.Bends
}
func (v *ProcessDiagram) pipePoints(l hmi.Link) []hmi.XY {
	return v.currentPipeRoute(l).Points
}
func processSegmentDistance(x, y float64, a, b hmi.XY) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	length := dx*dx + dy*dy
	if length == 0 {
		return math.Hypot(x-a.X, y-a.Y)
	}
	t := max(0, min(1, ((x-a.X)*dx+(y-a.Y)*dy)/length))
	return math.Hypot(x-a.X-t*dx, y-a.Y-t*dy)
}
func processCorners(n hmi.Node) [4]hmi.XY {
	return [4]hmi.XY{{X: n.X, Y: n.Y}, {X: n.X + n.Width, Y: n.Y}, {X: n.X + n.Width, Y: n.Y + n.Height}, {X: n.X, Y: n.Y + n.Height}}
}
func (v *ProcessDiagram) beginGesture(g *processGesture) { v.gesture = g; v.PushCapture(); v.Update() }
func (v *ProcessDiagram) OnLeftDown(x, y float64)        { v.pointerDown(x, y, processInputModifiers()) }
func (v *ProcessDiagram) pointerDown(x, y float64, mods processModifiers) {
	if !v.canEdit() || !finiteProcess(x) || !finiteProcess(y) {
		return
	}
	if v.gesture != nil || v.panning {
		v.OnPointerCancel()
	}
	x, y = (x-v.panX)/v.zoom, (y-v.panY)/v.zoom
	v.pointerX, v.pointerY = x, y
	v.SetFocus()
	// Small handles use screen-space hit budgets at every zoom level.
	if ids := v.SelectedNodes(); len(ids) == 1 && !mods.shift && !mods.action {
		view := v.views[ids[0]]
		for i, p := range processCorners(view.node) {
			if math.Hypot(x-p.X, y-p.Y) <= 7/v.zoom {
				v.beginGesture(&processGesture{kind: "resize", x: x, y: y, nodes: []hmi.Node{view.node}, corner: i})
				return
			}
		}
	}
	for _, l := range v.project.Links {
		if l.ID == v.selectedLink {
			for i, b := range l.Bends {
				if math.Hypot(x-b.X, y-b.Y) <= 7/v.zoom {
					v.selectedBend = i
					v.beginGesture(&processGesture{kind: "bend", x: x, y: y, link: l.ID, bend: i, baseBends: map[string][]hmi.XY{l.ID: slices.Clone(l.Bends)}, previewBends: map[string][]hmi.XY{l.ID: slices.Clone(l.Bends)}})
					v.notifySelection()
					return
				}
			}
		}
	}
	for i := len(v.project.Nodes) - 1; i >= 0; i-- {
		n := v.project.Nodes[i]
		view := v.views[n.ID]
		if !view.visible || !view.enabled {
			continue
		}
		for _, port := range []string{"west", "east", "north", "south"} {
			p := v.port(view, port)
			if math.Hypot(x-p.X, y-p.Y) <= 7/v.zoom {
				end := hmi.Endpoint{Node: n.ID, Port: port}
				if v.from == nil {
					v.from = &end
					v.Update()
					return
				}
				if v.from.Node == end.Node {
					v.from = nil
					v.Update()
					return
				}
				p := v.Project()
				p.Links = append(p.Links, hmi.Link{ID: nextProcessLinkID(p), From: *v.from, To: end, Routing: hmi.RoutingAvoid})
				id := p.Links[len(p.Links)-1].ID
				v.from = nil
				if v.applyCommand(p, nil) == nil {
					v.selectPipe(id)
				}
				return
			}
		}
	}
	v.from = nil
	for i := len(v.project.Nodes) - 1; i >= 0; i-- {
		n := v.project.Nodes[i]
		view := v.views[n.ID]
		if !view.visible || !view.enabled {
			continue
		}
		n = view.node
		if x >= n.X && x <= n.X+n.Width && y >= n.Y && y <= n.Y+n.Height {
			ids := v.SelectedNodes()
			selected := v.nodeSelected(n.ID)
			if mods.shift || mods.action {
				if selected {
					ids = slices.DeleteFunc(ids, func(id string) bool { return id == n.ID })
					v.SelectNodes(ids)
					return
				}
				ids = append(ids, n.ID)
				v.SelectNodes(ids)
			} else if !selected {
				v.SelectNodes([]string{n.ID})
			}
			v.selected = n.ID
			ids = v.SelectedNodes()
			bounds, err := hmi.TranslationBounds(v.project, ids)
			if err != nil {
				v.err = err
				return
			}
			g := &processGesture{kind: "move", x: x, y: y, bounds: bounds, baseBends: map[string][]hmi.XY{}, previewBends: map[string][]hmi.XY{}}
			for _, node := range v.project.Nodes {
				if v.nodeSelected(node.ID) {
					g.nodes = append(g.nodes, node)
				}
			}
			for _, l := range v.project.Links {
				if v.nodeSelected(l.From.Node) && v.nodeSelected(l.To.Node) {
					g.baseBends[l.ID] = slices.Clone(l.Bends)
					g.previewBends[l.ID] = slices.Clone(l.Bends)
				}
			}
			v.beginGesture(g)
			return
		}
	}
	for i := len(v.project.Links) - 1; i >= 0; i-- {
		l := v.project.Links[i]
		pts := v.pipePoints(l)
		for j := 1; j < len(pts); j++ {
			if processSegmentDistance(x, y, pts[j-1], pts[j]) <= 6/v.zoom {
				v.selectPipe(l.ID)
				return
			}
		}
	}
	before, primary := v.SelectedNodes(), v.selected
	if !mods.shift && !mods.action {
		v.SelectNodes(nil)
	}
	v.beginGesture(&processGesture{kind: "marquee", x: x, y: y, currentX: x, currentY: y, add: mods.shift || mods.action, selection: before, primary: primary})
}
func (v *ProcessDiagram) OnMouseMove(x, y float64) { v.pointerMove(x, y, processInputModifiers()) }
func (v *ProcessDiagram) pointerMove(x, y float64, mods processModifiers) {
	if !finiteProcess(x) || !finiteProcess(y) {
		return
	}
	if v.panning {
		v.panX += x - v.panStartX
		v.panY += y - v.panStartY
		v.panStartX, v.panStartY = x, y
		v.Update()
		return
	}
	x, y = (x-v.panX)/v.zoom, (y-v.panY)/v.zoom
	v.pointerX, v.pointerY = x, y
	g := v.gesture
	if g == nil {
		if v.from != nil {
			v.Update()
		}
		return
	}
	if !v.canEdit() {
		v.OnPointerCancel()
		return
	}
	if math.Hypot((x-g.x)*v.zoom, (y-g.y)*v.zoom) < 3 && !g.moved {
		return
	}
	g.moved = true
	g.currentX, g.currentY = x, y
	snap := func(n float64) float64 {
		if mods.alt {
			return n
		}
		return math.Round(n/10) * 10
	}
	dx, dy := x-g.x, y-g.y
	geometryChanged := false
	switch g.kind {
	case "move":
		anchor := g.nodes[0]
		for _, n := range g.nodes {
			if n.ID == v.selected {
				anchor = n
				break
			}
		}
		dx, dy = snap(anchor.X+dx)-anchor.X, snap(anchor.Y+dy)-anchor.Y
		if mods.shift {
			if math.Abs(dx) > math.Abs(dy) {
				dy = 0
			} else {
				dx = 0
			}
		}
		d, err := hmi.ClampTranslation(v.project.Width, v.project.Height, g.bounds, dx, dy)
		if err != nil {
			return
		}
		for _, n := range g.nodes {
			view := v.views[n.ID]
			geometryChanged = geometryChanged || view.node.X != n.X+d.X || view.node.Y != n.Y+d.Y
			view.node.X = n.X + d.X
			view.node.Y = n.Y + d.Y
		}
		for id, base := range g.baseBends {
			for i, p := range base {
				next := hmi.XY{X: p.X + d.X, Y: p.Y + d.Y}
				geometryChanged = geometryChanged || g.previewBends[id][i] != next
				g.previewBends[id][i] = next
			}
		}
	case "resize":
		n := g.nodes[0]
		b := processResize(n, g.corner, dx, dy, v.project.Width, v.project.Height, snap, mods.shift)
		view := v.views[n.ID]
		geometryChanged = view.node.X != b.X || view.node.Y != b.Y || view.node.Width != b.Width || view.node.Height != b.Height
		view.node.X, view.node.Y, view.node.Width, view.node.Height = b.X, b.Y, b.Width, b.Height
		view.widget.SetSize(b.Width, b.Height)
	case "bend":
		old := g.baseBends[g.link][g.bend]
		next := hmi.XY{X: max(0, min(v.project.Width, snap(old.X+dx))), Y: max(0, min(v.project.Height, snap(old.Y+dy)))}
		geometryChanged = g.previewBends[g.link][g.bend] != next
		g.previewBends[g.link][g.bend] = next
	}
	if geometryChanged {
		v.invalidatePipeRoutes()
	}
	v.Update()
}
func processResize(n hmi.Node, corner int, dx, dy, width, height float64, snap func(float64) float64, aspect bool) hmi.Rect {
	left, top, right, bottom := n.X, n.Y, n.X+n.Width, n.Y+n.Height
	west, north := corner == 0 || corner == 3, corner == 0 || corner == 1
	if aspect {
		sx, sy := dx/n.Width, dy/n.Height
		if west {
			sx = -sx
		}
		if north {
			sy = -sy
		}
		delta := sx
		if math.Abs(sy) > math.Abs(sx) {
			delta = sy
		}
		maxW, maxH := width-left, height-top
		if west {
			maxW = right
		}
		if north {
			maxH = bottom
		}
		scale := max(max(16/n.Width, 16/n.Height), min(min(maxW/n.Width, maxH/n.Height), 1+delta))
		w, h := n.Width*scale, n.Height*scale
		if west {
			left = right - w
		} else {
			right = left + w
		}
		if north {
			top = bottom - h
		} else {
			bottom = top + h
		}
	} else {
		if west {
			left = max(0, min(right-16, snap(n.X+dx)))
		} else {
			right = min(width, max(left+16, snap(right+dx)))
		}
		if north {
			top = max(0, min(bottom-16, snap(n.Y+dy)))
		} else {
			bottom = min(height, max(top+16, snap(bottom+dy)))
		}
	}
	return hmi.Rect{X: left, Y: top, Width: right - left, Height: bottom - top}
}
func (v *ProcessDiagram) OnLeftUp(x, y float64) { v.pointerUp(x, y, processInputModifiers()) }
func (v *ProcessDiagram) pointerUp(x, y float64, mods processModifiers) {
	g := v.gesture
	if g == nil {
		return
	}
	if !finiteProcess(x) || !finiteProcess(y) {
		v.OnPointerCancel()
		return
	}
	v.pointerMove(x, y, mods)
	if v.gesture != g {
		return
	}
	v.gesture = nil
	v.PopCapture()
	if !g.moved {
		v.Update()
		return
	}
	if g.kind == "marquee" {
		left, top, right, bottom := min(g.x, g.currentX), min(g.y, g.currentY), max(g.x, g.currentX), max(g.y, g.currentY)
		ids := []string{}
		if g.add {
			ids = slices.Clone(g.selection)
		}
		for _, n := range v.project.Nodes {
			inside := n.X >= left && n.Y >= top && n.X+n.Width <= right && n.Y+n.Height <= bottom
			if g.currentX < g.x {
				inside = n.X <= right && n.Y <= bottom && n.X+n.Width >= left && n.Y+n.Height >= top
			}
			if inside && !slices.Contains(ids, n.ID) {
				ids = append(ids, n.ID)
			}
		}
		v.SelectNodes(ids)
		return
	}
	p := v.Project()
	for i := range p.Nodes {
		for _, base := range g.nodes {
			if base.ID == p.Nodes[i].ID {
				preview := v.views[base.ID].node
				p.Nodes[i].X, p.Nodes[i].Y, p.Nodes[i].Width, p.Nodes[i].Height = preview.X, preview.Y, preview.Width, preview.Height
			}
		}
	}
	for i := range p.Links {
		if b, ok := g.previewBends[p.Links[i].ID]; ok {
			p.Links[i].Bends = b
		}
	}
	_ = v.applyCommand(p, nil)
}
func (v *ProcessDiagram) OnPointerCancel() {
	if v.panning {
		v.panning = false
		v.PopCapture()
	}
	if g := v.gesture; g != nil {
		geometryChanged := false
		for id, points := range g.previewBends {
			geometryChanged = geometryChanged || !slices.Equal(points, g.baseBends[id])
		}
		v.gesture = nil
		v.PopCapture()
		for _, n := range g.nodes {
			if view := v.views[n.ID]; view != nil {
				geometryChanged = geometryChanged || view.node.X != n.X || view.node.Y != n.Y || view.node.Width != n.Width || view.node.Height != n.Height
				view.node = n
				view.widget.SetSize(n.Width, n.Height)
			}
		}
		if geometryChanged {
			v.invalidatePipeRoutes()
		}
		if g.kind == "marquee" {
			v.SelectNodes(g.selection)
			v.selected = g.primary
		}
	}
	v.from = nil
	v.Update()
}
func (v *ProcessDiagram) AddSelectedPipeBend() error {
	if !v.canEdit() {
		return fmt.Errorf("hmi: authoring is locked")
	}
	for _, l := range v.project.Links {
		if l.ID == v.selectedLink {
			bestLength := -1.
			point := hmi.XY{}
			index := 0
			path := v.pipePoints(l)
			section := 0
			for j := 1; j < len(path); j++ {
				// Successful routes retain every mandatory bend as a vertex.
				for section < len(l.Bends) && path[j-1] == l.Bends[section] {
					section++
				}
				length := math.Hypot(path[j].X-path[j-1].X, path[j].Y-path[j-1].Y)
				if length > bestLength {
					bestLength = length
					point = hmi.XY{X: (path[j].X + path[j-1].X) / 2, Y: (path[j].Y + path[j-1].Y) / 2}
					index = section
				}
			}
			if bestLength <= 0 {
				return fmt.Errorf("pipe has no drawable segment")
			}
			if err := v.applyCommand(hmi.EditLinkBend(v.project, l.ID, index, &point, true)); err != nil {
				return err
			}
			v.selectedBend = index
			v.notifySelection()
			return nil
		}
	}
	return fmt.Errorf("select a pipe first")
}
func (v *ProcessDiagram) RemoveSelectedPipeBend() error {
	if !v.canEdit() {
		return fmt.Errorf("hmi: authoring is locked")
	}
	if v.selectedLink == "" || v.selectedBend < 0 {
		return fmt.Errorf("select a waypoint first")
	}
	return v.applyCommand(hmi.EditLinkBend(v.project, v.selectedLink, v.selectedBend, nil, false))
}
func (v *ProcessDiagram) OnKeyDown(key int, repeat bool) {
	v.processKey(key, repeat, processInputModifiers())
}
func (v *ProcessDiagram) processKey(key int, repeat bool, mods processModifiers) {
	if key == KeyEsc {
		active := v.gesture != nil || v.from != nil || v.panning
		v.OnPointerCancel()
		if !active {
			v.SelectNodes(nil)
		}
		return
	}
	if !v.canEdit() {
		return
	}
	if mods.action && !repeat {
		switch key {
		case 'A':
			v.SelectAll()
			return
		case 'D':
			_ = v.DuplicateSelected()
			return
		case 'Z':
			if mods.shift {
				v.Redo()
			} else {
				v.Undo()
			}
			return
		case 'Y':
			v.Redo()
			return
		case '0':
			v.FitToView()
			return
		}
	}
	if !mods.action {
		if len(v.SelectedNodes()) == 0 && (key == KeyLeft || key == KeyRight || key == KeyUp || key == KeyDown) {
			return
		}
		step := 1.
		if mods.shift {
			step = 10
		}
		switch key {
		case KeyLeft:
			_ = v.NudgeSelection(-step, 0)
		case KeyRight:
			_ = v.NudgeSelection(step, 0)
		case KeyUp:
			_ = v.NudgeSelection(0, -step)
		case KeyDown:
			_ = v.NudgeSelection(0, step)
		case KeyDelete, KeyBackSpace:
			if repeat {
				return
			}
			if v.selectedBend >= 0 {
				_ = v.RemoveSelectedPipeBend()
			} else {
				v.DeleteSelected()
			}
		case 'F':
			v.FitToView()
		}
	}
}
func (v *ProcessDiagram) drawInteractionOverlay(g paint.Painter) {
	if !v.editing {
		return
	}
	blue := paint.Color{R: 75, G: 170, B: 255, A: 255}
	size := 6 / v.zoom
	if ids := v.SelectedNodes(); len(ids) == 1 {
		for _, p := range processCorners(v.views[ids[0]].node) {
			g.SetBrush1(blue)
			g.Rectangle(p.X-size/2, p.Y-size/2, size, size)
			g.Fill()
		}
	}
	for _, l := range v.project.Links {
		if l.ID != v.selectedLink {
			continue
		}
		for i, p := range v.linkBends(l) {
			color := blue
			if i == v.selectedBend {
				color = paint.Color{R: 255, G: 187, B: 55, A: 255}
			}
			g.SetBrush1(color)
			g.Rectangle(p.X-size, p.Y-size, size*2, size*2)
			g.Fill()
		}
	}
	if v.from != nil {
		view := v.views[v.from.Node]
		if view != nil {
			a := v.port(view, v.from.Port)
			g.SetPen1(paint.Color{R: 255, G: 187, B: 55, A: 255}, 2/v.zoom)
			p := hmi.Route(a, hmi.XY{X: v.pointerX, Y: v.pointerY}, nil)
			for i := 1; i < len(p); i++ {
				g.Line(p[i-1].X, p[i-1].Y, p[i].X, p[i].Y)
				g.Stroke()
			}
			g.Arc(a.X, a.Y, 7/v.zoom, 0, 2*math.Pi)
			g.Stroke()
		}
	}
	if t := v.gesture; t != nil && t.kind == "marquee" && t.moved {
		g.SetBrush1(paint.Color{R: 75, G: 170, B: 255, A: 35})
		g.Rectangle(min(t.x, t.currentX), min(t.y, t.currentY), math.Abs(t.currentX-t.x), math.Abs(t.currentY-t.y))
		g.FillPreserve()
		g.SetPen1(blue, 1/v.zoom)
		g.Stroke()
	}
}

func (v *ProcessDiagram) SetReadOnly(b bool) {
	if v.readOnly != b {
		v.OnPointerCancel()
		v.readOnly = b
		v.Update()
	}
}
func (v *ProcessDiagram) IsReadOnly() bool { return v.readOnly }
