package a11y

// Walker is the minimum surface a tree-walkable widget needs to expose.
// gui.IWidget already implements every method here, so no widget-side
// glue is required to wire glui apps through Walk. Custom UI containers
// (graph items, designer fakes) implement the same shape and slot in.
type Walker interface {
	Children() []Walker
}

// genericWalker adapts an arbitrary value with a Children() method
// returning either []Walker or a slice of any concrete type whose
// elements themselves satisfy Walker. Apps using gui.IWidget go through
// AdaptIWidget below; custom hierarchies can implement Walker directly.

// Walk produces a *Node tree rooted at root. Hidden widgets are
// skipped — bridges only care about visible UI. Use WalkAll to include
// hidden subtrees (useful for designers and integration tests that
// want a full inventory).
//
// The walk is depth-first so child order in the returned tree matches
// the widget paint order. Recursive — applications with very deep
// trees should chunk via WalkSubtree if stack exhaustion is a concern.
func Walk(root interface{}) *Node {
	return walkInto(root, false)
}

// WalkAll behaves like Walk but does not skip hidden widgets. Use
// when you want a complete inventory regardless of current visibility.
func WalkAll(root interface{}) *Node {
	return walkInto(root, true)
}

func walkInto(root interface{}, includeHidden bool) *Node {
	if root == nil {
		return nil
	}

	state := readState(root)
	if !includeHidden && state.Has(StateHidden) {
		return nil
	}

	x, y, w, h := readBounds(root)
	n := &Node{
		Role:        roleOf(root),
		Name:        readName(root),
		Description: readDescription(root),
		Value:       readValue(root),
		State:       state,
		X:           x,
		Y:           y,
		W:           w,
		H:           h,
	}

	for _, c := range childrenOf(root) {
		if cn := walkInto(c, includeHidden); cn != nil {
			n.Children = append(n.Children, cn)
		}
	}
	return n
}

// roleOf resolves the widget's accessible role. Explicit
// AccessibleRole takes precedence; otherwise we infer from the
// concrete type name suffix or fall back to RoleUnknown so callers
// can still see the widget exists.
func roleOf(w interface{}) Role {
	if a, ok := w.(Accessible); ok {
		return a.AccessibleRole()
	}
	return guessRole(w)
}

// childrenOf returns the widget's child slice as a []interface{} so
// the walker doesn't depend on a single widget interface. Three
// shapes are supported:
//
//   - method Children() []interface{}
//   - method Children() []Walker
//   - method Children() of any slice type whose elements we can
//     interface-coerce one-by-one (handled by the gui.IWidget shim
//     in package gui via AdaptIWidget — kept out of this file to
//     avoid circular imports)
//
// Adapters for gui.IWidget live alongside the gui package; the helper
// here uses reflection-free direct interface checks so a11y has no
// knowledge of the gui type.
func childrenOf(w interface{}) []interface{} {
	if c, ok := w.(interface{ Children() []interface{} }); ok {
		return c.Children()
	}
	if c, ok := w.(interface{ Children() []Walker }); ok {
		out := make([]interface{}, 0, len(c.Children()))
		for _, k := range c.Children() {
			out = append(out, k)
		}
		return out
	}
	if c, ok := w.(childrenAdapter); ok {
		return c.AccessibleChildren()
	}
	return nil
}

// childrenAdapter is the escape hatch for hierarchies whose Children()
// returns a concrete slice type (gui.IWidget, graph.IItem, etc.).
// Adopters expose AccessibleChildren as a thin alias that returns
// []interface{}. Defining the helper here lets a11y stay free of
// downstream package imports.
type childrenAdapter interface {
	AccessibleChildren() []interface{}
}
