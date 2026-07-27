package ged

import "strings"

// TestNodeStatus is the state of one node in the test-explorer tree.
//
// It is deliberately a different enum from TestStatus in
// test-results-panel.go: that one buckets a finished `go test -v` line
// into pass/fail/skip, while the explorer models a *persistent* tree
// where most nodes are, at any moment, either merely discovered
// (Unknown) or in flight (Running).
type TestNodeStatus int

const (
	TestNodeUnknown TestNodeStatus = iota // discovered, not run (or not run yet in this session)
	TestNodeRunning                       // currently executing
	TestNodePass
	TestNodeFail
	TestNodeSkip
)

// String renders the status as its lower-case name, matching the
// vocabulary the runner uses (and keeping test failures readable).
func (s TestNodeStatus) String() string {
	switch s {
	case TestNodeRunning:
		return "running"
	case TestNodePass:
		return "pass"
	case TestNodeFail:
		return "fail"
	case TestNodeSkip:
		return "skip"
	}
	return "unknown"
}

// TestNode is one incoming test result pushed into the explorer. Name is
// the test's full path within its package as the runner spells it —
// "TestFoo" for a top-level test, "TestFoo/sub_case" for a subtest — and
// the model splits it on "/" to nest subtests under their parent.
//
// This is the explorer's own provider-agnostic input shape: whoever
// drives it (a `go test -json` reader, a discovery pass that only knows
// test names, or a unit test) converts to TestNode first. The panel
// therefore never depends on a particular runner package.
type TestNode struct {
	Name    string
	Status  TestNodeStatus
	Elapsed float64 // seconds; 0 when unknown
	Output  string  // captured output for this test
	File    string  // failure locator file, "" when none
	Line    int     // 1-based line that goes with File
}

// PkgNode is one package's worth of incoming results: the package's own
// state (a build failure gives a package a Fail with no tests) plus a
// flat list of the tests seen in this run.
type PkgNode struct {
	Path    string // import path, e.g. "github.com/uk0/silk/ged"
	Status  TestNodeStatus
	Elapsed float64
	Output  string
	Tests   []TestNode
}

// TestTreeNode is one node of the explorer's live tree: a package, a
// test, or a subtest. Nodes are created once and then merged into, so
// pointers stay valid across runs — the panel can hold on to one as its
// selection and the expansion flag survives a re-run.
type TestTreeNode struct {
	Name     string // display label: the import path for packages, the leaf segment for tests
	Pkg      string // owning import path (== Name on package nodes)
	Test     string // full test path within the package; "" on package nodes
	Status   TestNodeStatus
	Elapsed  float64
	Output   string // cached output from the last run that produced any
	File     string
	Line     int
	Expanded bool
	Children []*TestTreeNode
}

// IsPackage reports whether n is a package (root) node.
func (n *TestTreeNode) IsPackage() bool { return n.Test == "" }

// TestRef identifies one test inside a package. Returned by FailedTests
// so the host can build the `go test -run` command for a re-run without
// walking the tree itself.
type TestRef struct {
	Pkg  string
	Test string
}

// TestExplorerRow is one visible row of the flattened tree: the node
// plus the indent depth it renders at (0 for packages).
type TestExplorerRow struct {
	Node  *TestTreeNode
	Depth int
}

// TestExplorerModel is the pure, GL-free model behind TestExplorerPanel:
// a package -> test -> subtest tree with per-node status/elapsed/output,
// a text filter, a failed-only toggle, and a flatten-to-rows pass that
// honours each node's expansion flag.
//
// Merging (not replacing) is the point of the type. A `go test -run
// TestFoo` re-run only reports TestFoo, but the explorer must keep
// showing yesterday's verdict for everything else — so SetResults folds
// the new run into the existing tree and leaves untouched nodes alone.
type TestExplorerModel struct {
	pkgs       []*TestTreeNode          // package nodes, in first-seen order
	index      map[string]*TestTreeNode // nodeKey(pkg, test) -> node
	filter     string                   // lower-cased, "" when inactive
	failedOnly bool
}

// NewTestExplorerModel creates an empty model.
func NewTestExplorerModel() *TestExplorerModel {
	return &TestExplorerModel{index: make(map[string]*TestTreeNode)}
}

// nodeKey is the index key for a node. The NUL separator can't occur in
// an import path or a test name, so "pkg" and "pkg/test" can never
// collide.
func nodeKey(pkg, test string) string { return pkg + "\x00" + test }

// Packages returns the package nodes in first-seen order. The nodes are
// live — the panel walks them for drawing and flips Expanded on them.
func (m *TestExplorerModel) Packages() []*TestTreeNode { return m.pkgs }

// Find returns the node for a package (test == "") or a full test path
// within it, or nil when it was never seen.
func (m *TestExplorerModel) Find(pkg, test string) *TestTreeNode {
	return m.index[nodeKey(pkg, test)]
}

// Clear drops the whole tree, keeping the filter settings.
func (m *TestExplorerModel) Clear() {
	m.pkgs = nil
	m.index = make(map[string]*TestTreeNode)
}

// SetResults merges a run's results into the tree. Packages and tests
// that are absent from pkgs keep their previous status, elapsed time and
// output, so a targeted re-run never blanks the rest of the tree.
//
// Per field, an incoming value only overwrites when it carries
// information: Status when it is not Unknown, Elapsed when positive,
// Output and File when non-empty. That makes progressive pushes work —
// a "running" update followed by a "fail" update accumulates instead of
// erasing what came before.
func (m *TestExplorerModel) SetResults(pkgs []PkgNode) {
	if m.index == nil {
		m.index = make(map[string]*TestTreeNode)
	}
	for _, in := range pkgs {
		if in.Path == "" {
			continue
		}
		mergeTestNode(m.pkgNode(in.Path), in.Status, in.Elapsed, in.Output, "", 0)
		for _, t := range in.Tests {
			name := strings.TrimSpace(t.Name)
			if name == "" {
				continue
			}
			mergeTestNode(m.testNode(in.Path, name), t.Status, t.Elapsed, t.Output, t.File, t.Line)
		}
	}
}

// mergeTestNode folds one incoming result onto an existing node under the
// "only meaningful values overwrite" rule documented on SetResults.
func mergeTestNode(n *TestTreeNode, st TestNodeStatus, elapsed float64, output, file string, line int) {
	if st != TestNodeUnknown {
		n.Status = st
	}
	if elapsed > 0 {
		n.Elapsed = elapsed
	}
	if output != "" {
		n.Output = output
	}
	if file != "" {
		n.File = file
		n.Line = line
	}
}

// pkgNode returns the package node for path, creating (and appending) it
// on first sight. New nodes start expanded so a fresh run shows its
// tests without a click.
func (m *TestExplorerModel) pkgNode(path string) *TestTreeNode {
	k := nodeKey(path, "")
	if n, ok := m.index[k]; ok {
		return n
	}
	n := &TestTreeNode{Name: path, Pkg: path, Expanded: true}
	m.index[k] = n
	m.pkgs = append(m.pkgs, n)
	return n
}

// testNode returns the node for a full test path inside pkg, creating
// every missing "/"-separated ancestor along the way so a subtest always
// nests under its parent test even when the runner reported the subtest
// first (or the parent not at all).
func (m *TestExplorerModel) testNode(pkg, test string) *TestTreeNode {
	parent := m.pkgNode(pkg)
	var n *TestTreeNode
	path := ""
	for _, seg := range strings.Split(test, "/") {
		if path == "" {
			path = seg
		} else {
			path += "/" + seg
		}
		n = m.index[nodeKey(pkg, path)]
		if n == nil {
			n = &TestTreeNode{Name: seg, Pkg: pkg, Test: path, Expanded: true}
			m.index[nodeKey(pkg, path)] = n
			parent.Children = append(parent.Children, n)
		}
		parent = n
	}
	return n
}

// --- Filtering ---

// SetFilter sets the case-insensitive substring filter applied to node
// names (and, for tests, to their full "TestFoo/sub" path). An empty
// text clears it.
func (m *TestExplorerModel) SetFilter(text string) {
	m.filter = strings.ToLower(strings.TrimSpace(text))
}

// FilterText returns the active filter, lower-cased, "" when inactive.
func (m *TestExplorerModel) FilterText() string { return m.filter }

// SetFailedOnly restricts the rows to failures and the ancestors that
// lead to them.
func (m *TestExplorerModel) SetFailedOnly(b bool) { m.failedOnly = b }

// FailedOnly reports whether the failed-only toggle is on.
func (m *TestExplorerModel) FailedOnly() bool { return m.failedOnly }

// textMatch reports whether the node itself satisfies the text filter.
func (m *TestExplorerModel) textMatch(n *TestTreeNode) bool {
	if m.filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(n.Name), m.filter) ||
		strings.Contains(strings.ToLower(n.Test), m.filter)
}

// descendantTextMatch reports whether anything below n satisfies the
// text filter — that is what keeps a package row on screen when only one
// of its subtests matches.
func (m *TestExplorerModel) descendantTextMatch(n *TestTreeNode) bool {
	for _, c := range n.Children {
		if m.textMatch(c) || m.descendantTextMatch(c) {
			return true
		}
	}
	return false
}

// failedOK applies the failed-only toggle. A node survives when it
// failed itself or when it is on the path to a failure; unlike the text
// filter it is a hard per-node predicate, so a failed package does not
// drag its passing tests back into view.
func (m *TestExplorerModel) failedOK(n *TestTreeNode) bool {
	if !m.failedOnly {
		return true
	}
	return n.Status == TestNodeFail || hasFailedDescendant(n)
}

func hasFailedDescendant(n *TestTreeNode) bool {
	for _, c := range n.Children {
		if c.Status == TestNodeFail || hasFailedDescendant(c) {
			return true
		}
	}
	return false
}

// --- Flattening ---

// Rows flattens the tree into the visible row sequence: packages in
// first-seen order, each expanded node followed by its children, with
// the filter and the failed-only toggle applied. Pure — the panel calls
// it from Draw and from its hit-test so both always agree.
func (m *TestExplorerModel) Rows() []TestExplorerRow {
	var out []TestExplorerRow
	for _, p := range m.pkgs {
		out = m.appendRows(out, p, 0, false)
	}
	return out
}

// appendRows walks one subtree. textOK is true once an ancestor matched
// the text filter on its own merit, which reveals its whole subtree the
// way a tree filter is expected to behave.
func (m *TestExplorerModel) appendRows(out []TestExplorerRow, n *TestTreeNode, depth int, textOK bool) []TestExplorerRow {
	if !m.failedOK(n) {
		return out
	}
	self := m.textMatch(n)
	if !textOK && !self && !m.descendantTextMatch(n) {
		return out
	}
	out = append(out, TestExplorerRow{Node: n, Depth: depth})
	if !n.Expanded {
		return out
	}
	for _, c := range n.Children {
		out = m.appendRows(out, c, depth+1, textOK || self)
	}
	return out
}

// ExpandAll expands every node, CollapseAll collapses every node —
// including the packages, which then render as a one-line-per-package
// summary.
func (m *TestExplorerModel) ExpandAll()   { m.setExpandedAll(true) }
func (m *TestExplorerModel) CollapseAll() { m.setExpandedAll(false) }

func (m *TestExplorerModel) setExpandedAll(v bool) {
	for _, n := range m.index {
		n.Expanded = v
	}
}

// Counts tallies the leaf tests by outcome. Only leaves count: a parent
// test's verdict is just the roll-up of its subtests, and counting both
// would double-report.
func (m *TestExplorerModel) Counts() (pass, fail, skip int) {
	for _, p := range m.pkgs {
		walkTestLeaves(p, func(n *TestTreeNode) {
			switch n.Status {
			case TestNodePass:
				pass++
			case TestNodeFail:
				fail++
			case TestNodeSkip:
				skip++
			}
		})
	}
	return
}

// walkTestLeaves calls fn for every childless test node under n. Package
// nodes never reach fn, so an empty package contributes nothing.
func walkTestLeaves(n *TestTreeNode, fn func(*TestTreeNode)) {
	if len(n.Children) == 0 {
		if !n.IsPackage() {
			fn(n)
		}
		return
	}
	for _, c := range n.Children {
		walkTestLeaves(c, fn)
	}
}

// FailedTests returns the top-most failed tests: when a parent test and
// its subtests all failed, only the parent is listed, since re-running
// it re-runs the subtests too.
func (m *TestExplorerModel) FailedTests() []TestRef {
	var out []TestRef
	var walk func(n *TestTreeNode)
	walk = func(n *TestTreeNode) {
		if !n.IsPackage() && n.Status == TestNodeFail {
			out = append(out, TestRef{Pkg: n.Pkg, Test: n.Test})
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, p := range m.pkgs {
		walk(p)
	}
	return out
}
