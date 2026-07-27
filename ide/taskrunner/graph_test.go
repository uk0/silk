package taskrunner

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// --- helpers ---------------------------------------------------------

// node is shorthand for Node{ID, DependsOn}.
func node(id string, deps ...string) Node {
	return Node{ID: id, DependsOn: deps}
}

// mustTopoSort fails the test if the graph does not sort.
func mustTopoSort(t *testing.T, nodes []Node) []string {
	t.Helper()
	order, err := TopoSort(nodes)
	if err != nil {
		t.Fatalf("TopoSort returned error: %v", err)
	}
	return order
}

// posOf returns the index of id in order, or -1.
func posOf(order []string, id string) int {
	for i, got := range order {
		if got == id {
			return i
		}
	}
	return -1
}

// --- ordering --------------------------------------------------------

// TestTopoSortDiamond: a classic diamond (gen -> build/vet -> test) must
// come out with every node after its dependencies. The exact slice is
// pinned because the tie-break is submission order: build was submitted
// before vet, so it is emitted first.
func TestTopoSortDiamond(t *testing.T) {
	nodes := []Node{
		node("gen"),
		node("build", "gen"),
		node("vet", "gen"),
		node("test", "build", "vet"),
	}
	got := mustTopoSort(t, nodes)
	want := []string{"gen", "build", "vet", "test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopoSort = %v, want %v", got, want)
	}
	// The invariant behind the pinned slice.
	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			if posOf(got, dep) >= posOf(got, n.ID) {
				t.Errorf("%q must come after its dependency %q: %v", n.ID, dep, got)
			}
		}
	}
}

// TestTopoSortTieBreakIsInputOrder: the SAME graph submitted in reverse
// input order sorts differently but still deterministically — the ready
// queue always pops the earliest-submitted runnable node. With d,c,b,a on
// the input, "a" unblocks c before b because c was submitted first.
func TestTopoSortTieBreakIsInputOrder(t *testing.T) {
	nodes := []Node{
		node("d", "b", "c"),
		node("c", "a"),
		node("b", "a"),
		node("a"),
	}
	got := mustTopoSort(t, nodes)
	want := []string{"a", "c", "b", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopoSort = %v, want %v", got, want)
	}
}

// TestTopoSortIndependentKeepsInputOrder: with no edges at all the order
// is exactly the submission order.
func TestTopoSortIndependentKeepsInputOrder(t *testing.T) {
	nodes := []Node{node("z"), node("m"), node("a")}
	got := mustTopoSort(t, nodes)
	want := []string{"z", "m", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopoSort = %v, want %v", got, want)
	}
}

// TestTopoSortDeterministic: repeated sorts of the same graph must be
// byte-identical. Guards against a map iteration ever leaking into the
// order (the node index is a map).
func TestTopoSortDeterministic(t *testing.T) {
	nodes := []Node{
		node("build", "gen"),
		node("gen"),
		node("test", "build"),
		node("lint", "gen"),
		node("pkg", "test", "lint"),
	}
	first := mustTopoSort(t, nodes)
	for i := 0; i < 50; i++ {
		got := mustTopoSort(t, nodes)
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d: TopoSort = %v, want %v", i, got, first)
		}
	}
}

// TestTopoSortRepeatedEdge: listing the same dependency twice is not an
// error and must not corrupt the in-degree bookkeeping.
func TestTopoSortRepeatedEdge(t *testing.T) {
	got := mustTopoSort(t, []Node{node("a", "b", "b", "b"), node("b")})
	want := []string{"b", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopoSort = %v, want %v", got, want)
	}
}

// TestTopoSortEmpty: no nodes is a valid (empty) graph.
func TestTopoSortEmpty(t *testing.T) {
	got, err := TopoSort(nil)
	if err != nil {
		t.Fatalf("TopoSort(nil) returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("TopoSort(nil) = %v, want empty", got)
	}
}

// --- cycle detection -------------------------------------------------

// TestTopoSortCycle: a three-node loop is rejected with the cycle spelled
// out as a closed path, so the error message points at the loop.
func TestTopoSortCycle(t *testing.T) {
	nodes := []Node{node("a", "b"), node("b", "c"), node("c", "a")}
	order, err := TopoSort(nodes)
	if err == nil {
		t.Fatalf("TopoSort accepted a cycle, returned %v", order)
	}
	if order != nil {
		t.Errorf("TopoSort returned a partial order with the error: %v", order)
	}
	var cyc *CycleError
	if !errors.As(err, &cyc) {
		t.Fatalf("error is %T (%v), want *CycleError", err, err)
	}
	want := []string{"a", "b", "c", "a"}
	if !reflect.DeepEqual(cyc.Cycle, want) {
		t.Errorf("Cycle = %v, want %v", cyc.Cycle, want)
	}
	if msg := cyc.Error(); !strings.Contains(msg, "a -> b -> c -> a") {
		t.Errorf("Error() = %q, want it to spell out the loop", msg)
	}
}

// TestTopoSortCycleAmongSortableNodes: an orderable node next to a loop
// does not hide the loop.
func TestTopoSortCycleAmongSortableNodes(t *testing.T) {
	nodes := []Node{node("ok"), node("a", "b"), node("b", "a")}
	_, err := TopoSort(nodes)
	var cyc *CycleError
	if !errors.As(err, &cyc) {
		t.Fatalf("error is %T (%v), want *CycleError", err, err)
	}
	want := []string{"a", "b", "a"}
	if !reflect.DeepEqual(cyc.Cycle, want) {
		t.Errorf("Cycle = %v, want %v", cyc.Cycle, want)
	}
}

// TestTopoSortSelfDependency: a task depending on itself is the shortest
// possible cycle and is reported as one.
func TestTopoSortSelfDependency(t *testing.T) {
	_, err := TopoSort([]Node{node("a", "a")})
	var cyc *CycleError
	if !errors.As(err, &cyc) {
		t.Fatalf("error is %T (%v), want *CycleError", err, err)
	}
	if want := []string{"a", "a"}; !reflect.DeepEqual(cyc.Cycle, want) {
		t.Errorf("Cycle = %v, want %v", cyc.Cycle, want)
	}
}

// --- input validation ------------------------------------------------

func TestTopoSortDuplicateID(t *testing.T) {
	_, err := TopoSort([]Node{node("a"), node("b"), node("a")})
	var dup *DuplicateIDError
	if !errors.As(err, &dup) {
		t.Fatalf("error is %T (%v), want *DuplicateIDError", err, err)
	}
	if dup.ID != "a" {
		t.Errorf("DuplicateIDError.ID = %q, want %q", dup.ID, "a")
	}
}

func TestTopoSortMissingDep(t *testing.T) {
	_, err := TopoSort([]Node{node("a", "ghost")})
	var miss *MissingDepError
	if !errors.As(err, &miss) {
		t.Fatalf("error is %T (%v), want *MissingDepError", err, err)
	}
	if miss.ID != "a" || miss.Dep != "ghost" {
		t.Errorf("MissingDepError = %+v, want {ID:a Dep:ghost}", *miss)
	}
}

func TestTopoSortEmptyID(t *testing.T) {
	if _, err := TopoSort([]Node{node("")}); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("error = %v, want ErrEmptyID", err)
	}
}
