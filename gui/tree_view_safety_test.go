package gui

import (
	"testing"
)

// This file pins TreeView's "degrade gracefully on a misbehaving model" contract.
// A toolkit widget must not crash the host application when a model reports a
// child count but then hands back inconsistent or empty indices. initRoots used
// to panic("") on those two invariants; it now logs via core.Warn and skips the
// bad root, so building/refreshing the view stays panic-free while well-formed
// models keep building exactly as before.

// mustNotPanic runs fn and fails the test (instead of crashing the run) if fn
// panics. Returns whatever value fn recovered so callers can report it.
func mustNotPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked: %v", what, r)
		}
	}()
	fn()
}

// nilIndexModel lies: RowCount(root) reports the real root count, but Index for
// odd root rows returns a nil ModelIndex. This drives the mi.IsNil() failure
// path in initRoots. Embedding *kbTreeModel keeps Data/Parent/HasChildren/
// ColCount valid; only Index is overridden. Init re-registers self so the
// promoted HasChildren routes RowCount through this wrapper.
type nilIndexModel struct {
	*kbTreeModel
}

func newNilIndexModel(roots ...*kbTreeNode) *nilIndexModel {
	m := &nilIndexModel{kbTreeModel: &kbTreeModel{roots: roots}}
	m.Init(m)
	return m
}

func (m *nilIndexModel) Index(row, col int, parent ModelIndex) ModelIndex {
	if parent.IsNil() && row%2 == 1 {
		return ModelIndex{} // claimed by RowCount, but handed back as nil
	}
	return m.kbTreeModel.Index(row, col, parent)
}

// badRowModel lies the other way: Index returns a non-nil index whose Row does
// not match the requested row, driving the ri != mi.Row failure path.
type badRowModel struct {
	*kbTreeModel
}

func newBadRowModel(roots ...*kbTreeNode) *badRowModel {
	m := &badRowModel{kbTreeModel: &kbTreeModel{roots: roots}}
	m.Init(m)
	return m
}

func (m *badRowModel) Index(row, col int, parent ModelIndex) ModelIndex {
	mi := m.kbTreeModel.Index(row, col, parent)
	if parent.IsNil() && !mi.IsNil() {
		mi.Row = row + 100 // self-inconsistent: reported row != requested row
	}
	return mi
}

// TestTreeViewSafetyNilRootIndexDoesNotPanic: a model that reports N roots but
// returns nil indices for some of them must not crash SetModel/initRoots. The
// good roots still build; the nil ones are skipped.
func TestTreeViewSafetyNilRootIndexDoesNotPanic(t *testing.T) {
	tv := NewTreeView()
	m := newNilIndexModel(
		leaf("R0"), // even row -> valid
		leaf("R1"), // odd row  -> nil index, must be skipped
		leaf("R2"), // even row -> valid
		leaf("R3"), // odd row  -> nil index, must be skipped
	)

	mustNotPanic(t, "SetModel with nil-index model", func() {
		tv.SetModel(m)
	})

	// Only the even (valid) roots should survive; the nil ones are dropped.
	if got, want := rowLabels(tv), []string{"R0", "R2"}; !eqStrings(got, want) {
		t.Fatalf("visible rows after skipping nil roots = %v, want %v", got, want)
	}
}

// TestTreeViewSafetyInconsistentRootRowDoesNotPanic: a model whose Index reports
// a row number different from the one requested must not crash; the inconsistent
// roots are skipped rather than trusted.
func TestTreeViewSafetyInconsistentRootRowDoesNotPanic(t *testing.T) {
	tv := NewTreeView()
	m := newBadRowModel(leaf("X"), leaf("Y"))

	mustNotPanic(t, "SetModel with inconsistent-row model", func() {
		tv.SetModel(m)
	})

	// Every root is inconsistent, so none survive — but crucially, no panic.
	if got := len(tv.rows); got != 0 {
		t.Fatalf("inconsistent-row model: visible rows = %d, want 0 (all skipped)", got)
	}
}

// TestTreeViewSafetyWellFormedStillBuilds is the no-regression guard: the same
// well-formed fixture the keyboard tests use must build the expected visible
// rows unchanged after the panic->skip rewrite.
func TestTreeViewSafetyWellFormedStillBuilds(t *testing.T) {
	var tv *TreeView
	mustNotPanic(t, "SetModel with well-formed model", func() {
		tv = newKbTree()
	})
	if got, want := rowLabels(tv), []string{"A", "A0", "A1", "B"}; !eqStrings(got, want) {
		t.Fatalf("well-formed model visible rows = %v, want %v", got, want)
	}
}
