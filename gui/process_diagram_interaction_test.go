package gui

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/point"
)

func editableProcess(t *testing.T) *ProcessDiagram {
	t.Helper()
	v := NewProcessDiagram()
	v.SetEditing(true)
	v.SetSize(1000, 620)
	t.Cleanup(v.Close)
	return v
}
func TestProcessMultiSelectionMarqueeTogglesAndGroupDrag(t *testing.T) {
	v := editableProcess(t)
	v.pointerDown(150, 200, processModifiers{})
	v.pointerUp(150, 200, processModifiers{})
	v.pointerDown(515, 175, processModifiers{shift: true})
	v.pointerUp(515, 175, processModifiers{shift: true})
	if !reflect.DeepEqual(v.SelectedNodes(), []string{"reactor", "exchanger"}) {
		t.Fatal(v.SelectedNodes())
	}
	p := v.Project()
	p.Links[0].Bends = []hmi.XY{{X: 350, Y: 240}}
	if e := v.SetProject(p); e != nil {
		t.Fatal(e)
	}
	before := v.ProjectJSON()
	v.pointerDown(515, 175, processModifiers{})
	v.pointerMove(562, 206, processModifiers{})
	if v.ProjectJSON() != before || v.views["reactor"].node.X != 140 || v.views["exchanger"].node.X != 450 || v.linkBends(v.project.Links[0])[0] != (hmi.XY{X: 400, Y: 270}) {
		t.Fatal("group preview changed spacing/document")
	}
	v.pointerUp(562, 206, processModifiers{})
	if len(v.undo) != 1 {
		t.Fatal("one drag should create one undo", len(v.undo))
	}
	v.Undo()
	if v.ProjectJSON() != before {
		t.Fatal("group undo not exact")
	}
	v.Redo()
	v.pointerDown(565, 205, processModifiers{})
	v.pointerMove(-1000, 205, processModifiers{})
	v.OnPointerCancel()
	if v.views["reactor"].node.X != 140 {
		t.Fatal("cancel kept preview")
	}
	v.SelectNodes(nil)
	v.pointerDown(30, 30, processModifiers{})
	v.pointerMove(700, 420, processModifiers{})
	v.pointerUp(700, 420, processModifiers{})
	if !reflect.DeepEqual(v.SelectedNodes(), []string{"reactor", "exchanger"}) {
		t.Fatal("marquee mismatch", v.SelectedNodes())
	}
	v.pointerDown(565, 205, processModifiers{shift: true})
	v.pointerUp(565, 205, processModifiers{shift: true})
	if !reflect.DeepEqual(v.SelectedNodes(), []string{"reactor"}) {
		t.Fatal("toggle did not remove", v.SelectedNodes())
	}
}
func TestProcessResizeCornersArePreviewOnlyCancelableAndAspectBounded(t *testing.T) {
	v := editableProcess(t)
	v.SelectNodes([]string{"reactor"})
	before := v.ProjectJSON()
	v.pointerDown(220, 380, processModifiers{})
	v.pointerMove(267, 411, processModifiers{})
	if v.views["reactor"].node.Width != 180 || v.ProjectJSON() != before {
		t.Fatal("resize preview missing or persisted")
	}
	v.OnPointerCancel()
	if w, _ := v.views["reactor"].widget.Size(); w != 130 {
		t.Fatal("cancel failed to restore child size", w)
	}
	v.pointerDown(220, 380, processModifiers{})
	v.pointerMove(267, 411, processModifiers{})
	v.pointerUp(267, 411, processModifiers{})
	if v.Project().Nodes[0].Width != 180 || v.Project().Nodes[0].Height != 350 {
		t.Fatal("resize commit wrong", v.Project().Nodes[0])
	}
	v.Undo()
	if v.ProjectJSON() != before {
		t.Fatal("resize undo")
	}
	v.pointerDown(220, 380, processModifiers{})
	v.pointerMove(10000, 10000, processModifiers{shift: true})
	n := v.views["reactor"].node
	if n.X+n.Width > 1000 || n.Y+n.Height > 620 || math.Abs(n.Width/n.Height-130./320) > 1e-10 {
		t.Fatal("aspect resize escaped bounds", n)
	}
	v.OnPointerCancel()
}
func TestProcessPipeWaypointPickingDraggingAndDeletion(t *testing.T) {
	v := editableProcess(t)
	v.pointerDown(310, 220, processModifiers{})
	v.pointerUp(310, 220, processModifiers{})
	if v.SelectedLink() != "feed" {
		t.Fatal("pipe not selected", v.SelectedLink())
	}
	if e := v.AddSelectedPipeBend(); e != nil {
		t.Fatal(e)
	}
	p := v.Project()
	b := p.Links[0].Bends[0]
	before := v.ProjectJSON()
	v.pointerDown(b.X, b.Y, processModifiers{})
	v.pointerMove(b.X+37, b.Y+24, processModifiers{})
	if v.ProjectJSON() != before {
		t.Fatal("bend preview persisted")
	}
	v.OnPointerCancel()
	if v.ProjectJSON() != before {
		t.Fatal("bend cancel")
	}
	v.pointerDown(b.X, b.Y, processModifiers{})
	v.pointerMove(b.X+37, b.Y+24, processModifiers{})
	v.pointerUp(b.X+37, b.Y+24, processModifiers{})
	if v.Project().Links[0].Bends[0] == b {
		t.Fatal("bend did not move")
	}
	v.Undo()
	if v.ProjectJSON() != before {
		t.Fatal("bend undo")
	}
	v.selectedBend = 0
	v.processKey(KeyDelete, false, processModifiers{})
	if len(v.Project().Links[0].Bends) != 0 || len(v.Project().Links) != 2 {
		t.Fatal("waypoint delete removed pipe")
	}
	v.processKey(KeyDelete, false, processModifiers{})
	if len(v.Project().Links) != 1 {
		t.Fatal("pipe delete failed")
	}
}
func TestProcessKeyboardBatchCloneInternalPipesAndReadOnlyGuard(t *testing.T) {
	v := editableProcess(t)
	before := v.ProjectJSON()
	v.processKey('A', false, processModifiers{action: true})
	v.processKey('D', false, processModifiers{action: true})
	if len(v.Project().Nodes) != 8 || len(v.Project().Links) != 4 {
		t.Fatal("subgraph clone lost internal pipes")
	}
	v.processKey('Z', false, processModifiers{action: true})
	if v.ProjectJSON() != before {
		t.Fatal("keyboard undo")
	}
	v.processKey('Z', false, processModifiers{action: true, shift: true})
	if len(v.Project().Nodes) != 8 {
		t.Fatal("keyboard redo")
	}
	v.SelectNodes([]string{"reactor"})
	v.processKey(KeyRight, false, processModifiers{shift: true})
	if v.Project().Nodes[0].X != 100 {
		t.Fatal("keyboard nudge")
	}
	snapshot := v.ProjectJSON()
	v.SetReadOnly(true)
	v.processKey(KeyDelete, false, processModifiers{})
	v.Undo()
	if v.ProjectJSON() != snapshot || v.DuplicateSelected() == nil {
		t.Fatal("read-only mutation")
	}
}
func TestProcessAuthoringShowsNodesHiddenOrMovedByLiveBindings(t *testing.T) {
	v := editableProcess(t)
	p := v.Project()
	p.Points[0].Initial = false
	p.Nodes[0].Bindings = []hmi.PropertyBinding{{Tag: "DI11", Property: "visible"}, {Tag: "AI101", Property: "x", Steps: []point.Step{{Expr: "x*2"}}}}
	if e := v.SetProject(p); e != nil {
		t.Fatal(e)
	}
	if !v.views["reactor"].visible || v.views["reactor"].node.X != 90 {
		t.Fatal("live bindings hijacked authoring")
	}
	v.SetEditing(false)
	if v.views["reactor"].visible || v.views["reactor"].node.X != 145 {
		t.Fatal("live properties not applied")
	}
	v.SetEditing(true)
	if !v.views["reactor"].visible || v.views["reactor"].node.X != 90 {
		t.Fatal("authoring geometry not restored")
	}
}
func TestProcessEditorPreservesRawTextBodyAndRejectsBadAdvancedJSON(t *testing.T) {
	e := NewProcessEditor()
	defer e.Close()
	rows := [][]string{{"DI11", "bool", "DI", "", "true", "0", ""}, {"AI101", "float", "AI", "°C", "72.5", "0", ""}, {"STR1", "string", "BI", "", "  exact text  ", "0", ""}}
	if err := e.ApplyPointRows(rows); err != nil {
		t.Fatal(err)
	}
	if e.Diagram.Project().Points[2].Initial != "  exact text  " {
		t.Fatal("string initial trimmed")
	}
	body := "\n {\"device\":\"R1\"} \n"
	if err := e.ApplySourceRows([][]string{{"http", "http://127.0.0.1:1", "POST", "1000", "1000", "false", "false", body}}); err != nil {
		t.Fatal(err)
	}
	if e.Diagram.Project().Sources[0].Body != body {
		t.Fatal("request body altered")
	}
	for _, advanced := range []string{`[{"expr":"x","typo":true}]`, `[{"expr":"x"}] {}`, strings.Repeat(" ", 65537) + `[]`} {
		if _, err := rowSteps([]string{advanced}, 0, nil); err == nil {
			t.Fatal("bad advanced pipeline accepted")
		}
	}
	if e.undoButton.IsEnabled() != e.Diagram.CanUndo() || e.duplicateButton.IsEnabled() {
		t.Fatal("action availability wrong")
	}
	e.SetReadOnly(true)
	if !e.Diagram.IsEditing() || !e.Diagram.IsReadOnly() {
		t.Fatal("IO lock entered live mode")
	}
}
func BenchmarkProcessMultiDragPreview(b *testing.B) {
	v := NewProcessDiagram()
	defer v.Close()
	v.SetEditing(true)
	v.SelectNodes([]string{"reactor", "exchanger"})
	v.pointerDown(515, 175, processModifiers{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.pointerMove(550+float64(i%100), 200, processModifiers{})
	}
}
