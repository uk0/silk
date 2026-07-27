package ged

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// typeHierLoadedTree builds a small, fully-loaded hierarchy: an interface root
// with two implementors, everything expanded, so the flat list is
// [Painter, cairoPainter, mockPainter] with depths 0,1,1.
func typeHierLoadedTree() *TypeNode {
	return &TypeNode{
		Name: "Painter", Pkg: "paint", File: "paint/painter.go", Line: 10,
		Kind: TypeKindInterface, Expanded: true, Loaded: true,
		Children: []*TypeNode{
			{Name: "cairoPainter", Pkg: "cairo", File: "cairo/painter.go", Line: 44,
				Kind: TypeKindStruct, Loaded: true},
			{Name: "mockPainter", Pkg: "paint", File: "paint/mock.go", Line: 7,
				Kind: TypeKindStruct, Loaded: true},
		},
	}
}

// TestTypeHierarchyFlattenLazyExpand walks the whole lazy cycle on the model
// alone: a single unloaded root flattens to one row, expanding it fires
// SigExpand exactly once with the active mode, SetChildren makes the children
// visible at depth 1, collapsing hides them again, and a re-expand of a now
// Loaded node does not ask the host a second time.
func TestTypeHierarchyFlattenLazyExpand(t *testing.T) {
	m := NewTypeHierarchyModel()
	root := &TypeNode{Name: "Painter", Pkg: "paint", File: "paint/painter.go", Line: 10,
		Kind: TypeKindInterface}
	m.SetTargets([]*TypeNode{root})

	if got := m.RowCount(); got != 1 {
		t.Fatalf("RowCount after SetTargets = %d, want 1", got)
	}
	if row, _ := m.RowAt(0); row.Node != root || row.Depth != 0 || row.Cyclic {
		t.Errorf("row 0 = %+v, want root at depth 0, not cyclic", row)
	}

	var (
		gotNode *TypeNode
		gotMode string
		fires   int
	)
	m.SigExpand(func(node *TypeNode, mode string) {
		gotNode, gotMode = node, mode
		fires++
	})

	if !m.ToggleRow(0) {
		t.Fatal("ToggleRow(0) on an unloaded root returned false")
	}
	if fires != 1 || gotNode != root || gotMode != TypeHierarchySupertypes {
		t.Fatalf("SigExpand fires=%d node=%p mode=%q, want 1 fire for root with %q",
			fires, gotNode, gotMode, TypeHierarchySupertypes)
	}
	if !root.Expanded {
		t.Error("root not marked Expanded while the fetch is in flight")
	}
	if got := m.RowCount(); got != 1 {
		t.Errorf("RowCount before children arrive = %d, want 1", got)
	}

	kids := []*TypeNode{
		{Name: "cairoPainter", Pkg: "cairo", File: "cairo/painter.go", Line: 44, Kind: TypeKindStruct},
		{Name: "mockPainter", Pkg: "paint", File: "paint/mock.go", Line: 7, Kind: TypeKindStruct},
	}
	if !m.SetChildren(root, kids) {
		t.Fatal("SetChildren(root, kids) returned false")
	}
	if !root.Loaded {
		t.Error("SetChildren did not mark the node Loaded")
	}
	rows := m.Rows()
	if len(rows) != 3 {
		t.Fatalf("RowCount after SetChildren = %d, want 3", len(rows))
	}
	wantDepth := []int{0, 1, 1}
	for i, row := range rows {
		if row.Depth != wantDepth[i] {
			t.Errorf("row %d depth = %d, want %d", i, row.Depth, wantDepth[i])
		}
	}
	if rows[1].Node != kids[0] || rows[2].Node != kids[1] {
		t.Error("child rows are not the nodes handed to SetChildren, in order")
	}

	// Collapse hides the children; re-expanding a Loaded node must not re-ask.
	if !m.ToggleRow(0) || m.RowCount() != 1 {
		t.Fatalf("collapse left %d rows, want 1", m.RowCount())
	}
	if !m.ToggleRow(0) || m.RowCount() != 3 {
		t.Fatalf("re-expand left %d rows, want 3", m.RowCount())
	}
	if fires != 1 {
		t.Errorf("SigExpand fired %d times, want 1 (Loaded nodes must not re-fetch)", fires)
	}

	// A node whose fetch came back empty is a leaf: inert, no twisty.
	if !m.SetChildren(kids[1], nil) {
		t.Fatal("SetChildren(child, nil) returned false")
	}
	if typeNodeExpandable(kids[1]) {
		t.Error("a Loaded child with no children must not be expandable")
	}
	if m.ToggleRow(2) {
		t.Error("ToggleRow on a leaf returned true")
	}
}

// TestTypeHierarchySetChildrenRejectsUnknown checks the stale-reply guard: a
// node that is not part of the tree — including the root of a hierarchy the
// mode switch already discarded — cannot inject rows.
func TestTypeHierarchySetChildrenRejectsUnknown(t *testing.T) {
	m := NewTypeHierarchyModel()
	root := typeHierLoadedTree()
	m.SetTargets([]*TypeNode{root})

	stray := &TypeNode{Name: "Stranger", Pkg: "other", Kind: TypeKindStruct}
	if m.SetChildren(stray, []*TypeNode{{Name: "X", Kind: TypeKindStruct}}) {
		t.Error("SetChildren accepted a node outside the tree")
	}
	if m.SetChildren(nil, nil) {
		t.Error("SetChildren accepted a nil node")
	}
	if got := m.RowCount(); got != 3 {
		t.Errorf("RowCount = %d after rejected SetChildren, want 3", got)
	}

	// After a mode switch the old root is gone: a late reply for it is dropped.
	m.SetMode(TypeHierarchyImplementations)
	if m.SetChildren(root, []*TypeNode{{Name: "Late", Kind: TypeKindStruct}}) {
		t.Error("SetChildren accepted a reply for a discarded root")
	}
	if got := m.RowCount(); got != 0 {
		t.Errorf("RowCount after mode switch = %d, want 0", got)
	}

	// A self-reference is dropped rather than flattened.
	m.SetTargets([]*TypeNode{root})
	if !m.SetChildren(root, []*TypeNode{root, {Name: "Real", Kind: TypeKindStruct}}) {
		t.Fatal("SetChildren returned false for a known root")
	}
	if len(root.Children) != 1 || root.Children[0].Name != "Real" {
		t.Errorf("children = %+v, want the self-reference dropped", root.Children)
	}
}

// TestTypeHierarchyCycleGuard builds a genuinely looping hierarchy (A -> B -> A,
// with the repeat still expanded and carrying children) and checks the repeat is
// emitted once, flagged Cyclic, not descended into, and not expandable. Without
// the guard the flatten would recurse forever.
func TestTypeHierarchyCycleGuard(t *testing.T) {
	a := &TypeNode{Name: "A", Pkg: "p", File: "p/a.go", Line: 3,
		Kind: TypeKindInterface, Expanded: true, Loaded: true}
	b := &TypeNode{Name: "B", Pkg: "p", File: "p/b.go", Line: 9,
		Kind: TypeKindStruct, Expanded: true, Loaded: true}
	// Same identity as a (pkg + name + kind), resolved a second time.
	a2 := &TypeNode{Name: "A", Pkg: "p", File: "p/a.go", Line: 3,
		Kind: TypeKindInterface, Expanded: true, Loaded: true}
	a.Children = []*TypeNode{b}
	b.Children = []*TypeNode{a2}
	a2.Children = []*TypeNode{b}

	m := NewTypeHierarchyModel()
	m.SetTargets([]*TypeNode{a})

	rows := m.Rows()
	if len(rows) != 3 {
		t.Fatalf("cyclic tree flattened to %d rows, want 3 (A, B, A-stub)", len(rows))
	}
	if rows[0].Cyclic || rows[1].Cyclic {
		t.Error("A and B must not be flagged cyclic")
	}
	if rows[2].Node != a2 || rows[2].Depth != 2 || !rows[2].Cyclic {
		t.Errorf("row 2 = %+v, want the repeated A at depth 2 flagged cyclic", rows[2])
	}
	if m.ToggleRow(2) {
		t.Error("ToggleRow on a cyclic stub returned true")
	}
	if got := m.RowCount(); got != 3 {
		t.Errorf("RowCount after toggling the stub = %d, want 3", got)
	}

	// Siblings that repeat each other are NOT cyclic — only ancestors count.
	twin := &TypeNode{Name: "B", Pkg: "p", Kind: TypeKindStruct, Loaded: true}
	root := &TypeNode{Name: "R", Pkg: "p", Kind: TypeKindInterface, Expanded: true, Loaded: true,
		Children: []*TypeNode{{Name: "B", Pkg: "p", Kind: TypeKindStruct, Loaded: true}, twin}}
	m.SetTargets([]*TypeNode{root})
	for i, row := range m.Rows() {
		if row.Cyclic {
			t.Errorf("row %d (%s) flagged cyclic among siblings", i, row.Node.Name)
		}
	}
}

// TestTypeHierarchyMultiTargetRoot covers the ambiguous-cursor case: several
// candidates are gathered under one synthetic root, a single candidate is the
// root itself, nils are dropped, and an empty list clears the view.
func TestTypeHierarchyMultiTargetRoot(t *testing.T) {
	m := NewTypeHierarchyModel()
	targets := []*TypeNode{
		{Name: "Reader", Pkg: "io", File: "io/io.go", Line: 20, Kind: TypeKindInterface},
		{Name: "Reader", Pkg: "bufio", File: "bufio/bufio.go", Line: 30, Kind: TypeKindStruct},
		{Name: "Reader", Pkg: "bytes", File: "bytes/reader.go", Line: 12, Kind: TypeKindStruct},
	}
	m.SetTargets(targets)

	if !m.IsMultiTarget() {
		t.Fatal("three candidates did not produce a multi-target root")
	}
	rows := m.Rows()
	if len(rows) != 4 {
		t.Fatalf("RowCount = %d, want 4 (root + 3 targets)", len(rows))
	}
	rootNode := rows[0].Node
	if rootNode.Kind != TypeKindTargets {
		t.Errorf("root kind = %q, want %q", rootNode.Kind, TypeKindTargets)
	}
	if rootNode.Name != typeHierTargetsLabel(3) {
		t.Errorf("root name = %q, want %q", rootNode.Name, typeHierTargetsLabel(3))
	}
	if rootNode.File != "" {
		t.Errorf("synthetic root has File %q, want empty (nothing to jump to)", rootNode.File)
	}
	if !rootNode.Expanded || !rootNode.Loaded {
		t.Error("synthetic root must start expanded and loaded")
	}
	for i, want := range targets {
		row := rows[i+1]
		if row.Node != want || row.Depth != 1 {
			t.Errorf("row %d = (%s, depth %d), want (%s, depth 1)", i+1, row.Node.Name, row.Depth, want.Name)
		}
	}

	// Exactly one candidate: no wrapper.
	m.SetTargets([]*TypeNode{targets[0]})
	if m.IsMultiTarget() {
		t.Error("a single candidate produced a multi-target root")
	}
	if rows = m.Rows(); len(rows) != 1 || rows[0].Node != targets[0] {
		t.Errorf("single-target rows = %+v, want just the candidate", rows)
	}

	// nil entries are dropped before the count decides.
	m.SetTargets([]*TypeNode{nil, targets[1], nil})
	if m.IsMultiTarget() || m.RowCount() != 1 {
		t.Errorf("nils not dropped: multi=%v rows=%d", m.IsMultiTarget(), m.RowCount())
	}

	// Empty clears.
	m.SetTargets(nil)
	if m.RowCount() != 0 || m.IsMultiTarget() {
		t.Errorf("SetTargets(nil) left %d rows (multi=%v), want 0", m.RowCount(), m.IsMultiTarget())
	}
}

// TestTypeHierarchySetModeRejects checks an unknown mode and a re-selection both
// report "unchanged" and leave the tree alone.
func TestTypeHierarchySetModeRejects(t *testing.T) {
	m := NewTypeHierarchyModel()
	m.SetTargets([]*TypeNode{typeHierLoadedTree()})

	if m.SetMode("bogus") {
		t.Error("SetMode accepted an unknown mode")
	}
	if m.SetMode(TypeHierarchySupertypes) {
		t.Error("SetMode reported a change for the already-active mode")
	}
	if m.Mode() != TypeHierarchySupertypes || m.RowCount() != 3 {
		t.Errorf("rejected SetMode disturbed state: mode=%q rows=%d", m.Mode(), m.RowCount())
	}
	if !m.SetMode(TypeHierarchySubtypes) {
		t.Fatal("SetMode(subtypes) reported no change")
	}
	if m.Mode() != TypeHierarchySubtypes || m.RowCount() != 0 {
		t.Errorf("after switch: mode=%q rows=%d, want %q and 0",
			m.Mode(), m.RowCount(), TypeHierarchySubtypes)
	}
}

// TestTypeHierModeAt exercises the mode-selector hit-test: buttons live in the
// band between the title and the rows, laid out left-to-right with gaps.
func TestTypeHierModeAt(t *testing.T) {
	midY := typeHierHeaderH + typeHierModeBarH/2
	cases := []struct {
		name string
		x, y float64
		want int
	}{
		{"title band", typeHierModeX(0) + 4, 5, -1},
		{"row area", typeHierModeX(0) + 4, typeHierRowsTop + 1, -1},
		{"left of first button", 0, midY, -1},
		{"button 0", typeHierModeX(0) + typeHierModeBtnW/2, midY, 0},
		{"button 1", typeHierModeX(1) + typeHierModeBtnW/2, midY, 1},
		{"button 2", typeHierModeX(2) + typeHierModeBtnW/2, midY, 2},
		{"gap between 0 and 1", typeHierModeX(1) - 2, midY, -1},
		{"right of last button", typeHierModeX(2) + typeHierModeBtnW + 5, midY, -1},
		{"top edge of band", typeHierModeX(1) + 1, typeHierHeaderH, 1},
	}
	for _, c := range cases {
		if got := typeHierModeAt(c.x, c.y); got != c.want {
			t.Errorf("%s: typeHierModeAt(%v,%v) = %d, want %d", c.name, c.x, c.y, got, c.want)
		}
	}
}

// TestTypeHierRowAtY exercises the pure row hit-test, including the header/mode
// band above the rows, out-of-range and degenerate inputs.
func TestTypeHierRowAtY(t *testing.T) {
	const (
		top = typeHierRowsTop
		rh  = 22.0
		n   = 3
	)
	cases := []struct {
		name string
		y    float64
		want int
	}{
		{"in the title band", 5, -1},
		{"in the mode band", typeHierHeaderH + 4, -1},
		{"top of row 0", top, 0},
		{"middle of row 0", top + rh/2, 0},
		{"middle of row 2", top + 2*rh + rh/2, 2},
		{"last pixel of row 2", top + 3*rh - 0.5, 2},
		{"just past last row", top + 3*rh, -1},
		{"far below", 10000, -1},
	}
	for _, c := range cases {
		if got := typeHierRowAtY(c.y, top, rh, n); got != c.want {
			t.Errorf("%s: typeHierRowAtY(%v,%v,%v,%d) = %d, want %d",
				c.name, c.y, top, rh, n, got, c.want)
		}
	}
	if got := typeHierRowAtY(top+5, top, 0, n); got != -1 {
		t.Errorf("typeHierRowAtY with rowH=0 = %d, want -1", got)
	}
	if got := typeHierRowAtY(top+5, top, rh, 0); got != -1 {
		t.Errorf("typeHierRowAtY with count=0 = %d, want -1", got)
	}
}

// TestTypeHierTwistyHit checks the twisty band tracks the row's indentation, so
// a depth-1 label click is not swallowed by the depth-0 triangle.
func TestTypeHierTwistyHit(t *testing.T) {
	if !typeHierTwistyHit(typeHierRowX(0)+2, 0) {
		t.Error("depth-0 twisty missed its own triangle")
	}
	if typeHierTwistyHit(typeHierRowX(0)+2, 1) {
		t.Error("depth-1 row claimed a click on the depth-0 triangle")
	}
	if !typeHierTwistyHit(typeHierRowX(2)+1, 2) {
		t.Error("depth-2 twisty missed its own triangle")
	}
	if typeHierTwistyHit(typeHierRowX(1)+typeHierTwistyW+4, 1) {
		t.Error("a click past the triangle counted as a twisty hit")
	}
}

// TestTypeHierarchyModeClickResetsAndSignals drives the selector through the
// panel's click path (no GL): picking a new direction clears the tree and fires
// SigMode once, re-picking the active one is inert, and a programmatic SetMode
// resets without signalling.
func TestTypeHierarchyModeClickResetsAndSignals(t *testing.T) {
	p := NewTypeHierarchyPanel()
	p.SetSize(360, 400)
	p.SetTargets([]*TypeNode{typeHierLoadedTree()})
	if p.Model().RowCount() != 3 {
		t.Fatalf("setup: RowCount = %d, want 3", p.Model().RowCount())
	}

	var got []string
	p.SigMode(func(mode string) { got = append(got, mode) })

	midY := typeHierHeaderH + typeHierModeBarH/2
	p.OnLeftDown(typeHierModeX(2)+typeHierModeBtnW/2, midY) // 实现
	if len(got) != 1 || got[0] != TypeHierarchyImplementations {
		t.Fatalf("SigMode = %v, want [%q]", got, TypeHierarchyImplementations)
	}
	if p.Mode() != TypeHierarchyImplementations {
		t.Errorf("Mode() = %q, want %q", p.Mode(), TypeHierarchyImplementations)
	}
	if p.Model().RowCount() != 0 {
		t.Errorf("mode switch left %d rows, want the tree reset", p.Model().RowCount())
	}

	// Re-picking the active direction must not re-signal.
	p.OnLeftDown(typeHierModeX(2)+typeHierModeBtnW/2, midY)
	if len(got) != 1 {
		t.Errorf("SigMode fired again for the active mode: %v", got)
	}

	p.OnLeftDown(typeHierModeX(0)+typeHierModeBtnW/2, midY) // 父类型
	if len(got) != 2 || got[1] != TypeHierarchySupertypes {
		t.Errorf("SigMode = %v, want second entry %q", got, TypeHierarchySupertypes)
	}

	// Programmatic switches are the host's own doing — no callback.
	p.SetTargets([]*TypeNode{typeHierLoadedTree()})
	if !p.SetMode(TypeHierarchySubtypes) {
		t.Fatal("SetMode(subtypes) reported no change")
	}
	if len(got) != 2 {
		t.Errorf("SetMode fired SigMode: %v", got)
	}
	if p.Model().RowCount() != 0 {
		t.Errorf("SetMode left %d rows, want the tree reset", p.Model().RowCount())
	}
}

// TestTypeHierarchyRowClickActivates drives row clicks through the hit-test +
// signal path (no GL): a label click on a child jumps to its file:line, a twisty
// click collapses instead of jumping, and the title band is inert.
//
// Geometry: rows start at typeHierRowsTop (46) with rowHeight 22, so row 1
// spans [68, 90); a depth-1 label sits right of typeHierRowX(1)+typeHierTwistyW.
func TestTypeHierarchyRowClickActivates(t *testing.T) {
	p := NewTypeHierarchyPanel()
	p.SetSize(360, 400)
	root := typeHierLoadedTree()
	p.SetTargets([]*TypeNode{root})

	var (
		gotFile string
		gotLine int
		fires   int
	)
	p.SigActivate(func(file string, line int) {
		gotFile, gotLine = file, line
		fires++
	})

	child := root.Children[0]
	labelX := typeHierRowX(1) + typeHierTwistyW + 8
	p.OnLeftDown(labelX, typeHierRowsTop+p.rowHeight+p.rowHeight/2)
	if fires != 1 {
		t.Fatalf("label click fired SigActivate %d times, want 1", fires)
	}
	if gotFile != child.File || gotLine != child.Line {
		t.Errorf("SigActivate = (%q,%d), want (%q,%d)", gotFile, gotLine, child.File, child.Line)
	}

	// Twisty on the root collapses it and must not navigate.
	fires = 0
	p.OnLeftDown(typeHierRowX(0)+2, typeHierRowsTop+p.rowHeight/2)
	if fires != 0 {
		t.Errorf("twisty click fired SigActivate %d times, want 0", fires)
	}
	if p.Model().RowCount() != 1 {
		t.Errorf("twisty click left %d rows, want 1 (collapsed)", p.Model().RowCount())
	}

	// The title band is inert.
	p.OnLeftDown(labelX, 5)
	if fires != 0 {
		t.Errorf("title-band click fired SigActivate %d times, want 0", fires)
	}
}

// TestTypeHierarchyMultiRootClickToggles checks a row with no source location —
// the synthetic multiple-targets root — collapses on a label click instead of
// firing a jump to an empty file.
func TestTypeHierarchyMultiRootClickToggles(t *testing.T) {
	p := NewTypeHierarchyPanel()
	p.SetSize(360, 400)
	p.SetTargets([]*TypeNode{
		{Name: "Reader", Pkg: "io", File: "io/io.go", Line: 20, Kind: TypeKindInterface},
		{Name: "Reader", Pkg: "bufio", File: "bufio/bufio.go", Line: 30, Kind: TypeKindStruct},
	})

	fires := 0
	p.SigActivate(func(string, int) { fires++ })

	// Label position of the depth-0 synthetic root, well past its twisty.
	p.OnLeftDown(typeHierRowX(0)+typeHierTwistyW+20, typeHierRowsTop+p.rowHeight/2)
	if fires != 0 {
		t.Errorf("multi-target root click fired SigActivate %d times, want 0", fires)
	}
	if got := p.Model().RowCount(); got != 1 {
		t.Errorf("root click left %d rows, want 1 (collapsed)", got)
	}
}

// TestTypeHierarchyLabels covers the pure label helpers used by Draw.
func TestTypeHierarchyLabels(t *testing.T) {
	modes := map[string]string{
		TypeHierarchySupertypes:      "父类型",
		TypeHierarchySubtypes:        "子类型",
		TypeHierarchyImplementations: "实现",
		"bogus":                      "bogus",
	}
	for mode, want := range modes {
		if got := typeHierarchyModeLabel(mode); got != want {
			t.Errorf("typeHierarchyModeLabel(%q) = %q, want %q", mode, got, want)
		}
	}
	if got, want := typeHierarchyTitle(TypeHierarchyImplementations, 4), "类型层次 / 实现 (4)"; got != want {
		t.Errorf("typeHierarchyTitle = %q, want %q", got, want)
	}
	if got, want := typeHierTargetsLabel(3), "多个目标 / 3 targets"; got != want {
		t.Errorf("typeHierTargetsLabel(3) = %q, want %q", got, want)
	}

	details := []struct {
		node *TypeNode
		want string
	}{
		{&TypeNode{Pkg: "paint", File: "paint/painter.go", Line: 10}, "paint · painter.go:10"},
		{&TypeNode{Pkg: "paint"}, "paint"},
		{&TypeNode{File: "a/b/c.go", Line: 3}, "c.go:3"},
		{&TypeNode{File: "a/b/c.go"}, "c.go"},
		{&TypeNode{}, ""},
		{nil, ""},
	}
	for _, c := range details {
		if got := typeHierRowDetail(c.node); got != c.want {
			t.Errorf("typeHierRowDetail(%+v) = %q, want %q", c.node, got, c.want)
		}
	}

	glyphs := map[string]string{
		TypeKindInterface: "I",
		TypeKindStruct:    "S",
		TypeKindMethod:    "M",
		TypeKindTargets:   "•",
	}
	for kind, want := range glyphs {
		if got := typeHierKindGlyph(kind); got != want {
			t.Errorf("typeHierKindGlyph(%q) = %q, want %q", kind, got, want)
		}
	}
}

// TestTypeHierarchyFactoryRegistered checks the panel is reachable by name from
// the object factory and listed as a tool view, which is what lets silkide dock
// and persist it.
func TestTypeHierarchyFactoryRegistered(t *testing.T) {
	f := core.FindFactory("ged.TypeHierarchyPanel")
	if f == nil {
		t.Fatal(`factory "ged.TypeHierarchyPanel" not registered`)
	}
	obj := f.New()
	p, ok := obj.(*TypeHierarchyPanel)
	if !ok {
		t.Fatalf("factory New() = %T, want *TypeHierarchyPanel", obj)
	}
	if p.Model() == nil {
		t.Error("factory-created panel has no model (Init did not run)")
	}
	def, ok := gui.GetToolViewDef("ged.TypeHierarchyPanel")
	if !ok {
		t.Fatal("tool view \"ged.TypeHierarchyPanel\" not registered")
	}
	if def.Name != "类型层次" {
		t.Errorf("tool view name = %q, want %q", def.Name, "类型层次")
	}
}
