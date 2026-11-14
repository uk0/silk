package gui

import (
	"encoding/json"
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestProcessDiagramDragPortsHistoryCancelAndPersistence(t *testing.T) {
	v := NewProcessDiagram()
	defer v.Close()
	v.SetEditing(true)
	before := v.ProjectJSON()
	n := v.Project().Nodes[0]
	v.OnLeftDown(n.X+n.Width/2, n.Y+n.Height/2)
	v.OnMouseMove(n.X+n.Width/2+47, n.Y+n.Height/2+31)
	v.OnLeftUp(n.X+n.Width/2+47, n.Y+n.Height/2+31)
	if v.Project().Nodes[0].X != 140 {
		t.Fatal("snapped drag missing", v.Project().Nodes[0])
	}
	v.Undo()
	if v.ProjectJSON() != before {
		t.Fatal("undo not exact")
	}
	v.Redo()
	dragged := v.ProjectJSON()
	v.OnLeftDown(180, 200)
	v.OnMouseMove(220, 210)
	v.OnPointerCancel()
	if v.ProjectJSON() != dragged || v.views["reactor"].node.X != 140 {
		t.Fatal("cancel committed")
	}
	p := v.Project()
	a, b := hmi.Port(p.Nodes[0], "north"), hmi.Port(p.Nodes[1], "north")
	v.OnLeftDown(a.X, a.Y)
	v.OnLeftDown(b.X, b.Y)
	if len(v.Project().Links) != 3 {
		t.Fatal("port connection missing")
	}
	v.DeleteSelected()
	if v.LastError() != nil {
		t.Fatal(v.LastError())
	}
}
func TestProcessDiagramBindingsAreTransientAndQualityAware(t *testing.T) {
	isolateTagBridge(t)
	p := hmi.Template()
	p.Nodes[0].Bindings = []hmi.PropertyBinding{{Tag: "AI101", Property: "x", Steps: []point.Step{{Expr: "x*2"}}}, {Tag: "DI11", Property: "visible"}}
	v := NewProcessDiagram()
	defer v.Close()
	if e := v.SetProject(p); e != nil {
		t.Fatal(e)
	}
	saved := v.ProjectJSON()
	db := core.NewTagDB()
	db.SetValue("DI11", true)
	db.SetValue("AI101", 92.8)
	if _, e := v.BindTags(db); e != nil {
		t.Fatal(e)
	}
	drainUITasks()
	if v.views["temperature"].widget.(*ProcessValue).DisplayText() != "92.8 °C" || v.views["reactor"].node.X != 185.6 {
		t.Fatal("bound props missing")
	}
	if v.ProjectJSON() != saved {
		t.Fatal("runtime position persisted")
	}
	db.SetValue("DI11", false)
	drainUITasks()
	if v.views["reactor"].visible {
		t.Fatal("visibility binding ignored")
	}
	v.UnbindTags()
	db.SetValue("AI101", 999.)
	drainUITasks()
	if v.views["reactor"].node.X == 999 {
		t.Fatal("cancel failed")
	}
	v.SetSize(800, 500)
	pm := paint.NewPixmap(820, 520)
	g := pm.NewPainter()
	v.Draw(g)
	paint.ClosePainter(g)
	im, _ := pm.Image()
	_, _, _, a := im.At(819, 519).RGBA()
	paint.CloseSurface(pm)
	if a != 0 {
		t.Fatal("canvas escaped clip")
	}
}
func TestProcessEditorStagedManagersValidateWithoutIO(t *testing.T) {
	e := NewProcessEditor()
	defer e.Close()
	before := e.ProjectJSON()
	if err := e.ApplyPointRows([][]string{{"bad", "float", "AI", "", "oops", "0", ""}}); err == nil || e.ProjectJSON() != before {
		t.Fatal("invalid point mutated document")
	}
	if err := e.ApplySourceRows([][]string{{"http", "http://127.0.0.1:1/status", "POST", "1000", "500", "false", "false", `{"device":"R1"}`}}); err != nil {
		t.Fatal(err)
	}
	if err := e.ApplyMappingRows([][]string{{"http", "AI101", "/data/value", "float", "x*0.1"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.ApplyHeaderRows([][]string{{"http", "Content-Type", "application/json"}}); err != nil {
		t.Fatal(err)
	}
	saved := e.ProjectJSON()
	if err := e.ApplyHeaderRows([][]string{{"http", "Authorization", "secret"}}); err == nil || e.ProjectJSON() != saved {
		t.Fatal("credential persisted")
	}
	e.Diagram.selected = "temperature"
	if err := e.ApplyTransformRows([][]string{{"float", ""}, {"", "x*2"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.ApplyColorRows([][]string{{"x>=80", "#ff0000", "Hot"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.ApplyBindingRows([][]string{{"DI11", "enabled", "x"}}); err != nil {
		t.Fatal(err)
	}
	p := e.Diagram.Project()
	if len(p.Sources) != 1 || len(p.Nodes[3].Colors) != 1 {
		t.Fatal("manager config lost")
	}
	bytes, err := json.Marshal(p)
	if err != nil || strings.Contains(string(bytes), "secret") {
		t.Fatal("bad serialization")
	}
}
func TestTagUIAuthoritativeSnapshotAfterCallbackReordering(t *testing.T) {
	isolateTagBridge(t)
	tag := core.NewTagDB().GetOrCreate("ordered", core.Meta{})
	first, release := make(chan struct{}), make(chan struct{})
	stopBlock := tag.Subscribe(func(v core.Value) {
		if v.Raw == 1 {
			close(first)
			<-release
		}
	})
	defer stopBlock()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); tag.SetValue(1) }()
	<-first
	got := 0
	stop := BindTagUI(tag, func(v core.Value) { got = int(v.Float()) })
	defer stop()
	tag.SetValue(2)
	close(release)
	wg.Wait()
	drainUITasks()
	if got != 2 {
		t.Fatal("late subscriber callback overwrote current sample", got)
	}
}

func TestProcessDiagramOffGridClickZoomPanAndHistoryBudget(t *testing.T) {
	v := NewProcessDiagram()
	defer v.Close()
	v.SetEditing(true)
	p := v.Project()
	p.Nodes[0].X = 93
	p.Nodes[0].Y = 63
	if err := v.SetProject(p); err != nil {
		t.Fatal(err)
	}
	before := v.ProjectJSON()
	v.OnLeftDown(140, 200)
	v.OnLeftUp(140, 200)
	if before != v.ProjectJSON() {
		t.Fatal("selection snapped authored node")
	}
	v.OnMouseWheel(300, 240, 2)
	if math.Abs((300-v.panX)/v.zoom-300) > 1e-9 {
		t.Fatal("zoom anchor drift")
	}
	oldX := v.panX
	v.OnMiddleDown(20, 20)
	v.OnMouseMove(60, 70)
	v.OnMiddleUp(60, 70)
	if v.panX != oldX+40 {
		t.Fatal("pan ignored")
	}
	a := hmi.Port(p.Nodes[0], "north")
	b := hmi.Port(p.Nodes[1], "north")
	v.OnLeftDown(a.X*v.zoom+v.panX, a.Y*v.zoom+v.panY)
	v.OnLeftDown(b.X*v.zoom+v.panX, b.Y*v.zoom+v.panY)
	if len(v.Project().Links) != 3 {
		t.Fatal("zoomed port hit test failed")
	}
	v.selected = "reactor"
	if err := v.DuplicateSelected(); err != nil {
		t.Fatal(err)
	}
	if v.SelectedNode() != "reactor_copy1" {
		t.Fatal("duplicate missing")
	}
	for i := 0; i < 40; i++ {
		p = v.Project()
		p.Title = fmt.Sprint(i)
		if err := v.ApplyProject(p); err != nil {
			t.Fatal(err)
		}
	}
	if len(v.undo) != 32 {
		t.Fatal("unbounded history", len(v.undo))
	}
	v.ClearHistory()
	if len(v.undo) != 0 || len(v.redo) != 0 {
		t.Fatal("new document kept history")
	}
}
func TestProcessEditorLosslessAdvancedPipelinesAndNodeRemoval(t *testing.T) {
	e := NewProcessEditor()
	defer e.Close()
	if err := e.ApplySourceRows([][]string{{"http", "http://127.0.0.1:1", "GET", "1000", "100", "false", "false", ""}}); err != nil {
		t.Fatal(err)
	}
	steps := []point.Step{{Expr: "x*2"}, {Type: point.Float}, {Expr: "x+1"}}
	full := advancedMapping(steps)
	if full == "" {
		t.Fatal("complex pipeline hidden")
	}
	if err := e.ApplyMappingRows([][]string{{"http", "AI101", "/v", "float", "x*2", full}}); err != nil {
		t.Fatal(err)
	}
	got := e.Diagram.Project().Sources[0].Mappings[0].Steps
	if !reflect.DeepEqual(got, steps) {
		t.Fatal("mapping lost steps", got)
	}
	e.Diagram.selected = "reactor"
	if err := e.ApplyBindingRows([][]string{{"AI101", "x", "", full}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e.Diagram.Project().Nodes[0].Bindings[0].Steps, steps) {
		t.Fatal("binding lost steps")
	}
	p := e.Diagram.Project()
	rows := [][]string{}
	for _, n := range p.Nodes[1:] {
		rows = append(rows, []string{n.ID, n.Label, n.Tag, n.Unit, fmt.Sprint(n.X), fmt.Sprint(n.Y), fmt.Sprint(n.Width), fmt.Sprint(n.Height)})
	}
	if err := e.ApplyNodeRows(rows); err != nil {
		t.Fatal(err)
	}
	p = e.Diagram.Project()
	if len(p.Nodes) != 3 || len(p.Links) != 1 {
		t.Fatal("remove row did not remove node and incident pipe")
	}
}
func TestProcessDiagramEquipmentPortsAndRuntimeRuleFailures(t *testing.T) {
	v := NewProcessDiagram()
	defer v.Close()
	for _, kind := range []string{"pump", "valve", "tank", "indicator"} {
		p := hmi.Template()
		p.Nodes[0].Kind = kind
		p.Nodes[0].Height = 180
		if err := v.SetProject(p); err != nil {
			t.Fatal(err)
		}
		n := p.Nodes[0]
		port := hmi.Port(n, "east")
		if port.Y != n.Y+(n.Height-26)/2 {
			t.Fatal("nozzle / pipe mismatch", kind, port)
		}
	}
	p := hmi.Template()
	p.Nodes[0].Colors = []hmi.ColorRule{{When: "x/0>1", Color: "#ff0000"}}
	if err := v.SetProject(p); err != nil {
		t.Fatal(err)
	}
	if v.views["reactor"].widget.(*ProcessEquipment).quality != core.QualityBad {
		t.Fatal("invalid rule looked normal")
	}
}
func BenchmarkProcessDiagramDragPreview(b *testing.B) {
	v := NewProcessDiagram()
	defer v.Close()
	v.SetEditing(true)
	v.OnLeftDown(150, 200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.OnMouseMove(160+float64(i%100), 220)
	}
}
