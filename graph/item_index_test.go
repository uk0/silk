package graph

import "testing"

// TestIndexInParentOrder verifies IndexInParent returns the correct 0-based
// position of each child in the parent's iteration order (head-first, the same
// order Children() and the draw loop use). The tail child must report N-1.
func TestIndexInParentOrder(t *testing.T) {
	const n = 5
	parent := newTestItem()
	kids := make([]*RectItem, 0, n)
	for i := 0; i < n; i++ {
		c := newTestItem()
		c.SetParent(parent)
		kids = append(kids, c)
	}

	// Children() defines the canonical order; IndexInParent must agree with it.
	children := parent.Children()
	if len(children) != n {
		t.Fatalf("expected %d children, got %d", n, len(children))
	}

	for i, c := range kids {
		if got := c.IndexInParent(); got != i {
			t.Errorf("kids[%d].IndexInParent() = %d, want %d", i, got, i)
		}
		// Round-trip against the canonical enumeration.
		if children[i] != c.Self() {
			t.Errorf("Children()[%d] != kids[%d]", i, i)
		}
		if children[c.IndexInParent()] != c.Self() {
			t.Errorf("round-trip failed: Children()[IndexInParent()] != kids[%d]", i)
		}
	}

	// The last child is the one the off-by-one bug got wrong: it must be N-1.
	if got := kids[n-1].IndexInParent(); got != n-1 {
		t.Errorf("last child IndexInParent() = %d, want %d", got, n-1)
	}
	// And the head child must be 0.
	if got := kids[0].IndexInParent(); got != 0 {
		t.Errorf("head child IndexInParent() = %d, want 0", got)
	}
}

// TestIndexInParentOrphan verifies an item with no parent returns the
// not-found sentinel (-1).
func TestIndexInParentOrphan(t *testing.T) {
	orphan := newTestItem()
	if got := orphan.IndexInParent(); got != -1 {
		t.Errorf("orphan.IndexInParent() = %d, want -1", got)
	}
}
