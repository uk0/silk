package taskrunner

// Dependency-graph plumbing for the Runner: a DAG over task IDs, its
// topological order and the validation that rejects a malformed graph.
// Nothing in this file spawns a process or touches the OS, so the
// scheduling order is cheap to test on its own.

import (
	"fmt"
	"sort"
	"strings"
)

// Node is one vertex of the dependency graph: an ID plus the IDs that
// must finish before it may start. It is the graph-level projection of a
// Task (Task.ID / Task.DependsOn), which keeps this file free of any
// exec concern.
type Node struct {
	ID        string
	DependsOn []string
}

// CycleError reports that the submitted graph is not a DAG. Cycle is the
// offending path with its entry point repeated at both ends, e.g.
// ["build", "test", "build"], so the message reads as a loop.
type CycleError struct{ Cycle []string }

func (e *CycleError) Error() string {
	return "taskrunner: dependency cycle: " + strings.Join(e.Cycle, " -> ")
}

// DuplicateIDError reports two nodes sharing an ID. IDs address tasks in
// Cancel and in every Event, so they have to be unique.
type DuplicateIDError struct{ ID string }

func (e *DuplicateIDError) Error() string {
	return fmt.Sprintf("taskrunner: duplicate task id %q", e.ID)
}

// MissingDepError reports a dependency on an ID that is not part of the
// graph. Running the task anyway would silently ignore an unmet
// dependency, so it is an error instead.
type MissingDepError struct{ ID, Dep string }

func (e *MissingDepError) Error() string {
	return fmt.Sprintf("taskrunner: task %q depends on unknown task %q", e.ID, e.Dep)
}

// TopoSort returns every node ID in dependency order: an ID appears only
// after all of the IDs it depends on. It doubles as the graph validator
// and returns, without a partial result:
//
//   - ErrEmptyID for a node with no ID,
//   - *DuplicateIDError for a repeated ID,
//   - *MissingDepError for a dependency outside the graph,
//   - *CycleError when the remaining edges cannot be ordered.
//
// The order is deterministic and depends only on the input: Kahn's
// algorithm pops from a ready queue kept sorted by input position, so
// nodes that become runnable at the same time are emitted in submission
// order. The same nodes always yield the same slice, which is what makes
// a build log reproducible. Repeated edges ("a" listing "b" twice) are
// collapsed rather than rejected.
//
// The result is a total order; the Runner uses it as the scan order of
// its scheduler (and therefore as the tie-break between tasks that are
// ready at the same moment), not as a serialization of the run.
func TopoSort(nodes []Node) ([]string, error) {
	index := make(map[string]int, len(nodes))
	for i, n := range nodes {
		if n.ID == "" {
			return nil, ErrEmptyID
		}
		if _, dup := index[n.ID]; dup {
			return nil, &DuplicateIDError{ID: n.ID}
		}
		index[n.ID] = i
	}

	// indeg[i] counts the unsatisfied dependencies of nodes[i];
	// unblocks[j] lists the nodes waiting on nodes[j].
	indeg := make([]int, len(nodes))
	unblocks := make([][]int, len(nodes))
	for i, n := range nodes {
		seen := make(map[string]bool, len(n.DependsOn))
		for _, dep := range n.DependsOn {
			if dep == n.ID {
				return nil, &CycleError{Cycle: []string{n.ID, n.ID}}
			}
			j, ok := index[dep]
			if !ok {
				return nil, &MissingDepError{ID: n.ID, Dep: dep}
			}
			if seen[dep] {
				continue
			}
			seen[dep] = true
			indeg[i]++
			unblocks[j] = append(unblocks[j], i)
		}
	}

	// ready holds input positions, ascending — the tie-break.
	var ready []int
	for i := range nodes {
		if indeg[i] == 0 {
			ready = append(ready, i)
		}
	}
	order := make([]string, 0, len(nodes))
	for len(ready) > 0 {
		i := ready[0]
		ready = ready[1:]
		order = append(order, nodes[i].ID)
		for _, j := range unblocks[i] {
			indeg[j]--
			if indeg[j] == 0 {
				ready = insertReady(ready, j)
			}
		}
	}
	if len(order) != len(nodes) {
		return nil, &CycleError{Cycle: findCycle(nodes, index)}
	}
	return order, nil
}

// insertReady inserts idx into an ascending queue of input positions,
// keeping it sorted so the head is always the earliest-submitted node
// that is currently runnable.
func insertReady(ready []int, idx int) []int {
	at := sort.SearchInts(ready, idx)
	ready = append(ready, 0)
	copy(ready[at+1:], ready[at:])
	ready[at] = idx
	return ready
}

// findCycle returns one cycle as a path whose first and last element are
// the same ID, for the error message. It walks the dependency edges in
// input order with the classic white/grey/black DFS, so the reported
// cycle is deterministic. It returns nil when the graph is acyclic (only
// reachable if a caller uses it outside TopoSort).
func findCycle(nodes []Node, index map[string]int) []string {
	const (
		white = iota
		grey
		black
	)
	color := make([]int, len(nodes))
	var path []string

	var walk func(i int) []string
	walk = func(i int) []string {
		color[i] = grey
		path = append(path, nodes[i].ID)
		for _, dep := range nodes[i].DependsOn {
			j, ok := index[dep]
			if !ok {
				continue
			}
			switch color[j] {
			case white:
				if c := walk(j); c != nil {
					return c
				}
			case grey:
				// Back edge: the cycle is the tail of the current path
				// starting at dep, closed by dep itself.
				for k, id := range path {
					if id == dep {
						return append(append([]string(nil), path[k:]...), dep)
					}
				}
			}
		}
		path = path[:len(path)-1]
		color[i] = black
		return nil
	}

	for i := range nodes {
		if color[i] != white {
			continue
		}
		if c := walk(i); c != nil {
			return c
		}
	}
	return nil
}
