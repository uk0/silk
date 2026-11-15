package hmi

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/point"
)

func editJSON(t *testing.T, p Project) []byte {
	t.Helper()
	b, e := p.JSON()
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestEditMoveClampsWholeSubgraphAndDoesNotMutateInput(t *testing.T) {
	p := Template()
	p.Links[0].Bends = []XY{{X: 350, Y: 240}}
	before := editJSON(t, p)
	q, e := MoveNodes(p, []string{"reactor", "exchanger"}, -10000, 20)
	if e != nil {
		t.Fatal(e)
	}
	if q.Nodes[0].X != 0 || q.Nodes[1].X != 310 || q.Nodes[0].Y != 80 || q.Links[0].Bends[0] != (XY{X: 260, Y: 260}) {
		t.Fatal(q.Nodes, q.Links[0])
	}
	if !bytes.Equal(before, editJSON(t, p)) {
		t.Fatal("input mutated")
	}
	if _, e = MoveNodes(p, []string{"reactor"}, math.NaN(), 0); e == nil {
		t.Fatal("non-finite accepted")
	}
	if _, e = MoveNodes(p, []string{"reactor", "reactor"}, 1, 0); e == nil {
		t.Fatal("duplicate selection accepted")
	}
}
func TestEditCloneIsDeepAndPreservesExactInitialIntegers(t *testing.T) {
	p := Template()
	p.Points = append(p.Points, Point{Definition: point.Definition{Name: "INT1", Kind: point.Int}, Initial: int64(math.MaxInt64)})
	p.Sources = []Source{{ID: "source", URL: "http://127.0.0.1:1", Method: "GET", IntervalMS: 1000, TimeoutMS: 1000, Headers: map[string]string{"X-Test": "keep"}, Mappings: []point.Mapping{{Name: "AI101", Kind: point.Float, Steps: []point.Step{{Expr: "x*2"}}}}}}
	p.Nodes[0].Bindings = []PropertyBinding{{Tag: "AI101", Property: "x", Steps: []point.Step{{Expr: "x*2"}}}}
	p.Links[0].Bends = []XY{{X: 350, Y: 240}}
	before := editJSON(t, p)
	q, e := CloneProject(p)
	if e != nil {
		t.Fatal(e)
	}
	if q.Points[2].Initial != int64(math.MaxInt64) {
		t.Fatal("integer type or precision lost")
	}
	q.Sources[0].Headers["X-Test"] = "changed"
	q.Sources[0].Mappings[0].Steps[0].Expr = "x+1"
	q.Nodes[0].Bindings[0].Steps[0].Expr = "x+1"
	q.Nodes[3].Colors[0].Color = "#ffffff"
	q.Links[0].Bends[0].X = 99
	if !bytes.Equal(before, editJSON(t, p)) {
		t.Fatal("clone aliases input")
	}
}
func TestEditAlignDistributeResizeDeleteAndLayering(t *testing.T) {
	p := Template()
	ids := []string{"reactor", "exchanger", "separator"}
	q, e := AlignNodes(p, ids, "left")
	if e != nil || q.Nodes[2].X != 90 {
		t.Fatal(q.Nodes, e)
	}
	q, e = AlignNodes(p, ids, "bottom")
	if e != nil || q.Nodes[0].Y+q.Nodes[0].Height != q.Nodes[2].Y+q.Nodes[2].Height {
		t.Fatal(e)
	}
	q, e = DistributeNodes(p, ids, true)
	if e != nil {
		t.Fatal(e)
	}
	gap1 := q.Nodes[1].X - q.Nodes[0].X - q.Nodes[0].Width
	gap2 := q.Nodes[2].X - q.Nodes[1].X - q.Nodes[1].Width
	if gap1 != gap2 || q.Nodes[2].X != p.Nodes[2].X {
		t.Fatal("unequal gaps or moved endpoint", q.Nodes)
	}
	q, e = ResizeNode(p, "reactor", Rect{90, 60, 180, 350})
	if e != nil || q.Nodes[0].Width != 180 {
		t.Fatal(e)
	}
	if _, e = ResizeNode(p, "reactor", Rect{90, 60, 1, 350}); e == nil {
		t.Fatal("undersized node accepted")
	}
	q, e = ReorderNodes(p, []string{"reactor", "exchanger"}, true)
	if e != nil || q.Nodes[2].ID != "reactor" || q.Nodes[3].ID != "exchanger" {
		t.Fatal(q.Nodes, e)
	}
	q, e = DeleteNodes(p, []string{"exchanger"})
	if e != nil || len(q.Nodes) != 3 || len(q.Links) != 0 {
		t.Fatal(q, e)
	}
	if _, e = AlignNodes(p, []string{"reactor"}, "left"); e == nil {
		t.Fatal("meaningless single-node alignment")
	}
}
func TestEditDuplicatePreservesOnlyInternalPipesAndLongUnicodeIDs(t *testing.T) {
	p := Template()
	p.Links[0].Bends = []XY{{X: 350, Y: 240}}
	before := editJSON(t, p)
	q, ids, e := DuplicateNodes(p, []string{"reactor", "exchanger"}, 20, 20)
	if e != nil || len(ids) != 2 || len(q.Nodes) != 6 || len(q.Links) != 3 {
		t.Fatal(q, ids, e)
	}
	l := q.Links[2]
	if l.From.Node != ids[0] || l.To.Node != ids[1] || l.Bends[0] != (XY{X: 370, Y: 260}) || l.Tag != "DI11" {
		t.Fatal(l)
	}
	q.Links[2].Bends[0].X = 1
	if !bytes.Equal(before, editJSON(t, p)) {
		t.Fatal("duplicate aliases original")
	}
	p = Template()
	old := p.Nodes[0].ID
	p.Nodes[0].ID = strings.Repeat("反", 85)
	for i := range p.Links {
		if p.Links[i].From.Node == old {
			p.Links[i].From.Node = p.Nodes[0].ID
		}
	}
	q, ids, e = DuplicateNodes(p, []string{p.Nodes[0].ID}, 20, 20)
	if e != nil || !utf8.ValidString(ids[0]) || len(ids[0]) > 256 {
		t.Fatal(ids, e)
	}
}
func TestEditPipeBendsAreAtomicAndBudgeted(t *testing.T) {
	p := Template()
	b := XY{310, 220}
	q, e := EditLinkBend(p, "feed", 0, &b, true)
	if e != nil || len(q.Links[0].Bends) != 1 || len(p.Links[0].Bends) != 0 {
		t.Fatal(e)
	}
	before := editJSON(t, q)
	bad := XY{math.Inf(1), 0}
	if _, e = EditLinkBend(q, "feed", 0, &bad, false); e == nil {
		t.Fatal("invalid bend accepted")
	}
	if !bytes.Equal(before, editJSON(t, q)) {
		t.Fatal("invalid edit mutated input")
	}
	q, e = EditLinkBend(q, "feed", 0, nil, false)
	if e != nil || len(q.Links[0].Bends) != 0 {
		t.Fatal(e)
	}
	for i := 0; i < 32; i++ {
		q, e = EditLinkBend(q, "feed", i, &b, true)
		if e != nil {
			t.Fatal(e)
		}
	}
	if _, e = EditLinkBend(q, "feed", 32, &b, true); e == nil {
		t.Fatal("bend budget exceeded")
	}
}
