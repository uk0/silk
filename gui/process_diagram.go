package gui

import (
	"encoding/json"
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
	"math"
	"strconv"
)

type processNodeView struct {
	pipeline         *point.Pipeline
	colors           []processColorRule
	node             hmi.Node
	widget           IWidget
	visible, enabled bool
	rotation         float64
	bindings         map[string]*point.Pipeline
}

// ProcessDiagram draws one ordered widget surface, including all pipes, devices
// and values. Documents are inert. BindTags is explicit and Close cancels it.
type ProcessDiagram struct {
	Widget
	panX, panY, panStartX, panStartY float64
	panning, moved                   bool
	targets                          map[string][]*processNodeView
	project                          hmi.Project
	text                             string
	views                            map[string]*processNodeView
	samples                          map[string]core.Value
	stops                            []func()
	editing, closed                  bool
	selected                         string
	zoom                             float64
	undo, redo                       []string
	changed                          func()
	err                              error
	drag                             bool
	startX, startY                   float64
	beforeDrag                       string
	dragNode                         hmi.Node
	from                             *hmi.Endpoint
}

func init()                              { core.RegisterFactory("gui.ProcessDiagram", core.TypeOf((*ProcessDiagram)(nil))) }
func NewProcessDiagram() *ProcessDiagram { v := new(ProcessDiagram); v.Init(v); return v }
func (v *ProcessDiagram) Init(self IWidget) {
	v.Widget.Init(self)
	v.zoom = 1
	v.SetFocusPolicy(TabFocus)
	_ = v.SetProject(hmi.Template())
}
func (v *ProcessDiagram) ProjectJSON() string { return v.text }
func (v *ProcessDiagram) SetProjectJSON(s string) {
	p, e := hmi.Parse([]byte(s))
	if e != nil {
		v.err = e
		v.Update()
		return
	}
	v.err = v.SetProject(p)
}
func (v *ProcessDiagram) Project() hmi.Project { p, _ := hmi.Parse([]byte(v.text)); return p }
func (v *ProcessDiagram) LastError() error     { return v.err }
func (v *ProcessDiagram) SigChanged(fn func()) { v.changed = fn }
func (v *ProcessDiagram) SetProject(p hmi.Project) error {
	data, e := p.JSON()
	if e != nil {
		return e
	}
	p, e = hmi.Parse(data)
	if e != nil {
		return e
	}
	v.OnPointerCancel()
	v.UnbindTags()
	v.project = p
	v.text = string(data)
	v.err = nil
	v.views = map[string]*processNodeView{}
	v.samples = map[string]core.Value{}
	v.targets = map[string][]*processNodeView{}
	for _, n := range p.Nodes {
		var w IWidget
		if n.Kind == "text" {
			label := NewLabel(n.Label)
			label.SetTextColor(paint.Color{R: 230, G: 240, B: 250, A: 255})
			w = label
		} else if n.Kind == "value" {
			card := NewProcessValue()
			card.SetTitle(n.Label)
			card.SetUnit(n.Unit)
			b, _ := json.Marshal(n.Steps)
			card.SetTransformJSON(string(b))
			b, _ = json.Marshal(n.Colors)
			card.SetColorRulesJSON(string(b))
			w = card
		} else {
			eq := NewProcessEquipment()
			eq.SetKind(n.Kind)
			eq.SetLabel("")
			w = eq
		}
		w.SetSize(n.Width, n.Height)
		view := &processNodeView{node: n, widget: w, visible: true, enabled: true, bindings: map[string]*point.Pipeline{}}
		view.pipeline, _ = point.NewPipeline(n.Steps)
		for _, rule := range n.Colors {
			expr, _ := point.Compile(rule.When)
			rgb, _ := strconv.ParseUint(rule.Color[1:], 16, 24)
			view.colors = append(view.colors, processColorRule{expr, paint.Color{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}, rule.Text})
		}
		for _, b := range n.Bindings {
			view.bindings[b.Property], _ = point.NewPipeline(b.Steps)
		}
		v.views[n.ID] = view
		names := map[string]bool{}
		if n.Tag != "" {
			names[n.Tag] = true
		}
		for _, binding := range n.Bindings {
			names[binding.Tag] = true
		}
		for name := range names {
			v.targets[name] = append(v.targets[name], view)
		}
	}
	for _, d := range p.Points {
		raw := d.Initial
		if raw != nil {
			raw, _ = point.Convert(raw, d.Kind)
		}
		v.applySample(d.Name, core.Value{Raw: raw, Quality: core.QualityGood})
	}
	if v.views[v.selected] == nil {
		v.selected = ""
	}
	v.Update()
	return nil
}
func (v *ProcessDiagram) UnbindTags() {
	for _, s := range v.stops {
		s()
	}
	v.stops = nil
}
func (v *ProcessDiagram) BindTags(db *core.TagDB) (func(), error) {
	if v.closed || db == nil {
		return nil, ErrUIInvalid
	}
	for _, p := range v.project.Points {
		if _, ok := db.Get(p.Name); !ok {
			return nil, fmt.Errorf("hmi: unknown point %s", p.Name)
		}
	}
	v.UnbindTags()
	var stops []func()
	for _, p := range v.project.Points {
		tag, _ := db.Get(p.Name)
		name := p.Name
		stops = append(stops, BindTagUI(tag, func(s core.Value) {
			if !v.closed {
				v.applySample(name, s)
				v.Update()
			}
		}))
	}
	v.stops = stops
	return func() {
		for _, s := range stops {
			s()
		}
	}, nil
}
func (v *ProcessDiagram) applySample(name string, s core.Value) {
	v.samples[name] = s
	for _, view := range v.targets[name] {
		if view.node.Tag == name {
			sample := s
			if view.node.Kind != "value" {
				out, e := view.pipeline.Apply(s.Raw)
				if e != nil {
					sample.Quality = core.QualityBad
				} else {
					sample.Raw = out
				}
			}
			if widget, ok := view.widget.(interface{ SetSample(core.Value) }); ok {
				widget.SetSample(sample)
			} else if label, ok := view.widget.(*Label); ok {
				if sample.Quality == core.QualityGood {
					label.SetText(TagString(sample.Raw))
					label.SetTextColor(paint.Color{R: 230, G: 240, B: 250, A: 255})
				} else {
					label.SetText("—")
					label.SetTextColor(paint.Color{R: 140, G: 148, B: 160, A: 255})
				}
			}
			if equipment, ok := view.widget.(*ProcessEquipment); ok {
				equipment.hasStateColor = false
			}
			if sample.Quality == core.QualityGood && view.node.Kind != "value" {
				for _, rule := range view.colors {
					matched, e := rule.expr.Evaluate(sample.Raw)
					if e != nil {
						v.err = e
						bad := sample
						bad.Quality = core.QualityBad
						if sink, ok := view.widget.(interface{ SetSample(core.Value) }); ok {
							sink.SetSample(bad)
						}
						if label, ok := view.widget.(*Label); ok {
							label.SetText("—")
							label.SetTextColor(paint.Color{R: 140, G: 148, B: 160, A: 255})
						}
						break
					}
					if matched == true {
						if label, ok := view.widget.(*Label); ok {
							label.SetTextColor(rule.color)
							if rule.text != "" {
								label.SetText(rule.text)
							}
						}
						if equipment, ok := view.widget.(*ProcessEquipment); ok {
							equipment.stateColor = rule.color
							equipment.hasStateColor = true
						}
						break
					}
				}
			}
		}

		for _, b := range view.node.Bindings {
			if b.Tag != name || s.Quality != core.QualityGood {
				continue
			}
			out, e := view.bindings[b.Property].Apply(s.Raw)
			if e != nil {
				continue
			}
			switch b.Property {
			case "visible", "enabled":
				val, e := point.Convert(out, point.Bool)
				if e == nil {
					if b.Property == "visible" {
						view.visible = val.(bool)
					} else {
						view.enabled = val.(bool)
					}
				}
			case "x", "y", "rotation":
				val, e := point.Convert(out, point.Float)
				if e == nil {
					n := val.(float64)
					switch b.Property {
					case "x":
						view.node.X = max(0, min(v.project.Width-view.node.Width, n))
					case "y":
						view.node.Y = max(0, min(v.project.Height-view.node.Height, n))
					case "rotation":
						view.rotation = math.Mod(n, 360)
					}
				}
			}
		}
	}
}
func (v *ProcessDiagram) Close() {
	v.OnPointerCancel()
	v.UnbindTags()
	v.closed = true
	v.changed = nil
}
func (v *ProcessDiagram) SetParent(p IWidget) {
	if p != v.Parent() {
		v.OnPointerCancel()
		v.UnbindTags()
	}
	v.Widget.SetParent(p)
}
func (v *ProcessDiagram) SetEditing(b bool) {
	if v.editing != b {
		v.OnPointerCancel()
		v.editing = b
		v.Update()
	}
}
func (v *ProcessDiagram) IsEditing() bool      { return v.editing }
func (v *ProcessDiagram) SelectedNode() string { return v.selected }
func (v *ProcessDiagram) Zoom() float64        { return v.zoom }
func (v *ProcessDiagram) SetZoom(z float64) {
	if finiteProcess(z) {
		v.zoom = max(.25, min(3, z))
		v.Update()
	}
}
func (v *ProcessDiagram) EnumProperties(p core.IPropertyList) {
	p.AddProperty("Project JSON", v.ProjectJSON, v.SetProjectJSON)
	p.AddProperty("Editing", v.IsEditing, v.SetEditing)
	p.AddProperty("Zoom", v.Zoom, v.SetZoom)
}
func (v *ProcessDiagram) SizeHints() SizeHints {
	return SizeHints{Width: 800, Height: 500, Policy: GrowHorizontal | GrowVertical}
}
func (v *ProcessDiagram) Draw(g paint.Painter) {
	w, h := v.Size()
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, w, h)
	g.Clip()
	g.SetBrush1(paint.Color{R: 19, G: 39, B: 65, A: 255})
	g.Paint()
	g.Translate(v.panX, v.panY)
	g.Scale(v.zoom, v.zoom)
	g.SetFont(Theme().Font)
	if v.editing {
		g.SetBrush1(paint.Color{R: 50, G: 70, B: 90, A: 255})
		step := max(20., 10/v.zoom)
		for x := max(0, math.Floor(-v.panX/v.zoom/step)*step); x < min(v.project.Width, (w-v.panX)/v.zoom); x += step {
			for y := max(0, math.Floor(-v.panY/v.zoom/step)*step); y < min(v.project.Height, (h-v.panY)/v.zoom); y += step {
				g.Rectangle(x, y, 1/v.zoom, 1/v.zoom)
			}
		}
		g.Fill()
	}
	for _, l := range v.project.Links {
		a, b := v.views[l.From.Node], v.views[l.To.Node]
		if a == nil || b == nil || !a.visible || !b.visible {
			continue
		}
		pts := hmi.Route(v.port(a, l.From.Port), v.port(b, l.To.Port), l.Bends)
		color := paint.Color{R: 90, G: 110, B: 130, A: 255}
		sample, ok := v.samples[l.Tag]
		if ok && sample.Quality == core.QualityGood {
			if on, e := point.Convert(sample.Raw, point.Bool); e == nil && on == true {
				color = paint.Color{R: 37, G: 210, B: 158, A: 255}
			}
		}
		g.SetPen1(color, 3)
		for i := 1; i < len(pts); i++ {
			g.Line(pts[i-1].X, pts[i-1].Y, pts[i].X, pts[i].Y)
			g.Stroke()
		}
		if len(pts) > 1 {
			a, b := pts[len(pts)-2], pts[len(pts)-1]
			angle := math.Atan2(b.Y-a.Y, b.X-a.X)
			g.SetBrush1(color)
			g.MoveTo(b.X, b.Y)
			g.LineTo(b.X-9*math.Cos(angle-.45), b.Y-9*math.Sin(angle-.45))
			g.LineTo(b.X-9*math.Cos(angle+.45), b.Y-9*math.Sin(angle+.45))
			g.LineTo(b.X, b.Y)
			g.Fill()
		}
	}
	for _, n := range v.project.Nodes {
		view := v.views[n.ID]
		if view == nil || !view.visible {
			continue
		}
		n = view.node
		radius := math.Hypot(n.Width, n.Height) / 2
		cx, cy := n.X+n.Width/2, n.Y+n.Height/2
		if cx+radius < -v.panX/v.zoom || cy+radius < -v.panY/v.zoom || cx-radius > (w-v.panX)/v.zoom || cy-radius > (h-v.panY)/v.zoom {
			continue
		}
		g.Save()
		g.Translate(n.X+n.Width/2, n.Y+n.Height/2)
		g.Rotate(view.rotation * math.Pi / 180)
		g.Translate(-n.Width/2, -n.Height/2)
		if !view.enabled {
			g.Save()
			g.SetBrush1(paint.Color{R: 100, G: 100, B: 100, A: 80})
		}
		view.widget.Draw(g)
		if n.Kind != "value" && n.Kind != "text" {
			g.SetFont(Theme().Font)
			g.SetBrush1(paint.Color{R: 230, G: 240, B: 250, A: 255})
			g.DrawText1(4, n.Height-5, elideToWidth(Theme().Font, n.Label, max(0, n.Width-8)))
		}
		if !view.enabled {
			g.SetBrush1(paint.Color{R: 100, G: 100, B: 100, A: 80})
			g.Rectangle(0, 0, n.Width, n.Height)
			g.Fill()
			g.Restore()
		}
		if v.editing && v.selected == n.ID {
			g.SetPen1(paint.Color{R: 68, G: 160, B: 255, A: 255}, 1)
			g.Rectangle(0, 0, n.Width, n.Height)
			g.Stroke()
		}
		g.Restore()
		if v.editing {
			for _, port := range []string{"west", "east", "north", "south"} {
				p := v.port(view, port)
				g.SetBrush1(paint.Color{R: 55, G: 170, B: 255, A: 255})
				g.Arc(p.X, p.Y, 4, 0, 2*math.Pi)
				g.Fill()
			}
		}
	}
	if v.err != nil {
		g.SetBrush1(paint.Color{R: 255, G: 155, B: 140, A: 255})
		g.DrawText1(12, 22, elideToWidth(Theme().Font, v.err.Error(), max(0, w/v.zoom-24)))
	}
}
func (v *ProcessDiagram) port(n *processNodeView, s string) hmi.XY {
	p := hmi.Port(n.node, s)
	if n.rotation == 0 {
		return p
	}
	cx, cy := n.node.X+n.node.Width/2, n.node.Y+n.node.Height/2
	a := n.rotation * math.Pi / 180
	x, y := p.X-cx, p.Y-cy
	return hmi.XY{X: cx + x*math.Cos(a) - y*math.Sin(a), Y: cy + x*math.Sin(a) + y*math.Cos(a)}
}
func (v *ProcessDiagram) edit(p hmi.Project) error {
	before := v.text
	if e := v.SetProject(p); e != nil {
		return e
	}
	if v.text != before {
		v.undo = append(v.undo, before)
		if len(v.undo) > 32 {
			v.undo = v.undo[1:]
		}
		v.redo = nil
		if v.changed != nil {
			v.changed()
		}
	}
	return nil
}
func (v *ProcessDiagram) ApplyProject(p hmi.Project) error { return v.edit(p) }

// ClearHistory starts a new document boundary after an explicit Load.
func (v *ProcessDiagram) ClearHistory() { v.undo, v.redo = nil, nil }
func (v *ProcessDiagram) Undo() {
	if len(v.undo) == 0 {
		return
	}
	s := v.undo[len(v.undo)-1]
	v.undo = v.undo[:len(v.undo)-1]
	v.redo = append(v.redo, v.text)
	p, e := hmi.Parse([]byte(s))
	if e == nil {
		_ = v.SetProject(p)
		if v.changed != nil {
			v.changed()
		}
	}
}
func (v *ProcessDiagram) Redo() {
	if len(v.redo) == 0 {
		return
	}
	s := v.redo[len(v.redo)-1]
	v.redo = v.redo[:len(v.redo)-1]
	v.undo = append(v.undo, v.text)
	p, e := hmi.Parse([]byte(s))
	if e == nil {
		_ = v.SetProject(p)
		if v.changed != nil {
			v.changed()
		}
	}
}
func (v *ProcessDiagram) AddNode(kind string) error {
	p := v.Project()
	id := ""
	for i := 1; i < 10000; i++ {
		id = fmt.Sprintf("%s%d", kind, i)
		if v.views[id] == nil {
			break
		}
	}
	n := hmi.Node{ID: id, Kind: kind, Label: id, X: 40, Y: 40, Width: 120, Height: 180}
	if kind == "value" {
		n.Width, n.Height = 220, 100
	}
	p.Nodes = append(p.Nodes, n)
	if e := v.edit(p); e != nil {
		return e
	}
	v.selected = id
	return nil
}
func (v *ProcessDiagram) DeleteSelected() {
	p := v.Project()
	id := v.selected
	nodes := p.Nodes[:0]
	for _, n := range p.Nodes {
		if n.ID != id {
			nodes = append(nodes, n)
		}
	}
	p.Nodes = nodes
	links := p.Links[:0]
	for _, l := range p.Links {
		if l.From.Node != id && l.To.Node != id {
			links = append(links, l)
		}
	}
	p.Links = links
	_ = v.edit(p)
}
func (v *ProcessDiagram) OnLeftDown(x, y float64) {
	if !v.editing || !v.IsEnabled() || !finiteProcess(x) || !finiteProcess(y) {
		return
	}
	x, y = (x-v.panX)/v.zoom, (y-v.panY)/v.zoom
	v.SetFocus()
	for _, n := range v.project.Nodes {
		view := v.views[n.ID]
		if !view.visible || !view.enabled {
			continue
		}
		for _, port := range []string{"west", "east", "north", "south"} {
			p := v.port(view, port)
			if math.Hypot(x-p.X, y-p.Y) < 8 {
				end := hmi.Endpoint{Node: n.ID, Port: port}
				if v.from == nil {
					v.from = &end
					v.Update()
					return
				}
				if v.from.Node != end.Node {
					p := v.Project()
					p.Links = append(p.Links, hmi.Link{ID: nextProcessLinkID(p), From: *v.from, To: end})
					v.from = nil
					v.err = v.edit(p)
				}
				return
			}
		}
	}
	for i := len(v.project.Nodes) - 1; i >= 0; i-- {
		n := v.project.Nodes[i]
		view := v.views[n.ID]
		if !view.visible || !view.enabled {
			continue
		}
		n = view.node
		if x >= n.X && x <= n.X+n.Width && y >= n.Y && y <= n.Y+n.Height {
			v.selected = n.ID
			v.drag = true
			v.moved = false
			v.startX, v.startY = x, y
			v.beforeDrag = v.text
			v.dragNode = n
			v.PushCapture()
			v.Update()
			return
		}
	}
	v.selected = ""
	v.from = nil
	v.Update()
}
func (v *ProcessDiagram) OnMouseMove(x, y float64) {
	if v.panning && finiteProcess(x) && finiteProcess(y) {
		v.panX += x - v.panStartX
		v.panY += y - v.panStartY
		v.panStartX, v.panStartY = x, y
		v.Update()
		return
	}
	if !v.drag || !finiteProcess(x) || !finiteProcess(y) {
		return
	}
	if math.Hypot((x-v.panX)/v.zoom-v.startX, (y-v.panY)/v.zoom-v.startY) < 3 && !v.moved {
		return
	}
	v.moved = true
	if !v.IsEnabled() {
		v.OnPointerCancel()
		return
	}
	n := v.dragNode
	if view := v.views[n.ID]; view != nil {
		view.node.X = max(0, min(v.project.Width-n.Width, math.Round((n.X+(x-v.panX)/v.zoom-v.startX)/10)*10))
		view.node.Y = max(0, min(v.project.Height-n.Height, math.Round((n.Y+(y-v.panY)/v.zoom-v.startY)/10)*10))
		v.Update()
		return
	}
}
func (v *ProcessDiagram) OnLeftUp(x, y float64) {
	if !v.drag {
		return
	}
	if !finiteProcess(x) || !finiteProcess(y) {
		v.OnPointerCancel()
		return
	}
	v.OnMouseMove(x, y)
	if !v.moved {
		v.drag = false
		v.PopCapture()
		v.beforeDrag = ""
		return
	}
	view := v.views[v.selected]
	p := v.Project()
	for i := range p.Nodes {
		if p.Nodes[i].ID == v.selected {
			p.Nodes[i].X, p.Nodes[i].Y = view.node.X, view.node.Y
		}
	}
	v.drag = false
	v.PopCapture()
	v.beforeDrag = ""
	v.err = v.edit(p)
}
func (v *ProcessDiagram) OnPointerCancel() {
	if v.panning {
		v.panning = false
		v.PopCapture()
	}
	if v.drag {
		v.drag = false
		v.PopCapture()
		if view := v.views[v.dragNode.ID]; view != nil {
			view.node = v.dragNode
		}
		v.beforeDrag = ""
	}
	v.from = nil
	v.Update()
}
func (v *ProcessDiagram) OnHide() { v.OnPointerCancel() }
func (v *ProcessDiagram) OnMouseWheel(x, y, z float64) {
	if !finiteProcess(x) || !finiteProcess(y) || !finiteProcess(z) {
		return
	}
	old := v.zoom
	next := max(.25, min(3, old*math.Pow(1.1, z)))
	v.OnPointerCancel()
	v.panX = x - (x-v.panX)*next/old
	v.panY = y - (y-v.panY)*next/old
	v.zoom = next
	v.Update()
}
func (v *ProcessDiagram) OnMiddleDown(x, y float64) {
	if !finiteProcess(x) || !finiteProcess(y) {
		return
	}
	v.OnPointerCancel()
	v.panning = true
	v.panStartX, v.panStartY = x, y
	v.PushCapture()
}
func (v *ProcessDiagram) OnMiddleUp(x, y float64) {
	if v.panning {
		v.OnMouseMove(x, y)
		v.panning = false
		v.PopCapture()
	}
}
func (v *ProcessDiagram) OnResize() { v.OnPointerCancel() }
func (v *ProcessDiagram) FitToView() {
	v.OnPointerCancel()
	w, h := v.Size()
	v.panX, v.panY = 0, 0
	v.SetZoom(min(w/v.project.Width, h/v.project.Height))
}

func (v *ProcessDiagram) DuplicateSelected() error {
	p := v.Project()
	for _, n := range p.Nodes {
		if n.ID != v.selected {
			continue
		}
		for i := 1; ; i++ {
			id := fmt.Sprintf("%s_copy%d", n.ID, i)
			if v.views[id] == nil {
				n.ID = id
				break
			}
		}
		n.X = min(p.Width-n.Width, n.X+20)
		n.Y = min(p.Height-n.Height, n.Y+20)
		p.Nodes = append(p.Nodes, n)
		if e := v.edit(p); e != nil {
			return e
		}
		v.selected = n.ID
		return nil
	}
	return fmt.Errorf("select a node first")
}

func nextProcessLinkID(p hmi.Project) string {
	ids := map[string]bool{}
	for _, l := range p.Links {
		ids[l.ID] = true
	}
	for i := 1; ; i++ {
		id := fmt.Sprintf("link%d", i)
		if !ids[id] {
			return id
		}
	}
}
