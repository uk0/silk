package ged

import (
	"path/filepath"
	"strconv"
)

// Hierarchy directions offered by the panel, in selector order. The host maps
// each to its own resolver — gopls `typeHierarchy/supertypes`,
// `typeHierarchy/subtypes` and `textDocument/implementation` respectively —
// and pushes the answers back as TypeNodes. The model never sees those wire
// structs.
const (
	TypeHierarchySupertypes      = "supertypes"
	TypeHierarchySubtypes        = "subtypes"
	TypeHierarchyImplementations = "implementations"
)

// TypeNode kinds. The first three describe real Go declarations; TypeKindTargets
// marks only the synthetic root the model inserts when the host resolves several
// candidate targets for one cursor position (see SetTargets).
const (
	TypeKindInterface = "interface"
	TypeKindStruct    = "struct"
	TypeKindMethod    = "method"
	TypeKindTargets   = "targets"
)

// typeHierarchyModes lists the selectable directions left-to-right, matching
// the panel's mode buttons.
var typeHierarchyModes = []string{
	TypeHierarchySupertypes,
	TypeHierarchySubtypes,
	TypeHierarchyImplementations,
}

// TypeNode is one entry in the type hierarchy: a named declaration plus its
// source location and its lazily-fetched relatives in the current direction.
//
// It is the panel's own shape, deliberately free of LSP types: the host drives
// gopls, converts each result into a TypeNode (file path + 1-based line) and
// hands it over. Children stay nil until the user expands the node — Loaded
// distinguishes "no relatives" (Loaded, empty) from "not asked yet" (not
// Loaded), which is what makes the twisty and the lazy fetch decidable.
//
// The model takes ownership of the nodes handed to SetTargets/SetChildren: it
// mutates Expanded/Loaded/Children in place and hands the same pointers back
// through SigExpand, which is how an async reply finds its parent again. A
// given *TypeNode must therefore appear at only one place in the tree; hosts
// that resolve the same type twice should build two nodes.
type TypeNode struct {
	Name string // declaration name, e.g. "Painter" or "Widget.Draw"
	Pkg  string // package name or import path, host's choice; may be empty
	File string // source file; empty for the synthetic multi-target root
	Line int    // 1-based line of the declaration
	Kind string // interface | struct | method (| targets for the synthetic root)

	Children []*TypeNode
	Expanded bool
	Loaded   bool
}

// TypeHierarchyRow is one visible line of the flattened tree: the node, its
// indentation depth, and whether it merely repeats an ancestor. Cyclic rows are
// rendered as stubs and cannot be expanded — that is the cycle guard, and it is
// what keeps mutually-referencing types (A implements B implements A) from
// flattening forever.
type TypeHierarchyRow struct {
	Node   *TypeNode
	Depth  int
	Cyclic bool
}

// TypeHierarchyModel is the tree behind TypeHierarchyPanel: a mode, a set of
// roots, and the flattened row list derived from them. It is pure state — no
// widget, no GL, no LSP — so all of the expansion, cycle and multi-target
// behaviour is unit-testable on its own.
type TypeHierarchyModel struct {
	mode  string
	roots []*TypeNode
	rows  []TypeHierarchyRow

	cbExpand func(node *TypeNode, mode string)
}

// NewTypeHierarchyModel returns an empty model in supertypes mode.
func NewTypeHierarchyModel() *TypeHierarchyModel {
	return &TypeHierarchyModel{mode: TypeHierarchySupertypes}
}

// Mode returns the active direction.
func (m *TypeHierarchyModel) Mode() string { return m.mode }

// SetMode switches direction and resets the tree: the old roots describe a
// different relation and cannot be reused. It reports whether anything changed
// — false for an unknown mode or a re-selection of the current one — so the
// panel only signals the host (which must re-resolve) on a real switch.
func (m *TypeHierarchyModel) SetMode(mode string) bool {
	if mode == m.mode || !validTypeHierarchyMode(mode) {
		return false
	}
	m.mode = mode
	m.roots = nil
	m.rows = nil
	return true
}

// validTypeHierarchyMode reports whether mode is one of the three directions.
func validTypeHierarchyMode(mode string) bool {
	for _, m := range typeHierarchyModes {
		if m == mode {
			return true
		}
	}
	return false
}

// SetTargets replaces the tree with the host's resolved candidates. One
// candidate becomes the root directly; several are gathered under a synthetic
// "multiple targets" root (Kind TypeKindTargets, already expanded and loaded)
// so an ambiguous cursor position — an embedded name, a method promoted from
// two interfaces — still yields a single tree instead of a silent guess. nil
// entries are dropped; an empty list clears the view.
func (m *TypeHierarchyModel) SetTargets(targets []*TypeNode) {
	var roots []*TypeNode
	for _, n := range targets {
		if n != nil {
			roots = append(roots, n)
		}
	}
	switch len(roots) {
	case 0:
		m.roots = nil
	case 1:
		m.roots = roots
	default:
		m.roots = []*TypeNode{{
			Name:     typeHierTargetsLabel(len(roots)),
			Kind:     TypeKindTargets,
			Children: roots,
			Expanded: true,
			Loaded:   true,
		}}
	}
	m.rebuild()
}

// IsMultiTarget reports whether the tree is headed by the synthetic
// multiple-targets root.
func (m *TypeHierarchyModel) IsMultiTarget() bool {
	return len(m.roots) == 1 && m.roots[0].Kind == TypeKindTargets
}

// Roots returns the tree roots (a copy of the slice; the nodes are shared).
func (m *TypeHierarchyModel) Roots() []*TypeNode {
	out := make([]*TypeNode, len(m.roots))
	copy(out, m.roots)
	return out
}

// Rows returns the flattened, indented rows in display order (a copy of the
// slice; the nodes are shared).
func (m *TypeHierarchyModel) Rows() []TypeHierarchyRow {
	out := make([]TypeHierarchyRow, len(m.rows))
	copy(out, m.rows)
	return out
}

// RowCount returns the number of visible rows.
func (m *TypeHierarchyModel) RowCount() int { return len(m.rows) }

// RowAt returns the row at a flat index, and false when the index is out of
// range.
func (m *TypeHierarchyModel) RowAt(i int) (TypeHierarchyRow, bool) {
	if i < 0 || i >= len(m.rows) {
		return TypeHierarchyRow{}, false
	}
	return m.rows[i], true
}

// Clear empties the tree, keeping the mode.
func (m *TypeHierarchyModel) Clear() {
	m.roots = nil
	m.rows = nil
}

// SigExpand registers the lazy-fetch callback. It fires when the user expands a
// node whose relatives have never been fetched, receiving the node and the
// current mode; the host resolves them (possibly asynchronously) and calls
// SetChildren with the same node pointer.
func (m *TypeHierarchyModel) SigExpand(fn func(node *TypeNode, mode string)) {
	m.cbExpand = fn
}

// ToggleRow expands or collapses the node at a flat row index, rebuilding the
// rows. Expanding a not-yet-Loaded node fires SigExpand and leaves the node
// expanded so the children appear as soon as SetChildren lands. Leaves and
// cyclic stubs are inert. It reports whether the row toggled.
func (m *TypeHierarchyModel) ToggleRow(i int) bool {
	row, ok := m.RowAt(i)
	if !ok || row.Cyclic || !typeNodeExpandable(row.Node) {
		return false
	}
	n := row.Node
	n.Expanded = !n.Expanded
	if n.Expanded && !n.Loaded && m.cbExpand != nil {
		m.cbExpand(n, m.mode)
	}
	m.rebuild()
	return true
}

// SetChildren installs a lazily-fetched result: it marks node Loaded (so an
// empty answer becomes a leaf rather than re-asking forever) and replaces its
// children, dropping nils and any self-reference. It reports false when node is
// nil or no longer part of the tree — the guard that makes a late reply from a
// superseded query (a different mode, a new target) harmless.
func (m *TypeHierarchyModel) SetChildren(node *TypeNode, children []*TypeNode) bool {
	if node == nil || !m.contains(node) {
		return false
	}
	node.Children = nil
	for _, c := range children {
		if c == nil || c == node {
			continue
		}
		node.Children = append(node.Children, c)
	}
	node.Loaded = true
	m.rebuild()
	return true
}

// contains reports whether node is reachable from the roots. The walk tracks
// visited pointers so a tree that already contains a cycle cannot hang it.
func (m *TypeHierarchyModel) contains(node *TypeNode) bool {
	seen := make(map[*TypeNode]bool)
	var walk func(nodes []*TypeNode) bool
	walk = func(nodes []*TypeNode) bool {
		for _, n := range nodes {
			if n == nil || seen[n] {
				continue
			}
			if n == node {
				return true
			}
			seen[n] = true
			if walk(n.Children) {
				return true
			}
		}
		return false
	}
	return walk(m.roots)
}

// rebuild flattens the expanded tree into rows. Descent stops at a node whose
// identity already appears among its own ancestors: the repeat is emitted once,
// marked Cyclic, and not followed — bounding both the row count and the
// recursion depth for hierarchies that loop.
func (m *TypeHierarchyModel) rebuild() {
	m.rows = nil
	ancestors := make(map[string]bool)
	var walk func(nodes []*TypeNode, depth int)
	walk = func(nodes []*TypeNode, depth int) {
		for _, n := range nodes {
			if n == nil {
				continue
			}
			key := typeNodeKey(n)
			cyclic := ancestors[key]
			m.rows = append(m.rows, TypeHierarchyRow{Node: n, Depth: depth, Cyclic: cyclic})
			if cyclic || !n.Expanded || len(n.Children) == 0 {
				continue
			}
			ancestors[key] = true
			walk(n.Children, depth+1)
			delete(ancestors, key)
		}
	}
	walk(m.roots, 0)
}

// typeNodeExpandable reports whether a node should carry a twisty: either its
// relatives are known and non-empty, or they have never been fetched.
func typeNodeExpandable(n *TypeNode) bool {
	return n != nil && (!n.Loaded || len(n.Children) > 0)
}

// typeNodeKey is a node's identity for the cycle guard: the package-qualified
// name plus the kind. Two nodes with the same key denote the same declaration,
// however many times the host resolved it.
func typeNodeKey(n *TypeNode) string {
	if n == nil {
		return ""
	}
	return n.Pkg + "." + n.Name + ":" + n.Kind
}

// --- Pure label helpers (GL-free, unit-testable) ---

// typeHierTargetsLabel names the synthetic multiple-targets root.
func typeHierTargetsLabel(n int) string {
	return "多个目标 / " + strconv.Itoa(n) + " targets"
}

// typeHierarchyModeLabel is the button text for a direction.
func typeHierarchyModeLabel(mode string) string {
	switch mode {
	case TypeHierarchySupertypes:
		return "父类型"
	case TypeHierarchySubtypes:
		return "子类型"
	case TypeHierarchyImplementations:
		return "实现"
	}
	return mode
}

// typeHierarchyTitle renders the header, e.g. "类型层次 / 实现 (4)".
func typeHierarchyTitle(mode string, rows int) string {
	return "类型层次 / " + typeHierarchyModeLabel(mode) + " (" + strconv.Itoa(rows) + ")"
}

// typeHierRowDetail formats a row's dimmed right-hand locator: the package and
// "basename:line", skipping whichever parts the node lacks so the synthetic
// root shows nothing at all.
func typeHierRowDetail(n *TypeNode) string {
	if n == nil {
		return ""
	}
	loc := ""
	if n.File != "" {
		loc = filepath.Base(n.File)
		if n.Line > 0 {
			loc += ":" + strconv.Itoa(n.Line)
		}
	}
	switch {
	case n.Pkg != "" && loc != "":
		return n.Pkg + " · " + loc
	case n.Pkg != "":
		return n.Pkg
	}
	return loc
}

// typeHierKindGlyph is the one-character kind marker drawn before a name.
func typeHierKindGlyph(kind string) string {
	switch kind {
	case TypeKindInterface:
		return "I"
	case TypeKindStruct:
		return "S"
	case TypeKindMethod:
		return "M"
	}
	return "•"
}
