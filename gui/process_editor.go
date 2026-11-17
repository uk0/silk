package gui

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
)

// ProcessEditor is a designer-ready viewport and visual configuration workbench.
// It never starts network/database I/O. Hosts explicitly own a runtime and call
// SetRunning to lock authoring while the document drives live equipment.
type ProcessEditor struct {
	Widget
	locked                                                bool
	closed                                                bool
	Diagram                                               *ProcessDiagram
	buttons                                               []*Button
	status                                                *Label
	running                                               bool
	changed                                               func()
	err                                                   error
	undoButton, redoButton, duplicateButton, deleteButton *Button
	arrangeItems                                          []processArrangeAction
	deviceLibrary                                         hmi.DeviceLibrary
	deviceLibraryReady                                    bool
	deviceLibraryRevision                                 uint64
	closeDeviceLibrary                                    func()
	deviceLibraryChanged                                  func()
}

type processConfigTable struct{ Table }

func (t *processConfigTable) SizeHints() SizeHints {
	h := t.Table.SizeHints()
	h.Stretch = 1
	h.MinHeight = 100
	return h
}

func init()                            { core.RegisterFactory("gui.ProcessEditor", core.TypeOf((*ProcessEditor)(nil))) }
func NewProcessEditor() *ProcessEditor { v := new(ProcessEditor); v.Init(v); return v }
func (v *ProcessEditor) Init(self IWidget) {
	v.Widget.Init(self)
	v.Diagram = NewProcessDiagram()
	v.Diagram.SetParent(self)
	v.Diagram.SetEditing(true)
	v.status = NewLabel("Shift-click / drag a box to select · corners resize · blue ports connect · Esc cancels")
	v.status.SetParent(self)
	add := func(label string, fn func()) *Button {
		b := NewButton1(label, nil)
		b.SetParent(self)
		b.Action().BindFunc0(func() {
			fn()
			v.refreshActions()
			if !v.running && !v.locked {
				v.Diagram.SetFocus()
			}
		})
		v.buttons = append(v.buttons, b)
		return b
	}
	for _, kind := range []string{"reactor", "exchanger", "separator", "pump", "valve", "tank", "value", "indicator", "text"} {
		k := kind
		add("+ "+kind, func() { v.setError(v.Diagram.AddNode(k)) })
	}
	add("Points", v.EditPoints)
	add("HTTP sources", v.EditSources)
	add("HTTP mappings", v.EditMappings)
	add("HTTP headers", v.EditHeaders)
	add("Node / bindings", v.EditNode)
	add("Conversions", v.EditTransforms)
	add("Colors", v.EditColors)
	add("Properties", v.EditBindings)
	add("Pipes", v.EditLinks)
	add("Bends", v.EditBends)
	v.duplicateButton = add("Duplicate", func() { v.setError(v.Diagram.DuplicateSelected()) })
	v.undoButton = add("Undo", v.Diagram.Undo)
	v.redoButton = add("Redo", v.Diagram.Redo)
	v.deleteButton = add("Delete", v.Diagram.DeleteSelected)
	arrange := add("Arrange", func() {})
	v.initArrange(arrange)
	add("Device library", v.ShowDeviceLibrary)
	v.Diagram.SigSelectionChanged(func() {
		v.refreshActions()
		text := fmt.Sprintf("%d selected", len(v.Diagram.SelectedNodes()))
		if pipe := v.Diagram.SelectedLink(); pipe != "" {
			text += " · pipe " + pipe
		}
		v.status.SetText(text + " · Shift-click toggles · arrows nudge")
	})
	v.Diagram.SigChanged(func() {
		if v.changed != nil {
			v.changed()
		}
		v.status.SetText("Document updated · Ctrl/Cmd+Z undo · Shift+arrows move 10 units")
		v.refreshActions()
	})
	v.refreshActions()
}
func (v *ProcessEditor) setError(e error) {
	v.err = e
	if e != nil {
		v.status.SetText(e.Error())
	} else {
		v.status.SetText("Applied")
	}
}
func (v *ProcessEditor) LastError() error    { return v.err }
func (v *ProcessEditor) ProjectJSON() string { return v.Diagram.ProjectJSON() }
func (v *ProcessEditor) SetProjectJSON(s string) {
	v.Diagram.SetProjectJSON(s)
	v.setError(v.Diagram.LastError())
}
func (v *ProcessEditor) SigChanged(fn func()) { v.changed = fn }
func (v *ProcessEditor) SetRunning(b bool) {
	changed := v.running != b
	if b && v.closeDeviceLibrary != nil {
		v.closeDeviceLibrary()
	}
	v.running = b
	v.Diagram.SetEditing(!b)
	for _, button := range v.buttons {
		button.SetVisible(!b)
		button.SetEnabled(!b && !v.locked)
		if b || v.locked {
			button.HideSubPopup()
		}
	}
	v.refreshActions()
	if changed {
		if b {
			v.status.SetText("Live preview · authoring locked · live points are transient")
		} else {
			v.status.SetText("Shift-click / drag a box to select · blue ports connect · Esc cancels")
		}
		v.OnResize()
	}
}
func (v *ProcessEditor) EnumProperties(p core.IPropertyList) {
	p.AddProperty("Project JSON", v.ProjectJSON, v.SetProjectJSON)
	p.AddProperty("Device library JSON", v.DeviceLibraryJSON, v.SetDeviceLibraryJSON)
}
func (v *ProcessEditor) SizeHints() SizeHints {
	return SizeHints{Width: 1040, Height: 720, Policy: GrowHorizontal | GrowVertical}
}

// ProcessEditor is a complete composite, not an unimplemented Widget. Draw its
// own theme-aware surface so debug placeholder crosses and pale fallback fills
// never leak behind the toolbar/status text, including in private modal wrappers.
func (v *ProcessEditor) Draw(g paint.Painter) {
	g.SetBrush1(Theme().FormColor)
	g.Rectangle(0, 0, v.Width(), v.Height())
	g.Fill()
}

func (v *ProcessEditor) OnResize() {
	w, h := v.Size()
	if v.running {
		v.Diagram.SetBounds(0, 0, w, max(0, h-28))
		v.status.SetBounds(8, max(0, h-26), max(0, w-16), 24)
		return
	}
	cols := max(1, int(max(0, w-16)/126))
	rows := (len(v.buttons) + cols - 1) / cols
	bw := max(0, (w-16)/float64(cols))
	for i, b := range v.buttons {
		b.SetBounds(8+float64(i%cols)*bw, 8+float64(i/cols)*36, max(0, bw-5), 30)
	}
	top := 16 + float64(rows)*36
	v.Diagram.SetBounds(0, top, w, max(0, h-top-28))
	v.status.SetBounds(8, max(top, h-26), max(0, w-16), 24)
}
func (v *ProcessEditor) Close() {
	if v.closed {
		return
	}
	v.closed = true
	if v.closeDeviceLibrary != nil {
		v.closeDeviceLibrary()
	}
	v.SetReadOnly(true)
	for _, b := range v.buttons {
		b.HideSubPopup()
	}
	v.Diagram.Close()
	v.changed = nil
}

// EditRows creates an isolated staged table. Apply validates the complete
// candidate; Cancel never mutates the live document. Rows carry explicit IDs
// so sorting cannot silently address another point/source/node.
const processIntegerExpressionHelp = "Integer + - * and comparisons are exact. Bit/remainder operators (& | ^ &^ << >> %) require int; / remains floating-point. Example: (x & 8) != 0."

func (v *ProcessEditor) editRows(title string, headers []string, rows [][]string, newRow []string, apply func([][]string) error, help ...string) {
	if v.running || v.locked {
		return
	}
	model := NewSimpleTableModel(headers)
	for _, r := range rows {
		model.AddRow(r...)
	}
	for i := range headers {
		model.SetColumnWidth(i, 140)
	}
	table := new(processConfigTable)
	table.Init(table)
	table.SetModel(model)
	table.SetCellsEditable(true)
	instructions := "Double-click a cell to edit. Apply validates; Cancel discards."
	for _, text := range help {
		if text != "" {
			instructions += "\n" + text
		}
	}
	hint := NewLabel(instructions)
	hint.SetWrap(true)
	box := NewVBox()
	box.SetSpacing(6)
	box.AddWidget(table)
	row := NewHBox()
	row.SetSpacing(6)
	add := NewButton1("Add", nil)
	add.Action().BindFunc0(func() {
		if model.RowCount() < 512 {
			model.AddRow(newRow...)
			table.Update()
		}
	})
	del := NewButton1("Remove", nil)
	del.Action().BindFunc0(func() {
		if i := table.SelectedRow(); i >= 0 {
			model.RemoveRow(i)
			table.Update()
		}
	})
	if newRow != nil {
		row.AddWidget(add)
	}
	row.AddWidget(del)
	box.AddWidget(row)
	box.AddWidget(hint)
	dlg := NewDialog(title, v)
	dlg.SetContent(box)
	ok := dlg.AddButton("Apply", DialogOK)
	dlg.AddButton("Cancel", DialogCancel)
	// Replace the standard close action: malformed rows leave the dialog open.
	ok.Action().BindFunc0(func() {
		table.commitEdit()
		all := [][]string{}
		for i := 0; i < model.RowCount(); i++ {
			all = append(all, append([]string(nil), model.RowData(i)...))
		}
		if e := apply(all); e != nil {
			hint.SetText(e.Error() + "\n" + instructions)
			return
		}
		dlg.onBtnClick(dialogResultKey(DialogOK))
	})
	dlg.SetSize(600, 500)
	dlg.ShowModal()
}
func rawCell(r []string, i int) string {
	if i < len(r) {
		return r[i]
	}
	return ""
}
func cell(r []string, i int) string {
	if i < len(r) {
		return strings.TrimSpace(r[i])
	}
	return ""
}
func (v *ProcessEditor) ApplyPointRows(rows [][]string) error {
	p := v.Diagram.Project()
	p.Points = nil
	for _, r := range rows {
		age, e := strconv.Atoi(cell(r, 5))
		if e != nil {
			return fmt.Errorf("stale ms must be an integer")
		}
		k := point.Kind(cell(r, 1))
		initialText := cell(r, 4)
		if k == point.String {
			initialText = rawCell(r, 4)
		}
		initial, e := point.Convert(initialText, k)
		if e != nil {
			return e
		}
		p.Points = append(p.Points, hmi.Point{Definition: point.Definition{Name: cell(r, 0), Kind: k, Category: cell(r, 2), Unit: cell(r, 3), Description: rawCell(r, 6)}, Initial: initial, StaleMS: age})
	}
	return v.Diagram.ApplyProject(p)
}
func (v *ProcessEditor) EditPoints() {
	rows := [][]string{}
	for _, p := range v.Diagram.Project().Points {
		initial := ""
		if p.Initial != nil {
			initial = fmt.Sprint(p.Initial)
		}
		rows = append(rows, []string{p.Name, string(p.Kind), p.Category, p.Unit, initial, strconv.Itoa(p.StaleMS), p.Description})
	}
	v.editRows("Industrial points", []string{"Name", "Type", "DI / AI / BI", "Unit", "Initial", "Stale ms (0 off)", "Description"}, rows, []string{"AI_NEW", "float", "AI", "", "0", "5000", ""}, v.ApplyPointRows)
}

// Sources and mappings are in separate tables: adding an endpoint does not
// require writing JSON. Runtime credentials are intentionally not editable here.
func (v *ProcessEditor) ApplySourceRows(rows [][]string) error {
	p := v.Diagram.Project()
	old := map[string]hmi.Source{}
	for _, s := range p.Sources {
		old[s.ID] = s
	}
	p.Sources = nil
	for _, r := range rows {
		s := old[cell(r, 0)]
		s.ID = cell(r, 0)
		s.URL = cell(r, 1)
		s.Method = cell(r, 2)
		var e error
		s.IntervalMS, e = strconv.Atoi(cell(r, 3))
		if e != nil {
			return e
		}
		s.TimeoutMS, e = strconv.Atoi(cell(r, 4))
		if e != nil {
			return e
		}
		s.Enabled, e = strconv.ParseBool(cell(r, 5))
		if e != nil {
			return e
		}
		s.RepeatWrites, e = strconv.ParseBool(cell(r, 6))
		if e != nil {
			return e
		}
		s.Body = rawCell(r, 7)
		if len(s.Mappings) == 0 {
			for _, pt := range p.Points {
				s.Mappings = []point.Mapping{{Name: pt.Name, Kind: pt.Kind, Pointer: "/data/value"}}
				break
			}
		}
		p.Sources = append(p.Sources, s)
	}
	return v.Diagram.ApplyProject(p)
}
func (v *ProcessEditor) EditSources() {
	p := v.Diagram.Project()
	rows := [][]string{}
	for _, s := range p.Sources {
		rows = append(rows, []string{s.ID, s.URL, s.Method, strconv.Itoa(s.IntervalMS), strconv.Itoa(s.TimeoutMS), strconv.FormatBool(s.Enabled), strconv.FormatBool(s.RepeatWrites), s.Body})
	}
	v.editRows("HTTP endpoints (response fields via HTTP mappings)", []string{"ID", "URL", "Method", "Interval ms", "Timeout ms", "Enabled", "Repeat writes consent", "Request body"}, rows, []string{"HTTP_NEW", "http://127.0.0.1:8080/status", "GET", "1000", "5000", "false", "false", ""}, v.ApplySourceRows)
}
func (v *ProcessEditor) ApplyMappingRows(rows [][]string) error {
	p := v.Diagram.Project()
	for i := range p.Sources {
		p.Sources[i].Mappings = nil
	}
	for _, r := range rows {
		found := false
		for i := range p.Sources {
			if p.Sources[i].ID == cell(r, 0) {
				found = true
				var k point.Kind
				for _, d := range p.Points {
					if d.Name == cell(r, 1) {
						k = d.Kind
					}
				}
				steps, err := rowSteps(r, 5, mappingSteps(r))
				if err != nil {
					return err
				}
				p.Sources[i].Mappings = append(p.Sources[i].Mappings, point.Mapping{Name: cell(r, 1), Kind: k, Pointer: cell(r, 2), Steps: steps})
			}
		}
		if !found {
			return fmt.Errorf("unknown source %s", cell(r, 0))
		}
	}
	return v.Diagram.ApplyProject(p)
}
func (v *ProcessEditor) EditMappings() {
	rows := [][]string{}
	for _, s := range v.Diagram.Project().Sources {
		for _, m := range s.Mappings {
			rows = append(rows, []string{s.ID, m.Name, m.Pointer, mappingType(m.Steps), mappingExpr(m.Steps), advancedMapping(m.Steps)})
		}
	}
	if len(v.Diagram.Project().Sources) == 0 {
		return
	}
	v.editRows("HTTP response mappings", []string{"Source ID", "Point", "JSON Pointer", "Convert type", "Expression", "Advanced pipeline JSON (overrides)"}, rows, []string{"HTTP_NEW", "AI101", "/data/value", "", "", ""}, v.ApplyMappingRows, processIntegerExpressionHelp)
}
func (v *ProcessEditor) selectedIndex(p hmi.Project) int {
	for i, n := range p.Nodes {
		if n.ID == v.Diagram.SelectedNode() {
			return i
		}
	}
	return -1
}
func (v *ProcessEditor) ApplyNodeRows(rows [][]string) error {
	p := v.Diagram.Project()
	old := map[string]hmi.Node{}
	for _, n := range p.Nodes {
		old[n.ID] = n
	}
	var nodes []hmi.Node
	seen := map[string]bool{}
	for _, r := range rows {
		n, found := old[cell(r, 0)]
		if !found || seen[n.ID] {
			return fmt.Errorf("unknown or duplicate node")
		}
		seen[n.ID] = true
		n.Label = rawCell(r, 1)
		n.Tag = cell(r, 2)
		n.Unit = cell(r, 3)
		vals := []*float64{&n.X, &n.Y, &n.Width, &n.Height}
		for j, dst := range vals {
			f, e := strconv.ParseFloat(cell(r, 4+j), 64)
			if e != nil {
				return e
			}
			*dst = f
		}
		nodes = append(nodes, n)
	}
	p.Nodes = nodes
	links := p.Links[:0]
	for _, l := range p.Links {
		if seen[l.From.Node] && seen[l.To.Node] {
			links = append(links, l)
		}
	}
	p.Links = links
	return v.Diagram.ApplyProject(p)
}
func (v *ProcessEditor) EditNode() {
	rows := [][]string{}
	for _, n := range v.Diagram.Project().Nodes {
		rows = append(rows, []string{n.ID, n.Label, n.Tag, n.Unit, fmt.Sprint(n.X), fmt.Sprint(n.Y), fmt.Sprint(n.Width), fmt.Sprint(n.Height)})
	}
	v.editRows("Node properties / point binding", []string{"ID (existing)", "Label", "Point", "Unit", "X", "Y", "Width", "Height"}, rows, nil, v.ApplyNodeRows)
}
func (v *ProcessEditor) ApplyTransformRows(rows [][]string) error {
	p := v.Diagram.Project()
	i := v.selectedIndex(p)
	if i < 0 {
		return fmt.Errorf("select a node first")
	}
	p.Nodes[i].Steps = nil
	for _, r := range rows {
		p.Nodes[i].Steps = append(p.Nodes[i].Steps, point.Step{Type: point.Kind(cell(r, 0)), Expr: cell(r, 1)})
	}
	return v.Diagram.ApplyProject(p)
}
func (v *ProcessEditor) EditTransforms() {
	p := v.Diagram.Project()
	i := v.selectedIndex(p)
	if i < 0 {
		v.setError(fmt.Errorf("select a node first"))
		return
	}
	rows := [][]string{}
	for _, s := range p.Nodes[i].Steps {
		rows = append(rows, []string{string(s.Type), s.Expr})
	}
	v.editRows("Conversion pipeline (type OR expression)", []string{"Type: float/int/bool/string", "Expression over x"}, rows, []string{"float", ""}, v.ApplyTransformRows, processIntegerExpressionHelp)
}
func (v *ProcessEditor) ApplyColorRows(rows [][]string) error {
	p := v.Diagram.Project()
	i := v.selectedIndex(p)
	if i < 0 {
		return fmt.Errorf("select a node first")
	}
	p.Nodes[i].Colors = nil
	for _, r := range rows {
		p.Nodes[i].Colors = append(p.Nodes[i].Colors, hmi.ColorRule{When: cell(r, 0), Color: cell(r, 1), Text: rawCell(r, 2)})
	}
	return v.Diagram.ApplyProject(p)
}
func (v *ProcessEditor) EditColors() {
	p := v.Diagram.Project()
	i := v.selectedIndex(p)
	if i < 0 {
		v.setError(fmt.Errorf("select a node first"))
		return
	}
	rows := [][]string{}
	for _, r := range p.Nodes[i].Colors {
		rows = append(rows, []string{r.When, r.Color, r.Text})
	}
	v.editRows("Ordered color / state rules", []string{"Condition over x", "#RRGGBB", "State text (optional)"}, rows, []string{"x >= 80", "#ffb300", ""}, v.ApplyColorRows, "Conditions must return bool; the first matching rule wins.", processIntegerExpressionHelp)
}
func (v *ProcessEditor) ApplyBindingRows(rows [][]string) error {
	p := v.Diagram.Project()
	i := v.selectedIndex(p)
	if i < 0 {
		return fmt.Errorf("select a node first")
	}
	p.Nodes[i].Bindings = nil
	for _, r := range rows {
		steps := []point.Step{}
		if cell(r, 2) != "" {
			steps = append(steps, point.Step{Expr: cell(r, 2)})
		}
		var err error
		steps, err = rowSteps(r, 3, steps)
		if err != nil {
			return err
		}
		p.Nodes[i].Bindings = append(p.Nodes[i].Bindings, hmi.PropertyBinding{Tag: cell(r, 0), Property: cell(r, 1), Steps: steps})
	}
	return v.Diagram.ApplyProject(p)
}
func (v *ProcessEditor) EditBindings() {
	p := v.Diagram.Project()
	i := v.selectedIndex(p)
	if i < 0 {
		v.setError(fmt.Errorf("select a node first"))
		return
	}
	rows := [][]string{}
	for _, b := range p.Nodes[i].Bindings {
		expr := ""
		if len(b.Steps) > 0 {
			expr = b.Steps[0].Expr
		}
		advanced := ""
		if len(b.Steps) > 1 || len(b.Steps) == 1 && b.Steps[0].Type != "" {
			data, _ := json.Marshal(b.Steps)
			advanced = string(data)
		}
		rows = append(rows, []string{b.Tag, b.Property, expr, advanced})
	}
	v.editRows("Dynamic visual properties", []string{"Point", "visible/enabled/x/y/rotation", "Expression (optional)", "Advanced pipeline JSON (overrides)"}, rows, []string{"DI11", "visible", "x", ""}, v.ApplyBindingRows, processIntegerExpressionHelp)
}
func (v *ProcessEditor) ApplyLinkRows(rows [][]string) error {
	p := v.Diagram.Project()
	old := map[string][]hmi.XY{}
	for _, l := range p.Links {
		old[l.ID] = l.Bends
	}
	p.Links = nil
	for _, r := range rows {
		p.Links = append(p.Links, hmi.Link{Bends: old[cell(r, 0)], ID: cell(r, 0), From: hmi.Endpoint{Node: cell(r, 1), Port: cell(r, 2)}, To: hmi.Endpoint{Node: cell(r, 3), Port: cell(r, 4)}, Tag: cell(r, 5)})
	}
	return v.Diagram.ApplyProject(p)
}
func (v *ProcessEditor) EditLinks() {
	rows := [][]string{}
	for _, l := range v.Diagram.Project().Links {
		rows = append(rows, []string{l.ID, l.From.Node, l.From.Port, l.To.Node, l.To.Port, l.Tag})
	}
	v.editRows("Pipe connections / automatic orthogonal routes", []string{"ID", "From node", "From port", "To node", "To port", "Flow point"}, rows, []string{"pipe_new", "reactor", "east", "exchanger", "west", "DI11"}, v.ApplyLinkRows)
}

// ConfigJSON is useful for an external source editor without leaking credentials.
func (v *ProcessEditor) ConfigJSON() string {
	b, _ := json.Marshal(v.Diagram.Project().Sources)
	return string(b)
}

func mappingSteps(r []string) []point.Step {
	var out []point.Step
	if cell(r, 3) != "" {
		out = append(out, point.Step{Type: point.Kind(cell(r, 3))})
	}
	if cell(r, 4) != "" {
		out = append(out, point.Step{Expr: cell(r, 4)})
	}
	return out
}
func mappingType(s []point.Step) string {
	for _, p := range s {
		if p.Type != "" {
			return string(p.Type)
		}
	}
	return ""
}
func mappingExpr(s []point.Step) string {
	for _, p := range s {
		if p.Expr != "" {
			return p.Expr
		}
	}
	return ""
}
func (v *ProcessEditor) ApplyHeaderRows(rows [][]string) error {
	p := v.Diagram.Project()
	for i := range p.Sources {
		p.Sources[i].Headers = map[string]string{}
	}
	for _, r := range rows {
		found := false
		for i := range p.Sources {
			if p.Sources[i].ID == cell(r, 0) {
				found = true
				p.Sources[i].Headers[cell(r, 1)] = rawCell(r, 2)
			}
		}
		if !found {
			return fmt.Errorf("unknown HTTP source")
		}
	}
	return v.Diagram.ApplyProject(p)
}
func (v *ProcessEditor) EditHeaders() {
	rows := [][]string{}
	for _, s := range v.Diagram.Project().Sources {
		for k, value := range s.Headers {
			rows = append(rows, []string{s.ID, k, value})
		}
	}
	v.editRows("HTTP headers (inject credentials at runtime)", []string{"Source ID", "Header", "Value"}, rows, []string{"HTTP_NEW", "Content-Type", "application/json"}, v.ApplyHeaderRows)
}

func (v *ProcessEditor) ApplyBendRows(rows [][]string) error {
	p := v.Diagram.Project()
	for i := range p.Links {
		p.Links[i].Bends = nil
	}
	for _, r := range rows {
		x, e := strconv.ParseFloat(cell(r, 1), 64)
		if e != nil {
			return e
		}
		y, e := strconv.ParseFloat(cell(r, 2), 64)
		if e != nil {
			return e
		}
		found := false
		for i := range p.Links {
			if p.Links[i].ID == cell(r, 0) {
				found = true
				p.Links[i].Bends = append(p.Links[i].Bends, hmi.XY{X: x, Y: y})
			}
		}
		if !found {
			return fmt.Errorf("unknown pipe")
		}
	}
	return v.Diagram.ApplyProject(p)
}
func (v *ProcessEditor) EditBends() {
	rows := [][]string{}
	for _, l := range v.Diagram.Project().Links {
		for _, b := range l.Bends {
			rows = append(rows, []string{l.ID, fmt.Sprint(b.X), fmt.Sprint(b.Y)})
		}
	}
	v.editRows("Ordered pipe bend handles", []string{"Pipe ID", "X", "Y"}, rows, []string{"feed", "350", "300"}, v.ApplyBendRows)
}

// Advanced pipelines remain lossless when a document was authored through the SDK.
func advancedMapping(steps []point.Step) string {
	simple := []point.Step{}
	if typ := mappingType(steps); typ != "" {
		simple = append(simple, point.Step{Type: point.Kind(typ)})
	}
	if expr := mappingExpr(steps); expr != "" {
		simple = append(simple, point.Step{Expr: expr})
	}
	a, _ := json.Marshal(simple)
	b, _ := json.Marshal(steps)
	if string(a) == string(b) || len(steps) == 0 {
		return ""
	}
	return string(b)
}
func rowSteps(row []string, column int, fallback []point.Step) ([]point.Step, error) {
	if cell(row, column) == "" {
		return fallback, nil
	}
	var steps []point.Step
	if len(rawCell(row, column)) > 65536 {
		return nil, fmt.Errorf("pipeline JSON exceeds 64 KiB")
	}
	decoder := json.NewDecoder(strings.NewReader(rawCell(row, column)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&steps); err != nil {
		return nil, err
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, fmt.Errorf("trailing pipeline data")
	}
	if _, err := point.NewPipeline(steps); err != nil {
		return nil, err
	}
	return steps, nil
}

// SetReadOnly locks authoring without entering live-data preview mode, e.g.
// while an application-owned document load/save transaction is in progress.
func (v *ProcessEditor) SetReadOnly(b bool) {
	if b && v.closeDeviceLibrary != nil {
		v.closeDeviceLibrary()
	}
	v.locked = b
	v.Diagram.SetReadOnly(b)
	for _, button := range v.buttons {
		button.SetEnabled(!v.running && !b)
		if b {
			button.HideSubPopup()
		}
	}
	v.refreshActions()
}
