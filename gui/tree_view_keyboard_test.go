package gui

import (
	"testing"
)

// kbTreeNode is a node in the in-memory tree used to exercise TreeView keyboard
// navigation. Each node carries a display label and its children.
type kbTreeNode struct {
	name     string
	children []*kbTreeNode
}

func leaf(name string) *kbTreeNode {
	return &kbTreeNode{name: name}
}

func branch(name string, children ...*kbTreeNode) *kbTreeNode {
	return &kbTreeNode{name: name, children: children}
}

// kbTreeModel is a minimal hierarchical IGuiModel backed by kbTreeNode pointers,
// shaped after data.PrjModel: ModelIndex.Param holds the *kbTreeNode. Embedding
// GuiModel supplies HasChildren / bindItemView / unbindItemView.
type kbTreeModel struct {
	GuiModel
	roots []*kbTreeNode
}

func newKbTreeModel(roots ...*kbTreeNode) *kbTreeModel {
	m := &kbTreeModel{roots: roots}
	m.Init(m)
	return m
}

func (m *kbTreeModel) Index(row, col int, parent ModelIndex) ModelIndex {
	if col != 0 {
		return ModelIndex{}
	}
	if parent.IsNil() {
		if row < 0 || row >= len(m.roots) {
			return ModelIndex{}
		}
		return ModelIndex{row, col, m.roots[row], m}
	}
	n := parent.Param.(*kbTreeNode)
	if row < 0 || row >= len(n.children) {
		return ModelIndex{}
	}
	return ModelIndex{row, col, n.children[row], m}
}

func (m *kbTreeModel) Data(idx ModelIndex, role ItemDataRole) interface{} {
	if idx.IsNil() || role != DisplayRole {
		return nil
	}
	return idx.Param.(*kbTreeNode).name
}

func (m *kbTreeModel) HeaderData(section int, vertical bool, role ItemDataRole) interface{} {
	if vertical || role != DisplayRole || section != 0 {
		return nil
	}
	return "Name"
}

// indexOf returns the position of child within parent's child slice, or -1 for a
// root node. Used to rebuild a node's ModelIndex chain in Parent.
func (m *kbTreeModel) indexOf(parent, child *kbTreeNode) int {
	for i, c := range parent.children {
		if c == child {
			return i
		}
	}
	return -1
}

func (m *kbTreeModel) col0Index(n *kbTreeNode) ModelIndex {
	if n == nil {
		return ModelIndex{}
	}
	for i, r := range m.roots {
		if r == n {
			return m.Index(i, 0, ModelIndex{})
		}
	}
	// Not a root: locate its parent by scanning the tree.
	parent := m.parentOf(m.roots, nil, n)
	if parent == nil {
		return ModelIndex{}
	}
	return m.Index(m.indexOf(parent, n), 0, m.col0Index(parent))
}

// parentOf walks the forest to find the parent of target. cur is the parent of
// the nodes slice being scanned.
func (m *kbTreeModel) parentOf(nodes []*kbTreeNode, cur, target *kbTreeNode) *kbTreeNode {
	for _, n := range nodes {
		if n == target {
			return cur
		}
		if p := m.parentOf(n.children, n, target); p != nil {
			return p
		}
	}
	return nil
}

func (m *kbTreeModel) Parent(idx ModelIndex) ModelIndex {
	if idx.IsNil() {
		return ModelIndex{}
	}
	n := idx.Param.(*kbTreeNode)
	p := m.parentOf(m.roots, nil, n)
	if p == nil {
		return ModelIndex{}
	}
	return m.col0Index(p)
}

func (m *kbTreeModel) RowCount(parent ModelIndex) int {
	if parent.IsNil() {
		return len(m.roots)
	}
	return len(parent.Param.(*kbTreeNode).children)
}

func (m *kbTreeModel) ColCount() int { return 1 }

func (m *kbTreeModel) Flags(idx ModelIndex) ItemFlags { return ItemIsSelectable | ItemIsEnabled }

// newKbTree builds a TreeView over a fixed fixture and returns it.
//
// Tree:
//
//	A            (branch)
//	  A0         (branch -> A0a, A0b)
//	  A1         (leaf)
//	B            (leaf)
//
// SetModel runs OnEndReset -> ExpandAll(0), which expands the depth-0 roots
// (A, B), so A's children become visible but A0 stays collapsed. The initial
// visible rows are therefore: [A, A0, A1, B].
func newKbTree() *TreeView {
	tv := NewTreeView()
	m := newKbTreeModel(
		branch("A",
			branch("A0", leaf("A0a"), leaf("A0b")),
			leaf("A1"),
		),
		leaf("B"),
	)
	tv.SetModel(m)
	return tv
}

// rowLabels returns the display text of every currently visible row, top to
// bottom, so tests can assert both the row count and ordering.
func rowLabels(tv *TreeView) []string {
	out := make([]string, len(tv.rows))
	for i, r := range tv.rows {
		out[i] = tv.getCellText(r, 0)
	}
	return out
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestTreeKeyboardInitialLayout pins the fixture: ExpandAll(0) leaves the roots
// expanded and A0 collapsed, giving four visible rows in a known order.
func TestTreeViewKeyboardInitialLayout(t *testing.T) {
	tv := newKbTree()
	if got, want := rowLabels(tv), []string{"A", "A0", "A1", "B"}; !eqStrings(got, want) {
		t.Fatalf("initial visible rows = %v, want %v", got, want)
	}
}

// TestTreeKeyboardDownUpMoveAndClamp: Down advances the current row through the
// visible list and clamps at the last row; Up walks back and clamps at row 0.
func TestTreeViewKeyboardDownUpMoveAndClamp(t *testing.T) {
	tv := newKbTree()
	n := len(tv.rows) // 4

	// From "no current row" (-1), the first Down lands on row 0.
	tv.OnKeyDown(KeyDown, false)
	if tv.CurrentRow() != 0 {
		t.Fatalf("first Down: current = %d, want 0", tv.CurrentRow())
	}
	// Walk to the bottom.
	for i := 1; i < n; i++ {
		tv.OnKeyDown(KeyDown, false)
		if tv.CurrentRow() != i {
			t.Fatalf("Down step %d: current = %d, want %d", i, tv.CurrentRow(), i)
		}
	}
	// Extra Down at the end clamps.
	tv.OnKeyDown(KeyDown, false)
	if tv.CurrentRow() != n-1 {
		t.Fatalf("Down past end: current = %d, want %d (clamped)", tv.CurrentRow(), n-1)
	}

	// Walk back up.
	for i := n - 2; i >= 0; i-- {
		tv.OnKeyDown(KeyUp, false)
		if tv.CurrentRow() != i {
			t.Fatalf("Up step: current = %d, want %d", tv.CurrentRow(), i)
		}
	}
	// Extra Up at the top clamps at 0.
	tv.OnKeyDown(KeyUp, false)
	if tv.CurrentRow() != 0 {
		t.Fatalf("Up past start: current = %d, want 0 (clamped)", tv.CurrentRow())
	}
}

// TestTreeKeyboardRightExpands: Right on a collapsed parent (A0) expands it, so
// its children appear and the visible-row count grows; the current row stays on
// A0.
func TestTreeViewKeyboardRightExpands(t *testing.T) {
	tv := newKbTree()
	before := len(tv.rows) // 4

	// Move current onto A0 (row 1).
	tv.SetCurrentRow(1)
	if got := tv.getCellText(tv.rows[tv.CurrentRow()], 0); got != "A0" {
		t.Fatalf("setup: current row label = %q, want %q", got, "A0")
	}

	tv.OnKeyDown(KeyRight, false)

	if len(tv.rows) != before+2 {
		t.Fatalf("Right on collapsed parent: rows = %d, want %d (grew by 2 children)", len(tv.rows), before+2)
	}
	if got, want := rowLabels(tv), []string{"A", "A0", "A0a", "A0b", "A1", "B"}; !eqStrings(got, want) {
		t.Fatalf("after expand rows = %v, want %v", got, want)
	}
	// Current row must still be A0.
	if got := tv.getCellText(tv.rows[tv.CurrentRow()], 0); got != "A0" {
		t.Fatalf("after expand current row = %q, want %q", got, "A0")
	}
}

// TestTreeKeyboardRightDescendsWhenExpanded: a second Right on an already
// expanded node moves the current row to its first child rather than changing
// the layout.
func TestTreeViewKeyboardRightDescendsWhenExpanded(t *testing.T) {
	tv := newKbTree()
	tv.SetCurrentRow(1) // A0
	tv.OnKeyDown(KeyRight, false) // expand
	rows := len(tv.rows)

	tv.OnKeyDown(KeyRight, false) // descend to first child
	if len(tv.rows) != rows {
		t.Fatalf("Right on expanded node changed layout: rows = %d, want %d", len(tv.rows), rows)
	}
	if got := tv.getCellText(tv.rows[tv.CurrentRow()], 0); got != "A0a" {
		t.Fatalf("Right on expanded node: current = %q, want first child %q", got, "A0a")
	}
}

// TestTreeKeyboardLeftCollapses: Left on an expanded node collapses it, shrinking
// the visible-row count back.
func TestTreeViewKeyboardLeftCollapses(t *testing.T) {
	tv := newKbTree()
	tv.SetCurrentRow(1)           // A0
	tv.OnKeyDown(KeyRight, false) // expand -> 6 rows
	expanded := len(tv.rows)

	tv.OnKeyDown(KeyLeft, false) // collapse A0
	if len(tv.rows) != expanded-2 {
		t.Fatalf("Left on expanded node: rows = %d, want %d (shrank by 2)", len(tv.rows), expanded-2)
	}
	if got, want := rowLabels(tv), []string{"A", "A0", "A1", "B"}; !eqStrings(got, want) {
		t.Fatalf("after collapse rows = %v, want %v", got, want)
	}
	if got := tv.getCellText(tv.rows[tv.CurrentRow()], 0); got != "A0" {
		t.Fatalf("after collapse current row = %q, want %q", got, "A0")
	}
}

// TestTreeKeyboardLeftMovesToParent: Left on a collapsed child moves the current
// row up to its parent without changing the layout.
func TestTreeViewKeyboardLeftMovesToParent(t *testing.T) {
	tv := newKbTree()
	// A1 (row 2) is a leaf -> collapsed; Left should jump to its parent A (row 0).
	tv.SetCurrentRow(2)
	if got := tv.getCellText(tv.rows[tv.CurrentRow()], 0); got != "A1" {
		t.Fatalf("setup: current = %q, want %q", got, "A1")
	}
	rows := len(tv.rows)

	tv.OnKeyDown(KeyLeft, false)
	if len(tv.rows) != rows {
		t.Fatalf("Left to parent changed layout: rows = %d, want %d", len(tv.rows), rows)
	}
	if got := tv.getCellText(tv.rows[tv.CurrentRow()], 0); got != "A" {
		t.Fatalf("Left on collapsed child: current = %q, want parent %q", got, "A")
	}
}

// TestTreeKeyboardHomeEnd: Home lands on the first visible row, End on the last.
func TestTreeViewKeyboardHomeEnd(t *testing.T) {
	tv := newKbTree()
	tv.SetCurrentRow(2)

	tv.OnKeyDown(KeyHome, false)
	if tv.CurrentRow() != 0 {
		t.Fatalf("Home: current = %d, want 0", tv.CurrentRow())
	}

	tv.OnKeyDown(KeyEnd, false)
	if tv.CurrentRow() != len(tv.rows)-1 {
		t.Fatalf("End: current = %d, want %d", tv.CurrentRow(), len(tv.rows)-1)
	}
}

// TestTreeKeyboardEnterSpaceActivate: Enter and Space both fire the activate
// callback for the current row, reporting its ModelIndex; the same callback the
// content-click path uses.
func TestTreeViewKeyboardEnterSpaceActivate(t *testing.T) {
	tv := newKbTree()

	var got []string
	tv.SetActivatedCallback(func(o interface{}, mi ModelIndex) {
		got = append(got, tv.Model().Data(mi, DisplayRole).(string))
	})

	tv.SetCurrentRow(1) // A0
	tv.OnKeyDown(KeyEnter, false)
	tv.SetCurrentRow(3) // B
	tv.OnKeyDown(KeySpace, false)

	if want := []string{"A0", "B"}; !eqStrings(got, want) {
		t.Fatalf("activate via Enter/Space reported %v, want %v", got, want)
	}
}

// TestParentRowIndex covers the pure parent-lookup helper over the visible-row
// list once A0 is expanded: children point back to A0, A0/A1 point back to A,
// and the top-level rows report no parent (-1).
func TestTreeViewParentRowIndex(t *testing.T) {
	tv := newKbTree()
	tv.SetCurrentRow(1)
	tv.OnKeyDown(KeyRight, false) // [A, A0, A0a, A0b, A1, B]

	cases := []struct {
		row  int
		want int
	}{
		{0, -1}, // A (top level)
		{1, 0},  // A0 -> A
		{2, 1},  // A0a -> A0
		{3, 1},  // A0b -> A0
		{4, 0},  // A1 -> A
		{5, -1}, // B (top level)
	}
	for _, c := range cases {
		if got := tv.parentRowIndex(c.row); got != c.want {
			t.Errorf("parentRowIndex(%d) = %d, want %d", c.row, got, c.want)
		}
	}
}

// TestNavHelpersPure exercises the pure clamp/move helpers directly, independent
// of any TreeView instance or GL render.
func TestTreeViewNavHelpersPure(t *testing.T) {
	// clampRow
	if got := clampRow(-5, 4); got != 0 {
		t.Errorf("clampRow(-5,4) = %d, want 0", got)
	}
	if got := clampRow(10, 4); got != 3 {
		t.Errorf("clampRow(10,4) = %d, want 3", got)
	}
	if got := clampRow(2, 0); got != -1 {
		t.Errorf("clampRow(2,0) = %d, want -1 (empty)", got)
	}

	// nextVisibleRow / prevVisibleRow
	if got := nextVisibleRow(-1, 4); got != 0 {
		t.Errorf("nextVisibleRow(-1,4) = %d, want 0", got)
	}
	if got := nextVisibleRow(3, 4); got != 3 {
		t.Errorf("nextVisibleRow(3,4) = %d, want 3 (clamp)", got)
	}
	if got := prevVisibleRow(0, 4); got != 0 {
		t.Errorf("prevVisibleRow(0,4) = %d, want 0 (clamp)", got)
	}
	if got := prevVisibleRow(2, 4); got != 1 {
		t.Errorf("prevVisibleRow(2,4) = %d, want 1", got)
	}
	if got := nextVisibleRow(0, 0); got != -1 {
		t.Errorf("nextVisibleRow on empty = %d, want -1", got)
	}
}
