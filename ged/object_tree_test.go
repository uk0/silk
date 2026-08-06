package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
)

// rowMidY returns the y coordinate at the vertical middle of row r — the band
// a drop nests into rather than reorders through.
func rowMidY(insp *ObjectInspector, r int) float64 {
	return insp.contentTop() + float64(r)*insp.rowHeight + insp.rowHeight*0.5
}

// rowTopY returns a y coordinate just inside the top edge of row r, in the band
// that inserts before it.
func rowTopY(insp *ObjectInspector, r int) float64 {
	return insp.contentTop() + float64(r)*insp.rowHeight + 1
}

// boundInspector returns an inspector bound to view's scene, rows already
// built. Binding the scene is enough — it carries the view that shows it.
func boundInspector(view *GedView) *ObjectInspector {
	insp := NewObjectInspector()
	insp.SetScene(view.GedScene())
	return insp
}

// TestObjectTreeDragOntoContainerRowReparents is the headline of the object
// tree: dropping a row onto a container row nests the widget, and does it
// through the canvas's own ReparentCommand so one Ctrl+Z puts it back in the
// parent AND the sibling slot it left. Before, the same gesture fell through to
// the sibling reorder — B ended up next to the container instead of inside it,
// and the undo stack never heard about it.
func TestObjectTreeDragOntoContainerRowReparents(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	box := sceneWidget(t, scene, "gui.VBox", "C", 10, 10, 60, 40)
	btn := sceneWidget(t, scene, "gui.Button", "B", 10, 60, 40, 8)

	insp := boundInspector(view)

	// Rows: 0=root, 1=C, 2=B. Press B, drag onto the middle of C, release.
	insp.OnLeftDown(10, rowMidY(insp, 2))
	insp.OnMouseMove(10, rowMidY(insp, 1))
	if insp.dropInto != 1 {
		t.Fatalf("hovering the middle of the container row set dropInto=%d, want 1", insp.dropInto)
	}
	insp.OnLeftUp(10, rowMidY(insp, 1))

	if btn.Parent() != graph.IItem(box) {
		t.Fatalf("after the drop B is parented to %T, want the container C", btn.Parent())
	}
	if n := scene.UndoStack().Count(); n != 1 {
		t.Fatalf("undo stack holds %d commands after the drop, want 1", n)
	}

	scene.UndoStack().Undo()
	if btn.Parent() != graph.IItem(scene) {
		t.Errorf("after undo B is parented to %T, want the scene", btn.Parent())
	}
	if got := sceneOrder(scene); !eqStrings(got, []string{"C", "B"}) {
		t.Errorf("after undo the sibling order is %v, want [C B]", got)
	}
}

// TestObjectTreeDragOntoRootRowUnnests is the other half of the gesture: a
// widget already inside a container is dragged onto the form root row and comes
// back out, undoably.
func TestObjectTreeDragOntoRootRowUnnests(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	box := sceneWidget(t, scene, "gui.VBox", "C", 10, 10, 60, 40)
	inner := nestChild(t, box, "G")

	insp := boundInspector(view)

	// Rows: 0=root, 1=C, 2=G. Drag G onto the root row.
	insp.OnLeftDown(10, rowMidY(insp, 2))
	insp.OnMouseMove(10, rowMidY(insp, 0))
	insp.OnLeftUp(10, rowMidY(insp, 0))

	if inner.Parent() != graph.IItem(scene) {
		t.Fatalf("after dropping G on the root row it is parented to %T, want the scene", inner.Parent())
	}
	scene.UndoStack().Undo()
	if inner.Parent() != graph.IItem(box) {
		t.Errorf("after undo G is parented to %T, want the container", inner.Parent())
	}
}

// TestObjectTreeDragBetweenRowsStillReorders pins the gesture split: the middle
// of a row nests, the edges still reorder siblings the way they always did.
func TestObjectTreeDragBetweenRowsStillReorders(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	sceneWidget(t, scene, "gui.Button", "A", 10, 10, 40, 8)
	sceneWidget(t, scene, "gui.Button", "B", 10, 30, 40, 8)

	insp := boundInspector(view)

	// Rows: 0=root, 1=A, 2=B. Drag B to the top edge of A's row.
	insp.OnLeftDown(10, rowMidY(insp, 2))
	insp.OnMouseMove(10, rowTopY(insp, 1))
	if insp.dropInto != -1 {
		t.Fatalf("the top edge of a leaf row set dropInto=%d, want -1 (a reorder)", insp.dropInto)
	}
	insp.OnLeftUp(10, rowTopY(insp, 1))

	if got := sceneOrder(scene); !eqStrings(got, []string{"B", "A"}) {
		t.Errorf("after the reorder drop the order is %v, want [B A]", got)
	}
}

// TestObjectTreeRejectsIllegalNesting: a leaf cannot adopt anything, an item
// cannot be dropped into its own subtree (SetParentAt has no cycle guard), and
// a drop into the parent it already has is not work.
func TestObjectTreeRejectsIllegalNesting(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	box := sceneWidget(t, scene, "gui.VBox", "C", 10, 10, 60, 40)
	nestChild(t, box, "G")
	sceneWidget(t, scene, "gui.Button", "B", 10, 60, 40, 8)

	insp := boundInspector(view)
	// Rows: 0=root, 1=C, 2=G, 3=B
	rowC, rowG, rowB := 1, 2, 3

	if insp.canNestInto(rowB, rowG) {
		t.Error("a plain Button row accepted a child; only containers and the form root may")
	}
	if insp.canNestInto(rowC, rowG) {
		t.Error("the container accepted a drop into its own child — that builds a cycle")
	}
	if insp.canNestInto(rowG, rowC) {
		t.Error("dropping G into the container it already lives in counted as work")
	}
	if !insp.canNestInto(rowB, rowC) {
		t.Error("a top-level Button could not be dropped into a VBox container")
	}
	if !insp.canNestInto(rowG, 0) {
		t.Error("a nested widget could not be dropped onto the form root")
	}
}

// TestObjectTreeRowsCarryNameTypeAndLock covers what a row says: the widget's
// own name with the type as secondary text, and the two lock flags read
// straight off the item so a row cannot disagree with the canvas badge.
func TestObjectTreeRowsCarryNameTypeAndLock(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	ok := sceneWidget(t, scene, "gui.Button", "btnOK", 10, 10, 40, 8)
	ok.SetLockPos(true)
	sceneWidget(t, scene, "gui.Edit", "", 10, 30, 40, 8) // unnamed: falls back to the type

	insp := boundInspector(view)

	row := insp.items[insp.rowOfItem(ok)]
	if row.name != "btnOK" || row.typeName != "Button" {
		t.Errorf("row = (%q, %q), want (\"btnOK\", \"Button\")", row.name, row.typeName)
	}
	// The row caches no lock state — Draw reads the item live, so that a lock
	// toggled from the canvas context menu (which never rebuilds this panel)
	// still badges the row. Assert on the source of truth the badge reads.
	if !row.item.IsLockPos() || row.item.IsLockSize() {
		t.Errorf("row locks = (pos %v, size %v), want (true, false) — the item's own flags",
			row.item.IsLockPos(), row.item.IsLockSize())
	}
	ok.SetLockSize(true)
	if !row.item.IsLockSize() {
		t.Error("a lock set after the rebuild is invisible to the row; the badge would show stale state")
	}
	if got := insp.RowNames(); len(got) != 3 || got[2] != "edit" {
		t.Errorf("rows = %v, want the unnamed widget listed as its lower-cased type", got)
	}
}

// TestObjectTreeFilterNarrowsRows: the filter box keeps the rows whose name or
// type matches, listed flat, and clearing it restores the tree.
func TestObjectTreeFilterNarrowsRows(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	box := sceneWidget(t, scene, "gui.VBox", "leftPane", 10, 10, 60, 40)
	nestChild(t, box, "btnSave")
	sceneWidget(t, scene, "gui.Button", "btnCancel", 10, 60, 40, 8)

	insp := boundInspector(view)
	if got := len(insp.RowNames()); got != 4 {
		t.Fatalf("unfiltered rows = %d, want 4 (root + 3 widgets)", got)
	}

	insp.SetFilter("btn")
	if got := insp.RowNames(); !eqStrings(got, []string{"btnSave", "btnCancel"}) {
		t.Errorf("filter \"btn\" left rows %v, want [btnSave btnCancel]", got)
	}
	for _, it := range insp.items {
		if it.depth != 0 {
			t.Errorf("filtered row %q kept depth %d; matches are listed flat", it.name, it.depth)
		}
	}

	// Case-insensitive, and the type counts too — the palette matches on both.
	insp.SetFilter("VBOX")
	if got := insp.RowNames(); !eqStrings(got, []string{"leftPane"}) {
		t.Errorf("filter \"VBOX\" left rows %v, want [leftPane]", got)
	}

	insp.SetFilter("")
	if got := len(insp.RowNames()); got != 4 {
		t.Errorf("after clearing the filter rows = %d, want 4", got)
	}
}

// TestObjectTreeFilterSuspendsDrag: while a filter is on, the rows are a flat
// list of matches, so neither the sibling arithmetic nor a nesting target means
// anything. The drag must not start at all.
func TestObjectTreeFilterSuspendsDrag(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	sceneWidget(t, scene, "gui.VBox", "boxA", 10, 10, 60, 40)
	sceneWidget(t, scene, "gui.Button", "btnB", 10, 60, 40, 8)

	insp := boundInspector(view)
	insp.SetFilter("b")
	// Rows are now [boxA, btnB]; press the second one and try to drag.
	insp.OnLeftDown(10, rowMidY(insp, 1))
	if insp.dragIdx != 0 {
		t.Fatalf("a press with a filter active armed a drag (dragIdx=%d)", insp.dragIdx)
	}
	insp.OnMouseMove(10, rowMidY(insp, 0))
	insp.OnLeftUp(10, rowMidY(insp, 0))

	if got := sceneOrder(scene); !eqStrings(got, []string{"boxA", "btnB"}) {
		t.Errorf("a drag under an active filter changed the tree: %v, want [boxA btnB]", got)
	}
}

// TestObjectTreeSelectionIsTwoWay: a row press selects the widget on the
// canvas, and the canvas selection highlights the row back. A multi-selection
// has no single row and clears the highlight.
func TestObjectTreeSelectionIsTwoWay(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	a := sceneWidget(t, scene, "gui.Button", "A", 10, 10, 40, 8)
	b := sceneWidget(t, scene, "gui.Button", "B", 10, 30, 40, 8)

	insp := boundInspector(view)

	// Row -> canvas.
	insp.OnLeftDown(10, rowMidY(insp, 2))
	sel := view.Selection()
	if sel.Count() != 1 || sel.ItemList()[0] != graph.IItem(b) {
		t.Fatalf("pressing B's row left the canvas selection at %d item(s), want just B", sel.Count())
	}

	// Canvas -> row.
	insp.SyncSelection([]graph.IItem{a})
	if got := insp.SelectedItem(); got != graph.IItem(a) {
		t.Errorf("selecting A on the canvas highlighted %v, want A's row", got)
	}
	insp.SyncSelection([]graph.IItem{a, b})
	if got := insp.SelectedItem(); got != nil {
		t.Errorf("a two-item canvas selection highlighted %v, want no row", got)
	}
}

// TestObjectTreeHighlightFollowsItemAcrossRebuild: the highlight is anchored to
// the widget, not to a row number. Nesting a widget moves its row; a kept index
// would light up whichever widget inherited the slot.
func TestObjectTreeHighlightFollowsItemAcrossRebuild(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	sceneWidget(t, scene, "gui.VBox", "C", 10, 10, 60, 40)
	btn := sceneWidget(t, scene, "gui.Button", "B", 10, 60, 40, 8)

	insp := boundInspector(view)
	insp.SelectItem(btn)
	if insp.selectedIdx != 2 {
		t.Fatalf("B starts at row %d, want 2", insp.selectedIdx)
	}

	btn.SetWidgetName("B2")
	sceneWidget(t, scene, "gui.Button", "A", 10, 80, 40, 8)
	insp.Rebuild()

	if got := insp.SelectedItem(); got != graph.IItem(btn) {
		t.Errorf("after a rebuild the highlight sits on %v, want the same widget", got)
	}
}

// TestDropIsInto pins the band split the drag depends on, without a panel.
func TestDropIsInto(t *testing.T) {
	for _, tc := range []struct {
		frac float64
		want bool
	}{
		{0.0, false}, {0.24, false}, {0.25, true}, {0.5, true},
		{0.74, true}, {0.75, false}, {0.99, false},
	} {
		if got := dropIsInto(tc.frac); got != tc.want {
			t.Errorf("dropIsInto(%.2f) = %v, want %v", tc.frac, got, tc.want)
		}
	}
}

// TestRowAt pins the row arithmetic, including the scroll offset — a second
// copy of the header offset is how a panel selects the row above the one you
// clicked.
func TestRowAt(t *testing.T) {
	const top, rh = 48.0, 24.0
	if idx, frac := rowAt(top+1, top, 0, rh); idx != 0 || frac > 0.25 {
		t.Errorf("just under the header: row %d frac %.2f, want row 0 near its top", idx, frac)
	}
	if idx, _ := rowAt(top+2*rh+1, top, 0, rh); idx != 2 {
		t.Errorf("third row resolved to %d, want 2", idx)
	}
	if idx, _ := rowAt(top+1, top, 2*rh, rh); idx != 2 {
		t.Errorf("scrolled down two rows, the top row resolved to %d, want 2", idx)
	}
	if idx, _ := rowAt(top, top, 0, 0); idx != -1 {
		t.Errorf("a zero row height resolved to row %d, want -1", idx)
	}
}

// TestInspectorRowMatches covers the filter predicate on its own.
func TestInspectorRowMatches(t *testing.T) {
	cases := []struct {
		name, typeName, filter string
		want                   bool
	}{
		{"btnSave", "Button", "", true},
		{"btnSave", "Button", "save", true},
		{"btnSave", "Button", "butt", true},
		{"btnSave", "Button", "edit", false},
		{"", "Edit", "edit", true},
	}
	for _, tc := range cases {
		if got := inspectorRowMatches(tc.name, tc.typeName, strings.ToLower(tc.filter)); got != tc.want {
			t.Errorf("inspectorRowMatches(%q, %q, %q) = %v, want %v",
				tc.name, tc.typeName, tc.filter, got, tc.want)
		}
	}
}
