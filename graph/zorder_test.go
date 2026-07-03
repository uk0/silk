package graph

import (
	"strings"
	"testing"
)

// orderOf walks the parent's sibling ring and returns a stable label for
// each child so test expectations read as a simple string like "ABC".
// Stacking order == iteration order: head first (back), tail last (front).
func orderOf(parent *RectItem, labels map[IItem]string) string {
	var b strings.Builder
	for _, c := range parent.Children() {
		b.WriteString(labels[c])
	}
	return b.String()
}

// buildZorderScene returns a parent with n labelled children (A, B, C, ...)
// in insertion order plus a label lookup for orderOf.
func buildZorderScene(n int) (*RectItem, []*RectItem, map[IItem]string) {
	parent := NewRectItem()
	labels := make(map[IItem]string)
	kids := make([]*RectItem, 0, n)
	for i := 0; i < n; i++ {
		c := NewRectItem()
		c.SetParent(parent)
		labels[IItem(c)] = string(rune('A' + i))
		kids = append(kids, c)
	}
	return parent, kids, labels
}

func TestZOrderRaise(t *testing.T) {
	parent, kids, labels := buildZorderScene(3) // A B C
	if got := orderOf(parent, labels); got != "ABC" {
		t.Fatalf("initial order = %q, want ABC", got)
	}
	// Raise B: A B C -> A C B
	kids[1].Raise()
	if got := orderOf(parent, labels); got != "ACB" {
		t.Errorf("after Raise(B): %q, want ACB", got)
	}
	// Raise the head A: A C B -> C A B
	kids[0].Raise()
	if got := orderOf(parent, labels); got != "CAB" {
		t.Errorf("after Raise(A): %q, want CAB", got)
	}
}

func TestZOrderRaiseFrontIsNoop(t *testing.T) {
	parent, kids, labels := buildZorderScene(3) // A B C
	// C is the tail (frontmost); Raise must not change anything.
	kids[2].Raise()
	if got := orderOf(parent, labels); got != "ABC" {
		t.Errorf("Raise on frontmost changed order to %q, want ABC", got)
	}
}

func TestZOrderLower(t *testing.T) {
	parent, kids, labels := buildZorderScene(3) // A B C
	// Lower B: A B C -> B A C
	kids[1].Lower()
	if got := orderOf(parent, labels); got != "BAC" {
		t.Errorf("after Lower(B): %q, want BAC", got)
	}
	// Lower the tail C: B A C -> B C A
	kids[2].Lower()
	if got := orderOf(parent, labels); got != "BCA" {
		t.Errorf("after Lower(C): %q, want BCA", got)
	}
}

func TestZOrderLowerBackIsNoop(t *testing.T) {
	parent, kids, labels := buildZorderScene(3) // A B C
	// A is the head (backmost); Lower must not change anything.
	kids[0].Lower()
	if got := orderOf(parent, labels); got != "ABC" {
		t.Errorf("Lower on backmost changed order to %q, want ABC", got)
	}
}

func TestZOrderBringToFront(t *testing.T) {
	parent, kids, labels := buildZorderScene(4) // A B C D
	// Bring the head A to front: A B C D -> B C D A
	kids[0].BringToFront()
	if got := orderOf(parent, labels); got != "BCDA" {
		t.Errorf("after BringToFront(A): %q, want BCDA", got)
	}
	// Bring a middle item C to front: B C D A -> B D A C
	kids[2].BringToFront()
	if got := orderOf(parent, labels); got != "BDAC" {
		t.Errorf("after BringToFront(C): %q, want BDAC", got)
	}
}

func TestZOrderBringToFrontAlreadyFront(t *testing.T) {
	parent, kids, labels := buildZorderScene(3) // A B C
	kids[2].BringToFront()                      // C already frontmost
	if got := orderOf(parent, labels); got != "ABC" {
		t.Errorf("BringToFront on frontmost changed order to %q, want ABC", got)
	}
}

func TestZOrderSendToBack(t *testing.T) {
	parent, kids, labels := buildZorderScene(4) // A B C D
	// Send the tail D to back: A B C D -> D A B C
	kids[3].SendToBack()
	if got := orderOf(parent, labels); got != "DABC" {
		t.Errorf("after SendToBack(D): %q, want DABC", got)
	}
	// Send a middle item C to back: D A B C -> C D A B
	kids[2].SendToBack()
	if got := orderOf(parent, labels); got != "CDAB" {
		t.Errorf("after SendToBack(C): %q, want CDAB", got)
	}
}

func TestZOrderSendToBackAlreadyBack(t *testing.T) {
	parent, kids, labels := buildZorderScene(3) // A B C
	kids[0].SendToBack()                        // A already backmost
	if got := orderOf(parent, labels); got != "ABC" {
		t.Errorf("SendToBack on backmost changed order to %q, want ABC", got)
	}
}

func TestZOrderSingleChildIsNoop(t *testing.T) {
	parent, kids, labels := buildZorderScene(1) // A
	kids[0].Raise()
	kids[0].Lower()
	kids[0].BringToFront()
	kids[0].SendToBack()
	if got := orderOf(parent, labels); got != "A" {
		t.Errorf("single-child reorder changed order to %q, want A", got)
	}
	if parent.Children()[0] != IItem(kids[0]) {
		t.Error("single child lost its parent link after reorder")
	}
}

func TestZOrderNoParentIsNoop(t *testing.T) {
	// An orphan item must tolerate every reorder call without panicking.
	orphan := NewRectItem()
	orphan.Raise()
	orphan.Lower()
	orphan.BringToFront()
	orphan.SendToBack()
	if orphan.Parent() != nil {
		t.Error("orphan gained a parent from a reorder no-op")
	}
}

// TestZOrderRingStaysConsistent walks the ring forward and backward after a
// mix of operations to confirm next/prev pointers never desynchronise.
func TestZOrderRingStaysConsistent(t *testing.T) {
	parent, kids, labels := buildZorderScene(4) // A B C D
	// A mix of operations; exact order is asserted elsewhere, here we only
	// care that the ring's next/prev links stay consistent afterwards.
	kids[0].BringToFront()
	kids[3].Lower()
	kids[1].SendToBack()

	children := parent.Children()
	if len(children) != 4 {
		t.Fatalf("lost children: have %d, want 4", len(children))
	}
	// Forward labels.
	fwd := orderOf(parent, labels)
	// Walk backward from the tail and confirm it is the reverse of forward.
	head := parent.NakedItem().child
	var rev strings.Builder
	for p := head.prev; ; p = p.prev {
		rev.WriteString(labels[p.Self()])
		if p == head {
			break
		}
	}
	want := reverseString(fwd)
	if rev.String() != want {
		t.Errorf("ring inconsistent: forward=%q backward=%q (want backward=%q)", fwd, rev.String(), want)
	}
}

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
