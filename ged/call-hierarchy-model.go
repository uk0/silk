package ged

import "strconv"

// CallDirection selects which side of the call graph the hierarchy walks.
// Qt Creator's call hierarchy view has exactly these two modes and a
// toggle between them; the tree is rebuilt from scratch on a switch
// because "who calls me" and "who do I call" share no nodes.
type CallDirection int

const (
	// CallIncoming lists the callers of the root symbol (who calls me).
	CallIncoming CallDirection = iota
	// CallOutgoing lists the callees of the root symbol (who I call).
	CallOutgoing
)

// String renders the direction as the wire-ish name used in labels and
// logs: "incoming" / "outgoing".
func (d CallDirection) String() string {
	if d == CallOutgoing {
		return "outgoing"
	}
	return "incoming"
}

// CallNode is one symbol in the call hierarchy tree — the root is the
// symbol under the cursor, its children are its callers (incoming) or
// callees (outgoing), and so on down.
//
// Children are fetched lazily: a freshly created node has Loaded == false
// and no Children, and only when the user expands it does the model ask
// the host to resolve that one level (see CallHierarchyModel.SigExpand).
// That keeps the panel off a full transitive closure, which for a hot
// function is effectively unbounded.
//
// Like the sibling panels' row structs this is the panel's own shape, NOT
// an LSP type: the host drives gopls (callHierarchy/incomingCalls etc.),
// converts each result into a CallNode with a 1-based Line, and pushes it
// in via SetChildren. The model never learns about LSP wire types.
type CallNode struct {
	Name   string // symbol name, e.g. "Frame.Draw"
	Detail string // signature / container detail shown dimmed after the name
	File   string // absolute or workspace-relative path to the definition
	Line   int    // 1-based line of the definition (or of the call site)
	Kind   string // "func", "method", "closure", … (host's vocabulary)

	Children []*CallNode // resolved one level; nil until Loaded
	Expanded bool        // user has opened this node
	Loaded   bool        // children have been resolved (possibly to zero)

	// Recursive marks a node whose (File, Line, Name) identity already
	// appears on its own ancestor path. Recursion is the normal case in a
	// call graph (direct or mutual), and expanding such a node would walk
	// the same cycle forever — so the model flags it, refuses to expand it
	// and the panel renders it as a leaf with a cycle marker. Set by
	// SetChildren; hosts do not fill it in.
	Recursive bool
}

// callNodeKey is the identity used for cycle detection: file + line +
// name. Pointer identity is not enough (the host builds a fresh CallNode
// per fetch, so the same function reached twice is two pointers), and the
// name alone is too coarse (same method name on different types).
func callNodeKey(n *CallNode) string {
	if n == nil {
		return ""
	}
	return n.File + ":" + strconv.Itoa(n.Line) + ":" + n.Name
}

// callNodePath returns the chain of nodes from root down to target
// (inclusive), or nil when target is not reachable from root. The walk
// carries an in-path visited set so a host-built pointer cycle cannot make
// it recurse forever.
func callNodePath(root, target *CallNode) []*CallNode {
	if root == nil || target == nil {
		return nil
	}
	inPath := make(map[*CallNode]bool)
	var walk func(n *CallNode) []*CallNode
	walk = func(n *CallNode) []*CallNode {
		if n == nil || inPath[n] {
			return nil
		}
		if n == target {
			return []*CallNode{n}
		}
		inPath[n] = true
		defer delete(inPath, n)
		for _, c := range n.Children {
			if sub := walk(c); sub != nil {
				return append([]*CallNode{n}, sub...)
			}
		}
		return nil
	}
	return walk(root)
}

// callNodeExpandable reports whether a node should draw an expander
// triangle. An unloaded node always gets one — its children are unknown
// until fetched, exactly like Qt Creator's optimistic expanders — a loaded
// node only when the fetch actually produced children, and a recursive
// node never.
func callNodeExpandable(n *CallNode) bool {
	if n == nil || n.Recursive {
		return false
	}
	if !n.Loaded {
		return true
	}
	return len(n.Children) > 0
}

// CallRow is one visible line of the flattened tree: the node, its
// indentation depth (root == 0) and whether it draws an expander. The
// panel renders and hit-tests rows; it never walks the tree itself.
type CallRow struct {
	Node       *CallNode
	Depth      int
	Expandable bool
}

// flattenCallTree walks the tree in display order, descending only into
// expanded (and non-recursive) nodes. Pure: no widget, no GL. The in-path
// visited set means even a cyclic pointer graph terminates, while a node
// legitimately reached twice on *different* branches still shows twice.
func flattenCallTree(root *CallNode) []CallRow {
	if root == nil {
		return nil
	}
	var rows []CallRow
	inPath := make(map[*CallNode]bool)
	var walk func(n *CallNode, depth int)
	walk = func(n *CallNode, depth int) {
		if n == nil || inPath[n] {
			return
		}
		rows = append(rows, CallRow{Node: n, Depth: depth, Expandable: callNodeExpandable(n)})
		if !n.Expanded || n.Recursive {
			return
		}
		inPath[n] = true
		defer delete(inPath, n)
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(root, 0)
	return rows
}

// CallHierarchyModel is the pure (GL-free) tree behind
// CallHierarchyPanel: one root, a direction, lazy per-node expansion
// driven by a host callback, cycle detection, and a generation counter
// that makes late fetch results harmless.
//
// The fetch protocol is deliberately callback-in / push-back-in, matching
// the other panels' "host drives the tooling" split:
//
//	m.SigExpand(func(n *ged.CallNode) { go resolve(n) })   // host fetches
//	…later, on the UI thread:  m.SetChildren(n, kids)      // host pushes
//
// Because the answer arrives after an unknown delay, every Expand stamps
// the request with the current generation. Anything that invalidates the
// tree — a new root, a direction switch, Clear, an explicit Cancel — bumps
// the generation, so a result for the old world is dropped by SetChildren
// instead of grafting stale callers onto the new tree.
type CallHierarchyModel struct {
	root      *CallNode
	direction CallDirection
	gen       int
	pending   map[*CallNode]int // node -> generation the fetch was issued under
	cbExpand  func(node *CallNode)
}

// NewCallHierarchyModel creates an empty incoming-direction model.
func NewCallHierarchyModel() *CallHierarchyModel {
	return &CallHierarchyModel{
		direction: CallIncoming,
		gen:       1,
		pending:   make(map[*CallNode]int),
	}
}

// Root returns the tree root, or nil when the model is empty.
func (m *CallHierarchyModel) Root() *CallNode { return m.root }

// SetRoot replaces the tree with a new root symbol and invalidates every
// in-flight fetch. The node is taken as-is (a host that already has the
// first level may hand it in pre-populated with Loaded == true).
func (m *CallHierarchyModel) SetRoot(root *CallNode) {
	m.invalidate()
	m.root = root
}

// Clear drops the tree and invalidates every in-flight fetch.
func (m *CallHierarchyModel) Clear() {
	m.invalidate()
	m.root = nil
}

// Direction returns the current walk direction.
func (m *CallHierarchyModel) Direction() CallDirection { return m.direction }

// Incoming reports whether the model is walking callers (true) or callees
// (false).
func (m *CallHierarchyModel) Incoming() bool { return m.direction == CallIncoming }

// SetDirection switches between callers and callees. On a real change the
// tree is reset to the bare root (children dropped, root collapsed and
// marked unloaded) and the generation is bumped, so the host is expected
// to re-resolve the first level in the new direction. It returns whether
// anything changed.
func (m *CallHierarchyModel) SetDirection(d CallDirection) bool {
	if d == m.direction {
		return false
	}
	m.direction = d
	m.invalidate()
	if m.root != nil {
		m.root.Children = nil
		m.root.Expanded = false
		m.root.Loaded = false
		m.root.Recursive = false
	}
	return true
}

// SigExpand registers the callback the model fires when a node needs its
// children resolved. It runs at most once per node per generation; the
// host answers by calling SetChildren (immediately or later).
func (m *CallHierarchyModel) SigExpand(fn func(node *CallNode)) {
	m.cbExpand = fn
}

// Generation returns the current fetch generation. It only ever grows;
// results stamped with an older value are stale.
func (m *CallHierarchyModel) Generation() int { return m.gen }

// PendingCount returns how many fetches are in flight for the current
// generation. Requests left over from an invalidated generation are not
// counted — they are already cancelled, they are only kept around long
// enough to recognise (and refuse) a late answer.
func (m *CallHierarchyModel) PendingCount() int {
	n := 0
	for _, gen := range m.pending {
		if gen == m.gen {
			n++
		}
	}
	return n
}

// Cancel invalidates every in-flight fetch without touching the tree —
// results that arrive afterwards are ignored.
func (m *CallHierarchyModel) Cancel() { m.invalidate() }

// invalidate bumps the generation, which cancels every in-flight fetch.
//
// The request ledger deliberately survives the bump for one generation:
// entries stamped with the outgoing generation are what lets SetChildren
// recognise a late answer as stale instead of silently grafting it onto
// the new tree. Entries older than that have already had their chance to
// be refused, so they are pruned here and the ledger stays bounded.
func (m *CallHierarchyModel) invalidate() {
	for n, gen := range m.pending {
		if gen != m.gen {
			delete(m.pending, n)
		}
	}
	m.gen++
}

// Contains reports whether node belongs to the current tree.
func (m *CallHierarchyModel) Contains(node *CallNode) bool {
	return callNodePath(m.root, node) != nil
}

// Expand opens a node, requesting its children from the host when they
// have not been resolved yet. Recursive nodes, unknown nodes and nil are
// no-ops, and a node whose fetch is already in flight does not fire a
// second request.
func (m *CallHierarchyModel) Expand(node *CallNode) {
	if node == nil || node.Recursive || !m.Contains(node) {
		return
	}
	node.Expanded = true
	if node.Loaded {
		return
	}
	if gen, inflight := m.pending[node]; inflight && gen == m.gen {
		return
	}
	m.pending[node] = m.gen
	if m.cbExpand != nil {
		m.cbExpand(node)
	}
}

// Collapse closes a node. Already-resolved children are kept so
// re-expanding costs nothing.
func (m *CallHierarchyModel) Collapse(node *CallNode) {
	if node == nil {
		return
	}
	node.Expanded = false
}

// Toggle collapses an expanded node and expands a collapsed one.
func (m *CallHierarchyModel) Toggle(node *CallNode) {
	if node == nil {
		return
	}
	if node.Expanded {
		m.Collapse(node)
		return
	}
	m.Expand(node)
}

// SetChildren attaches one resolved level to node and reports whether it
// was applied. Only an answer to a live request is accepted; it is
// dropped — returning false, leaving the tree untouched — when
//
//   - no fetch is outstanding for node (nothing asked for these children,
//     or an earlier answer already satisfied the request),
//   - the fetch was issued under an older generation, i.e. SetRoot /
//     SetDirection / Clear / Cancel happened while it was in flight, or
//   - node is no longer part of the current tree.
//
// A host doing the fetch asynchronously can also compare Generation()
// before pushing, but does not have to: the guards above make a late
// answer harmless.
//
// Children whose (file, line, name) identity already appears on node's
// ancestor path (node itself included) are flagged Recursive and pinned
// collapsed, so the cycle is shown once and never walked.
func (m *CallHierarchyModel) SetChildren(node *CallNode, children []*CallNode) bool {
	if node == nil {
		return false
	}
	gen, requested := m.pending[node]
	if !requested {
		return false
	}
	delete(m.pending, node)
	if gen != m.gen {
		return false
	}
	path := callNodePath(m.root, node)
	if path == nil {
		return false
	}

	ancestors := make(map[string]bool, len(path))
	for _, a := range path {
		ancestors[callNodeKey(a)] = true
	}

	node.Children = nil
	for _, c := range children {
		if c == nil {
			continue
		}
		if ancestors[callNodeKey(c)] {
			c.Recursive = true
			c.Expanded = false
		}
		node.Children = append(node.Children, c)
	}
	node.Loaded = true
	return true
}

// Rows flattens the tree into display order, honouring Expanded.
func (m *CallHierarchyModel) Rows() []CallRow {
	return flattenCallTree(m.root)
}
