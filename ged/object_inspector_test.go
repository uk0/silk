package ged

import (
	"testing"
)

// nestChild creates a FakeWidget named child and nests it inside container,
// mirroring the tree that "Lay Out" produces (a container holding a widget).
func nestChild(t *testing.T, container *FakeWidget, child string) *FakeWidget {
	t.Helper()
	fw, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create nested %s: %v", child, err)
	}
	fw.SetWidgetName(child)
	fw.SetParent(container)
	return fw
}

// rowOf returns the flattened inspector-row index of the row named name.
func rowOf(t *testing.T, insp *ObjectInspector, name string) int {
	t.Helper()
	for i, it := range insp.items {
		if it.name == name {
			return i
		}
	}
	t.Fatalf("inspector row %q not found", name)
	return -1
}

// depthOf returns the inspector depth of the row named name, or -1 if absent.
func depthOf(insp *ObjectInspector, name string) int {
	for _, it := range insp.items {
		if it.name == name {
			return it.depth
		}
	}
	return -1
}

// callReorderSafe drives reorderItems and turns any panic into a clean test
// failure (pre-fix this panicked with an index-out-of-range).
func callReorderSafe(t *testing.T, insp *ObjectInspector, from, to int) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("reorderItems(%d, %d) panicked: %v", from, to, r)
		}
	}()
	insp.reorderItems(from, to)
}

// TestObjectInspectorReorderTopLevelAfterNested reproduces the P0 crash: with
// tree [A(VBox holding G), B] the inspector rows are 0=root,1=A,2=G,3=B, so B
// is a top-level item whose ROW index (3) exceeds the direct-child count (2).
// Dragging B must reorder direct children by SIBLING index without panicking,
// and must leave G nested under A.
func TestObjectInspectorReorderTopLevelAfterNested(t *testing.T) {
	scene := NewGedScene()
	a := sceneWidget(t, scene, "gui.VBox", "A", 10, 10, 40, 30)
	nestChild(t, a, "G")
	sceneWidget(t, scene, "gui.Button", "B", 10, 50, 40, 8)

	insp := NewObjectInspector()
	insp.SetScene(scene)
	insp.Rebuild()

	// Rows: 0=root, 1=A, 2=G, 3=B — B's row index is 3, not sibling index 1.
	if got := rowOf(t, insp, "B"); got != 3 {
		t.Fatalf("expected B at inspector row 3, got %d", got)
	}
	if got := sceneOrder(scene); !eqStrings(got, []string{"A", "B"}) {
		t.Fatalf("initial direct-child order = %v, want [A B]", got)
	}

	// Drag B (row 3) to the top (drop line above A -> dropIdx 1).
	callReorderSafe(t, insp, rowOf(t, insp, "B"), 1)

	if got := sceneOrder(scene); !eqStrings(got, []string{"B", "A"}) {
		t.Fatalf("after reorder = %v, want [B A]", got)
	}
	// G must still be a grandchild under A (nesting preserved).
	if d := depthOf(insp, "G"); d != 2 {
		t.Errorf("G depth after reorder = %d, want 2 (still nested under A)", d)
	}
	if d := depthOf(insp, "B"); d != 1 {
		t.Errorf("B depth after reorder = %d, want 1 (top-level)", d)
	}
}

// TestObjectInspectorReorderNestedArrangements checks that reordering a
// top-level item positioned after a nested item works whether the nesting is
// first or in the middle of the direct-child list.
func TestObjectInspectorReorderNestedArrangements(t *testing.T) {
	cases := []struct {
		name    string
		build   func(t *testing.T, s *GedScene) // adds A,B,C as direct children in order
		drag    string
		dropRow int // flattened drop-line row (as OnMouseMove computes it)
		want    []string
	}{
		{
			name: "nested first, drag C to top",
			build: func(t *testing.T, s *GedScene) {
				a := sceneWidget(t, s, "gui.VBox", "A", 10, 10, 40, 30)
				nestChild(t, a, "G") // A(G)
				sceneWidget(t, s, "gui.Button", "B", 10, 50, 40, 8)
				sceneWidget(t, s, "gui.Button", "C", 10, 70, 40, 8)
			},
			// rows: 0 root,1 A,2 G,3 B,4 C ; drag C to top -> dropIdx 1
			drag: "C", dropRow: 1, want: []string{"C", "A", "B"},
		},
		{
			name: "nested middle, drag C before B",
			build: func(t *testing.T, s *GedScene) {
				sceneWidget(t, s, "gui.Button", "A", 10, 10, 40, 8)
				b := sceneWidget(t, s, "gui.VBox", "B", 10, 30, 40, 30)
				nestChild(t, b, "G") // B(G)
				sceneWidget(t, s, "gui.Button", "C", 10, 70, 40, 8)
			},
			// rows: 0 root,1 A,2 B,3 G,4 C ; drop line above B -> dropIdx 2
			drag: "C", dropRow: 2, want: []string{"A", "C", "B"},
		},
		{
			name: "nested first, drag B to end",
			build: func(t *testing.T, s *GedScene) {
				a := sceneWidget(t, s, "gui.VBox", "A", 10, 10, 40, 30)
				nestChild(t, a, "G") // A(G)
				sceneWidget(t, s, "gui.Button", "B", 10, 50, 40, 8)
				sceneWidget(t, s, "gui.Button", "C", 10, 70, 40, 8)
			},
			// rows: 0 root,1 A,2 G,3 B,4 C ; drop past end -> dropIdx = len(items)=5
			drag: "B", dropRow: 5, want: []string{"A", "C", "B"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scene := NewGedScene()
			tc.build(t, scene)

			insp := NewObjectInspector()
			insp.SetScene(scene)
			insp.Rebuild()

			callReorderSafe(t, insp, rowOf(t, insp, tc.drag), tc.dropRow)

			if got := sceneOrder(scene); !eqStrings(got, tc.want) {
				t.Fatalf("after reorder = %v, want %v", got, tc.want)
			}
			// The nested grandchild G must never surface as a direct child.
			for _, n := range sceneOrder(scene) {
				if n == "G" {
					t.Fatalf("grandchild G leaked into direct children: %v", sceneOrder(scene))
				}
			}
		})
	}
}

// TestObjectInspectorReorderOutOfRangeNoop confirms an out-of-range row index
// is a safe no-op: no panic and the direct-child order is unchanged.
func TestObjectInspectorReorderOutOfRangeNoop(t *testing.T) {
	scene := NewGedScene()
	a := sceneWidget(t, scene, "gui.VBox", "A", 10, 10, 40, 30)
	nestChild(t, a, "G")
	sceneWidget(t, scene, "gui.Button", "B", 10, 50, 40, 8)

	insp := NewObjectInspector()
	insp.SetScene(scene)
	insp.Rebuild()

	before := sceneOrder(scene)

	// fromIdx past the end, negative fromIdx, and a wildly-large drop target
	// must all no-op without panicking.
	callReorderSafe(t, insp, len(insp.items), 1)
	callReorderSafe(t, insp, -1, 1)
	callReorderSafe(t, insp, rowOf(t, insp, "B"), 999)

	if got := sceneOrder(scene); !eqStrings(got, before) {
		t.Fatalf("order changed after out-of-range/no-op reorders: got %v, want %v", got, before)
	}
}
