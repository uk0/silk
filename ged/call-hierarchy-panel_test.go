package ged

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// newCallNode is a terse fixture helper: an unloaded leaf with a distinct
// (file, line, name) identity.
func newCallNode(name, file string, line int) *CallNode {
	return &CallNode{Name: name, File: file, Line: line, Kind: "func"}
}

// TestCallHierarchyFlattenHonorsExpanded checks the flatten walks only
// into expanded nodes, stamps the right depth on each row, and only marks
// a row expandable when it can actually open (unloaded == optimistic,
// loaded-and-empty == leaf).
func TestCallHierarchyFlattenHonorsExpanded(t *testing.T) {
	leaf := newCallNode("c", "c.go", 3)
	mid := newCallNode("b", "b.go", 2)
	mid.Children = []*CallNode{leaf}
	mid.Loaded = true
	root := newCallNode("a", "a.go", 1)
	root.Children = []*CallNode{mid}
	root.Loaded = true

	rows := flattenCallTree(root)
	if len(rows) != 1 {
		t.Fatalf("collapsed root flattened to %d rows, want 1", len(rows))
	}
	if !rows[0].Expandable {
		t.Error("loaded root with children is not marked expandable")
	}

	root.Expanded = true
	rows = flattenCallTree(root)
	if len(rows) != 2 {
		t.Fatalf("expanded root flattened to %d rows, want 2", len(rows))
	}
	if rows[0].Depth != 0 || rows[1].Depth != 1 {
		t.Errorf("depths = %d,%d, want 0,1", rows[0].Depth, rows[1].Depth)
	}
	if rows[1].Node != mid {
		t.Errorf("row 1 node = %+v, want mid", rows[1].Node)
	}

	mid.Expanded = true
	rows = flattenCallTree(root)
	if len(rows) != 3 {
		t.Fatalf("two levels expanded flattened to %d rows, want 3", len(rows))
	}
	if rows[2].Depth != 2 || rows[2].Node != leaf {
		t.Errorf("row 2 = depth %d node %+v, want depth 2 leaf", rows[2].Depth, rows[2].Node)
	}
	// Unloaded leaf gets an optimistic expander; once resolved to zero
	// children it becomes a real leaf.
	if !rows[2].Expandable {
		t.Error("unloaded leaf should be optimistically expandable")
	}
	leaf.Loaded = true
	rows = flattenCallTree(root)
	if rows[2].Expandable {
		t.Error("loaded childless leaf must not be expandable")
	}

	if got := flattenCallTree(nil); got != nil {
		t.Errorf("flattenCallTree(nil) = %+v, want nil", got)
	}
}

// TestCallHierarchyLazyExpandFetch drives the lazy protocol: expanding an
// unloaded node asks the host exactly once, a re-expand while the fetch is
// in flight does not ask again, SetChildren clears the pending request and
// grows the row list, and re-expanding a loaded node never refetches.
func TestCallHierarchyLazyExpandFetch(t *testing.T) {
	p := NewCallHierarchyPanel()
	p.SetSize(300, 400)
	root := newCallNode("Frame.Draw", "gui/frame.go", 100)
	p.SetRoot(root)

	var asked []*CallNode
	p.SigExpand(func(n *CallNode) { asked = append(asked, n) })

	m := p.Model()
	m.Expand(root)
	if len(asked) != 1 || asked[0] != root {
		t.Fatalf("Expand asked %d times (%v), want 1 for root", len(asked), asked)
	}
	if m.PendingCount() != 1 {
		t.Errorf("PendingCount = %d, want 1 while the fetch is in flight", m.PendingCount())
	}
	if len(p.Rows()) != 1 {
		t.Errorf("rows = %d before the children arrive, want 1", len(p.Rows()))
	}

	// Re-expanding while in flight must not issue a second fetch.
	m.Expand(root)
	if len(asked) != 1 {
		t.Errorf("in-flight re-expand issued a duplicate fetch: %d asks", len(asked))
	}

	kids := []*CallNode{newCallNode("A", "a.go", 10), newCallNode("B", "b.go", 20)}
	if !p.SetChildren(root, kids) {
		t.Fatal("SetChildren rejected a live fetch")
	}
	if m.PendingCount() != 0 {
		t.Errorf("PendingCount = %d after the answer, want 0", m.PendingCount())
	}
	if got := p.Rows(); len(got) != 3 {
		t.Fatalf("rows = %d after expansion, want 3", len(got))
	}

	// Collapse + re-expand reuses the resolved children: no new fetch.
	m.Collapse(root)
	if len(p.model.Rows()) != 1 {
		t.Errorf("collapsed rows = %d, want 1", len(p.model.Rows()))
	}
	m.Expand(root)
	if len(asked) != 1 {
		t.Errorf("re-expanding a loaded node refetched: %d asks", len(asked))
	}
}

// TestCallHierarchyCycleDetected checks a child whose (file, line, name)
// identity already appears on its ancestor path is flagged Recursive, is
// refused by Expand, and is never descended into by the flatten — even if
// something forces Expanded and hangs children off it.
func TestCallHierarchyCycleDetected(t *testing.T) {
	m := NewCallHierarchyModel()
	root := newCallNode("A", "a.go", 1)
	m.SetRoot(root)

	m.Expand(root)
	b := newCallNode("B", "b.go", 2)
	if !m.SetChildren(root, []*CallNode{b}) {
		t.Fatal("SetChildren(root) rejected")
	}
	m.Expand(b)

	// A calls B calls A: same identity as the root, different pointer.
	aAgain := newCallNode("A", "a.go", 1)
	if !m.SetChildren(b, []*CallNode{aAgain}) {
		t.Fatal("SetChildren(b) rejected")
	}
	if !aAgain.Recursive {
		t.Fatal("cycle back to the root was not marked Recursive")
	}
	if b.Recursive {
		t.Error("non-recursive child B was marked Recursive")
	}

	// Expand refuses a recursive node: no state change, no fetch.
	asked := 0
	m.SigExpand(func(*CallNode) { asked++ })
	m.Expand(aAgain)
	if aAgain.Expanded || asked != 0 {
		t.Errorf("recursive node expanded=%v asks=%d, want false/0", aAgain.Expanded, asked)
	}

	rows := m.Rows()
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (A, B, A↻)", len(rows))
	}
	if rows[2].Node != aAgain || rows[2].Expandable {
		t.Errorf("recursive row = %+v expandable=%v, want the cycle node, not expandable",
			rows[2].Node, rows[2].Expandable)
	}

	// Defensive: even a forced Expanded + children must not be walked.
	aAgain.Expanded = true
	aAgain.Children = []*CallNode{newCallNode("deep", "d.go", 9)}
	if got := m.Rows(); len(got) != 3 {
		t.Errorf("rows = %d after forcing a recursive node open, want 3", len(got))
	}
}

// TestCallRowAtY exercises the pure row hit-test: rows start at
// topOffset, rowH tall; header / out-of-range / degenerate inputs give -1.
func TestCallRowAtY(t *testing.T) {
	const (
		top = callHierarchyHeaderH
		rh  = 20.0
		n   = 3
	)
	cases := []struct {
		name string
		y    float64
		want int
	}{
		{"above rows (header)", 10, -1},
		{"top of row 0", top, 0},
		{"middle of row 0", top + rh/2, 0},
		{"middle of row 2", top + 2*rh + rh/2, 2},
		{"last pixel of row 2", top + 3*rh - 0.5, 2},
		{"just past last row", top + 3*rh, -1},
		{"far below", 10000, -1},
	}
	for _, c := range cases {
		if got := callRowAtY(c.y, top, rh, n); got != c.want {
			t.Errorf("%s: callRowAtY(%v,%v,%v,%d) = %d, want %d",
				c.name, c.y, top, rh, n, got, c.want)
		}
	}
	if got := callRowAtY(50, top, 0, n); got != -1 {
		t.Errorf("callRowAtY with rowH=0 = %d, want -1", got)
	}
	if got := callRowAtY(top+5, top, rh, 0); got != -1 {
		t.Errorf("callRowAtY with count=0 = %d, want -1", got)
	}
}

// TestCallRowGeometryHelpers checks the indentation ladder and the two
// pure x-axis hit-tests (expander box, header toggle).
func TestCallRowGeometryHelpers(t *testing.T) {
	if got, want := callRowIndent(0), callRowIndentBase; got != want {
		t.Errorf("callRowIndent(0) = %v, want %v", got, want)
	}
	if got, want := callRowIndent(2), callRowIndentBase+2*callRowIndentStep; got != want {
		t.Errorf("callRowIndent(2) = %v, want %v", got, want)
	}
	if got, want := callRowIndent(-3), callRowIndentBase; got != want {
		t.Errorf("callRowIndent(-3) = %v, want %v", got, want)
	}

	// Depth 1's expander box sits at its own indent, not depth 0's.
	if !callExpanderHit(callRowIndent(1)+1, 1) {
		t.Error("point inside the depth-1 expander box missed")
	}
	if callExpanderHit(callRowIndent(0)+1, 1) {
		t.Error("depth-0 x hit the depth-1 expander box")
	}
	if callExpanderHit(callRowIndent(1)+callExpanderSize, 1) {
		t.Error("x just past the expander box still hit")
	}
	if callExpanderHit(callRowIndent(0)-1, 0) {
		t.Error("x left of the expander box hit")
	}

	if !callToggleHit(callToggleX+1, callToggleY+1) {
		t.Error("point inside the direction toggle missed")
	}
	if callToggleHit(callToggleX+callToggleW+1, callToggleY+1) {
		t.Error("point right of the direction toggle hit")
	}
	if callToggleHit(callToggleX+1, callToggleY+callToggleH+1) {
		t.Error("point below the direction toggle hit")
	}
}

// TestCallHierarchyLabels checks the pure display strings.
func TestCallHierarchyLabels(t *testing.T) {
	if got, want := callDirectionLabel(true), "调用者 / Incoming"; got != want {
		t.Errorf("callDirectionLabel(true) = %q, want %q", got, want)
	}
	if got, want := callDirectionLabel(false), "被调用 / Outgoing"; got != want {
		t.Errorf("callDirectionLabel(false) = %q, want %q", got, want)
	}
	if got, want := callHierarchyTitle(nil), "调用层次 / Call Hierarchy"; got != want {
		t.Errorf("callHierarchyTitle(nil) = %q, want %q", got, want)
	}
	if got, want := callHierarchyTitle(newCallNode("Draw", "a.go", 1)), "调用层次: Draw"; got != want {
		t.Errorf("callHierarchyTitle(root) = %q, want %q", got, want)
	}
	if got, want := callRowLocator(newCallNode("Draw", "gui/frame.go", 42)), "frame.go:42"; got != want {
		t.Errorf("callRowLocator = %q, want %q", got, want)
	}
	if got := callRowLocator(nil); got != "" {
		t.Errorf("callRowLocator(nil) = %q, want empty", got)
	}
	rec := newCallNode("A", "a.go", 1)
	rec.Recursive = true
	if got, want := callRowName(rec), "A ↻"; got != want {
		t.Errorf("callRowName(recursive) = %q, want %q", got, want)
	}
}

// TestCallHierarchyExpanderVsRowHit drives clicks through the panel (no
// GL): a click on the expander triangle expands (fetch fires) without
// activating, a click on the row body activates without refetching, and a
// second expander click collapses.
func TestCallHierarchyExpanderVsRowHit(t *testing.T) {
	p := NewCallHierarchyPanel()
	p.SetSize(300, 400)
	root := newCallNode("A", "a.go", 1)
	p.SetRoot(root)

	fetched := 0
	p.SigExpand(func(n *CallNode) {
		fetched++
		p.SetChildren(n, []*CallNode{newCallNode("B", "b.go", 2)})
	})
	var activated []string
	p.SigActivate(func(file string, line int) { activated = append(activated, file) })

	y0 := callHierarchyHeaderH + p.rowHeight/2 // middle of row 0

	p.OnLeftDown(callRowIndent(0)+2, y0) // on the expander
	if fetched != 1 {
		t.Fatalf("expander click fetched %d times, want 1", fetched)
	}
	if len(activated) != 0 {
		t.Errorf("expander click also activated the row: %v", activated)
	}
	if got := p.Rows(); len(got) != 2 {
		t.Fatalf("rows = %d after expanding, want 2", len(got))
	}

	p.OnLeftDown(200, y0) // on the row body
	if len(activated) != 1 || activated[0] != "a.go" {
		t.Errorf("row-body click activated %v, want [a.go]", activated)
	}
	if fetched != 1 {
		t.Errorf("row-body click refetched: %d", fetched)
	}

	p.OnLeftDown(callRowIndent(0)+2, y0) // collapse again
	if got := p.Rows(); len(got) != 1 {
		t.Errorf("rows = %d after collapsing, want 1", len(got))
	}
	if fetched != 1 {
		t.Errorf("collapse refetched: %d", fetched)
	}
}

// TestCallHierarchySigActivateCarriesLocation clicks a depth-1 child row
// and checks SigActivate reports that node's file and 1-based line.
func TestCallHierarchySigActivateCarriesLocation(t *testing.T) {
	p := NewCallHierarchyPanel()
	p.SetSize(300, 400)
	root := newCallNode("A", "a.go", 1)
	p.SetRoot(root)
	p.SigExpand(func(n *CallNode) {
		p.SetChildren(n, []*CallNode{
			newCallNode("B", "pkg/b.go", 20),
			newCallNode("C", "pkg/c.go", 33),
		})
	})
	p.Model().Expand(root)
	p.Refresh()
	if got := p.Rows(); len(got) != 3 {
		t.Fatalf("rows = %d, want 3", len(got))
	}

	var (
		gotFile string
		gotLine int
		fired   bool
	)
	p.SigActivate(func(file string, line int) {
		gotFile, gotLine, fired = file, line, true
	})

	y := callHierarchyHeaderH + 2*p.rowHeight + p.rowHeight/2 // row 2 == C
	p.OnLeftDown(220, y)
	if !fired {
		t.Fatal("OnLeftDown did not fire SigActivate")
	}
	if gotFile != "pkg/c.go" || gotLine != 33 {
		t.Errorf("SigActivate = (%q,%d), want (pkg/c.go,33)", gotFile, gotLine)
	}

	// A click below the last row is inert.
	fired = false
	p.OnLeftDown(220, callHierarchyHeaderH+5*p.rowHeight)
	if fired {
		t.Error("click past the last row fired SigActivate")
	}
}

// TestCallHierarchyStaleFetchIgnored covers both stale flavours: a result
// issued under an older generation (the direction flipped meanwhile) and a
// result for a node that no longer belongs to the tree (the root changed).
func TestCallHierarchyStaleFetchIgnored(t *testing.T) {
	p := NewCallHierarchyPanel()
	p.SetSize(300, 400)
	root := newCallNode("A", "a.go", 1)
	p.SetRoot(root)

	var captured *CallNode
	p.SigExpand(func(n *CallNode) { captured = n })
	p.Model().Expand(root)
	if captured != root {
		t.Fatalf("SigExpand captured %+v, want root", captured)
	}
	gen0 := p.Model().Generation()

	// Flipping the direction invalidates the in-flight fetch.
	p.ToggleDirection()
	if p.Model().Generation() <= gen0 {
		t.Fatalf("Generation = %d after a direction switch, want > %d",
			p.Model().Generation(), gen0)
	}
	if p.SetChildren(captured, []*CallNode{newCallNode("B", "b.go", 2)}) {
		t.Error("SetChildren applied a fetch from a stale generation")
	}
	if len(root.Children) != 0 || root.Loaded {
		t.Errorf("stale fetch mutated the tree: children=%d loaded=%v",
			len(root.Children), root.Loaded)
	}
	if got := p.Rows(); len(got) != 1 {
		t.Errorf("rows = %d after a stale fetch, want 1", len(got))
	}

	// A result for a node that is no longer in the tree is dropped too.
	orphan := newCallNode("A", "a.go", 1)
	p.SetRoot(orphan)
	if p.SetChildren(root, []*CallNode{newCallNode("C", "c.go", 3)}) {
		t.Error("SetChildren applied a fetch for a discarded tree")
	}
	if p.SetChildren(nil, nil) {
		t.Error("SetChildren(nil) reported success")
	}

	// Cancel invalidates without touching the tree.
	p.Model().Expand(orphan)
	if p.Model().PendingCount() != 1 {
		t.Fatalf("PendingCount = %d, want 1", p.Model().PendingCount())
	}
	p.Model().Cancel()
	if p.Model().PendingCount() != 0 {
		t.Errorf("PendingCount = %d after Cancel, want 0", p.Model().PendingCount())
	}
	if p.SetChildren(orphan, []*CallNode{newCallNode("D", "d.go", 4)}) {
		t.Error("SetChildren applied a cancelled fetch")
	}
	if p.Model().Root() != orphan {
		t.Error("Cancel replaced the root")
	}
}

// TestCallHierarchyDirectionToggle checks the header toggle: it flips the
// direction, fires SigDirection with the new value, resets the tree so the
// host re-resolves, and ignores clicks elsewhere in the header.
func TestCallHierarchyDirectionToggle(t *testing.T) {
	p := NewCallHierarchyPanel()
	p.SetSize(300, 400)
	root := newCallNode("A", "a.go", 1)
	root.Children = []*CallNode{newCallNode("B", "b.go", 2)}
	root.Loaded = true
	root.Expanded = true
	p.SetRoot(root)
	if !p.Incoming() {
		t.Fatal("panel should default to the incoming direction")
	}
	if len(p.Rows()) != 2 {
		t.Fatalf("rows = %d before the toggle, want 2", len(p.Rows()))
	}

	var got []bool
	p.SigDirection(func(incoming bool) { got = append(got, incoming) })

	p.OnLeftDown(callToggleX+2, callToggleY+2)
	if p.Incoming() {
		t.Error("toggle click did not switch to outgoing")
	}
	if len(p.Rows()) != 1 {
		t.Errorf("rows = %d after the switch, want 1 (tree reset)", len(p.Rows()))
	}

	// Header clicks outside the toggle are inert.
	p.OnLeftDown(250, 5)
	if len(got) != 1 {
		t.Errorf("SigDirection fired %d times, want 1 (header click leaked)", len(got))
	}

	p.OnLeftDown(callToggleX+2, callToggleY+2)
	if !p.Incoming() {
		t.Error("second toggle click did not switch back to incoming")
	}
	if len(got) != 2 || got[0] != false || got[1] != true {
		t.Errorf("SigDirection sequence = %v, want [false true]", got)
	}
	if p.Model().Direction().String() != "incoming" {
		t.Errorf("Direction() = %q, want incoming", p.Model().Direction().String())
	}
}

// TestCallHierarchyPanelFactoryRegistered checks the factory id resolves
// to a constructible, initialised *CallHierarchyPanel and that the tool
// view is registered, matching how silkide instantiates it for docking.
func TestCallHierarchyPanelFactoryRegistered(t *testing.T) {
	obj := core.New("ged.CallHierarchyPanel")
	p, ok := obj.(*CallHierarchyPanel)
	if !ok {
		t.Fatalf("factory ged.CallHierarchyPanel built %T, want *CallHierarchyPanel", obj)
	}
	if p.Model() == nil {
		t.Error("factory-built panel has no model (Init did not run)")
	}
	if len(p.Rows()) != 0 {
		t.Errorf("fresh panel has %d rows, want 0", len(p.Rows()))
	}
	def, ok := gui.GetToolViewDef("ged.CallHierarchyPanel")
	if !ok {
		t.Fatal("tool view ged.CallHierarchyPanel is not registered")
	}
	if def.Name == "" {
		t.Error("tool view registered without a name")
	}
}

// TestCallHierarchyClear checks Clear drops the tree and invalidates
// in-flight fetches.
func TestCallHierarchyClear(t *testing.T) {
	p := NewCallHierarchyPanel()
	root := newCallNode("A", "a.go", 1)
	p.SetRoot(root)
	p.Model().Expand(root)

	p.Clear()
	if p.Model().Root() != nil {
		t.Error("Clear left a root behind")
	}
	if len(p.Rows()) != 0 {
		t.Errorf("rows = %d after Clear, want 0", len(p.Rows()))
	}
	if p.SetChildren(root, []*CallNode{newCallNode("B", "b.go", 2)}) {
		t.Error("SetChildren applied a fetch that Clear invalidated")
	}
}
